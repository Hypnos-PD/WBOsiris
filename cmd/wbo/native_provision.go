package main

import (
	"flag"
	"fmt"
	"os"

	"wbo/internal/nativeclient"
)

// native provision：把一份本地资源铺进客户端，让它能离线跑到主界面。
//
// 这一步是**写操作**，写的是客户端的资源目录。正式流程应该在 `native import`
// 出来的副本上做；直接写 Steam 的安装目录要显式加 --write-steam，因为那会破坏
// "Steam 目录只读"这条规矩（本机做实验时才这么干）。
//
// 之所以需要它：客户端把资源分成两份——PreinResource/AssetBundle 是真正加载的
// 那一份，Persistent/dat 是下载缓存。只给 dat（比如直接用 wbunpacker 的产物）会
// 让客户端在读 Prein 路径时拿到 DirectoryNotFoundException，界面上的表现是
// "下载数据时发生错误"。清单也要两份，而且 dat 那份的文件名是客户端自己算的。
func runNativeProvision(args []string) int {
	fs := flag.NewFlagSet("native provision", flag.ContinueOnError)
	clientFlag := fs.String("client-dir", "", "客户端目录；省略时按 Steam 库自动发现并核对")
	sourceFlag := fs.String("source", "", "资源库根目录（wbunpacker 的 data_dir，里面有 manifests/raw 与 blobs/raw）")
	langFlag := fs.String("lang", "Chs", "语言变体：Chs/Cht/Eng/Jpn/Kor")
	hnameFlag := fs.String("manifest-hname", "", "Persistent/dat 下清单 blob 的名字；省略时从 Player.log 里学")
	logFlag := fs.String("player-log", "", "Player.log 的路径；省略时按 Proton 前缀推导")
	dryRun := fs.Bool("dry-run", false, "只报告计划，不写任何文件")
	writeSteam := fs.Bool("write-steam", false, "明知故犯：往 Steam 的安装目录里写（本机实验用）")
	if fs.Parse(args) != nil {
		return 2
	}
	if *sourceFlag == "" {
		fmt.Fprintln(os.Stderr, "需要 --source 指定资源库根目录")
		return 2
	}

	profile, err := nativeclient.ProfileByID("")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	install, err := findVerifiedInstall(*clientFlag, profile, func(format string, args ...any) {
		fmt.Printf(format+"\n", args...)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if nativeclient.IsSteamManaged(install) && !*writeSteam && !*dryRun {
		fmt.Fprintf(os.Stderr, "这份客户端在 Steam 的安装目录里：%s\n"+
			"正式流程应该先 `wbo native import` 出一份副本，再对副本做 provision。\n"+
			"确实要写 Steam 目录就加 --write-steam（本机实验用；之后可以用 Steam 的"+
			"「验证游戏文件完整性」恢复）。\n", install.Root)
		return 1
	}

	report, err := nativeclient.Provision(nativeclient.ProvisionOptions{
		ClientDir:     install.Root,
		Source:        nativeclient.ResourceSource{Root: *sourceFlag},
		Lang:          *langFlag,
		ManifestHname: *hnameFlag,
		PlayerLog:     *logFlag,
		DryRun:        *dryRun,
	}, func(format string, args ...any) { fmt.Printf(format+"\n", args...) })
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("清单（PreinResource/manifests）：%s\n", report.ManifestCopy)
	fmt.Printf("清单（Persistent/dat 缓存）：%s\n", report.ManifestBlob)
	fmt.Printf("资源：%d 个，%.2f GiB（跳过已存在的 %d 个）\n",
		report.Bundles, float64(report.Bytes)/(1<<30), report.Skipped)
	if *dryRun {
		fmt.Println("（--dry-run：什么都没写）")
	}
	return 0
}
