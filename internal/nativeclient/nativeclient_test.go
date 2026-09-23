package nativeclient

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedProfileIsUsable(t *testing.T) {
	profile, err := ProfileByID("")
	if err != nil {
		t.Fatalf("内置 profile 应该能加载: %v", err)
	}
	if len(profile.RequiredFiles) == 0 {
		t.Fatal("内置 profile 没有登记任何文件")
	}
	var critical int
	for _, file := range profile.RequiredFiles {
		if file.Path == CriticalPath {
			critical++
		}
		if file.SHA256 != strings.ToLower(file.SHA256) {
			t.Errorf("%s 的哈希没有归一化成小写", file.Path)
		}
		if strings.Contains(file.Path, `\`) {
			t.Errorf("%s 的路径没有归一化成正斜杠", file.Path)
		}
	}
	if critical != 1 {
		t.Fatalf("关键文件应该恰好登记一次，实际 %d 次", critical)
	}
	if profile.GameAssemblySHA256 != strings.ToLower(profile.GameAssemblySHA256) {
		t.Error("module 哈希没有归一化成小写")
	}
}

func TestProfileRejectsBadInput(t *testing.T) {
	hash := strings.Repeat("a", 64)
	// 关键文件由实现决定，夹具跟着它走，免得两边各改各的。
	module := CriticalPath
	valid := `{"schema":1,"id":"x","processName":"p.exe","moduleName":"` + module + `","gameAssemblySHA256":"` +
		hash + `","requiredFiles":[{"path":"` + module + `","bytes":1,"sha256":"` + hash + `"}]}`
	if _, err := LoadProfile(strings.NewReader(valid)); err != nil {
		t.Fatalf("最小合法 profile 应该通过: %v", err)
	}
	cases := map[string]string{
		"缺少关键文件":     `{"schema":1,"id":"x","processName":"p.exe","moduleName":"m.dll","gameAssemblySHA256":"` + hash + `","requiredFiles":[]}`,
		"路径逃逸":       `{"schema":1,"id":"x","processName":"p.exe","moduleName":"m.dll","gameAssemblySHA256":"` + hash + `","requiredFiles":[{"path":"../m.dll","bytes":1,"sha256":"` + hash + `"}]}`,
		"哈希长度不对":     `{"schema":1,"id":"x","processName":"p.exe","moduleName":"m.dll","gameAssemblySHA256":"` + hash + `","requiredFiles":[{"path":"m.dll","bytes":1,"sha256":"abc"}]}`,
		"schema 不认识": `{"schema":9,"id":"x","processName":"p.exe","moduleName":"m.dll","gameAssemblySHA256":"` + hash + `","requiredFiles":[{"path":"m.dll","bytes":1,"sha256":"` + hash + `"}]}`,
		"多余字段":       `{"schema":1,"id":"x","processName":"p.exe","moduleName":"m.dll","gameAssemblySHA256":"` + hash + `","requiredFiles":[{"path":"m.dll","bytes":1,"sha256":"` + hash + `"}],"whatever":1}`,
	}
	for name, payload := range cases {
		payload = strings.ReplaceAll(payload, "m.dll", module)
		if _, err := LoadProfile(strings.NewReader(payload)); err == nil {
			t.Errorf("%s：应该被拒绝", name)
		}
	}
}

func TestParseLibraryFolders(t *testing.T) {
	// 新版格式：libraryfolders/<n>/path
	current := `"libraryfolders"
{
	"0"
	{
		"path"		"/home/u/.local/share/Steam"
		"apps"		{ "228980" "1" }
	}
	"1"
	{
		"path"		"/mnt/games/SteamLibrary"
	}
}`
	doc, err := parseVDF([]byte(current))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	got := libraryPaths(doc)
	want := []string{"/home/u/.local/share/Steam", "/mnt/games/SteamLibrary"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("库目录不对: %v", got)
	}
	// 老版格式：libraryfolders/<n> 直接是路径字符串，且键序是乱的要能排好。
	legacy, err := parseVDF([]byte(`"LibraryFolders" { "1" "/b" "0" "/a" }`))
	if err != nil {
		t.Fatalf("解析老版失败: %v", err)
	}
	if got := libraryPaths(legacy); strings.Join(got, "|") != "/a|/b" {
		t.Fatalf("老版库目录不对: %v", got)
	}
	if _, err := parseVDF([]byte(`"libraryfolders" { "0" { "path" "/a"`)); err == nil {
		t.Fatal("截断的 VDF 应该报错")
	}
}

func TestCheckReportsEachVerdict(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "GameAssembly.dll", "reference-module")
	writeFile(t, root, "changed.bin", "something-else")

	profile := &Profile{
		Schema: 1, ID: "test", ProcessName: ProcessName, ModuleName: ModuleName,
		GameAssemblySHA256: hashOf("reference-module"),
		RequiredFiles: []ProfileFile{
			{Path: "GameAssembly.dll", SHA256: hashOf("reference-module")},
			{Path: "changed.bin", SHA256: hashOf("expected-content")},
			{Path: "missing.bin", SHA256: hashOf("whatever")},
		},
	}
	if err := profile.normalize(); err != nil {
		t.Fatalf("profile 无效: %v", err)
	}
	probe, err := Check(root, profile)
	if err != nil {
		t.Fatalf("核对失败: %v", err)
	}
	statuses := map[string]string{}
	for _, file := range probe.Files {
		statuses[file.Path] = file.Status
	}
	if statuses["GameAssembly.dll"] != StatusMatch {
		t.Errorf("关键文件应该一致，实际 %s", statuses["GameAssembly.dll"])
	}
	if statuses["changed.bin"] != StatusMismatch {
		t.Errorf("内容不同的文件应该判为不一致，实际 %s", statuses["changed.bin"])
	}
	if statuses["missing.bin"] != StatusMissing {
		t.Errorf("缺失的文件应该判为缺失，实际 %s", statuses["missing.bin"])
	}
	// 关键文件一致 = 契约可用；其余不一致只影响 Identical。
	if !probe.Supported() {
		t.Error("关键文件一致时 Supported 应该为真")
	}
	if probe.Identical() {
		t.Error("有文件不一致时 Identical 应该为假")
	}
	if len(probe.Divergent()) != 2 {
		t.Errorf("应该有 2 个不一致的文件，实际 %d", len(probe.Divergent()))
	}
}

func TestDiscoverDeduplicatesSymlinkedRoots(t *testing.T) {
	base := t.TempDir()
	steam := filepath.Join(base, "Steam")
	writeFile(t, steam, "steamapps/libraryfolders.vdf", `"libraryfolders" { "0" { "path" "`+steam+`" } }`)
	if err := os.MkdirAll(filepath.Join(steam, "steamapps", "common", InstallDir), 0o755); err != nil {
		t.Fatal(err)
	}
	// 真实环境里 ~/.steam/root 与 ~/.steam/steam 都是指向同一份安装的软链接；
	// 不去重就会把同一份客户端报成多份，让 import 误判成"有多份客户端"。
	link := filepath.Join(base, "link")
	if err := os.Symlink(steam, link); err != nil {
		t.Skipf("这个环境不支持软链接: %v", err)
	}
	result := discoverFrom([]string{steam, link})
	if len(result.SteamRoots) != 1 {
		t.Fatalf("Steam 根目录应该去重成 1 个，实际 %v", result.SteamRoots)
	}
	if len(result.Installs) != 1 {
		t.Fatalf("同一份客户端应该只报一次，实际 %d 次：%v", len(result.Installs), result.Installs)
	}
	if result.Installs[0].Root != canonical(filepath.Join(steam, "steamapps", "common", InstallDir)) {
		t.Fatalf("客户端路径应该是解析后的真实路径，实际 %s", result.Installs[0].Root)
	}
}

func TestDiscoverFindsProtonPrefix(t *testing.T) {
	base := t.TempDir()
	steam := filepath.Join(base, "Steam")
	client := filepath.Join(steam, "steamapps", "common", InstallDir)
	if err := os.MkdirAll(client, 0o755); err != nil {
		t.Fatal(err)
	}
	// 没有任何库文件时，Steam 根目录自己就是一个库。
	result := discoverFrom([]string{steam})
	if len(result.Installs) != 1 {
		t.Fatalf("应该找到 1 份客户端，实际 %d", len(result.Installs))
	}
	if result.Installs[0].ProtonPrefix != "" {
		t.Fatal("没有 compatdata 时不应该报告 Proton 前缀")
	}
	if err := os.MkdirAll(filepath.Join(steam, "steamapps", "compatdata", SteamAppID, "pfx"), 0o755); err != nil {
		t.Fatal(err)
	}
	result = discoverFrom([]string{steam})
	if result.Installs[0].ProtonPrefix == "" {
		t.Fatal("有 compatdata/<appid>/pfx 时应该报告 Proton 前缀")
	}
}

func TestCheckRejectsChangedCriticalFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "GameAssembly.dll", "patched-by-someone-else")
	profile := &Profile{
		Schema: 1, ID: "test", ProcessName: ProcessName, ModuleName: ModuleName,
		GameAssemblySHA256: hashOf("reference-module"),
		RequiredFiles:      []ProfileFile{{Path: "GameAssembly.dll", SHA256: hashOf("reference-module")}},
	}
	if err := profile.normalize(); err != nil {
		t.Fatalf("profile 无效: %v", err)
	}
	probe, _ := Check(root, profile)
	if probe.Supported() {
		t.Fatal("关键文件不一致时必须判定为不支持")
	}
}

