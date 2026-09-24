# WBOsiris

WBOsiris 是《影之诗：超凡世界》规则描述与确定性执行工具。规则作者使用 `.wbo`
编写卡牌、使用 `.wbotest` 编写场景；工具链负责检查、格式化、编译和执行测试。

## 快速开始

```bash
go run ./cmd/wbo check --strict-references --source-root . cards tests
go run ./cmd/wbo test --ruleset wbo-standard-0.3.0 --source-root . tests
go run ./cmd/wbo compile --source-root . --output cards.wbos cards
go run ./cmd/wbo simulate --source-root . --scenario "最大能量已满时保持上限并抽牌" cards tests
go run ./cmd/wbo serve --source-root . --listen :23215
go run ./cmd/wbo native probe
./scripts/card_coverage.sh --min-percent 40
```

## 目录

```text
cards/            卡牌规则源码
tests/            规则场景源码
cmd/wbo/          wbo 命令行入口
internal/engine/  卡牌语言、类型化 IR 与规则执行（ir、syntax、ruleset、runner、project、ai、search、deckcode）
internal/server/  线上规则服务（HTTP/SSE，给网页与桌面端用）
native/           原版客户端接入：proto（信封与契约）、broker（本地服务端）、profile（档案层）、
                  client（发现/导入/资源铺设/启动）、shim（Rust 代理 DLL）、netlog（C 探针）
docs/             语言、编译格式和工具文档
```

牌桌界面用原版《影之诗：超凡世界》客户端，而不是自绘 GUI：`native/` 负责把它接到本机
服务上（不改 Steam 安装目录），见 [原版客户端](docs/tooling/native-client.md)。
`internal/server/` 提供网页与桌面端用的规则会话服务，房间支持邀请加入、刷新恢复与
本机席位列表，见 [网络房间与恢复](docs/tooling/network-rooms.md)。

文档入口见 [docs/README.md](docs/README.md)。
