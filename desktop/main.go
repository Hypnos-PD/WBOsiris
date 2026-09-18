// WBOsiris 桌面客户端：一个 Go 进程同时装规则引擎与界面。
//
//   - 规则服务跑在进程内（127.0.0.1 随机端口），卡池嵌在二进制里，脱机构筑/练习/回放可用；
//   - 界面就是 web/ 构建出来的那份前端，通过 Wails 的 AssetServer 提供；
//   - 页面加载时注入 window.WBO_API_BASE，前端据此连进程内服务；
//     想连线上（https://sva.hypd.asia/wbo，进大厅需要登录）就在设置里改服务地址。
package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	wbo "wbo"
	"wbo/internal/project"
	"wbo/internal/server"
)

//go:embed all:frontend/dist
var assets embed.FS

// version 由构建脚本用 -ldflags 注入（见 scripts/release/build-*）。
var version = "dev"

func main() {
	// 主界面素材（约 675 MB 的 40 张立绘）不进二进制：它们放在可执行文件旁边，
	// 由客户端直接当静态文件提供，这样二进制保持在几十 MB。
	assetsDir := flag.String("assets-dir", "", "主界面素材目录（默认找可执行文件旁的 assets/，再找 ../share/WBOsiris/assets）")
	apiBase := flag.String("api-base", "", "覆盖界面默认连接的服务地址（默认线上 https://sva.hypd.asia/wbo；填 local 用本机离线服务）")
	flag.Parse()
	mounted := resolveAssetsDir(*assetsDir)

	baseURL, shutdown, err := startRuleServer()
	if err != nil {
		log.Fatalf("启动规则服务失败: %v", err)
	}
	defer shutdown()
	log.Printf("WBOsiris %s · 规则服务 %s", version, baseURL)

	injected := ""
	if *apiBase == "local" {
		injected = baseURL
	} else if strings.TrimSpace(*apiBase) != "" {
		injected = strings.TrimSpace(*apiBase)
	}
	if injected == "" {
		log.Printf("界面默认连接线上服务 %s（本机离线服务：%s，可在设置里切换）", defaultRemoteBase, baseURL)
	} else {
		log.Printf("界面默认连接 %s（本机离线服务：%s）", injected, baseURL)
	}

	err = wails.Run(&options.App{
		Title:            "WBOsiris",
		Width:            1440,
		Height:           900,
		MinWidth:         1024,
		MinHeight:        680,
		BackgroundColour: &options.RGBA{R: 11, G: 11, B: 11, A: 1},
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Middleware: assetserver.ChainMiddleware(injectRuntimeConfig(baseURL, injected), serveAssetsFromDisk(mounted)),
		},
	})
	if err != nil {
		log.Fatalf("客户端退出: %v", err)
	}
}

// resolveAssetsDir 找主界面素材目录：显式传参优先，其次可执行文件旁的 assets/，
// 再退回安装目录（AppImage 里是 usr/share/WBOsiris/assets）。
func resolveAssetsDir(explicit string) string {
	candidates := []string{}
	if explicit != "" {
		candidates = append(candidates, explicit)
	}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		candidates = append(candidates, filepath.Join(dir, "assets"), filepath.Join(dir, "..", "share", "WBOsiris", "assets"))
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(filepath.Join(candidate, "home")); err == nil && info.IsDir() {
			absolute, absErr := filepath.Abs(candidate)
			if absErr == nil {
				log.Printf("主界面素材目录: %s", absolute)
				return absolute
			}
		}
	}
	return ""
}

// serveAssetsFromDisk 把 /assets/home/** 映射到磁盘上的素材目录；
// 磁盘上找不到就交回内嵌的 AssetServer（内置的那张默认立绘在 embed 里）。
func serveAssetsFromDisk(directory string) assetserver.Middleware {
	return func(next http.Handler) http.Handler {
		if directory == "" {
			return next
		}
		fileServer := http.FileServer(http.Dir(directory))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/assets/home/") {
				next.ServeHTTP(w, r)
				return
			}
			relative := strings.TrimPrefix(r.URL.Path, "/assets/")
			clean := filepath.Clean("/" + relative)
			if strings.Contains(clean, "..") {
				http.NotFound(w, r)
				return
			}
			if info, err := os.Stat(filepath.Join(directory, clean)); err != nil || info.IsDir() {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			r2 := r.Clone(r.Context())
			r2.URL.Path = clean
			fileServer.ServeHTTP(w, r2)
		})
	}
}

// defaultRemoteBase 是界面默认连接的线上服务（部署在 WBArts 站点的 /wbo/ 前缀下）。
const defaultRemoteBase = "https://sva.hypd.asia/wbo"

