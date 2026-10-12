package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"tunnelstudio.local/studio/internal/config"
	rt "tunnelstudio.local/studio/internal/runtime"
	"tunnelstudio.local/studio/internal/vault"
)

func main() {
	data, e := run(os.Args[1:])
	r := rt.Reply{OK: e == nil, Data: data}
	if e != nil {
		r.Error = e.Error()
	}
	json.NewEncoder(os.Stdout).Encode(r)
	if e != nil {
		os.Exit(1)
	}
}
func run(args []string) (any, error) {
	if len(args) == 0 {
		return nil, errors.New("缺少操作")
	}
	action := args[0]
	arg := ""
	if len(args) > 1 {
		arg = args[1]
	}
	if action == "version" {
		return map[string]any{"version": rt.Version, "schema": 1, "protocols": []string{"wireguard", "amneziawg"}, "backend": "amneziawg-go v3.1.20260828"}, nil
	}
	if action == "validate" {
		b, e := io.ReadAll(io.LimitReader(os.Stdin, 100001))
		if e != nil {
			return nil, e
		}
		p, e := vault.Decode(b)
		if e != nil {
			return nil, e
		}
		c, e := config.Parse(p)
		return map[string]any{"warnings": cWarnings(c)}, e
	}
	if e := rt.RootCheck(); e != nil {
		return nil, e
	}
	v := vault.Vault{Root: rt.Root}
	if action == "diagnose" {
		return rt.Diagnose(v), nil
	}
	if action == "install" {
		if e := v.Init(); e != nil {
			return nil, e
		}
	}
	if _, e := v.Check(); e != nil {
		return nil, e
	}
	if action == "serve" {
		return nil, rt.Serve(v)
	}
	if action == "status" {
		m := v.Summary()
		s, e := rt.Call(v, "status")
		m["running"] = e == nil
		if e == nil {
			m["tunnel"] = s
		}
		if p, e := v.Read(); e == nil {
			m["protocol"] = p.Protocol
			m["configured"] = true
		}
		_, e = os.Stat(rt.BootPath())
		m["boot"] = e == nil
		_, je := os.Stat(v.Path("runtime/routes.json"))
		m["recoveryRequired"] = s == nil && je == nil
		return m, nil
	}
	if action == "load" {
		return v.Read()
	}
	if action == "backups" {
		return v.List()
	}
	unlock, e := rt.Lock(v.Path("manager.lock"))
	if e != nil {
		return nil, e
	}
	defer unlock()
	switch action {
	case "install":
		if e = rt.Stopped(v); e != nil {
			return nil, e
		}
		exe, e := os.Executable()
		if e != nil {
			return nil, e
		}
		if _, e = os.Stat(v.Path("profile.json")); e == nil {
			if _, e = v.Read(); e != nil {
				return nil, e
			}
		}
		hash, e := v.Install(exe)
		if e != nil {
			return nil, e
		}
		if _, e = os.Stat(rt.BootPath()); e == nil {
			if e = rt.Boot(v, true); e != nil {
				return nil, e
			}
		}
		return map[string]string{"release": hash}, nil
	case "start", "boot-start":
		if action == "boot-start" {
			r, e := v.Release()
			if e != nil {
				return nil, e
			}
			exe, e := os.Executable()
			if e != nil {
				return nil, e
			}
			hash, e := vault.Digest(exe)
			if e != nil || hash != r.Current {
				return nil, errors.New("自启资源已过时；请重新设置自启")
			}
		}
		return nil, rt.Start(v)
	case "stop":
		if e = rt.Stopped(v); e == nil {
			return nil, nil
		}
		if _, e = rt.Call(v, "stop"); e != nil {
			return nil, e
		}
		for i := 0; i < 60; i++ {
			if e = rt.Stopped(v); e == nil {
				return nil, nil
			}
			time.Sleep(100 * time.Millisecond)
		}
		return nil, errors.New("停止尚未完成，请刷新确认")
	case "save-file", "restore":
		if e = rt.Stopped(v); e != nil {
			return nil, e
		}
		if action == "restore" {
			return nil, v.Restore(arg)
		}
		b, e := consumeUpload(arg)
		if e != nil {
			return nil, e
		}
		p, e := vault.Decode(b)
		if e != nil {
			return nil, e
		}
		if e = v.Save(p); e != nil {
			return nil, e
		}
		c, _ := config.Parse(p)
		return map[string]any{"warnings": c.Warnings}, nil
	case "validate-file":
		b, e := consumeUpload(arg)
		if e != nil {
			return nil, e
		}
		p, e := vault.Decode(b)
		if e != nil {
			return nil, e
		}
		c, e := config.Parse(p)
		return map[string]any{"warnings": cWarnings(c)}, e
	case "backup":
		return v.Backup()
	case "rollback":
		if e = rt.Stopped(v); e != nil {
			return nil, e
		}
		r, e := v.Release()
		if e != nil {
			return nil, e
		}
		previous, e := v.Binary(r.Previous)
		if e != nil {
			return nil, e
		}
		if p, e := v.Read(); e == nil {
			raw, _ := json.Marshal(p)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			cmd := exec.CommandContext(ctx, previous, "validate")
			cmd.Stdin = strings.NewReader(string(raw))
			output, err := cmd.Output()
			cancel()
			var check rt.Reply
			if err != nil || json.Unmarshal(output, &check) != nil || !check.OK {
				return nil, errors.New("上一版本无法校验当前配置，未切换版本")
			}
		} else if !os.IsNotExist(e) {
			return nil, e
		}
		if e = v.Rollback(); e != nil {
			return nil, e
		}
		if _, e = os.Stat(rt.BootPath()); e == nil {
			return nil, rt.Boot(v, true)
		}
		return nil, nil
	case "boot-on":
		if _, e = v.Read(); e != nil {
			return nil, e
		}
		return nil, rt.Boot(v, true)
	case "boot-off":
		return nil, rt.Boot(v, false)
	default:
		return nil, fmt.Errorf("不支持的操作")
	}
}
func cWarnings(c *config.Parsed) []string {
	if c == nil {
		return nil
	}
	return c.Warnings
}
func consumeUpload(path string) ([]byte, error) {
	prefix := "/data/data/com.minikano.f50_sms/files/"
	clean := filepath.Clean(path)
	if !strings.HasPrefix(clean, prefix) || clean != path || strings.ContainsAny(path, "\r\n\x00") {
		return nil, errors.New("上传路径不是 KANO 私有文件路径")
	}
	// Reject all symlink components under the app's files directory.
	rel := strings.TrimPrefix(clean, prefix)
	at := strings.TrimSuffix(prefix, "/")
	for _, part := range strings.Split(rel, "/") {
		at += "/" + part
		if e := vault.NoLink(at); e != nil {
			return nil, e
		}
	}
	f, e := os.Open(clean)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Size() > 100000 {
		return nil, errors.New("上传文件类型或大小无效")
	}
	b, e := io.ReadAll(io.LimitReader(f, 100001))
	if e != nil {
		return nil, e
	}
	if _, e = vault.Decode(b); e != nil {
		return nil, e
	}
	if e = os.Remove(clean); e != nil {
		return nil, errors.New("无法清理上传的配置，请检查权限")
	}
	return b, nil
}
