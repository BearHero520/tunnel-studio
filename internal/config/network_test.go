package config

import (
	"strings"
	"testing"
)

func TestNetworkModes(t *testing.T) {
	p := sample(t)
	c, e := Parse(p)
	if e != nil || c.NetworkMode != "tunnel" {
		t.Fatal("legacy mode migration", e)
	}
	p.NetworkMode = "gateway"
	p.Text = strings.Replace(p.Text, "[Peer]", "DNS = 1.1.1.1, 2606:4700:4700::1111\n[Peer]", 1)
	c, e = Parse(p)
	if e != nil || c.IPv6Mode != "block" || len(c.DNS) != 1 {
		t.Fatal("default proxy IPv6 handling", e)
	}
	p.IPv6Mode = "nat"
	c, e = Parse(p)
	if e != nil || len(c.DNS) != 2 {
		t.Fatal("explicit IPv6 NAT", e)
	}
	p.LANInterface = "br0;reboot"
	if _, e = Parse(p); e == nil {
		t.Fatal("accepted shell syntax")
	}
	p.LANInterface = "br0"
	p.IPv6Mode = "routed"
	if _, e = Parse(p); e == nil {
		t.Fatal("host routed IPv6 silently accepted")
	}
	p.NetworkMode = "lan"
	p.Text = strings.Replace(p.Text, ", 2606:4700:4700::1111", "", 1)
	if _, e = Parse(p); e != nil {
		t.Fatal(e)
	}
	p.NetworkMode = "unknown"
	if _, e = Parse(p); e == nil {
		t.Fatal("unknown mode")
	}
}