func TestImportCopiesAndVerifies(t *testing.T) {
	source := t.TempDir()
	writeFile(t, source, "GameAssembly.dll", "reference-module")
	writeFile(t, source, "ShadowverseWB_Data/level0", "asset")
	writeFile(t, source, "ShadowverseWB_Data/Plugins/x86/audio.dll", "plugin")

	destination := filepath.Join(t.TempDir(), "work")
	plan, err := PlanImport(source, destination)
	if err != nil {
		t.Fatalf("计划失败: %v", err)
	}
	if plan.Files != 3 {
		t.Fatalf("应该统计到 3 个文件，实际 %d", plan.Files)
	}
	if err := RunImport(plan, nil); err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(destination, "ShadowverseWB_Data", "Plugins", "x86", "audio.dll"))
	if err != nil {
		t.Fatalf("副本缺少文件: %v", err)
	}
	if string(got) != "plugin" {
		t.Fatalf("副本内容不对: %q", got)
	}
	// 源目录必须一个字节都没动。
	if raw, _ := os.ReadFile(filepath.Join(source, "GameAssembly.dll")); string(raw) != "reference-module" {
		t.Fatal("源目录被改动了")
	}
	profile := &Profile{
		Schema: 1, ID: "test", ProcessName: ProcessName, ModuleName: ModuleName,
		GameAssemblySHA256: hashOf("reference-module"),
		RequiredFiles:      []ProfileFile{{Path: "GameAssembly.dll", SHA256: hashOf("reference-module")}},
	}
	if err := profile.normalize(); err != nil {
		t.Fatalf("profile 无效: %v", err)
	}
	probe, err := VerifyImport(destination, profile)
	if err != nil {
		t.Fatalf("核对副本失败: %v", err)
	}
	if !probe.Supported() || !probe.Identical() {
		t.Fatal("副本应该与 profile 一致")
	}
	// 重复导入是允许的。
	if err := RunImport(plan, nil); err != nil {
		t.Fatalf("重复导入应该安全: %v", err)
	}
}

