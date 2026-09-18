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

## 构建

开发（本机窗口）：

```bash
cd desktop
wails dev              # 前端热更新；Linux 需要 -tags webkit2_41（按发行版的 WebKitGTK 版本）
```

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
