package nativeclient

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSteam 搭一棵假的 Steam 树。形状照着本机真实的安装来：Steam 自带的 Proton 在
// steamapps/common 下，用户装的兼容工具在 compatibilitytools.d 下，运行时的目录名
// 由 appmanifest 的 installdir 决定。
type fakeSteam struct {
	root string
}

func newFakeSteam(t *testing.T) fakeSteam {
	t.Helper()
	steam := fakeSteam{root: t.TempDir()}
	steam.write("config/config.vdf", `"InstallConfigStore"
{
	"Software"
	{
		"Valve"
		{
			"Steam"
			{
				"CompatToolMapping"
				{
					"2135150"
					{
						"name"		"Proton-GE Latest"
						"config"	""
						"priority"	"250"
					}
				}
			}
		}
	}
}`)
	// 官方 Proton：version 第一行是构建时间戳，第二行才是版本。
	steam.addProton("steamapps/common/Proton - Experimental", "1789805668\nexperimental-11.0-20260917b-x86_64\n",
		`"manifest" { "version" "2" "require_tool_appid" "4183110" }`)
	steam.addProton("steamapps/common/Proton 9.0 (Beta)", "1749140930\nproton-9.0-4f\n", "")
	steam.addProton("steamapps/common/Proton Hotfix", "1787945902\nhotfix-20260828-ptr-x86_64\n", "")
	steam.addProton("compatibilitytools.d/Proton-GE Latest", "1784963766\nGE-Proton11-3\n", "")
	steam.write("steamapps/appmanifest_4183110.acf",
		`"AppState" { "appid" "4183110" "installdir" "SteamLinuxRuntime_4" }`)
	steam.write("steamapps/common/SteamLinuxRuntime_4/_v2-entry-point", "#!/bin/sh\n")
	steam.write("steamapps/compatdata/2584990/config_info", "11.0-100\n")
	return steam
}

func (s fakeSteam) addProton(relative, version, manifest string) {
	s.write(relative+"/proton", "#!/usr/bin/python3\n")
	s.write(relative+"/version", version)
	if manifest != "" {
		s.write(relative+"/toolmanifest.vdf", manifest)
	}
}

func (s fakeSteam) write(relative, content string) {
	path := filepath.Join(s.root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		panic(err)
	}
}

func (s fakeSteam) resolve(t *testing.T, appID string) ProtonTool {
	t.Helper()
	prefix := filepath.Join(s.root, "steamapps", "compatdata", appID)
	tool, err := ResolveProton(s.root, s.root, appID, prefix)
	if err != nil {
		t.Fatalf("解析 Proton 失败: %v", err)
	}
	return tool
}

// 没有映射时（Steam 自己挑的），只能靠 compatdata 里记的版本反推。官方 Proton 的
// 记录是 "11.0-100"，工具目录里写的是构建串，两边不相等，所以这条测试盯的是主次
// 版本号这一层：命中的必须是 Experimental，不能是 9.0 或 Hotfix。
func TestResolveProtonFromRecordedVersion(t *testing.T) {
	steam := newFakeSteam(t)
	tool := steam.resolve(t, SteamAppID)
	if tool.Name != "Proton - Experimental" {
		t.Fatalf("应该选中 Experimental，实际 %s（%s）", tool.Name, tool.Version)
	}
	if tool.Mapping != "" {
		t.Errorf("这份 config.vdf 没有给 %s 指定工具，映射不该有值：%q", SteamAppID, tool.Mapping)
	}
	if !strings.HasSuffix(tool.EntryPoint, "SteamLinuxRuntime_4/_v2-entry-point") {
		t.Errorf("运行时入口不对：%s", tool.EntryPoint)
	}
}

// 用户显式指定过兼容工具时以它为准，哪怕 compatdata 记的是另一个版本。
func TestResolveProtonHonoursMapping(t *testing.T) {
	steam := newFakeSteam(t)
	tool := steam.resolve(t, "2135150")
	if tool.Name != "Proton-GE Latest" {
		t.Fatalf("应该按 config.vdf 的映射选中 Proton-GE Latest，实际 %s", tool.Name)
	}
	if tool.Mapping != "Proton-GE Latest" {
		t.Errorf("映射名没读出来：%q", tool.Mapping)
	}
}

func TestCompatToolName(t *testing.T) {
	steam := newFakeSteam(t)
	if name := CompatToolName(steam.root, "2135150"); name != "Proton-GE Latest" {
		t.Errorf("读错映射：%q", name)
	}
	if name := CompatToolName(steam.root, SteamAppID); name != "" {
		t.Errorf("没有映射时应该返回空串，实际 %q", name)
	}
	if name := CompatToolName(filepath.Join(steam.root, "不存在"), SteamAppID); name != "" {
		t.Errorf("文件不存在时应返回空串，实际 %q", name)
	}
}