// injectRuntimeConfig 在返回 HTML 时插入运行期配置：
//   - WBO_LOCAL_API_BASE：进程内离线服务的地址（设置页“本机（离线）”用它）
//   - WBO_API_BASE：只有显式指定（--api-base）时才注入，作为默认地址覆盖
//
// 前端设置里存过地址（localStorage 的 wbo-api-base）时，那一份最优先。
func injectRuntimeConfig(localURL, injected string) assetserver.Middleware {
	script := `<script>window.WBO_LOCAL_API_BASE = "` + localURL + `";`
	if injected != "" {
		script += `window.WBO_API_BASE = "` + injected + `";`
	}
	script += `</script>`
	snippet := []byte(script)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || !wantsHTML(r) {
				next.ServeHTTP(w, r)
				return
			}
			recorder := &bodyRecorder{header: http.Header{}}
			next.ServeHTTP(recorder, r)
			body := recorder.body.Bytes()
			contentType := recorder.header.Get("Content-Type")
			if recorder.status != http.StatusOK || !strings.Contains(contentType, "text/html") {
				for key, values := range recorder.header {
					w.Header()[key] = values
				}
				w.WriteHeader(recorder.status)
				_, _ = w.Write(body)
				return
			}
			if index := bytes.LastIndex(body, []byte("</head>")); index >= 0 {
				body = append(body[:index:index], append(snippet, body[index:]...)...)
			} else {
				body = append(snippet, body...)
			}
			for key, values := range recorder.header {
				if strings.EqualFold(key, "Content-Length") {
					continue
				}
				w.Header()[key] = values
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		})
	}
}

func wantsHTML(r *http.Request) bool {
	path := r.URL.Path
	if path == "/" || path == "" || strings.HasSuffix(path, ".html") {
		return true
	}
	return !strings.HasPrefix(path, "/assets/") && filepath.Ext(path) == ""
}

type bodyRecorder struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (b *bodyRecorder) Header() http.Header { return b.header }

func (b *bodyRecorder) WriteHeader(status int) { b.status = status }

func (b *bodyRecorder) Write(data []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(data)
}

// startRuleServer 把嵌入的卡池落到缓存目录，起一个只监听本机的规则服务，
// 返回它的地址与关闭函数。端口用 0 让系统分配，避免和本机其它服务抢端口。
func startRuleServer() (string, func(), error) {
	cardsRoot, err := materializeCards()
	if err != nil {
		return "", nil, err
	}
	cardsDir := filepath.Join(cardsRoot, "cards")
	loaded := project.LoadWithRoot([]string{cardsDir}, false, cardsRoot)
	if loaded.HasErrors() {
		return "", nil, fmt.Errorf("卡池编译失败: %v", loaded.Diagnostics)
	}
	cards, tests, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		return "", nil, err
	}
	handler, err := server.NewWithPacks(cards, tests, "", "")
	if err != nil {
		return "", nil, err
	}
	handler.SetVersion(version)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	httpServer := &http.Server{Handler: handler.Handler()}
	go func() {
		if serveErr := httpServer.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Printf("规则服务停止: %v", serveErr)
		}
	}()
	return "http://" + listener.Addr().String(), func() { _ = httpServer.Close() }, nil
}

// materializeCards 把嵌入的 cards/ 解到用户缓存目录，返回可作为 source-root 的目录。
// 目录名带内容指纹，卡池变了就重新解一次。
func materializeCards() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		root = os.TempDir()
	}
	fingerprint, err := embeddedFingerprint()
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, "WBOsiris", "cards-"+fingerprint)
	if _, err := os.Stat(filepath.Join(target, "cards")); err == nil {
		return target, nil
	}
	temporary := target + ".tmp"
	_ = os.RemoveAll(temporary)
	err = fs.WalkDir(wbo.Cards, "cards", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		destination := filepath.Join(temporary, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		source, openErr := wbo.Cards.Open(path)
		if openErr != nil {
			return openErr
		}
		defer source.Close()
		if mkErr := os.MkdirAll(filepath.Dir(destination), 0o755); mkErr != nil {
			return mkErr
		}
		file, createErr := os.Create(destination)
		if createErr != nil {
			return createErr
		}
		if _, copyErr := io.Copy(file, source); copyErr != nil {
			file.Close()
			return copyErr
		}
		return file.Close()
	})
	if err != nil {
		return "", err
	}
	if err := os.Rename(temporary, target); err != nil {
		// 并发启动时另一个进程可能已经解好了，直接用现成的。
		if _, statErr := os.Stat(filepath.Join(target, "cards")); statErr == nil {
			return target, nil
		}
		return "", err
	}
	return target, nil
}

// embeddedFingerprint 用文件名与大小算一个指纹，足够区分卡池版本。
func embeddedFingerprint() (string, error) {
	hash := sha256.New()
	err := fs.WalkDir(wbo.Cards, "cards", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		fmt.Fprintf(hash, "%s:%d\n", path, info.Size())
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil))[:12], nil
}
