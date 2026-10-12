package tests

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	ac "github.com/amnezia-vpn/amneziawg-go/v3/conn"
	ad "github.com/amnezia-vpn/amneziawg-go/v3/device"
	at "github.com/amnezia-vpn/amneziawg-go/v3/tun"
	am "github.com/amnezia-vpn/amneziawg-go/v3/tun/tuntest"
	wc "golang.zx2c4.com/wireguard/conn"
	wd "golang.zx2c4.com/wireguard/device"
	wm "golang.zx2c4.com/wireguard/tun/tuntest"
	"tunnelstudio.local/studio/internal/config"
)

type backend interface {
	IpcSet(string) error
	IpcGet() (string, error)
	Up() error
	Close()
}
type waitRead struct {
	at.Device
	once  sync.Once
	ready chan struct{}
}

func (w *waitRead) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	w.once.Do(func() { close(w.ready) })
	return w.Device.Read(bufs, sizes, offset)
}

type capture struct {
	ac.Bind
	mu      sync.Mutex
	packets [][]byte
}

func (b *capture) Send(bufs [][]byte, ep ac.Endpoint) error {
	b.mu.Lock()
	for _, p := range bufs {
		b.packets = append(b.packets, bytes.Clone(p))
	}
	b.mu.Unlock()
	return b.Bind.Send(bufs, ep)
}
func packet(v6 bool, from, to byte) []byte {
	payload := []byte("Tunnel Studio authenticated transport test")
	n := 20
	if v6 {
		n = 40
	}
	p := make([]byte, n+len(payload))
	copy(p[n:], payload)
	if v6 {
		p[0] = 0x60
		p[6] = 59
		p[7] = 64
		binary.BigEndian.PutUint16(p[4:6], uint16(len(payload)))
		p[8] = 0xfd
		p[24] = 0xfd
		p[23] = from
		p[39] = to
	} else {
		p[0] = 0x45
		p[8] = 64
		p[9] = 253
		binary.BigEndian.PutUint16(p[2:4], uint16(len(p)))
		copy(p[12:16], []byte{10, 99, 0, from})
		copy(p[16:20], []byte{10, 99, 0, to})
	}
	return p
}
func TestEncryptedInteroperability(t *testing.T) {
	for _, mode := range []string{"wireguard-to-official", "amneziawg-obfuscation", "amneziawg3-security"} {
		t.Run(mode, func(t *testing.T) {
			var ds [2]backend
			var incoming [2]<-chan []byte
			var outgoing [2]chan<- []byte
			var priv, pub [2]string
			for i := 0; i < 2; i++ {
				k, e := ecdh.X25519().GenerateKey(rand.Reader)
				if e != nil {
					t.Fatal(e)
				}
				priv[i] = base64.StdEncoding.EncodeToString(k.Bytes())
				pub[i] = base64.StdEncoding.EncodeToString(k.PublicKey().Bytes())
			}
			cap := &capture{Bind: ac.NewDefaultBind()}
			for i := 0; i < 2; i++ {
				if mode == "wireguard-to-official" && i == 1 {
					tun := wm.NewChannelTUN()
					incoming[i], outgoing[i] = tun.Inbound, tun.Outbound
					ds[i] = wd.NewDevice(tun.TUN(), wc.NewDefaultBind(), wd.NewLogger(wd.LogLevelSilent, ""))
				} else {
					tun := am.NewChannelTUN()
					incoming[i], outgoing[i] = tun.Inbound, tun.Outbound
					wrapper := &waitRead{Device: tun.TUN(), ready: make(chan struct{})}
					bind := ac.NewDefaultBind()
					if i == 0 {
						bind = cap
					}
					ds[i] = ad.NewDevice(wrapper, bind, ad.NewLogger(ad.LogLevelSilent, ""))
					select {
					case <-wrapper.ready:
					case <-time.After(time.Second):
						t.Fatal("TUN reader did not enter first Read")
					}
				}
				defer ds[i].Close()
			}
			hp := make([]byte, 32)
			rand.Read(hp)
			for i := 0; i < 2; i++ {
				proto := "wireguard"
				extra := ""
				if mode != "wireguard-to-official" {
					proto = "amneziawg"
					extra = "Jc = 2\nJmin = 30\nJmax = 70\nS1 = 16\nS2 = 20\nS3 = 24\nS4 = 28\nH1 = 100-110\nH2 = 200-210\nH3 = 300-310\nH4 = 400-410\nI1 = <b 0xdeadbeef><r 12><t>\n"
				}
				if mode == "amneziawg3-security" {
					extra += "HeaderProtectionKey = " + base64.StdEncoding.EncodeToString(hp) + "\nContentPaddingAddition = 16-32\nRandomTrailers = true\nDisableCookies = false\nRekeyAfterTime = 90-120\nRekeyTimeout = 3-5\nRejectAfterTime = 180\nKeepaliveTimeout = 10\nMaxHandshakeAttempts = 18\n"
				}
				text := fmt.Sprintf("[Interface]\nPrivateKey = %s\nAddress = 10.99.0.%d/32, fd00::%d/128\nListenPort = 0\n%s[Peer]\nPublicKey = %s\nAllowedIPs = 10.99.0.%d/32, fd00::%d/128\n", priv[i], i+1, i+1, extra, pub[1-i], 2-i, 2-i)
				c, e := config.Parse(config.Profile{Protocol: proto, Text: text})
				if e != nil {
					t.Fatal(e)
				}
				u, e := c.UAPI(func(s string) (string, error) { return s, nil })
				if e != nil {
					t.Fatal(e)
				}
				if e = ds[i].IpcSet(u); e != nil {
					t.Fatalf("backend rejected %s config", mode)
				}
				if e = ds[i].Up(); e != nil {
					t.Fatal(e)
				}
			}
			for i := 0; i < 2; i++ {
				raw, e := ds[1-i].IpcGet()
				if e != nil {
					t.Fatal(e)
				}
				port := ""
				for _, line := range strings.Split(raw, "\n") {
					if strings.HasPrefix(line, "listen_port=") {
						port = strings.TrimPrefix(line, "listen_port=")
					}
				}
				key, _ := base64.StdEncoding.DecodeString(pub[1-i])
				if e = ds[i].IpcSet(fmt.Sprintf("public_key=%x\nendpoint=127.0.0.1:%s\n", key, port)); e != nil {
					t.Fatal("endpoint rejected")
				}
			}
			for _, v6 := range []bool{false, true} {
				for i := 0; i < 2; i++ {
					want := packet(v6, byte(i+1), byte(2-i))
					select {
					case outgoing[i] <- want:
					case <-time.After(time.Second):
						t.Fatal("TUN blocked")
					}
					select {
					case got := <-incoming[1-i]:
						if !bytes.Equal(got, want) {
							t.Fatal("decrypted payload mismatch")
						}
					case <-time.After(4 * time.Second):
						t.Fatal("first packet lost; no retransmission allowed in this test")
					}
				}
			}
			cap.mu.Lock()
			defer cap.mu.Unlock()
			if len(cap.packets) < 3 {
				t.Fatal("no encrypted wire traffic captured")
			}
			for _, p := range cap.packets {
				if bytes.Contains(p, []byte("Tunnel Studio authenticated")) {
					t.Fatal("plaintext leaked onto UDP transport")
				}
			}
		})
	}
}
