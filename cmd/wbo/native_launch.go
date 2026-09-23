package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"wbo/internal/nativeclient"
)

// native launch 把"带本地服务端的原版客户端"整个起起来，用户不需要知道 Steam 有
// 启动选项这回事：
//
//	/home/you/wbo native launch --broker --capture ~/wbo-capture
//
// 它发现并核对客户端、推出该用哪个 Proton 与运行时、在挂载命名空间里把代理 DLL
// 盖到客户端的那个插件位置上，最后执行自己拼出来的命令行。**磁盘上什么都不改**：
// Steam 的文件一个字节都没动，代理只存在于这个命名空间里。这样就不用复制 27 GiB
// （本机是 ext4，没有 reflink），也不用碰 overlayfs（没加载）。
//
// 想自己给命令（调试）时用 `-- 命令…`：那时环境变量原样继承，不再自己推导。
//
// 这套只在 Linux/Proton 上成立：它依赖 bubblewrap 的挂载命名空间。Windows 上
// 需要另一套做法（同一个文件名的 DLL 无法在不改盘的情况下被覆盖），尚未实现。

const (
	pluginDirectory = "ShadowverseWB_Data/Plugins/x86_64"
	pluginName      = "Cysharp.Net.Http.YetAnotherHttpHandler.Native.dll"
	pluginRelative  = pluginDirectory + "/" + pluginName
)

// originalName 是代理期待原件出现的名字。
//
// 代理的 44 个导出是 PE 转发，由 Windows 加载器按名字去找这个模块；我们没法在
// 运行时改它的查找路径，所以只能真的让它存在。
const originalName = "yaha_orig.dll"

// commonHeaderHex 是客户端用来跟服务器做 X25519 的那 52 字节常量里的一份记录。
//
// 它的前 32 字节被当成"对端公钥"：客户端拿它跟自己的临时私钥做 ECDH。这份内容来自
// 内嵌在 GameAssembly.dll 里的加密元数据，磁盘上搜不到明文，只有运行时内存里才有，
// 所以代理是在进程里按这段字节找到它再改写的。
//
// 来源：WBArts 的 server/ranking/protocol.go（DefaultCommonHeaderHex），与这份
// 1.9.5 客户端实测发出的 App-Ver/Routing-Header 一致。
//
// 找的是前 32 字节而不是整 52 字节：实测内存里那 32 字节出现 3 次，整块只出现 2 次，
// 多出来的那一处没有尾部——它才像是被直接当密钥用的那份。按 32 字节找能把三处都改到，
// 按整块找会漏掉它。
const peerPatternHex = "5EBB1B7E66F37753855EAEE1D9F9EFFEE740591A306A1BBC034A9569A778A562"

// confRelative 是代理按"自己的模块路径"去找配置的位置。代理被盖在插件目录里，
// 所以配置也要出现在那里；launcher 用 bwrap 的 --file 现场生成，不往游戏目录写东西。
const confRelative = "ShadowverseWB_Data/Plugins/x86_64/yaha-shim.conf"

