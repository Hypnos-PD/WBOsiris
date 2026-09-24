# 原版客户端：发现、校验与导入

WBO 的对局界面用原版《影之诗：超凡世界》客户端，而不是自己画的牌桌。这一层负责
把本机装的那份客户端变成可以安全使用的一份副本；它不实现任何游戏规则，也不修改
Steam 的安装目录。

```
wbo native probe    # 找客户端、核对构建、报告能不能用
wbo native import   # 复制成工作目录里的副本（--dry-run 只看计划）
```

## 为什么以哈希为准，而不是版本号

线协议契约（路由、DTO 字段布局、内存偏移）全部提取自某个具体构建的
`GameAssembly.dll`。所以"支持哪些客户端"只有一个判据：这个文件的 SHA-256。
`gameVersion` 只是给人看的标签，任何判定都不依赖它。

profile 在 `internal/nativeclient/profiles/`，一条对应一个受支持的构建。改游戏或
换版本之后，要么重新提取契约并新增一条 profile，要么明确报告"不支持"。

核对结果分两级：

| 级别 | 含义 | 用途 |
| --- | --- | --- |
| `Supported` | `GameAssembly.dll` 一致 | 契约有效，可以放行 |
| `Identical` | 登记的文件全部逐字节一致 | 提示用户"这份客户端被改动过" |

分两级是必要的：实测发现本机客户端的
`ShadowverseWB_Data/Plugins/x86_64/Cysharp.Net.Http.YetAnotherHttpHandler.Native.dll`
与契约提取时的那份不同（长度一样、内容不同），而它并不影响契约有效性。若只认
"全部一致"，这类客户端会被无谓地拒绝。

## 已经确认的事实

以 2026-09 本机（Linux + Proton 11.0-100）实测为准：

- 客户端目录 **27 GiB、38372 个文件**。Steam 清单 `appmanifest_2584990.acf` 里的
  `SizeOnDisk` 写的是 1.29 GB，与实际情况差一个数量级，**不能**用来做空间预判。
- 客户端是 Windows 构建，由 Proton 运行；`steamapps/compatdata/<appid>/pfx` 存在
  就说明是 Proton，这是我们判断隔离方式的唯一依据——不能靠操作系统推断。
- `~/.steam/root` 与 `~/.steam/steam` 都是指向 `~/.local/share/Steam` 的软链接，
  同一份安装会以多个路径出现。发现逻辑按真实路径去重，否则 `import` 会把同一份
  客户端误判成"有多份"而拒绝执行。

## 安全边界

- **Steam 的安装目录永远只读**，补丁只写副本。这样既不影响 Steam 校验游戏完整性，
  也让"卸载 = 删目录"成立。
- 目标是**整棵目录树**的复制，不只是 profile 登记的那些文件——客户端启动还需要
  大量没有逐个登记的资源。
- 复制之后**重新核对目的地**，而不是相信复制过程本身。
- 目标目录与源目录互为父子时直接拒绝；源目录含符号链接时拒绝（跟随后会读到目录之外）。
- 文件先写 `.wbo-import` 临时名再原子替换，中断不会留下半个 `GameAssembly.dll`。

## 已知问题

1. **27 GiB 的全量复制不可接受**，这是当前 `import` 最大的问题。三个方向：
   reflink（btrfs/XFS 上近乎免费）、按文件符号链接 + 只真复制被改的文件、
   或 Linux 侧直接用挂载命名空间把 Steam 目录只读挂进沙箱、再覆盖一个打了补丁的
   `GameAssembly.dll`（不需要复制）。Windows 上没有等价的单文件覆盖手段，需要单独设计。
2. 客户端改造走的是**代理 DLL**，不改 `GameAssembly.dll`（见下一节）。它已在 Arch 上
   用 mingw-w64 交叉编译出 Windows 产物并核对过导出表，但还没有在真实客户端上跑过。
3. 每次核对都会读取全部登记文件（其中一个是 195 MiB）。安装时无所谓，若之后要在
   每次启动都核对，需要按 (路径, 大小, mtime) 做缓存。
4. 本机根分区是 ext4，**没有 reflink**（`cp --reflink=always` 直接失败），所以
   "近乎免费的副本"这条在 Linux 上不成立。要么老老实实复制 27 GiB，要么走挂载
   命名空间覆盖（不需要复制）。Windows 侧同理，需要单独设计。

