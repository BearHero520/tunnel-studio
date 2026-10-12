package vault

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"tunnelstudio.local/studio/internal/config"
)

func setup(t *testing.T) (Vault, config.Profile) {
	t.Helper()
	v := Vault{filepath.Join(t.TempDir(), "owned")}
	if e := v.Init(); e != nil {
		t.Fatal(e)
	}
	k, _ := ecdh.X25519().GenerateKey(rand.Reader)
	peer, _ := ecdh.X25519().GenerateKey(rand.Reader)
	p := config.Profile{Protocol: "wireguard", Text: "[Interface]\nPrivateKey = " + base64.StdEncoding.EncodeToString(k.Bytes()) + "\nAddress = 10.77.0.2/32\n[Peer]\nPublicKey = " + base64.StdEncoding.EncodeToString(peer.PublicKey().Bytes()) + "\nAllowedIPs = 10.77.0.1/32\n"}
	return v, p
}
func TestSaveBackupRestore(t *testing.T) {
	v, p := setup(t)
	if e := v.Save(p); e != nil {
		t.Fatal(e)
	}
	first, e := v.Backup()
	if e != nil {
		t.Fatal(e)
	}
	p.Protocol = "amneziawg"
	if e = v.Save(p); e != nil {
		t.Fatal(e)
	}
	if e = v.Restore(first); e != nil {
		t.Fatal(e)
	}
	got, e := v.Read()
	if e != nil || got.Protocol != "wireguard" {
		t.Fatal("restore failed")
	}
	list, _ := v.List()
	if len(list) != 3 {
		t.Fatal("restore did not preserve previous config")
	}
	if e = v.Restore("../profile.json"); e == nil {
		t.Fatal("traversal accepted")
	}
}
func TestOwnershipAndCorruption(t *testing.T) {
	v, _ := setup(t)
	os.WriteFile(v.Path("owner.json"), []byte(`{"owner":"other"}`), 0600)
	if e := v.Init(); e == nil {
		t.Fatal("foreign root taken over")
	}
	v, p := setup(t)
	v.Save(p)
	before, _ := os.ReadFile(v.Path("profile.json"))
	p.Text = "invalid"
	if e := v.Save(p); e == nil {
		t.Fatal("invalid save succeeded")
	}
	after, _ := os.ReadFile(v.Path("profile.json"))
	if string(before) != string(after) {
		t.Fatal("failed save changed active config")
	}
}
func TestReleaseRollbackAndTamper(t *testing.T) {
	v, _ := setup(t)
	src := filepath.Join(t.TempDir(), "candidate")
	os.WriteFile(src, []byte("release A"), 0700)
	a, e := v.Install(src)
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(src, []byte("release B"), 0700)
	b, e := v.Install(src)
	if e != nil {
		t.Fatal(e)
	}
	if a == b {
		t.Fatal("hash collision")
	}
	if e = v.Rollback(); e != nil {
		t.Fatal(e)
	}
	r, _ := v.Release()
	if r.Current != a || r.Previous != b {
		t.Fatal("wrong release pointer")
	}
	os.WriteFile(v.Path("releases/"+b+"/studio"), []byte("tampered"), 0700)
	if e = v.Rollback(); e == nil {
		t.Fatal("tampered rollback accepted")
	}
}
