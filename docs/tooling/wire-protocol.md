# 对局线协议 v1

这一层协议是"人、AI、工具"共用的唯一入口：网页客户端只是它的一个图形前端，
练习模式的 AI 是服务端内的一个策略，自对弈与外部 bot 走的是同一组消息。
协议先定下来，客户端、服务端与 AI 才能各自演化（这一点是照着 ygopro 的
ocgcore / 客户端 / WindBot 三分结构学的）。

约定：

- 传输是 HTTP + JSON；长连接用 Server-Sent Events。默认地址 `http://127.0.0.1:23215`
  （端口刻意避开 8080 这类常见值；数字取自 WBO：W=23、B=2、O=15）；
  线上部署在 `https://sva.hypd.asia/wbo`，见[部署说明](deployment.md)。
- 所有响应都带 `Cache-Control: no-store`；状态里的 `revision` 单调递增。
- 凭据用 `Authorization: Bearer <token>`，玩家与观众都用这个头。
- 未实现的动作必须被明确拒绝（`400/403/409` 或 `result.status = "rejected"`），
  不允许静默忽略。

## 1. 角色与凭据

| 角色 | 获取方式 | 能做什么 |
| --- | --- | --- |
| 房主 `own` | `POST /api/matches` 返回 `playerToken` | 换牌、提交动作、认输、读录像 |
| 客方 `oppo` | `POST /api/matches/{id}/join?code=…` | 同上 |
| 观众 `spectator` | `POST /api/matches/{id}/spectate` | 只读：状态、公开事件、推送流 |

玩家令牌只在本人的响应里返回；邀请码只返回给等待中的房主。观众拿不到任何令牌与邀请码，
提交动作会得到 `403 spectator credentials are read-only`。

## 2. 状态投影

`state` 是**按观察者转换**的：`own` 永远是"你"的席位，`oppo` 是对手，
`turn.active == "own"` 表示轮到你。观众视角（`viewer: "spectator"`）里双方都是
自己的原始席位，**双方手牌只给张数**，也不包含 `pendingChoice`。

```jsonc
{
  "matchId": "ab12cd",
  "side": "own",
  "waiting": false,
  "matchPhase": "main",          // waiting | mulligan | main
  "mulliganReady": true,
  "opponentReady": true,
  "bot": true,                   // 练习模式：对手是 AI
  "botError": "",                // 非空表示 AI 驱动失败，客户端应提示并停止等待
  "state": {
    "firstPlayer": "own",        // 同样是观察者视角
    "turn": { "active": "own", "number": 6 },
    "phase": "main",
    "revision": 42,
    "viewer": "own",
    "own": { "pp": 5, "maxpp": 5, "leaderLife": 17, "leaderMax": 20, "hand": [ … ], "field": [ … ], "deckCount": 30, "handCount": 6 },
    "oppo": { "pp": 4, "leaderLife": 20, "handCount": 7, "field": [ … ] },
    "pendingChoice": { … },
    "gameOver": false,
    "winner": ""
  },
  "legalActions": [ { "kind": "play", "actor": "own", "source": "…" } ],
  "capabilities": { "play": true, "attack": true, "endTurn": true, … },
  "events": [ { "kind": "card_drawn", "side": "own", "count": 1, "sequence": 12 } ]
}
```

实体（`field`/`hand`/`crests`/`graveyard`/…）的字段见 `EntityView`：`instanceId`、
`cardId`、`cost`、`attack`、`life`、`keywords`、`traits`、`evolved`、`superEvolved`、
`attacksUsed`/`attackLimit`、`summoningSick`、`counters`、`fusion` 等。

## 3. 主动作与选择请求

两套词表要区分开，这是最容易写错的地方：

| 合法动作 `legalActions[].kind` | 提交时的命令 |
| --- | --- |
| `play` / `engage` / `evolve` / `superevolve` / `accelerate` / `crystallize` / `fusion` / `end_turn` / `use_extra_pp` | 同名 `kind` |
| `attack_leader` | `{"kind":"attack","source":"<攻击者>"}`（不带 `defender`） |
| `attack_entity` | `{"kind":"attack","source":"<攻击者>","defender":"<被攻击实例>"}` |

选择请求（`state.pendingChoice`）由引擎产生，只发给 `publicTo` 指定的那一方：

```jsonc
{
  "requestId": "…", "actionId": "…", "stateRevision": 41,
  "kind": "target",              // target | mode | fusion_material
  "minSelections": 1, "maxSelections": 1,
  "candidates": [ { "kind": "entity", "instanceId": "…" }, { "kind": "leader", "leaderSide": "oppo" } ]
}
```

