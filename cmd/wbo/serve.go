package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"wbo/internal/server"
)

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	root := fs.String("source-root", ".", "源文件根目录")
	// 默认端口刻意避开 8080/8000/3000 这类常见值，免得和本地其它服务抢端口；
	// 数字取自 WBO（W=23、B=2、O=15）。
	listen := fs.String("listen", ":23215", "监听地址")
	illustrations := fs.String("illustration-root", "", "主界面插图素材目录（默认自动找 ../WBArts/data；传 --illustration-root=none 可禁用）")
	// 托管在线上时要求"进大厅先登录"，校验复用 WBArts 的账号（/api/auth/me）。
	// 本地开发默认不校验，直接传 --auth-verify-url 即可打开。
	authVerify := fs.String("auth-verify-url", "", "大厅登录校验地址，例如 https://sva.hypd.asia/api/auth/me；留空表示本地模式不要求登录")
	version := fs.String("version", "", "写入 /api/health 的版本标识（部署脚本会填提交号）")
	if fs.Parse(args) != nil {
		return 2
	}
	base, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	illustrationRoot := ""
	switch *illustrations {
	case "none":
		illustrationRoot = ""
	case "":
		illustrationRoot = server.DefaultIllustrationRoot(base)
	default:
		illustrationRoot = *illustrations
	}
	h, err := server.NewWithOptions(base, []string{filepath.Join(base, "cards"), filepath.Join(base, "tests")}, illustrationRoot, *authVerify)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *version == "" {
		// 没显式传就读服务目录里的 VERSION（部署脚本会写）。
		if data, readErr := os.ReadFile(filepath.Join(base, "VERSION")); readErr == nil {
			*version = string(data)
		}
	}
	h.SetVersion(*version)
	fmt.Fprintln(os.Stderr, "WBO simulator listening on", *listen)
	if err := http.ListenAndServe(*listen, h.Handler()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
