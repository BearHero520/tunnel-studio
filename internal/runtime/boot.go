package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"tunnelstudio.local/studio/internal/vault"
)

const bootTag = "# KANO_TUNNEL_STUDIO_BOOT "

type bootPaths struct{ shared, magisk string }

func deviceBootPaths() bootPaths {
	return bootPaths{"/sdcard/ufi_tools_boot.sh", "/data/adb/service.d/kano-tunnel-studio.sh"}
}

func bootMarker(id string) string { return "#!/system/bin/sh\n# " + vault.Owner + " " + id + "\n" }
func shellQuote(s string) string  { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
func bootLine(v vault.Vault, id string) string {
	return "nohup sh " + shellQuote(v.Path("boot.sh")) + " </dev/null >/dev/null 2>&1 & " + bootTag + id
}

// Shared storage may be a FUSE filesystem with fixed permissions. Edit the existing
// file without chmod/rename, preserving every unrelated byte and its permissions.
func editBootHook(original []byte, line, id string, enabled bool) ([]byte, error) {
	var out []byte
	for _, part := range bytes.SplitAfter(original, []byte("\n")) {
		text := strings.TrimSuffix(strings.TrimSuffix(string(part), "\n"), "\r")
		if strings.Contains(text, bootTag) {
			if text != line || !strings.HasSuffix(text, bootTag+id) {
				return nil, errors.New("KANO 自启项已被修改或属于其他资源，未覆盖")
			}
			continue
		}
		out = append(out, part...)
	}
	if enabled {
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
		out = append(out, []byte(line+"\n")...)
	}
	return out, nil
}

func readBootFile(path string) ([]byte, error) {
	f, e := openBootFile(path, os.O_RDONLY)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return bootFileContents(f)
}
func bootFileContents(f *os.File) ([]byte, error) {
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Size() > 128*1024 {
		return nil, errors.New("自启文件类型或大小无效")
	}
	b, e := io.ReadAll(io.LimitReader(f, 128*1024+1))
	if len(b) > 128*1024 {
		return nil, errors.New("自启文件超过大小限制")
	}
	return b, e
}
func updateBootHook(path, line, id string, enabled bool) error {
	flags := os.O_RDWR
	if enabled {
		flags |= os.O_CREATE
	}
	f, e := openBootFile(path, flags)
	if os.IsNotExist(e) && !enabled {
		return nil
	}
	if e != nil {
		return e
	}
	defer f.Close()
	unlock, e := lockBootFile(f)
	if e != nil {
		return e
	}
	defer unlock()
	before, e := bootFileContents(f)
	if e != nil {
		return e
	}
	after, e := editBootHook(before, line, id, enabled)
	if e != nil || bytes.Equal(before, after) {
		return e
	}
	if _, e = f.Seek(0, io.SeekStart); e != nil {
		return e
	}
	current, e := bootFileContents(f)
	if e != nil {
		return e
	}
	if !bytes.Equal(before, current) {
		return errors.New("KANO 自启脚本已同时变化，请重试")
	}
	if _, e = f.Seek(0, io.SeekStart); e != nil {
		return e
	}
	if _, e = f.Write(after); e != nil {
		return e
	}
	if e = f.Truncate(int64(len(after))); e != nil {
		return e
	}
	return f.Sync()
}

func ownedBootFile(path, marker string) (bool, error) {
	b, e := readBootFile(path)
	if os.IsNotExist(e) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	if !strings.HasPrefix(string(b), marker) {
		return false, errors.New("自启文件已被其他资源占用")
	}
	return true, nil
}

func Boot(v vault.Vault, enabled bool) error { return setBoot(v, deviceBootPaths(), enabled) }
func setBoot(v vault.Vault, paths bootPaths, enabled bool) error {
	id, e := v.Check()
	if e != nil {
		return e
	}
	marker := bootMarker(id.ID)
	launcher := v.Path("boot.sh")
	if e = vault.NoLink(filepath.Dir(paths.magisk)); e != nil && !os.IsNotExist(e) {
		return e
	}
	for _, path := range []string{launcher, paths.magisk} {
		if _, e = ownedBootFile(path, marker); e != nil {
			return e
		}
	}
	line := bootLine(v, id.ID)
	if b, err := readBootFile(paths.shared); err == nil {
		if _, e = editBootHook(b, line, id.ID, enabled); e != nil {
			return e
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if !enabled {
		if e = updateBootHook(paths.shared, line, id.ID, false); e != nil {
			return e
		}
		for _, path := range []string{launcher, paths.magisk} {
			if e = os.Remove(path); e != nil && !os.IsNotExist(e) {
				return e
			}
		}
		return nil
	}
	r, e := v.Release()
	if e != nil {
		return e
	}
	bin, e := v.Binary(r.Current)
	if e != nil {
		return e
	}
	log := v.Path("runtime/boot-launch.log")
	body := marker + "umask 077\n" +
		"test ! -L " + shellQuote(v.Root) + " && test ! -L " + shellQuote(v.Path("runtime")) + " && test -d " + shellQuote(v.Path("runtime")) + " || exit 1\n" +
		"test ! -L " + shellQuote(log) + " || exit 1\n" +
		"printf 'launcher invoked\\n' > " + shellQuote(log) + "\n" +
		"if test ! -L " + shellQuote(v.Path("releases")) + " && test ! -L " + shellQuote(filepath.Dir(bin)) + " && test ! -L " + shellQuote(bin) +
		" && test \"$(sha256sum " + shellQuote(bin) + " | cut -d ' ' -f 1)\" = " + shellQuote(r.Current) + "; then\n" +
		"  exec " + shellQuote(bin) + " boot-start >/dev/null 2>&1\n" +
		"fi\nprintf 'binary verification failed\\n' >> " + shellQuote(log) + "\nexit 1\n"
	if e = vault.Atomic(launcher, []byte(body), 0700); e != nil {
		return e
	}
	if e = updateBootHook(paths.shared, line, id.ID, true); e != nil {
		return e
	}
	// Keep a Magisk fallback where available. Both entries share a worker lock.
	if e = vault.NoLink(filepath.Dir(paths.magisk)); e == nil {
		return vault.Atomic(paths.magisk, []byte(marker+line+"\n"), 0700)
	} else if !os.IsNotExist(e) {
		return e
	}
	return nil
}

type BootInfo struct {
	Enabled     bool        `json:"enabled"`
	KANO        bool        `json:"kano"`
	Magisk      bool        `json:"magisk"`
	Detail      string      `json:"detail,omitempty"`
	Last        *BootRecord `json:"last,omitempty"`
	Launch      string      `json:"launch,omitempty"`
	CurrentBoot bool        `json:"currentBoot"`
}

func BootStatus(v vault.Vault) BootInfo { return bootStatus(v, deviceBootPaths()) }
func bootStatus(v vault.Vault, paths bootPaths) BootInfo {
	var info BootInfo
	id, e := v.Check()
	if e != nil {
		info.Detail = "无法核对自启资源所有权"
		return info
	}
	if b, e := readBootFile(paths.shared); e == nil {
		line := bootLine(v, id.ID)
		for _, s := range strings.Split(string(b), "\n") {
			if strings.TrimSuffix(s, "\r") == line {
				info.KANO = true
			}
		}
	}
	launcher, le := ownedBootFile(v.Path("boot.sh"), bootMarker(id.ID))
	legacy := false
	me := vault.NoLink(filepath.Dir(paths.magisk))
	if me == nil {
		legacy, me = ownedBootFile(paths.magisk, bootMarker(id.ID))
	} else if os.IsNotExist(me) {
		me = nil
	}
	// Recognize old Magisk entries so installation migrates enabled v0.1.6 setups.
	info.Magisk = legacy && me == nil
	info.Enabled = (info.KANO && launcher && le == nil) || info.Magisk
	if info.KANO && !launcher || le != nil || me != nil {
		info.Detail = "自启入口或所属资源异常，请重新设置自启"
	}
	if b, e := readBootFile(v.Path("runtime/boot-status.json")); e == nil {
		var record BootRecord
		if json.Unmarshal(b, &record) == nil && record.OwnerID == id.ID {
			info.Last = &record
			if b, e := os.ReadFile("/proc/sys/kernel/random/boot_id"); e == nil {
				info.CurrentBoot = strings.TrimSpace(string(b)) == record.BootID
			}
		}
	}
	if b, e := readBootFile(v.Path("runtime/boot-launch.log")); e == nil {
		// Only expose fixed launcher messages, never arbitrary file contents.
		if strings.Contains(string(b), "launcher invoked") {
			info.Launch = "invoked"
		}
		if strings.Contains(string(b), "binary verification failed") {
			info.Launch = "verification-failed"
		}
	}
	return info
}

type BootEvent struct {
	At      string `json:"at"`
	State   string `json:"state"`
	Detail  string `json:"detail"`
	Attempt int    `json:"attempt"`
}
type BootRecord struct {
	OwnerID string      `json:"ownerID"`
	BootID  string      `json:"bootID"`
	Version string      `json:"version"`
	Started string      `json:"startedAt"`
	Updated string      `json:"updatedAt"`
	State   string      `json:"state"`
	Detail  string      `json:"detail"`
	Attempt int         `json:"attempt"`
	Events  []BootEvent `json:"events"`
}
type bootStep struct{ state, detail string }
type bootRunner struct {
	now   func() time.Time
	sleep func(time.Duration)
	step  func() bootStep
	save  func(BootRecord) error
}

var bootSecret = regexp.MustCompile(`[A-Za-z0-9+/]{43}=|[a-fA-F0-9]{64}`)

func safeBootDetail(s string) string {
	s = bootSecret.ReplaceAllString(s, "[已隐藏]")
	if len([]rune(s)) > 400 {
		s = string([]rune(s)[:400])
	}
	return s
}

// A previous kernel's journal can be handled by Serve. Anything ambiguous from
// this kernel remains untouched and must never be automatically retried.
func bootJournalSafe(v vault.Vault, ownerID, bootID string) error {
	b, e := readBootFile(v.Path("runtime/routes.json"))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return errors.New("无法安全读取路由事务，停止自启重试")
	}
	var old struct {
		OwnerID string `json:"ownerID"`
		BootID  string `json:"bootID"`
	}
	if json.Unmarshal(b, &old) != nil || old.OwnerID != ownerID || old.BootID == "" || bootID == "unavailable" || old.BootID == bootID {
		return errors.New("存在未完成路由事务，停止自启重试；请查看只读诊断")
	}
	return nil
}

// The worker sleeps without the manager lock, leaving refresh/stop/disable usable.
// Only a cleanly stopped daemon may be retried; unresolved transactions are terminal.
func (r bootRunner) run(record BootRecord) error {
	begin := r.now()
	deadline := begin.Add(5 * time.Minute)
	record.Started = begin.UTC().Format(time.RFC3339Nano)
	for {
		record.Attempt++
		step := r.step()
		record.State, record.Detail = step.state, safeBootDetail(step.detail)
		if !r.now().Before(deadline) && step.state == "waiting" {
			record.State = "timeout"
			record.Detail = "等待超过 5 分钟；最后原因：" + record.Detail
		}
		record.Updated = r.now().UTC().Format(time.RFC3339Nano)
		record.Events = append(record.Events, BootEvent{record.Updated, record.State, record.Detail, record.Attempt})
		if len(record.Events) > 32 {
			record.Events = record.Events[len(record.Events)-32:]
		}
		if e := r.save(record); e != nil {
			return fmt.Errorf("无法保存开机启动记录：%w", e)
		}
		if record.State != "waiting" {
			if record.State == "started" || record.State == "already-running" || record.State == "cancelled" {
				return nil
			}
			return errors.New(record.Detail)
		}
		r.sleep(10 * time.Second)
	}
}