回答时回填 `requestId`/`actionId`/`stateRevision`，并按候选类型给出
`selectedInstanceIds` / `selectedLeaderSides` / `selectedOptionIds`（模式多选时用它）。
数量必须正好等于 `minSelections`。

## 4. 端点

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/health` | 存活探测 |
| `GET` | `/api/cards` | 卡表 + `practiceDeck` + `formats`（每张卡带 `formats` 与 `deckLegal`） |
| `POST` | `/api/deckcode` | 卡组码：给 `{cards, format}` 编码，给 `{code}` 解码（官网 hash 规则） |
| `GET` | `/api/illustrations` | 主界面插图清单（内置 + WBArts 素材库），含合成参数与素材 URL |
| `GET` | `/illustration-assets/…` | 只读地取插图素材（skel/atlas/图集/背景/缩略图） |
| `GET` | `/api/scenarios` | 规则测试场景（调试用） |
| `POST` | `/api/matches` | 建房：`{deck?, format?, mode?, botDeck?, botPolicy?}` |
| `GET` | `/api/matches` | 房间列表 `{id, waiting, started, spectatable, bot, turn}`，不含任何凭据 |
| `POST` | `/api/matches/{id}/join?code=…` | 加入：`{deck, format?}`，返回客方凭据 |
| `POST` | `/api/matches/{id}/mulligan` | 换牌：`{selectedInstanceIds, expectedRevision?}` |
| `POST` | `/api/matches/{id}/spectate` | 申请观众凭据（返回只读状态） |
| `GET` | `/api/matches/{id}` | 读取状态（玩家或观众） |
| `POST` | `/api/matches/{id}` | 提交动作或选择回答（观众 403） |
| `GET` | `/api/matches/{id}/replay` | 录像帧（逐帧状态快照） |
| `GET` | `/api/matches/{id}/stream` | SSE 推送（见下） |

`format` 取 `rotation`（指定模式，默认）或 `unlimited`（无限制模式）。
`mode: "bot"` 创建练习模式：客方席位由 `botPolicy`（`greedy`/`random`）驱动，
建房即就绪，人类的每一次提交之后服务端会推进 AI 直到再次轮到人类。

## 5. 并发与错误

- `expectedRevision` 是可选的乐观锁：与当前 `revision` 不一致时返回
  `{ "result": { "status": "rejected", "errorCode": "stale_state" } }`，客户端应重新读取状态。
- `result.status` 取值：`completed`（含"引擎在等新的选择"）、`suspended`
  （有 `pendingChoice`）、`rejected`/`illegal`（`errorCode` 说明原因）、`fault`
  （引擎执行预算超限，属于 bug，应当记录）。
- HTTP 状态码：`400` 请求或牌组不合法、`401` 凭据无效、`403` 观众尝试操作、
  `404` 房间不存在、`409` 时机不对（换牌阶段、等待对手、选择属于对手）。

## 6. 推送流

```
GET /api/matches/{id}/stream        Authorization: Bearer <token>
```

每当局面的 `revision` 变化时推一条：

```
event: state
data: {"matchId":"ab12cd","side":"own","revision":43,"state":{…},"events":[…]}
```

客户端可以把它当成"状态已变"的通知：拿到数据后直接用它渲染，也可以再拉一次
`GET /api/matches/{id}`。断开重连不需要补偿（每次推送都是完整状态）。每 15 秒一条
`: keep-alive` 注释行。观众用同一个端点，得到的是双方手牌都隐藏的状态。

## 7. 写一个外部 bot

最小循环（伪代码）：

```text
token = POST /api/matches            # 建房并拿到 playerToken
POST /api/matches/{id}/mulligan      # 空列表 = 保留全部起手
loop:
  state = GET /api/matches/{id}
  if state.state.pendingChoice: POST 回答（按候选挑一个）
  elif state.legalActions:    POST 一个合法动作（注意上表的两套词表）
  else: 等待（轮不到自己）
```

服务端内置的 `internal/ai` 就是这个循环的实现（`Policy` 接口 + `Driver`），
`wbo selfplay` 用同一套代码批量自对弈，所以"陪练 AI"和"外部 bot"不会分叉。

跑起来的细节（三种模式、观战、卡组、最容易写错的两条）见
[外部 bot 运行手册](external-bot.md)；那边还指着 WBCapture 里那份能直接加载 ONNX
模型的参考实现 `wbo-bot`。
