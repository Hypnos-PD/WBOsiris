# WBO Arena Client

本目录是 WBOsiris 的 React/TypeScript 牌桌客户端。

```bash
npm install
npm run dev
```

当前页面提供本地可操作牌桌演示。`App.tsx` 的演示状态将替换为 Go 模拟器的服务端状态，
前端只消费 `state`、`legalActions`、`pendingChoice` 和 `events`，不实现规则判断。

视觉素材参考并部分复制自同一工作区的 `WBArts/data`，包括牌框、职业图标、卡背和示例卡图。