## 客户端怎么接到本地

原版客户端的所有服务端调用都走一个独立插件：

```
ShadowverseWB_Data/Plugins/x86_64/Cysharp.Net.Http.YetAnotherHttpHandler.Native.dll
```

它是上游 Cysharp/YetAnotherHttpHandler 的 Rust cdylib，导出 45 个 C ABI 函数。所以
客户端改造不需要动 195 MiB 的 `GameAssembly.dll`，只要把这个插件换成
[`native/yaha-shim`](../../native/yaha-shim/README.md)：44 个导出原样转发给改名后的
原件，只接管 `yaha_request_set_uri`，在那里把 URI 改写到本地。

```
客户端 → yaha-shim（改 URI）→ yaha_orig（TLS/HTTP2/连接池照旧）→ 本地 broker
```

这么做顺带解决了两件事：**不需要知道客户端原本连哪个域名**（运行时组装的，二进制和
资源里都没有明文），以及**不需要处理证书**——把 `https` 改写成 `http` 就不走 TLS 了。
改写前的原始 URI 会记进 `yaha-shim.log`，第一次运行就能把真实地址摸出来。

对应地，本地这一侧是 `wbo native broker`：

```bash
wbo native broker --listen 127.0.0.1:50172 --capture /tmp/wbo-capture
```

它说的是 **h2c**（明文 HTTP/2 先验知识）：客户端被改写成 `http://` 之后仍然按 HTTP/2
直连，没有 TLS 握手也没有 Upgrade 协商，所以服务端不能是普通 HTTP/1.1。当前所有路由
都返回 501，它唯一的职责是把每个请求（方法、路径、头、**原始请求体**）完整录下来。
先有真实流量，再决定实现哪条路由。

## 启动：用户不需要动 Steam 启动选项

把代理装进游戏同样不需要动磁盘，也不需要用户去 Steam 里写启动项。一条命令：

```bash
wbo native launch --broker --capture ~/wbo-capture
```

`wbo native launch` 自己把这件事做完：

1. 按 Steam 库发现客户端，用 profile 核对 `GameAssembly.dll` 的 SHA-256（不通过就
   不启动，而不是让它去撞一堵看不懂的墙）；
2. 推出这份客户端该用哪个 Proton、哪个 Steam Linux Runtime；
3. 在 bubblewrap 的挂载命名空间里把代理 DLL 盖到插件位置上（**Steam 目录一个字节
   都不改**）；
4. 生成会话密钥、把它们写进现场生成的代理配置，按需起 `native broker`；
5. 执行**自己拼出来的**命令行——和 Steam 在没有启动选项时给这个游戏拼的那条一模一样。

换句话说：Steam 在这条路上只被**只读**地读了两次——客户端装在哪、用的是哪个兼容工具。
要启动游戏也不需要在 Steam 界面里点，不存在"先设置启动项再启动"这一步。

### 怎么推出 Proton 与运行时（都不是猜的）

以 2026-09 本机实测为准，Steam 给这个游戏拼出的命令行是（从 `/proc/<pid>/cmdline`
读出来的，不是推的）：

```
<steam>/ubuntu12_32/reaper SteamLaunch AppId=2584990 -- \
<steam>/steamapps/common/SteamLinuxRuntime_4/_v2-entry-point --verb=waitforexitandrun -- \
<steam>/steamapps/common/Proton - Experimental/proton waitforexitandrun <客户端>/ShadowverseWB.exe
```

我们复现它用到的每一条依据都能追到文件：

| 要素 | 依据 |
| --- | --- |
| 客户端目录 | Steam 库 + `appmanifest_2584990.acf` 的 `installdir` |
| 用哪个 Proton | 先看 `config/config.vdf` 里 `CompatToolMapping` 给这个 app 的显式指定；没有就按 `compatdata/2584990/config_info` 里记的版本反推 |
| 用哪个运行时 | 由选中的 Proton 在 `toolmanifest.vdf` 里声明的 `require_tool_appid`，再用对应 `appmanifest_<appid>.acf` 的 `installdir` 换出目录名 |
| 环境变量 | 拿一个由 Steam 正常启动的游戏进程，读 `/proc/<pid>/environ` 对照出来的 |

