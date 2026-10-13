package runtime

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tunnelstudio.local/studio/internal/vault"
)

func bootFixture(t *testing.T) (vault.Vault, bootPaths) {
	t.Helper()
	base := t.TempDir()
	v := vault.Vault{Root: filepath.Join(base, "owned")}
	if e := v.Init(); e != nil {
		t.Fatal(e)
	}
	source := filepath.Join(base, "candidate")
	if e := os.WriteFile(source, []byte("test binary A"), 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := v.Install(source); e != nil {
		t.Fatal(e)
	}
	return v, bootPaths{filepath.Join(base, "ufi_tools_boot.sh"), filepath.Join(base, "service.d", "kano-tunnel-studio.sh")}
}

func TestBootKANOWithoutMagisk(t *testing.T) {
	v, paths := bootFixture(t)
	before := []byte("#!/system/bin/sh\r\nsh /data/another/service.sh &\r\n# keep this comment\r\n")
	os.WriteFile(paths.shared, before, 0644)
	for i := 0; i < 2; i++ {
		if e := setBoot(v, paths, true); e != nil {
			t.Fatal(e)
		}
	}
	b, _ := os.ReadFile(paths.shared)
	if !bytes.HasPrefix(b, before) || bytes.Count(b, []byte(bootTag)) != 1 {
		t.Fatal("unrelated content changed or duplicate KANO entry")
	}
	info := bootStatus(v, paths)
	if !info.Enabled || !info.KANO || info.Magisk {
		t.Fatal("KANO registration requires Magisk", info)
	}
	body, _ := os.ReadFile(v.Path("boot.sh"))
	if !bytes.Contains(body, []byte("boot-start")) || bytes.Contains(body, []byte("sleep 25")) || !bytes.Contains(body, []byte("sha256sum")) {
		t.Fatal("launcher does not invoke verified worker")
	}
	if e := setBoot(v, paths, false); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(paths.shared)
	if !bytes.Equal(b, before) || bootStatus(v, paths).Enabled {
		t.Fatal("disable did not preserve other boot entries")
	}
	if e := setBoot(v, paths, false); e != nil {
		t.Fatal("disable not idempotent", e)
	}
}

func TestBootMigratesLegacyAndRewritesAfterUpgrade(t *testing.T) {
	v, paths := bootFixture(t)
	os.Mkdir(filepath.Dir(paths.magisk), 0700)
	id, _ := v.Check()
	os.WriteFile(paths.magisk, []byte(bootMarker(id.ID)+"(sleep 25; old boot-start) &\n"), 0700)
	if !bootStatus(v, paths).Enabled {
		t.Fatal("legacy enabled state lost")
	}
	if e := setBoot(v, paths, true); e != nil {
		t.Fatal(e)
	}
	old, _ := v.Release()
	source := filepath.Join(t.TempDir(), "new")
	os.WriteFile(source, []byte("test binary B"), 0700)
	newHash, e := v.Install(source)
	if e != nil {
		t.Fatal(e)
	}
	if e = setBoot(v, paths, true); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(v.Path("boot.sh"))
	if !bytes.Contains(b, []byte(newHash)) || bytes.Contains(b, []byte(old.Current)) {
		t.Fatal("launcher still points to old release")
	}
	fallback, _ := os.ReadFile(paths.magisk)
	if !bytes.Contains(fallback, []byte(v.Path("boot.sh"))) || bytes.Contains(fallback, []byte("sleep 25")) {
		t.Fatal("Magisk fallback not migrated")
	}
	if e = v.Rollback(); e != nil {
		t.Fatal(e)
	}
	if e = setBoot(v, paths, true); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(v.Path("boot.sh"))
	if !bytes.Contains(b, []byte(old.Current)) || bytes.Contains(b, []byte(newHash)) {
		t.Fatal("rollback did not rewrite launcher")
	}
}

func TestBootRejectsCollisionsAndLinks(t *testing.T) {
	for _, target := range []string{"shared", "launcher", "magisk"} {
		t.Run(target, func(t *testing.T) {
			v, paths := bootFixture(t)
			path := paths.shared
			content := "do not touch\n" + bootTag + strings.Repeat("b", 32) + "\n"
			if target == "launcher" {
				path, content = v.Path("boot.sh"), "# another owner\n"
			}
			if target == "magisk" {
				os.Mkdir(filepath.Dir(paths.magisk), 0700)
				path, content = paths.magisk, "# another owner\n"
			}
			os.WriteFile(path, []byte(content), 0600)
			for _, enabled := range []bool{true, false} {
				if e := setBoot(v, paths, enabled); e == nil {
					t.Fatal("foreign entry accepted")
				}
			}
			b, _ := os.ReadFile(path)
			if string(b) != content {
				t.Fatal("foreign content modified")
			}
		})
	}
	t.Run("symlink", func(t *testing.T) {
		v, paths := bootFixture(t)
		target := filepath.Join(t.TempDir(), "untouched")
		os.WriteFile(target, []byte("keep\n"), 0600)
		if e := os.Symlink(target, paths.shared); e != nil {
			t.Skip("host cannot create symlink:", e)
		}
		if e := setBoot(v, paths, true); e == nil {
			t.Fatal("symlink accepted")
		}
		b, _ := os.ReadFile(target)
		if string(b) != "keep\n" {
			t.Fatal("symlink target changed")
		}
	})
	t.Run("magisk-directory-link", func(t *testing.T) {
		v, paths := bootFixture(t)
		target := t.TempDir()
		id, _ := v.Check()
		content := []byte(bootMarker(id.ID) + "keep\n")
		foreign := filepath.Join(target, filepath.Base(paths.magisk))
		os.WriteFile(foreign, content, 0600)
		if e := os.Symlink(target, filepath.Dir(paths.magisk)); e != nil {
			t.Skip("host cannot create directory symlink:", e)
		}
		for _, enabled := range []bool{true, false} {
			if e := setBoot(v, paths, enabled); e == nil {
				t.Fatal("Magisk parent symlink accepted")
			}
		}
		b, _ := os.ReadFile(foreign)
		if !bytes.Equal(b, content) {
			t.Fatal("file behind Magisk directory symlink modified")
		}
	})
}

func TestBootWorkerWaitsRetriesAndRecords(t *testing.T) {
	now := time.Unix(100, 0)
	steps := []bootStep{{"waiting", "等待热点接口就绪"}, {"waiting", "DNS 未就绪"}, {"waiting", "启动未完成"}, {"started", "开机启动完成"}}
	var records []BootRecord
	calls := 0
	runner := bootRunner{
		now:   func() time.Time { return now },
		sleep: func(d time.Duration) { now = now.Add(d) },
		step:  func() bootStep { result := steps[calls]; calls++; return result },
		save:  func(r BootRecord) error { records = append(records, r); return nil },
	}
	if e := runner.run(BootRecord{BootID: "current", OwnerID: "owner"}); e != nil {
		t.Fatal(e)
	}
	last := records[len(records)-1]
	if calls != 4 || last.State != "started" || last.Attempt != 4 || len(last.Events) != 4 {
		t.Fatal("delayed readiness/retry not recorded", last)
	}
}

func TestBootWorkerTerminalStatesAndTimeout(t *testing.T) {
	for _, state := range []string{"already-running", "cancelled", "failed", "waiting"} {
		t.Run(state, func(t *testing.T) {
			now := time.Unix(100, 0)
			calls := 0
			var last BootRecord
			runner := bootRunner{
				now:   func() time.Time { return now },
				sleep: func(d time.Duration) { now = now.Add(d) },
				step:  func() bootStep { calls++; return bootStep{state, strings.Repeat("a", 64)} },
				save:  func(r BootRecord) error { last = r; return nil },
			}
			e := runner.run(BootRecord{})
			if (state == "failed" || state == "waiting") != (e != nil) {
				t.Fatal("wrong terminal outcome", e)
			}
			if state == "waiting" {
				if last.State != "timeout" || calls != 31 || now.Sub(time.Unix(100, 0)) != 5*time.Minute {
					t.Fatal("wait not bounded", last)
				}
			} else if calls != 1 {
				t.Fatal("terminal state retried")
			}
			if strings.Contains(last.Detail, strings.Repeat("a", 64)) {
				t.Fatal("sensitive error not sanitized")
			}
		})
	}
	runner := bootRunner{now: time.Now, step: func() bootStep { return bootStep{"started", ""} }, save: func(BootRecord) error { return errors.New("disk full") }}
	if e := runner.run(BootRecord{}); e == nil {
		t.Fatal("missing diagnostics reported as success")
	}
}

func TestBootRetryJournalSafety(t *testing.T) {
	v, _ := bootFixture(t)
	for _, content := range []string{
		`{"bootID":"current","ownerID":"owner"}`,
		`{"bootID":"previous","ownerID":"other"}`,
		`{"bootID":"","ownerID":"owner"}`,
		`{bad json`,
	} {
		os.WriteFile(v.Path("runtime/routes.json"), []byte(content), 0600)
		if e := bootJournalSafe(v, "owner", "current"); e == nil {
			t.Fatal("ambiguous journal allowed for automatic retry")
		}
		b, _ := os.ReadFile(v.Path("runtime/routes.json"))
		if string(b) != content {
			t.Fatal("journal modified by preflight")
		}
	}
	os.WriteFile(v.Path("runtime/routes.json"), []byte(`{"bootID":"previous","ownerID":"owner"}`), 0600)
	if e := bootJournalSafe(v, "owner", "current"); e != nil {
		t.Fatal("previous boot journal blocked", e)
	}
	os.Remove(v.Path("runtime/routes.json"))
	if e := bootJournalSafe(v, "owner", "current"); e != nil {
		t.Fatal(e)
	}
}

func TestBootLauncherExecutesVerifiedBinary(t *testing.T) {
	if _, e := os.Stat("/system/bin/sh"); e != nil {
		t.Skip("Android shell execution checked on emulator")
	}
	v, paths := bootFixture(t)
	marker := v.Path("runtime/test-invoked")
	source := filepath.Join(t.TempDir(), "fake-studio")
	os.WriteFile(source, []byte("#!/system/bin/sh\nprintf '%s' \"$1\" > "+shellQuote(marker)+"\n"), 0700)
	hash, e := v.Install(source)
	if e != nil {
		t.Fatal(e)
	}
	if e = setBoot(v, paths, true); e != nil {
		t.Fatal(e)
	}
	if b, e := exec.Command("/system/bin/sh", v.Path("boot.sh")).CombinedOutput(); e != nil {
		t.Fatalf("launcher failed: %v %s", e, b)
	}
	b, _ := os.ReadFile(marker)
	if string(b) != "boot-start" {
		t.Fatal("launcher did not call boot worker")
	}
	os.Remove(marker)
	os.WriteFile(v.Path("releases/"+hash+"/studio"), []byte("#!/system/bin/sh\nexit 0\n"), 0700)
	if e = exec.Command("/system/bin/sh", v.Path("boot.sh")).Run(); e == nil {
		t.Fatal("tampered binary executed")
	}
	if _, e = os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("tamper guard bypassed")
	}
	log, _ := os.ReadFile(v.Path("runtime/boot-launch.log"))
	if !bytes.Contains(log, []byte("binary verification failed")) {
		t.Fatal("launcher failure not logged")
	}
}

func TestBootAndroidSharedStorage(t *testing.T) {
	if _, e := os.Stat("/system/bin/sh"); e != nil {
		t.Skip("requires Android shared storage")
	}
	dir, e := os.MkdirTemp("/sdcard", "kts-boot-test-")
	if e != nil {
		t.Fatal("cannot create isolated shared-storage fixture:", e)
	}
	defer os.Remove(dir)
	v, paths := bootFixture(t)
	paths.shared = filepath.Join(dir, "fixture.sh")
	defer os.Remove(paths.shared)
	before := []byte("#!/system/bin/sh\n# existing service\n")
	if e = os.WriteFile(paths.shared, before, 0600); e != nil {
		t.Fatal(e)
	}
	if e = setBoot(v, paths, true); e != nil {
		t.Fatal("Android shared storage rejected boot registration:", e)
	}
	if e = setBoot(v, paths, false); e != nil {
		t.Fatal("Android shared storage rejected boot disable:", e)
	}
	b, _ := os.ReadFile(paths.shared)
	if !bytes.Equal(before, b) {
		t.Fatal("unrelated shared-storage content changed")
	}
}
