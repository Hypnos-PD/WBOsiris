package nativeclient

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Install 是找到的一份客户端。
//
// ProtonPrefix 非空表示这份客户端是 Windows 构建、由 Proton/Wine 运行：它决定了
// 隔离方式（Linux 侧用网络命名空间，Windows 侧用 AppContainer）与路径映射
// （客户端看到的是 D:\… 之类的 DOS 盘符，宿主机看到的是真实路径）。
type Install struct {
	Root         string
	Library      string
	SteamRoot    string
	ProtonPrefix string
}

// Discovery 是一次发现的结果，用于报告而不是判定。
type Discovery struct {
	SteamRoots []string
	Libraries  []string
	Installs   []Install
}

// Discover 在默认位置查找 Steam 与已安装的客户端。
//
// 找不到不是错误：用户完全可能把客户端放在别处，调用方应该把结果原样报出来，
// 并支持显式指定目录。
func Discover() Discovery {
	return discoverFrom(steamRoots())
}

// discoverFrom 是发现逻辑的本体；把候选根目录作为参数传进来，测试就不必依赖
// 这台机器上真实装了哪些游戏。
func discoverFrom(roots []string) Discovery {
	var result Discovery
	for _, steamRoot := range roots {
		steamRoot = canonical(steamRoot)
		if !isDir(steamRoot) || contains(result.SteamRoots, steamRoot) {
			continue
		}
		result.SteamRoots = append(result.SteamRoots, steamRoot)
		for _, library := range Libraries(steamRoot) {
			// 同一个库经常通过软链接出现多次（~/.steam/root → ~/.local/share/Steam），
			// 按真实路径去重，否则同一份客户端会被报成好几份，import 会因为
			// "找到多份客户端"而拒绝执行。
			library = canonical(library)
			if contains(result.Libraries, library) {
				continue
			}
			result.Libraries = append(result.Libraries, library)
			prefix := findProtonPrefix(library)
			clientRoot := filepath.Join(library, "steamapps", "common", InstallDir)
			if !isDir(clientRoot) {
				continue
			}
			result.Installs = append(result.Installs, Install{
				Root:         clientRoot,
				Library:      library,
				SteamRoot:    steamRoot,
				ProtonPrefix: prefix,
			})
		}
	}
	sort.Strings(result.SteamRoots)
	sort.Strings(result.Libraries)
	sort.Slice(result.Installs, func(i, j int) bool { return result.Installs[i].Root < result.Installs[j].Root })
	return result
}

// Inspect 把显式指定的目录包成一份 Install，并补上同库里的 Proton 前缀信息。
func Inspect(root string) (Install, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Install{}, err
	}
	abs = canonical(abs)
	install := Install{Root: abs}
	// <库>/steamapps/common/<目录> → <库>
	library := filepath.Dir(filepath.Dir(filepath.Dir(abs)))
	if filepath.Base(filepath.Dir(filepath.Dir(abs))) == "steamapps" {
		install.Library = library
		install.ProtonPrefix = findProtonPrefix(library)
	}
	return install, nil
}

// Libraries 返回一个 Steam 根目录下的全部库目录（含它自己）。
func Libraries(steamRoot string) []string {
	libraries := []string{steamRoot}
	path := filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf")
	raw, err := os.ReadFile(path)
	if err != nil {
		return libraries
	}
	doc, err := parseVDF(raw)
	if err != nil {
		return libraries
	}
	for _, entry := range libraryPaths(doc) {
		if entry == "" || contains(libraries, entry) {
			continue
		}
		libraries = append(libraries, entry)
	}
	return libraries
}

// findProtonPrefix 找出这个库里该应用的 Proton/Wine 前缀。
//
// 目录存在就说明客户端由 Proton 运行，这是我们判断"要不要走 Linux 侧隔离"的
// 唯一依据——不能靠操作系统推断，因为在 Linux 上装的完全可以是 Windows 构建。
func findProtonPrefix(library string) string {
	if library == "" {
		return ""
	}
	prefix := filepath.Join(library, "steamapps", "compatdata", SteamAppID)
	if isDir(filepath.Join(prefix, "pfx")) {
		return prefix
	}
	return ""
}

func steamRoots() []string {
	var roots []string
	for _, key := range []string{"STEAM_ROOT", "STEAM_PATH", "SteamPath"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			roots = append(roots, value)
		}
	}
	home, err := os.UserHomeDir()
	if err == nil {
		switch runtime.GOOS {
		case "windows":
			roots = append(roots, platformSteamRoots()...)
		case "darwin":
			roots = append(roots, filepath.Join(home, "Library", "Application Support", "Steam"))
		default:
			roots = append(roots,
				filepath.Join(home, ".local", "share", "Steam"),
				filepath.Join(home, ".steam", "steam"),
				filepath.Join(home, ".steam", "root"),
				filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", "data", "Steam"),
			)
		}
	} else {
		roots = append(roots, platformSteamRoots()...)
	}
	return roots
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// canonical 解析软链接，得到一个可以当作身份的路径；解析失败时退回绝对路径。
func canonical(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