func runNativeLaunch(args []string) int {
	// 日志要最先建立：从 Steam 启动时出错是看不见的，唯一能查的就是这个文件。
	stateDir, logFile, err := openLaunchLog(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer logFile.Close()
	logf := func(format string, values ...any) {
		line := fmt.Sprintf(format, values...)
		fmt.Fprintln(os.Stderr, line)
		fmt.Fprintln(logFile, line)
	}
	logf("=== %s", time.Now().Format(time.RFC3339))
	logf("命令行 %s", strings.Join(os.Args, " "))

	fs := flag.NewFlagSet("native launch", flag.ContinueOnError)
	fs.SetOutput(logFile)
	shim := fs.String("shim", "", "代理 DLL；默认取 wbo 同目录下的 yaha_shim.dll")
	broker := fs.Bool("broker", false, "同时在沙箱里起 broker")
	listen := fs.String("listen", "127.0.0.1:50172", "broker 监听地址")
	capture := fs.String("capture", "", "broker 录制目录")
	state := fs.String("state", "", "会话目录（放代理日志和现场生成的配置）；默认 ~/.local/state/wbo/launch")
	dryRun := fs.Bool("dry-run", false, "只打印将要执行的命令")
	noPatch := fs.Bool("no-patch", false, "对照实验用：不改写客户端内存里的公钥常量")
	clientFlag := fs.String("client-dir", "", "客户端目录；省略时按 Steam 库自动发现并核对")
	protonFlag := fs.String("proton", "", "Proton 目录或 proton 脚本的绝对路径；省略时按 Steam 配置推导")
	runtimeFlag := fs.String("runtime", "", "Steam Linux Runtime 入口（如 .../_v2-entry-point）；省略时按 Proton 的声明推导")
	sessionKeyFlag := fs.String("session-key", "", "会话私钥；省略时用会话目录里的 session-key（本轮启动刚生成的那把）")
	uuidFlag := fs.String("uuid", "", "客户端凭据 uuid；给了才会解开加密的请求体")
	authKeyFlag := fs.String("auth-key", "", "客户端凭据 auth key（base64）")
	// flag 包遇到第一个非旗标参数就停，所以先把 "--" 之前的部分交给它，
	// 之后的原样当作 Steam 给的命令。
	separator := indexOf(args, "--")
	var flags, rest []string
	if separator < 0 {
		flags = args
	} else {
		flags, rest = args[:separator], args[separator+1:]
	}
	if fs.Parse(flags) != nil {
		return 2
	}
	if *state != "" {
		stateDir = *state
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		logf("建会话目录失败 %v", err)
		return 1
	}
	logf("会话目录 %s", stateDir)
	if runtime.GOOS != "linux" {
		logf("native launch 目前只实现了 Linux/Proton（依赖 bubblewrap 的挂载命名空间）")
		return 1
	}
	// 两条路：给了 "-- 命令" 就用 Steam 给的那条（启动选项模式，保留给调试），
	// 没给就自己把 Steam 那条命令行拼出来（正常路径，用户不需要知道 Steam 启动
	// 选项是什么东西）。
	clientDir, tool, err := resolveTarget(rest, *clientFlag, *protonFlag, *runtimeFlag, logf)
	if err != nil {
		logf("%v", err)
		return 1
	}
	logf("客户端目录 %s", clientDir)
	if tool != nil {
		logf("Proton %s（%s）", tool.Name, tool.Version)
		logf("运行时 %s", tool.EntryPoint)
		if tool.Mapping != "" {
			logf("兼容工具由 config.vdf 指定：%s", tool.Mapping)
		}
	} else {
		logf("命令由调用方提供，环境变量原样继承")
	}
	shimPath := *shim
	if shimPath == "" {
		var looked []string
		shimPath, looked, err = findShim()
		if err != nil {
			logf("找不到代理 DLL yaha_shim.dll，找过这些位置：")
			for _, candidate := range looked {
				logf("  %s", candidate)
			}
			logf("把构建产物拷到其中一个位置，或者用 --shim 指定绝对路径：")
			logf("  cp native/yaha-shim/target/x86_64-pc-windows-gnu/release/yaha_shim.dll %s", firstDirOf(looked))
			logf("  %v", err)
			return 1
		}
	}
	for _, required := range []string{shimPath, filepath.Join(clientDir, filepath.FromSlash(pluginRelative))} {
		if _, err := os.Stat(required); err != nil {
			logf("找不到 %s", required)
			return 1
		}
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		logf("找不到 bwrap（bubblewrap），它是这套做法的前提")
		return 1
	}
	self, err := os.Executable()
	if err != nil {
		logf("%v", err)
		return 1
	}
	// 只覆盖插件本名是不够的：代理的 44 个导出是 PE 转发，加载器要按名字找到
	// yaha_orig.dll。所以在会话目录里搭一份插件目录的替身（其余文件软链接回真实
	// 位置），代理和原件都放进去，再把整个目录挂上去——游戏目录仍然一个字节不改。
	overlays, err := prepareOverlays(stateDir, clientDir, shimPath)
	if err != nil {
		logf("搭替身目录失败 %v", err)
		return 1
	}
	// 生成会话密钥对。客户端写死的"对端公钥"会被换成我们这把公钥，于是它加密出来的
	// 东西只有拿着私钥的 broker 能解开——这是能当服务器的唯一办法，因为原公钥对应的
	// 私钥在官方服务器上。
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		logf("生成会话密钥失败 %v", err)
		return 1
	}
	publicKey := privateKey.PublicKey().Bytes()
	// 客户端按大端 MPI 读这 32 字节再翻转，所以写进去的必须是翻转后的形式。
	keyBytes := reverse(publicKey)
	if err := os.WriteFile(filepath.Join(stateDir, "session-key"), privateKey.Bytes(), 0o600); err != nil {
		logf("保存会话密钥失败 %v", err)
		return 1
	}
	confPath, err := writeShimConfig(stateDir, *listen, keyBytes, *noPatch)
	if err != nil {
		logf("%v", err)
		return 1
	}
	// 从进程外部改写公钥常量：代理是 Unity 按需加载的，等它进进程再扫就太晚了。
	pattern, err := hex.DecodeString(peerPatternHex)
	if err != nil {
		logf("常量配置不合法 %v", err)
		return 1
	}
	// 代理自己在 init_context 里同步完成改写（那一步正好在第一个请求之前），
	// 从进程外改这条路就不用走了。
	_ = pattern
	stopPatcher := make(chan struct{})
	// broker 在宿主机侧起，不放进沙箱：bwrap 要等命名空间里所有进程结束才退出，
	// 后台常驻的 broker 会让它永远不返回。等到要做网络隔离时再挪进去，那时候
	// 需要的是"主命令退出时把 broker 收掉"的写法。
	var brokerProcess *exec.Cmd
	if *broker {
		brokerArgs := []string{"native", "broker", "--listen", *listen}
		if *capture != "" {
			brokerArgs = append(brokerArgs, "--capture", *capture)
		}
		// 会话私钥默认就是这次启动刚写下的那一把：代理改写的公钥与它配对，
		// 不给的话 broker 只能录到密文。
		sessionKey := *sessionKeyFlag
		if sessionKey == "" {
			sessionKey = filepath.Join(stateDir, "session-key")
		}
		if _, err := os.Stat(sessionKey); err == nil {
			brokerArgs = append(brokerArgs, "--session-key", sessionKey)
		}
		if *uuidFlag != "" {
			brokerArgs = append(brokerArgs, "--uuid", *uuidFlag)
		}
		if *authKeyFlag != "" {
			brokerArgs = append(brokerArgs, "--auth-key", *authKeyFlag)
		}
		brokerProcess, err = startBroker(self, brokerArgs, *listen, logFile, logf)
		if err != nil {
			logf("%v", err)
			return 1
		}
		defer stopBroker(brokerProcess, logf)
	}
	command := rest
	if len(command) == 0 {
		command = tool.GameCommand(clientDir)
	}
	line := buildLaunchCommand(launchPlan{
		Bwrap:     bwrap,
		ClientDir: clientDir,
		Overlays:  overlays,
		Command:   command,
	})
	if *dryRun {
		logf("将执行 %s", strings.Join(line, " "))
		return 0
	}
	logf("把代理盖到 %s", filepath.Join(clientDir, filepath.FromSlash(pluginRelative)))
	logf("执行 %s", strings.Join(line, " "))
	process := exec.Command(line[0], line[1:]...)
	// 子进程的输出也并进日志：游戏起不来的原因通常就在这里面。
	process.Stdin = os.Stdin
	process.Stdout = io.MultiWriter(os.Stdout, logFile)
	process.Stderr = io.MultiWriter(os.Stderr, logFile)
	// 配置走环境变量，不往游戏目录里放文件：bwrap 的 --file 会在底层文件系统上
	// 真的创建那个文件，等于改动了 Steam 的安装目录。环境变量会一路传到游戏进程。
	environment := append(os.Environ(), "YAHA_SHIM_CONFIG="+windowsPath(confPath))
	if tool != nil {
		environment = append(environment, tool.Environment(clientDir)...)
	}
	process.Env = environment
	runErr := process.Run()
	close(stopPatcher)
	logf("子进程结束 %v", runErr)
	if runErr != nil {
		if exit, ok := runErr.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		return 1
	}
	return 0
}

