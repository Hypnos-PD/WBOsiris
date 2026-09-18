# 训练环境协议（wbo-env/1）

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
{"type":"ready","protocol":"wbo-env/1","engine":"25b6ab5","ruleset":"wbo-standard-0.3.0",
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
③ 需要选择（target/mode）时由环境按 greedy 规则自动回答（v1 不暴露）。

终局：

```json
{"type":"done","seed":7,"done":true,"winner":"own","reward":1,"turn":11}
```

`reward` 只在终局给（赢 `+1` / 输 `-1`），中间步为 0。出错给
`{"type":"error","fault":"…"}`，训练侧应当记录并终止这一局。

`{"cmd":"quit"}` 结束进程。

## 版本与兼容

- `wbo-env/1`：命令/事件形状（本文）。
- `wbo-obs/1`：`state.view` 的字段集合（即 `runner.StateView`）。
- 破坏性改动必须升版本号；新增可选字段不升版本，训练侧要容忍未知字段。
- 训练侧在 `engine.lock` 里固定引擎的 tag/commit 与 `poolHash`，每条样本、每个 checkpoint
  都带上这三项（协议、编码、卡池指纹）。
