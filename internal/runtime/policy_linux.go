//go:build linux

package runtime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"time"
	"tunnelstudio.local/studio/internal/config"
)

var errNetworkUncertain = errors.New("网络操作超时，需检查事务记录")

func command(tool string, args ...string) (string, error) {
	if tool != "ip" && tool != "iptables" && tool != "ip6tables" {
		return "", errors.New("未知网络工具")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, "/system/bin/"+tool, args...).CombinedOutput()
	if ctx.Err() != nil {
		return "", fmt.Errorf("%s: %w", tool, errNetworkUncertain)
	}
	if e != nil {
		return "", fmt.Errorf("%s %s 失败：%s", tool, strings.Join(args, " "), strings.TrimSpace(string(b)))
	}
	return string(b), nil
}
func runOperation(tool string, args []string) error {
	if tool == "sysctl" {
		if len(args) != 2 || !strings.HasPrefix(args[0], "/proc/sys/net/") {
			return errors.New("无效 sysctl")
		}
		return os.WriteFile(args[0], []byte(args[1]+"\n"), 0600)
	}
	if tool == "" {
		tool = "ip"
	}
	_, e := command(tool, args...)
	return e
}
func undoOperation(op undo) error {
	if op.Tool == "sysctl" && read(op.Args[0]) != op.Expected {
		return errors.New("系统转发设置已被其他操作修改，不覆盖")
	}
	return runOperation(op.Tool, op.Args)
}
func prepareProxy(c *config.Parsed) ([]netip.Prefix, []Operation, error) {
	if !autoProxy(c) {
		return nil, nil, nil
	}
	var local []netip.Prefix
	var prep []Operation
	if lanProxy(c) {
		if _, e := net.InterfaceByName(c.LANInterface); e != nil {
			return nil, nil, errors.New("未找到热点接口 " + c.LANInterface)
		}
	}
	interfaces, e := net.Interfaces()
	if e != nil {
		return nil, nil, e
	}
	for _, iface := range interfaces {
		if iface.Name == Interface {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			p, e := netip.ParsePrefix(a.String())
			if e != nil {
				continue
			}
			if iface.Name == c.LANInterface {
				local = append(local, p.Masked())
			} else {
				local = append(local, netip.PrefixFrom(p.Addr(), p.Addr().BitLen()))
			}
		}
	}
	// Refuse to adopt existing chains or mark bits owned by another service.
	for _, tool := range []string{"iptables", "ip6tables"} {
		tables := []string{"filter", "mangle", "nat"}
		if tool == "ip6tables" && c.IPv6Mode == "block" {
			tables = []string{"filter"}
		}
		if tool == "ip6tables" && c.IPv6Mode == "routed" {
			tables = []string{"filter", "mangle"}
		}
		for _, table := range tables {
			s, e := command(tool, "-w", "3", "-t", table, "-S")
			if e != nil {
				return nil, nil, fmt.Errorf("缺少 %s %s 支持；IPv6 NAT 不可用时可选择阻止 IPv6。%w", tool, table, e)
			}
			if strings.Contains(s, "KTS_") || strings.Contains(s, "0x40000000") || strings.Contains(s, "0x20000000") {
				return nil, nil, errors.New("检测到已有 Tunnel Studio 链或保留标记，拒绝覆盖")
			}
		}
	}
	sysctl := func(path, value string) error {
		old := read(path)
		if old == value {
			return nil
		}
		if old != "0" && old != "1" && old != "2" {
			return errors.New("无法读取网络设置 " + path)
		}
		prep = append(prep, Operation{Tool: "sysctl", Args: []string{path, value}, Undo: []string{path, old}, Expected: value})
		return nil
	}
	if lanProxy(c) {
		if e := sysctl("/proc/sys/net/ipv4/ip_forward", "1"); e != nil {
			return nil, nil, e
		}
		if c.IPv6Mode != "block" && read("/proc/sys/net/ipv6/conf/all/forwarding") != "1" {
			// Preserve router advertisements when enabling forwarding.
			for _, iface := range interfaces {
				path := "/proc/sys/net/ipv6/conf/" + iface.Name + "/accept_ra"
				if read(path) == "1" {
					if e := sysctl(path, "2"); e != nil {
						return nil, nil, e
					}
				}
			}
			if read("/proc/sys/net/ipv6/conf/default/accept_ra") == "1" {
				if e := sysctl("/proc/sys/net/ipv6/conf/default/accept_ra", "2"); e != nil {
					return nil, nil, e
				}
			}
			if e := sysctl("/proc/sys/net/ipv6/conf/all/forwarding", "1"); e != nil {
				return nil, nil, e
			}
		}
	}
	return local, prep, nil
}
