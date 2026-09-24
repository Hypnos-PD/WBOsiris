#!/usr/bin/env python3
"""把 delta 的 GameAssembly 补丁摊开看：补了哪些方法、替换成了什么。

用途：我们要在"不改客户端"和"改客户端"之间做决定，而 delta 已经走过后一条路。
这个脚本把它的补丁段映射回 dump 里的方法，并把 `jmp/call` 的目标换算成真正的
运行时地址（objdump 按文件偏移反汇编，直接看会以为是数据）。

用法：
    inspect-delta-patch.py list   <GameAssembly.delta> <dump.cs>
    inspect-delta-patch.py apply  <原版 GameAssembly.dll> <GameAssembly.delta> <输出路径>
    inspect-delta-patch.py code   <已打补丁的 dll> <文件偏移> [长度]

`apply` 只写到你指定的路径；**永远不要指向 Steam 的安装目录**。
"""

import bisect
import re
import struct
import subprocess
import sys


def load_segments(path: str):
    raw = open(path, "rb").read()
    if raw[:8] != b"SVWBDEL1":
        raise SystemExit("不是 delta 补丁格式")
    old_size, new_size, count = struct.unpack_from("<QQI", raw, 8)
    position = 28
    segments = []
    for _ in range(count):
        offset, length = struct.unpack_from("<QI", raw, position)
        position += 12
        if offset + length > new_size or position + length > len(raw):
            raise SystemExit("补丁段越界")
        segments.append((offset, length, raw[position:position + length]))
        position += length
    return segments, old_size, new_size


def load_methods(dump: str):
    methods = []
    pattern = re.compile(r"// RVA: 0x([0-9A-Fa-f]+) Offset: 0x([0-9A-Fa-f]+)")
    with open(dump, encoding="utf-8", errors="replace") as handle:
        previous = ""
        for line in handle:
            match = pattern.search(line)
            if match:
                methods.append((int(match.group(2), 16), previous.strip()))
                continue
            stripped = line.strip()
            if stripped and not stripped.startswith("//"):
                previous = stripped
    methods.sort()
    return methods


def load_pe(path: str):
    raw = open(path, "rb").read()
    pe_offset = struct.unpack_from("<I", raw, 0x3C)[0]
    if raw[pe_offset:pe_offset + 4] != b"PE\0\0":
        raise SystemExit("不是 PE 文件")
    coff = pe_offset + 4
    section_count = struct.unpack_from("<H", raw, coff + 2)[0]
    optional_size = struct.unpack_from("<H", raw, coff + 16)[0]
    optional = coff + 20
    magic = struct.unpack_from("<H", raw, optional)[0]
    image_base = struct.unpack_from("<Q", raw, optional + 24)[0] if magic == 0x20B \
        else struct.unpack_from("<I", raw, optional + 28)[0]
    table = optional + optional_size
    sections = []
    for index in range(section_count):
        entry = table + index * 40
        name = raw[entry:entry + 8].rstrip(b"\0").decode("utf-8", "replace")
        virtual_size, virtual_address = struct.unpack_from("<II", raw, entry + 8)
        raw_size, raw_pointer = struct.unpack_from("<II", raw, entry + 16)
        sections.append({"name": name, "va": virtual_address, "vsize": virtual_size,
                         "raw": raw_pointer, "raw_size": raw_size})
    return raw, image_base, sections


def resolve_jump(raw, sections, offset, length):
    """把一条 e8/e9 rel32 解析成运行时 RVA 与文件偏移。"""
    if raw[offset] not in (0xE8, 0xE9):
        return None
    rel32 = struct.unpack_from("<i", raw, offset + 1)[0]
    for section in sections:
        if section["raw"] <= offset < section["raw"] + section["raw_size"]:
            rva_next = section["va"] + (offset + length - section["raw"])
            target_rva = rva_next + rel32
            for candidate in sections:
                size = max(candidate["vsize"], candidate["raw_size"])
                if candidate["va"] <= target_rva < candidate["va"] + size:
                    return target_rva, candidate["raw"] + (target_rva - candidate["va"]), candidate["name"]
    return None


def command_list(delta_path: str, dump_path: str) -> int:
    segments, old_size, new_size = load_segments(delta_path)
    methods = load_methods(dump_path)
    offsets = [method[0] for method in methods]
    print(f"补丁段 {len(segments)} 个，文件 {old_size} → {new_size} 字节")
    for offset, length, data in segments:
        index = bisect.bisect_right(offsets, offset) - 1
        name = methods[index][1] if 0 <= index < len(methods) else "（不在任何方法内）"
        marker = ""
        if data[:1] in (b"\xe8", b"\xe9") and len(data) >= 5:
            marker = "  → 跳转/调用"
        print(f"  {offset:#010x} 长度 {length:>7}  {name[:88]}{marker}")
    return 0


def command_apply(base: str, delta_path: str, out_path: str) -> int:
    raw = open(base, "rb").read()
    segments, old_size, new_size = load_segments(delta_path)
    if len(raw) != old_size:
        raise SystemExit(f"原件大小不符：{len(raw)} != {old_size}")
    result = bytearray(raw)
    for offset, length, data in segments:
        if offset > len(result):
            result.extend(bytes(offset - len(result)))
        result[offset:offset + length] = data
    if len(result) != new_size:
        raise SystemExit("结果长度不对")
    open(out_path, "wb").write(result)
    import hashlib
    print("写出", out_path, len(result), "字节 sha256",
          hashlib.sha256(result).hexdigest())
    return 0


def command_code(dll: str, offset: int, length: int) -> int:
    raw, image_base, sections = load_pe(dll)
    for section in sections:
        if section["raw"] <= offset < section["raw"] + section["raw_size"]:
            print(f"{offset:#x} 在节 {section['name']}（VA {section['va']:#x}）")
            break
    resolved = resolve_jump(raw, sections, offset, 5)
    if resolved:
        target_rva, target_offset, name = resolved
        print(f"跳转目标 RVA {target_rva:#x} → 文件偏移 {target_offset:#x}（节 {name}）")
    output = subprocess.run(
        ["objdump", "-D", "-b", "binary", "-m", "i386:x86-64",
         f"--start-address={offset}", f"--stop-address={offset + length}", dll],
        capture_output=True, text=True)
    print("\n".join(output.stdout.splitlines()[7:]))
    return 0


def main() -> int:
    command = sys.argv[1]
    if command == "list":
        return command_list(sys.argv[2], sys.argv[3])
    if command == "apply":
        return command_apply(sys.argv[2], sys.argv[3], sys.argv[4])
    if command == "code":
        length = int(sys.argv[4]) if len(sys.argv) > 4 else 64
        return command_code(sys.argv[2], int(sys.argv[3], 16), length)
    raise SystemExit(__doc__)


if __name__ == "__main__":
    raise SystemExit(main())
