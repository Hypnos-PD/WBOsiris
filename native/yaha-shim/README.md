# yaha-shim：把客户端的 HTTP 传输接到本地

原版客户端的所有服务端调用都走
`Cysharp.Net.Http.YetAnotherHttpHandler.Native.dll`（上游 Cysharp/YetAnotherHttpHandler
的 Rust cdylib，46 个导出的 C ABI 函数）。它就是一个放在游戏目录旁边的插件 DLL，
所以我们**不需要修改 `GameAssembly.dll`**：把这个文件换成我们的代理，再把它原来的
文件改名留在旁边即可。

```
客户端进程
  └─ Cysharp...Native.dll   ← 我们的代理（本 crate）
       └─ 46 个导出里的 45 个直接转发
       └─ yaha_request_set_uri 被我们接管：改写 URI，再转给原件
            └─ yaha_orig.dll  ← 改名后的原版，HTTP/2、TLS、连接池全部原样保留
```

## 为什么拦在这里

- **不改游戏文件**：只多一个 DLL、改一个文件名，`GameAssembly.dll` 一个字节都不动。
- **不用知道服务端地址**：客户端的目标地址是运行时组装的，二进制和数据文件里都没有
  明文。我们不需要找到它——在 URI 变成字符串的那一刻拦下来就行，顺手把原始地址记进
  日志，第一次运行就能把真实的域名摸出来。
- **不重写 HTTP**：TLS、HTTP/2、流式上传下载仍然由上游实现负责，我们只改一个 URL。
- **跨平台**：和客户端一样是 Windows x86-64 PE，在 Proton 下行为一致。

## 构建

需要 Windows 目标。Linux 上交叉编译装 mingw-w64 的工具链（Arch：`paru -S mingw-w64`，
五个包全要），它提供 `x86_64-w64-mingw32-gcc`，正好对应 Rust 的
`x86_64-pc-windows-gnu`：

```bash
cargo build --release --target x86_64-pc-windows-gnu
# 产物：target/x86_64-pc-windows-gnu/release/yaha_shim.dll
```

Windows 上用 MSVC 直接 `cargo build`（`--target x86_64-pc-windows-msvc`）。
不要选 `x86_64-pc-windows-gnullvm`：那个目标要的是 clang 版工具链
（`x86_64-w64-mingw32-clang`），gcc 这套用不了。

## 怎么装进游戏

**不要手动复制。** 直接跑：

```bash
wbo native launch --broker --capture ~/wbo-capture
```

它自己发现并核对客户端、推出该用哪个 Proton 与运行时，再把命令行拼出来。
**不需要在 Steam 里写启动选项**，也不需要从 Steam 界面启动游戏。

`wbo native launch` 会在挂载命名空间里做三件事，**磁盘上一个字节都不改**：

- 把 `yaha_shim.dll` 盖到 `ShadowverseWB_Data/Plugins/x86_64/Cysharp…Native.dll`
  这个位置上（这个文件本身没动，只在这个命名空间里被替换）；
- 用 bwrap 的 `--file` 现场生成 `yaha-shim.conf` 放在代理旁边——代理是按自己的
  模块路径找配置的，所以配置文件必须出现在插件目录里；
- 可选地起 `native broker`，随沙箱一起结束。

前提是本机装了 `bubblewrap`，且客户端由 Proton 运行。Windows 上没有等价的单文件
覆盖手段（同名 DLL 无法在不改盘的情况下被替换），那条路尚未实现。

手工安装仍然可行：把出厂 DLL 改名成 `yaha_orig.dll` 留在旁边，把代理放到原名，
再把 `yaha-shim.conf` 放到同一目录。这种方式会改动目录内容，仅在明确知道后果时使用。

## 配置

`yaha-shim.conf` 与 DLL 同目录，简单 `key = value`，`#` 开头是注释：

```ini
# 所有请求改写到这里
local = http://127.0.0.1:50172
# 记录被改写前的原始 URI，用于摸清客户端连过哪些地址
log = yaha-shim.log
```

配置也接受环境变量 `YAHA_SHIM_CONFIG` 指定的路径。找不到配置文件时用内置默认值，
并把日志写到 DLL 同目录的 `yaha-shim.log`。

## 验证

`test/run-test.sh` 在**真实 Windows 环境**（Proton 的前缀）里跑一个最小控制台程序，
不需要游戏也不需要 Steam。它加载代理、解析 9 个符号（8 个转发 + 1 个实体）、再走
一遍 `init_runtime → init_context → build_client → request_new → set_uri`。

实测结果：

