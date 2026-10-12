//go:build linux

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"github.com/amnezia-vpn/amneziawg-go/v3/device"
	"github.com/amnezia-vpn/amneziawg-go/v3/tun"
	"golang.org/x/sys/unix"
	"tunnelstudio.local/studio/internal/config"
	"tunnelstudio.local/studio/internal/vault"
)

func RootCheck() error {
	if os.Geteuid() != 0 {
		return errors.New("需要 KANO 已获授权的 root 执行通道")
	}
	return nil
}
func Lock(path string) (func(), error) {
	fd, e := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); e != nil {
		unix.Close(fd)
		return nil, errors.New("另一个操作正在执行")
	}
	return func() { unix.Flock(fd, unix.LOCK_UN); unix.Close(fd) }, nil
}
func Stopped(v vault.Vault) error {
	unlock, e := Lock(v.Path("runtime/daemon.lock"))
	if e != nil {
		return errors.New("请先停止本插件隧道")
	}
	unlock()
	if _, e := os.Stat("/sys/class/net/" + Interface); e == nil {
		return errors.New("kts0 已存在或未完成清理；拒绝接管")
	}
	return nil
}
func ip(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, e := exec.CommandContext(ctx, "/system/bin/ip", args...).CombinedOutput()
	if e != nil {
		return "", fmt.Errorf("ip %s 执行失败", strings.Join(args, " "))
	}
	return string(out), nil
}
func read(path string) string {
	b, e := os.ReadFile(path)
	if e != nil {
		return "unavailable"
	}
	return strings.TrimSpace(string(b))
}
func Diagnose(v vault.Vault) map[string]any {
	m := map[string]any{"uid": os.Geteuid(), "interface": Interface, "selinux": read("/sys/fs/selinux/enforce"), "ipv4Forward": read("/proc/sys/net/ipv4/ip_forward"), "ipv6Forward": read("/proc/sys/net/ipv6/conf/all/forwarding"), "nat66Required": false, "version": Version}
	for _, p := range []string{"/dev/net/tun", "/dev/tun"} {
		f, e := os.Stat(p)
		if e != nil {
			m[p] = "unavailable"
		} else {
			m[p] = f.Mode().String()
		}
	}
	for _, a := range [][]string{{"-4", "rule", "show"}, {"-6", "rule", "show"}, {"-brief", "link", "show"}} {
		s, e := ip(a...)
		if e != nil {
			s = e.Error()
		}
		m[strings.Join(a, " ")] = s
	}
	if _, e := v.Check(); e == nil {
		m["owned"] = true
		m["status"], _ = Call(v, "status")
	}
	return m
}
func Start(v vault.Vault) error {
	if e := Stopped(v); e != nil {
		return e
	}
	p, e := v.Read()
	if e != nil {
		return e
	}
	if _, e = config.Parse(p); e != nil {
		return e
	}
	r, e := v.Release()
	if e != nil {
		return e
	}
	bin, e := v.Binary(r.Current)
	if e != nil {
		return e
	}
	if er := vault.NoLink(v.Path("runtime/last-start.json")); er != nil && !os.IsNotExist(er) {
		return er
	}
	out, e := os.OpenFile(v.Path("runtime/last-start.json"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if e != nil {
		return e
	}
	defer out.Close()
	cmd := exec.Command(bin, "serve")
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if e = cmd.Start(); e != nil {
		return e
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	for i := 0; i < 100; i++ {
		if _, e = Call(v, "status"); e == nil {
			return nil
		}
		select {
		case <-exited:
			var failure Reply
			if b, er := os.ReadFile(v.Path("runtime/last-start.json")); er == nil && json.Unmarshal(b, &failure) == nil && failure.Error != "" {
				return errors.New(failure.Error)
			}
			return errors.New("隧道进程在就绪前退出，请查看诊断")
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("启动未就绪；查看只读诊断及 runtime/last-start.json。若进程仍在启动，请稍后刷新")
}
func resolve(s string) (string, error) {
	host, port, e := net.SplitHostPort(s)
	if e != nil {
		return "", e
	}
	if a, e := netip.ParseAddr(host); e == nil {
		return net.JoinHostPort(a.String(), port), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if e != nil || len(ips) == 0 {
		return "", errors.New("设备 DNS 无法解析")
	}
	return net.JoinHostPort(ips[0].String(), port), nil
}

type undo struct {
	Args []string `json:"args"`
}
type journal struct {
	BootID     string `json:"bootID"`
	OwnerID    string `json:"ownerID"`
	Operations []undo `json:"operations"`
}

func routesAvailable() error {
	for _, f := range []string{"-4", "-6"} {
		s, e := ip(f, "rule", "show")
		if e != nil {
			return e
		}
		for _, line := range strings.Split(s, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "18460:") || strings.Contains(line, "lookup 18460") {
				return errors.New("路由优先级或表 18460 已被占用")
			}
		}
		s, e = ip(f, "route", "show", "table", "18460")
		if e == nil && strings.TrimSpace(s) != "" {
			return errors.New("独立路由表已被占用")
		}
	}
	return nil
}
func Serve(v vault.Vault) error {
	id, e := v.Check()
	if e != nil {
		return e
	}
	unlock, e := Lock(v.Path("runtime/daemon.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	if _, e = os.Stat("/sys/class/net/" + Interface); e == nil {
		return errors.New("接口已存在，拒绝接管")
	}
	bootID := read("/proc/sys/kernel/random/boot_id")
	if b, er := os.ReadFile(v.Path("runtime/routes.json")); er == nil {
		var old journal
		if json.Unmarshal(b, &old) != nil || old.OwnerID != id.ID || old.BootID == "" || bootID == "unavailable" || old.BootID == bootID {
			return errors.New("发现本次启动中的未完成路由事务，请按恢复文档核对；不自动删除可能重用的规则")
		}
		// Routes do not survive kernel reboot. Still check the table for conflicts below.
		if e = os.Remove(v.Path("runtime/routes.json")); e != nil {
			return e
		}
	} else if !os.IsNotExist(er) {
		return er
	}
	if bootID == "unavailable" {
		return errors.New("无法取得内核 boot ID，无法安全记录资源所有权")
	}
	p, e := v.Read()
	if e != nil {
		return e
	}
	c, e := config.Parse(p)
	if e != nil {
		return e
	}
	uapi, e := c.UAPI(resolve)
	if e != nil {
		return e
	}
	if len(c.Sources) > 0 {
		if e = routesAvailable(); e != nil {
			return e
		}
	}
	t, e := tun.CreateTUN(Interface, c.MTU)
	if e != nil {
		return errors.New("无法创建独立 TUN；检查 root、TUN 权限和 SELinux")
	}
	logger := &device.Logger{Verbosef: func(string, ...any) {}, Errorf: func(string, ...any) {}}
	d := device.NewDevice(t, conn.NewDefaultBind(), logger)
	defer d.Close()
	if e = d.IpcSet(uapi); e != nil {
		return errors.New("固定后端拒绝配置")
	}
	if _, e = ip("link", "set", "dev", Interface, "alias", vault.Owner+":"+id.ID); e != nil {
		return e
	}
	for _, a := range c.Addresses {
		if _, e = ip("address", "add", a.String(), "dev", Interface, "noprefixroute"); e != nil {
			return e
		}
	}
	if _, e = ip("link", "set", "dev", Interface, "up"); e != nil {
		return e
	}
	if e = d.Up(); e != nil {
		return errors.New("无法打开 UDP 传输")
	}
	var applied []undo
	ambiguous := false
	defer func() {
		clean := !ambiguous
		for i := len(applied) - 1; i >= 0; i-- {
			if _, e := ip(applied[i].Args...); e != nil {
				clean = false
			}
		}
		if clean {
			os.Remove(v.Path("runtime/routes.json"))
		}
	}()
	for _, args := range RoutePlan(c, uapi) {
		inverse := append([]string(nil), args...)
		inverse[2] = "del"
		next := append(applied, undo{inverse})
		raw, _ := json.Marshal(journal{BootID: bootID, OwnerID: id.ID, Operations: next})
		if e = vault.Atomic(v.Path("runtime/routes.json"), raw, 0600); e != nil {
			return e
		}
		if _, e = ip(args...); e != nil { // Keep journal on ambiguous failure for explicit recovery.
			ambiguous = true
			return e
		}
		applied = next
	}
	sock := v.Path("runtime/control.sock")
	if e = vault.NoLink(sock); e == nil {
		if f, _ := os.Lstat(sock); f.Mode()&os.ModeSocket == 0 {
			return errors.New("控制通道路径已被其他文件占用")
		}
		os.Remove(sock)
	} else if !os.IsNotExist(e) {
		return e
	}
	listener, e := net.Listen("unix", sock)
	if e != nil {
		return e
	}
	defer listener.Close()
	defer os.Remove(sock)
	if e = os.Chmod(sock, 0600); e != nil {
		return e
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(stop)
	go func() {
		select {
		case <-stop:
			listener.Close()
		case <-d.Wait():
			listener.Close()
		}
	}()
	for {
		client, e := listener.Accept()
		if e != nil {
			return nil
		}
		client.SetDeadline(time.Now().Add(3 * time.Second))
		var req struct {
			Action string `json:"action"`
		}
		e = json.NewDecoder(io.LimitReader(client, 2048)).Decode(&req)
		if e != nil {
			client.Close()
			continue
		}
		if req.Action == "stop" {
			json.NewEncoder(client).Encode(map[string]any{"stopping": true})
			client.Close()
			return nil
		}
		if req.Action == "status" {
			raw, e := d.IpcGet()
			status := map[string]any{"running": e == nil, "protocol": p.Protocol, "interface": Interface, "version": Version, "sources": p.Sources}
			peers := []map[string]string{}
			var peer map[string]string
			for _, line := range strings.Split(raw, "\n") {
				kv := strings.SplitN(line, "=", 2)
				if len(kv) != 2 {
					continue
				}
				if kv[0] == "public_key" {
					peer = map[string]string{"id": strconv.Itoa(len(peers) + 1)}
					peers = append(peers, peer)
				}
				if peer != nil {
					switch kv[0] {
					case "last_handshake_time_sec", "rx_bytes", "tx_bytes":
						peer[kv[0]] = kv[1]
					}
				}
			}
			status["peers"] = peers
			json.NewEncoder(client).Encode(status)
		} else {
			json.NewEncoder(client).Encode(map[string]any{"error": "unsupported action"})
		}
		client.Close()
	}
}
