#!/usr/bin/env bash
# 构建 WBOsiris 的 Linux AppImage，并做与 WBCapture 相同的预处理。
#
#   scripts/release/build-appimage.sh                 # 完整包（含全部主界面素材）
#   scripts/release/build-appimage.sh --slim          # 不带素材（体积小，素材走线上）
#   scripts/release/build-appimage.sh --skip-build    # 复用已有二进制
#
# 预处理三步（照抄 WBCapture 的 scripts/release/repack-appimage.sh 思路）：
#   1. GTK AppRun hook 里 export GDK_BACKEND=x11 → ${GDK_BACKEND:-wayland,x11}
#   2. 删掉被捆进 AppDir 的 libwayland-*.so*，改用宿主的 Wayland ABI
#   3. 用 linuxdeploy-plugin-appimage 重新打包，覆盖原文件
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
cd "$root"

version=${WBO_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
out_dir=${WBO_OUT_DIR:-$root/dist}
tools_dir=${WBO_TOOLS_DIR:-$root/.cache/wbo-release-tools}
slim=0
skip_build=0
for arg in "$@"; do
  case "$arg" in
    --slim) slim=1 ;;
    --skip-build) skip_build=1 ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) printf '未知参数: %s\n' "$arg" >&2; exit 2 ;;
  esac
done

binary="$root/desktop/build/bin/WBOsiris"
if ((!skip_build)); then
  # 用仓库固定的 Wails CLI；wails.json 里的 frontend:build 会先构建并同步 web/dist。
  "$root/scripts/wails.sh" build -ldflags "-X main.version=$version"
fi
[[ -x "$binary" ]] || { printf '找不到二进制：%s\n' "$binary" >&2; exit 1; }

mkdir -p "$tools_dir" "$out_dir"
download() {
  local url=$1 target=$2
  if [[ ! -x "$target" ]]; then
    printf '下载 %s\n' "$url"
    curl --fail --location --silent --show-error --output "$target" "$url"
    chmod +x "$target"
  fi
}
linuxdeploy="$tools_dir/linuxdeploy-x86_64.AppImage"
appimage_plugin="$tools_dir/linuxdeploy-plugin-appimage.AppImage"
gtk_plugin="$tools_dir/linuxdeploy-plugin-gtk.sh"
download https://github.com/linuxdeploy/linuxdeploy/releases/download/continuous/linuxdeploy-x86_64.AppImage "$linuxdeploy"
download https://github.com/linuxdeploy/linuxdeploy-plugin-appimage/releases/download/continuous/linuxdeploy-plugin-appimage.AppImage "$appimage_plugin"
if [[ ! -x "$gtk_plugin" ]]; then
  curl --fail --location --silent --show-error --output "$gtk_plugin" \
    https://raw.githubusercontent.com/linuxdeploy/linuxdeploy-plugin-gtk/master/linuxdeploy-plugin-gtk.sh
  chmod +x "$gtk_plugin"
fi

appdir="$out_dir/WBOsiris.AppDir"
rm -rf "$appdir"
mkdir -p "$appdir/usr/bin" "$appdir/usr/share/applications" "$appdir/usr/share/icons/hicolor/512x512/apps"
install -m 0755 "$binary" "$appdir/usr/bin/WBOsiris"
install -m 0644 "$root/desktop/build/appicon.png" "$appdir/usr/share/icons/hicolor/512x512/apps/WBOsiris.png"

if ((!slim)); then
  printf '收录主界面素材（约 675 MB）…\n'
  mkdir -p "$appdir/usr/share/WBOsiris"
  cp -a "$root/web/public/assets/home" "$appdir/usr/share/WBOsiris/assets"
fi

cat > "$appdir/usr/share/applications/WBOsiris.desktop" <<'DESKTOP'
[Desktop Entry]
Type=Application
Name=WBOsiris
Comment=影之诗：超凡世界 规则沙盒与对战客户端
Exec=WBOsiris
Icon=WBOsiris
Categories=Game;CardGame;
Terminal=false
DESKTOP

cat > "$appdir/AppRun" <<'APPRUN'
#!/bin/sh
here=$(dirname "$(readlink -f "$0")")
# 素材目录（打包时放在 usr/share/WBOsiris/assets；slim 包里不存在，客户端会退回内嵌默认立绘）
assets="$here/usr/share/WBOsiris/assets"
if [ -d "$assets" ]; then
  exec "$here/usr/bin/WBOsiris" --assets-dir "$assets" "$@"
fi
exec "$here/usr/bin/WBOsiris" "$@"
APPRUN
chmod +x "$appdir/AppRun"

printf '用 linuxdeploy 打包（含 GTK/WebKit 依赖）…\n'
env APPIMAGE_EXTRACT_AND_RUN=1 \
    LINUXDEPLOY_PLUGIN_GTK="$gtk_plugin" \
    OUTPUT="WBOsiris-${version}-x86_64.AppImage" \
    "$linuxdeploy" --appdir "$appdir" --plugin gtk --output appimage

appimage=$(ls -1 "$out_dir"/WBOsiris-*-x86_64.AppImage 2>/dev/null | head -1)
[[ -n "$appimage" ]] || appimage="$root/WBOsiris-${version}-x86_64.AppImage"
[[ -f "$appimage" ]] || { printf 'linuxdeploy 没有产出 AppImage\n' >&2; exit 1; }

# ── 预处理：Wayland ABI + GTK 后端（与 WBCapture 的 repack-appimage.sh 同一套）──
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
(
  cd "$work"
  "$appimage" --appimage-extract >/dev/null
)
extracted="$work/squashfs-root"
gtk_hook="$extracted/apprun-hooks/linuxdeploy-plugin-gtk.sh"
if [[ -f "$gtk_hook" ]]; then
  sed -i 's|^export GDK_BACKEND=x11.*$|export GDK_BACKEND="${GDK_BACKEND:-wayland,x11}"|' "$gtk_hook"
  grep -Fqx 'export GDK_BACKEND="${GDK_BACKEND:-wayland,x11}"' "$gtk_hook" || {
    printf '未能改写 GTK 后端 hook\n' >&2; exit 1; }
else
  printf '提示：没有找到 GTK hook（%s），跳过 GDK_BACKEND 改写\n' "$gtk_hook" >&2
fi
mapfile -d '' bundled_wayland < <(find "$extracted/usr" \( -type f -o -type l \) -name 'libwayland-*.so*' -print0)
if [[ ${#bundled_wayland[@]} -gt 0 ]]; then
  printf '移除随包携带的 Wayland 库：%s\n' "${bundled_wayland[@]#$extracted/}"
  find "$extracted/usr" \( -type f -o -type l \) -name 'libwayland-*.so*' -delete
fi

repacked="$appimage.repacked"
env APPIMAGE_EXTRACT_AND_RUN=1 ARCH=x86_64 LDAI_OUTPUT="$repacked" OUTPUT="$repacked" \
  "$appimage_plugin" --appimage-extract-and-run --appdir="$extracted"
[[ -s "$repacked" ]] || { printf '重新打包失败\n' >&2; exit 1; }
chmod +x "$repacked"
mv -f "$repacked" "$appimage"
rm -f "$appimage.sig"

printf '完成：%s\n' "$appimage"