// openLaunchLog 准备会话目录和日志文件。
//
// 从 Steam 启动时，进程的输出和退出码都不会出现在任何地方；没有这个文件，
// "点了启动但没反应"就没法查。默认位置固定，用户不用去临时目录里翻。
func openLaunchLog(args []string) (string, *os.File, error) {
	directory := ""
	for index, value := range args {
		if value == "--state" && index+1 < len(args) {
			directory = args[index+1]
		}
	}
	if directory == "" {
		base := os.Getenv("XDG_STATE_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", nil, err
			}
			base = filepath.Join(home, ".local", "state")
		}
		directory = filepath.Join(base, "wbo", "launch")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", nil, err
	}
	file, err := os.OpenFile(filepath.Join(directory, "launch.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", nil, err
	}
	return directory, file, nil
}

// windowsPath 把宿主机路径翻译成 Windows 看得到的写法。
//
// 代理跑在游戏进程里，是 Windows 代码；Wine 默认把 Z: 映射到根目录，所以文件
// 在两边是同一个，只是写法不同。直接给它 Unix 路径会被当成"当前盘符下的相对路径"，
// 而游戏的当前盘符是它自己的安装盘。
func windowsPath(path string) string {
	trimmed := strings.TrimPrefix(path, "/")
	return `Z:\` + strings.ReplaceAll(trimmed, "/", `\`)
}

type launchPlan struct {
	Bwrap     string
	ClientDir string
	Overlays  overlays
	Command   []string
}

// overlays 是替身目录与它们各自的挂载点，按挂载顺序排列（浅的先挂，深的后挂）。
type overlays struct {
	Bindings []overlayBinding
}

type overlayBinding struct {
	Source string
	Target string
}

// startBroker 在宿主机侧起一个 broker，并等它真的开始监听。
//
// 不等就返回的话，游戏可能在 broker 就绪前发出第一个请求，那一次就会失败——
// 而"第一个请求"恰恰是我们最想录到的。
func startBroker(self string, arguments []string, listen string, logFile *os.File,
	logf func(string, ...any)) (*exec.Cmd, error) {
	command := exec.Command(self, arguments...)
	command.Stdout = io.MultiWriter(os.Stdout, logFile)
	command.Stderr = io.MultiWriter(os.Stderr, logFile)
	setProcessGroup(command)
	if err := command.Start(); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", listen, 200*time.Millisecond)
		if err == nil {
			connection.Close()
			logf("broker 就绪 %s（pid %d）", listen, command.Process.Pid)
			return command, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	stopBroker(command, logf)
	return nil, fmt.Errorf("broker 在 5 秒内没有监听 %s", listen)
}

func stopBroker(command *exec.Cmd, logf func(string, ...any)) {
	if command == nil || command.Process == nil {
		return
	}
	if err := killProcessGroup(command); err != nil {
		logf("收 broker 失败 %v", err)
	}
	_, _ = command.Process.Wait()
	logf("broker 已停止")
}

// writeShimConfig 现场生成代理的配置。
//
// 日志写到会话目录而不是插件目录：游戏目录是 Steam 的，不该往里丢文件。配置本身
// 通过 bwrap 的 --file 出现在插件目录里，磁盘上不落任何东西。
// writeShimConfig 现场生成代理的配置。
//
// 日志写在会话目录：游戏目录是 Steam 的，不该往里丢文件。原件不需要在这里指定，
// 替身目录里已经按代理期待的名字放好了。
func writeShimConfig(stateDir, listen string, peerKey []byte, noPatch bool) (string, error) {
	path := filepath.Join(stateDir, "yaha-shim.conf")
	if noPatch {
		lines := []string{
			"local = http://" + listen,
			"log = " + filepath.Join(stateDir, "yaha-shim.log"),
			"no-patch = true",
		}
		content := strings.Join(lines, "\n") + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return "", err
		}
		return path, nil
	}
	lines := []string{
		"local = http://" + listen,
		"log = " + filepath.Join(stateDir, "yaha-shim.log"),
		// 代理在进程内存里找这段字节并改写前 32 字节。
		"pattern = " + peerPatternHex,
		"peerkey = " + hex.EncodeToString(peerKey),
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func reverse(value []byte) []byte {
	result := make([]byte, len(value))
	for index := range value {
		result[index] = value[len(value)-1-index]
	}
	return result
}

// buildLaunchCommand 拼出 bwrap 命令行。抽成纯函数是为了能对它写测试——
// 这条命令一旦拼错，表现是"游戏起不来"，很难在现场定位。
func buildLaunchCommand(plan launchPlan) []string {
	line := []string{
		plan.Bwrap,
		// 整个文件系统原样可见，只有替身目录列出的那几个路径被盖掉。
		"--dev-bind", "/", "/",
	}
	for _, binding := range plan.Overlays.Bindings {
		line = append(line, "--bind", binding.Source, binding.Target)
	}
	return append(append(line, "--"), plan.Command...)
}

// prepareOverlays 在会话目录里搭出游戏目录的替身。
//
// 规则很简单：文件用硬链接，目录用空目录占位再显式挂载真实目录。**不能用软链接**
// ——软链接会指回被覆盖的路径，覆盖上去之后就成了自引用。
//
// 需要这份替身是因为 PE 转发：代理的 44 个导出由 Windows 加载器按名字解析，
// 它按标准搜索顺序找，第一步就是 exe 所在目录。所以 yaha_orig.dll 必须出现在
// 游戏根目录里，而我们不想往 Steam 的目录里写东西。
func prepareOverlays(stateDir, clientDir, shim string) (overlays, error) {
	var result overlays
	realPluginDir := filepath.Join(clientDir, filepath.FromSlash(pluginDirectory))
	original := filepath.Join(realPluginDir, pluginName)
	clientOverlay := filepath.Join(stateDir, "client")
	// 每次都重建：上一轮可能留下游戏写进来的文件，混进新的替身里会很难解释。
	if err := os.RemoveAll(clientOverlay); err != nil {
		return result, err
	}
	if err := mirrorDirectory(clientDir, clientOverlay, map[string]string{pluginName: shim}); err != nil {
		return result, err
	}
	if err := linkFile(original, filepath.Join(clientOverlay, originalName)); err != nil {
		return result, err
	}
	// 先挂根目录替身，再按深度把真实目录挂回去。
	result.Bindings = append(result.Bindings, overlayBinding{clientOverlay, clientDir})
	for _, binding := range mirroredDirectories(clientDir, clientOverlay) {
		result.Bindings = append(result.Bindings, binding)
	}
	pluginOverlay := filepath.Join(stateDir, "plugins")
	if err := os.RemoveAll(pluginOverlay); err != nil {
		return result, err
	}
	if err := mirrorDirectory(realPluginDir, pluginOverlay, map[string]string{pluginName: shim}); err != nil {
		return result, err
	}
	if err := linkFile(original, filepath.Join(pluginOverlay, originalName)); err != nil {
		return result, err
	}
	// 插件目录最深，最后挂，盖在真实目录之上。
	result.Bindings = append(result.Bindings, overlayBinding{pluginOverlay,
		filepath.Join(clientDir, filepath.FromSlash(pluginDirectory))})
	return result, nil
}

// mirrorDirectory 把 source 里的每个条目放进 target：文件硬链接、目录建成空目录；
// replacements 里的名字改成指向别处。
func mirrorDirectory(source, target string, replacements map[string]string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(target, name)
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		if replacement, ok := replacements[name]; ok {
			if err := linkFile(replacement, path); err != nil {
				return err
			}
			continue
		}
		if entry.IsDir() {
			// 目录不能硬链接；留一个空目录，稍后用挂载填上真实内容。
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := linkFile(filepath.Join(source, name), path); err != nil {
			return err
		}
	}
	return nil
}

// mirroredDirectories 列出需要用真实目录填上的挂载点，浅的在前。
//
// 挂载源和目标**是同一个字符串**，这不是笔误：bwrap 把源解析在宿主机视图上、
// 把目标解析在已经搭好的沙箱视图上。根目录替身挂上去之后，这条路径在沙箱里指向
// 替身留的空目录，于是"把真实目录挂到这条路径上"正好把空目录填成真内容。
//
// 之前这里写错过一次：源写成替身里的空目录，游戏看到的就还是空目录，Unity 直接报
// "There should be 'ShadowverseWB_Data' folder next to the executable"。
func mirroredDirectories(realRoot, overlayRoot string) []overlayBinding {
	entries, err := os.ReadDir(realRoot)
	if err != nil {
		return nil
	}
	var directories []string
	for _, entry := range entries {
		if entry.IsDir() {
			directories = append(directories, entry.Name())
		}
	}
	sort.Slice(directories, func(i, j int) bool {
		return strings.Count(directories[i], "/") < strings.Count(directories[j], "/")
	})
	var bindings []overlayBinding
	for _, name := range directories {
		bindings = append(bindings, overlayBinding{
			Source: filepath.Join(realRoot, name),
			Target: filepath.Join(realRoot, name),
		})
	}
	return bindings
}

// linkFile 让 target 与 source 是同一个文件。硬链接而不是软链接：原件要被 Windows
// 加载器按名字找到，普通文件比符号链接少一层不确定性。
//
// 会话目录和 Steam 库不在同一个文件系统时硬链接会以 EXDEV 失败（默认位置在
// ~/.local/state，通常没事；用户把库放在别的盘就会碰上），这时退回真复制一份。
// 替身目录只镜像客户端根目录和插件目录这两层，所以最坏情况下多复制的是
// GameAssembly.dll 那一份大文件，不是整棵 27 GiB。
func linkFile(source, target string) error {
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Link(source, target); err == nil {
		return nil
	}
	return copyFile(source, target)
}

// copyFile 是硬链接不可用时的退路：逐字节复制。
func copyFile(source, target string) error {
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

// findClientDir 从 Steam 给的命令里认出客户端目录。
//
// Steam 的 %command% 形如 `…/run-in-sniper --verb=… -- …/proton waitforexitandrun
// …/ShadowverseWB.exe`，所以"以进程名结尾的那个参数"就是可执行文件。
func findClientDir(command []string) (string, error) {
	for _, argument := range command {
		if !strings.EqualFold(filepath.Base(argument), nativeclient.ProcessName) {
			continue
		}
		directory := filepath.Dir(argument)
		if _, err := os.Stat(filepath.Join(directory, filepath.FromSlash(pluginRelative))); err != nil {
			return "", fmt.Errorf("在 %s 里找不到客户端插件 %s", directory, pluginRelative)
		}
		return directory, nil
	}
	return "", fmt.Errorf("在命令里找不到 %s，无法确定客户端目录；确认启动选项里带上了 %%command%%", nativeclient.ProcessName)
}

// findShim 在几个合理的位置里找代理 DLL：和 wbo 同目录、wbo 旁边的 lib 子目录，
// 以及用户数据目录。找不到时把找过的路径原样返回，便于照抄。
func findShim() (string, []string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", nil, err
	}
	directory := filepath.Dir(self)
	var candidates []string
	candidates = append(candidates, filepath.Join(directory, "yaha_shim.dll"))
	if data := os.Getenv("XDG_DATA_HOME"); data != "" {
		candidates = append(candidates, filepath.Join(data, "wbo", "yaha_shim.dll"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".local", "share", "wbo", "yaha_shim.dll"),
			filepath.Join(home, ".local", "lib", "wbo", "yaha_shim.dll"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, candidates, nil
		}
	}
	return "", candidates, fmt.Errorf("这几个位置都没有 yaha_shim.dll")
}

func firstDirOf(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return filepath.Dir(paths[0])
}

func indexOf(values []string, want string) int {
	for index, value := range values {
		if value == want {
			return index
		}
	}
	return -1
}
