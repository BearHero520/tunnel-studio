package vault

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"tunnelstudio.local/studio/internal/config"
)

const Owner = "kano.tunnel.studio/1"

type Vault struct{ Root string }
type Identity struct {
	Owner string `json:"owner"`
	ID    string `json:"id"`
}

var IDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var HashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (v Vault) Path(name string) string { return filepath.Join(v.Root, name) }
func NoLink(path string) error {
	f, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if f.Mode()&os.ModeSymlink != 0 {
		return errors.New("拒绝符号链接")
	}
	return nil
}
func Atomic(path string, b []byte, mode os.FileMode) error {
	if e := NoLink(path); e != nil && !os.IsNotExist(e) {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".txn-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(f.Name(), path)
}
func (v Vault) Check() (Identity, error) {
	var id Identity
	if e := NoLink(v.Root); e != nil {
		return id, e
	}
	if e := NoLink(v.Path("owner.json")); e != nil {
		return id, e
	}
	b, e := os.ReadFile(v.Path("owner.json"))
	if e != nil {
		return id, e
	}
	if json.Unmarshal(b, &id) != nil || id.Owner != Owner || !IDPattern.MatchString(id.ID) {
		return id, errors.New("目录不属于 Tunnel Studio，拒绝接管")
	}
	for _, d := range []string{"releases", "backups", "runtime"} {
		if e := NoLink(v.Path(d)); e != nil {
			return id, e
		}
		f, e := os.Stat(v.Path(d))
		if e != nil || !f.IsDir() {
			return id, errors.New("资源目录无效")
		}
	}
	return id, nil
}
func (v Vault) Init() error {
	if _, e := os.Lstat(v.Root); e == nil {
		_, e = v.Check()
		return e
	} else if !os.IsNotExist(e) {
		return e
	}
	if e := os.Mkdir(v.Root, 0700); e != nil {
		return e
	}
	for _, d := range []string{"releases", "backups", "runtime"} {
		if e := os.Mkdir(v.Path(d), 0700); e != nil {
			return e
		}
	}
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return e
	}
	raw, _ := json.Marshal(Identity{Owner, hex.EncodeToString(b)})
	return Atomic(v.Path("owner.json"), raw, 0600)
}
func Decode(b []byte) (config.Profile, error) {
	var p config.Profile
	if len(b) > 100000 {
		return p, errors.New("配置数据过大")
	}
	if e := json.Unmarshal(b, &p); e != nil {
		return p, errors.New("配置 JSON 无效")
	}
	_, e := config.Parse(p)
	return p, e
}
func (v Vault) Read() (config.Profile, error) {
	if _, e := v.Check(); e != nil {
		return config.Profile{}, e
	}
	if e := NoLink(v.Path("profile.json")); e != nil {
		return config.Profile{}, e
	}
	b, e := os.ReadFile(v.Path("profile.json"))
	if e != nil {
		return config.Profile{}, e
	}
	return Decode(b)
}
func (v Vault) Backup() (string, error) {
	p, e := v.Read()
	if e != nil {
		return "", e
	}
	raw, _ := json.MarshalIndent(p, "", "  ")
	nonce := make([]byte, 4)
	if _, e = rand.Read(nonce); e != nil {
		return "", e
	}
	name := time.Now().UTC().Format("20060102T150405.000000000") + "-" + hex.EncodeToString(nonce) + ".json"
	return name, Atomic(v.Path("backups/"+name), raw, 0600)
}
func (v Vault) Save(p config.Profile) error {
	if _, e := v.Check(); e != nil {
		return e
	}
	if _, e := config.Parse(p); e != nil {
		return e
	}
	if _, e := os.Lstat(v.Path("profile.json")); e == nil {
		if _, e = v.Backup(); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	return Atomic(v.Path("profile.json"), b, 0600)
}
func (v Vault) List() ([]string, error) {
	if _, e := v.Check(); e != nil {
		return nil, e
	}
	items, e := os.ReadDir(v.Path("backups"))
	out := []string{}
	for _, x := range items {
		if x.Type().IsRegular() && filepath.Ext(x.Name()) == ".json" {
			out = append(out, x.Name())
		}
	}
	return out, e
}
func (v Vault) Restore(name string) error {
	if filepath.Base(name) != name || filepath.Ext(name) != ".json" {
		return errors.New("备份名称无效")
	}
	if _, e := v.Check(); e != nil {
		return e
	}
	path := v.Path("backups/" + name)
	if e := NoLink(path); e != nil {
		return e
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	p, e := Decode(b)
	if e != nil {
		return e
	}
	return v.Save(p)
}
func Digest(path string) (string, error) {
	if e := NoLink(path); e != nil {
		return "", e
	}
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type Release struct {
	Current  string `json:"current"`
	Previous string `json:"previous"`
}

func (v Vault) Release() (Release, error) {
	var r Release
	if _, e := v.Check(); e != nil {
		return r, e
	}
	if e := NoLink(v.Path("release.json")); e != nil {
		return r, e
	}
	b, e := os.ReadFile(v.Path("release.json"))
	if e != nil {
		return r, e
	}
	if json.Unmarshal(b, &r) != nil || !HashPattern.MatchString(r.Current) || (r.Previous != "" && !HashPattern.MatchString(r.Previous)) {
		return r, errors.New("版本清单无效")
	}
	return r, nil
}
func (v Vault) Binary(hash string) (string, error) {
	if !HashPattern.MatchString(hash) {
		return "", errors.New("版本 ID 无效")
	}
	d := v.Path("releases/" + hash)
	if e := NoLink(d); e != nil {
		return "", e
	}
	path := filepath.Join(d, "studio")
	got, e := Digest(path)
	if e != nil {
		return "", e
	}
	if got != hash {
		return "", errors.New("资源校验失败")
	}
	return path, nil
}
func (v Vault) Install(source string) (string, error) {
	if _, e := v.Check(); e != nil {
		return "", e
	}
	hash, e := Digest(source)
	if e != nil {
		return "", e
	}
	path := v.Path("releases/" + hash)
	if e = os.Mkdir(path, 0700); e == nil {
		b, e := os.ReadFile(source)
		if e != nil {
			return "", e
		}
		if e = Atomic(filepath.Join(path, "studio"), b, 0700); e != nil {
			return "", e
		}
	} else if !os.IsExist(e) {
		return "", e
	}
	if _, e = v.Binary(hash); e != nil {
		return "", e
	}
	old, e := v.Release()
	if e != nil && !os.IsNotExist(e) {
		return "", e
	}
	if old.Current == hash {
		return hash, nil
	}
	r := Release{hash, old.Current}
	b, _ := json.Marshal(r)
	return hash, Atomic(v.Path("release.json"), b, 0600)
}
func (v Vault) Rollback() error {
	r, e := v.Release()
	if e != nil {
		return e
	}
	if r.Previous == "" {
		return errors.New("没有可回滚版本")
	}
	if _, e = v.Binary(r.Previous); e != nil {
		return e
	}
	r.Current, r.Previous = r.Previous, r.Current
	b, _ := json.Marshal(r)
	return Atomic(v.Path("release.json"), b, 0600)
}
func (v Vault) Summary() map[string]any {
	r, e := v.Release()
	if e != nil {
		return map[string]any{"installed": false, "detail": fmt.Sprint(e)}
	}
	return map[string]any{"installed": true, "release": r}
}