func TestImportRefusesNestedPaths(t *testing.T) {
	source := t.TempDir()
	nested := filepath.Join(source, "work")
	if _, err := PlanImport(source, nested); err == nil {
		t.Fatal("目标在源内部时应该拒绝")
	}
	if _, err := PlanImport(source, source); err == nil {
		t.Fatal("源与目标相同时应该拒绝")
	}
	outer := t.TempDir()
	inner := filepath.Join(outer, "client")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanImport(inner, outer); err == nil {
		t.Fatal("源在目标内部时应该拒绝")
	}
}

func TestFreeSpaceWalkUpToExistingAncestor(t *testing.T) {
	// 目标目录还不存在是常态，要能从它往上找到已存在的祖先。
	missing := filepath.Join(t.TempDir(), "a", "b", "c")
	available, err := FreeSpace(missing)
	if err != nil {
		t.Fatalf("应该能沿父目录找到文件系统: %v", err)
	}
	if available <= 0 {
		t.Fatalf("可用空间应该为正数，实际 %d", available)
	}
}

func TestRunImportRefusesWhenDestinationIsTooSmall(t *testing.T) {
	source := t.TempDir()
	writeFile(t, source, "GameAssembly.dll", "x")
	plan := &ImportPlan{Source: source, Destination: filepath.Join(t.TempDir(), "work"), Files: 1}
	// 直接把需求抬到 1 EiB，任何文件系统都装不下；这里验证的是检查本身会拦住，
	// 而不是去猜某个具体磁盘的大小。
	plan.Bytes = 1 << 60
	if err := RunImport(plan, nil); err == nil {
		t.Fatal("空间不足时应该拒绝导入")
	}
}

func writeFile(t *testing.T, root, relative, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hashOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
