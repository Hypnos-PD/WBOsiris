package nativeclient

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ProtonTool 是把这份客户端跑起来所需要的全部外部信息。
//
// Steam 在没有启动选项时给这个游戏拼出的命令行是这样（2026-09 本机实测，从
// /proc/<pid>/cmdline 读出来的，不是推的）：
//
//	<steam>/ubuntu12_32/reaper SteamLaunch AppId=2584990 -- \
//	<steam>/steamapps/common/SteamLinuxRuntime_4/_v2-entry-point --verb=waitforexitandrun -- \
//	<steam>/steamapps/common/Proton - Experimental/proton waitforexitandrun <客户端>/ShadowverseWB.exe
//
// 我们的启动器要自己拼出这一段——用户不该为了让游戏连上本地服务端去写启动选项。
// 在这件事上 Steam 只提供两样只读的东西：客户端装在哪、这份客户端用哪个 Proton。
//
// 两者都不是从版本号猜的：Proton 由 compatdata 里记录的版本加 config.vdf 里用户
// 显式指定的兼容工具共同定位，运行时由那个 Proton 自己声明的 require_tool_appid
// 经 appmanifest 反查目录名。这两条链路都能追到文件，找不到时会明说找不到。
type ProtonTool struct {
	AppID      string
	SteamRoot  string
	Library    string
	CompatData string

	// Name / Directory / Script / Version 是选中的 Proton 工具。
	Name      string
	Directory string
	Script    string
	Version   string

	// EntryPoint 是 Steam Linux Runtime 的入口，RuntimeDir 是它所在的目录。
	// 运行时缺失时 EntryPoint 为空，调用方应当据此给出提示而不是硬闯。
	EntryPoint string
	RuntimeDir string

	// Mapping 是 config.vdf 里用户给这个 app 显式指定的兼容工具名，没指定时为空。
	Mapping string
}

// GameCommand 拼出在这个运行时里跑起这份客户端的命令。
func (t ProtonTool) GameCommand(clientDir string) []string {
	var line []string
	if t.EntryPoint != "" {
		line = append(line, t.EntryPoint, "--verb=waitforexitandrun", "--")
	}
	return append(line, t.Script, "waitforexitandrun", filepath.Join(clientDir, ProcessName))
}

// Environment 返回启动这个游戏必须补上的环境变量。
//
// 这份清单不是想出来的：是拿一个由 Steam 正常启动的游戏进程，读
// /proc/<pid>/environ 对照出来的。Steam 自己设置、我们从父进程继承不到的那些键
// 全在这里；WINEPREFIX / WINEDLLPATH / LD_PRELOAD 这类由 Proton 的脚本和运行时
// 自己补，不归我们管。
func (t ProtonTool) Environment(clientDir string) []string {
	steamapps := filepath.Join(t.Library, "steamapps")
	variables := map[string]string{
		"STEAM_COMPAT_CLIENT_INSTALL_PATH": t.SteamRoot,
		"STEAM_COMPAT_DATA_PATH":           t.CompatData,
		"STEAM_COMPAT_APP_ID":              t.AppID,
		"STEAM_COMPAT_INSTALL_PATH":        clientDir,
		"STEAM_COMPAT_LIBRARY_PATHS":       steamapps,
		"STEAM_COMPAT_TOOL_PATHS":          joinPaths(t.Directory, t.RuntimeDir),
		"STEAM_COMPAT_MOUNTS":              joinPaths(steamworksShared(t.Library), t.Directory, t.RuntimeDir),
		"STEAM_COMPAT_SHADER_PATH":         filepath.Join(steamapps, "shadercache", t.AppID),
		"STEAM_COMPAT_PROTON":              "1",
		"STEAM_COMPAT_FLAGS":               "search-cwd",
		"SteamAppId":                       t.AppID,
		"SteamGameId":                      t.AppID,
		"SteamOverlayGameId":               t.AppID,
	}
	keys := make([]string, 0, len(variables))
	for key := range variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+variables[key])
	}
	return environment
}

// ResolveProton 从本机 Steam 的配置里推出这份客户端该用哪个 Proton、哪个运行时。
//
// prefix 是 compatdata 里那个前缀目录；为空时由 library 推出来。它承载着两个关键
// 信息：这份客户端是不是 Proton 运行的，以及它上一次是用哪个兼容工具跑起来的。
func ResolveProton(steamRoot, library, appID, prefix string) (ProtonTool, error) {
	tool := ProtonTool{AppID: appID, SteamRoot: steamRoot, Library: library, CompatData: prefix}
	if tool.CompatData == "" && library != "" {
		tool.CompatData = filepath.Join(library, "steamapps", "compatdata", appID)
	}
	candidates := protonCandidates(steamRoot, library)
	if len(candidates) == 0 {
		return tool, fmt.Errorf("nativeclient: 在 %s 下没有找到任何 Proton", steamRoot)
	}
	tool.Mapping = CompatToolName(steamRoot, appID)
	chosen, err := chooseProton(candidates, tool.Mapping, recordedToolVersion(tool.CompatData))
	if err != nil {
		return tool, err
	}
	tool, _ = withProton(tool, chosen.Directory)
	tool.EntryPoint, tool.RuntimeDir = runtimeEntryPoint(steamRoot, library, tool.Directory)
	return tool, nil
}

