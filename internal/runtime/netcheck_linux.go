//go:build linux

package runtime

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/sys/unix"
	"tunnelstudio.local/studio/internal/config"
	"tunnelstudio.local/studio/internal/netcheck"
	"tunnelstudio.local/studio/internal/vault"
)

func tunnelSnapshot(v vault.Vault) map[string]any {
	m := map[string]any{"capturedAt": time.Now().UTC().Format(time.RFC3339Nano)}
	s, e := Call(v, "status")
	m["status"] = s
	if e != nil {
		m["statusError"] = e.Error()
	}
	stats := map[string]string{}
	for _, name := range []string{"rx_bytes", "tx_bytes", "rx_packets", "tx_packets", "rx_dropped", "tx_dropped", "rx_errors", "tx_errors"} {
		stats[name] = read("/sys/class/net/" + Interface + "/statistics/" + name)
	}
	m["interfaceCounters"] = stats
	return m
}

// NetworkCheck is explicitly invoked. It does not change policy, restart the
// tunnel, use account credentials, follow redirects, or persist private config.
func NetworkCheck(v vault.Vault, target string) (map[string]any, error) {
	started := time.Now()
	if _, e := v.Check(); e != nil {
		return nil, e
	}
	s, e := Call(v, "status")
	if e != nil || s["running"] != true {
		return nil, errors.New("请在隧道运行时检测；停止后不能复现代理路径")
	}
	if _, e = net.InterfaceByName(Interface); e != nil {
		return nil, errors.New("隧道接口不存在")
	}
	p, e := v.Read()
	if e != nil {
		return nil, e
	}
	c, e := config.Parse(p)
	if e != nil {
		return nil, e
	}
	if !autoProxy(c) {
		return nil, errors.New("联网检测目前要求自动代理模式，以便核对相同的策略路由")
	}
	dns := ""
	for _, a := range c.DNS {
		if a.Is4() {
			dns = a.String()
			break
		}
	}
	if dns == "" {
		return nil, errors.New("配置中没有 IPv4 DNS；不自动改用其他 DNS")
	}
	if target != "" {
		a, err := netip.ParseAddrPort(target)
		if err != nil || !a.Addr().Is4() || a.Port() == 0 || a.Addr().IsUnspecified() || a.Addr().IsMulticast() {
			return nil, errors.New("内网检测目标应为 IPv4:端口，例如 192.168.50.10:443")
		}
		covered := false
		for _, route := range c.Routes {
			if route.Contains(a.Addr()) {
				covered = true
			}
		}
		if !covered {
			return nil, errors.New("内网检测地址不在配置的 AllowedIPs 内")
		}
	}
	dialer := &net.Dialer{Timeout: 4 * time.Second, Control: func(_, _ string, raw syscall.RawConn) error {
		var sockErr error
		if e := raw.Control(func(fd uintptr) {
			// Both are mandatory: there must be no silent direct-uplink fallback.
			sockErr = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, Interface)
			if sockErr == nil {
				sockErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_MARK, 0x40000000)
			}
		}); e != nil {
			return e
		}
		return sockErr
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	report := map[string]any{"capturedAt": started.UTC().Format(time.RFC3339Nano), "version": Version, "interface": Interface, "boundToTunnel": true, "dnsServer": dns, "localHTTPAddress": target, "before": tunnelSnapshot(v), "limits": "IPv4 tunnel tests only, from U30 Air itself, not a tethered computer. Public HTTPS front doors are not authenticated Codex or streaming tests. HTTP 401/403 is an HTTP response, not a TCP/TLS failure. Optional local HTTP uses GET / without credentials, with small and 8192-byte probe headers; no response bodies are stored. At most 5 samples under a 20-second network budget; not a long-term stability test. No direct-uplink comparison."}
	results := make(chan []netcheck.TargetResult, 1)
	go func() { results <- netcheck.Run(ctx, dns, dialer.DialContext) }()
	samples := make([]map[string]any, 0, 5)
	for i := 0; i < 5 && ctx.Err() == nil; i++ {
		if i > 0 {
			timer := time.NewTimer(time.Until(started.Add(time.Duration(i) * 3 * time.Second)))
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
			if ctx.Err() != nil {
				break
			}
		}
		sample := map[string]any{"sequence": i + 1, "capturedAt": time.Now().UTC().Format(time.RFC3339Nano)}
		// Repeated UDP DNS requests expose intermittent loss hidden by one snapshot.
		sample["dnsUDP"] = netcheck.QueryDNS(ctx, dialer.DialContext, dns, "chatgpt.com", "udp", dnsmessage.TypeA)
		if target != "" {
			padding := 0
			if i%2 == 1 {
				padding = 8192
			}
			sample["probeHeaderBytes"] = padding
			sample["localHTTP"] = netcheck.CheckLocalHTTP(ctx, dialer.DialContext, target, padding)
		}
		sample["tunnel"] = tunnelSnapshot(v)
		samples = append(samples, sample)
	}
	report["targets"] = <-results
	report["samples"] = samples
	report["after"] = tunnelSnapshot(v)
	report["durationMs"] = time.Since(started).Milliseconds()
	report["ipv6Mode"] = c.IPv6Mode
	report["mtu"] = c.MTU
	// Only allowlisted, non-secret peer settings are included.
	peers := []map[string]string{}
	for i, p := range c.Peers {
		peers = append(peers, map[string]string{"id": strconv.Itoa(i + 1), "persistentKeepalive": p["PersistentKeepalive"]})
	}
	report["peerSettings"] = peers
	return report, nil
}
