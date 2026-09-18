#!/usr/bin/env bash
# 把 WBOsiris 规则服务部署到 WBA 主机（与 WBArts 的 deploy-backend.sh 同一套形态：
# 本地构建 → 上传 → 原子替换 → 重启 → 健康检查，失败自动回滚）。
#
#   scripts/deploy-backend.sh                  # 默认部署到 rain-1
#   scripts/deploy-backend.sh --host rain-1 --dry-run
#
# 传输用 ssh/scp，不需要服务器上有 Git 凭据；CI 里用同一脚本 + SSH key 即可。
set -euo pipefail

host=${DEPLOY_HOST:-rain-1}
service=${DEPLOY_SERVICE:-wbosiris.service}
remote_dir=${DEPLOY_DIR:-/opt/1panel/www/sites/sva.hypd.asia/wbosiris}
local_port=${DEPLOY_LOCAL_PORT:-23216}
health_url=${DEPLOY_HEALTH_URL:-http://127.0.0.1:23216/api/health}
public_url=${DEPLOY_PUBLIC_URL:-https://sva.hypd.asia/wbo/api/health}
dry_run=0
skip_tests=0

usage() {
  cat <<'EOF'
Usage: scripts/deploy-backend.sh [options]

Builds cmd/wbo for linux/amd64, ships it with cards/ and tests/ to the WBA host,
swaps the binary atomically, restarts wbosiris.service and checks /api/health.

Options:
  --host HOST       SSH host (default: rain-1)
  --dir PATH        Remote install directory
  --service NAME    systemd unit (default: wbosiris.service)
  --health-url URL  Local health endpoint on the host
  --public-url URL  Public health endpoint (through nginx)
  --skip-tests      Skip go test ./... before building
  --dry-run         Show the plan without changing the host
  -h, --help        Show this help
EOF
}

while (($#)); do
  case "$1" in
    --host) host=${2:?missing value for --host}; shift 2 ;;
    --dir) remote_dir=${2:?missing value for --dir}; shift 2 ;;
    --service) service=${2:?missing value for --service}; shift 2 ;;
    --health-url) health_url=${2:?missing value for --health-url}; shift 2 ;;
    --public-url) public_url=${2:?missing value for --public-url}; shift 2 ;;
    --skip-tests) skip_tests=1; shift ;;
    --dry-run) dry_run=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'Unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

root=$(git -C "$(dirname "$0")/.." rev-parse --show-toplevel)
cd "$root"
commit=$(git rev-parse --short HEAD)
dirty=$(git status --porcelain | head -5 || true)
if [[ -n "$dirty" ]]; then
  printf 'Warning: working tree has local changes:\n%s\n' "$dirty" >&2
fi

if ((dry_run)); then
  printf 'Plan: host=%s dir=%s service=%s commit=%s\n' "$host" "$remote_dir" "$service" "$commit"
  printf 'Would run: GOOS=linux GOARCH=amd64 go build -o build/wbosiris-server ./cmd/wbo\n'
  printf 'Would upload: binary + cards/ + tests/ -> %s\n' "$remote_dir"
  printf 'Would restart %s and check %s / %s\n' "$service" "$health_url" "$public_url"
  exit 0
fi

if ((!skip_tests)); then
  go test ./... >/dev/null
fi
mkdir -p build
GOOS=linux GOARCH=amd64 go build -o build/wbosiris-server ./cmd/wbo

printf 'Uploading to %s:%s (%s)\n' "$host" "$remote_dir" "$commit"
ssh -- "$host" "set -euo pipefail; mkdir -p '$remote_dir'"
scp -q build/wbosiris-server "$host:$remote_dir/wbosiris-server.new"
tar -C . -cf - cards tests | ssh -- "$host" "set -euo pipefail; mkdir -p '$remote_dir'; tar -C '$remote_dir' -xf -"

ssh -- "$host" "
set -euo pipefail
dir='$remote_dir'
service='$service'
backup=\"\$dir/wbosiris-server.rollback\"
had=0
restore() {
  if ((had)); then mv -f \"\$backup\" \"\$dir/wbosiris-server\"; fi
  systemctl restart \"\$service\" >/dev/null 2>&1 || true
}
if [[ -f \"\$dir/wbosiris-server\" ]]; then cp -p \"\$dir/wbosiris-server\" \"\$backup\"; had=1; fi
install -m 0755 \"\$dir/wbosiris-server.new\" \"\$dir/wbosiris-server\"
if ! systemctl restart \"\$service\" || ! systemctl is-active --quiet \"\$service\"; then
  echo 'restart failed; rolling back' >&2
  restore
  exit 1
fi
if ! curl --fail --silent --retry 10 --retry-connrefused --retry-delay 1 --max-time 3 -- '$health_url' >/dev/null 2>&1; then
  echo "health check failed: $health_url; rolling back" >&2
  restore
  exit 1
fi
rm -f \"\$backup\"
"

if [[ -n "$public_url" ]]; then
  curl --fail --silent --show-error --max-time 15 -- "$public_url" >/dev/null
  printf 'Public health check passed: %s\n' "$public_url"
fi
printf 'Deployed %s to %s (%s)\n' "$commit" "$host" "$service"
