package nativeclient

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// 把本地资源铺进客户端：这是"客户端能跑到主界面"的最后一块拼图。
//
// 实测（2026-09-24，1.9.5 / Proton）客户端启动时会从**两个**位置读资源：
//
//	StreamingAssets/PreinResource/AssetBundle/<前两位>/<hname>   ← 真正加载的那一份
//	Persistent/dat/<前两位>/<hname>                              ← 下载缓存
//
// 只把资源放进 dat 是不够的：客户端会拿 Prein 那一条路径去 File.ReadAllBytes，
// 拿不到就抛 DirectoryNotFoundException，表现是"下载数据时发生错误"。
// 反过来，只铺 Prein 可以走到主界面（实测）。
//
// 清单自己也要各放一份，而且两边的**文件名不一样**：
//
//	StreamingAssets/PreinResource/manifests/assetbundle.<语言>.manifest   ← 固定名
//	Persistent/dat/<前两位>/<客户端算出来的名字>                          ← 32 位 base32
//
// 那个 32 位名字是客户端用 `CalcHName(SHA-1, checksum, size, name)` 算出来的
// （见 docs/tooling/native-client.md 的逆向记录）。它的三个输入里头我们只认定了
// 编码方式，输入还没复刻出来，所以这里不去猜：客户端找不到文件时会把**完整路径**
// 写进 Player.log，我们照抄。

// ResourceSource 是一份本地资源库，布局与 wbunpacker 的产物一致：
//
//	<root>/manifests/raw/assetbundle.<语言>.manifest
//	<root>/blobs/raw/<前两位>/<hname>
type ResourceSource struct {
	Root string
}

// ManifestPath 是这份资源的清单文件。
func (s ResourceSource) ManifestPath(lang string) string {
	return filepath.Join(s.Root, "manifests", "raw", "assetbundle."+lang+".manifest")
}

// BlobDir 是资源本体所在的目录。
func (s ResourceSource) BlobDir() string {
	return filepath.Join(s.Root, "blobs", "raw")
}

// BlobPath 是某个 hname 对应的资源本体。
func (s ResourceSource) BlobPath(hname string) string {
	return filepath.Join(s.BlobDir(), hname[:2], hname)
}

// 客户端目录里的三个落点。
func preinBundleDir(clientRoot string) string {
	return filepath.Join(clientRoot, "ShadowverseWB_Data", "StreamingAssets", "PreinResource", "AssetBundle")
}

func preinManifestPath(clientRoot, lang string) string {
	return filepath.Join(clientRoot, "ShadowverseWB_Data", "StreamingAssets", "PreinResource", "manifests",
		"assetbundle."+lang+".manifest")
}

func datDir(clientRoot string) string {
	return filepath.Join(clientRoot, "ShadowverseWB_Data", "Persistent", "dat")
}

// ProvisionOptions 决定把哪份资源铺进哪份客户端。
type ProvisionOptions struct {
	ClientDir string
	Source    ResourceSource
	Lang      string
	// ManifestHname 是 Persistent/dat 下那份清单的名字。留空时从 Player.log 里学。
	ManifestHname string
	// PlayerLog 只在测试或调试时显式指定；正常路径由 ProtonPrefix 推导。
	PlayerLog string
	DryRun    bool
}

// ProvisionReport 是这次铺开的结果。
type ProvisionReport struct {
	ManifestHname string
	ManifestCopy  string
	ManifestBlob  string
	Bundles       int
	Bytes         int64
	Skipped       int
}