```
== 导出符号
  yaha_get_last_error                                  解析成功
  yaha_request_set_uri                                 解析成功
  ...（9 个全部解析成功，转发目标 yaha_orig.dll 正常加载）
== 走一遍初始化 → 建请求 → 设置 URI
  set_uri("https://api.example.com/Version/info?x=1") = 成功
== 代理日志
  | rewrite https://api.example.com/Version/info?x=1 -> http://127.0.0.1:50172/Version/info?x=1
== 结论：全部通过
```

`set_uri` 返回成功这一项很关键：原件会用 `Uri::try_from` 解析我们改写后的字符串，
说明它接受这个结果，后续连接会真的发往本地。控制台输出会被 Proton 吞掉，所以测试
另外写一份 `yahatest-result.txt`。

代理自身的依赖也核过：只导入系统 UCRT 与 `KERNEL32`/`ntdll`，mingw 运行时是静态
链接的，不会出现"目标机器少一个 `libwinpthread-1.dll` 就加载失败"。

## 下一步

代理已经可用，剩下的是把它放进真正的游戏进程：

1. 客户端副本（27 GiB 复制，或挂载命名空间覆盖——本机是 ext4，没有 reflink，
   overlayfs 也没加载，所以只能二选一）；
2. 起 `wbo native broker`；
3. 跑一次，用 `yaha-shim.log` 里的原始 URI 确定拦截范围，用 broker 录到的请求体
   决定先实现哪条路由。

## 改写公钥：为什么要按元数据偏移改

客户端用它写死的那个静态公钥跟服务端做 X25519。要解开它的报文，就得把那个公钥换成
我们的（见 `docs/tooling/native-client.md` 的"信封"一节）。

常量来自内嵌的加密元数据：客户端每次调用 `CommonHeader()` 都从元数据里现拷一份
52 字节出来。所以"在内存里搜到那 32 字节再改掉"这条路是**不够的**——实测（1.9.5
客户端）全内存搜索能改到若干份副本，却改不到客户端真正读的那一份，表现是 broker
一直报"载荷认证失败"，而 shim 日志写着"改写完成"。所以这里改成两级：

1. **按偏移改元数据**：找到元数据块（用区间表自洽性判定，与
   `delta/tools/patch_common_header.py` 同一套判据），改写 `基址 + 0xFFC0C8`；
2. **兜底扫全内存**：按 32 字节模式改其余副本。

偏移 `0xFFC0C8` 是**跟版本走**的常量，换客户端版本要重新提取。

### 还没解决

即使改成按元数据偏移改写（会话日志里能看到"按元数据偏移改写"），**客户端随后发出的
第一批请求仍然是原公钥加密的**：broker 报"载荷认证失败"。已知事实：

- 用 `delta/tools/patch_common_header.py` 在**同一个进程**里核对时，那个地址确实
  已经是我们的公钥（"已经是我们的公钥，跳过"），说明改的位置对；
- 但客户端随后发请求时用的仍不是它——也就是"客户端到底从哪儿读这个公钥"还没有定论；

下一步要查的就是这件事：在进程里定位"请求用的那份公钥"，而不是"元数据里那份"。
在此之前，标题界面之前的请求解不开，夹具也就答不上（broker 会照常录下密文）。

## 校验导出表

`proxy.def` 是这份代理唯一容易写错、错了又必然炸在启动时的地方（漏一个导出或写错
转发目标，游戏就直接加载失败）。**权威清单是出厂 DLL 自己的导出表**，不是上游那份
生成的 C# 绑定：`yaha_client_config_unix_domain_socket_path` 在 Rust 侧是
`#[cfg(unix)]`，Windows 构建里根本没有它（C# 在 Windows 上会先抛
`PlatformNotSupportedException`，所以从不调用）。照 C# 绑定去写会多出一个转发目标
——而那个目标在 `yaha_orig.dll` 里并不存在。

核对方法：

```bash
# 原版 DLL 的导出表；本仓 scripts 里没有现成工具，用任意 PE 工具导出即可
python3 /tmp/pe_exports.py Cysharp.Net.Http.YetAnotherHttpHandler.Native.dll | sort -u > want.txt
grep -oE '^    yaha_[a-z0-9_]+' proxy.def | sed 's/^ *//' | sort -u > have.txt
diff want.txt have.txt        # 应该没有输出
```

当前这份 `proxy.def` 已经用上面的方法核对过：**45 个导出与原版逐个对应**，其中 44 个
转发到 `yaha_orig`，`yaha_request_set_uri` 由我们自己实现，且每个转发目标都确认
在原版 DLL 中存在。
