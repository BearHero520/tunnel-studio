//go:build !linux

package runtime

import (
	"errors"
	"os"
	"tunnelstudio.local/studio/internal/vault"
)

func Lock(path string) (func(), error) { return func() {}, nil }
func DetachAttrs(c any)                {}
func RootCheck() error {
	if os.Getenv("KTS_HOST_TEST") == "1" {
		return nil
	}
	return errors.New("设备操作仅支持 Android/Linux root")
}
func Start(v vault.Vault) error      { return errors.New("该平台不启动设备隧道") }
func Serve(v vault.Vault) error      { return errors.New("该平台不启动设备隧道") }
func Stopped(v vault.Vault) error    { return nil }
func BootStart(v vault.Vault) error  { return errors.New("开机启动仅支持 Android/Linux") }
func CancelBoot(v vault.Vault) error { return nil }
func openBootFile(path string, flags int) (*os.File, error) {
	if e := vault.NoLink(path); e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	return os.OpenFile(path, flags, 0600)
}
func lockBootFile(f *os.File) (func(), error) { return func() {}, nil }
func Diagnose(v vault.Vault) map[string]any   { return map[string]any{"platform": "host-only"} }
func NetworkCheck(v vault.Vault, target string) (map[string]any, error) {
	return nil, errors.New("联网检测仅支持 Android/Linux 隧道")
}
