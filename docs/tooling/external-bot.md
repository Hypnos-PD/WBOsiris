# 外部 bot：陪练与自对弈观战

线协议（[对局线协议](wire-protocol.md)）把"人、AI、工具"统一成同一组消息，所以外部程序
用**玩家凭据**就能占一个席位：自己建房、或者拿邀请码加入客方。本文是这一条路的运行手册；
协议细节看线协议文档，模型/编码那一侧看 WBDecima 与 WBCapture 的文档。

## 参考实现

`WBCapture` 仓库里的 `wbo-bot`（Rust）就是这套循环的实现，并且能直接加载**部署好的
ONNX 模型**当大脑：

```bash
cd ../WBCapture
cargo run -p wbo-bot -- --help
```

它在 WBCapture 侧而不在 WBO 里，原因是它要用的两样东西都在那边：观测编码器
（`core/src/ai/encoding.rs`，与 WBDecima 逐值对齐）和 ONNX 推理（tract）。
用 Go 再实现一遍编码器等于把"客户端会怎么算"抄第二份，迟早漂移。协议这一侧的
适配（状态 → 编码输入）也在 WBCapture：`core/src/ai/wire_state.rs`。

## 三种模式

```bash
cd ../WBCapture

# 1) 陪练：模型 vs 服务端内置 greedy（不需要人参与，自测用）
cargo run -p wbo-bot -- --mode vs-greedy --pace-ms 400

# 2) 自对弈：房主与客方两席都由本程序驱动，浏览器里只读观战
cargo run -p wbo-bot -- --mode self-play --pace-ms 600

# 3) 加入已有房间（你在牌桌界面建房 → 把邀请码给它）
cargo run -p wbo-bot -- --mode join --match <房间号> --code <邀请码> --pace-ms 400
```

`--pace-ms` 是每步之间的停顿（观战留 400–600ms；`0` 全速跑数据）。默认连
`http://127.0.0.1:23215`，可用 `--base-url` 改。

## 观战

任何已开始的房间都能申请观众凭据。牌桌界面：大厅 → 该房间 → 「观战」。观众只读，
看不到双方手牌，也不能提交动作（`POST /api/matches/{id}/spectate` 返回的是只读凭据）。
终局后可以在录像页逐帧回看（`GET /api/matches/{id}/replay`）。

## 卡组

外部 bot 和真人走同一套建房/加入字段（`deck`、`botDeck`、`botPolicy`），所以卡组就是
它提交的那副牌。`wbo-bot` 提供三种取牌方式（优先级从高到低）：

1. `--deck-file <path> --deck-index <n>`：从卡组池文件挑一副，WBDecima 的
   `data/wba_decks_big.json`（1746 副）直接能用；
2. `--deck-code <code>`：官网规则的卡组码（牌桌卡组页的导入导出就是这个）；
3. 都不给：服务端练习牌组（`GET /api/cards` 的 `practiceDeck`）。

自对弈想让两席打不同牌：`--deck-b-code`（练习模式下它同时是内置 AI 的牌组）。

## 自己写一个 bot

最小循环在[线协议](wire-protocol.md#7-写一个外部-bot)里；两条最容易写错的：

- **两套词表**：合法动作里攻击分 `attack_leader` / `attack_entity`，提交时都叫 `attack`
  （带 `source`，实体目标再带 `defender`）；`play`/`engage`/`evolve`… 同名但要带 `source`。
- **投影**：`state` 是按观察者转换的（客方看到自己是 `own`），但 `legalActions[].actor` 是
  **原始席位名**。拿这两个字符串直接比，客方那一侧会一步都不走。

## 边界

- 房间在服务端内存里：进程重启就没了，一房一局。
- 观战没有录屏推送，终局后靠 `/replay` 的逐帧快照。
- 外部 bot 占用的是玩家席位，所以一次只能占一个；要两席都自动就开两个进程（或直接用
  `wbo-bot --mode self-play`）。
