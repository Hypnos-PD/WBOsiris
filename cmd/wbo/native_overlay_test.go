package main

import (
	"os"
	"path/filepath"
	"testing"
)

// 注入必须在替身里发生：真目录（这里扮演 Steam 的安装目录）一个字节都不能多出来。
// 这条测试是补一个真实事故——直接把源文件 bind 到不存在的目标路径时，bwrap 会在
// 宿主机上把目标建出来，等于往游戏目录里写了文件。
func TestInjectFileDoesNotTouchTheRealDirectory(t *testing.T) {
	clientDir := filepath.Join(t.TempDir(), "ShadowverseWB")
	if err := os.MkdirAll(filepath.Join(clientDir, "ShadowverseWB_Data", "PreinResource"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "assetbundle.Chs.manifest")
	if err := os.WriteFile(source, []byte("manifest"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	var overlays overlays
	relative := filepath.Join("ShadowverseWB_Data", "PreinResource", "manifests", "assetbundle.Chs.manifest")
	if err := injectFile(state, clientDir, relative, source, &overlays); err != nil {
		t.Fatalf("注入失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(clientDir, relative)); !os.IsNotExist(err) {
		t.Fatalf("真目录里不该出现被注入的文件")
	}
	want := filepath.Join(clientDir, "ShadowverseWB_Data", "PreinResource")
	if len(overlays.Bindings) == 0 || overlays.Bindings[0].Target != want {
		t.Fatalf("替身该挂在最近的已存在目录 %s 上，实际 %v", want, overlays.Bindings)
	}
	injected := filepath.Join(overlays.Bindings[0].Source, "manifests", "assetbundle.Chs.manifest")
	if content, err := os.ReadFile(injected); err != nil {
		t.Fatalf("替身里没有注入的文件: %v", err)
	} else if string(content) != "manifest" {
		t.Errorf("替身里的内容不对: %q", content)
	}
}

func TestPreparePluginOverlayLinksEverythingAndProvidesOriginal(t *testing.T) {
	client := t.TempDir()
	real := filepath.Join(client, filepath.FromSlash(pluginDirectory))
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileBytes(t, filepath.Join(client, "ShadowverseWB.exe"), "game-exe")
	// 真实目录里：插件本名 + 一堆别的插件。
	writeFileBytes(t, filepath.Join(real, pluginName), "original-plugin")
	writeFileBytes(t, filepath.Join(real, "libnative.dll"), "other-plugin")

	state := t.TempDir()
	shim := filepath.Join(t.TempDir(), "yaha_shim.dll")
	writeFileBytes(t, shim, "our-shim")

	result, err := prepareOverlays(state, client, shim)
	_ = result
	if err != nil {
		t.Fatalf("搭替身失败: %v", err)
	}
	// 插件本名必须是我们代理的硬链接。
	linked, err := os.Stat(filepath.Join(state, "plugins", pluginName))
	if err != nil {
		t.Fatalf("替身里没有插件: %v", err)
	}
	wanted, err := os.Stat(shim)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(linked, wanted) {
		t.Fatal("插件本名不是代理")
	}
	// 原件必须按加载器期待的名字存在——代理的 44 个转发全靠它。
	// 它是硬链接，所以用 SameFile 判断是不是同一个文件。
	originalCopy, err := os.Stat(filepath.Join(state, "plugins", originalName))
	if err != nil {
		t.Fatalf("没有提供原件 %s: %v", originalName, err)
	}
	originalReal, err := os.Stat(filepath.Join(real, pluginName))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(originalCopy, originalReal) {
		t.Fatal("插件目录里的原件不是同一个文件")
	}
	// 别的插件必须还在，否则游戏会因为缺插件起不来。
	if _, err := os.Stat(filepath.Join(state, "plugins", "libnative.dll")); err != nil {
		t.Fatalf("其余插件没有保留: %v", err)
	}
	// 关键：原件也必须出现在游戏根目录。PE 转发由加载器按标准顺序解析，第一步
	// 就是 exe 所在目录，而不是转发模块自己所在的插件目录。
	if _, err := os.Stat(filepath.Join(state, "client", originalName)); err != nil {
		t.Fatalf("游戏根目录里没有原件，转发会解析失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state, "client", "ShadowverseWB.exe")); err != nil {
		t.Fatalf("根目录替身没有保留 exe: %v", err)
	}
	// 关键：替身里绝不能有自引用的软链接，否则覆盖上去就成死循环。
	for _, name := range []string{"ShadowverseWB.exe", originalName} {
		info, err := os.Lstat(filepath.Join(state, "client", name))
		if err != nil {
			t.Fatalf("替身缺少 %s: %v", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%s 是软链接，覆盖后会自引用", name)
		}
	}
	// 游戏目录必须一个字节都没动。
	if raw, _ := os.ReadFile(filepath.Join(real, pluginName)); string(raw) != "original-plugin" {
		t.Fatal("游戏目录被改动了")
	}
	// 目录的挂载点必须是"游戏看到的那条路径"。写成替身自己的宿主机路径的话，
	// 游戏看到的仍然是替身里的空目录——Unity 会直接报找不到 ShadowverseWB_Data。
	var dataBinding *overlayBinding
	for index := range result.Bindings {
		if result.Bindings[index].Target == filepath.Join(client, "ShadowverseWB_Data") {
			dataBinding = &result.Bindings[index]
		}
	}
	if dataBinding == nil {
		t.Fatalf("没有把 ShadowverseWB_Data 挂回游戏路径：%+v", result.Bindings)
	}
	// 源和目标必须是同一个字符串：源在宿主机视图上解析成真实目录，目标在沙箱
	// 视图上解析成替身留的空目录，于是空目录被填成真内容。
	if dataBinding.Source != dataBinding.Target {
		t.Fatalf("目录的挂载源与目标应该相同，实际 %s → %s", dataBinding.Source, dataBinding.Target)
	}
	// 重复准备要能覆盖旧的替身（同一个会话重跑）。
	if _, err := prepareOverlays(state, client, shim); err != nil {
		t.Fatalf("重复准备应该安全: %v", err)
	}
}

func writeFileBytes(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
