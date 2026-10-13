//go:build linux

package runtime

import (
	"errors"
	"os"
	"strings"
	"testing"

	"tunnelstudio.local/studio/internal/config"
)

func TestBootWorkerLockDistinguishesCollision(t *testing.T) {
	v, _ := bootFixture(t)
	path := v.Path("runtime/boot-worker.lock")
	unlock, e := Lock(path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Lock(path); !errors.Is(e, errOperationBusy) {
		t.Fatal("worker contention not recognized", e)
	}
	if e = BootStart(v); e != nil {
		t.Fatal("duplicate worker should leave existing worker alone", e)
	}
	unlock()
	os.Remove(path)
	if e = os.Symlink(v.Path("owner.json"), path); e != nil {
		t.Fatal(e)
	}
	if e = BootStart(v); e == nil || errors.Is(e, errOperationBusy) {
		t.Fatal("unsafe worker lock reported as successful duplicate")
	}
}

func TestBootAndroidReadinessProbe(t *testing.T) {
	if _, e := os.Stat("/system/bin/getprop"); e != nil {
		t.Skip("requires Android getprop and ip")
	}
	// Read-only marked route lookup, without changing interfaces or policy.
	c := &config.Parsed{NetworkMode: "tunnel", Peers: []map[string]string{{"Endpoint": "127.0.0.1:51820"}}}
	if e := bootNetworkReady(c); e != nil {
		t.Fatal("ready Android rejected by readiness probe:", e)
	}
	c.NetworkMode, c.LANInterface = "gateway", "kts_missing"
	if e := bootNetworkReady(c); e == nil || !strings.Contains(e.Error(), "热点接口") {
		t.Fatal("missing LAN not treated as waiting", e)
	}
}
