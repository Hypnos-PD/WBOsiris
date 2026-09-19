# 部署：规则服务跑在 WBA 主机上

规则服务（`cmd/wbo serve`）与 WBArts 同机部署，挂在该站点的 `/wbo/` 路径前缀下；
桌面客户端和线上调试页都通过这个前缀访问。账号沿用 WBArts 的 JWT，**不新建账号体系**。

## 拓扑（rain-1）

| 内容 | 位置 |
| --- | --- |
| 站点根（WBArts，静态 + 它的 `/api`） | `/opt/1panel/www/sites/sva.hypd.asia/index` |
| 本服务目录（二进制 + `cards/` + `tests/`） | `/opt/1panel/www/sites/sva.hypd.asia/wbosiris` |
| systemd | `/etc/systemd/system/wbosiris.service` |
| OpenResty 反代片段 | `/opt/1panel/www/sites/sva.hypd.asia/proxy/wbosiris.conf` |
| 监听 | `127.0.0.1:23216`（WBArts 后端是 `127.0.0.1:4864`，不要混用） |
| 公开地址 | `https://sva.hypd.asia/wbo/api/health` |

vhost 里已经有 `include /www/sites/sva.hypd.asia/proxy/*.conf;`，所以**新增片段不用改 vhost**：

```bash
scp deploy/nginx-wbosiris.conf rain-1:/opt/1panel/www/sites/sva.hypd.asia/proxy/wbosiris.conf
ssh rain-1 'docker exec 1Panel-openresty-WUf4 nginx -t && docker exec 1Panel-openresty-WUf4 nginx -s reload'
```

反代片段里必须保留两件事：Cloudflare 的真实 IP 头（七个 `proxy_set_header`，供限流使用）
与 `proxy_buffering off; proxy_read_timeout 1h;`（对局状态用 SSE 推送，服务端每 15 秒心跳）。

## 部署

```bash
scripts/deploy-backend.sh              # 本地 go test + 构建 → 上传 → 原子替换 → 重启 → 健康检查
scripts/deploy-backend.sh --dry-run    # 只看计划
scripts/deploy-backend.sh --skip-tests # 已经跑过测试时
```

脚本形态与 WBArts 的 `deploy-backend.sh` 一致：上传新二进制 → `install` 替换 →
`systemctl restart` → `/api/health` 检查 → **任一步失败就把上一版二进制放回去并重启**。
区别是不依赖服务器上的 Git 凭据：本地构建后 `scp` 过去，因此 CI 里加个 SSH key 就能用
（见 `.github/workflows/deploy-backend.yml`）。

## 账号（方案 A：转发校验）

服务用 `--auth-verify-url http://127.0.0.1:4864/api/auth/me` 把客户端的
`Authorization: Bearer <jwt>` 转给 WBArts 后端校验——**走 loopback，不经过 Cloudflare**。

- 语义与 WBArts 完全一致：同样的 JWT（HS256、30 天）、同样的数据库复核（角色 / 封禁）。
- 需要登录的动作：`POST /api/matches`（建房）与 `POST /api/matches/{id}/join`（加入）。
  `GET /api/health` 会返回 `lobbyRequiresLogin: true` 供客户端判断。
- 单人模式、卡组管理、本地 `wbo serve` **不需要登录**：本地实例不配置
  `--auth-verify-url` 时该字段为 `false`。
- 代价：每个受保护动作多一次 loopback 请求。后续如果想省掉，可以升级到
  "共享 `JWT_SECRET` 本地验签 + 复用 `DATABASE_URL` 复核"（方案 C），语义不变。

验证过的行为：

```bash
curl -s https://sva.hypd.asia/wbo/api/health            # {"ok":true,...,"lobbyRequiresLogin":true}
curl -s -X POST https://sva.hypd.asia/wbo/api/matches   # 401 login required（无 token 或假 token）
```

## 运行约束

- **只能起一个进程**：房间与对局状态在内存里，SSE 直接读内存状态；要水平扩容得先把状态
  挪到 Postgres/Redis。当前部署是单实例，重启会丢进行中的房间（录像已在客户端本地保存）。
- 卡池从本服务目录的 `cards/`、`tests/` 读取；素材（主界面插图）直接用站点已有的
  `index/data/home-illustration/...`，不重复上传 675 MB。
- 我们自己的限流还没做：上生产后建议在 Cloudflare 上给 `/wbo/api/*` 加一条与 `/api/*`
  同级的速率规则（创建房间是最容易被刷的动作）。

## 回滚

```bash
ssh rain-1 'systemctl status wbosiris.service; journalctl -u wbosiris.service -n 50 --no-pager'
# 部署脚本失败时会自动回滚到上一版二进制；手动回滚就用同一条命令重跑旧版本
```

## CI 自动部署还缺什么（2026-09-19 核实）

`.github/workflows/deploy-backend.yml` 的 `deploy` 任务要求仓库里存在：

- 变量 `WBO_DEPLOY_ENABLED=true`（没有它，任务直接跳过，只有 `test` 会跑）；
- 机密 `WBO_DEPLOY_SSH_KEY`（能登录部署机的私钥）。

这两个**当前都不存在**（`gh api repos/Hypnos-PD/WBOsiris/actions/variables` 与 `.../secrets` 都是空），
所以自动部署从来没跑成功过——线上版本要么是手动部署的，要么落后。现在补上变量与机密即可；
在那之前，改完 `cards/`/`cmd/`/`internal/` 后要手动跑一次脚本，否则线上仍是旧规则：

```bash
scripts/deploy-backend.sh              # 本地测试 + 构建 + 上传 + 原子替换 + 健康检查（失败自动回滚）
curl -s https://sva.hypd.asia/wbo/api/health   # 返回的 version 必须等于 git rev-parse --short HEAD
```

另外 `test` 任务会先构建 `web/` 前端（`desktop/` 用 `go:embed` 嵌它，而产物不进仓库），
所以前端编译在 main 上也有门禁；触发路径包含 `web/**` 与 `desktop/**`。
