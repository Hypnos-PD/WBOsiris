# 文档

## 语言

- [卡牌 DSL](language/cards.md)：卡牌规则的作者语义和示例。
- [测试 DSL](language/tests.md)：初始状态、动作和断言。
- [形式文法](language/grammar.md)：词法、EBNF 和检查器约束。

## 编译器

- [类型化 IR](compiler/ir.md)：源语言规范化后的中间表示。
- [WBOS 容器](compiler/wbos.md)：`.wbos` 编译产物、版本和编码。
- [规则字节码](compiler/bytecode.md)：未来低层执行格式和虚拟机契约。

## 工具

- [命令行工具](tooling/cli.md)：`check`、`format`、`compile`、`test` 与 `selfplay`。
- [网页牌组库](tooling/deck-library.md)：命名构筑、导入导出、本地保存与网络对战选牌。
- [网络房间与恢复](tooling/network-rooms.md)：邀请对手、恢复席位、连接状态与本地凭据。
- [对局线协议](tooling/wire-protocol.md)：状态/动作/选择/推送流，以及写外部 bot 的最小循环。
- [部署](tooling/deployment.md)：规则服务挂在 WBA 的 `/wbo/` 前缀、账号沿用 WBArts JWT、单实例约束。
- [桌面客户端](tooling/desktop-client.md)：Go + Wails 的客户端、素材放包内、AppImage 预处理与发布。
- [训练环境协议](tooling/env-protocol.md)：`wbo env` 的 JSON-lines 协议（供 WBDecima 驱动）。
- [决策点克隆与搜索预算](tooling/search-and-clone.md)：决策点存档、克隆实测成本与搜索规模结论。

## 规则

- [回合与战斗规则草案](rules/turn-combat.md)：回合边界、恢复约束和战斗实现前提。
- [规则与素材来源契约](authority.md)：规则文本、素材和模拟器参考的职责与冲突处理。

阅读顺序建议：卡牌 DSL、测试 DSL、类型化 IR、WBOS 容器、规则字节码。形式文法
主要供编译器实现和语言兼容性审查使用。