// 反推不中时不许自作主张：候选列出来让人指定。
func TestChooseProtonRefusesToGuess(t *testing.T) {
	candidates := []protonCandidate{
		{Name: "Proton - Experimental", Directory: "/a", Version: "experimental-11.0-x"},
		{Name: "Proton 9.0 (Beta)", Directory: "/b", Version: "proton-9.0-4f"},
	}
	if _, err := chooseProton(candidates, "", "12.0-1"); err == nil {
		t.Fatal("版本对不上时应该报错，而不是随便挑一个")
	}
	if _, err := chooseProton(candidates, "", "11.0-100"); err != nil {
		t.Fatalf("版本能对上时不该报错: %v", err)
	}
	if _, err := chooseProton(candidates[:1], "", ""); err != nil {
		t.Fatalf("只有一个候选时可以直接用它: %v", err)
	}
}

func TestToolNameMatches(t *testing.T) {
	cases := []struct {
		directory string
		mapping   string
		want      bool
	}{
		{"Proton - Experimental", "proton_experimental", true},
		{"Proton 9.0 (Beta)", "proton_9.0", true},
		{"Proton Hotfix", "proton_hotfix", true},
		{"Proton-GE Latest", "Proton-GE Latest", true},
		{"Proton Hotfix", "proton_experimental", false},
	}
	for _, item := range cases {
		candidate := protonCandidate{Name: item.directory}
		if got := toolNameMatches(candidate, item.mapping); got != item.want {
			t.Errorf("%s vs %s：期望 %v，实际 %v", item.directory, item.mapping, item.want, got)
		}
	}
}

func TestMajorMinor(t *testing.T) {
	cases := map[string]string{
		"11.0-100":                    "11.0",
		"GE-Proton11-3":               "",
		"9.0-202":                     "9.0",
		"experimental-11.0-20260917b": "11.0",
		"":                            "",
		"hotfix-20260828-ptr-x86_64":  "",
	}
	for version, want := range cases {
		if got := majorMinor(version); got != want {
			t.Errorf("majorMinor(%q) = %q，期望 %q", version, got, want)
		}
	}
}

// 环境变量这份清单是拿一个由 Steam 启动的真实游戏进程对照出来的，改动它要有理由。
func TestEnvironmentCoversSteamVariables(t *testing.T) {
	tool := ProtonTool{
		AppID:      SteamAppID,
		SteamRoot:  "/steam",
		Library:    "/steam",
		CompatData: "/steam/steamapps/compatdata/2584990",
		Directory:  "/steam/steamapps/common/Proton - Experimental",
		RuntimeDir: "/steam/steamapps/common/SteamLinuxRuntime_4",
	}
	environment := tool.Environment("/steam/steamapps/common/ShadowverseWB")
	for _, want := range []string{
		"SteamAppId=" + SteamAppID,
		"SteamGameId=" + SteamAppID,
		"STEAM_COMPAT_CLIENT_INSTALL_PATH=/steam",
		"STEAM_COMPAT_DATA_PATH=/steam/steamapps/compatdata/2584990",
		"STEAM_COMPAT_INSTALL_PATH=/steam/steamapps/common/ShadowverseWB",
		"STEAM_COMPAT_PROTON=1",
	} {
		if !containsString(environment, want) {
			t.Errorf("环境变量里缺少 %s", want)
		}
	}
}

func TestGameCommand(t *testing.T) {
	tool := ProtonTool{
		Script:     "/steam/common/Proton - Experimental/proton",
		EntryPoint: "/steam/common/SteamLinuxRuntime_4/_v2-entry-point",
	}
	line := tool.GameCommand("/steam/common/ShadowverseWB")
	want := []string{
		"/steam/common/SteamLinuxRuntime_4/_v2-entry-point",
		"--verb=waitforexitandrun",
		"--",
		"/steam/common/Proton - Experimental/proton",
		"waitforexitandrun",
		"/steam/common/ShadowverseWB/" + ProcessName,
	}
	if len(line) != len(want) {
		t.Fatalf("命令长度不对：%v", line)
	}
	for index := range want {
		if line[index] != want[index] {
			t.Errorf("第 %d 段：期望 %q，实际 %q", index, want[index], line[index])
		}
	}
	if withoutRuntime := (ProtonTool{Script: tool.Script}).GameCommand("/client"); len(withoutRuntime) != 3 {
		t.Errorf("没有运行时入口时不该凭空补一段：%v", withoutRuntime)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
