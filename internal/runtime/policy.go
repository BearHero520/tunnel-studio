package runtime

import (
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"tunnelstudio.local/studio/internal/config"
)

const ProxyMark = "0x40000000/0x40000000"
const TransportMark = "0x20000000/0x20000000"
const ProxyPriority = "9000"

type Operation struct {
	Tool     string   `json:"tool"`
	Args     []string `json:"args"`
	Undo     []string `json:"undo"`
	Expected string   `json:"expected,omitempty"`
}

func autoProxy(c *config.Parsed) bool {
	return c.NetworkMode == "device" || c.NetworkMode == "lan" || c.NetworkMode == "gateway"
}
func hostProxy(c *config.Parsed) bool { return c.NetworkMode == "device" || c.NetworkMode == "gateway" }
func lanProxy(c *config.Parsed) bool  { return c.NetworkMode == "lan" || c.NetworkMode == "gateway" }

// ProxyPlan is a deterministic, owned, reversible policy. Hooks are attached last.
// The two masked mark bits never replace Android's lower network-selection bits.
func ProxyPlan(c *config.Parsed, uapi string, local []netip.Prefix) []Operation {
	if !autoProxy(c) {
		var out []Operation
		for _, a := range RoutePlan(c, uapi) {
			b := append([]string(nil), a...)
			b[2] = "del"
			out = append(out, Operation{Tool: "ip", Args: a, Undo: b})
		}
		return out
	}
	var out, hooks []Operation
	addIP := func(a ...string) {
		b := append([]string(nil), a...)
		b[2] = "del"
		out = append(out, Operation{Tool: "ip", Args: a, Undo: b})
	}
	fw := func(tool, table, verb, chain string, args ...string) Operation {
		a := append([]string{"-w", "3", "-t", table, verb, chain}, args...)
		b := append([]string(nil), a...)
		switch verb {
		case "-N":
			b[4] = "-X"
		case "-A", "-I":
			b[4] = "-D"
		}
		return Operation{Tool: tool, Args: a, Undo: b}
	}
	for _, family := range []string{"-4", "-6"} {
		v4 := family == "-4"
		tool := "ip6tables"
		if v4 {
			tool = "iptables"
		}
		bypass := map[string]bool{}
		for _, s := range []string{"0.0.0.0/8", "127.0.0.0/8", "169.254.0.0/16", "224.0.0.0/4", "240.0.0.0/4", "::/128", "::1/128", "fe80::/10", "ff00::/8"} {
			p := netip.MustParsePrefix(s)
			if p.Addr().Is4() == v4 {
				bypass[p.String()] = true
			}
		}
		for _, p := range local {
			if p.Addr().Is4() == v4 && p.Bits() > 0 {
				bypass[p.Masked().String()] = true
			}
		}
		for _, line := range strings.Split(uapi, "\n") {
			if strings.HasPrefix(line, "endpoint=") {
				a, e := netip.ParseAddrPort(strings.TrimPrefix(line, "endpoint="))
				if e == nil && a.Addr().Is4() == v4 {
					bypass[netip.PrefixFrom(a.Addr(), a.Addr().BitLen()).String()] = true
				}
			}
		}
		var excluded []string
		for s := range bypass {
			excluded = append(excluded, s)
		}
		sort.Strings(excluded)
		if !v4 && c.IPv6Mode == "block" {
			out = append(out, fw(tool, "filter", "-N", "KTS_BLOCK"), fw(tool, "filter", "-A", "KTS_BLOCK", "-m", "mark", "--mark", TransportMark, "-j", "RETURN"))
			for _, s := range excluded {
				out = append(out, fw(tool, "filter", "-A", "KTS_BLOCK", "-d", s, "-j", "RETURN"))
			}
			out = append(out, fw(tool, "filter", "-A", "KTS_BLOCK", "-j", "REJECT"))
			if hostProxy(c) {
				hooks = append(hooks, fw(tool, "filter", "-I", "OUTPUT", "-j", "KTS_BLOCK"))
			}
			if lanProxy(c) {
				hooks = append(hooks, fw(tool, "filter", "-I", "FORWARD", "-i", c.LANInterface, "-j", "KTS_BLOCK"))
			}
			continue
		}
		for _, r := range c.Routes {
			if r.Addr().Is4() == v4 {
				addIP(family, "route", "add", r.String(), "dev", Interface, "table", "18460", "proto", "186")
			}
		}
		addIP(family, "rule", "add", "priority", ProxyPriority, "fwmark", ProxyMark, "lookup", "18460")
		out = append(out, fw(tool, "mangle", "-N", "KTS_ROUTE"), fw(tool, "mangle", "-A", "KTS_ROUTE", "-m", "mark", "--mark", TransportMark, "-j", "RETURN"))
		// DNS is marked before LAN bypass so queries to the gateway can be DNATed.
		dns := ""
		for _, a := range c.DNS {
			if a.Is4() == v4 {
				dns = a.String()
				break
			}
		}
		// Protect all resolved endpoints before any DNS interception, including UDP/53.
		for _, line := range strings.Split(uapi, "\n") {
			if strings.HasPrefix(line, "endpoint=") {
				a, e := netip.ParseAddrPort(strings.TrimPrefix(line, "endpoint="))
				if e == nil && a.Addr().Is4() == v4 {
					out = append(out, fw(tool, "mangle", "-A", "KTS_ROUTE", "-d", a.Addr().String(), "-j", "RETURN"))
				}
			}
		}
		if dns != "" {
			for _, p := range []string{"udp", "tcp"} {
				out = append(out, fw(tool, "mangle", "-A", "KTS_ROUTE", "-p", p, "--dport", "53", "-j", "MARK", "--set-xmark", ProxyMark), fw(tool, "mangle", "-A", "KTS_ROUTE", "-p", p, "--dport", "53", "-j", "RETURN"))
			}
		}
		for _, s := range excluded {
			out = append(out, fw(tool, "mangle", "-A", "KTS_ROUTE", "-d", s, "-j", "RETURN"))
		}
		for _, r := range c.Routes {
			if r.Addr().Is4() == v4 {
				out = append(out, fw(tool, "mangle", "-A", "KTS_ROUTE", "-d", r.String(), "-j", "MARK", "--set-xmark", ProxyMark))
			}
		}
		nat := v4 || c.IPv6Mode == "nat"
		mtu := c.MTU
		if mtu == 0 {
			mtu = 1280
		}
		mss := mtu - 60
		if v4 {
			mss = mtu - 40
		}
		out = append(out, fw(tool, "mangle", "-N", "KTS_MSS"), fw(tool, "mangle", "-A", "KTS_MSS", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", strconv.Itoa(mss)))
		hooks = append(hooks, fw(tool, "mangle", "-I", "POSTROUTING", "-o", Interface, "-m", "mark", "--mark", ProxyMark, "-j", "KTS_MSS"))
		if lanProxy(c) {
			// Clamp the remote SYN/SYN-ACK too: it controls LAN uploads.
			// Return packets do not carry the outbound proxy mark.
			hooks = append(hooks, fw(tool, "mangle", "-I", "FORWARD", "-i", Interface, "-o", c.LANInterface, "-j", "KTS_MSS"))
		}
		if nat {
			out = append(out, fw(tool, "nat", "-N", "KTS_SNAT"), fw(tool, "nat", "-A", "KTS_SNAT", "-j", "MASQUERADE"))
			hooks = append(hooks, fw(tool, "nat", "-I", "POSTROUTING", "-o", Interface, "-m", "mark", "--mark", ProxyMark, "-j", "KTS_SNAT"))
		}
		if dns != "" {
			out = append(out, fw(tool, "nat", "-N", "KTS_DNS"))
			for _, p := range []string{"udp", "tcp"} {
				out = append(out, fw(tool, "nat", "-A", "KTS_DNS", "-p", p, "--dport", "53", "-j", "DNAT", "--to-destination", dns))
			}
			if hostProxy(c) {
				hooks = append(hooks, fw(tool, "nat", "-I", "OUTPUT", "-m", "mark", "--mark", ProxyMark, "-j", "KTS_DNS"))
			}
			if lanProxy(c) {
				hooks = append(hooks, fw(tool, "nat", "-I", "PREROUTING", "-i", c.LANInterface, "-m", "mark", "--mark", ProxyMark, "-j", "KTS_DNS"))
			}
		}
		if lanProxy(c) {
			out = append(out, fw(tool, "filter", "-N", "KTS_FWD"), fw(tool, "filter", "-A", "KTS_FWD", "-i", c.LANInterface, "-o", Interface, "-m", "mark", "--mark", ProxyMark, "-j", "ACCEPT"), fw(tool, "filter", "-A", "KTS_FWD", "-i", Interface, "-o", c.LANInterface, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"))
			hooks = append(hooks, fw(tool, "filter", "-I", "FORWARD", "-j", "KTS_FWD"))
		}
		// Attach traffic selectors only after NAT and forwarding are in place.
	}
	for _, family := range []string{"-4", "-6"} {
		if family == "-6" && c.IPv6Mode == "block" {
			continue
		}
		tool := "iptables"
		if family == "-6" {
			tool = "ip6tables"
		}
		if lanProxy(c) {
			hooks = append(hooks, fw(tool, "mangle", "-I", "PREROUTING", "-i", c.LANInterface, "-j", "KTS_ROUTE"))
		}
		if hostProxy(c) {
			hooks = append(hooks, fw(tool, "mangle", "-I", "OUTPUT", "-j", "KTS_ROUTE"))
		}
	}
	return append(out, hooks...)
}
