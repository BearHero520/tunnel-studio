package config

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

func sample(t *testing.T) Profile {
	t.Helper()
	k, _ := ecdh.X25519().GenerateKey(rand.Reader)
	p, _ := ecdh.X25519().GenerateKey(rand.Reader)
	return Profile{Protocol: "amneziawg", Text: "[Interface]\nPrivateKey = " + base64.StdEncoding.EncodeToString(k.Bytes()) + "\nAddress = 10.44.0.2/32, fd00::2/128\n[Peer]\nPublicKey = " + base64.StdEncoding.EncodeToString(p.PublicKey().Bytes()) + "\nAllowedIPs = 0.0.0.0/0, ::/0\nEndpoint = [2001:db8::1]:51820\n"}
}
func TestRejectUnsafeOrIncompatible(t *testing.T) {
	base := sample(t)
	cases := map[string]string{"shell": "PostUp = touch /tmp/x", "unknown": "AdvancedSecurity = true", "H overlap": "H1 = 2", "S overflow": "S4 = 65536", "junk": "Jmin = 500\nJmax = 100", "negative": "Jc = -1", "invalid tag": "I1 = <exec bad>", "huge tag": "I1 = <r 99999999>", "odd hex": "I1 = <b 0x123>", "bool": "RandomTrailers = yes", "bad header key": "HeaderProtectionKey = bad", "range": "RekeyTimeout = 20-10", "zero timing": "RejectAfterTime = 0", "partial protection": "HeaderProtectionKey = " + strings.Repeat("A", 42) + "E="}
	for name, field := range cases {
		t.Run(name, func(t *testing.T) {
			p := base
			p.Text = strings.Replace(p.Text, "[Peer]", field+"\n[Peer]", 1)
			if _, e := Parse(p); e == nil {
				t.Fatal("accepted invalid input")
			}
		})
	}
	p := base
	p.Protocol = "wireguard"
	p.Text = strings.Replace(p.Text, "[Peer]", "S1 = 12\n[Peer]", 1)
	if _, e := Parse(p); e == nil {
		t.Fatal("WG accepted AWG")
	}
}
func TestUAPIAndRedaction(t *testing.T) {
	p := sample(t)
	c, e := Parse(p)
	if e != nil {
		t.Fatal(e)
	}
	s, e := c.UAPI(func(s string) (string, error) { return s, nil })
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(s, "Address") || !strings.Contains(s, "allowed_ip=::/0") || !strings.Contains(s, "endpoint=[2001:db8::1]:51820") {
		t.Fatal("incorrect UAPI")
	}
	p.Sources = []string{"0.0.0.0/0"}
	if _, e = Parse(p); e == nil {
		t.Fatal("global source accepted")
	}
}
func TestFormats(t *testing.T) {
	p := sample(t)
	p.Text = strings.ReplaceAll(p.Text, "\n", "\r\n")
	if _, e := Parse(p); e != nil {
		t.Fatal(e)
	}
	p.Text = strings.Replace(p.Text, "0.0.0.0/0", "10.1.2.3/24", 1)
	if _, e := Parse(p); e == nil {
		t.Fatal("noncanonical allowed IP accepted")
	}
}