// ProtonAt 用调用方显式给出的 Proton 目录（或 proton 脚本路径）与运行时入口构造
// 一份 ProtonTool。自动推导不成立时这是唯一的出路，所以错误信息要说清哪里不对。
func ProtonAt(steamRoot, library, appID, prefix, proton, runtime string) (ProtonTool, error) {
	tool, err := withProton(ProtonTool{
		AppID:      appID,
		SteamRoot:  steamRoot,
		Library:    library,
		CompatData: prefix,
	}, proton)
	if err != nil {
		return tool, err
	}
	if tool.CompatData == "" && library != "" {
		tool.CompatData = filepath.Join(library, "steamapps", "compatdata", appID)
	}
	tool.EntryPoint, tool.RuntimeDir = runtimeEntryPoint(steamRoot, library, tool.Directory)
	if runtime != "" {
		info, err := os.Stat(runtime)
		if err != nil || info.IsDir() {
			return tool, fmt.Errorf("nativeclient: --runtime 指定的 %s 不是可执行文件", runtime)
		}
		tool.EntryPoint = runtime
		tool.RuntimeDir = canonical(filepath.Dir(runtime))
	}
	return tool, nil
}

// withProton 把 Proton 目录（或 proton 脚本的路径）填进 ProtonTool。
func withProton(tool ProtonTool, proton string) (ProtonTool, error) {
	if proton == "" {
		return tool, fmt.Errorf("nativeclient: 没有指定 Proton")
	}
	directory := proton
	if filepath.Base(proton) == "proton" {
		directory = filepath.Dir(proton)
	}
	script := filepath.Join(directory, "proton")
	if info, err := os.Stat(script); err != nil || info.IsDir() {
		return tool, fmt.Errorf("nativeclient: %s 里没有 proton 脚本", directory)
	}
	tool.Name = filepath.Base(directory)
	tool.Directory = canonical(directory)
	tool.Script = script
	tool.Version = toolVersion(directory)
	return tool, nil
}

// CompatToolName 返回用户给这个 app 显式指定的兼容工具名。
//
// Steam 把这份选择写在 config/config.vdf 的 CompatToolMapping 里。没有这一项时
// 说明是 Steam 自己挑的（我们只能靠 compatdata 里记录的版本来反推），返回空串。
func CompatToolName(steamRoot, appID string) string {
	raw, err := os.ReadFile(filepath.Join(steamRoot, "config", "config.vdf"))
	if err != nil {
		return ""
	}
	document, err := parseVDF(raw)
	if err != nil {
		return ""
	}
	mapping := findBlock(document, "CompatToolMapping")
	if mapping == nil {
		return ""
	}
	entry, ok := mapping[appID].(map[string]any)
	if !ok {
		return ""
	}
	name, _ := entry["name"].(string)
	return name
}

// findBlock 在 VDF 树里按名字找第一个块。
//
// 顶层会随 Steam 版本变化（InstallConfigStore/Software/Valve/Steam/…），所以不写死
// 层级；按名字排序后递归，结果才是确定的。
func findBlock(node map[string]any, key string) map[string]any {
	if child, ok := node[key].(map[string]any); ok {
		return child
	}
	keys := make([]string, 0, len(node))
	for name := range node {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		child, ok := node[name].(map[string]any)
		if !ok {
			continue
		}
		if found := findBlock(child, key); found != nil {
			return found
		}
	}
	return nil
}

// protonCandidate 是一份装在本机的 Proton 工具。
type protonCandidate struct {
	Name      string
	Directory string
	Script    string
	Version   string
}

// protonCandidates 列出本机所有可用的 Proton。
//
// 三个来源：Steam 自带的（根库的 steamapps/common）、其它库里的、以及用户自己放进
// compatibilitytools.d 的。判断依据是目录里有可执行的 proton 脚本。
func protonCandidates(steamRoot, library string) []protonCandidate {
	roots := []string{
		filepath.Join(steamRoot, "steamapps", "common"),
		filepath.Join(library, "steamapps", "common"),
		filepath.Join(steamRoot, "compatibilitytools.d"),
	}
	seen := map[string]bool{}
	var candidates []protonCandidate
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				names = append(names, entry.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			directory := canonical(filepath.Join(root, name))
			if seen[directory] {
				continue
			}
			script := filepath.Join(directory, "proton")
			if info, err := os.Stat(script); err != nil || info.IsDir() {
				continue
			}
			seen[directory] = true
			candidates = append(candidates, protonCandidate{
				Name:      name,
				Directory: directory,
				Script:    script,
				Version:   toolVersion(directory),
			})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Directory < candidates[j].Directory })
	return candidates
}

