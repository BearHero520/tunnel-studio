package runtime

import (
	"net/netip"
	"strings"
	"testing"
	"tunnelstudio.local/studio/internal/config"
)

func proxyFixture() *config.Parsed {
	return &config.Parsed{NetworkMode: "gateway", LANInterface: "br0", IPv6Mode: "block", Routes: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0"), netip.MustParsePrefix("::/0")}, DNS: []netip.Addr{netip.MustParseAddr("1.1.1.1")}}
}
func planText(plan []Operation) string {
	var b strings.Builder
	for _, op := range plan {
		b.WriteString(op.Tool + " " + strings.Join(op.Args, " ") + "\n")
	}
	return b.String()
}
func TestGatewayPolicy(t *testing.T) {
	plan := ProxyPlan(proxyFixture(), "endpoint=203.0.113.10:51820\n", []netip.Prefix{netip.MustParsePrefix("192.168.0.0/24")})
	s := planText(plan)
	for _, want := range []string{"priority 9000 fwmark " + ProxyMark, "-I OUTPUT -j KTS_ROUTE", "-I PREROUTING -i br0 -j KTS_ROUTE", "-A KTS_SNAT -j MASQUERADE", "--set-mss 1240", "--ctstate ESTABLISHED,RELATED -j ACCEPT", "-p udp --dport 53 -j DNAT --to-destination 1.1.1.1", "-p tcp --dport 53 -j DNAT --to-destination 1.1.1.1", "-d 192.168.0.0/24 -j RETURN", "--mark " + TransportMark + " -j RETURN", "-A KTS_BLOCK -j REJECT"} {
		if !strings.Contains(s, want) {
			t.Fatal("missing", want)
		}
	}
	if strings.Contains(s, "ip6tables -w 3 -t nat") || strings.Contains(s, "--set-mark") || strings.Contains(s, "flush") {
		t.Fatal("unsupported NAT66, lower-mark overwrite, or global flush")
	}
	if strings.Index(s, "-d 203.0.113.10 -j RETURN") > strings.Index(s, "--dport 53 -j MARK") {
		t.Fatal("DNS precedes transport bypass")
	}
	if strings.Index(s, "-I OUTPUT -j KTS_ROUTE") < strings.Index(s, "-I POSTROUTING") {
		t.Fatal("traffic enabled before NAT")
	}
	for _, op := range plan {
		if len(op.Undo) == 0 {
			t.Fatal("operation without undo")
		}
		for _, v := range op.Undo {
			if v == "-F" || v == "flush" {
				t.Fatal("global cleanup")
			}
		}
	}
}
func TestProxyScopesAndIPv6(t *testing.T) {
	c := proxyFixture()
	c.NetworkMode = "device"
	s := planText(ProxyPlan(c, "", nil))
	if strings.Contains(s, "-i br0") || strings.Contains(s, "KTS_FWD") {
		t.Fatal("device mode changes forwarding")
	}
	c.NetworkMode = "lan"
	c.IPv6Mode = "routed"
	s = planText(ProxyPlan(c, "", nil))
	if strings.Contains(s, "-I OUTPUT") || strings.Contains(s, "KTS_BLOCK") || strings.Contains(s, "ip6tables -w 3 -t nat") {
		t.Fatal("routed LAN mode unexpectedly handles host or NAT66")
	}
	c.NetworkMode = "gateway"
	c.IPv6Mode = "nat"
	s = planText(ProxyPlan(c, "", nil))
	if !strings.Contains(s, "ip6tables -w 3 -t nat -A KTS_SNAT -j MASQUERADE") {
		t.Fatal("missing explicit NAT66")
	}
	c.NetworkMode = "tunnel"
	if len(ProxyPlan(c, "", nil)) != 0 {
		t.Fatal("legacy tunnel mode changed network")
	}
}
