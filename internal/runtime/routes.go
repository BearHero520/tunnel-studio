package runtime

import (
	"net/netip"
	"sort"
	"strings"
	"tunnelstudio.local/studio/internal/config"
)

// RoutePlan changes only a reserved table. Throw routes preserve endpoint and LAN access.
func RoutePlan(c *config.Parsed, uapi string) [][]string {
	if len(c.Sources) == 0 {
		return nil
	}
	bypass := map[string]netip.Prefix{}
	for _, s := range c.Sources {
		bypass[s.String()] = s
	}
	for _, line := range strings.Split(uapi, "\n") {
		if strings.HasPrefix(line, "endpoint=") {
			ep, e := netip.ParseAddrPort(strings.TrimPrefix(line, "endpoint="))
			if e == nil {
				a := ep.Addr()
				p := netip.PrefixFrom(a, a.BitLen())
				bypass[p.String()] = p
			}
		}
	}
	var out [][]string
	for _, r := range c.Routes {
		if _, ok := bypass[r.String()]; ok {
			continue
		}
		f := "-6"
		if r.Addr().Is4() {
			f = "-4"
		}
		out = append(out, []string{f, "route", "add", r.String(), "dev", Interface, "table", "18460", "proto", "186"})
	}
	var keys []string
	for k := range bypass {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p := bypass[k]
		f := "-6"
		if p.Addr().Is4() {
			f = "-4"
		}
		out = append(out, []string{f, "route", "add", "throw", p.String(), "table", "18460", "proto", "186"})
	}
	for _, s := range c.Sources {
		f := "-6"
		if s.Addr().Is4() {
			f = "-4"
		}
		out = append(out, []string{f, "rule", "add", "priority", "18460", "from", s.String(), "lookup", "18460"})
	}
	return out
}
