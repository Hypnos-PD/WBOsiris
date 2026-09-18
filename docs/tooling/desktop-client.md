# 桌面客户端（WBOsiris）

客户端是 **Go + Wails**（Linux 用 WebKitGTK，Windows 用 WebView2），目标产物是
`WBOsiris.exe`（含 NSIS 安装器）与 `WBOsiris-<版本>-x86_64.AppImage`，命名与打包形态
对齐 WBCapture。

## 一个进程，两件事

- **规则引擎在进程内**：客户端启动时把嵌入的卡池（`cards/**/*.wbo`，约 4 MB）解到
  用户缓存目录（`<cache>/WBOsiris/cards-<指纹>`），编译一次，然后起一个只监听
  `127.0.0.1` 的规则服务（端口由系统分配，避免和本机其它服务抢端口）。
- **界面就是 `web/` 构建出来的那份前端**：Wails 的 AssetServer 提供 `desktop/frontend/dist`，
  并在返回 HTML 时注入 `window.WBO_API_BASE`，前端据此连进程内服务。
  想连线上（`https://sva.hypd.asia/wbo`，进大厅需要登录）就在**设置 → 规则服务**里改地址，
  存在本机 `localStorage` 的 `wbo-api-base`，优先于注入值。

界面结构按约定：**主界面只有五项菜单**（单人模式 / 进入大厅 / 卡组管理 / 回放列表 / 设置），
没有侧栏、没有状态小字；子页面才显示侧栏。

## 主界面素材放在包内，不放二进制里

40 张主界面插图合计约 675 MB。如果 embed 进 Go 二进制，每次启动/构建都要处理这么大一块，
所以二进制里只保留内置默认那张 `hi_1001`，其余的按下面的规则找：

1. `--assets-dir` 显式指定；
2. 可执行文件旁的 `assets/`；
3. `<安装目录>/../share/WBOsiris/assets`（AppImage 里就是 `usr/share/WBOsiris/assets`）。

客户端会把 `/assets/home/**` 的请求交给这个目录（磁盘上没有时才回落到内嵌资源）。
素材本身由 `scripts/bundle_illustrations.mjs` 逐字节复制（不压缩、不转码）：

```bash
node scripts/bundle_illustrations.mjs --dry-run    # 只看规模
node scripts/bundle_illustrations.mjs              # 复制 40 张到 web/public/assets/home
```

## 本地测试

### 1. 界面快速循环（改 UI 用这个，不需要 Wails）

```bash
go run ./cmd/wbo serve --source-root .          # 规则服务，127.0.0.1:23215，本地模式不要求登录
cd web && npm run dev                            # Vite 5173，改前端即时热更新
```

浏览器打开 `http://localhost:5173`。前端在 localhost 下默认连 `127.0.0.1:23215`，
所以两条命令起来就能构筑、练习对战（对 AI）、看回放。

想直接拿**线上**服务调 UI（比如验证大厅与登录），生产端点带 `Access-Control-Allow-Origin: *`，
所以本地页面可以直接连：

```bash
cd web && VITE_API_BASE=https://sva.hypd.asia/wbo npm run dev
```

（线上进大厅要 WBArts 账号：设置页登录，jwt 存在 localStorage。）

### 2. 桌面壳（真窗口 / 改 Go 用这个）

```bash
scripts/wails.sh dev            # 用仓库固定的 Wails CLI（go.mod 的 tool 依赖），不用全局装
```

Wails CLI 已经作为 tool 依赖钉在 `go.mod` 里（`v2.16.0`），所以新环境只要 `go mod download`
就能用；Linux 上脚本会自动检测 WebKitGTK 版本，只有 4.1 的系统补上 `-tags webkit2_41`
（Arch、新版 Debian/Ubuntu 都是这种）。想直接调 CLI 也可以 `go tool wails dev`。

`wails dev` 会自己构建前端、开真窗口，并 watch `desktop/` 自动重建；它另起一个
`http://localhost:34115` 的 dev server。注意：它**不 watch `web/src`**，改前端请用上面第 1 条，
或者把 `wails.json` 的 `frontend:dev:watcher` 指到自定义脚本。

跑打包后的二进制：

```bash
scripts/wails.sh build -ldflags "-X main.version=dev"
./desktop/build/bin/WBOsiris                                   # 内嵌默认立绘
./desktop/build/bin/WBOsiris --assets-dir web/public/assets/home   # 用完整的 40 张立绘
```

客户端默认连**进程内**规则服务（离线可用）；要试线上大厅就在「设置 → 规则服务」填
`https://sva.hypd.asia/wbo`，保存后会自动重新连接。

### 3. 无头环境冒烟（CI / 没有桌面时）

