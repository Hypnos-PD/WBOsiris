package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"wbo/internal/nativebroker"
	"wbo/internal/nativeclient"
	"wbo/internal/nativeprofile"
	"wbo/internal/nativeproto"
)

// native 这一组子命令是"游戏导入器"的前半段：发现本地客户端、核对它是不是我们
// 支持的那个构建、把它复制成一份属于工作目录的副本。它不修改 Steam 的安装目录，
// 也不碰补丁——补丁是下一步，而且必须在副本上做。

func runNative(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "用法: wbo native <probe|import|broker|launch> [选项]")
		return 2
	}
	switch args[0] {
	case "probe":
		return runNativeProbe(args[1:])
	case "import":
		return runNativeImport(args[1:])
	case "broker":
		return runNativeBroker(args[1:])
	case "launch":
		return runNativeLaunch(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "用法: wbo native <probe|import|broker|launch> [选项]")
		return 2
	}
}

func runNativeProbe(args []string) int {
	fs := flag.NewFlagSet("native probe", flag.ContinueOnError)
	clientDir := fs.String("client-dir", "", "直接指定客户端目录；省略时按 Steam 库查找")
	profileID := fs.String("profile", "", "profile id；省略时使用内置的那一份")
	asJSON := fs.Bool("json", false, "以 JSON 输出")
	if fs.Parse(args) != nil {
		return 2
	}
	profile, err := nativeclient.ProfileByID(*profileID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	var installs []nativeclient.Install
	var discovery nativeclient.Discovery
	if *clientDir != "" {
		install, err := nativeclient.Inspect(*clientDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		installs = []nativeclient.Install{install}
	} else {
		discovery = nativeclient.Discover()
		installs = discovery.Installs
	}
	if *asJSON {
		return reportProbeJSON(profile, discovery, installs)
	}
	return reportProbeText(profile, discovery, installs)
}

func reportProbeJSON(profile *nativeclient.Profile, discovery nativeclient.Discovery, installs []nativeclient.Install) int {
	type entry struct {
		Install nativeclient.Install `json:"install"`
		Probe   *nativeclient.Probe  `json:"probe,omitempty"`
		Error   string               `json:"error,omitempty"`
	}
	payload := struct {
		Profile     string   `json:"profile"`
		GameVersion string   `json:"gameVersion"`
		SteamRoots  []string `json:"steamRoots"`
		Libraries   []string `json:"libraries"`
		Installs    []entry  `json:"installs"`
	}{Profile: profile.ID, GameVersion: profile.GameVersion, SteamRoots: discovery.SteamRoots, Libraries: discovery.Libraries}
	supported := 0
	for _, install := range installs {
		item := entry{Install: install}
		probe, err := nativeclient.Check(install.Root, profile)
		if err != nil {
			item.Error = err.Error()
		} else {
			item.Probe = probe
			if probe.Supported() {
				supported++
			}
		}
		payload.Installs = append(payload.Installs, item)
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(string(encoded))
	if supported == 0 {
		return 1
	}
	return 0
}

func reportProbeText(profile *nativeclient.Profile, discovery nativeclient.Discovery, installs []nativeclient.Install) int {
	fmt.Printf("profile  %s（客户端版本 %s）\n", profile.ID, profile.GameVersion)
	if len(discovery.SteamRoots) > 0 {
		fmt.Printf("Steam    %s\n", strings.Join(discovery.SteamRoots, ", "))
	}
	if len(installs) == 0 {
		fmt.Println("没有找到已安装的客户端。用 --client-dir 指定目录。")
		return 1
	}
	supported := 0
	for _, install := range installs {
		fmt.Printf("\n客户端   %s\n", install.Root)
		if install.ProtonPrefix != "" {
			fmt.Printf("运行方式 Proton（前缀 %s）\n", install.ProtonPrefix)
		} else {
			fmt.Println("运行方式 Windows 原生")
		}
		probe, err := nativeclient.Check(install.Root, profile)
		if err != nil {
			fmt.Printf("核对失败 %v\n", err)
			continue
		}
		critical, _ := probe.Critical()
		fmt.Printf("关键文件 %s: %s\n", critical.Path, verdictText(critical))
		if probe.Supported() {
			supported++
		}
		if probe.Identical() {
			fmt.Printf("全部 %d 个登记文件一致。\n", len(probe.Files))
			continue
		}
		for _, file := range probe.Divergent() {
			marker := "警告"
			if file.Critical {
				marker = "错误"
			}
			fmt.Printf("  [%s] %s: %s\n", marker, file.Path, verdictText(file))
		}
	}
	fmt.Println()
	if supported == 0 {
		fmt.Println("结论 这份客户端不是 profile 描述的那个构建，不能用。")
		return 1
	}
	fmt.Println("结论 可以用。")
	return 0
}

func verdictText(file nativeclient.FileVerdict) string {
	switch file.Status {
	case nativeclient.StatusMatch:
		return "一致"
	case nativeclient.StatusMissing:
		return "缺失"
	case nativeclient.StatusUnreadable:
		return "无法读取：" + file.Reason
	default:
		if file.Reason != "" {
			return "不一致（" + file.Reason + "）"
		}
		return "不一致"
	}
}

func runNativeImport(args []string) int {
	fs := flag.NewFlagSet("native import", flag.ContinueOnError)
	destination := fs.String("to", "", "工作目录（必填）")
	clientDir := fs.String("from", "", "客户端目录；省略时按 Steam 库查找")
	profileID := fs.String("profile", "", "profile id；省略时使用内置的那一份")
	dryRun := fs.Bool("dry-run", false, "只打印计划，不复制")
	force := fs.Bool("force", false, "即使关键文件不一致也继续")
	if fs.Parse(args) != nil {
		return 2
	}
	if *destination == "" {
		fmt.Fprintln(os.Stderr, "缺少 --to")
		return 2
	}
	profile, err := nativeclient.ProfileByID(*profileID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	source := *clientDir
	if source == "" {
		installs := nativeclient.Discover().Installs
		switch len(installs) {
		case 0:
			fmt.Fprintln(os.Stderr, "没有找到已安装的客户端，用 --from 指定目录。")
			return 1
		case 1:
			source = installs[0].Root
		default:
			fmt.Fprintln(os.Stderr, "找到多份客户端，用 --from 指定其中之一：")
			for _, install := range installs {
				fmt.Fprintf(os.Stderr, "  %s\n", install.Root)
			}
			return 1
		}
	}
	probe, err := nativeclient.Check(source, profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !probe.Supported() {
		critical, _ := probe.Critical()
		fmt.Fprintf(os.Stderr, "源客户端的 %s 与 profile 不一致：%s\n", critical.Path, verdictText(critical))
		if !*force {
			fmt.Fprintln(os.Stderr, "拒绝导入。确认要强行继续就加 --force。")
			return 1
		}
		fmt.Fprintln(os.Stderr, "--force：继续导入。")
	}
	plan, err := nativeclient.PlanImport(source, *destination)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("导入计划  %s → %s\n", plan.Source, plan.Destination)
	fmt.Printf("文件 %d 个，合计 %.1f MiB\n", plan.Files, float64(plan.Bytes)/(1<<20))
	if *dryRun {
		return 0
	}
	var lastReported int64
	err = nativeclient.RunImport(plan, func(done, total int64, path string) {
		// 每 256 MiB 报一次，避免把日志刷满。
		if done-lastReported >= 256<<20 {
			lastReported = done
			fmt.Printf("  已复制 %.0f%%\n", float64(done)/float64(total)*100)
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// 复制完必须重新核对目的地：这是副本可用的唯一证据。
	verify, err := nativeclient.VerifyImport(plan.Destination, profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !verify.Supported() {
		fmt.Fprintln(os.Stderr, "副本核对失败，导入结果不可信。")
		return 1
	}
	fmt.Printf("导入完成  %s\n", plan.Destination)
	if !verify.Identical() {
		fmt.Printf("注意：目的地有 %d 个文件与 profile 不一致（关键文件已核对通过）。\n", len(verify.Divergent()))
	}
	return 0
}

// runNativeBroker 起本地的"服务端"，让被改写的客户端有地方可去。
//
// 现在所有路由都按未实现处理，它唯一的作用是把真实请求完整录下来——客户端连过
// 哪些地址、请求体长什么样、调用顺序如何，跑一次就知道了。
func runNativeBroker(args []string) int {
	fs := flag.NewFlagSet("native broker", flag.ContinueOnError)
	listen := fs.String("listen", "127.0.0.1:50172", "监听地址，要和客户端改写目标一致")
	captureDir := fs.String("capture", "", "把请求录制到这个目录")
	seconds := fs.Int("seconds", 0, "运行多少秒后退出；0 表示一直运行到 Ctrl-C")
	sessionKey := fs.String("session-key", "", "会话私钥（32 字节）；给了它才会尝试解开加密的请求体")
	uuid := fs.String("uuid", "", "客户端凭据 uuid（解报文需要）")
	authKey := fs.String("auth-key", "", "客户端凭据 auth key（base64，解报文需要）")
	commonHeader := fs.String("common-header", defaultCommonHeaderHex,
		"52 字节常量（hex）；只有第 32..52 字节参与密钥派生，换版本要更新")
	if fs.Parse(args) != nil {
		return 2
	}
	options := nativebroker.Options{Listen: *listen, CaptureDir: *captureDir}
	// 三个都给了才启用解码；缺任何一个就只录制密文（仍然有用）。
	if *sessionKey != "" || *uuid != "" || *authKey != "" {
		server, auth, err := buildDecoder(*sessionKey, *uuid, *authKey, *commonHeader)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		options.Decoder = server
		options.Auth = auth
		fmt.Println("已启用请求解密（需要客户端已换成对应的公钥）")
		// 能解密才谈得上应答：引导阶段的响应取自夹具，它们不依赖规则引擎，
		// 作用只是让客户端走到主界面。
		fixtures, err := nativeproto.LoadFixtures()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		// 档案层要放在夹具前面：/Load/index 虽然夹具里也有，但那一份的收藏是空的，
		// 客户端拿空收藏会退回标题界面。
		profile, err := nativeprofile.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		options.Handler = nativebroker.Chain(
			nativebroker.NewProfileHandler(profile, fixtures, log.Default()),
			nativebroker.NewFixtureHandler(fixtures, log.Default()),
		)
		fmt.Printf("已启用引导响应：覆盖 %d 条路由\n", len(fixtures.Routes()))
		fmt.Printf("已启用档案层：收藏 %d 个 base 卡（契约 %s）\n",
			len(profile.OwnedBaseCardIDs()), profile.SourceSHA256()[:12])
		// 如实交代这批响应的来源——它们是本地重建的，不是抓到的官方响应。
		if provenance := fixtures.Provenance(); provenance != "" {
			fmt.Printf("  来源：%s\n", provenance)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *seconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(*seconds)*time.Second)
		defer cancel()
	}
	if err := nativebroker.Serve(ctx, options); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// defaultCommonHeaderHex 是 1.9.1.18238 的 52 字节常量。
//
// 换版本要更新：它的第 32..52 字节参与密钥派生与认证。前 32 字节是"对端公钥"，
// 被换掉之后这里记的仍然只是原值——服务端只用后 20 字节。
const defaultCommonHeaderHex = "5EBB1B7E66F37753855EAEE1D9F9EFFEE740591A306A1BBC034A9569A778A562" +
	"6854C854E96E762E4F18C9F7370608BE437ECBBA"

func buildDecoder(sessionKeyPath, uuidText, authKeyBase64, commonHeaderText string) (*nativeproto.Server, nativeproto.Auth, error) {
	var server *nativeproto.Server
	var auth nativeproto.Auth
	if sessionKeyPath == "" || uuidText == "" || authKeyBase64 == "" {
		return nil, auth, fmt.Errorf("启用解密需要同时提供 --session-key、--uuid、--auth-key")
	}
	privateKey, err := os.ReadFile(sessionKeyPath)
	if err != nil {
		return nil, auth, err
	}
	header, err := hex.DecodeString(strings.TrimSpace(commonHeaderText))
	if err != nil {
		return nil, auth, fmt.Errorf("--common-header 不是合法 hex: %w", err)
	}
	server, err = nativeproto.NewServer(privateKey, header)
	if err != nil {
		return nil, auth, err
	}
	auth.UUID, err = hex.DecodeString(strings.ReplaceAll(uuidText, "-", ""))
	if err != nil {
		return nil, auth, fmt.Errorf("--uuid 不是合法 hex: %w", err)
	}
	auth.AuthKey, err = base64.StdEncoding.DecodeString(authKeyBase64)
	if err != nil {
		return nil, auth, fmt.Errorf("--auth-key 不是合法 base64: %w", err)
	}
	return server, auth, nil
}