反推不中时**不会**自作主张挑一个：列出候选让人用 `--proton` 指定。挑错的后果是
游戏起不来，而错误信息里看不出这件事跟 Proton 有关。

已知行为：Proton 用 `compatdata/<appid>/pfx.lock` 把同一个前缀的实例串行化。已经有一份
游戏在跑时，`native launch` 会**等它退出**再启动，不会和它抢前缀。

给 `--` 后面跟一条现成命令的形式仍然保留（调试用），那时环境变量原样继承。

这条路依赖 bubblewrap（挂载命名空间），所以目前只在 Linux/Proton 成立，并且要求
客户端由 Proton 运行——本机是 ext4（没有 reflink），overlayfs 也没加载，所以
"复制一份客户端"和"用 overlay 合并"都不划算，直接在命名空间里换一个文件是最省的。
会话目录与 Steam 库不在同一个文件系统时（硬链接会以 EXDEV 失败），替身目录会自动
退回真复制，最坏情况多复制的是 `GameAssembly.dll` 那一份大文件，不是整棵 27 GiB。

## 请求是加密的：信封这一层

客户端发给"服务器"的报文体不是裸的 MessagePack，而是套了一层信封：

```
base64( [u32 小端 载荷长度][32B 客户端临时公钥][36B AES-CTR 元数据][16B GCM tag][AES-GCM 载荷] )
```

密钥来自 X25519：客户端拿自己的临时私钥，跟一个**写在客户端里的静态公钥**协商。
所以服务端要能解开，前提是客户端用的是**我们的**公钥——这需要把客户端里那个常量
换掉（见 `delta/tools/patch_common_header.py`；常量源头在元数据里，不是就地构造的数组）。

`internal/nativeproto` 是这条信封的服务端实现：`DecodeRequest` 解请求、`EncodeResponse`
封响应。验证分三层，因为"自己写的编解码器两边对拍"这种测试**抓不住两边同错的 bug**：

| 层 | 内容 |
| --- | --- |
| 原版 oracle | 客户端方向的输出必须逐字节等于**原版 DLL 实测向量**（WBArts 的 `codec_test.go` 提供的输入/输出对） |
| 独立参考编码器 | 服务端要用**另一份实现**（照 WBArts `codec.go` 重排的参考编码器）造的请求对拍，不共用生产代码里的 KDF |
| 反向对照 | 客户端没换公钥时服务端解不开——这个前提本身也要成立 |

第一层不是摆设：`mix()` 里 `mixed[i]` 被写成 `mixed[0]` 这个错，只有它抓得到。
当时的表现极具误导性——元数据解得开（那一步只用到 `shared`），载荷一律"认证失败"，
看起来像"客户端没用我们的公钥"，害得排查方向整个偏了一轮。这类问题现在在
`DecodeRequest` 的报错里被直接分开：元数据只依赖 `shared`，所以"元数据里的凭据对得上"
就等价于"共享密钥是对的"，那时该去查 KDF，而不是去查公钥改写。

日常用 `wbo native launch` 就够了，它会**自己**把这次启动生成的会话私钥喂给 broker；
缺的只有客户端凭据：

```bash
wbo native launch --broker --capture ~/wbo-capture \
    --uuid 4466cfd0-323b-4a25-80e1-7056accada88 \
    --auth-key 'cGfMEcF57uS+DpXzovgT9YK4TmdJQ3sF0YcEgxuOwYIn6ejXlCGn9q3gom1utK19qn8='
```

凭据（uuid / auth key）来自客户端存档，可用 WBArts 的 `tools/svwb-credentials` 提取；
`Sid` 从请求头取。直接跑 `wbo native broker` 也一样能工作（多给一个 `--session-key`），
那是调试用；三个凭据缺任何一个就只录制密文，仍然有用。解不开不算错误——客户端在
公钥改写生效**之前**发出去的那些请求（实测是每次启动最先来的 `CrashLog/send`）
本来就用的是原公钥，broker 会照常录制并如实报告是哪一种解不开。

换版本时 `--common-header` 要跟着更新（只有第 32..52 字节参与密钥派生）。

## 引导阶段的响应从哪来

