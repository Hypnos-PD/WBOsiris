#!/usr/bin/env python3
"""给客户的 GameAssembly.dll 打上"离线跑得起来"需要的最小补丁。

这是**改写客户端代码**，所以只对副本动手：`native import` 出来的工作目录，或者临时
放到 /tmp。永远不要指向 Steam 的安装目录（`--write-steam` 那类"明知故犯"开关这里
没有）。

四处补丁，都是本机实测出来的（1.9.5 / profile shadowverse-wb-steam-1.9.1.18238）：

| 文件偏移 | 方法 | 改成 | 为什么 |
| --- | --- | --- | --- |
| 0x6D270C0 | `TitleUtil.IsCreatedClientId()` | `xor eax,eax; ret` | 强制返回 false，跳过"已建号"分支，让登录链走 /Account/signUp |
| 0xA3B1A0 | `HttpEndPointGameAPI.CompressRequest(byte[])` | `mov rax,rdx; ret` | 请求不再做 LZ4 帧压缩（空载荷时它自己会崩），broker 直接收裸 MessagePack |
| 0xA3B6D0 | `HttpEndPointGameAPI.DecompressResponse(byte[])` | `mov rax,rcx; ret` | 同上，响应直通 |
| 0xBBD5A9 | `ManifestSynchronizer.SynchronizeManifestDB` 里的下载调用 | `xor eax,eax; nop×3` | 断掉"联网取清单"这一步；本地 provision 会负责把清单摆好 |

用法：
    patch-native-client.py <原版 GameAssembly.dll> <输出路径> [--check]

`--check` 只看这四处的现状，不改文件。
"""

import sys

# 偏移、原字节、替换字节、说明
PATCHES = [
    (0x6D270C0, bytes.fromhex("4883ec"), bytes.fromhex("31c0c3"), "IsCreatedClientId → false"),
    (0xA3B1A0, bytes.fromhex("40555357"), bytes.fromhex("488bc2c3"), "CompressRequest → 直通"),
    (0xA3B6D0, bytes.fromhex("40534883"), bytes.fromhex("488bc1c3"), "DecompressResponse → 直通"),
    (0xBBD5A9, bytes.fromhex("e8e2f5ffff"), bytes.fromhex("31c0909090"),
     "SynchronizeManifestDB 的清单下载调用 → 断掉"),
]


def main() -> int:
    if len(sys.argv) < 3:
        print(__doc__)
        return 2
    source, destination = sys.argv[1], sys.argv[2]
    check = "--check" in sys.argv[3:]
    raw = bytearray(open(source, "rb").read())
    for offset, expect, replacement, note in PATCHES:
        current = bytes(raw[offset:offset + len(expect)])
        if current == replacement:
            print(f"0x{offset:X} 已经是补过的（{note}）")
            continue
        if current != expect:
            print(f"0x{offset:X} 与预期不符（{note}）：期望 {expect.hex()}，实际 {current.hex()}")
            print("这份 dll 不是我们支持的那个构建，或者已经被别的补丁改过。")
            return 1
        print(f"0x{offset:X} {note}：{expect.hex()} → {replacement.hex()}")
        raw[offset:offset + len(replacement)] = replacement
    if check:
        print("（--check：没有写文件）")
        return 0
    with open(destination, "wb") as handle:
        handle.write(raw)
    print(f"已写出 {destination}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
