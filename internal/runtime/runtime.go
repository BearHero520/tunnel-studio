package runtime

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"time"
	"tunnelstudio.local/studio/internal/vault"
)

const Root = "/data/adb/kano-tunnel-studio"
const Interface = "kts0"
const Version = "0.1.5"

type Reply struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func Call(v vault.Vault, action string) (map[string]any, error) {
	if _, e := v.Check(); e != nil {
		return nil, e
	}
	c, e := net.DialTimeout("unix", v.Path("runtime/control.sock"), time.Second)
	if e != nil {
		return nil, errors.New("隧道未运行或控制通道不可用")
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(4 * time.Second))
	if e = json.NewEncoder(c).Encode(map[string]string{"action": action}); e != nil {
		return nil, e
	}
	var out map[string]any
	e = json.NewDecoder(c).Decode(&out)
	return out, e
}
func BootPath() string { return "/data/adb/service.d/kano-tunnel-studio.sh" }
func Boot(v vault.Vault, enabled bool) error {
	id, e := v.Check()
	if e != nil {
		return e
	}
	path := BootPath()
	marker := "#!/system/bin/sh\n# " + vault.Owner + " " + id.ID + "\n"
	b, e := os.ReadFile(path)
	if e == nil {
		if e = vault.NoLink(path); e != nil {
			return e
		}
		if len(b) < len(marker) || string(b[:len(marker)]) != marker {
			return errors.New("自启文件已被其他资源占用")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if !enabled {
		if os.IsNotExist(e) {
			return nil
		}
		return os.Remove(path)
	}
	if e = vault.NoLink(filepath.Dir(path)); e != nil {
		return errors.New("未发现 Magisk service.d；不创建其他自启入口")
	}
	r, e := v.Release()
	if e != nil {
		return e
	}
	bin, e := v.Binary(r.Current)
	if e != nil {
		return e
	}
	// Rewritten when the release pointer changes; verify the executable before root execution.
	body := marker + "(sleep 25; test ! -L '" + bin + "' && test \"$(sha256sum '" + bin + "' | cut -d ' ' -f 1)\" = '" + r.Current + "' && '" + bin + "' boot-start >/dev/null 2>&1) &\n"
	return vault.Atomic(path, []byte(body), 0700)
}
