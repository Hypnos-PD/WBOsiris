//go:build windows

package nativeclient

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// platformSteamRoots 返回 Windows 上的 Steam 安装位置。
//
// 注册表是唯一可靠来源：Steam 允许装在任意位置，Program Files 下的默认路径
// 只作为兜底，方便用户把库挪走之后仍然能发现。
func platformSteamRoots() []string {
	var roots []string
	if key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE); err == nil {
		if value, _, err := key.GetStringValue("SteamPath"); err == nil && value != "" {
			roots = append(roots, filepath.Clean(value))
		}
		key.Close()
	}
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, registry.QUERY_VALUE); err == nil {
		if value, _, err := key.GetStringValue("InstallPath"); err == nil && value != "" {
			roots = append(roots, filepath.Clean(value))
		}
		key.Close()
	}
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		if base := os.Getenv(env); base != "" {
			roots = append(roots, filepath.Join(base, "Steam"))
		}
	}
	return roots
}
