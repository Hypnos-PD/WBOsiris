# 训练环境协议（wbo-env/3）

`wbo env` 是给训练项目（WBDecima）用的**进程内、零 HTTP**的接口：
引擎侧跑一个进程，训练侧通过 stdin/stdout 交换 JSON-lines。
协议与观测编码各有版本号，训练侧把它们连同卡池指纹写进数据集与模型。

## 启动

```bash
wbo env --source-root . --engine "$(git rev-parse --short HEAD)" cards
```

`--engine` 是回报给训练侧的引擎版本标识（提交号/标签），卡池指纹由引擎自己算。
stdin 每行一条命令，stdout 每行一个事件；诊断信息走 stderr。

## 握手

```json
{"cmd":"hello"}
{"type":"ready","protocol":"wbo-env/3","engine":"25b6ab5","ruleset":"wbo-standard-0.3.0",
 "poolHash":"3cd6f8ef32453426","encoding":"wbo-obs/1"}
```

训练侧至少要校验 `protocol` 与 `encoding`；`engine`/`poolHash` 变化说明引擎或卡池换了。

## 开一局

```json
{"cmd":"reset","seed":7,"deck":[10001110,…],"oppoDeck":[…],
 "format":"rotation","firstPlayer":"own","opponent":"greedy"}
```

| 字段 | 说明 |
| --- | --- |
| `seed` | 本局随机种子（同一 seed + 同一动作序列 ⇒ 同一局） |
| `deck` / `oppoDeck` | 40 张卡组；省略则用内置练习卡组 |
| `format` | `rotation`（指定模式，默认）或 `unlimited` |
| `firstPlayer` | `own`（学习者先手）/ `oppo`；省略时按 seed 决定 |
| `opponent` | `greedy`（默认）、`random`，或 `external`（对手侧也交给训练侧，见下） |

学习者固定在 `own` 侧；观察里的 `own`/`oppo` 都按学习者视角转换，对手手牌只给张数。

## 决策与推进

需要学习者做**主动作**时吐出 `state`：

```json
{"type":"state","seed":7,"side":"own","turn":3,"phase":"main",
 "view": { …runner.StateView… },
 "legal":[{"kind":"play","actor":"own","source":"…"},{"kind":"attack_leader",…}]}
```

训练侧用**下标**提交：

```json
{"cmd":"step","action":2}
```

引擎会：① 结算该动作；② 让对手走到再次轮到学习者（或终局）；
③ 需要学习者做选择（target / mode / fusion_material）时，按下面的"选择"小节继续推进。

## 选择（v2）

学习者这一侧的选择不再由环境代答，而是拆成**自回归子动作**：

- 需要选择时同样吐 `state` 事件，`view` 与主动作时一致，额外带 `choice` 字段；
- `legal` 里是选择类动作：`select`（选一个还没选过的候选）与
  `confirm`（达到下限且未满上限时提交）；
- 选满 `maxSelections` 会**自动提交**，不需要 `confirm`；
- 提交时返回的累积选择放在 `choice.selected`，key 形如 `e:<instanceId>`（实体）、
  `l:<side>`（主战者）、`o:<optionId>`（模式选项）；
- `choice.candidates[].cardId` 给出实体候选对应的卡牌 ID（查得到才有），方便观测编码。

```json
{"type":"state","side":"own","turn":5,"phase":"main","view":{ … },
 "choice":{"requestId":"5b28…","kind":"target","minSelections":2,"maxSelections":2,
           "selected":["e:ab12…"],
           "candidates":[{"key":"e:ab12…","kind":"entity","instanceId":"ab12…","cardId":10001110},
                         {"key":"l:oppo","kind":"leader","leaderSide":"oppo"}]},
 "legal":[{"kind":"select","actor":"own","source":"l:oppo"},
          {"kind":"deselect","actor":"own","source":"e:ab12…"}]}
```

训练侧仍然只按下标走：`{"cmd":"step","action":0}`。选择期间的下标指向 `legal` 中的选择类动作，
不要与主动作混用。这样"主动作 + 选择序列"就是一套统一、可变长的动作空间。

两个已经踩过的坑（都写在这里避免重犯）：

