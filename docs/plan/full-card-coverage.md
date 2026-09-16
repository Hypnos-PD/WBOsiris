# 全卡覆盖计划与挂起登记

目标：把 `../WBArts/data/cards.json` 里的全部卡牌写成 WBO 规则（含 90000 卡包的衍生卡），
每张卡都配至少一个 `tests/**/*.wbotest` 场景，`check` 与 `test` 在任何提交点都必须全绿。

## 口径与政策

1. **范围**：全部卡牌。已导入骨架但效果还没实现的卡用导入器写入的 `unplayable;` 标记，
   `scripts/card_coverage.sh` 因此不会把它算成可玩——覆盖率只统计真正写完的卡。
2. **每张卡都要有场景测试**。纯关键词卡也必须至少有"打出后拥有该关键词"的断言；
   触发类效果要覆盖触发与不触发两侧。
3. **遇到缺少原语就挂起并登记**，不写近似实现、不用 `unplayable` 之外的写法伪装完成。
   挂起卡保留 `unplayable;`，并在下面的登记表里写明缺什么、影响哪些卡、草稿是什么。
4. **权威是官方卡牌文本与 QA**（见 [规则与素材来源契约](../authority.md)）。
   `../SWB-RL` 的结构化规则只作**第二意见**：用来交叉验证触发时机、目标与参数，
   以及在难以判断时给出一个可执行的假设。它与文本冲突时以文本为准。

## 现状（生成于 `node scripts/card_worklist.mjs`）

```text
总计 904 · 已完成 391 · 未实现(骨架) 55 · 未导入 458
卡包      总数  已完成  未实现  未导入
10000        56      56       0       0
10001       142     142       0       0
10002        77      77       0       0
10003        77      62      15       0
10004        76       1       0      75
10005        76       0       0      76
10006        76       0       0      76
10007        77       0       0      77
10008        78       0       0      78
10009        76       0       0      76
90000        93      53      40       0
未完成卡按文本复杂度：中(41–100字) 291 · 短(≤40字) 186 · 长(>100字) 19 · 白板 2
```

## 工作流

```bash
# 1. 导入骨架（身份/身材/五语言文本，效果先标 unplayable）
./scripts/import_wbarts_packs.sh ../WBArts cards 10002

# 2. 按官方文本补 effect 块；缺原语则挂起并登记

# 3. 编译校验 + 场景测试（本地约 2.4s / 4.3s）
go run ./cmd/wbo check --strict-references --source-root . cards tests
go run ./cmd/wbo test --ruleset wbo-standard-0.3.0 --source-root . tests

# 4. 进度与下一批候选
node scripts/card_worklist.mjs --pack 10002 --limit 20
```

每批一个提交，提交信息写明：批号、卡包、卡数、场景数、挂起项。批次结束后跑一次全量
`check` + `test`，保证主分支随时可用。

## 挂起登记