```bash
Xvfb :99 -screen 0 1440x900x24 &
DISPLAY=:99 WEBKIT_DISABLE_COMPOSITING_MODE=1 LIBGL_ALWAYS_SOFTWARE=1 GDK_BACKEND=x11 \
  ./desktop/build/bin/WBOsiris &
sleep 15
DISPLAY=:99 magick import -window root /tmp/wbo-client.png    # 截图核对界面
```

WebKit 在虚拟显示里需要 `WEBKIT_DISABLE_COMPOSITING_MODE=1` 与软件 GL，否则窗口是空白。

### 4. 打包产物验收

```bash
scripts/release/build-appimage.sh --slim        # 先出小体积 AppImage 验证启动
scripts/release/build-appimage.sh               # 完整包（含 675 MB 素材）
```

AppImage 需要能运行 FUSE 或 `APPIMAGE_EXTRACT_AND_RUN=1`（脚本内部已用后者）。

### 常见坑

- **`command not found: wails`**：CLI 不需要全局安装，用 `scripts/wails.sh` 或 `go tool wails`；
  如果确实想装到全局，`go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0` 之后
  记得把 `$(go env GOPATH)/bin` 加进 PATH（zsh：`export PATH="$PATH:$(go env GOPATH)/bin"`）。
- **`wails doctor` 会说 `libwebkit` 缺失**：它只探测 WebKitGTK 4.0；Arch 只有 4.1，
  带上 `-tags webkit2_41` 就能编译，这条警告可以直接忽略。
- **窗口一片空白**：无头/虚拟显示下需要上面那三个环境变量；真机上一般是缺 WebKitGTK 运行库。
- **立绘上下有黑边**：`HomeIllustration.tsx` 会按容器比例取景，容器尺寸为 0 时不会建播放器；
  检查元素是否真的铺满（例如 `inset: -3%` 那层没被别处的 `overflow` 裁掉）。
- **Linux 需要 GTK/WebKit 开发包**：Debian/Ubuntu 是 `libgtk-3-dev libwebkit2gtk-4.1-dev`，
  Arch 是 `gtk3 webkit2gtk-4.1`。

Linux AppImage：

```bash
scripts/release/build-appimage.sh            # 完整包（含 675 MB 素材）
scripts/release/build-appimage.sh --slim     # 不含素材，体积小，素材走线上
scripts/release/build-appimage.sh --skip-build
```

脚本会下载并缓存 linuxdeploy / linuxdeploy-plugin-gtk / linuxdeploy-plugin-appimage
（`.cache/wbo-release-tools/`），组 AppDir（`usr/bin` + `.desktop` + 图标 + AppRun +
`usr/share/WBOsiris/assets`），用 GTK 插件把 GTK/WebKit 依赖一起打进去，然后做
**与 WBCapture 相同的 AppImage 预处理**：

1. GTK AppRun hook：`export GDK_BACKEND=x11` → `export GDK_BACKEND="${GDK_BACKEND:-wayland,x11}"`
   （Wayland 会话也能正常起窗）；
2. 删掉被捆进 AppDir 的 `libwayland-*.so*`，改用宿主的 Wayland ABI
   （这是 AppImage 在 Wayland 上崩溃的常见原因）；
3. 用 linuxdeploy-plugin-appimage 重新打包并覆盖原文件，同时删掉失效的 `.sig`。

每一步都有断言：hook 没改成、Wayland 库没删干净、重新打包没产出，都会直接失败退出。

Windows：

```powershell
pwsh -File scripts/release/build-windows.ps1                  # .exe + NSIS 安装器
pwsh -File scripts/release/build-windows.ps1 -SkipBuild
```

在 Windows 上跑（WebView2 加载器与 NSIS 都依赖 Windows 环境）；产物在 `dist/`，
素材目录 `dist/assets/home/` 与 exe 并列。CI 见 `.github/workflows/release-client.yml`
（windows-latest 与 ubuntu-22.04 双矩阵，打 tag 或手动触发）。

## 已验证 / 待验证

已验证：

- Linux 构建通过（`wails build -tags webkit2_41`），二进制约 94 MB（含前端资源，不含立绘）；
- 在 Xvfb 里启动成功，进程内规则服务起在随机端口，界面与主界面立绘正常；
- 主界面立绘铺满整窗（相机按容器比例 cover + 6% 安全放大，见 `HomeIllustration.tsx`）。

待验证（需要真机 / CI）：

- AppImage 产物在 Wayland 与 X11 会话下的实际启动（脚本已含预处理与断言）；
- Windows 的 `.exe` 与 NSIS 安装器（需要 Windows 环境）；
- WebView2 / WebKitGTK 下的 Spine 立绘渲染（客户端里主界面用的就是它）。
