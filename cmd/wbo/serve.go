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
	illustrations := fs.String("illustration-root", "", "主界面插图素材目录（默认自动找 ../WBArts/data；传空目录名 --illustration-root=none 可禁用）")
	if fs.Parse(args) != nil {
		return 2
	}
	base, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var newServer func(string, []string) (*server.Server, error)
	if *illustrations == "none" {
		newServer = func(root string, paths []string) (*server.Server, error) {
			return server.NewWithIllustrations(root, paths, "")
		}
	} else if *illustrations != "" {
		newServer = func(root string, paths []string) (*server.Server, error) {
			return server.NewWithIllustrations(root, paths, *illustrations)
		}
	} else {
		newServer = server.New
	}
	h, err := newServer(base, []string{filepath.Join(base, "cards"), filepath.Join(base, "tests")})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Fprintln(os.Stderr, "WBO simulator listening on", *listen)
	if err := http.ListenAndServe(*listen, h.Handler()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