| 编号 | 缺少的原语 | 影响卡牌 | 草稿 / 说明 |
| --- | --- | --- | --- |
| S-01 | ~~移除能力（失去【谢幕曲】）~~ **已解决**：`remove lastwords from …` / `remove all abilities from …` | 已解锁 90051140 腐臭的僵尸、10251310 诅咒派对、10252120 尸兵；`../SWB-RL` 规则里另有 7 张同类卡（10321120、10433110、10474120、10861110、10862110、10871110…）等后续卡包补写时直接使用 | 实例级触发能力抑制：`suppressed`（按触发种类）与 `suppressAll`，索引与已排队触发同步剔除，随连击快照一起保存。详见 [已完成的语言扩展](#已完成的语言扩展) 的 S-01 行。 |
| S-03 | ~~从双方战场中选择随从~~ **已解决**：`field.followers`（双方战场合并，可加 `other`） | 已解锁 10201310 逆向变化；10301110 涸绝的使徒、10304120、10534120、10554110 等"选择战场上的1个（其他）随从"直接可用 | 我此前把"跨方选择随从"和"随从与主战者混合的集合"混为一谈：前者一直支持（`field.followers` 的 `Side` 为空，运行时合并双方战场）。真正的缺口是后者，见 S-19。 |
| S-06 | ~~牌组内卡牌费用变更~~ **已解决**：`halve cost 集合;`（并确认 `reduce cost 集合 N minimum M` 可按集合作用） | 已解锁 10244120 绚丽的凤凰·小凤；10334120（"使自己的牌组中的所有随从的费用 -3"）等后续卡包直接使用 `reduce cost own.deck.followers 3 minimum 0;` | 官方 FAQ：奇数费用向上取整（9 → 5），重复减半基于当前费用，且只影响发动时在牌组里的卡。`HalveCost` 单独成为一种 IR 节点，因为它不是固定增量。 |
| S-07 | ~~"其他随从进化 / 超进化"事件~~ **已解决**：`when own\|oppo follower evolved\|super_evolved [other]` | 已解锁 10241110 庇护的智龙、10212120 妖精击剑士、10252110 爆破之翼·圮尤拉；全卡另有 2 张（10721110、10862120）后续直接使用 | 事件绑定改名的随从（`evolved`），`other` 排除来源实例自身。`when self evolved/super_evolved` 保持原样。 |
| S-08 | ~~筛选器缺少"受伤状态 / 攻击力"~~ **已解决**：`where damaged`、`where attack 比较 整数` | 10272120 绝望之王·阿基姆已解锁（`where attack <= 4`）；全卡另有 3 张同类筛选（10341120、10462110、90044310）。10223110 仍未完成，缺的是"条件里判断交战对象是否受伤"，见 S-18 | 10223110 的【攻击时】需要读取 `opponent` 绑定的受伤状态，属于条件而不是筛选器；筛选器侧（`damaged`、`attack`）本条已补齐。 |
| S-09 | ~~`count(...)` 不接受 `other`~~ **已解决**：`count(集合 other)` | 已解锁 10371110 破坏的肯定者、10374110 破坏的继承者·阿克西娅；10253120 的现有写法可以继续保留（等价） | `parseEffectAmount`/`numericIR` 接受集合后的 `other`，IR 侧用 `ExcludeRef` 包一层；`validCountSource` 补上 `ExcludeRef`（排除对象是 `SelfRef` 或绑定，不是集合）。顺带让 `destroy 集合 other` 也能用。 |
| S-10 | ~~纹章效果文本来源~~ **已解决**：纹章文本在 `alt_modes` 里 | — | 我一开始只看了 `skill_texts`，误判成"数据缺失"。`../WBArts/data/cards.json` 的 `alt_modes` 数组带 `type_key` 与五语言 `text_*`（统计：crest 63 条、faith 5、crystallize 5、accelerate 5），现有 10000/10001 的纹章卡就是这么写的。教训：报"数据缺失"前先把卡表的所有字段翻一遍（含 `alt_modes`）。 |

挂起卡的依赖也会一并挂起：例如 S-01 未解决前，任何"召唤腐臭的僵尸"或"把僵尸加入手牌"
的卡都保持 `unplayable;`，避免生成行为错误的衍生体。

| S-12 | ~~纹章块内的集合条件 / 按目标自身数值成倍~~ **已解决** | 已解锁 10204120 格里姆尼尔（纹章里 `if count(own.field.followers where form super_evolved) >= 1`）与 10233310 帕梅拉的舞蹈（`double stats own.field.followers;`） | 前半在 S-04 之后其实已经可用（纹章块走同一套效果校验），我此前误判为缺口；后半新增 `DoubleStats` 节点：按每个目标自己的当前数值翻倍。 |
| S-13 | ~~`add card … to hand` 不产生绑定~~ **已解决**：`added` 绑定 | 已解锁 10271120 猫偶；全卡另有约 7 张"加入手牌后立即修改"的卡（10641310、10643310、10844110、10922310 等），后续卡包直接使用 | `add 1 card X to hand` 现在把成功进入手牌的实例绑成 `added`（手牌满被丢弃的不计入），Go 测试同时验证只有新卡被强化、手里的同名旧卡不受影响。 |
| S-11 | `.wbotest` 无法声明先手 | 影响"抽牌耗尽判负"这类与先手有关的规则验证 | 规则本身已实现（`execute.go` 在无法满足抽牌时让对方获胜），但触发条件是 `firstPlayer != ""`，而场景测试不提供先手信息，这条路径只能由 Go 侧测试覆盖，场景测试写不出来。 |
| S-18 | ~~条件里判断绑定对象的受伤状态~~ **已解决**：`if <绑定> damaged { … }` | 已解锁 10223110 剑士公主·萝泽（`attack { if opponent damaged { destroy opponent; } }`） | `internal/ir` 新增 `IsDamagedCondition`，条件求值新增 `conditionIn(c, self, bindings)`（执行与预检两处都带当前帧），绑定缺失或对象不是随从时为假。 |
| S-19 | 随从与主战者混合的随机集合 | 10524110 威猛的《战车》·奥辂昂（"随机对战场上的1个其他随从或自己的主战者或对手的主战者造成7点伤害"） | `character_set` 只允许 `own.field.followers or own.leader` / `oppo...` 这种同方混合，而且只用于 `choose/require`。跨方的"随从或双方主战者"混合随机目标尚未支持；需要把主战者纳入随机/选择集合。 |

## 已完成的语言扩展

| 编号 | 扩展 | 改动 | 验证 |
| --- | --- | --- | --- |
| S-04 | `if count(集合 [where …]) 比较 整数` —— 集合计数条件 | `internal/ir/model.go`（新增 `CountCondition`）、`internal/project/typed_ir.go`（解析，含集合与筛选）、`internal/project/validate.go` 与 `strict_validate.go`（形状校验，复用 `parseEffectAmount` 处理括号）、`internal/runner/execute.go`（求值并比较）、`internal/ir/decode.go`（容器解码）、文档 | Go 单测 `internal/project/count_condition_test.go`（带筛选/不带筛选、拒绝缺比较符与多余 token）；7 张卡共 10 个场景 |
| S-05 | `enhance N replaces { ... }` —— 爆能强化的"改为"档 | `internal/project/validate.go`（允许 `replaces`）、`internal/project/typed_ir.go`（写入 `Ability.Relation`）、`internal/runner/runner.go`（`commitPlay` 在支付该档时跳过基础效果与入场曲）、`docs/language/cards.md`/`grammar.md` | Go 单测 `internal/project/enhance_replaces_test.go`（关系被保留、fanfare 不受影响、拒绝 `extends`/缺档位等错误形状）；卡片 10222310 + 2 个场景（普通档只打 1 个，爆能档恰好打 3 个且没有多出一次基础伤害） |
| S-02 | `draw N for own\|oppo` —— 让对方抽牌 | `internal/project/validate.go`（接受可选 `for` 子句）、`internal/project/typed_ir.go`（写入 `DrawEffect.Owner`）、`docs/language/cards.md`、`docs/language/grammar.md` | Go 单测 `internal/project/draw_owner_test.go`（默认仍是自己、`for oppo` 生效、保留过滤器、拒绝 `for self`/重复 `for`/`draw all for oppo` 无过滤器、格式稳定）；卡片 10221310 + 2 个场景 |
| S-14 | `fused.cost` / `fused.distinct` 的作用域 | 原实现只允许在 `fusion` 块内读，但"若已与本卡牌融合，则改为抽取 2 张"这类文本的判断点在打出/入场结算时。`internal/project/strict_validate.go` 新增 `effectContext.materials`：卡牌只要声明了 `fusion`，其任意效果块（含 `when … if …`）都可读 `fused`；没声明融合的卡牌读它仍然报错。文档同步 `cards.md` / `grammar.md` | Go 单测 `internal/project/fused_scalar_scope_test.go`（法术 `effect` 内可读且条件被完整保留、无融合声明时拒绝）；卡片 10213310 + 2 个场景，并由 `internal/runner/fused_play_effect_test.go` 用真实卡表端到端覆盖"融合后抽 2 / 未融合抽 1" |
| S-14b | `fused.cost` / `fused.distinct` 作为数值 | S-14 只让它们在条件里可读；`damage oppo.field.followers fused.distinct;` 这类把融合数当伤害值的文本还需要数值入口。`internal/project/numeric.go`（`fused.` → `Scalar{Kind:"fusion_material_scalar"}`）、`internal/project/validate.go`（`parseEffectAmount` 接受 `fused.`）、`internal/ir/amount.go`（`validNumericExpr` 与容器解码）、`internal/runner/numeric.go`（按材料求值） | Go 单测 `internal/project/fused_amount_test.go`；卡片 10324110 篡夺的继承者·辛瑟莱兹按融合种类造成伤害 + 2 个场景 |
| S-15 | 测试 DSL 的实例字段断言 | 新增 `alias.attack_limit`（取 `attackLimit()`，"1 回合可以攻击 N 次"）与 `alias.damage_reduction`；`internal/project/validate.go`、`strict_validate.go`、`internal/ir/test_decode.go`、`internal/runner/assert.go` 四处白名单同步，写错字段名从"静默不相等"改成检查期报错 | 卡片 10274120 场景直接断言 `source.attack_limit == 2`；全量 444 个场景回归 |
| S-01 | `remove lastwords from …` / `remove all abilities from …`（实例级失去能力） | `internal/project/validate.go`（`remove` 新增两种形状）、`typed_ir.go` + `keyword_effect.go`（解析成 `remove_ability`，`Keyword` 取 `lastwords`/`all`）、`internal/ir/encode.go`/`decode.go`（新效果种类）、`internal/runner/grant.go`（`suppressed`/`suppressAll`、剔除已排队触发、重新索引）、`internal/runner/execute.go`（派发）、`internal/runner/continuation.go`（连击快照保存抑制状态）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/ability_removal_test.go`（两种形状编成 `remove_ability`、拒绝 `remove fanfare`/漏 `from`/`add lastwords`/带 `until`）与 `internal/runner/ability_removal_test.go`（僵尸链在第二次死亡后停止、`remove all abilities` 同时清掉固有关键词与谢幕曲）；卡片 90051140、10252120、10251310 + 3 个场景 |

| S-16 | `ability_destruction_guard`（不会被能力破坏） | 新固有关键词：`internal/ir/decode.go` 的 `validKeyword`、`internal/project/validate.go` 的 `abilities`、`strict_validate.go` 的固有能力形状表、`internal/runner/runner.go`（`destroyByEffect` 受保护，必杀改走新的 `destroyByCombat` 不受保护）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/runner/granted_ability_flow_test.go`（能力破坏被挡下、战斗破坏照样生效）；卡片 10273110 + 2 个场景 |
| S-07 | `when own\|oppo follower evolved\|super_evolved [other]` —— 其他随从的进化事件 | `internal/ir/model.go`（`EventTrigger.ExcludeSelf`）、`internal/ir/decode.go`（校验 `other` 只用于带对象的非受伤事件）、`internal/project/strict_validate.go`（基础事件模式接受进化动词与 `other`）、`internal/project/typed_ir.go`（事件名映射与 `ExcludeSelf`）、`internal/project/validate.go`（`evolved` 绑定）、`internal/runner/runner.go`（进化事件改为把改名随从绑成 `evolved`）、`internal/runner/trigger_index.go`（按实例排除自身）；文档 `grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/evolution_event_test.go`（`other`/绑定/来源区域、拒绝重复 `other` 与 `self … other`）；卡片 10241110、10212120、10252110 + 6 个场景（含"不因普通进化触发"与"不响应自己超进化"两个反向场景） |
| S-17 | `set cost T N;` —— 把费用设为固定值 | `internal/project/validate.go`（`set` 接受 `cost`）、`typed_ir.go`（`set_cost`）、`internal/ir/encode.go`/`decode.go`、`internal/runner/execute.go`（写 `i.cost`）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/evolution_event_test.go` 覆盖 `set cost self 1;` 与错误形状；卡片 10212120 场景断言 `source.cost == 1`。"费用变为 N"的全卡需求还有 15 张，本项是通用原语 |
| S-08 | `where damaged` / `where attack 比较 整数` —— 筛选器补两个词条 | `internal/project/validate.go`（`parseWhere` 接受 `damaged` 与 `attack`）、`typed_ir.go`（`filterIR` 生成 `is_damaged` / `compare field:"attack"`）、`internal/ir/decode.go`（谓词白名单）、`internal/runner/execute.go`（`matches` 求值：`damageTaken > 0`、`currentAttack()`）；文档 `grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/filter_and_added_test.go`；卡片 10272120 + 2 个场景（含"攻击力 5 以上不在范围内"的反向场景） |
| S-13 | `add card … to hand` 产出 `added` 绑定 | `internal/project/typed_ir.go`（写入 `Output`）、`internal/project/validate.go`（`added` 可见性）、`internal/ir/encode.go`/`decode.go`（`add_card.output` 必填 `added`）、`internal/runner/execute.go`（把真正进入手牌的实例绑成 `added`）；文档 `cards.md`/`compiler/ir.md` | Go 单测 `internal/project/filter_and_added_test.go`（`added` 只在 `add` 之后可见）与 `internal/runner/added_binding_test.go`（只强化新加入的那张，手里同名旧卡保持 1/1）；卡片 10271120 + 2 个场景 |
| S-18 | `if <绑定> damaged { … }` —— 条件读取绑定实例的受伤状态 | `internal/ir/model.go`（`IsDamagedCondition`）、`internal/ir/decode.go`（容器解码）、`internal/project/typed_ir.go`/`validate.go`/`strict_validate.go`（`<标识符> damaged` 形状）、`internal/runner/execute.go`（`conditionIn` 带帧求值）、`internal/runner/session.go` 与 `runner.go`（执行与预检改用 `conditionIn`）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/damaged_binding_condition_test.go`（条件形状被保留、拒绝多 token 与无绑定写法）；卡片 10223110 + 4 个场景（爆能/非爆能、攻击受伤/未受伤四个方面） |
| S-06 | `halve cost 集合;` —— 牌组内卡牌费用减半 | `internal/project/validate.go`（`halve` 语句形状）、`typed_ir.go`（`halve_cost`）、`internal/ir/encode.go`/`decode.go`（新节点与形状校验）、`internal/runner/execute.go`（`ceil(cost/2)`，按集合逐个结算）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/halve_cost_test.go`（目标保留为牌组集合、拒绝 `halve cost`/`halve deck`/多余 token/`countdown`）与 `internal/runner/deck_cost_test.go`（5→3→2 的重复减半、`followers` 不碰法术、整副牌组 `reduce` 夹在 0）；卡片 10244120 + 2 个场景（奇数向上取整、只影响发动时在牌组里的卡） |
| S-03 | 复查：`field.followers` 选择双方战场随从 | 无需改代码，只补文档与测试：`internal/project/cross_side_selection_test.go`（`field.followers other` 保持 `Side=""` 且排除自身、`own.field.followers` 不变） | 卡片 10201310 + 2 个场景（指定对手随从并降到 0 生命、指定自己的随从）。`grammar.md` 早已写明 `field` 是双方战场合并集合，我此前误判为缺口 |
| S-12 | `double stats 集合;` —— 按目标自身数值翻倍 | `internal/project/validate.go`（`double` 语句形状）、`typed_ir.go`（`double_stats`）、`internal/ir/encode.go`/`decode.go`、`internal/runner/execute.go`（攻击力、当前生命、已受伤害各 ×2）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/double_stats_test.go`、`internal/runner/double_stats_test.go`（3/6 且已受 4 点伤害 → 6/4、伤害 8；护符不受影响）；卡片 10233310 + 3 个场景（土之印 +1 与获得纹章、土之秘术 10 足够时翻倍并抽牌、不足时只抽牌） |
| S-20 | `other 绑定名` —— 从集合里排除一个绑定 | 原 `other` 只能排除来源自身；`internal/project/keyword_effect.go`/`validate.go` 新增 `otherExclusion`/`otherExclusionEnd`，`choose/require/random` 与 `add/remove/buff` 的目标集合都接受可选绑定名。运行时不需要改动：`ExcludeRef` 早就按任意 Ref 求值 | Go 单测 `internal/project/other_binding_test.go`（`other opponent` 指向绑定、裸 `other` 仍排除自身）；卡片 10263110 + 2 个场景（破坏非交战对手、护符不足时不破坏） |
| S-21 | `rally`（协作）计数器与 `rally >= N` 条件 | `internal/runner/runner.go`（`player.rally`、`pendingRally`、打出随从先挂起）、`internal/runner/execute.go`（`countRally`/`creditRally`）、`internal/runner/session.go`（结算结束、触发队列之前入账）、`internal/runner/numeric.go`/`assert.go`/`simulator.go`/`continuation.go`、`internal/ir/model.go`/`amount.go`/`test_decode.go`、测试状态 `rally N;`；文档 `cards.md`/`grammar.md`/`tests.md`/`compiler/ir.md` | Go 单测 `internal/project/rally_test.go` 与 `internal/runner/rally_test.go`（法术不计入、打出在结算后入账、能力召唤立即计入）；卡片 10224110 + 3 个场景（协作 19 不发动、协作 20 发动并召唤两个骑士、手动进化同样召唤） |
| S-22 | `summon N card X for own\|oppo;` —— 在指定一方战场召唤 | `internal/project/validate.go`（`summon` 接受 `for` 子句）、`typed_ir.go`（写入 `CardEffect.Owner`）；运行时本来就按 `Owner` 处理 | `internal/project/rally_test.go` 覆盖目标归属；卡片 10224120 + 2 个场景（在对手战场召唤 2 个骑士并各触发一次监听、对手满场时只召唤 1 个） |
| S-23 | ~~条件里读 `self.cost` / `self.attack` / `self.life`~~ **已解决**：`if self.cost != 2` | 已解锁 10331110 真理的肯定者、10332110 真理的祈祷者；后续"若本卡牌的费用/生命值…"直接可用 | 解析成 `Scalar{Kind:"self_scalar"}`。条件求值的 `CompareCondition` 分支原本只把 `self_counter`/`scalar` 交给 `numericValue`，漏了 `self_scalar`，于是它落进融合材料分支恒为 0——场景测试第一次跑就抓到了这个 bug。 |
| S-24 | 牌组"没有重复卡牌"条件 | 10301310 至高的凌驾（"若自己的牌组中没有重复卡牌，则回复自己 3 点能量点"） | 需要按卡牌 ID 统计牌组重复并作为条件；现有集合计数看不出"重复"。 |
| S-25 | ~~费用增加（`+N`）~~ **已解决**：`raise cost T N;` | 已解锁 10333310 虚假的术式；90044310 等后续卡包直接可用 | 复用 `AdjustEntityField(delta=+N, minimum=0)`，没有新增 IR 节点。顺带给 `damage`/`heal` 补上 `other [绑定]`，写法与 `buff`/`add` 一致。 |
| S-26 | 牌组去重 | 10303210 试炼的石板（"使自己的牌组中的重复卡牌消失并只保留 1 张"） | 需要一条按卡牌 ID 去重的牌组操作，涉及随机或稳定保留顺序的规则。 |
| S-27 | "本随从获得身材增加时"事件 | 10361120 圣骑士团员（"本随从在战场上获得攻击力或生命值增加时，回复自己的主战者 1 点生命值"） | 现有事件只有入场、离场、进化、破坏、受伤、回血、融合、弃牌、回合边界；没有"能力使某随从身材增加"的事件。 |
| S-28 | ~~按极值筛选的伤害~~ **已解决**：`damage 集合 数值 highest\|lowest [base.] attack\|life\|cost` | 已解锁 10341310 雷霆之怒；后续"生命值/攻击力最大"的伤害与回复直接可用 | `TargetEffect` 新增 `Extremum`，运行时在过滤前用 `extremumCandidates` 收窄目标；`damage all.leaders … highest life` 走 `damageExtremumLeaders`，并列时双方都挨打（场景测试覆盖并列）。 |
| S-29 | ~~纹章数标量~~ **已解决**：`own.crests` / `oppo.crests` | 已解锁 10364120 绝望的显现·玛温 | 玩家标量新增 `crests`（本方纹章实例数）；"分配 X 点伤害"复用既有的 `distributed overflow <leader>` 语义（按战场顺序填满每个随从当前生命，余量溢出给主战者），场景测试覆盖了溢出与不溢出两种情况。 |
| S-30 | 牌组替换与牌组底部变身 | 10304110 绝大的显现·麦哲佩恩（纹章"使自己的牌组变为麦哲佩恩牌组；使牌组底部的亡者之王卡牌变身"） | 需要用预置牌组替换当前牌组，并支持"牌组底部第 N 张"变身；属于新的牌组级操作。 |
| S-31 | 主战者获得关键词 | 10362210 安息的神殿（谢幕曲"使自己的主战者获得【屏障】"） | `add <keyword> to own.leader` 不被支持：关键词挂在实例上，主战者没有实例。需要一条主战者级关键词/状态。 |
| S-32 | 进化与超进化共用一次选择 | 10334110 真理的继承者·蓓哈丽雅（"进化时使其消失；超进化时使对手战场上与其同名的所有随从消失"） | 需要"超进化时复用普通进化那次选择的目标"，现有 `superevolve replaces evolve` 只处理能力替换，没有共享绑定。 |
| S-33 | 动态倒计数减少 | 10362210、10363210（"【启动】本护符的倒计数-X，X 为自己的纹章数"） | `reduce countdown T N` 只收整数常量；`AdjustEffect.Delta` 是 int。草案：允许数值表达式作为增量。 |
| S-34 | "费用发生过变化的随从"事件 | 10332210 真理的研究设施（"自己使用费用发生变化的随从时，抽取 1 张卡牌"） | 需要给实例记录"费用被改过"并用它做事件筛选；现有事件与筛选都没有这个状态。 |
| S-35 | 信仰值（faith）机制 | 10354110 混融的继承者·莎木·纳克雅（"信仰值-10，使自己的信仰获得「自己选择的【模式】数+1」"） | `alt_modes` 里有 5 张 faith 卡，但引擎没有信仰值、模式计数与纹章式"信仰"实体的语义。 |
| S-36 | 多选模式（选择 N 个能力） | 10353310 叫唤与憎恶（"【模式】选择 2 个能力发动"）、10852310 | `mode` 是单选：玩家选择 1 个 option。需要多选响应、去重与顺序规则。 |
| S-37 | 条件合取 | 10304120 涸绝的显现（"若自己和对手的能量点最大值为 10"） | 条件没有 `and`；该卡用嵌套 `if own.maxpp == 10 { if oppo.maxpp == 10 { … } }` 绕过，但更复杂的条件需要合取/析取。 |
| S-38 | ~~"自己使用卡牌时"事件~~ **已解决**：`when own card played [other] [where …]` | 已解锁 10323110 篡夺的团结者、10324120 空绝的显现·奥克托丽丝；全卡另有 14 张"自己使用…时"（10571120、10572120、10822110、90021210…） | 打出后发 `card_played` 事件并绑定 `played`。顺带修好"纹章用 `reduce countdown self 1` 推进吟唱"：纹章不在战场，需要显式纳入目标，归零时走纹章退场（触发谢幕曲）。 |
| S-39 | 动态随机/选择数量 | 10373110 破坏的团结者、10364110 安息的继承者·妃花（"破坏对手的战场上的随机 X 个随从"，X 为动态值）、10811110 | `SelectionEffect.Count` 是整数常量；"随机 X 个"需要动态数量（10811110 还要求算术表达式）。 |
| S-40 | `set cost` 的持续时间 | 10334120 绝尽的显现·莱奥（"回合结束前，使其费用变为 0"） | `set cost`/`reduce cost` 都是永久的；需要"回合结束前"这类临时费用修改与到期还原。 |

`DrawEffect.Owner` 与执行器（`g.playerForSide(self, e.Owner)`）本来就支持任意一方，缺的只是语法入口，所以这次扩展只动了验证与解析两处，没有改运行时。

## 批次记录

### 批次 33（语言扩展 S-38 + 卡包 10003 / 90000）

- **S-38 完成**：`when own card played [other] [where …]`。打出后发出 `card_played` 事件并绑定
  `played`；`other` 排除本卡牌自身，`where` 支持类型/trait/费用筛选。这条事件在全卡里
  有 16 张卡使用，是"自己使用…时"家族的通用入口。
- 顺带修好一类结构问题：纹章用 `reduce countdown self 1` 推进吟唱——纹章不在战场上，
  `effectTargets` 默认把纹章过滤掉，需要显式纳入；归零时改走纹章退场路径（退场事件 + 谢幕曲）。
- 完成 2 张 10003 卡：10323110 篡夺的团结者、10324120 空绝的显现·奥克托丽丝；
  并顺手把它们的 8 张衍生卡写完（90021310/20/30/40 黄金短剑/杯/靴/项链、90024310 空绝的残光、
  90054310/20 绝叫的扩散/爱绝的飞翔、90004330 涸绝的甘露），90000 卡包因此从 45 到 53。
- 新增 7 个场景（`tests/10003/batch-33-card-played.wbotest`）+ 1 组 Go 测试。
  踩坑：黄金短剑等衍生卡以前是导入骨架（`unplayable`），要先实现它们才能在场景里打出。
- 全量回归：`check` 0 错 0 警；`test` 579 全绿；语料快照更新为 579 个场景。

### 批次 32（语言扩展 + 卡包 10003）

- **S-09 完成**：`count(集合 other)`（统计时排除来源自身），顺带让 `destroy 集合 other` 可用；
  IR 侧用 `ExcludeRef` 包一层，`validCountSource` 补上 `ExcludeRef`（被排除的是实例，不是集合）。
- **新增 `set attack T N`**（与 `set life` 对称）与标量 `self.damage_taken`
  （"回复至上限"写作 `heal self self.damage_taken`，"同时给主战者等量生命"则先
  `heal own.leader self.damage_taken` 再回复自己）。
- 完成 7 张卡：10342110 侮蔑的祈祷者、10344110 侮蔑的继承者·安吉拉弗利特、
  10371110 破坏的肯定者、10374110 破坏的继承者·阿克西娅、10322120 活泼的斥候、
  10304120 涸绝的显现·吉尔内莉莎、10354120 绝叫与爱绝的显现·鲁鲁纳伊&巴娜蕾卡。
- 新增 10 个场景（`tests/10003/batch-32-heal-and-other.wbotest`）+ 1 组 Go 测试。
  又踩到一次预期错误：破坏的肯定者的 X 包含场上已有的同名随从（3 张其他卡牌 → +3/+3），
  引擎行为正确，改的是断言。
- 全量回归：`check` 0 错 0 警；`test` 572 全绿；语料快照更新为 572 个场景。

### 批次 31（卡包 10003）

- 完成 13 张卡：10333110 真理的团结者、10343110 侮蔑的团结者、10324110 篡夺的继承者·辛瑟莱兹、
  10312210 不弑之乡、10322210 篡夺的据点、10343310 威猛炽焰、10323310 奉还的剑闪、
  10352210 混融之城、10351110 混融的肯定者、10372110 破坏的祈祷者、10372210 破坏的荒野、
  10352110 混融的祈祷者、10353110 混融的团结者。
- **语言扩展 S-14b**：`fused.cost` / `fused.distinct` 现在也能当数值用
  （`damage oppo.field.followers fused.distinct;`）。原来只有条件入口，
  "X 为与本卡牌融合的种类"这类伤害文本写不出来。
- 新增 22 个场景（`tests/10003/batch-31-fusion-and-modes.wbotest`）+ 1 组 Go 测试。
  踩到两个测试侧的坑：① 演化动作需要先声明 `ep 1`；② `90031210`（大地之魔片）带土之印，
  按规则不能被能力破坏，挑选"被破坏的自己卡牌"时要换一张普通护符。
- 全量回归：`check` 0 错 0 警；`test` 562 全绿；语料快照更新为 562 个场景。

### 批次 30（语言扩展 S-28/S-29 + 卡包 10003）

- **S-28 完成**：`damage 集合 数值 highest|lowest [base.] attack|life|cost`。目标集合在过滤前
  用 `extremumCandidates` 收窄；主战者用 `damage all.leaders … highest life`，
  并列时双方一起挨打（`damageExtremumLeaders`）。
- **S-29 完成**：玩家标量新增 `crests`（本方纹章数）。"分配 X 点伤害"直接复用既有的
  `distributed overflow oppo.leader`：按战场顺序填满每个随从的当前生命，余量溢出给主战者。
- 完成 2 张卡：10341310 雷霆之怒（打击生命值最大的所有随从；觉醒时额外打击生命值最大的
  主战者）、10364120 绝望的显现·玛温（入场曲加入绝望的奔流；进化获得纹章，纹章在自己
  未攻击的回合结束时按纹章数分配伤害）。
- 新增 6 个场景（`tests/10003/batch-30-extremum-and-crests.wbotest`）+ 1 组 Go 测试
  （`internal/project/extremum_damage_test.go`）。
- 全量回归：`check` 0 错 0 警；`test` 541 全绿；语料快照更新为 541 个场景。

### 批次 29（语言扩展 S-23/S-25 + 卡包 10003）

- **S-23 完成**：条件可以读来源实例自己的数值（`self.cost` / `self.attack` / `self.life`）。
  实现时踩到一个隐蔽 bug：`CompareCondition` 求值只把 `self_counter`/`scalar` 交给
  `numericValue`，`self_scalar` 掉进了融合材料分支恒为 0，导致"费用不为 2"永远成立——
  场景测试立刻抓到，已修。
- **S-25 完成**：`raise cost T N;` 加费（复用 `AdjustEntityField`，无新 IR 节点）；
  同时把 `other [绑定]` 推广到 `damage`/`heal`，这样"对战场上的其他所有随从造成 3 点伤害"能直写。
- 完成 3 张卡：10331110 真理的肯定者、10332110 真理的祈祷者、10333310 虚假的术式。
- 新增 5 个场景（`tests/10003/batch-29-cost-conditions.wbotest`）+ 1 组 Go 测试
  （`internal/project/self_cost_and_raise_test.go`）。
- 全量回归：`check` 0 错 0 警；`test` 535 全绿；语料快照更新为 535 个场景。

### 批次 28（卡包 10003）

- 完成 15 张卡：10332310 双重创造、10311120 妖精剑的后继者、10313110 不弑的团结者、
  10312110 不弑的祈祷者、10321310 护盾强袭、10351120 泡沫鬼姬、10311110 不弑的肯定者、
  10314120 绝命的显现·艾斯迪亚、10361310 圣辉闪烁、10362220 羽翼狮子像、
  10371120 音速飞行兵、10374120 奏绝的显现·莉洁纳（用到刚加的"不会被能力破坏"）、
  10321110 篡夺的肯定者、10322110 篡夺的祈祷者、10363110 安息的团结者（纹章）。
- **又一次靠 QA 纠正直觉**：我原以为不弑的团结者只会召唤 1 个复制，测试里断言 2 个随从；
  官方 QA 明确写着"之后会反复发动…直到战场上限"——运行时行为本来就对（复制体继承当前
  身材，再 -0/-1，逐个 4/5、4/4、4/3、4/2），改的是测试期望。
- 新增 21 个场景（`tests/10003/batch-28-basics.wbotest`）。
- 本批还暴露 8 条新缺口，已登记 S-23…S-30（`self.cost` 条件、牌组无重复条件、
  费用增加、牌组去重、"获得身材增加"事件、极值伤害、纹章数标量、牌组替换）。
- 全量回归：`check` 0 错 0 警；`test` 530 全绿；语料快照更新为 530 个场景。

### 批次 26–27（卡包 10003 前两批）

- 完成 16 张卡：10301110 涸绝的使徒、10312120 树海的战士、10321120 剑圣的同胞、
  10341120 风雪龙人、10351310 前进的暴虐、10352120 新来的守墓人、10371310 丝线突袭、
  10331120 五行修行者、10302110 抗拒叹息之人、10303110 充满勇气之人、
  10373310 歼灭的歌声、10372120 现场工程师、10342210 侮蔑之国、10313310 驱逐的死矢、
  10331310 水晶的指引、10362110 安息的祈祷者。
- 这一批基本没有新原语：跨方选择随从（`field.followers other`）、`where damaged`、
  `repeat own.combo`、`remove lastwords from summoned`、纹章 + 吟唱 + 谢幕曲、
  `not own.attacked_this_turn` 条件、`engage` 与倒计时都直接可用——正是前几批扩语言的回报。
- 新增 17 个场景（`tests/10003/batch-26-basics.wbotest`、`batch-27-spells-and-crests.wbotest`）
  + 1 个 Go 测试（`internal/runner/opponent_evolution_test.go`：手牌里的
  "对手随从超进化时"触发需要对手行动，场景测试做不到）。
- 全量回归：`check` 0 错 0 警；`test` 509 全绿；语料快照更新为 509 个场景。

### 批次 25（导入卡包 10003）

- 用 `./scripts/import_wbarts_packs.sh ../WBArts cards 10003` 导入 73 张骨架
  （身份、身材、五语言文本齐全，效果先标 `unplayable`），语料快照的卡数从 379 更新为 452。
- 10003 现在 4/77：既有 4 张已实现，其余 73 张按文本长度分批铺开。
- `check` 0 错 0 警；`test` 492 全绿。

### 批次 23–24（语言扩展 S-21/S-22 + 卡包 10002 收尾）

- **语言扩展 S-21 完成**：`rally`（协作）计数器与 `rally >= N` 条件。协作统计本场对战中
  进入过自己战场的随从数量；法术与护符不计入，能力召唤的随从立即计入，打出的随从在
  本次结算结束后才计入。最后一条来自官方 QA："协作 19 时打出吉尔达利娅不发动【协作_20】，
  必须先达到 20 再打出"——`../SWB-RL` 的规则在这里与 QA 冲突（它算上自身入场），按项目
  的权威顺序以 QA 为准。
- **语言扩展 S-22 完成**：`summon N card X for own|oppo;`，在指定一方的战场上召唤。
- 完成 2 张卡：10224110 静寂的安纳提玛·吉尔达利娅（协作 20 时入场曲自身超进化；
  己方其他随从入场时对敌方全体造成 1 点伤害；本随从进化时召唤 2 个带突进的铁甲骑士）、
  10224120 雷维翁超越者·尤里乌斯（入场曲在对手战场召唤 2 个骑士；敌方随从入场时
  给它"到对手回合结束为止无法攻击"，并对敌方主战者造成 1 点、回复自己 1 点）。
- **卡包 10002 全部 77 张完成**，接下来开始导入 10003 并继续铺开。
- 新增 5 个场景（`tests/10002/batch-23-rally.wbotest`、`batch-24-enemy-summon.wbotest`）
  + 2 组 Go 测试（`internal/project/rally_test.go`、`internal/runner/rally_test.go`）。
- 全量回归：`check` 0 错 0 警；`test` 492 全绿；语料快照更新为 492 个场景。

### 批次 22（卡包 10002）

- 完成 1 张长文本卡：10214120 缠绕密林·丽梅格（入场曲让敌方 2 个随从到对手回合结束为止
  无法攻击；超进化时给敌方 2 个随从附加"回合结束时对自己的主战者造成 1 点、
  对本随从造成 2 点伤害"）。
- 新增 2 个场景（`tests/10002/batch-22-long-grant.wbotest`）+ 1 个流程测试
  （`internal/runner/granted_ability_flow_test.go` 里的丽梅格用例：附加能力只在
  对手自己的回合结束时发动，场景测试走不到对手回合结束）。
- 10002 只剩 2 张：10224110（【协作_20】关键词）、10224120（在对手战场召唤 + 监听对手入场）。
- 全量回归：`check` 0 错 0 警；`test` 487 全绿；语料快照更新为 487 个场景。

### 批次 21（语言扩展 S-20 + 卡包 10002）

- **语言扩展 S-20 完成**：`other 绑定名`。原来 `other` 只能排除来源自身；
  现在可以排除任意绑定，例如【攻击时】的"非交战对手"写作
  `random victim from oppo.field.followers other opponent;`。运行时无需改动——
  `ExcludeRef` 本来就按任意 Ref 求值。
- 完成 4 张卡：10242210 炎龙之剑（启动 1 破坏自身，强化随从并附加"谢幕曲：召唤炎龙之剑"）、
  10262310 神圣守护（守护随从 +0/+1，并按它的生命值对随机敌方随从造成等量伤害）、
  10234120 精金炼金术师·诺曼（土之秘术 1 + 三选一模式，进化时再发动一次）、
  10263110 速断之刃·阿尼耶丝（疾驰；护符 ≥2 时攻击破坏非交战对手）。
- 新增 7 个场景（`tests/10002/batch-21-engage-and-modes.wbotest`）+ 2 组 Go 测试
  （炎龙之剑的附加谢幕曲要真的召唤出护符；`other 绑定名` 的解析形状）。
  顺带确认 `sum(target, life)` 可直接作为伤害数值："X 为选择的随从的生命值"无需新原语。
- 10002 只剩 3 张：10224110（协作关键词）、10224120（在对手战场召唤）、10214120（长文本附加能力）。
- 全量回归：`check` 0 错 0 警；`test` 485 全绿；语料快照更新为 485 个场景。

### 批次 19–20（S-12 收尾 + 卡包 10002）

- 完成 2 张卡：10204120 飓风天业·格里姆尼尔（入场曲获得纹章；纹章在己方回合结束时、
  若场上有超进化随从则对敌方所有随从造成 2 点伤害）、10233310 帕梅拉的舞蹈
  （土之印 +1 并获得纹章；纹章回合结束时抽 1 张，土之秘术 10 使己方所有随从攻击力/生命值翻倍）。
- 语言侧：**新原语 `double stats 集合;`**（按每个目标自己的当前数值翻倍，已受伤害一并翻倍），
  补上 S-12 的后半；前半"纹章块里的集合条件"复查后发现 S-04 之后已经可用，是误判，
  S-12 现已关闭。
- 新增 6 个场景（`tests/10002/batch-19-crest-conditions.wbotest`、
  `batch-20-stat-doubling.wbotest`），土之秘术两条分别覆盖"10 点足够"与"9 点不足"。
- 全量回归：`check` 0 错 0 警；`test` 478 全绿；语料快照更新为 478 个场景。

### 批次 18（复查 S-03 + 卡包 10002）

- **S-03 复查结论：跨方选择随从一直可用**。`field.followers` 的 `Side` 为空，
  运行时把双方战场合并；`other` 同样只排除来源自身。我此前把它和"随从与主战者
  混合的集合"混为一谈，后者才是真缺口，已登记为 S-19（10524110 等）。
- 完成 1 张卡：10201310 逆向变化（选择战场上的 1 个随从，使其 +2/-2）。
- 新增 2 个场景（`tests/10002/batch-18-cross-side.wbotest`）：指定对手随从（降到 0 生命被破坏）
  与指定自己的随从（10/10 → 12/8）。
- 新增 `internal/project/cross_side_selection_test.go` 固定集合形状，
  避免以后有人"顺手"给空 Side 加限制。
- 全量回归：`check` 0 错 0 警；`test` 472 全绿；语料快照更新为 472 个场景。

### 批次 17（语言扩展 S-06 + 卡包 10002）

- **语言扩展 S-06 完成**：`halve cost 集合;`。取当前费用向上取整的一半，
  可作用于 `own.deck` 这类整副牌组集合；同时确认既有的
  `reduce cost 集合 N minimum M` 也是按集合逐个结算的（"牌组中所有随从 -3" 可直接写）。
- 依据官方 FAQ（WBArts 卡页 QA）：奇数费用向上取整（9 → 5）、重复减半基于当前费用、
  只影响发动瞬间在牌组里的卡——后者决定实现必须是"结算时遍历集合"，而不是给牌组挂持续效果。
- 完成 1 张卡：10244120 绚丽凤凰·小凤（入场曲使牌组中所有卡牌费用减半）。
- 新增 2 个场景（`tests/10002/batch-17-deck-costs.wbotest`）+ 2 组 Go 测试
  （`internal/project/halve_cost_test.go`、`internal/runner/deck_cost_test.go`）。
- 10002 只剩 10 张未实现，全部卡在已登记缺口（S-03 跨方选择、S-12 纹章内集合条件等）。
- 全量回归：`check` 0 错 0 警；`test` 470 全绿；语料快照更新为 470 个场景。

### 批次 16（语言扩展 S-18 + 卡包 10002）

- **语言扩展 S-18 完成**：`if <绑定> damaged { … }`。条件求值新增带帧的
  `conditionIn(c, self, bindings)`，执行与预检两处都使用它；绑定缺失或对象不是随从时为假。
- 完成 1 张卡：10223110 剑士公主·萝泽（爆能强化 5 抽费用 ≤2 的皇家护卫随从并把它降到 0 费；
  突进；攻击受伤的随从时破坏交战对手）。这是 10002 里最后一张有完整文本实现空间的卡，
  其余 11 张都还卡在 S-03 / S-06 / S-12 等已登记缺口上。
- 新增 4 个场景（`tests/10002/batch-16-strike-and-enhance.wbotest`）：爆能与非爆能两侧、
  攻击受伤与未受伤两侧。未受伤那条同时证明它只吃战斗伤害（3/2 撞 10/10 后防守者剩 7）。
- 全量回归：`check` 0 错 0 警；`test` 468 全绿；语料快照更新为 468 个场景。

### 批次 15（语言扩展 + 卡包 10002）

- **语言扩展 S-08 完成（筛选器部分）**：`where damaged` 与 `where attack 比较 整数`。
  10223110 需要的"条件里判断交战对象受伤"仍缺（新登记 S-18）。
- **语言扩展 S-13 完成**：`add 1 card X to hand` 产出 `added` 绑定，可以只强化刚加入的那张。
- 记一条既有约定：`summon copies of` 只接受仍留在手牌或战场的目标，所以 10272120 写成
  `summon copies of target; banish target;`（先复制再消失），而不是照抄文本语序。
- 完成 2 张卡：10271120 猫偶（有超进化随从时把强化过的悬丝傀儡加入手牌）、
  10272120 绝望之王·阿基姆（进化时让攻击力 ≤4 的随从消失并召唤复制）。
- 新增 4 个场景（`tests/10002/batch-15-filters-and-added.wbotest`），
  另有 `internal/project/filter_and_added_test.go` 与
  `internal/runner/added_binding_test.go` 覆盖数值与绑定细节
  （场景只能数手牌张数，强化数值必须在 Go 侧断言）。
- 全量回归：`check` 0 错 0 警；`test` 464 全绿；语料快照更新为 464 个场景。

### 批次 14（语言扩展 + 卡包 10002）

- **语言扩展 S-07 完成**：`when own|oppo follower evolved|super_evolved [other] { … }`。
  事件把本次进化的随从绑定成 `evolved`（`when self …` 仍用 `self`），`other` 排除来源实例，
  这样"自己的其他随从超进化时"能精确表达。超进化同时也算进化事件这一规则不变，
  所以监听 `evolved` 的卡在超进化时也会发动（见 `docs/rules/turn-combat.md`）。
- **语言扩展 S-17 完成**：`set cost T N;`。相对调整继续用 `reduce cost`，需要"费用变为 N"
  时用新原语（全卡文本里有 16 张卡要这个写法，之前只能近似表达）。
- 完成 3 张卡：10241110 庇护的智龙（在手牌中因己方随从超进化降 3 费）、
  10212120 妖精击剑士（同样条件下费用变为 1；入场曲加入妖精）、
  10252110 爆破之翼·圮尤拉（突进；其他随从超进化时给双方 +2/+0）。
- 新增 6 个场景（`tests/10002/batch-14-evolution-events.wbotest`）。其中两个是反向验证：
  普通进化不触发、以及"其他"随从不响应自己的超进化。
- 全量回归：`check` 0 错 0 警；`test` 460 全绿；语料快照更新为 460 个场景。

### 批次 13（语言扩展 + 卡包 10002）

- **语言扩展 S-16 完成**：新固有关键词 `ability_destruction_guard`（「不会被能力破坏」）。
  能力造成的破坏会跳过它，战斗规则造成的破坏（生命归零、必杀）不受影响；
  必杀因此改走新的 `destroyByCombat`。
- 完成 4 张卡：10272310 伊卡洛斯的飞翔（手牌创造物获得突进与谢幕曲抽 1）、
  10261120 恶意的神谕·达姆斯（给敌方随从附加"自己的回合结束时破坏本卡牌"）、
  10271210 创造物弹射器（入场曲加两张核心；启动 3 破坏自身并复制手牌创造物，
  复制体获得"对手回合结束时破坏"）、10273110 暗狱的余晖·贾丝珀（入场曲加过往核心；
  进化时给手牌创造物守护与不会被能力破坏）。
- 新增 7 个场景（`tests/10002/batch-13-granted-abilities.wbotest`）+ 3 个 Go 测试
  （`internal/runner/granted_ability_flow_test.go`）。场景能直接观察到的只有
  "获得关键词 / 加入手牌 / 复制体入场"，附加能力的发动时点、谢幕曲抽牌与
  "不会被能力破坏"必须由 Go 测试走完整流程：出牌 → 附加 → 回合结束 / 破坏。
- 记录一个换算：文本里的"召唤对应数量的复制随从"是「そのコピー1枚」的简中误译，
  一律按 1 张复制处理（先例：10173140、10274120）。
- 全量回归：`check` 0 错 0 警；`test` 454 全绿；语料快照更新为 454 个场景。

### 批次 12（语言扩展 + 卡包 10002 / 90000）

- **语言扩展 S-01 完成**：`remove lastwords from 集合` 与 `remove all abilities from 集合`。
  失去的能力记在实例上（不改变卡牌定义），同一个块里对 `summoned` 执行即可让衍生体
  不再无限触发谢幕曲；已排队的同类触发会一起作废，连击快照里也会保存抑制状态。
- 完成 3 张卡：90051140 腐臭的僵尸（谢幕曲召唤一个已失去谢幕曲的自己）、
  10252120 尸兵（入场曲召唤 2 个腐臭的僵尸）、10251310 诅咒派对（把怨灵、骸骨士兵、
  腐臭的僵尸各 1 张加入手牌）。90051140 之前因为 S-01 挂起，连带卡的这两张也一并解锁。
- 新增 3 个场景（`tests/10002/batch-12-lastwords-removal.wbotest`）。场景只能观察
  "死后场上剩下 1 个僵尸"，链是否真的停止由 `internal/runner/ability_removal_test.go`
  连杀两次验证（第二次死亡不再产生新个体，墓场资源停在 2）。
- 全量回归：`check` 0 错 0 警；`test` 447 全绿；语料快照更新为 447 个场景。

### 批次 11（语言扩展 + 卡包 10002）

- **语言扩展 S-14 完成**：`fused.cost` / `fused.distinct` 不再限于 `fusion` 块。判断点在
  "打出时 / 入场时"的卡现在能直接写 `if fused.distinct >= 1 { … } else { … }`。
- **语言扩展 S-15 完成**：测试 DSL 增加 `attack_limit`、`damage_reduction` 两个实例字段断言，
  并把写错字段名从静默不相等改成检查期报错（四处白名单同步）。
- 完成 5 张卡：10213310 花园的指引（融合后改抽 2 张）、10234110 暴食的安纳提玛·拉拉安瑟姆
  （灵气 + 土之秘术谢幕曲 + 超进化破坏 2 个）、10251110 银色子弹·雷文（毁灭 + 进化时
  破坏 2 个并自伤 2）、10271110 引擎剑士（入场曲与进化时各召唤 1 个攻击创造物并把过往核心
  加入手牌）、10274120 精神武艺·迦尔拉（入场曲复制手牌里的创造物随从 + 超进化后可攻击 2 次）。
- 新增 13 个场景（`tests/10002/batch-11-effects.wbotest`）。融合的两个分支被拆开：
  场景测试覆盖"未融合抽 1"与"融合指令合法 / 无合法材料时非法"，"融合后抽 2"由
  `internal/runner/fused_play_effect_test.go` 端到端覆盖
  （`.wbotest` 一个 `action` 只能有一个主动作，`fuse` 与 `play` 无法连写）。
- 两个新踩到的坑记在场景注释里：`own.shadows` 按规则把**被破坏的护符**也算一点
  （见 `docs/rules/turn-combat.md`）；没有合法材料时 `fuse` 直接是
  `illegal fusion_material_required`，不会再产生一次选择请求。
- 顺手清理：本文档此前有一批行首残留的 `+`（补丁前缀被当成正文写进文件），已去掉。
- 全量回归：`check` 0 错 0 警；`test` 444 全绿；语料快照更新为 444 个场景。

### 批次 09–10（语言扩展 + 卡包 10002）

- **语言扩展 S-04 完成**：`if count(集合 [where …]) 比较 整数`。条件不再只接受标量，
  可以直接比较"集合里有多少个满足筛选的对象"；`sum(...)` 同样可用。
- 完成 6 张卡：10242110 鲸鱼骑兵、10201110、10222110、10231110、10262110，
  加上批次 08 的 10222310 共 7 张受益于本次扩展。
- 新增 10 个场景（`tests/10002/batch-09-count-conditions.wbotest`、
  `batch-10-count-cards.wbotest`），每张卡都覆盖条件成立与不成立两侧。
- 挂起 1 张（S-13）：10271120，原因是"加入手牌的那张牌"没有绑定可以引用。
- 又一次踩到身材假设：`{ super_evolved; }` 只设形态不加身材，断言必须从卡表算。
- 全量回归：`go test ./...` 全绿（一次空条件导致 panic 已修，语料快照更新为 431 场景）；
  `check` 0 错 0 警；`test` 431 全绿。

### 批次 08（语言扩展 + 卡包 10002）

- **语言扩展 S-05 完成**：新增 `enhance N replaces { … }`。爆能强化默认是"追加"，
  文本写"改为"的卡现在能表达成替换：支付该档时跳过基础效果与入场曲。
- 完成 1 张：10222310 焰火占卜（普通档随机 1 个随从 4 点；爆能 4 改为随机 3 个）。
- 新增 2 个场景（`tests/10002/batch-08-enhance-replaces.wbotest`）。第二个场景能证明替换生效：
  三个 0/8 的随从结算后都是 0/4，说明没有额外多打一次基础效果（否则会有一个被打到 0）。
- 术语更正：`alt_modes.type_key` 的中文对应是 **crest=纹章、faith=信仰、crystallize=结晶、
  accelerate=激奏**（我此前把 accelerate 写成"加速"，已改）。
- 全量回归：`go test ./...` 全绿（语料快照更新为 421 场景）；`check` 0 错 0 警；`test` 421 全绿。

### 批次 07（纹章卡，卡包 10002）

- 纠正：**纹章文本一直在卡表的 `alt_modes` 里**（63 条纹章、5 条 faith、5 条 crystallize、
  5 条 accelerate，都带五语言文本）。我此前只查了 `skill_texts`，误报成数据缺失，S-10 已关闭。
- 完成 6 张纹章卡：10214110 翅翼女王·提泰妮娅、10232110 否定的咏唱·芭赛特、
  10243110 苍海的制裁·尼普顿、10264120 呜咽的圣骑士·维尔伯特、10263310 疯狂的恩宠、
  10254120 流动堕落的冥河·凯伦。纹章块的 `name` / `text` 直接取自 `alt_modes`（含各语言
  的冒号形式与 ruby 标记），只手工写 DSL 效果行。
- 新增 8 个场景（`tests/10002/batch-07-crests.wbotest`）。
- **测试模式发现**：纹章的"回合开始 / 吟唱递减 / 谢幕曲"用 `action { end_turn; }` 验证，
  把纹章放在 `oppo` 上（自己结束回合 → 对手回合开始触发）；`advance turn_start own`
  不会让吟唱递减。纹章的"随从进入战场时"类监听可以直接用初始状态的纹章 + 自己出牌验证。
- 挂起 2 张（S-12）：10204120、10233310。
- 全量回归：`go test ./...` 全绿；`check` 0 错 0 警；`test` 419 个场景全部通过。

### 批次 06（语言扩展 + 卡包 10002）

- **语言扩展 S-02 完成**：新增 `draw N for own|oppo`，解锁"让对方抽牌"。
- 完成 1 张：10221310 商谈成立（自己抽 2 张、对方抽 1 张）。
- 新增 2 个场景（`tests/10002/batch-06-draw-owner.wbotest`），其中一个断言**对方手牌**
  确实增加了 1 张（用 `oppo.hand count card …` 验证，而不是只验证自己）。
- 语料快照测试（`internal/ir/decode_test.go`、`internal/project/integration_test.go`）
  里的卡数与场景数需要随批次更新：253 → 379、354 → 411。已在注释里写明这是全卡覆盖
  工作的固定维护项。
- 新登记 S-11：`.wbotest` 不能声明先手，所以"抽牌耗尽判负"只能由 Go 测试覆盖。
- 全量回归：`go test ./...` 全绿；`check` 379 张卡 0 错 0 警；`test` 411 个场景全部通过。

### 批次 01（卡包 10002 + 90000 衍生卡）

- 完成 14 张：10211110 狂野女孩、10221120 暗斗的忍者大师、10252310 使唤蝙蝠、
  10211310 森林的游行、10251120 怨恨的栽培者、10273310 心有灵犀的共斗、
  10212110 热情的精灵·莱昂内尔、10241120 飞跃的银白幼龙、10262120 有洁癖的审判者、
  10261210 流光香炉、10222120 平凡骑士·拉奇尔、10241310 虎鲸的呼声、
  90041130 大海虎鲸、90074130 维多利亚。
- 新增 17 个场景（`tests/10002/batch-01-basics.wbotest`），覆盖：关键词、召唤批量衍生体、
  谢幕曲加手牌、入场曲条件（觉醒/非觉醒两侧）、消失、启动费用与回复上限、按类型检索抽牌、
  进化回复能量、攻击时触发（攻击随从触发 / 攻击主战者不触发）。
- 挂起 3 张（S-01）：90051140、10251310、10252120。
- 编译器反馈：17 张里 15 张一次通过；2 张暴露了写法问题（`self.attack` 作为伤害量是正确写法；
  `remove lastwords` 不存在）。
- 全量回归：`check` 379 张卡 0 错 0 警；`test` 371 个场景全部通过。

### 批次 02（卡包 10002）

- 完成 10 张：10221110、10204110、10231120、10242120、10261110、10231310、
  10212310、10243310、10202110、10203110。
- 新增 15 个场景（`tests/10002/batch-02-effects.wbotest`），覆盖：随机多目标伤害、
  消失敌方护符、手牌返回牌组后按类型抽牌、入场曲+进化时双段召唤、按形态筛选目标、
  `require` 缺少目标时整条指令非法、土之秘术追加伤害、连击门槛改变发动次数、
  手牌费用下降下限、超进化解禁条件两侧、集合进化只影响未进化随从。
- 挂起 1 张（S-04）：10242110。
- 语言发现：集合进化可以写（`evolve own.field.followers silent;`，已进化目标不受影响）；
  `reduce cost T N minimum M` 可用；解禁条件写 `if own.superevolve_unlocked`；
  但"集合计数/存在性"作为 `if` 条件不可用。
- 全量回归：`check` 379 张卡 0 错 0 警；`test` 386 个场景全部通过。

### 批次 03（卡包 10002）

- 完成 9 张：10233110、10213110、10203120、10211120、10223120、10244110、
  10232120、10253110、10272110。
- 新增 13 个场景（`tests/10002/batch-03-effects.wbotest`），覆盖：交战时魔力增幅、
  入场曲+进化时两段效果、爆能强化追加效果、连击门槛改变目标数量、超进化赋身材、
  觉醒前后加入手牌两侧、土之印累加、变身（手牌中的随从变成指定卡）。
- 挂起 2 张（S-05 爆能强化"改为"、S-06 牌组费用变更）：10222310、10244120。
- 语言发现：**可以直接变身手牌中的随从**（`transform target into card C`）、
  `add 1 earthsigil;` 累加到已有土之印、`spellboost own.hand 1` 驱动手牌费用下降、
  `when own follower summoned` 等事件监听可用。
- 教训（写进流程）：断言里的身材必须来自卡表而不是估计——本批有 4 个场景第一版
  因为把 1/1 的衍生体当成 2/2、把 2/2 当成 3/3 而失败，修正后全部通过。
- 全量回归：`check` 379 张卡 0 错 0 警；`test` 399 个场景全部通过。

### 批次 04（卡包 10002）

- 完成 4 张：10253120（按其他随从数量加攻击力的疾驰随从）、10232310（魔力增幅累积 X
  的分配伤害）、10274110（召唤维多利亚 / 给人偶随从守护 / 进化召唤改良型悬丝傀儡）、
  10264110（从牌组随机召唤三种低费随从、超进化给其他随从 +0/+2 与灵气）。
- 新增 8 个场景（`tests/10002/batch-04-effects.wbotest`）。其中两条覆盖边界：
  没有其他随从时不加攻击力；分配伤害只剩一个候选时全部落在它身上（分配是随机的，
  `.wbotest` 一个 action 只能有一个主动作，所以用单候选把随机性消掉）。
- 挂起 3 张（S-07 两张、S-08 一张）。
- 语言发现：`buff self +count(...)/+0` 合法（但 `count` 里不能写 `other`）；
  分配伤害写 `damage <集合> N distributed`；`when own follower summoned where trait ...`
  可以给"该次事件召出的随从"加关键词；测试的状态里可以直接覆盖命名计数器
  （`spell source = 10232310 { counter x 2; }`），用来跳过前置增幅步骤。
- 全量回归：`check` 379 张卡 0 错 0 警；`test` 407 个场景全部通过。

### 批次 05（卡包 10002，模式类）

- 完成 1 张：10254110 九尾的巫女（【模式】二选一：召唤 4 只一尾狐 / 随机破坏 4 个敌方
  随从并回复主战者 4 点）。
- 新增 2 个场景（`tests/10002/batch-05-modes.wbotest`），分别覆盖两个模式分支。
- 语言发现（重要）：**模式块可用**，写法是
  `mode { option N { label chs/eng/jpn/kor/cht "…"; <效果> } … }`，
  标签的多语言文本可以直接取自卡表原文。全卡有 **49 张**带【模式】的卡，其中 5 张
  已在 10000/10001 里写好，其余可照此推进。
- 全量回归：`check` 379 张卡 0 错 0 警；`test` 409 个场景全部通过。
