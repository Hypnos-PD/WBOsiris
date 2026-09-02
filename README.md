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

当前 `web/` 是本地牌桌 GUI 演示，使用与模拟器一致的状态概念；Go 模拟器服务接入仍在进行中。
GUI 参考了 `../WBArts` 的牌框、职业图标、卡背和卡牌素材，素材复制到 `web/public/assets/`。

文档入口见 [docs/README.md](docs/README.md)。