客户端启动时会问一批与规则无关的问题（版本、标题、账号初始化……）。这些响应对不上，
它就走不到主界面；但它们也不需要规则引擎。这一层用**夹具**回答：
`internal/nativeproto/contracts/startup-fixtures.json`，来自 delta 的
`research/native-offline/startup-fixtures.json`，覆盖 15 条路由。

**来源要说清楚**：这份夹具的 `provenance` 自己写明是"本地重建的响应"——字段布局取自
原客户端的 MessagePack 序列化器与 DTO 注册、成功码取自客户端代码，**不是抓到的官方
响应**。`wbo native broker` 启动时会把这句话原样打出来，不让人误以为是官方数据。

`wbo native broker` 一旦启用解密，就会同时挂上夹具处理器：

- 命中夹具的路由 → 编成 MessagePack → 用**本次会话**的密钥封成信封 → base64 → 200；
- 没有会话（没解开报文）或路由不在夹具里 → 退回未实现（501），不凭空造响应。

注意一个实现细节：夹具是 JSON，读的时候用 `UseNumber` 防止大整数被折成浮点，
但编码成 MessagePack 之前必须把 `json.Number` 换回具体数值类型——它本质是字符串，
直接编码出去会变成 `"1"` 而不是 `1`，客户端解析就会失败。

## 客户端现在走到哪一步（1.9.5 实测）

在标题界面点"点击开始游戏"之后，客户端**按顺序**问这些（broker 日志里有原样记录）：

| 顺序 | 路由 | 我们的回应 |
| --- | --- | --- |
| 1 | `/Version/info` | 夹具 |
| 2 | `/Session/start` | 夹具 |
| 3 | `/StartUp/index` | 夹具 |
| 4 | `/Load/index` | **档案层**（带玩家收藏）+ 夹具 |
| 5 | `/User/getIsAllowSendAdjust` | 夹具 |
| 6 | `/Account/getGameStartCountryAgeGroup` | 夹具 |
| 7 | `/StartUp/index`（第二次） | 夹具 |
| 8 | `/User/updateIsAllowSendThirdParty` | 夹具 |

八条全部 200，客户端没有报错——然后它弹一个 **"推荐进行账号关联"** 对话框，问题就卡在
这里：**还没有看到 `/Mypage/index`**，也就是还没进主界面。这是当前唯一的拦路石。

关于这个对话框，已经排除的可能：

- **不是输入问题**：鼠标与键盘都能到对话框上（Esc 能弹出"结束游戏"确认框，点它的
  "取消"也生效），点"稍后设置"确实会让客户端再跑一遍登录链；
- **不是漏发的请求**：代理现在会把没改写的 URI 也记一行（`pass-through …`），
  实测客户端发的每一条都改写到了本机，没有偷偷发给真服务器；
- **不是凭据或版本对不上**：客户端本地存档（`SaveData.db`，只是按固定掩码 XOR，
  WBArts 的 `tools/svwb-credentials` 里有两个掩码常量）里的 217 项与我们的夹具一致
  ——`TextLanguage=4`、`CountryId=344`、`IsTutorialComplete=True`、`SendUseInfo=False`
  都对得上。

所以拦路的是**登录流程本身的状态机**：客户端认定这个账号"没做过账号关联"，于是每次
登录完都推荐一次，而"稍后设置"之后它又回到标题重来。下一步要查的是它凭什么判定
"没关联"——本地存档里没有这个标志，那么要么在某个响应字段里，要么是一条我们还没
实现的查询路由。

顺带记两条实测到的客户端错误码含义（截图里会显示"错误代码: N"）：

| 代码 | 实测触发条件 |
| --- | --- |
| 2 | 连不上 broker（本机实测：端口被上一轮的 broker 占着，新的没起来） |
| 3 | 响应体不是 base64 文本（本档第一版档案处理器直接回了二进制信封） |

调试时有两件趁手的工具，都不是产品的一部分：`wbo native broker` 会把解开的请求体
打印成一行 JSON（`internal/nativebroker/describe.go`）；驱动客户端不需要人坐在那儿
——XTEST 合成点击能点动标题界面（注意 `ButtonRelease` 的 detail 必须是按钮号，
写 0 会让客户端只看到"一直按着"）。但**这个对话框上的按钮，合成点击点不动**，
所以这一步仍然需要真人点一次。
