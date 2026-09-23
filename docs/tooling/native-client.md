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

把代理装进游戏同样不需要动磁盘，办法是 `wbo native launch`：把它写进 Steam 的
启动选项，它会在挂载命名空间里把代理盖到插件位置上，再执行 Steam 给的原始命令。

```
/path/to/wbo native launch --broker --capture ~/wbo-capture -- %command%
```

这条路依赖 bubblewrap（挂载命名空间），所以目前只在 Linux/Proton 成立，并且要求
客户端由 Proton 运行——本机是 ext4（没有 reflink），overlayfs 也没加载，所以
"复制一份客户端"和"用 overlay 合并"都不划算，直接在命名空间里换一个文件是最省的。

## 请求是加密的：信封这一层

客户端发给"服务器"的报文体不是裸的 MessagePack，而是套了一层信封：

```
base64( [u32 小端 载荷长度][32B 客户端临时公钥][36B AES-CTR 元数据][16B GCM tag][AES-GCM 载荷] )
```

密钥来自 X25519：客户端拿自己的临时私钥，跟一个**写在客户端里的静态公钥**协商。
所以服务端要能解开，前提是客户端用的是**我们的**公钥——这需要把客户端里那个常量
换掉（见 `delta/tools/patch_common_header.py`；常量源头在元数据里，不是就地构造的数组）。

`internal/nativeproto` 是这条信封的服务端实现：`DecodeRequest` 解请求、`EncodeResponse`
封响应。它不依赖客户端在场，测试自带一对密钥与一条独立实现的客户端方向做双向对拍；
另外有一条反向对照，确认"客户端没换公钥时服务端解不开"这个前提成立。

启用方式（`wbo native broker`）：

```bash
wbo native broker --capture /tmp/cap \
    --session-key ~/verify/session-key \
    --uuid 4466cfd0-323b-4a25-80e1-7056accada88 \
    --auth-key 'cGfMEcF57uS+DpXzovgT9YK4TmdJQ3sF0YcEgxuOwYIn6ejXlCGn9q3gom1utK19qn8='
```

三个都给了才会尝试解密；缺任何一个就只录制密文（仍然有用）。解不开不算错误——
那通常意味着这一版客户端还没换成我们的公钥，broker 会照常录制与回应。

凭据（uuid / auth key）来自客户端存档，可用 WBArts 的 `tools/svwb-credentials` 提取；
`Sid` 从请求头取。换版本时 `--common-header` 要跟着更新（只有第 32..52 字节参与密钥派生）。
