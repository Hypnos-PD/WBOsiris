# 构建 WBOsiris 的 Windows 产物（.exe + NSIS 安装器）。
# 需要在 Windows 上运行：wails 的 Windows 目标依赖 WebView2 加载器与 NSIS。
#
#   pwsh -File scripts/release/build-windows.ps1
#
param(
  [string]$Version = "",
  [switch]$SkipBuild
)
$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
Set-Location $root

if (-not $Version) {
  $Version = (git describe --tags --always --dirty 2>$null)
  if (-not $Version) { $Version = "dev" }
}

$binary = Join-Path $root "desktop\build\bin\WBOsiris.exe"
if (-not $SkipBuild) {
  Push-Location (Join-Path $root "desktop")
  try {
    wails build -platform windows/amd64 -nsis -ldflags "-X main.version=$Version"
  } finally {
    Pop-Location
  }
}
if (-not (Test-Path $binary)) { throw "找不到 $binary" }

$outDir = Join-Path $root "dist"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$target = Join-Path $outDir "WBOsiris-$Version-windows-amd64.exe"
Copy-Item -Force $binary $target

# 主界面素材放在 exe 旁的 assets\：客户端默认就从这里读（不塞进二进制）。
$assetsSource = Join-Path $root "web\public\assets\home"
if (Test-Path $assetsSource) {
  $assetsTarget = Join-Path $outDir "assets\home"
  if (Test-Path $assetsTarget) { Remove-Item -Recurse -Force $assetsTarget }
  New-Item -ItemType Directory -Force -Path (Split-Path $assetsTarget) | Out-Null
  Copy-Item -Recurse -Force $assetsSource $assetsTarget
  Write-Host "主界面素材已复制到 $(Split-Path $assetsTarget)"
}

Get-ChildItem -Path (Join-Path $root "desktop\build\bin") -Filter "*.exe" | ForEach-Object {
  Write-Host ("构建产物: {0} ({1:N1} MB)" -f $_.FullName, ($_.Length / 1MB))
}
Write-Host "完成：$target"
