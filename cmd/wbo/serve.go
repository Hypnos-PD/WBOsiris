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
	if fs.Parse(args) != nil {
		return 2
	}
	base, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	h, err := server.New(base, []string{filepath.Join(base, "cards"), filepath.Join(base, "tests")})
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
