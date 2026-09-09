# WBOsiris

WBOsiris 是《影之诗：超凡世界》规则描述与确定性执行工具。规则作者使用 `.wbo`
编写卡牌、使用 `.wbotest` 编写场景；工具链负责检查、格式化、编译和执行测试。

## 快速开始

```bash
go run ./cmd/wbo check --strict-references --source-root . cards tests
go run ./cmd/wbo test --ruleset wbo-standard-0.3.0 --source-root . tests
go run ./cmd/wbo compile --source-root . --output cards.wbos cards
go run ./cmd/wbo simulate --source-root . --scenario "最大能量已满时保持上限并抽牌" cards tests
npm --prefix web install
npm --prefix web run dev
go run ./cmd/wbo serve --source-root . --listen :8080
./scripts/card_coverage.sh --min-percent 40
```

## 目录

```text
cards/      卡牌规则源码
tests/      规则场景源码
cmd/        wbo 命令行入口
internal/   编译器、规则集和执行器实现
docs/       语言、编译格式和工具文档
web/        React/TypeScript 可操作牌桌客户端
```

当前 `web/` 是本地牌桌 GUI，默认连接 `wbo serve` 提供的规则会话服务。
GUI 使用项目内置的牌框、职业图标、卡背和卡牌素材，素材位于 `web/public/assets/`。
卡组页提供本地命名牌组库、导入导出和构筑校验；主页与房间页显示当前选中的牌组。
使用方式和文件格式见 [网页牌组库](docs/tooling/deck-library.md)。
房间支持邀请加入、刷新恢复与本机席位列表，详见 [网络房间与恢复](docs/tooling/network-rooms.md)。

文档入口见 [docs/README.md](docs/README.md)。