- **不要限制"按下标递增地选"**：那会让 `min == max` 时走进死路（选了尾部候选就再也凑不够数量）。
  现在允许任意顺序加选。
- **不要提供撤销（deselect）**：采样策略会在 select/deselect 之间来回循环，一局永远走不完。
  只允许加选之后，子步骤数天然有界（不超过 `maxSelections`），任何合法组合仍然可达。
  引擎另外保留了"同一请求超过 64 个子步骤就报错"的兜底。

## 外部对手与自对弈（v3）

`opponent: "external"` 时不使用内置策略：**两边都由训练侧决定**。
每次 `state` 事件用 `side`（`own` / `oppo`）标明这一步属于哪一方，
该方的选择请求同样按上面的 select/confirm 暴露；
`step` 提交的动作属于最近一次 `state` 事件的那一方。

```json
{"cmd":"reset","seed":7,"opponent":"external"}
{"type":"state","side":"oppo","turn":3,"phase":"main","view":{…oppo 视角…},"legal":[…]}
```

规则与约定：

- `view` 始终是**当前行动方**的视角，隐藏信息按引擎的脱敏规则处理；
- `reward` 仍然只按 `own` 的立场给（赢 `+1` / 输 `-1`），对手侧的回报取相反数；
- 同一次会话内动作 ID 递增，保证可复现；
- 这样自对弈、联赛（对冻结快照）、以及人类参与都能走同一条路径，
  训练侧不需要分叉出"对手是内置策略"的特例。

终局：

```json
{"type":"done","seed":7,"done":true,"winner":"own","reward":1,"turn":11}
```

`reward` 只在终局给（赢 `+1` / 输 `-1`），中间步为 0。出错给
`{"type":"error","fault":"…"}`，训练侧应当记录并终止这一局。

## 历史窗口

每个 `state` 事件额外带 `history`：**最近 16 条该视角可见的对局事件**（旧的在前面）。
事件就是流式接口里的 `ir.RuntimeEvent`：`Kind` / `Side` / `From` / `To` / `Reason` /
`CardID` / `Count` / `Actual` / `Sequence`（以及实体/主战者指针）。

```json
{"type":"state","side":"own","turn":5,"phase":"main","view":{…},"legal":[…],
 "history":[{"Kind":"turn_started","Side":"own","Sequence":41},
            {"Kind":"card_drawn","Side":"own","Count":1,"Sequence":42},
            {"Kind":"card_played","Side":"own","InstanceID":"ab12…","CardID":10001110,"Sequence":43},
            {"Kind":"follower_summoned","Side":"own","From":"hand","To":"field","Sequence":44}]}
```

约定：

- 只有**本视角看得见**的事件进窗口：`PrivateTo` 不是本方的事件会被脱敏成
  `Kind`/`Side`/`Count`/`Sequence`（与 SSE 流一致），所以历史窗口不会泄露对手手牌；
- 窗口按 `Sequence` 升序，长度不足时就是短窗口（开局第一步是空数组）；
- `lookahead` 返回的假想局面同样带 `history`，内容是"假设这么打下去"的历史，
  搜索/推演可以直接用同一套编码；
- 为什么需要它：观测里原来只有聚合量（墓地有哪些牌、这回合是否攻击过），
  「先出 A 再出 B」与「先出 B 再出 A」在模型眼里完全一样，而规则上常常不等价。

`{"cmd":"quit"}` 结束进程。

## 版本与兼容

- `wbo-env/3`：命令/事件形状（本文）。新增 `opponent: "external"`，两边都可由训练侧驱动。
- `wbo-env/2`：学习者选择进入动作列表（历史上含 deselect，后因循环问题移除），对手固定为内置策略。
- `wbo-env/1`：历史版本，学习者选择由环境按 greedy 自动回答，训练侧无法学习目标与模式选择。
- `wbo-obs/1`：`state.view` 的字段集合（即 `runner.StateView`）。
- 破坏性改动必须升版本号；新增可选字段不升版本，训练侧要容忍未知字段。
- 训练侧在 `engine.lock` 里固定引擎的 tag/commit 与 `poolHash`，每条样本、每个 checkpoint
  都带上这三项（协议、编码、卡池指纹）。
