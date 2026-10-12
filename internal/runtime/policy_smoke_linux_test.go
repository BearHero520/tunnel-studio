//go:build linux

package runtime

import (
	"github.com/amnezia-vpn/amneziawg-go/v3/tun"
	"os"
	"strings"
	"testing"
)

// Explicit opt-in: exercise only unhooked owned chains and a marked test table.
// No OUTPUT/PREROUTING/FORWARD hooks or sysctl changes enter the live namespace.
func TestAndroidPolicySyntax(t *testing.T) {
	if os.Getenv("KTS_POLICY_SMOKE") != "1" {
		t.Skip("isolated Android emulator opt-in only")
	}
	if os.Geteuid() != 0 {
		t.Fatal("test needs emulator root")
	}
	if e := routesAvailable(); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{Interface, "kts_testlan"} {
		if _, e := ip("link", "show", "dev", name); e == nil {
			t.Fatal("test interface already exists; refusing", name)
		}
		device, e := tun.CreateTUN(name, 1280)
		if e != nil {
			t.Fatal(e)
		}
		defer device.Close()
		if _, e := ip("link", "set", name, "up"); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := ip("address", "add", "10.77.0.2/32", "dev", Interface); e != nil {
		t.Fatal(e)
	}
	if _, e := ip("-6", "address", "add", "fd77::2/128", "dev", Interface, "nodad"); e != nil {
		t.Fatal(e)
	}
	for _, v6 := range []string{"block", "nat"} {
		t.Run(v6, func(t *testing.T) {
			c := proxyFixture()
			c.LANInterface = "kts_testlan"
			c.IPv6Mode = v6
			local, _, e := prepareProxy(c)
			if e != nil {
				t.Fatal(e)
			}
			var applied []Operation
			var parents []Operation
			created := map[string]bool{}
			defer func() {
				for i := len(applied) - 1; i >= 0; i-- {
					if e := runOperation(applied[i].Tool, applied[i].Undo); e != nil {
						t.Error(e)
					}
				}
				for i := len(parents) - 1; i >= 0; i-- {
					if e := runOperation(parents[i].Tool, parents[i].Undo); e != nil {
						t.Error(e)
					}
				}
			}()
			for _, op := range ProxyPlan(c, "endpoint=203.0.113.10:51820\n", local) {
				hook := false
				for _, a := range op.Args {
					if a == "-I" {
						hook = true
					}
				}
				if hook {
					key := op.Tool + op.Args[3]
					if !created[key] {
						parent := Operation{Tool: op.Tool, Args: []string{"-w", "3", "-t", op.Args[3], "-N", "KTS_TEST_HOOK"}, Undo: []string{"-w", "3", "-t", op.Args[3], "-X", "KTS_TEST_HOOK"}}
						if e := runOperation(parent.Tool, parent.Args); e != nil {
							t.Fatal(e)
						}
						parents = append(parents, parent)
						created[key] = true
					}
					op.Args = append([]string(nil), op.Args...)
					op.Undo = append([]string(nil), op.Undo...)
					op.Args[5] = "KTS_TEST_HOOK"
					op.Undo[5] = "KTS_TEST_HOOK"
				}
				if e := runOperation(op.Tool, op.Args); e != nil {
					t.Fatal(e)
				}
				applied = append(applied, op)
			}
			route, e := ip("-4", "route", "get", "198.51.100.10", "mark", "0x40000000")
			if e != nil || !strings.Contains(route, "dev "+Interface) {
				t.Fatal("marked route", route, e)
			}
		})
	}
}
