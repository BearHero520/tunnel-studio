package config

import (
	"errors"
	"net/netip"
	"regexp"
	"strings"
)

func parseNetwork(p Profile, c *Parsed) error {
	c.NetworkMode = p.NetworkMode
	if c.NetworkMode == "" {
		c.NetworkMode = "tunnel"
		if len(c.Sources) > 0 {
			c.NetworkMode = "sources"
		}
	}
	switch c.NetworkMode {
	case "tunnel", "sources", "device", "lan", "gateway":
	default:
		return errors.New("流量模式无效")
	}
	if c.NetworkMode == "tunnel" || c.NetworkMode == "sources" {
		if c.NetworkMode == "tunnel" && len(c.Sources) > 0 {
			return errors.New("仅隧道模式不能同时填写来源网段")
		}
		return nil
	}
	if len(c.Sources) > 0 {
		return errors.New("自动代理模式不能同时填写来源网段")
	}
	c.LANInterface = p.LANInterface
	if c.LANInterface == "" {
		c.LANInterface = "br0"
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,14}$`).MatchString(c.LANInterface) || c.LANInterface == "lo" || c.LANInterface == "kts0" {
		return errors.New("热点接口名称无效")
	}
	c.IPv6Mode = p.IPv6Mode
	if c.IPv6Mode == "" {
		c.IPv6Mode = "block"
	}
	if c.IPv6Mode != "block" && c.IPv6Mode != "nat" && c.IPv6Mode != "routed" {
		return errors.New("IPv6 模式无效")
	}
	if c.IPv6Mode == "routed" && c.NetworkMode != "lan" {
		return errors.New("免 NAT66 的 IPv6 路由模式只用于热点，服务端需配置热点前缀回程路由；本机代理请选择阻止 IPv6 或 NAT66")
	}
	has4, has6 := false, false
	for _, a := range c.Addresses {
		if a.Addr().Is4() {
			has4 = true
		} else {
			has6 = true
		}
	}
	route4, route6 := false, false
	for _, r := range c.Routes {
		if r.Addr().Is4() {
			route4 = true
		} else {
			route6 = true
		}
	}
	if !has4 || !route4 {
		return errors.New("自动代理模式需要 IPv4 Address 和 IPv4 AllowedIPs")
	}
	if c.IPv6Mode != "block" && (!has6 || !route6) {
		return errors.New("IPv6 代理需要 IPv6 Address 和 IPv6 AllowedIPs")
	}
	// Replace the legacy tunnel-only DNS warning with the active proxy semantics.
	warnings := c.Warnings[:0]
	for _, w := range c.Warnings {
		if !strings.HasPrefix(w, "DNS 仅保存供参考") {
			warnings = append(warnings, w)
		}
	}
	c.Warnings = warnings
	for _, s := range strings.Split(c.Interface["DNS"], ",") {
		if strings.TrimSpace(s) == "" {
			continue
		}
		a, e := netip.ParseAddr(strings.TrimSpace(s))
		if e != nil {
			return e
		}
		if !a.Is4() && c.IPv6Mode == "block" {
			continue
		}
		if !a.Is4() && c.IPv6Mode == "routed" {
			return errors.New("免 NAT66 路由模式请使用 IPv4 DNS，IPv6 DNS 重定向需要 NAT 表")
		}
		covered := false
		for _, r := range c.Routes {
			if r.Contains(a) {
				covered = true
			}
		}
		if !covered {
			return errors.New("DNS 地址必须包含在同族 AllowedIPs 内")
		}
		c.DNS = append(c.DNS, a)
	}
	if len(c.DNS) == 0 {
		if c.IPv6Mode == "block" && strings.TrimSpace(c.Interface["DNS"]) != "" {
			return errors.New("当前阻止外网 IPv6，但 DNS 只配置了 IPv6 地址；请填写可用的 IPv4 DNS 或启用 IPv6 代理")
		}
		c.Warnings = append(c.Warnings, "未配置可用 DNS：不重定向 DNS，请确认现有解析器可用。")
	}
	if c.IPv6Mode == "block" {
		c.Warnings = append(c.Warnings, "所选范围的外网 IPv6 将被阻止以避免绕过 IPv4 隧道；局域网和隧道传输保留。")
	}
	if c.IPv6Mode == "routed" {
		c.Warnings = append(c.Warnings, "IPv6 不使用 NAT66；服务端 Peer AllowedIPs 和回程路由必须覆盖热点 IPv6 前缀。")
	}
	return nil
}
