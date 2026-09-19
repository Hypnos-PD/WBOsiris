# 训练环境协议（wbo-env/2）

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
{"type":"ready","protocol":"wbo-env/2","engine":"25b6ab5","ruleset":"wbo-standard-0.3.0",
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
| `opponent` | `greedy`（默认）或 `random` |

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
- `legal` 里是选择类动作：`select`（选一个候选）、`deselect`（撤销已选）、
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
不要与主动作混用。这样"主动作 + 选择序列"就是一套统一、可变长的动作空间，
多选组合不会因为顺序而变得不可达（v2 的早期草案曾限制递增顺序，会在
`min == max` 时走进死路，已废弃）。

终局：

```json
{"type":"done","seed":7,"done":true,"winner":"own","reward":1,"turn":11}
```

`reward` 只在终局给（赢 `+1` / 输 `-1`），中间步为 0。出错给
`{"type":"error","fault":"…"}`，训练侧应当记录并终止这一局。

`{"cmd":"quit"}` 结束进程。

## 版本与兼容

- `wbo-env/2`：命令/事件形状（本文），学习者选择进入动作列表（select/deselect/confirm）。
- `wbo-env/1`：历史版本，学习者选择由环境按 greedy 自动回答，训练侧无法学习目标与模式选择。
- `wbo-obs/1`：`state.view` 的字段集合（即 `runner.StateView`）。
- 破坏性改动必须升版本号；新增可选字段不升版本，训练侧要容忍未知字段。
- 训练侧在 `engine.lock` 里固定引擎的 tag/commit 与 `poolHash`，每条样本、每个 checkpoint
  都带上这三项（协议、编码、卡池指纹）。
