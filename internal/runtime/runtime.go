package runtime

import (
	"encoding/json"
	"errors"
	"net"
	"time"
	"tunnelstudio.local/studio/internal/vault"
)

const Root = "/data/adb/kano-tunnel-studio"
const Interface = "kts0"
const Version = "0.1.7"

var errOperationBusy = errors.New("另一个操作正在执行")

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
