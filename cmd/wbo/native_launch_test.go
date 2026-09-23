package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildLaunchCommandWithoutBroker(t *testing.T) {
	line := buildLaunchCommand(launchPlan{
		Bwrap:     "/usr/bin/bwrap",
		ClientDir: "/games/ShadowverseWB",
		Overlays: overlays{Bindings: []overlayBinding{
			{Source: "/tmp/session/client", Target: "/games/ShadowverseWB"},
			{Source: "/tmp/session/client/Data", Target: "/games/ShadowverseWB/Data"},
			{Source: "/tmp/session/plugins", Target: "/games/ShadowverseWB/" + pluginDirectory},
		}},
		Command: []string{"/steam/run", "waitforexitandrun", "/games/ShadowverseWB/ShadowverseWB.exe"},
	})
	joined := strings.Join(line, " ")
	// 挂载顺序有讲究：根目录替身先挂，真实目录挂回它下面，插件目录最后挂。
	want := "--bind /tmp/session/client /games/ShadowverseWB " +
		"--bind /tmp/session/client/Data /games/ShadowverseWB/Data " +
		"--bind /tmp/session/plugins /games/ShadowverseWB/" + pluginDirectory
	if !strings.Contains(joined, want) {
		t.Fatalf("挂载顺序或目标不对：%s", joined)
	}
	if !strings.Contains(joined, "--dev-bind / /") {
		t.Fatalf("没有保留完整文件系统视图：%s", joined)
	}
	if strings.Contains(joined, "native broker") {
		t.Fatalf("没有要求 broker 时不应该启动它：%s", joined)
	}
	// 配置走环境变量（由调用方设置 YAHA_SHIM_CONFIG），不能在游戏目录里创建文件：
	// bwrap 的 --file 会在底层文件系统上真的建出那个文件，等于改动 Steam 的安装目录。
	if strings.Contains(joined, "--file") {
		t.Fatalf("不该用 --file 往游戏目录里写东西：%s", joined)
	}
	if line[len(line)-1] != "/games/ShadowverseWB/ShadowverseWB.exe" {
		t.Fatalf("命令行没有原样传到末尾：%s", joined)
	}
}

func TestBuildLaunchCommandWithBroker(t *testing.T) {
	// broker 必须在沙箱之外：bwrap 要等命名空间里所有进程结束才退出，把常驻的
	// broker 放进去会让 launcher 永远不返回。所以命令行里不该出现它。
	line := buildLaunchCommand(launchPlan{
		Bwrap:     "/usr/bin/bwrap",
		ClientDir: "/games/ShadowverseWB",
		Overlays: overlays{Bindings: []overlayBinding{
			{Source: "/tmp/session/client", Target: "/games/ShadowverseWB"},
			{Source: "/tmp/session/client/Data", Target: "/games/ShadowverseWB/Data"},
			{Source: "/tmp/session/plugins", Target: "/games/ShadowverseWB/" + pluginDirectory},
		}},
		Command: []string{"/steam/run", "/games/ShadowverseWB/ShadowverseWB.exe"},
	})
	joined := strings.Join(line, " ")
	if strings.Contains(joined, "native broker") || strings.Contains(joined, "/bin/sh") {
		t.Fatalf("命令里不该有 broker 或 shell 包装：%s", joined)
	}
	if line[len(line)-1] != "/games/ShadowverseWB/ShadowverseWB.exe" {
		t.Fatalf("游戏命令位置不对：%s", joined)
	}
	if line[len(line)-2] != "/steam/run" {
		t.Fatalf("命令参数没有原样保留：%s", joined)
	}
}

func TestFindClientDir(t *testing.T) {
	root := t.TempDir()
	client := filepath.Join(root, "ShadowverseWB")
	plugin := filepath.Join(client, filepath.FromSlash(pluginRelative))
	if err := os.MkdirAll(filepath.Dir(plugin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plugin, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(client, "ShadowverseWB.exe")
	command := []string{"/steam/run", "--verb=waitforexitandrun", "/proton", "waitforexitandrun", exe}
	found, err := findClientDir(command)
	if err != nil {
		t.Fatalf("应该认出客户端目录：%v", err)
	}
	if found != client {
		t.Fatalf("目录不对：%s", found)
	}
	// 命令里没有 exe 时要明确报错，而不是猜。
	if _, err := findClientDir([]string{"/steam/run", "/proton"}); err == nil {
		t.Fatal("认不出目录时应该报错")
	}
	// 有 exe 但没有插件（比如指向了别的目录）也要报错。
	if _, err := findClientDir([]string{filepath.Join(root, "ShadowverseWB.exe")}); err == nil {
		t.Fatal("插件不存在时应该报错")
	}
}