// Provision 把资源铺进客户端目录。
//
// 文件优先用硬链接（同一文件系统的常见情形下零拷贝、零额外空间），跨文件系统时
// 退回真复制。已经铺好的文件按"大小一致"跳过，所以重复执行是廉价的，也不会把
// 客户端自己后来下载的东西冲掉。
func Provision(opts ProvisionOptions, logf func(string, ...any)) (*ProvisionReport, error) {
	if opts.Lang == "" {
		return nil, fmt.Errorf("nativeclient: 需要指定语言变体（Chs/Cht/Eng/Jpn/Kor）")
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	install, err := Inspect(opts.ClientDir)
	if err != nil {
		return nil, err
	}
	manifest := opts.Source.ManifestPath(opts.Lang)
	if !isFile(manifest) {
		return nil, fmt.Errorf("nativeclient: 资源库里没有清单 %s", manifest)
	}
	blobs := opts.Source.BlobDir()
	if !isDir(blobs) {
		return nil, fmt.Errorf("nativeclient: 资源库里没有 blob 目录 %s", blobs)
	}

	report := &ProvisionReport{
		ManifestHname: opts.ManifestHname,
		ManifestCopy:  preinManifestPath(install.Root, opts.Lang),
	}
	if report.ManifestHname == "" {
		hname, err := LearnManifestHname(opts.PlayerLog, install)
		if err != nil {
			return nil, err
		}
		report.ManifestHname = hname
		logf("清单名（从 Player.log 学到）：%s", hname)
	}
	report.ManifestBlob = filepath.Join(datDir(install.Root), report.ManifestHname[:2], report.ManifestHname)

	names, err := listBundles(blobs)
	if err != nil {
		return nil, err
	}
	report.Bundles = len(names)

	if opts.DryRun {
		logf("--dry-run：计划铺开 %d 个资源、%d 份清单", len(names), 2)
		return report, nil
	}

	if err := materialize(manifest, report.ManifestCopy); err != nil {
		return nil, err
	}
	if err := materialize(manifest, report.ManifestBlob); err != nil {
		return nil, err
	}

	done := 0
	for _, name := range names {
		source := filepath.Join(blobs, name[:2], name)
		target := filepath.Join(preinBundleDir(install.Root), name[:2], name)
		skipped, size, err := materializeCounting(source, target)
		if err != nil {
			return nil, err
		}
		report.Bytes += size
		if skipped {
			report.Skipped++
		}
		done++
		if done%2000 == 0 {
			logf("已铺 %d/%d", done, len(names))
		}
	}
	logf("铺开完成：%d 个资源、%.2f GiB，跳过（已存在）%d 个", report.Bundles, float64(report.Bytes)/(1<<30), report.Skipped)
	return report, nil
}

// listBundles 列出资源库里所有 hname（前两位目录名 + 名字，都是 base32）。
func listBundles(blobs string) ([]string, error) {
	entries, err := os.ReadDir(blobs)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		children, err := os.ReadDir(filepath.Join(blobs, entry.Name()))
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			if child.IsDir() {
				continue
			}
			names = append(names, child.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// materialize 让 target 与 source 指向同一份内容，已经是同一份就什么都不做。
func materialize(source, target string) error {
	_, _, err := materializeCounting(source, target)
	return err
}

// materializeCounting 是 materialize 的本体，顺带报告大小与是否跳过。
func materializeCounting(source, target string) (skipped bool, size int64, err error) {
	in, err := os.Stat(source)
	if err != nil {
		return false, 0, err
	}
	if out, err := os.Stat(target); err == nil && out.Size() == in.Size() {
		return true, in.Size(), nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return false, 0, err
	}
	if err := os.Link(source, target); err == nil {
		return false, in.Size(), nil
	}
	return false, in.Size(), copyFileTo(source, target)
}

// copyFileTo 是硬链接不可用时的退路（跨文件系统时会以 EXDEV 失败）。
func copyFileTo(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// manifestMissing 匹配客户端在 Player.log 里写的"本地缺哪份清单"。
//
// 客户端打印的是 Windows 路径，例如
// `Could not find file "S:\…\Persistent\dat\RJ\RJKAM6CYLA7Q3GV3746GJZVN2SR3XTYD"`，
// 也就是 dat 下面还有一层"前两位"目录。
var manifestMissing = regexp.MustCompile(`Could not find file "[^"]*[Pp]ersistent[\\/]dat[\\/][A-Z2-7]{2}[\\/]([A-Z2-7]{26,32})"`)

// LearnManifestHname 从客户端日志里学出清单 blob 的名字。
//
// 客户端算名字的算法没复刻出来（`CalcHName` 的三个输入只认定了一个），但它在找不到
// 文件时会把完整路径原样打进 Player.log——那就让它自己说。代价是资源必须由客户端
// 先跑一次（`wbo native launch`，到标题界面点一次"开始游戏"）才会留下这行日志。
func LearnManifestHname(playerLog string, install Install) (string, error) {
	path := playerLog
	if path == "" {
		if install.ProtonPrefix == "" {
			return "", fmt.Errorf("nativeclient: 这份客户端不是 Proton 跑的，推导不出 Player.log；" +
				"要么用 --manifest-hname 直接给名字，要么用 --player-log 指定日志")
		}
		// ProtonPrefix 是 compatdata/<appid>，日志在它下面的 pfx 里。
		path = filepath.Join(install.ProtonPrefix, "pfx", "drive_c", "users", "steamuser", "AppData", "LocalLow",
			"Cygames", "ShadowverseWB", "Player.log")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("nativeclient: 读不到 %s：%w", path, err)
	}
	matches := manifestMissing.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("nativeclient: %s 里没有清单缺失记录；"+
			"先跑一次 wbo native launch 并点到「开始游戏」，让客户端把它想要的路径写下来", path)
	}
	// 客户端每次缺文件会重试，取最后一条即可。
	hname := matches[len(matches)-1][1]
	if len(hname) != 32 {
		return "", fmt.Errorf("nativeclient: 日志里的 %s 不像清单名（清单名是 32 位 base32，资源是 26 位）", hname)
	}
	return hname, nil
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// IsSteamManaged 判断这份客户端是不是 Steam 的库直接管着的。
//
// 正式流程不应该往 Steam 的目录里写，命令层用它来要求显式的"明知故犯"开关。
func IsSteamManaged(install Install) bool {
	return strings.Contains(filepath.ToSlash(install.Root), "/steamapps/common/")
}
