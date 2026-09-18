#!/usr/bin/env bash
# 用仓库固定的 Wails CLI（go.mod 里的 tool 依赖）执行 wails 命令，
# 这样不需要全局安装、也不会和别人的版本不一致：
#
#   scripts/wails.sh dev
#   scripts/wails.sh build -ldflags "-X main.version=dev"
#
# Linux 上会自动挑 WebKitGTK 版本标签：只有 4.1 的系统（Arch、新版 Debian/Ubuntu）
# 需要 -tags webkit2_41，而 Wails 默认按 4.0 编译。
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

args=("$@")
needs_tag=0
for arg in "$@"; do
  case "$arg" in
    dev|build) needs_tag=1 ;;
    -tags|--tags) needs_tag=0; break ;;
  esac
done
if ((needs_tag)) && pkg-config --exists webkit2gtk-4.1 2>/dev/null && ! pkg-config --exists webkit2gtk-4.0 2>/dev/null; then
  args+=(-tags webkit2_41)
fi

cd desktop
exec go tool wails "${args[@]}"
