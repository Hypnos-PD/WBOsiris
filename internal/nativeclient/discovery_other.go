//go:build !windows

package nativeclient

// platformSteamRoots 返回非 Windows 平台上的额外 Steam 安装位置。
//
// Linux/macOS 的主位置在 steamRoots 里按 HOME 推导，这里只补环境变量能覆盖的
// 情况，保持实现不依赖平台特有 API。
func platformSteamRoots() []string {
	return nil
}
