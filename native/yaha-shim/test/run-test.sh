#!/usr/bin/env bash
# 在真实 Windows 环境里验证代理 DLL，不需要游戏、不需要 Steam。
#
# 用 Proton 的前缀跑一个最小控制台程序：它加载代理、解析 9 个符号（其中 8 个是
# 转发，能解析就说明 yaha_orig.dll 被加载且目标存在），再走一遍
# init → build_client → request_new → set_uri。
#
# 用法：
#   native/yaha-shim/test/run-test.sh
# 可用环境变量覆盖：
#   YAHA_ORIGINAL  出厂 DLL（默认取本机 Steam 安装里的那份）
#   YAHA_SHIM      构建产物（默认 target/x86_64-pc-windows-gnu/release/yaha_shim.dll）
#   PROTON         proton 可执行文件
#   RUN_IN_SNIPER  SteamLinuxRuntime 的入口
#   COMPAT_DATA    Proton 前缀，默认借游戏自己的
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SHIM_ROOT="$(cd "$HERE/.." && pwd)"
STEAM="${STEAM_ROOT:-$HOME/.local/share/Steam}"
GAME="$STEAM/steamapps/common/ShadowverseWB/ShadowverseWB_Data/Plugins/x86_64"

YAHA_ORIGINAL="${YAHA_ORIGINAL:-$GAME/Cysharp.Net.Http.YetAnotherHttpHandler.Native.dll}"
YAHA_SHIM="${YAHA_SHIM:-$SHIM_ROOT/target/x86_64-pc-windows-gnu/release/yaha_shim.dll}"
PROTON="${PROTON:-$STEAM/steamapps/common/Proton - Experimental/proton}"
RUN_IN_SNIPER="${RUN_IN_SNIPER:-$STEAM/steamapps/common/SteamLinuxRuntime_sniper/run-in-sniper}"
COMPAT_DATA="${COMPAT_DATA:-$STEAM/steamapps/compatdata/2584990}"

for required in "$YAHA_ORIGINAL" "$YAHA_SHIM" "$PROTON" "$RUN_IN_SNIPER"; do
    if [ ! -e "$required" ]; then
        echo "找不到 $required" >&2
        exit 1
    fi
done

WORK="$(mktemp -d)"
trap 'echo "工作目录保留在 $WORK"' EXIT

# 布局和游戏里一模一样：代理用回原名，出厂 DLL 改名成 yaha_orig.dll 放在旁边。
cp "$YAHA_ORIGINAL" "$WORK/yaha_orig.dll"
cp "$YAHA_SHIM" "$WORK/Cysharp.Net.Http.YetAnotherHttpHandler.Native.dll"
printf 'local = http://127.0.0.1:50172\nlog = yaha-shim.log\n' > "$WORK/yaha-shim.conf"
x86_64-w64-mingw32-gcc -O1 -o "$WORK/yahatest.exe" "$HERE/yahatest.c"

cd "$WORK"
# Proton 把控制台输出吞掉，结果由 yahatest 写进 yahatest-result.txt。
STEAM_COMPAT_CLIENT_INSTALL_PATH="$STEAM" STEAM_COMPAT_DATA_PATH="$COMPAT_DATA" \
    timeout 300 "$RUN_IN_SNIPER" -- "$PROTON" run "$WORK/yahatest.exe" >/dev/null 2>&1 || true

echo "== 测试结果 =="
cat "$WORK/yahatest-result.txt" 2>/dev/null || { echo "没有结果文件，测试没有跑起来" >&2; exit 1; }
echo "== 代理日志 =="
cat "$WORK/yaha-shim.log" 2>/dev/null || echo "（没有日志，说明改写没有发生）"

grep -q '结论：全部通过' "$WORK/yahatest-result.txt"