// toolVersion 读 Proton 目录里 version 文件的版本行。
//
// 这个文件第一行是构建时间戳、第二行才是版本（experimental-11.0-20260917b-x86_64、
// GE-Proton11-3）。所以取最后一行非空、又不是纯数字的。
func toolVersion(directory string) string {
	raw, err := os.ReadFile(filepath.Join(directory, "version"))
	if err != nil {
		return ""
	}
	var found string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, err := strconv.ParseFloat(line, 64); err == nil {
			continue
		}
		found = line
	}
	return found
}

// recordedToolVersion 读 compatdata 里记录的兼容工具版本。
//
// config_info 只有一行，最省事；退一步读 version 文件的第一行。
func recordedToolVersion(prefix string) string {
	if prefix == "" {
		return ""
	}
	if raw, err := os.ReadFile(filepath.Join(prefix, "config_info")); err == nil {
		if line := firstLine(string(raw)); line != "" {
			return line
		}
	}
	raw, err := os.ReadFile(filepath.Join(prefix, "version"))
	if err != nil {
		return ""
	}
	return firstLine(string(raw))
}

func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// chooseProton 在候选里选出这份客户端该用的那个。
//
// 顺序是有意为之：用户显式指定过就用它，否则用 compatdata 里记录的版本反推。
// 反推不中就给候选列表让人指定，绝不自作主张挑一个：挑错的后果是游戏起不来，
// 而错误信息里看不出这件事跟 Proton 有关。
func chooseProton(candidates []protonCandidate, mapping, recorded string) (protonCandidate, error) {
	if mapping != "" {
		matches := matchTools(candidates, func(candidate protonCandidate) bool {
			return toolNameMatches(candidate, mapping)
		})
		switch len(matches) {
		case 1:
			return matches[0], nil
		case 0:
			// 落空不致命：映射里的名字可能对应一个已经被删掉的工具，接着按记录反推。
		default:
			return protonCandidate{}, fmt.Errorf("nativeclient: config.vdf 指定了 %q，但它匹配到多个 Proton：%s",
				mapping, describeTools(matches))
		}
	}
	if recorded != "" {
		matches := matchTools(candidates, func(candidate protonCandidate) bool {
			return toolVersionMatches(candidate, recorded)
		})
		switch len(matches) {
		case 1:
			return matches[0], nil
		case 0:
		default:
			return protonCandidate{}, fmt.Errorf("nativeclient: 兼容工具版本 %q 匹配到多个 Proton：%s",
				recorded, describeTools(matches))
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	return protonCandidate{}, fmt.Errorf("nativeclient: 无法确定用哪个 Proton（compatdata 记录 %q，config.vdf 映射 %q），候选：%s；用 --proton 指定",
		recorded, mapping, describeTools(candidates))
}

func matchTools(candidates []protonCandidate, keep func(protonCandidate) bool) []protonCandidate {
	var matches []protonCandidate
	for _, candidate := range candidates {
		if keep(candidate) {
			matches = append(matches, candidate)
		}
	}
	return matches
}

// toolNameMatches 比较 Steam 内部工具名与目录名。
//
// 内部名形如 proton_experimental / proton_9.0 / proton_hotfix，而目录名是给人看的
// （Proton - Experimental、Proton 9.0 (Beta)）。归一化之后允许互为前缀：目录名里的
// (Beta) 在内部名里是不存在的。
func toolNameMatches(candidate protonCandidate, mapping string) bool {
	if candidate.Name == mapping {
		return true
	}
	want := normalizeToolName(mapping)
	if want == "" {
		return false
	}
	for _, value := range []string{candidate.Name, candidate.Version} {
		have := normalizeToolName(value)
		if have == "" {
			continue
		}
		if have == want || strings.HasPrefix(have, want) || strings.HasPrefix(want, have) {
			return true
		}
	}
	return false
}

func normalizeToolName(value string) string {
	var builder strings.Builder
	lastSeparator := false
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastSeparator = false
		case r == '.' || r == '-' || r == '_' || r == ' ':
			if !lastSeparator {
				builder.WriteByte('_')
				lastSeparator = true
			}
		default:
			// 括号之类的装饰字符直接丢掉。
		}
	}
	return strings.Trim(builder.String(), "_")
}

