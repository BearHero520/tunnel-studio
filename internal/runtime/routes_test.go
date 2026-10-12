package runtime

import (
	"net/netip"
	"strings"
	"testing"
	"tunnelstudio.local/studio/internal/config"
)

func TestScopedRoutes(t *testing.T) {
	c := &config.Parsed{Routes: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0"), netip.MustParsePrefix("::/0")}}
	if len(RoutePlan(c, "")) != 0 {
		t.Fatal("manual mode changed routes")
	}
	c.Sources = []netip.Prefix{netip.MustParsePrefix("192.168.80.0/24"), netip.MustParsePrefix("fd80::/64")}
	plan := RoutePlan(c, "endpoint=203.0.113.9:51820\nendpoint=[2001:db8::9]:51820\n")
	var text string
	for _, args := range plan {
		text += strings.Join(args, " ") + "\n"
	}
	for _, want := range []string{"route add throw 203.0.113.9/32", "route add throw 2001:db8::9/128", "route add throw 192.168.80.0/24", "-6 rule add priority 18460 from fd80::/64 lookup 18460"} {
		if !strings.Contains(text, want) {
			t.Fatal("missing endpoint or LAN bypass")
		}
	}
	for _, bad := range []string{"flush", "wg0", "iptables", "table main"} {
		if strings.Contains(text, bad) {
			t.Fatal("unexpected external resource")
		}
	}
}
