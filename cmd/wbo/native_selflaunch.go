package main

import (
	"fmt"
	"path/filepath"

	"wbo/internal/nativeclient"
)

// resolveTarget 决定这次启动跑哪一份客户端、以及（自己启动时）用哪个 Proton。
//
// 两条路：给了 "-- 命令" 就用调用方给的命令行，环境变量原样继承（Steam 启动选项
// 那条老路径，保留下来给调试用）；没给就自己把命令拼出来——正常路径，用户不需要
// 知道 Steam 启动选项是什么东西，也不需要为了用 WBO 去改它。
func resolveTarget(rest []string, clientFlag, protonFlag, runtimeFlag string,
	logf func(string, ...any)) (string, *nativeclient.ProtonTool, error) {
	if len(rest) > 0 {
		clientDir, err := findClientDir(rest)
		if err != nil {
			return "", nil, err
		}
		return clientDir, explicitTool(clientDir, protonFlag, runtimeFlag), nil
	}
	profile, err := nativeclient.ProfileByID("")
	if err != nil {
		return "", nil, err
	}
	install, err := findVerifiedInstall(clientFlag, profile, logf)
	if err != nil {
		return "", nil, err
	}
	if install.ProtonPrefix == "" {
		return "", nil, fmt.Errorf("这份客户端不是 Proton 运行的（%s 下没有 prefix），"+
			"自己启动它的做法还没实现；用 -- 指定一条现成的命令行", install.Root)
	}
	if protonFlag != "" || runtimeFlag != "" {
		tool, err := nativeclient.ProtonAt(install.SteamRoot, install.Library, profile.SteamAppID,
			install.ProtonPrefix, protonFlag, runtimeFlag)
		if err != nil {
			return "", nil, err
		}
		return install.Root, &tool, nil
	}
	tool, err := nativeclient.ResolveProton(install.SteamRoot, install.Library, profile.SteamAppID, install.ProtonPrefix)
	if err != nil {
		return "", nil, err
	}
	if tool.EntryPoint == "" {
		logf("警告：没有找到 Steam Linux Runtime 的入口，将在宿主机上直接跑 Proton（可能缺运行库）")
	}
	return install.Root, &tool, nil
}

// explicitTool 处理"命令由调用方提供、但还想让 WBO 补上 Proton 环境"这种组合。
// 没给 --proton / --runtime 时返回 nil，表示环境原样继承。
func explicitTool(clientDir, protonFlag, runtimeFlag string) *nativeclient.ProtonTool {
	if protonFlag == "" && runtimeFlag == "" {
		return nil
	}
	install, err := nativeclient.Inspect(clientDir)
	if err != nil {
		return nil
	}
	tool, err := nativeclient.ProtonAt(install.SteamRoot, install.Library, nativeclient.SteamAppID,
		install.ProtonPrefix, protonFlag, runtimeFlag)
	if err != nil {
		return nil
	}
	return &tool
}

// findVerifiedInstall 找到一份**核对通过**的客户端。
//
// 核对不是可选项：线协议契约、DTO 槽位、内存偏移全部来自某个具体构建的
// GameAssembly.dll，拿别的构建去连只会得到一堆看不懂的失败。所以这里返回的每一份
// 客户端都过了一遍 SHA-256，报告出来的东西也来自那次核对。
func findVerifiedInstall(clientFlag string, profile *nativeclient.Profile,
	logf func(string, ...any)) (nativeclient.Install, error) {
	if clientFlag != "" {
		install, err := nativeclient.Inspect(clientFlag)
		if err != nil {
			return install, err
		}
		if err := verifyInstall(install, profile, logf); err != nil {
			return install, err
		}
		return install, nil
	}
	discovery := nativeclient.Discover()
	if len(discovery.Installs) == 0 {
		return nativeclient.Install{}, fmt.Errorf("没有在 Steam 库里找到 %s；用 --client-dir 指定目录", nativeclient.InstallDir)
	}
	var rejected []string
	for _, install := range discovery.Installs {
		if err := verifyInstall(install, profile, logf); err != nil {
			rejected = append(rejected, fmt.Sprintf("%s: %v", install.Root, err))
			continue
		}
		return install, nil
	}
	return nativeclient.Install{}, fmt.Errorf("找到的客户端都不匹配 profile %s：\n  %s", profile.ID,
		joinLines(rejected))
}

// verifyInstall 核对一份客户端，把关键文件的结论写进日志。
func verifyInstall(install nativeclient.Install, profile *nativeclient.Profile,
	logf func(string, ...any)) error {
	probe, err := nativeclient.Check(install.Root, profile)
	if err != nil {
		return err
	}
	critical, _ := probe.Critical()
	if !probe.Supported() {
		return fmt.Errorf("关键文件 %s 与 profile 不一致（%s），这份客户端不是 %s 那个构建",
			critical.Path, verdictText(critical), profile.ID)
	}
	logf("核对通过：%s 与 profile %s 一致（%d 个登记文件）", critical.Path, profile.ID, len(probe.Files))
	if divergent := probe.Divergent(); len(divergent) > 0 {
		names := make([]string, 0, len(divergent))
		for _, file := range divergent {
			names = append(names, filepath.ToSlash(file.Path))
		}
		logf("注意：另有 %d 个登记文件与提取契约时不同（不影响契约有效性）：%s",
			len(divergent), joinLines(names))
	}
	return nil
}

func joinLines(values []string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += "; "
		}
		result += value
	}
	return result
}