// toolVersionMatches 比较候选的版本行与 compatdata 里记录的版本。
//
// 官方 Proton 记录的是一份对人友好的版本（11.0-100），而工具目录里写的是构建串
// （experimental-11.0-20260917b-x86_64），两边不会相等。所以除了整体比较，还比
// 主次版本号（11.0）——实测这套够把 Experimental 与 9.0、Hotfix 区分开。
func toolVersionMatches(candidate protonCandidate, recorded string) bool {
	if candidate.Version == "" {
		return false
	}
	if candidate.Version == recorded || strings.Contains(candidate.Version, recorded) {
		return true
	}
	series := majorMinor(recorded)
	if series == "" {
		return false
	}
	return strings.Contains(candidate.Version, series)
}

// majorMinor 从 11.0-100 里取出 11.0；取不到时返回空串。
func majorMinor(version string) string {
	fields := strings.FieldsFunc(version, func(r rune) bool {
		return !(r >= '0' && r <= '9') && r != '.'
	})
	for _, field := range fields {
		parts := strings.Split(field, ".")
		// 至少要 major.minor 才够区分版本，单独的 11 会误伤。
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			return parts[0] + "." + parts[1]
		}
	}
	return ""
}

// runtimeEntryPoint 找到这个 Proton 要求的 Steam Linux Runtime 入口。
//
// 依据是 Proton 自己在 toolmanifest.vdf 里声明的 require_tool_appid，再用
// appmanifest_<appid>.acf 的 installdir 换出目录名——这正是 Steam 的查法，
// 比按目录名硬猜稳。声明缺失或运行时没装齐时，退回按已知目录名找。
func runtimeEntryPoint(steamRoot, library, protonDir string) (string, string) {
	if appID := requiredToolAppID(protonDir); appID != "" {
		for _, root := range []string{library, steamRoot} {
			installDir := appInstallDir(root, appID)
			if installDir == "" {
				continue
			}
			directory := filepath.Join(root, "steamapps", "common", installDir)
			if entry, ok := entryPointIn(directory); ok {
				return entry, canonical(directory)
			}
		}
	}
	known := []struct{ directory, entry string }{
		{"SteamLinuxRuntime_4", "_v2-entry-point"},
		{"SteamLinuxRuntime_sniper", "_v2-entry-point"},
		{"SteamLinuxRuntime_sniper", "run-in-sniper"},
		{"SteamLinuxRuntime_soldier", "_v2-entry-point"},
		{"SteamLinuxRuntime", "run"},
	}
	for _, root := range []string{library, steamRoot} {
		for _, want := range known {
			directory := filepath.Join(root, "steamapps", "common", want.directory)
			candidate := filepath.Join(directory, want.entry)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, canonical(directory)
			}
		}
	}
	return "", ""
}

// requiredToolAppID 读 Proton 的 toolmanifest.vdf，取出它依赖的运行时 appid。
func requiredToolAppID(protonDir string) string {
	raw, err := os.ReadFile(filepath.Join(protonDir, "toolmanifest.vdf"))
	if err != nil {
		return ""
	}
	document, err := parseVDF(raw)
	if err != nil {
		return ""
	}
	manifest, ok := document["manifest"].(map[string]any)
	if !ok {
		return ""
	}
	value, _ := manifest["require_tool_appid"].(string)
	return strings.TrimSpace(value)
}

// appInstallDir 读 appmanifest 里的 installdir；读不到时返回空串。
func appInstallDir(steamRoot, appID string) string {
	raw, err := os.ReadFile(filepath.Join(steamRoot, "steamapps", "appmanifest_"+appID+".acf"))
	if err != nil {
		return ""
	}
	document, err := parseVDF(raw)
	if err != nil {
		return ""
	}
	state, ok := document["AppState"].(map[string]any)
	if !ok {
		return ""
	}
	value, _ := state["installdir"].(string)
	return strings.TrimSpace(value)
}

func entryPointIn(directory string) (string, bool) {
	for _, name := range []string{"_v2-entry-point", "run-in-sniper", "run"} {
		candidate := filepath.Join(directory, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

// steamworksShared 是 Steamworks 共享运行库的目录，装了才挂进运行时。
func steamworksShared(library string) string {
	directory := filepath.Join(library, "steamapps", "common", "Steamworks Shared")
	if isDir(directory) {
		return directory
	}
	return ""
}

func joinPaths(paths ...string) string {
	var kept []string
	for _, path := range paths {
		if path != "" {
			kept = append(kept, path)
		}
	}
	return strings.Join(kept, ":")
}

func describeTools(candidates []protonCandidate) string {
	described := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		described = append(described, candidate.Name+"("+candidate.Version+")")
	}
	return strings.Join(described, ", ")
}
