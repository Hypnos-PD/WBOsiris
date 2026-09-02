# WBO Arena Client

本目录是 WBOsiris 的 React/TypeScript 牌桌客户端。

```bash
npm install
npm run dev
```

同时启动项目根目录的规则服务：

```bash
go run ./cmd/wbo serve --source-root . --listen :8080
```

页面默认连接规则服务，消费 `state`、`legalActions`、`pendingChoice` 和 `events`，不实现规则判断。

视觉素材参考并部分复制自同一工作区的 `WBArts/data`，包括牌框、职业图标、卡背和示例卡图。
