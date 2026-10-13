//go:build linux

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"tunnelstudio.local/studio/internal/config"
	"tunnelstudio.local/studio/internal/vault"
)

func openBootFile(path string, flags int) (*os.File, error) {
	fd, e := unix.Open(path, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if e != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: e}
	}
	return os.NewFile(uintptr(fd), path), nil
}
func lockBootFile(f *os.File) (func(), error) {
	if e := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		// Android emulated storage may not implement flock. The CLI still holds
		// the private manager lock and updateBootHook rechecks the shared bytes.
		if errors.Is(e, unix.ENOSYS) || errors.Is(e, unix.EOPNOTSUPP) {
			return func() {}, nil
		}
		return nil, fmt.Errorf("无法锁定自启脚本：%w", e)
	}
	return func() { unix.Flock(int(f.Fd()), unix.LOCK_UN) }, nil
}
func CancelBoot(v vault.Vault) error {
	if _, e := v.Check(); e != nil {
		return e
	}
	return vault.Atomic(v.Path("runtime/boot-cancel"), []byte(read("/proc/sys/kernel/random/boot_id")), 0600)
}
func BootStart(v vault.Vault) error {
	id, e := v.Check()
	if e != nil {
		return e
	}
	unlock, e := Lock(v.Path("runtime/boot-worker.lock"))
	if e != nil {
		// KANO and Magisk may both invoke the same worker.
		if errors.Is(e, errOperationBusy) {
			return nil
		}
		return e
	}
	defer unlock()
	bootID := read("/proc/sys/kernel/random/boot_id")
	record := BootRecord{OwnerID: id.ID, BootID: bootID, Version: Version}
	runner := bootRunner{
		now: time.Now, sleep: time.Sleep,
		save: func(record BootRecord) error {
			b, e := json.MarshalIndent(record, "", "  ")
			if e != nil {
				return e
			}
			return vault.Atomic(v.Path("runtime/boot-status.json"), b, 0600)
		},
		step: func() bootStep { return bootAttempt(v, id.ID, bootID) },
	}
	return runner.run(record)
}

func bootAttempt(v vault.Vault, ownerID, bootID string) bootStep {
	unlock, e := Lock(v.Path("manager.lock"))
	if e != nil {
		if errors.Is(e, errOperationBusy) {
			return bootStep{"waiting", "管理操作正在执行"}
		}
		return bootStep{"failed", "无法安全取得管理锁"}
	}
	defer unlock()
	if _, e = v.Check(); e != nil {
		return bootStep{"failed", "自启资源所有权检查失败"}
	}
	if bootID == "unavailable" {
		return bootStep{"failed", "无法取得内核 boot ID"}
	}
	info := BootStatus(v)
	if !info.Enabled {
		return bootStep{"cancelled", "开机自启已关闭或入口失效"}
	}
	if b, e := readBootFile(v.Path("runtime/boot-cancel")); e == nil && string(b) == bootID {
		return bootStep{"cancelled", "本次开机已手动停止，不再自动重试"}
	} else if e != nil && !os.IsNotExist(e) {
		return bootStep{"failed", "无法读取自启取消记录"}
	}
	r, e := v.Release()
	if e != nil {
		return bootStep{"failed", "资源版本清单无效"}
	}
	exe, e := os.Executable()
	if e != nil {
		return bootStep{"failed", "无法确定自启程序"}
	}
	hash, e := vault.Digest(exe)
	if e != nil || hash != r.Current {
		return bootStep{"failed", "自启资源已过时；请重新设置自启"}
	}
	if _, e = Call(v, "status"); e == nil {
		return bootStep{"already-running", "隧道已经运行"}
	}
	daemonUnlock, e := Lock(v.Path("runtime/daemon.lock"))
	if e != nil {
		if errors.Is(e, errOperationBusy) {
			return bootStep{"waiting", "隧道进程正在启动或清理"}
		}
		return bootStep{"failed", "无法安全取得隧道进程锁"}
	}
	daemonUnlock()
	if e = Stopped(v); e != nil {
		return bootStep{"failed", e.Error()}
	}
	if e = bootJournalSafe(v, ownerID, bootID); e != nil {
		return bootStep{"failed", e.Error()}
	}
	p, e := v.Read()
	if e != nil {
		return bootStep{"failed", "已保存配置无效；请读取并重新保存"}
	}
	c, e := config.Parse(p)
	if e != nil {
		return bootStep{"failed", "已保存配置无法解析"}
	}
	if e = bootNetworkReady(c); e != nil {
		return bootStep{"waiting", e.Error()}
	}
	if e = Start(v); e == nil {
		return bootStep{"started", "开机启动完成；请另行确认握手和实际联网"}
	}
	if journalErr := bootJournalSafe(v, ownerID, bootID); journalErr != nil {
		return bootStep{"failed", journalErr.Error()}
	}
	return bootStep{"waiting", "启动未完成：" + e.Error()}
}

func bootNetworkReady(c *config.Parsed) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, "/system/bin/getprop", "sys.boot_completed").Output()
	if e != nil || strings.TrimSpace(string(b)) != "1" {
		return errors.New("等待 Android 完成开机")
	}
	if lanProxy(c) {
		iface, e := net.InterfaceByName(c.LANInterface)
		if e != nil || iface.Flags&net.FlagUp == 0 {
			return errors.New("等待热点接口就绪")
		}
		addresses, e := iface.Addrs()
		ready := false
		for _, addr := range addresses {
			if p, e := netip.ParsePrefix(addr.String()); e == nil && p.Addr().Is4() && !p.Addr().IsUnspecified() {
				ready = true
			}
		}
		if e != nil || !ready {
			return errors.New("等待热点 IPv4 地址就绪")
		}
	}
	for _, peer := range c.Peers {
		endpoint := peer["Endpoint"]
		if endpoint == "" {
			continue
		}
		endpoint, e := resolve(endpoint)
		if e != nil {
			return errors.New("等待隧道端点 DNS 可用")
		}
		host, _, _ := net.SplitHostPort(endpoint)
		family := "-4"
		if a, e := netip.ParseAddr(host); e == nil && a.Is6() {
			family = "-6"
		}
		route, e := ip(family, "route", "get", host, "mark", "0x20000000")
		if e != nil || !strings.Contains(route, " dev ") || strings.Contains(route, "dev "+Interface+" ") {
			return errors.New("等待隧道端点的外部路由可用")
		}
	}
	return nil
}
