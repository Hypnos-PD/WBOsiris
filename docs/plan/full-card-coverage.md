# 全卡覆盖计划与挂起登记

目标：把 `../WBArts/data/cards.json` 里的全部卡牌写成 WBO 规则（含 90000 卡包的衍生卡），
每张卡都配至少一个 `tests/**/*.wbotest` 场景，`check` 与 `test` 在任何提交点都必须全绿。

## 口径与政策

1. **范围**：全部卡牌。已导入骨架但效果还没实现的卡用导入器写入的 `unplayable;` 标记，
   `scripts/card_coverage.sh` 因此不会把它算成可玩——覆盖率只统计真正写完的卡。
   判定条件是"`effect` 块里**只有**一条 `unplayable;`"：真正无法使用的卡
   （未来核心、过往核心）还带着融合块，应当算作已完成（见 [脚本说明](../tooling/cli.md)）。
2. **每张卡都要有场景测试**。纯关键词卡也必须至少有"打出后拥有该关键词"的断言；
   触发类效果要覆盖触发与不触发两侧。
3. **遇到缺少原语就挂起并登记**，不写近似实现、不用 `unplayable` 之外的写法伪装完成。
   挂起卡保留 `unplayable;`，并在下面的登记表里写明缺什么、影响哪些卡、草稿是什么。
4. **权威是官方卡牌文本与 QA**（见 [规则与素材来源契约](../authority.md)）。
   `../SWB-RL` 的结构化规则只作**第二意见**：用来交叉验证触发时机、目标与参数，
   以及在难以判断时给出一个可执行的假设。它与文本冲突时以文本为准。

## 现状（生成于 `node scripts/card_worklist.mjs`）

```text
总计 904 · 已完成 746 · 未实现(骨架) 82 · 未导入 76
卡包      总数  已完成  未实现  未导入
10000        56      56       0       0
10001       142     142       0       0
10002        77      77       0       0
10003        77      75       2       0
10004        76      73       3       0
10005        76      66      10       0
10006        76      74       2       0
10007        77      77       0       0
10008        78      13      65       0
10009        76       0       0      76
90000        93      93       0       0
未完成卡按文本复杂度：中(41–100字) 162 · 短(≤40字) 65 · 长(>100字) 10
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
| S-42 | `damage_cap N`（实例伤害上限）与主战者 `damage_taken_up`（受到的伤害 +1） | `internal/project/validate.go`（固有词形状与 `abilities` 集合）、`strict_validate.go`（固有能力/关键词形状表）、`typed_ir.go`（写入 `IntrinsicState`）、`internal/ir/decode.go`（固有状态与关键词白名单）、`internal/runner/runner.go`（`instance.damageCap`、`resetCardState`、`modifyDamage` 在减伤之后压上限；`damageLeaderFrom`/`damageLeaders` 在屏障判定前加一）、`internal/runner/continuation.go`（`DamageCap` 快照与恢复）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/runner/damage_cap_test.go`（8/3/2 三种输入、"受伤 +1"、与屏障同时存在时为 0）；卡片 10401110、10464120、10474120、10444110 + 7 个场景 |
| S-43 | `own.crests` / `oppo.crests` 作为可操作目标集合 | `internal/project/validate.go`（目标集合接受 `crests`）、`typed_ir.go`（纹章集合的成员固定为 `card`）、`internal/ir/decode.go`（只接受 `zone:"crests"` 且 `member:"card"`）、`internal/runner/history.go`（`crestZoneRef`：只有显式纹章集合才把纹章纳入目标）、`internal/runner/runner.go`（`destroyTargets` 对纹章走 `expireCrest`）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/crest_target_and_play_frame_test.go`、`internal/ir/crest_test.go`（拒绝泛化的纹章区域引用）、`internal/runner/crest_test.go`（纹章不会被当成普通卡移动或变身）；卡片 10453310、10454120 + 4 个场景 |
| S-44 | 一次打出的效果块共享打出帧 | `internal/runner/runner.go`（`commitPlay` 为外层效果、入场曲与爆能强化创建同一个 `frame`）、`internal/project/validate.go`（把入场曲的输出并入后续 `enhance` 的可见绑定；`producedBindings` 补上 `added`；登记 `played` 事件绑定）；文档 `cards.md`/`grammar.md` | Go 单测 `internal/project/crest_target_and_play_frame_test.go`（声明顺序两侧）、`internal/runner/enhance_play_frame_test.go`（爆能强化的复制体获得【毁灭】且本体没有）；卡片 10424110 + 2 个场景 |
| S-45 | `add copies of … to hand`（复制同名卡加入手牌） | `internal/project/validate.go`（`add` 形状）、`typed_ir.go`（`add_copies`）、`internal/ir/encode.go`/`decode.go`、`internal/runner/execute.go`（按目标卡牌定义创建实例并用 `putInHandOrOverdraw` 加入手牌）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/copies_and_hand_summon_test.go`（形状、目标绑定、拒绝未知目的地）；卡片 10443310 + 3 个场景 |
| S-46 | `summon <绑定>`（把手牌对象召唤到战场） | `internal/project/validate.go`（`summon` 两 token 形状）、`typed_ir.go`（`summon_from_hand`）、`internal/ir/encode.go`/`decode.go`、`internal/runner/execute.go`（`move` 到手牌之外的战场、置入场等待、发 `summoned` 事件）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/copies_and_hand_summon_test.go`（形状、拒绝未定义绑定）；卡片 10412110 + 2 个场景 |
| S-47 | 筛选器 `attacked this turn` / `not attacked this turn` | `internal/project/validate.go`（`parseWhere` 词条）、`typed_ir.go`（`filterIR` 谓词）、`internal/ir/encode.go`/`decode.go`（谓词白名单）、`internal/runner/execute.go`（`matches` 读取 `attacksUsed`）；文档 `cards.md`/`grammar.md` | Go 单测 `internal/project/attacked_filter_test.go`（合取保留两个词条、两种极性、拒绝其它 `not` 形状）；卡片 10464110 + 3 个场景 |
| S-50 | `<绑定>.attack\|life\|cost` 数值 | `internal/ir/amount.go`（`validNumericExpr`、`decodeNumericValue`）、`internal/ir/counters.go`（`ValidBindingName`）、`internal/project/numeric.go`（生成 `binding_scalar`）、`internal/project/validate.go`（`parseEffectAmount` 形状）、`internal/runner/numeric.go`（按绑定求值）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/binding_scalar_test.go`（生成 `binding_scalar`、拒绝不支持的字段）；卡片 10473110 + 2 个场景 |
| S-53 | `raise countdown <集合> N` | `internal/project/validate.go`（`raise` 接受 `countdown`）、`typed_ir.go`（沿用 `adjust_entity_field` 的正增量） | Go 单测 `internal/project/binding_scalar_test.go`（`Field=countdown`、拒绝缺增量）；卡片 90064310 + 1 个场景 |
| S-54 | `set_attack_limit <目标> N` | `internal/project/validate.go`（`set_attack_limit` 由 `self` 放宽为任意 `value_ref`） | Go 单测 `internal/project/binding_scalar_test.go`（目标保留为绑定）；卡片 90034350 + 1 个场景 |
| S-55 | 护符固有关键词 `aura` | `internal/project/strict_validate.go`（非随从只对 `aura` 开例外） | Go 单测 `internal/project/amulet_aura_test.go`（`aura` 编入 intrinsic、护符上的 `ward` 仍被拒绝）；卡片 90064210 + 1 个场景 |
| S-56 | `raise\|reduce cost T N until …`（带期限的费用修改） | `internal/ir/model.go`（`AdjustEffect.Until`）、`internal/ir/encode.go`/`decode.go`（形状与期限白名单）、`internal/project/validate.go`/`typed_ir.go`（解析）、`internal/runner/execute.go`（按到期侧记录差量）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/runner/temporary_cost_test.go`（到期只撤销自己的差量）；卡片 90044310 + 2 个场景 |
| S-59 | `add N card C to deck`（加入牌组） | `internal/project/validate.go`/`typed_ir.go`（目的地放宽到 `deck`）、`internal/ir/decode.go`（`add_card`/`add_copies` 允许 `deck`）、`internal/runner/execute.go`（随机位置插入牌组）；文档 `cards.md`/`grammar.md` | Go 单测 `internal/project/add_to_deck_test.go`（目的地与 `added` 输出、拒绝未知目的地）；卡片 10551310 + 1 个场景 |
| S-68 | `during own\|oppo turn` 用于回复事件 | `internal/project/strict_validate.go`（事件模式放宽到玩家侧事件）、`internal/ir/decode.go`（`duringTurn` 允许 `healed`） | Go 单测 `internal/project/healed_turn_event_test.go`（回复事件带上回合窗口、`self survives damage during …` 未回归）；卡片 10563110 + 2 个场景 |
| S-62 | 抽牌事件 `card_drawn` | `internal/project/strict_validate.go`（`card drawn` / `self drawn` 事件形状）、`typed_ir.go`（映射到 `card_drawn`、`self drawn` 隐式 `sourceZone=hand`）、`validate.go`（登记 `drawn` 绑定）、`internal/ir/decode.go`（事件白名单与自身监听的形状）、`internal/runner/execute.go`（`triggerDrawn` 按每张抽到的卡派发）；文档 `cards.md`/`grammar.md` | Go 单测 `internal/project/drawn_event_test.go`（双方抽牌、`during` 窗口、`self drawn`、`drawn.cost` 作为数值）；卡片 10561120、10522110、10562120 + 8 个场景 |
| S-70 | `summon random N card A or card B [for own\|oppo]`（随机池召唤） | `internal/ir/summon_pool.go`（新节点与池校验）、`internal/ir/decode.go`（解码与卡牌引用检查）、`internal/project/summon_pool.go`/`typed_ir.go`/`validate.go`（解析与形状）、`internal/runner/summon_pool.go` + `session.go`（随机抽取并召唤）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/summon_pool_test.go`（池内容、`for oppo`、拒绝单卡池）；卡片 10564120 + 3 个场景 |
| S-58 | `own\|oppo.hand\|deck has N same cost`（同费用张数） | `internal/ir/model.go`（`SameCostCondition`）、`internal/ir/decode.go`、`internal/project/strict_validate.go`/`validate.go`/`typed_ir.go`、`internal/runner/execute.go`（按当前费用统计最多同费张数）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/same_cost_and_summoned_all_test.go`；卡片 10553310 + 2 个场景 |
| S-71 | `when own\|oppo follower attacks [leader]`（宣告攻击事件） | `internal/ir/model.go`（`EventTrigger.TargetKind`）、`internal/ir/decode.go`（事件白名单与目标种类校验）、`internal/project/strict_validate.go`/`typed_ir.go`/`validate.go`（事件形状与 `attacker` 绑定）、`internal/runner/runner.go`（事件带上攻击方一侧、绑定名 `attacker`）、`internal/runner/trigger_index.go`（按目标种类过滤）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/attack_event_test.go`（双方、`attacks leader`、筛选与临时增益）；卡片 10474110、10544120 + 8 个场景 |
| S-73 | `mode random N { … }`（随机模式） | `internal/ir/model.go`（`ModeEffect.Random`）、`internal/ir/decode.go`、`internal/project/validate.go`/`strict_validate.go`/`typed_ir.go`、`internal/runner/session.go`（`pushRandomMode`：随机选 N 个不同选项并按编号入栈）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/random_mode_test.go`（编译保留 `random` 与数量、拒绝 0 与单选项）；卡片 10532310 + 2 个场景 |
| S-75 | `where not <词条>`（否定筛选） | `internal/ir/model.go`（`NotPredicate`）、`internal/ir/decode.go`（解码、卡牌引用检查）、`internal/project/validate.go`（`negatedWhereTerm`）、`typed_ir.go`（`filterIR`）、`internal/runner/execute.go`（`matches` 取反）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/not_filter_test.go`；卡片 10603210 + 1 个场景 |
| S-77 | `where enhanced`（本次通过爆能强化打出） | `internal/runner/runner.go`（`instance.enhancedPlay` 在 `applyPlaySetup` 前设置）、`internal/project/validate.go`/`typed_ir.go`（筛选词条）、`internal/ir/decode.go`/`encode.go`（谓词白名单）、`internal/runner/execute.go`（求值）；文档 `cards.md`/`grammar.md` | Go 单测 `internal/project/enhanced_filter_test.go`；卡片 10622310 + 3 个场景 |
| S-78 | `where lastwords`（拥有【谢幕曲】筛选） | `internal/project/validate.go`（`parseWhere`/`negatedWhereTerm` 接受 `lastwords`）、`typed_ir.go`（`has_lastwords` 谓词）、`internal/ir/decode.go`/`encode.go`（谓词白名单）、`internal/runner/execute.go`（按卡牌定义的 `lastwords` 触发判定）；文档 `cards.md`/`grammar.md` | 卡片 10663210、10664110 + 3 个场景（含"只破坏过同一种护符时只召唤一张"）。局限：只看卡牌定义的固有能力，`grant` 临时获得的【谢幕曲】不算 |
| S-79 | 破坏历史召唤的 `distinct names`（随机 N 种各 1 张） | `internal/ir/history_summon.go`（`DistinctNames`）、`internal/ir/decode.go`（解码并写回效果）、`internal/project/history_summon.go`（解析 `distinct names`）、`internal/runner/history_summon.go`（每抽一张后按卡牌 ID 排除同名候选）；文档 `cards.md` | Go 单测 + 卡片 10664110 + 2 个场景。解码器最初漏了 `DistinctNames` 回填，运行期恒为 `false`，被"只有一种护符时只召唤一张"的场景抓住 |
| S-80 | 非法术卡的选择不该阻塞打出／进化：`require` → `choose` | 29 张随从/护符卡的 34 处 `require`（入场曲、爆能强化、进化时、超进化时）改写为 `choose`；`engage`（启动能力）与法术顶层的 `require` 保留；文档 `cards.md` 写明适用边界 | 官方 QA：[随从/护符没有可选择的手牌也能使用、能力也发动，法术不能](https://shadowverse-wb.com/chs/usersupport/?tab=2#q2lo-vv80cvw)、[卡西乌斯没有创造物随从时照样打出并造成 0 点伤害](https://shadowverse-wb.com/chs/usersupport/?tab=2#49odoxq3_z)。新增 `tests/10004/batch-73-target-availability.wbotest` 7 个场景（打出、进化、抽牌、破坏、启动能力仍不可用） |
| S-63 | `draw N from deck where … distinct names`（抽取 N 种） | `internal/ir/model.go`（`DrawEffect.DistinctNames`）、`internal/ir/encode.go`/`decode.go`（编解码与形状校验）、`internal/project/typed_ir.go`/`validate.go`（解析与形状）、`internal/runner/execute.go`（每抽一张排除同名候选，张数上限取不同卡名数）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/draw_owner_test.go`（编译保留标记、编码解码后仍在、拒绝无筛选/`all`/缺 `names`）；卡片 10574120 + 4 个场景 |
| S-67 | `raise maxlife` / `reduce maxlife`（主战者生命上限增减） | `internal/ir/deck_replace.go`（`Delta`）、`internal/ir/decode.go`（新 kind 与增量范围校验）、`internal/project/deck_replace.go`（`raise|reduce maxlife` 解析）、`typed_ir.go`/`validate.go`（语句分发）、`internal/runner/deck_replace.go`（夹在 1..65535 并把当前生命降到上限）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/deck_replace_test.go`（增量编译、编解码保留、拒绝负数/越界/绑定量）；卡片 10534110 + 4 个场景 |
| S-84 | 入场监听加回合窗口：`when own follower summoned during own turn` | `internal/ir/decode.go`（`duringTurn` 允许 `follower_summoned`；运行时的 `triggerTurnMatches` 本来就通用，项目校验也早已接受这种写法，只有解码白名单没跟上）；文档 `cards.md` | 卡片 10754120 + 2 个场景（三个僵尸各触发一次"入场时打击对手主战者1点"，以及 10724110 后续直接可用） |
| S-83 | `entered_artifacts`（本场对战中进入过战场的创造物·随从种类数） | `internal/runner/runner.go`（`player.enteredArtifacts`、初始状态记录）、`internal/runner/execute.go`（`triggerSummoned` 里 `recordEnteredArtifact`）、`internal/runner/numeric.go`/`assert.go`（数值与断言）、`internal/ir/amount.go`/`test_decode.go`、`internal/project/strict_validate.go`、`internal/runner/session.go`/`continuation.go`（克隆与存档）；文档 `cards.md`/`tests.md` | 卡片 10771120、10771310、10772120、10773310、10774110、10774120 + 9 个场景（含"种类不足/达标"两侧与超进化后检查顺序） |
| S-57 | 归属判断（`count(own.<区域> where card <绑定>)`） | 无需新节点：`same_card` 谓词 + 区域计数；但修好了 `conditionIn` 里 `CountCondition` 丢帧的问题（`internal/runner/execute.go`） | 卡片 90064320 + 2 个场景（敌方随从时不加牌、自己的护符时追加 2 点伤害并加牌） |
| S-66 | `banish` 输出 `banished` | `internal/project/typed_ir.go`（写入输出）、`validate.go`（登记绑定）、`internal/ir/encode.go`/`decode.go`（形状白名单）、`internal/runner/execute.go`（记录实际消失的实例）；文档 `cards.md`/`grammar.md`/`compiler/ir.md` | Go 单测 `internal/project/banish_output_test.go`（输出保留、`count(banished)` 作为分配伤害量）；卡片 10543110 + 2 个场景 |
| S-61 | `summoned_all`（一次结算的全部召唤） | `internal/runner/bindings.go`（`bindSummoned`）、`internal/runner/execute.go`/`session.go`（所有召唤类效果改为写入累计绑定）、`internal/project/validate.go`（登记 `summoned_all`）；文档 `cards.md`/`grammar.md` | Go 单测 `internal/project/same_cost_and_summoned_all_test.go`；卡片 10571110 + 3 个场景 |

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
| S-24 | ~~牌组"没有重复卡牌"条件~~ **已解决**：`own|oppo.deck has [no] duplicates` | 已解锁 10301310 至高的凌驾 | 新增 `DeckDuplicatesCondition`：按卡牌 ID 扫描该玩家牌组，第二次出现即判定有重复；带 `no` 的形式取反，空牌组视为没有重复。 |
| S-25 | ~~费用增加（`+N`）~~ **已解决**：`raise cost T N;` | 已解锁 10333310 虚假的术式；90044310 等后续卡包直接可用 | 复用 `AdjustEntityField(delta=+N, minimum=0)`，没有新增 IR 节点。顺带给 `damage`/`heal` 补上 `other [绑定]`，写法与 `buff`/`add` 一致。 |
| S-26 | ~~牌组去重~~ **已解决**：`banish duplicates in own|oppo.deck;` | 已解锁 10303210 试炼的石板 | 新效果种类 `banish_duplicates`：按牌组顺序保留每种卡牌的第一张，其余移入消失区（走普通区域移动，不是破坏）。 |
| S-27 | ~~身材增减事件~~ **已解决**：`when self stats increased`、`when own\|oppo follower life decreased` | 已解锁 10361120 圣骑士团员、10314110 不弑的继承者·库露露；全卡另有 2 张身材增加监听（10812110、10814120） | 身材增加在 `buff_stats` 上发独立事件（临时增益到期不算增加）；生命值减少用两条来源——减益发独立事件，伤害则让原有 `damaged` 事件一并匹配 `life_decreased` 监听（不额外发事件，避免污染事件流）。`set life` 的设置不算减少。都支持 `once per own turn`。 |
| S-28 | ~~按极值筛选的伤害~~ **已解决**：`damage 集合 数值 highest\|lowest [base.] attack\|life\|cost` | 已解锁 10341310 雷霆之怒；后续"生命值/攻击力最大"的伤害与回复直接可用 | `TargetEffect` 新增 `Extremum`，运行时在过滤前用 `extremumCandidates` 收窄目标；`damage all.leaders … highest life` 走 `damageExtremumLeaders`，并列时双方都挨打（场景测试覆盖并列）。 |
| S-29 | ~~纹章数标量~~ **已解决**：`own.crests` / `oppo.crests` | 已解锁 10364120 绝望的显现·玛温 | 玩家标量新增 `crests`（本方纹章实例数）；"分配 X 点伤害"复用既有的 `distributed overflow <leader>` 语义（按战场顺序填满每个随从当前生命，余量溢出给主战者），场景测试覆盖了溢出与不溢出两种情况。 |
| S-30 | 牌组替换与牌组底部变身 | 10304110 绝大的显现·麦哲佩恩（纹章"使自己的牌组变为麦哲佩恩牌组；使牌组底部的亡者之王卡牌变身"） | 需要用预置牌组替换当前牌组，并支持"牌组底部第 N 张"变身；属于新的牌组级操作。 |
| S-31 | ~~主战者获得关键词~~ **已解决**：`add barrier to own.leader;` | 已解锁 10362210 安息的神殿 | 玩家状态新增 `leaderAbilities`，主战者伤害路径先检查【屏障】（把下一次伤害降为 0 并消耗），连击快照一并保存。官方 QA 明确与"受到的伤害 +1"同时存在时仍是 0。 |
| S-32 | ~~进化与超进化共用一次选择~~ **已解决**：`where card <绑定>` + `superevolve extends evolve` | 已解锁 10334110 真理的继承者·蓓哈丽雅；全卡另有 12 张"同名"文本（10354110、10443310、10572310、90074320…）可继续用同一机制 | 筛选器支持 `card <绑定>`（同一卡牌定义），且 `filter`/`matches` 现在带上当前帧以便解析绑定；`extends` 计划的两步共享绑定帧，超进化因此能沿用进化时的选择。 |
| S-33 | ~~动态倒计数减少~~ **已解决**：`reduce countdown T 数值引用` | 已解锁 10362210 安息的神殿、10363210 闪耀的失意 | `AdjustEffect` 新增 `DeltaExpr`（动态增量在结算到该语句时求值），IR 用 `deltaValue` 字段保存。 |
| S-34 | ~~"费用发生过变化的随从"事件~~ **已解决**：筛选器 `where cost changed` | 已解锁 10332210 真理的研究设施 | 实例新增 `costChanged`：加费/减费/设置费用（`reduce`/`raise`/`set cost`/`halve cost`）都会置位，连击快照一并保存；配合"自己使用卡牌时"事件即可筛出这类随从。 |
| S-35 | ~~信仰值（faith）机制~~ **已解决**：`faith { counter value 0; … }`、`faith N { … }`、`distribute faith <卡牌ID> { … }`、`grant faith { … }` | 已解锁 10634120 古旧天晶、10664120 古旧天书、90034330 天晶深渊、10614120 古旧天枪、10624120 古旧天剑 | 信仰实体与纹章共用结构/区域，开局按初始牌组放置；信仰值用 `counter value`；`faith N { … }` 是"信仰值-N 后执行"的支付块（不足则整块跳过）；`distribute faith` 把信仰值逐点随机分配（不消费）；`grant faith { … }` 给信仰附加任意事件监听。**仍缺**：把"模式选择数 +1"附加到信仰（10354110 混融的继承者）。 |
| S-36 | ~~多选模式（选择 N 个能力）~~ **已解决**：`mode N { … }` | 已解锁 10353310 叫唤与憎恶；10852310 后续可用 | 回放协议新增 `SelectedOptionIDs`（多选）与 `mode 1, 3;` 测试写法；请求的 Min/Max 数量等于 N，选项去重并按编号顺序结算，连击快照沿用挂起请求里保存的数量。 |
| S-37 | 条件合取 | 10304120 涸绝的显现（"若自己和对手的能量点最大值为 10"） | 条件没有 `and`；该卡用嵌套 `if own.maxpp == 10 { if oppo.maxpp == 10 { … } }` 绕过，但更复杂的条件需要合取/析取。 |
| S-38 | ~~"自己使用卡牌时"事件~~ **已解决**：`when own card played [other] [where …]` | 已解锁 10323110 篡夺的团结者、10324120 空绝的显现·奥克托丽丝；全卡另有 14 张"自己使用…时"（10571120、10572120、10822110、90021210…） | 打出后发 `card_played` 事件并绑定 `played`。顺带修好"纹章用 `reduce countdown self 1` 推进吟唱"：纹章不在战场，需要显式纳入目标，归零时走纹章退场（触发谢幕曲）。 |
| S-39 | ~~动态随机/选择数量~~ **已解决**：`random … count <数值表达式>` | 已解锁 10373110 破坏的团结者、10364110 安息的继承者·妃花；10811110 仍缺"两个计数相减"的算术（见 S-41） | `SelectionEffect` 新增 `CountExpr`，运行时在结算到该语句时求值（0 表示不选、超过候选数按候选数截断）；续局恢复沿用挂起请求里保存的数量。 |
| S-41 | 数值算术（加减） | 10811110 昔日的天秤·马龙（"X 为对手的战场上的随从数减去自己的战场上的随从数"） | 数值表达式只有 `count/sum/scalar/negate`，没有二元加法或减法。 |
| S-40 | ~~`set cost` 的持续时间~~ **已解决**：`set cost T N until …` | 已解锁 10334120 绝尽的显现·莱奥 | 实例新增 `temporaryCost`（按结束方记录差量），到期在 `expireTurnEffects` 里按差量还原，连击快照一并保存；这样即使到期前又有永久加减费也不会被覆盖。 |
| S-42 | ~~伤害上限与主战者"受到的伤害 +1"~~ **已解决**：`damage_cap N` / `add damage_taken_up to <主战者>` | 已解锁 10401110、10444120、10464120、10711110（伤害上限），10474120 及后续所有"使对手主战者获得受伤 +1"的卡 | `damage_cap N` 表示单次受到伤害最多 `N`（= 卡面"受到的 N+1 点或以上变为 N 点"）；主战者关键词 `damage_taken_up` 在屏障判定前先加一。 |
| S-43 | ~~以纹章为效果目标~~ **已解决**：`own.crests` / `oppo.crests` | 已解锁 10453310 堕落（解放奥义破坏自己的纹章）、10454120 彼列（超进化推进自己纹章的吟唱）、90064310 延迟全部纹章的吟唱 | 纹章不在战场上，普通目标解析仍然看不到它们；只有显式写 `own.crests` / `oppo.crests` 的集合会解析到纹章实体（`destroy`、倒计数调整等）。破坏纹章走 `expireCrest`，因此触发谢幕曲；IR 只接受 `zone:"crests"` 且 `member:"card"` 的窄形式。 |
| S-44 | ~~爆能强化读取入场曲的输出~~ **已解决**：一次打出共享打出帧 | 已解锁 10424110 塞达&贝阿朵丽丝（爆能强化让入场曲召唤的复制体获得【毁灭】）；后续所有"爆能强化改写入场曲衍生体"的卡可用 | 外层效果、入场曲与爆能强化属于同一次打出，按声明顺序共用同一个 `frame`，后声明的块可以读取先声明块的输出（`summoned`、`added`、`drawn`、`destroyed` 与选择绑定）。顺带修好：运行时早就绑定 `played` 的 `when own card played` 在检查器里没有登记，写作 `evolve played silent;` 会被误报为未定义绑定。 |
| S-45 | ~~复制同名卡加入手牌~~ **已解决**：`add copies of <集合> to hand;` | 已解锁 10443310 星晶兽吸收之力；后续"使其消失，将1张同名的卡牌加入自己的手牌"的卡可用 | 新效果 `add_copies`：按每个目标当前的卡牌定义创建一张新卡加入手牌（手牌满时按过抽处理，不计入 `added`）。目标可以是已经被消失的实例，因为只读取卡牌身份。 |
| S-46 | ~~把手牌中的对象召唤到战场~~ **已解决**：`summon <绑定>;` | 已解锁 10412110 美妆少女·克洛伊（爆能强化 8 召唤选中的手牌随从并把自己返回手牌） | 新效果 `summon_from_hand`：把手牌实例直接移动战场，不发动入场曲，随从获得入场等待；仍发出 `summoned` 事件，因此入场监听会响应。 |
| S-47 | ~~筛选"本回合没有攻击过"~~ **已解决**：`where attacked this turn` / `where not attacked this turn` | 已解锁 10464110 土之法则·伽莱翁；后续"未攻击过的随从"文本可用 | 按实例本回合已经进行的攻击次数筛选（回合开始时清零）。`not` 只支持 `not attacked this turn`，其它 `not ...` 形状仍被拒绝。 |
| S-48 | 按位置选择随从（"从左起第 N 个"） | 10423310 骁勇骑士（"使自己的战场上的从左起的1个皇家护卫·随从获得『1回合可以攻击2次』"） | 选择只有随机、极值与玩家指定三种；没有按战场顺序取第 N 个的写法。 |
| S-49 | 对手攻击主战者时的事件 | 10474110 光之法则·龙敖的纹章（"对手的拥有【疾驰】的随从攻击主战者时，回合结束前，使其-3/-0"） | 只有自己的随从 `attack` 触发能力，没有"对方随从宣告攻击"的监听（也没有攻击方的绑定）。 |
| S-50 | ~~以绑定实例的攻击力作为数值~~ **已解决**：`<绑定>.attack\|life\|cost` | 已解锁 10473110 向往天空的回归者·卡西乌斯 | 数值表达式新增 `binding_scalar`：读取绑定里第一个实例的当前数值；绑定名不在这里校验存在性，写错名字得 0（与筛选里的 `card <绑定>` 一致的取舍）。 |
| S-53 | ~~给倒计数加值~~ **已解决**：`raise countdown <集合> N` | 已解锁 90064310 绝望的奔流（"使自己的所有纹章的倒计数+1"） | 原先只有 `reduce`；现在 `raise` 与 `reduce` 共用 `adjust_entity_field`，支持 `cost` 与 `countdown`。 |
| S-54 | ~~让指定随从可以攻击两次~~ **已解决**：`set_attack_limit <目标> N` | 已解锁 90034350 宏大的回归（"选择自己的战场上的1个随从，使其获得「1回合可以攻击2次」"） | 原先只接受 `set_attack_limit self N`；IR 与运行时本来就按目标集合处理，因此只放宽了验证形状。 |
| S-55 | ~~护符的【灵气】~~ **已解决**：护符可以声明 `aura` | 已解锁 90064210 月影指环 | 原先所有固有关键词都要求随从；现在 `aura` 例外（其余关键词仍然只允许随从）。运行时目标保护按战场实例判断，无需改动。 |
| S-56 | ~~带期限的费用修改~~ **已解决**：`raise\|reduce cost T N until …` | 已解锁 90044310 银冰吐息（"对手的回合结束前，使对手的所有手牌的费用+1"） | 原先只有 `set cost` 接受 `until`；现在加减费同样记录差量并在到期侧还原。 |
| S-57 | ~~判断选中的卡牌属于哪一方~~ **已解决**：`count(own.<区域> where card <绑定>) >= 1` | 已解锁 90064320 天书深渊；10663210 崇高的天书同构 | 用"同名 + 己方区域计数"表达归属，无需新的谓词；同时修好条件里的计数会丢掉绑定帧的问题。 |
| S-58 | ~~手牌中同费用的张数~~ **已解决**：`own\|oppo.hand\|deck has N same cost` | 已解锁 10553310 严酷的奥夜花 | 新增 `SameCostCondition`：按**当前费用**统计区域里出现次数最多的费用是否达到 N。 |
| S-59 | ~~把卡牌加入牌组~~ **已解决**：`add N card C to deck;`（`add copies of … to deck` 同样可用） | 已解锁 10551310 奥夜花的开战 | 新卡在随机位置插入牌组，不视为抽牌；仍然输出 `added`。 |
| S-60 | 按牌组随机随从变身并复制 | 10533310 壮美的明越花（"使自己的战场上的所有随从分别变身为自己的牌组中的随机1张随从的复制随从"） | 需要"从牌组随机取一张随从的定义并让每个己方随从变成它的复制"的组合操作。 |
| S-61 | ~~同一次打出的多个召唤输出~~ **已解决**：`summoned_all` | 已解锁 10571110 舞台缔造者 | 所有召唤类效果都会把成功入场的实例累加进 `summoned_all`；`summoned` 语义不变（最近一次）。 |

| S-72 | 返回张数与按返回张数抽牌 | 10554120 奥夜花·释藤（"使自己的所有手牌返回牌组。抽取X张卡牌。X为因本能力返回牌组的张数"） | `return` 没有输出绑定或返回数量；`draw` 的数量也不能引用"本次操作的计数"。 |
| S-62 | ~~抽牌事件~~ **已解决**：`when own\|oppo card drawn [during … turn]` 与 `when self drawn` | 已解锁 10561120 连结的使徒、10522110 迅猛的武术家、10562120 穷途末路的巫女 | 绑定 `drawn` 指向被抽到的实例；运行时按每张抽到的卡派发监听（公开事实仍是聚合事件），`during` 回合窗口同时扩展到此事件。 |
| S-63 | ~~抽牌去重~~ **已解决**：`draw N from deck where … distinct names` | 已解锁 10574120 尽小花·伊鞠 | 与牌组召唤同义：每抽中一张就排除同卡名的其余候选，张数上限是候选里不同卡名的数量；只允许与筛选搭配（不能配 `draw all` 或裸 `draw N`）。`DrawEffect.DistinctNames` 参与编解码，`internal/project/draw_owner_test.go` 覆盖编译、解码回填与错误形状。 |
| S-64 | 从破坏历史复制同名卡加入手牌 | 10572310 苏生调律（"将随机2种与本次对战中被破坏的自己的随从同名的卡牌各1张…加入手牌"） | `add copies of` 不接受破坏历史目标（`effectTargets` 过滤掉 `destroyed` 实例），也没有"同名的不同种类各1张"。 |
| S-65 | 从手牌按位置批量选择 | 10502110 星辉女神（"将自己的手牌中从左起的3张卡牌的复制卡牌各1张…加入手牌"） | 选择只有随机、极值与玩家指定；没有"手牌从左起 N 张"。 |
| S-66 | ~~本次操作的消失数量~~ **已解决**：`banish` 输出 `banished` | 已解锁 10543110 破灭屠戮者 | 与 `destroyed` 同构；数量用 `count(banished)` 读取，可当伤害量或增益量。 |
| S-67 | ~~主战者生命上限的增减~~ **已解决**：`raise maxlife own\|oppo.leader N` / `reduce maxlife …` | 已解锁 10534110 漫步的《愚者》·琳库露的纹章 | 增量形式 `kind: "adjust_leader_max_life"`（`delta=true`），上限夹在 1..65535，当前生命高于新上限时降到上限——与既有 `set maxlife` 及 SWB-RL 的 `change_leader_max_health` 同一口径。 |
| S-81 | 取前 N 张的求和与求和比较 | 10502120 手持军配团扇的伟丈夫（"自己的手牌中原始费用最大的3张卡牌的费用合计 大于 对手…则破坏对手的战场上的所有随从"） | `sum(集合, 字段)` 不支持"最大的 N 张"；条件只有 `count(...) 比较 整数`，没有 `sum(...) 比较 sum(...)`。需要给求和加极值/张数上限，并新增求和之间的比较条件。 |
| S-82 | ~~"自己发动【土之秘术】时"事件~~ **已解决**：`when own\|oppo earthrite [while self in hand]` | 已解锁 10731310 召唤仆从、10733310 饕餮魔咒；卡包 10007 因此收满 | 新事件 `earthrite`：`internal/runner/session.go` 在土之印实际扣除成功后派发（不足时整块跳过，不派发），`internal/project/strict_validate.go`/`typed_ir.go` 解析事件头，`internal/ir/decode.go` 加入白名单；监听可挂在手牌卡上（沿用 `while self in hand`）。 |
| S-83 | ~~"本次对战中进入战场的自己的创造物·随从的种类数"~~ **已解决**：`own\|oppo.entered_artifacts` | 已解锁 10771120 炫酷舞者、10771310 跑酷、10772120 大胆的涂鸦师、10773310 瞬移斩击、10774110 虚刻的安纳提玛·斯卡雷特、10774120 奋厉追赶·米乌；10008 的 10873310 直接可用 | 新玩家标量：随从进入战场时按卡牌 ID 去重记录"创造物·随从"种类（`internal/runner/execute.go` 的 `triggerSummoned`/`recordEnteredArtifact`），数值、断言、存档快照都接上；测试状态里 `field`/`destroyed` 中已存在的创造物视作本场入场过。 |
| S-85 | "可以无视【守护】进行攻击" | 10851110 通透的信念·安瑟珠（【疾驰】+ 无视守护） | 攻击路径只认识守护本身（防守方有守护时不能攻击非守护目标），没有"攻击时忽略守护"的写法；需要在实例上加一个固有状态并在攻击合法性检查里放行。 |
| S-68 | ~~回合窗口事件~~ **已解决**：`during own\|oppo turn` 可用于回复事件 | 已解锁 10563110 至圣威仪 | 检查器原先只允许"受到伤害时"带 `during`；运行时的回合匹配本来就通用。 |
| S-69 | 跨方混合随机集合 | 10524110 威猛的《战车》·奥辂昂（"随机对战场上的1个其他随从或自己的主战者或对手的主战者造成7点伤害"） | 混合集合只支持"同一方的随从+该方主战者"，无法表达"双方随从+双方主战者"。 |
| S-70 | ~~从两种指定卡中随机召唤~~ **已解决**：`summon random N card A or card B [...] [for own\|oppo];` | 已解锁 10564120 雾卷花·茎白 | 新 IR 节点 `SummonPoolEffect`：每次抽取消费一次对局随机数，池内卡牌定义必须互不相同（2..16 种）。 |
| S-71 | ~~其他随从的宣告攻击事件~~ **已解决**：`when own\|oppo follower attacks [leader] [where …]` | 已解锁 10474110 光之法则·龙敖、10544120 波摇花·夕夜 | 绑定 `attacker`；`attacks leader` 用 `EventTrigger.targetKind="leader"` 限定攻击目标。 |
| S-73 | ~~随机发动若干个模式能力~~ **已解决**：`mode random N { … }` | 已解锁 10532310 魔猫戏法 | 引擎随机选出 N 个互不相同的选项并按编号顺序结算；玩家响应协议不变（随机模式不产生请求）。 |
| S-74 | 逐个记住"尚未发动"的模式能力 | 10574110 转动的《命运之轮》·斯洛士（"从以下未发动的能力中随机发动1个能力"） | 需要按选项记录已发动状态并跨回合保存（类似限次记录，但按选项编号）。 |
| S-75 | ~~否定筛选（"非侵蚀者随从"）~~ **已解决**：`where not <词条>` | 已解锁 10603210 黑暗次元 | 支持 `trait`/`type`/`class`/`form`/`keyword`/`damaged` 六种词条的取反；`not attacked this turn` 仍是专用写法。 |
| S-76 | 再次发动自身【入场曲】 | 10604110 恐惧的象征·欧米伽奥提普（随机能力之一为"本随从+4/+4。发动本随从的【入场曲】"） | 没有"重新执行本卡牌入场曲"的效果；照抄一段等价文本无法表达可能递归的能力。 |
| S-77 | ~~"通过【爆能强化】使用卡牌时"事件~~ **已解决**：`where enhanced` | 已解锁 10622310 威风的行军 | 打出事件现在带上"本次支付了爆能强化档位"的标记，筛选词条 `enhanced` 读取它。 |
| S-51 | 主战者临时"受到的伤害变为 0" | 10444120 世界的伙伴·佐伊（爆能强化 10：主战者直到对手回合结束"受到的1点或以上伤害变为0"） | 主战者关键词只有永久形式；`until ... turn ends` 的期限只作用于随从关键词。 |
| S-52 | 牌组中发动与【瞬念召唤】 | 10404110 天司长的继承者·圣德芬（"在牌组中发动…【瞬念召唤】本卡牌…被瞬念召唤时获得纹章并返回手牌"） | 需要"在手牌/牌组中监听回合开始""从牌组召唤并选择是否返回手牌"的整套语义。 |

`DrawEffect.Owner` 与执行器（`g.playerForSide(self, e.Owner)`）本来就支持任意一方，缺的只是语法入口，所以这次扩展只动了验证与解析两处，没有改运行时。

## 批次记录

### 批次 85（卡包 10008 首批 13 张）

- 完成 13 张：10801110 自律的圣鸟·汉萨、10801120 无尽旅途·蕾娜、10841120 沙尘守宝龙、
  10804120 高洁的黑翼·奥莉薇、10831120 玛纳利亚书记官·波比、10872120 门扉接续者·拉姿莉、
  10872110 人造的馈赠·蕾拉、10822120 织田信长、10812120 情念的毒荆·莉柯瑞丝、
  10861120 勤劳的女祭司·泰瑞莎、10811120 异端隐士·西特拉斯、10811130 忧郁少女·莫埃尔、
  10832310 其乐融融的团聚。
- **修正进化点上限**：`gain own.ep` / `gain own.sep` 现在夹在 2（此前只保证不为负）。
  依据是[官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#dzyifirn1cll)：
  『高洁的黑翼·奥莉薇』在只剩 1 点超进化点时只会回复 1 点，因为上限是 2；
  本作初始 EP/SEP 都是 2（`internal/runner/match.go`），所以两者的上限一致地取 2。
- 新登记缺口：S-85 "可以无视【守护】进行攻击"（10851110 通透的信念·安瑟珠）——
  现有语言只有守护本身与"不能被选中"的保护，没有攻击时忽略守护的写法。
- 新增 14 个场景（`tests/10008/batch-85-basics.wbotest`）。
- 全量回归：`check` 0 错 0 警；`test` 1142 全绿；`go test ./...` 全绿；语料快照更新为 1142 个场景。

### 批次 84（导入卡包 10008 骨架）

- `./scripts/import_wbarts_packs.sh ../WBArts cards 10008` 生成 78 张骨架，
  语料快照从 756 张变成 834 张（骨架仍是 `unplayable`，不计入可玩数）。
- 下一步按文本由短到长补 10008 的实现；剩下的未导入只有 10009（76 张）。
- 全量回归：`check` 0 错 0 警；`test` 1128 全绿；`go test ./...` 全绿。

### 批次 83（S-82 土之秘术事件 + 卡包 10007 收满）

- **S-82 `when own|oppo earthrite [while self in hand]`**：在土之印**实际扣除成功后**
  派发 `earthrite` 事件（层数不足、整块跳过时不派发），监听可以挂在手牌里的卡牌上。
  解析（`strict_validate.go`/`typed_ir.go`）、解码白名单（`decode.go`）、运行时派发
  （`session.go` 的 `PayResourceEffect`）与文档都补齐。
- 完成最后 2 张：10731310 召唤仆从、10733310 饕餮魔咒（都在手牌中随土之秘术发动减费，
  之后分别抽 2 张 / 破坏 1 张敌方随从并增加土之印）。**卡包 10007 至此 77/77 收满。**
- 新增 5 个场景（`tests/10007/batch-83-earthrite-event.wbotest`）与
  `internal/project/earthrite_event_test.go`（事件形状 + 拒绝非法写法）。
- 全量回归：`check` 0 错 0 警；`test` 1128 全绿；`go test ./...` 全绿；语料快照更新为 1128 个场景。

### 批次 82（卡包 10007 第八批 8 张）

- 完成 8 张：10703210 巴别隆城（用实例计数器 `counter step` 记住"下一个能力"，
  启动舍弃手牌推进吟唱）、10704110 特殊目标·海雷姆哈妮（攻击随从时获得屏障、
  给交战对手「无法攻击」与「自己的回合结束时消失」，谢幕曲获得纹章——按专项核查口径，
  三个效果都在"攻击随从"条件下）、10721110 曲行工兵、10724120 宽严的音帅·塞扎尔、
  10733110 甜美存在、10734120 可爱杰作、10754110 傍死的安纳提玛·徒姬
  （`summon random 2 from own.deck.followers … distinct names`，梦魇=职业 abysscraft）、
  10761210 阳光耳饰。
- 新增 15 个场景（`tests/10007/batch-82-effects.wbotest`）。
- 全量回归：`check` 0 错 0 警；`test` 1123 全绿；`go test ./...` 全绿。

### 批次 81（卡包 10007 第七批 6 张：安纳提玛与鹿王）

- 完成 6 张带纹章的卡：10714110 操量的安纳提玛·达斯特迪兹（纹章：回合结束时连击 3
  就强化牌组随从）、10714120 冰界鹿王（回合结束按攻击力分配伤害；纹章：连击 3 加 1 张
  森林的奥秘）、10724110 统音的安纳提玛·吉尔达利娅（协作 20 获得纹章并进化；纹章：
  自己回合里随从入场就打对手主战者 1 点）、10734110 万食的安纳提玛·拉拉安瑟姆
  （土之秘术获得纹章；纹章：对手回合结束时召唤自己并进化）、10744110 焦灰的安纳提玛·
  班德奈特（把纹章交给对手，纹章在对手回合开始自伤 2 点、回复后再自伤 1 点）、
  10744120 龙峪的古龙（纹章：回合结束召唤巨翼飞龙，超进化把纹章吟唱 +2）。
- 测试侧用到测试驱动动作 `advance turn_end oppo;` 来验证"对手回合结束时"的纹章能力。
- 又确认了一次"已经在战场上的随从不会重放【入场曲】"：焦灰的安纳提玛的 9 点伤害
  与"把纹章交给对手"必须拆成两个场景（打出 / 超进化）。
- 新增 13 个场景（`tests/10007/batch-81-crests.wbotest`）。
- 全量回归：`check` 0 错 0 警；`test` 1108 全绿；`go test ./...` 全绿；语料快照更新为 1108 个场景。

### 批次 80（S-83 创造物种类数 + 卡包 10007 第六批 6 张）

- **S-83 `own|oppo.entered_artifacts`**：本场对战中进入过自己战场的创造物·随从**种类**数
  （按卡牌 ID 去重）。随从进入战场时在 `triggerSummoned` 里登记，数值、断言、
  沙盒克隆与存档快照都接上；测试状态里 `field`/`destroyed` 中已经存在的创造物
  视作本场入场过，这样单动作场景也能摆出"已经入场过 3 种"的盘面。
- 完成 6 张：10771120 炫酷舞者、10771310 跑酷（种类达标时"发动所有模式"直接展开两个效果）、
  10772120 大胆的涂鸦师、10773310 瞬移斩击、10774110 虚刻的安纳提玛·斯卡雷特、
  10774120 奋厉追赶·米乌（超进化会先执行 evolve 块召唤古老的创造物，之后才检查种类数，
  所以刚好凑满第三种时能拿到【疾驰】——场景测试固定了这个顺序）。
- **修正批次 79 的令牌 ID 错误**：10773110 创造物长枪兵召唤的『解析的创造物』
  是 90071130，不是 90071110（那是悬丝傀儡）。新场景用真实创造物重写了断言，
  所以这次错误立刻暴露；写测试时遇到的"监听不触发"正是它。
- 新增 9 个场景（`tests/10007/batch-80-artifact-kinds.wbotest`）。
- 全量回归：`check` 0 错 0 警；`test` 1095 全绿；`go test ./...` 全绿；语料快照更新为 1095 个场景。

### 批次 79（卡包 10007 第五批 11 张）

- 完成 11 张：10762110 新约白之章、10762120 新约黑之章、10763110 审理的守卫、
  10764120 安息的白翼、10723110 三连骑士、10723310 决死的猛击、10742120 成熟的佣兵、
  10743110 贪食的魔龙、10753110 骸骨驯兽师、10754120 死亡主持人·马克米朗、
  10773110 创造物长枪兵。
- **S-84**：`during own turn` 现在也能修饰入场监听（`when own follower summoned during
  own turn where trait departed`）。运行时的回合匹配本来就是通用的，项目校验也早已接受，
  只有 IR 解码白名单没跟上——这属于"编码器/解码器/校验器三处必须同时改"的又一个实例。
- **条件范围专项核查落地**：10763110 审理的守卫按批次 67 记下的口径实现（英文把
  "获得【屏障】"并进"护符≥3"条件句，所以屏障也在条件内）。
- **10754120 死亡主持人**：把"对对手的主战者造成1点伤害"放进入场监听里——
  官方英文与 SWB-RL 都把这一句并进"自己的亡者·随从在自己回合进入战场时"，
  因此三只僵尸各触发一次，共 3 点伤害（场景测试固定了这一点）。
- 写卡时又踩到一次顺序问题：`【入场曲】【模式】`必须把 `mode` **嵌在 `fanfare` 里**
  （10351110 等既有卡就是这么写的）；平铺写会让模式先结算，晚召唤的随从吃不到增益。
- 新增 19 个场景（`tests/10007/batch-79-basics.wbotest`）。
- 全量回归：`check` 0 错 0 警；`test` 1086 全绿；`go test ./...` 全绿；语料快照更新为 1086 个场景。

### 批次 78（卡包 10007 第四批 10 张）

- 完成 10 张：10713310 恶意扩大（含纹章）、10741120 载运飞龙、10762210 完美的时钟、
  10712120 弓兵指挥者、10713110 冰箭射手、10732310 暴食的零嘴、10731120 小型怪兽、
  10763210 海蚀三叉戟、10761110 营利支援者、10772310 闪光一瞬。
- 恶意扩大与忧虑缩小的纹章互相把对方加回手牌（`countdown 1` + 谢幕曲加牌），
  两张卡因此形成闭环；`add combo 1`（弓兵指挥者）、`damage target self.attack`（冰箭射手）、
  `mode` 里的 `earthrite`（暴食的零嘴）、护符启动破坏自身（完美的时钟、海蚀三叉戟）、
  `if own.superevolve_unlocked`（闪光一瞬）都用的既有原语。
- 新登记缺口：S-82 土之秘术发动事件（10731310）、S-83 本场对战中进入战场的创造物种类数
  （10771120、10773310、10774110）。
- 新增 18 个场景（`tests/10007/batch-78-basics.wbotest`）。
- 全量回归：`check` 0 错 0 警；`test` 1067 全绿；`go test ./...` 全绿；语料快照更新为 1067 个场景。

### 批次 77（卡包 10007 第三批 12 张）

- 完成 12 张：10742110 豪龙守门人、10711110 巨型熊、10722110 听略谍报兵、
  10751120 恶魔鼓手·拉兹、10771110 个性店主、10722120 斩奏医护兵、10752110 猫咪走绳师、
  10732110 迷人怪兽、10764110 裁神的安纳提玛·罗德欧、10761120 广域传教士、
  10721310 敌我的调律、10741310 百无聊赖的睥睨。
- 用的都是既有原语：`summon copies of self`（复制体继承关键词）、`damage_cap 3`、
  `earthrite 2`、`raise countdown`、`draw N from deck where type amulet`、
  `count(own.hand.amulets)`、`cannot_attack … until oppo turn ends`、
  `gain own.maxpp 1` + `transform own.hand|deck into card … where card …`。
- 测试侧补上 `alias.damage_cap == N` 断言（此前的实例字段表里只有 `damage_reduction`）。
- 新增 19 个场景（`tests/10007/batch-77-basics.wbotest`）。写测试时再次确认：**已在战场上**的
  随从再进化只会走 `evolve` 块，不会重放入场曲，所以"入场曲 + 进化时"的卡要拆成两个场景
  （打出测入场曲、`evolve` 测进化时）。
- 全量回归：`check` 0 错 0 警；`test` 1049 全绿；`go test ./...` 全绿；语料快照更新为 1049 个场景。

### 批次 76（卡包 10007 第二批 11 张）

- 完成 11 张：10741110 宣扬的龙人、10743310 黑炎的奔流、10752310 讴歌青春、
  10711120 精灵陷阱师、10751310 灵魂调律、10752120 乌鸦杂耍师、10712310 忧虑缩小
  （含纹章）、10722310 无音的包围、10753310 夜之歌的演唱会、10703110 享乐的上级市民、
  10772110 悠然的滑手。全部用既有原语。
- 两处需要写清的结算：`evolve summoned_all silent` 让本次召唤的三张一起进化（用 10613110
  "自己的随从进化时回复主战者 1 点"观测到三次进化事件）；`damage 集合 N distributed`
  按战场顺序填满当前生命，6 点伤害会全部落在第一个随从上（不是平均分）。
- 分支内产生的绑定不会外泄，`added` 这类绑定要在各自分支里使用
  （无音的包围的两个分支各自 `add rush to added;`）。
- 新增 13 个场景（`tests/10007/batch-76-basics.wbotest`）。
- 全量回归：`check` 0 错 0 警；`test` 1030 全绿；`go test ./...` 全绿；语料快照更新为 1030 个场景。

### 批次 75（导入卡包 10007 + 首批 11 张）

- **导入卡包 10007**：`./scripts/import_wbarts_packs.sh ../WBArts cards 10007` 生成 77 张骨架，
  语料快照因此从 679 张变成 756 张（骨架仍是 `unplayable`，不计入可玩数）。
- 完成首批 11 张（都是短文本）：10701110 纯真孩童、10701310 颓废之泪、10704120 巴别隆市长·
  埃尔塔罗、10712110 绿风细剑师、10751110 暗夜键盘手·露露米、10731110 小巧捕食者、
  10732120 甜蜜猎食者、10711310 人格切换、10721120 传调联络兵、10702110 神话记者、
  10742310 焦龙的午睡。全部用既有原语，没有新增 IR。
- 两处按官方文本核对过的细节：连击把本张牌自己算进去（`combo 1` 打出后是 2，所以
  「连击_3」那一档要求打出前至少连击 2）；`add N earthsigil` 在没有土之印护符时
  召唤层数为 N 的土之印（甜蜜猎食者因此写成 `add 2 earthsigil;`）。
- 测试侧补上 `own.earthsigils == N` 断言（只读玩家标量，此前只有过程语句能读）。
- 新增 15 个场景（`tests/10007/batch-75-basics.wbotest`）。
- 全量回归：`check` 0 错 0 警；`test` 1017 全绿；`go test ./...` 全绿；语料快照更新为 756 张卡 / 1017 个场景。

### 批次 74（S-63 抽取同名去重 + S-67 生命上限增减 + 卡包 10005）

- **S-63 `draw N from deck where … distinct names`**：用于"抽取 2 种费用为 1 的法术"。
  与牌组召唤同义——每抽中一张就排除同卡名的其余候选，所以张数上限是候选里不同卡名的
  数量（先按不同卡名数收窄 `count`，再逐次随机抽选）。只允许与筛选搭配。
- **S-67 `raise maxlife` / `reduce maxlife`**：主战者生命上限的增减。增量形式把上限夹在
  1..65535，当前生命高于新上限时降到上限——与既有 `set maxlife`（阿斯塔罗特的宣判）
  和 SWB-RL 的 `change_leader_max_health` 同一口径。顺带核过 10534110 纹章的
  [官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#7wzi4bb1a)：变身进入战场
  不触发"进入战场时"的能力，所以纹章写 `when own follower summoned where card 10534110`。
- 完成 3 张：10534110 漫步的《愚者》·琳库露（纹章：自己的琳库露入场时对手生命上限 -2；
  突进；超进化时把 10 张自己加入牌组）、10521110 好施的名人（舍弃 1 张手牌后回复
  3 点，舍弃的是法术则改为 6 点）、10574120 尽小花·伊鞠（舍弃 1 张手牌并抽取 1 张法术；
  使用法术时若已进化则召唤『伊鞠的小鬼』；超进化时抽取 2 种费用为 1 的法术）。
- 新登记缺口：S-81 取前 N 张的求和与求和比较（10502120 仍挂起）。
- 新增 11 个场景（`tests/10005/batch-74-maxlife-and-draw.wbotest`、
  `tests/10005/batch-74-discard-and-heal.wbotest`）。
- 全量回归：`check` 0 错 0 警；`test` 1002 全绿；`go test ./...` 全绿；语料快照更新为 1002 个场景。

### 批次 73（条件范围专项核查：S-80 选择不该阻塞打出）

起因：复查 90064320 天书深渊的条件范围时顺手问了"还有没有同类问题"。结论是
**条件范围本身没有别的错**，但同一类"选择与后续效果的范围"里抓到一个更大的问题：

- **S-80**：`require` 被当成所有卡牌的"可打出性"检查，于是随从/护符只要入场曲里
  选不到目标就整张卡不能打出、进化时选不到目标就不能进化。官方 QA 明确只有
  **法术**与**启动能力**才有这条限制：随从/护符没有可选对象时照样能打出、能力照常结算。
  把 29 张随从/护符卡的 34 处 `require` 改成 `choose`（`engage` 与法术保留 `require`），
  与既有的"有候选必须选、无候选绑定 `none` 并继续结算"语义一致，没有引入新原语。
- **工具化**：新增 `scripts/card_scope_screen.mjs`（含 `node --test` 单测），输出
  A 句数错位、B「〜を選んだなら」身份条件、C 非法术卡仍在用 `require` 三张清单，
  以后每批可以顺手跑一次。
- 核查结论与"同形不同解"的四张卡（90064320 vs 10372110／10372210／10653110／10753110）
  记在[条件范围专项核查](#条件范围专项核查配合批次-67批次-73)一节。
- 新增 7 个场景（`tests/10004/batch-73-target-availability.wbotest`）。
- 全量回归：`check` 0 错 0 警；`test` 991 全绿；`go test ./...` 全绿；语料快照更新为 991 个场景。

### 批次 72（S-78 谢幕曲筛选 + S-79 历史召唤去重 + 卡包 10006 收尾）

- **S-78 `where lastwords`**：筛选"拥有【谢幕曲】"的卡牌，读取卡牌定义里的
  `lastwords` 触发（含通过 `grant` 写进定义的形态；运行中临时授予的不算）。
  编码器/解码器白名单与 `parseWhere`/`negatedWhereTerm` 同步补齐。
- **S-79 破坏历史召唤的 `distinct names`**：用于"随机 2 种…各 1 张"。
  官方 QA 明确了结算顺序：先从全部历史记录里等概率抽 1 张，再从"与第 1 张不同种类"
  的剩余候选里抽第 2 张，因此同名记录多的更容易被选中；候选耗尽时少召唤。
  与牌组召唤的 `distinct names` 语义一致。
- 修好一处自己写出来的 bug：`internal/ir/decode.go` 解出 `distinctNames` 却忘了
  回填到 `HistorySummonEffect`，运行期恒为 `false`。补的"只破坏过同一种护符时
  只召唤一张"场景立刻抓到了它——**编解码白名单加字段时，解码分支必须真的把字段
  传进效果结构体，`check` 不会覆盖这条路径**。
- 完成 3 张：10603110 彷徨于黑暗之兽（入场曲从【疾驰】【毁灭】【威慑】【虹吸】
  【灵气】【屏障】里随机 3 个，`mode random 3`）、10663210 崇高的天书（吟唱 2；
  入场曲选 1 张其他卡牌破坏，选到自己的护符时回复 2 点能量点；谢幕曲随机召唤
  1 张被破坏的低费谢幕曲护符）、10664110 崇高的憎恶·康蒂玛（入场曲随机 2 种
  各 1 张；超进化时同样的归属判断 + 对敌方全体 3 点伤害并破坏）。
- 新增 5 个场景（`tests/10006/batch-72-lastwords-and-random-keywords.wbotest`）。
- 全量回归：`check` 0 错 0 警；`test` 984 全绿；`go test ./...` 全绿；语料快照更新为 984 个场景。

### 批次 71（信仰授权能力 + 卡包 10006）

- **`grant faith { … }`：把事件监听附加到信仰实体**：新增 `FaithRef`（"本卡牌定义的信仰"），
  `grant` 现在既接受随从，也接受信仰；附加的事件监听不再限于谢幕曲与回合开始/结束，
  可以是任意事件（含 `where` 筛选，例如"通过【爆能强化】使用卡牌时"）。
- 完成 2 张：10614120 古旧天枪·萨莎妮德（入场曲：信仰值-10 加入天枪深渊并让信仰获得
  "自己的随从进化时对对手主战者造成 1 点伤害"；信仰随自己的随从进化 +1）、
  10624120 古旧天剑·伊德梅塔（入场曲加入天剑深渊；进化时信仰值-5 并让信仰获得
  "通过爆能强化使用卡牌时，自己战场所有随从 +1/+1"；信仰随爆能强化使用卡牌 +1）。
- 新增 7 个场景（`tests/10006/batch-71-faith-abilities.wbotest`）与 Go 单测
  `internal/runner/faith_grant_test.go`（授权后的事件确实由信仰触发）。
- 全量回归：`check` 0 错 0 警；`test` 979 全绿；`go test ./...` 全绿；语料快照更新为 979 个场景。

### 批次 70（信仰值作为数值 + 90034330 天晶深渊）

- **新原语 `distribute faith <卡牌ID> { option N { … } … }`**：把指定信仰的信仰值**逐点**
  独立等概率分配给若干能力（每点消费一次对局随机数），再按选项编号顺序、
  每个分配到的点执行一次能力；信仰值本身不消费。
- 完成 90034330 天晶深渊：召唤 1 个天晶魔手，然后按信仰值逐点随机加身材 / 回复主战者 / 打击对手主战者。
- 新增 2 个场景（`tests/90000/batch-70-faith-distribution.wbotest`）与 Go 单测
  `internal/runner/faith_distribution_test.go`（逐点分配合计等于信仰值、且不消费信仰值）。
- 全量回归：`check` 0 错 0 警；`test` 972 全绿；`go test ./...` 全绿；语料快照更新为 972 个场景。

### 批次 69（S-35 信仰机制第一阶段 + 卡包 10006）

- **信仰（faith）实体与信仰值**：卡牌可以声明 `faith { counter value 0; <事件监听> 五种本地化 }`。
  开局时按"初始牌组里出现的信仰定义"把信仰实体放进主战者区域（与纹章共用区域、最多五个），
  信仰值放在 `counter value` 里，由信仰自己的事件监听增长（例如"自己的护符被破坏时，信仰值+1"）。
  实现方式：信仰与纹章共用实体结构（`Card.FaithCard()`、区域、触发索引），卡牌类型为 `faith`。
- **信仰值支付块 `faith N { … }`**：与 `earthrite`/`necromancy` 同构；信仰不存在或信仰值不足时
  整块跳过（不扣值、不产生副作用），足够时扣 N 再执行。
- 完成 2 张：10634120 古旧天晶·卡卢基典瑟拉（手牌中每次天晶魔手入场减 1 费；
  入场曲召唤两个带【疾驰】的天晶魔手；进化时把『天晶深渊』加入手牌；信仰值随天晶魔手入场 +1）、
  10664120 古旧天书·莲妥丝（入场曲选 3 张其他卡牌破坏、守护；回合结束时信仰值-10 并把『天书深渊』加入手牌；
  自己的护符被破坏时信仰值+1）。
- 新增 5 个场景（`tests/10006/batch-69-faith.wbotest`），覆盖信仰增长、支付成功/不足两侧。
- 仍待实现（后续批次）：`信仰值` 作为数值参与随机分配（90034330 天晶深渊的 X/Y/Z），
  以及"使自己的信仰获得「…」"（10614120、10624120、10354110）——需要在信仰实体上加授权能力。
- 全量回归：`check` 0 错 0 警；`test` 970 全绿；`go test ./...` 全绿；语料快照更新为 970 个场景。

### 批次 68（卡包 10006 第四批，4 张）

- 完成 4 张：10641310 天刀授予、10643310 断头的天刀、10673310 恶劣的天斧、
  10674110 恶劣的纯心·卡密希拉。
- 用到的既有原语：舍弃时按 `self.cost` 判断并把减费后的同名卡加入手牌
  （`add 1 card … to hand; set cost added N;`）、`reduce cost self 1 until turn ends`
  （临时减费，`until` 现在允许省略 `minimum`）、
  `random ally … where form unevolved and base.cost >= 5`、
  `when own follower summoned other where base.cost >= 5`、`count(own.field.followers where base.cost >= 5)`
  作为伤害量。
- 新增 9 个场景（`tests/10006/batch-68-discard-and-elder.wbotest`）。
- 全量回归：`check` 0 错 0 警；`test` 965 全绿；`go test ./...` 全绿；语料快照更新为 965 个场景。

### 条件范围专项核查（配合批次 67、批次 73）

发现 90064320 天书深渊的条件范围错误后，做了两类系统排查：

1. **机械筛查**：对全部已实现卡，比较中/英文本的句数并筛出"含条件词且英文句数少于中文"的候选，
   共 7 张：10262110、90064320、10344110、10544110、10543310、10153110、10474110。
   - 10262110 弹幕驱魔人、10543310 懒惰的波摇花、10474110 光之法则·龙敖：条件只影响紧随其后的量
     （"改为发动 2 次" / "追加 2 点"），实现正确。
   - 10544110 约束的《正义》：日/英文都把"回复 8 点"放在"进化前"分支内，实现正确。
   - 10344110 侮蔑的继承者：只是【守护】等关键词被英文并进同一句，无条件范围问题。
   - 90064320 天书深渊：**已按日/英文修正**（"加回手牌"在条件句内）。
     SWB-RL 的实现把"加回手牌"放在条件外，与卡牌英文文本冲突——按 [权威契约](../authority.md) 以卡牌文本为准，
     并在 `tests/10006/batch-67-enhanced-and-own-amulet.wbotest` 里固定了两侧行为。
   - 10153110 蓝蔷薇千金·赛蕾丝：日/中/韩/繁中是「…なら、4回復。これは【バリア】を持つ。」，
     英文并成一句（"restore 4 instead and give this follower Barrier"）。SWB-RL 的
     `test_ceres_turn_end_uses_owner_scope_and_super_branch` 明确断言"未超进化时不获得【屏障】"，
     与英文一致，因此保留现有实现（屏障只在超进化分支内），并把该歧义记录在此。
     同类结构还有未导入的 10763110 审理的守卫（英文同样并入条件句），届时按同一口径处理。
2. **权威顺序**：卡牌文本（多语言）+ QA 优先，SWB-RL 只作第二意见。遇到冲突时在场景测试里固定最终行为，
   并把冲突写进本节，避免以后再翻案。

**批次 73 把这次排查工具化并对全部 904 张卡重跑**：`scripts/card_scope_screen.mjs`
（判定逻辑有 `scripts/card_scope_screen.test.mjs` 覆盖，`node --test scripts/card_scope_screen.test.mjs`）
输出三个清单：

- **A 句数错位（19 张）**：中文/日文句子比英文多、且中文含条件词。逐张核对后条件范围都与文本一致：
  多数是"条件只影响紧随其后的量"（10262110、10543310、10943110、10823110 的"改为发动 N 次"）
  或对 X／数量说明句的切分（10133310、10503210、10554120），以及 `<hr>` 分隔的独立能力块
  （90044330 天刀深渊：前半是"被舍弃时"，后半是打出时的无条件效果，实现正确）。
- **B 所选卡身份条件（8 张）**：日文出现「〜を選んだなら」。除 90064320 外还有
  10372110 破坏的祈祷者、10372210 破坏的荒野、10521110 好施的名人、10653110 渴命的破坏者、
  10663210 崇高的天书、10664110 崇高的憎恶·康蒂玛、10753110 骸骨驯兽师（未导入）。
  其中 10521110、10663210、10664110 的条件句是能力最后一句（只影响回复量或是否追加效果），无范围问题；
  10372110／10372210／10653110／10753110 的形状与 90064320 最接近（「選んだなら、A。B。」）。
- **两处"同形不同解"必须记牢**：
  90064320 的条件是**所选卡的身份**（「自分のアミュレットを選んだなら」），官方英文把后续的
  "2 点伤害"和"加回手牌"两句都并进条件内，因此按条件内处理；
  10372110／10372210／10653110／10753110 的条件是**是否做出了选择**，后续句是独立效果，
  官方英文没有并进条件内，且 10372110 的[官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#yxrw-1c_x)
  明确"选中的卡没被破坏时，2 点伤害照常发动"（选到无法被能力破坏的莉洁纳仍然发动），
  因此后续句与选择/破坏是否成功无关；三张卡的现有实现（`choose` + 直接结算后续句）与 SWB-RL 一致，保持不变。
  **以后不要用"统一口径"去改这四张卡**——它们靠的是各自英文写法和 QA，而不是同一条规则。
- **C 规则侧 `require`（批次 73 修正）**：见 S-80。

### 批次 67（S-77 爆能强化打出事件 + S-57 归属判断复核 + 卡包 10006 / 90000）

- **S-77 `where enhanced`**：打出事件新增"本次是否支付了【爆能强化】档位"的标记
  （`instance.enhancedPlay` 在 `applyPlaySetup` 之前设置），筛选词条 `enhanced` 用它判断，
  因此"自己通过【爆能强化】使用卡牌时"可以写成
  `when own card played where enhanced { … }`。
- **S-57 归属判断不再需要新原语**：`count(own.field.amulets where card target) >= 1`
  可以判断"选中的卡牌是自己的护符"（`same_card` 谓词 + 区域计数）。
  顺带修好 `CountCondition` 在 `conditionIn` 里丢掉绑定帧的问题——之前
  `count(集合 where card <绑定>)` 在条件里恒为 0（`condition` 那条路径传的是 nil 帧）。
- 完成 2 张：10622310 威风的行军（纹章：通过爆能强化使用卡牌时召唤勇烈的士兵；
  爆能强化 3 让纹章吟唱 +2）、90064320 天书深渊。
- **修正 90064320 的条件范围**：日文/英文原文是
  「自分のアミュレットを選んだなら、相手のリーダーに2ダメージ。『天書の深淵』1枚を自分の手札に加える。」，
  "把同名卡加入手牌"在条件句**内部**；中文用句号切断后容易被读成无条件。
  现在只有选到自己的护符时才追加 2 点伤害并加回手牌。
  **教训**：中文文本的句号不保证条件范围，遇到"若…则…。…"的写法必须核对日文/英文原文。
- 新增 5 个场景（`tests/10006/batch-67-enhanced-and-own-amulet.wbotest`）；Go 单测
  `internal/project/enhanced_filter_test.go`。
- 全量回归：`check` 0 错 0 警；`test` 956 全绿；`go test ./...` 全绿；语料快照更新为 956 个场景。

### 批次 66（S-75 否定筛选 + 卡包 10006）

- **S-75 `where not <词条>`**：筛选取反，目前支持 `trait` / `type` / `class` / `form` / `keyword` / `damaged`
  六种词条（`not attacked this turn` 保持原有专用写法）。新增 `NotPredicate`（IR、解码、
  卡牌引用检查、运行时可不像；解析在 `parseWhere` 与 `filterIR` 两处）。
- 完成 10603210 黑暗次元：吟唱 2，自己的回合结束时对战场上的所有**非侵蚀者**随从造成 2 点伤害。
- 新增 1 个场景（`tests/10006/batch-66-negated-filter.wbotest`）；Go 单测
  `internal/project/not_filter_test.go`（取反保留、`not attacked this turn` 未回归、拒绝不支持的词条）。
- 全量回归：`check` 0 错 0 警；`test` 951 全绿；`go test ./...` 全绿；语料快照更新为 951 个场景。

### 批次 65（卡包 10006 第三批，8 张）

- 完成 8 张：10674120 古旧天斧·尤泽塔、10644120 古旧天刀·波菈莱、
  10623310 惨烈的天剑（爆能强化 3"改为发动所有能力"用 `enhance 3 replaces`）、
  10643110 隔断的龙斗士、10673110 愚劣的兵器、10633310 魔恋的天晶、
  10644110 断头的斩姬·相枛津、10624110 惨烈的剑王·罗德诺艾尔四世
  （同一次打出的 `enhance 7` 与 `enhance 8` 会按规则全部发动，因此能一次召唤三个士兵）。
- 用到的既有原语：`when self discarded`（古旧天刀被舍弃时召唤自己）、
  `superevolve replaces evolve`（超进化改为加入 3 张）、`count(集合 where base.cost >= 5)`
  门槛判断、`damage … distributed`。
- 新增 14 个场景（`tests/10006/batch-65-effects.wbotest`）。
- 新登记缺口：S-76 再次发动自身【入场曲】（10604110 恐惧的象征·欧米伽奥提普的随机能力之一，
  以及同类"发动本随从的入场曲"文本）；S-77 "通过【爆能强化】使用卡牌时"事件
  （10622310 威风的行军的纹章）。
- 全量回归：`check` 0 错 0 警；`test` 950 全绿；`go test ./...` 全绿；语料快照更新为 950 个场景。

### 批次 64（卡包 10006 第二批，24 张）

- 完成 24 张：10611110 慈育的森民、10672120 胆小鬼先锋、10654120 古旧天眼·比芭提、
  10641120 熟透的海鱼、10613110 慈惠的心腹、10633110 魔醉的教师、10632110 魔境的学生、
  10632120 冒险魔导书、10652120 失恋恶魔、10654110 枯渴的魔神·阿尔弭斯、10642120 尖刺龙、
  10653310 枯渴的天眼、10622110 忠烈的近卫兵、10662210 救赎的圣典、10663110 崇拜的圣骑士、
  10671310 天斧授予、10613310 慈爱的天枪、10614110 慈爱的凛华·奥尔提雅、10623110 暴烈的参谋、
  10653110 渴命的破坏者、10672310 平庸的制图、10631120 空想的图书管理员、
  10634110 魔恋的爱慕·希姆（纹章）、10661210 污浊的圣水。
- 这一批主要复用既有原语：爆能强化追加召唤、`necromancy`/`enhance` 触发的静默进化、
  `when self evolved`、`when own follower evolved`、`when own follower summoned where card …`、
  `clash { destroy opponent; }` 配 `ability_destruction_guard`、`summoned_all` 批量强化、
  纹章 + `when own follower attacks where card …`（魔恋的爱慕的纹章）。
- 新增 28 个场景（`tests/10006/batch-64-effects.wbotest`）。
- 全量回归：`check` 0 错 0 警；`test` 936 全绿；`go test ./...` 全绿；语料快照更新为 936 个场景。

### 批次 63（导入卡包 10006 + 首批 29 张）

- 用 `./scripts/import_wbarts_packs.sh ../WBArts cards 10006` 导入 76 张骨架，
  语料快照的卡数 603 → 679。
- 完成 29 张：10612120 咆哮狼人、10631110 天晶魔手、10661310 天书授予、10662110 崇敬的涂描者、
  10631310 天晶授予、10671110 低劣的玩具、10611120 伊甸之猴、10601110 浑浊之民、
  10612110 慈颜的拥趸、10621110 勇烈的士兵、10651120 逃避幽灵者、10652110 渴欲的唤灵师、
  10651110 渴望的恶魔、10651310 天眼授予、10672110 拙劣的人偶、10621310 天剑授予、
  10601120 匍匐的异类、10622120 猫人水手、10642110 果断的剑圣、10611310 天枪授予、
  10632310 正常的侵蚀、10652310 "最强"的诱惑、10661110 崇奉的懦者、10671120 聪明的创造者、
  10612310 向女王献花、10641110 决断的龙人、10621120 懒惰女仆、10662120 飞马骑手、10642310 赤流。
- 新能力用到的既有原语：`summoned_all`（天剑授予爆能强化给三个士兵 +2/+2）、
  `summon copies of target; banish target;`（"最强"的诱惑）、`when self summoned`、
  `when own follower evolved while self in hand`（向女王献花的手牌减费）。
- 新增 27 个场景（`tests/10006/batch-63-basics.wbotest`）。
- 新登记缺口：S-75 否定筛选（"非侵蚀者随从"，10603210 黑暗次元）；
  另外 10652310 的多选目标必须在同一效果里先写两条 `require`，否则合法性预检会退化为
  `unsupported_preflight`（已写进流程教训）。
- 全量回归：`check` 0 错 0 警；`test` 908 全绿；`go test ./...` 全绿；语料快照更新为 908 个场景。

### 批次 62（S-66 消失输出 + 卡包 10005 第八批）

- **S-66 `banish` 输出 `banished`**：本次实际消失的实例写入绑定 `banished`
  （与 `destroy` 的 `destroyed` 同构），因此"因本能力消失的卡牌的张数"可以写成
  `count(banished)`，也能用于分配伤害。
- 完成 10543110 破灭屠戮者：超进化时让自己的牌组里费用 1/3/5/7/9 的卡牌全部消失，
  并按消失张数对对手全体随从分配伤害。
- 新增 2 个场景（`tests/10005/batch-62-banish-count.wbotest`）；Go 单测
  `internal/project/banish_output_test.go`。
- 全量回归：`check` 0 错 0 警；`test` 881 全绿；`go test ./...` 全绿；语料快照更新为 881 个场景。

### 批次 61（S-73 随机模式 + 卡包 10005 第七批）

- **S-73 `mode random N { … }`**：由引擎随机选出 N 个互不相同的选项（不向玩家提问），
  按选项编号顺序结算（与玩家多选模式的顺序一致）。IR 的 `ModeEffect` 新增 `random` 字段，
  解析、形状校验与运行时都已接通。
- 完成 10532310 魔猫戏法：【土之秘术_2】后从"召唤泥尘巨像 / 回复主战者 2 点 / 土之印 +3"
  中随机发动 2 个；土之秘术不足时整段不发动。
- 新增 2 个场景（`tests/10005/batch-61-random-modes.wbotest`）；Go 单测
  `internal/project/random_mode_test.go`。
- 全量回归：`check` 0 错 0 警；`test` 879 全绿；`go test ./...` 全绿；语料快照更新为 879 个场景。

### 批次 60（S-71 宣告攻击事件 + 卡包 10004 / 10005）

- **S-71 `when own|oppo follower attacks [leader] [where …]`**：监听其他随从宣告攻击，
  绑定 `attacker` 指向攻击方；`attacks leader` 额外限定目标为主战者（IR 的
  `EventTrigger.targetKind`）。运行时把宣告攻击事件的 `side` 设为攻击方一侧，
  监听派发的绑定名从内部的 `attack` 改为 `attacker`。
- 完成 2 张：
  - 10474110 光之法则·龙敖（入场曲连打 6 次 1 点 + 强化对手手牌、奥义获得纹章；
    纹章让对手带【疾驰】的随从攻击主战者时 -3/-0 到回合结束）；
  - 10544120 波摇花·夕夜（入场曲召唤大海虎鲸、进化时舍弃一张手牌并获得纹章；
    纹章让自己的海洋随从攻击时 +1/+0，并且每回合一次把大海虎鲸加入手牌）。
- 新增 8 个场景（`tests/10005/batch-60-attack-events.wbotest`）；Go 单测
  `internal/project/attack_event_test.go`。
- 全量回归：`check` 0 错 0 警；`test` 877 全绿；`go test ./...` 全绿；语料快照更新为 877 个场景。

### 批次 59（S-58 同费用手牌判断 + S-61 召唤累计输出 + 卡包 10005 第六批）

- **S-58 `own|oppo.hand|deck has N same cost`**：判断区域里是否存在 N 张以上**当前费用**
  相同的卡牌。新增 `SameCostCondition`（IR、解码、条件编译、运行时统计与形状校验）。
- **S-61 `summoned_all`**：在一次结算里累计各次召唤成功入场的全部实例；
  `summoned` 仍然只保留最近一次操作的结果。所有召唤类效果（普通召唤、复制、手牌召唤、
  亡者召还、牌组/历史/随机池召唤）都会写入这个绑定。
- 完成 2 张：10553310 严酷的奥夜花（纹章：回合结束抽 1 张，若手牌有 4 张以上同费卡牌则召唤带【守护】的骸骨士兵）、
  10571110 舞台缔造者（入场曲召唤改良型·悬丝傀儡与悬丝傀儡；爆能强化 7 让这两个傀儡都获得【疾驰】）。
- 新增 5 个场景（`tests/10005/batch-59-same-cost-and-puppets.wbotest`）；Go 单测
  `internal/project/same_cost_and_summoned_all_test.go`。
- 新登记缺口：S-72 "返回张数/按返回张数抽牌"（10554120 奥夜花·释藤的入场曲）。
- 全量回归：`check` 0 错 0 警；`test` 869 全绿；`go test ./...` 全绿；语料快照更新为 869 个场景。

### 批次 58（S-70 随机池召唤 + 卡包 10005 第五批）

- **S-70 `summon random N card A or card B [...] [for own|oppo];`**：从几种指定卡牌定义中
  随机召唤（每次抽取消费一次对局随机数），新 IR 节点 `SummonPoolEffect`，
  卡牌引用检查、编解码、运行时与形状校验都已接好；推导卡不是召唤 —— 不发动入场曲。
- 完成 10564120 雾卷花·茎白：入场曲获得纹章并把随机 2 张手牌返回牌组、抽 2 张；
  纹章在自己回合抽到 1/3/5 费卡牌时在本方战场、抽到 2/4/6 费卡牌时在对手战场
  随机召唤『纯洁白狐』或『神圣猎鹰』。
- 新增 3 个场景（`tests/10005/batch-58-pool-summon.wbotest`）；Go 单测
  `internal/project/summon_pool_test.go`。
- 全量回归：`check` 0 错 0 警；`test` 864 全绿；`go test ./...` 全绿；语料快照更新为 864 个场景。

### 批次 57（S-62 抽牌事件 + 卡包 10005 第四批）

- **S-62 抽牌事件**：新增 `when own|oppo card drawn [during own|oppo turn]` 与
  `when self drawn`（"抽到本卡牌时"，隐式在持有者手牌中监听）。绑定名 `drawn` 指向被抽到的实例，
  可以当数值用（`drawn.cost`）。
  实现要点：运行时按"每个被抽到的实例"派发监听（公开事件流仍然只记一次聚合的 `card_drawn` 事实），
  监听在该实例进入手牌后触发；`during` 回合窗口扩展到 `card_drawn`。
- 完成 3 张：10561120 连结的使徒（爆能强化 4 抽牌并获得毁灭；自己抽牌时获得突进）、
  10522110 迅猛的武术家（抽到本卡牌时费用变为 3；入场曲抽牌并回复）、
  10562120 穷途末路的巫女（入场曲与进化时抽牌；自己抽牌时打击敌方全体）。
- 新增 8 个场景（`tests/10005/batch-57-draw-triggers.wbotest`）；Go 单测
  `internal/project/drawn_event_test.go`。
- 新登记缺口：S-70 从两种指定卡中随机召唤（10564120 雾卷花·茎白的纹章要"随机1个纯洁白狐或神圣猎鹰"）、
  S-71 其他随从的宣告攻击事件（10544120 波摇花·夕夜的纹章）。
- 全量回归：`check` 0 错 0 警；`test` 861 全绿；`go test ./...` 全绿；语料快照更新为 861 个场景。

### 批次 56（S-68 回合窗口事件 + 卡包 10005 第三批）

- **S-68 `during own|oppo turn` 扩展到回复事件**：原先只允许"受到伤害时"，
  现在主战者回复也可以限制回合窗口（"自己的主战者回复时，若为自己的回合"）。
  运行时与 IR 早已通用，只放宽了检查器与解码器。
- 完成 5 张：10534120 明越花·阿罗（魔力增幅减费、入场曲 10 点伤害、进化时把其他随从变成壮丽大神隼）、
  10563110 至圣威仪（回复主战者时召唤纯洁白狐）、10544110 约束的《正义》·伊兰翠、
  10564110 思念的《力量》·索菲娜、10514120 虫风花·魅禄（入场曲与进化时都发动【模式】）。
- 新增 15 个场景（`tests/10005/batch-56-mysteria.wbotest`）；Go 单测
  `internal/project/healed_turn_event_test.go`。
- 新登记缺口：S-66 消失数量（`banish` 没有输出绑定）、S-67 主战者生命上限的增减
  （10534110 愚者的纹章）、S-69 混合同侧目标集合之外的单次随机选择
  （10524110 战车要同时随机"其他随从或双方主战者"）。
- 全量回归：`check` 0 错 0 警；`test` 853 全绿；`go test ./...` 全绿；语料快照更新为 853 个场景。

### 批次 55（S-59 加入牌组 + 卡包 10005 第二批）

- **S-59 `add N card C to deck;`**：新卡在随机位置插入牌组（不视为抽牌，仍输出 `added`）；
  `add_copies` 也支持 `to deck`。
- 顺带修好 `other` 的解析边界：`transform … other into card C` 之前会把 `into` 当成绑定名，
  现在 `otherFollowers` 关键词集合包含 `into`、`preserving`、`minimum`、`to`、`from`、`for`。
- 完成 22 张：10551310 奥夜花的开战、10541310 波摇花的裁决（伤害量取 `drawn.cost`）、
  10533110 元素支配者、10541120 水滴打拍者、10521310 丽金花的挥霍、10542120 水母舞姬、
  10573110 神经遮蔽者（`repeat count(…) { gain own.pp 1; }`）、10543310 懒惰的波摇花、
  10532120 余韵俳谐师、10561110 先见的神官、10522120 吉祥蛙、10552120 牵线搭桥的青鬼、
  10553110 致命掠夺者（`transform field.followers other into card 90051110`）、
  10503310 《世界》的呈现、10523110 不动如山的将校、10504110 八界花·下天央、
  10521120 烟管美玉、10511120 森林羽子板工匠、10513110 引路船工、10563210 坚固的雾卷花、
  10572110 新时代地理学者、10524120 丽金花·云庆（纹章）。
- 新增 34 个场景（`tests/10005/batch-55-effects.wbotest`）；Go 单测
  `internal/project/add_to_deck_test.go`。
- 新登记缺口：S-61 同一次打出的多个召唤输出、S-62 抽牌事件、S-63 抽牌去重（"抽取2种…"）、
  S-64 从破坏历史复制同名卡加入手牌、S-65 从手牌按位置批量选择。
- 全量回归：`check` 0 错 0 警；`test` 838 全绿；`go test ./...` 全绿；语料快照更新为 838 个场景。

### 批次 54（导入卡包 10005 + 首批 27 张）

- 用 `./scripts/import_wbarts_packs.sh ../WBArts cards 10005` 导入 76 张骨架，
  语料快照的卡数 527 → 603。
- 完成 27 张（含 10004 已有的 10473110？否——本批全部来自 10005）：
  10571310 尽小花的临照、10512120 和气蔼蔼的妖精、10511110 熟虑的狸猫、10513310 优雅的虫风花、
  10572120 清宵玉兔、10552310 残虐的炸裂、10561310 雾卷花的激愤、10531310 明越花的转变、
  10522310 温柔援军、10562110 毫不动摇的圣骑士、10573310 诚心的尽小花、10523310 荣耀的丽金花、
  10542310 日珥咆哮、10511310 虫风花的飞翔、10501110 挥毫的怪物、10512110 新晋搭档、
  10551110 鼓舞之狼、10551120 红符的魂魄道士、10531110 创造魔法师、10512310 寂静的助力、
  10542110 铁锤龙骑士、10552110 制造麻烦的唤灵师、10531120 流动控符师、10541110 涌泉打水人、
  10571120 繁花技师、10562210 穹顶护甲、10532110 失眠女巫（纹章）。
- 动态抽牌（"抽 X 张，X 为连击/守护随从数"）用 `repeat <数值> { draw 1; }` 表达；
  "改为"型的超进化用 `superevolve replaces evolve`。
- 顺带修掉运行时缺陷：`damage all.leaders … lowest life` 原先用最大值比较，
  会让两个主战者都受伤（低生命中最小值的分支）。
- 新增 35 个场景（`tests/10005/batch-54-basics.wbotest`）。
- 新登记缺口：S-58 手牌中同费用的张数（10553310 严酷的奥夜花的纹章条件）、
  S-59 把卡牌加入牌组（10551310 奥夜花的开战）、S-60 按牌组随机随从变身的复制（10533310 壮美的明越花）。
- 全量回归：`check` 0 错 0 警；`test` 804 全绿；`go test ./...` 全绿；语料快照更新为 804 个场景。

### 批次 53（S-56 临时加减费 + 卡包 90000 收尾）

- **S-56 `raise|reduce cost <集合> N until …`**：费用修改支持期限。`AdjustEffect` 新增
  `Until`，`adjust_entity_field` 在到期侧记录差量（沿用 `temporaryCost`），
  `expireTurnEffects` 按差量还原；支持 `until [own|oppo] turn ends`。原先只有 `set cost` 能写期限。
- 完成 90044310 银冰吐息（模式一破坏对手所有受伤随从；模式二到对手回合结束前让对手手牌费用 +1）。
- 新增 2 个场景（`tests/90000/batch-53-temporary-cost.wbotest`）与 Go 单测
  `internal/runner/temporary_cost_test.go`（临时加费到期只撤销自己的差量，不吞掉期间的永久加费）。
- 新登记缺口：S-57 "判断选中的卡牌属于哪一方"（90064320 天书深渊"若选择了自己的护符"）；
  90034330 天晶深渊同时卡在 S-35（信仰值）与未导入的 10006 卡（天晶魔手）。
- 全量回归：`check` 0 错 0 警；`test` 769 全绿；`go test ./...` 全绿；语料快照更新为 769 个场景。

### 批次 52（S-55 护符灵气 + 覆盖口径修正 + 卡包 90000）

- **覆盖口径修正**：`scripts/card_worklist.mjs` / `card_coverage.sh` 原先只要文件里有
  `unplayable;` 就算未实现，于是"未来核心 / 过往核心"这类**真正**无法使用的卡永远无法计入完成。
  现在改为"`effect` 块里只有一条 `unplayable;`"才是骨架占位符；判定逻辑抽到
  `scripts/card_placeholder.mjs` 并配 `scripts/card_placeholder.test.mjs`（`node --test`）。
  修正后 90071210 未来核心、90071220 过往核心正确计入已完成。
- **S-55 护符【灵气】**：`aura` 是唯一允许出现在护符上的固有关键词（月影指环），
  其余关键词仍然只允许随从；运行时的目标保护本来就按战场实例判断，无需改动。
- 完成 16 张：90074140 伊鞠的小鬼、90014110 冰晶剑士·伊芙、90011120 新绿的妖精、
  90074210 新约·白之章、90074220 新约·黑之章、90074310 奏绝的独唱、90021210 令人战栗的海盗旗、
  90054130 一尾狐、90021350 闪耀的金币、90023110 安静的女仆·诺嘉、90044330 天刀深渊、
  90064210 月影指环、90014320 绝命的痛击、90074320 天斧深渊，以及上面两张核心卡。
- 新增 15 个场景（`tests/90000/batch-52-amulets-and-triggers.wbotest`）；Go 单测
  `internal/project/amulet_aura_test.go`。
- 全量回归：`check` 0 错 0 警；`test` 767 全绿；`go test ./...` 全绿；语料快照更新为 767 个场景。

### 批次 51（语言扩展 S-50 / S-53 / S-54 + 卡包 10004 / 90000）

- **S-50 `<绑定>.attack|life|cost` 作为数值**：`validNumericExpr`、`decodeNumericValue`（amount.go）、
  `ir.ValidBindingName`、`project/numeric.go`（不再是 own/oppo 的 `x.y` 一律按绑定实例读取）、
  `validate.go` 的 `parseEffectAmount`、`runner/numeric.go`（读取绑定里第一个实例的当前值）。
  用在这类文本："对对手的战场上的所有随从造成 X 点伤害，X 为选择的随从的攻击力"。
- **S-53 `raise countdown <集合> N`**：给倒计数"加"（原先只有 `reduce`）。
  **S-54 `set_attack_limit <目标> N`**：允许把"1回合可以攻击2次"给指定随从，不再只限 `self`。
  两者都只是验证与编译层的形状扩展，IR/运行时早就支持任意目标与正增量。
- 完成 20 张：
  - 10004：10473110 向往天空的回归者·卡西乌斯（按手牌创造物的攻击力对敌方全体造成伤害；
    谢幕曲把『城堡创造物』加入手牌）。
  - 90000：90021130 娜哈特的私兵（白板）、90022110 异形、90064110 圣骑兵、90071130 解析的创造物、
    90071140 古老的创造物、90071150 神秘的创造物、90071160 绚烂的创造物、90054330 天眼深渊、
    90034340 苍奏之四、90004320 绝大的证明、90031310 玛纳利亚魔弹、90024330 亡命者的枪击、
    90024320 天剑深渊（爆能强化 1 用 `enhance 1 replaces` 复现"改为3点"）、90014330 天枪深渊、
    90074150 击针看守、90072130 屠戮人偶、90032110 洋葱军团兵、90034350 宏大的回归、
    90064310 绝望的奔流（消失 + `raise countdown own.crests 1`）。
- 新增 16 个场景（`tests/90000/batch-51-tokens-and-spells.wbotest`）；Go 单测
  `internal/project/binding_scalar_test.go`。
- 全量回归：`check` 0 错 0 警；`test` 752 全绿；`go test ./...` 全绿；语料快照更新为 752 个场景。

### 批次 50（语言扩展 S-47 + 卡包 10004 第七批）

- **S-47 筛选器 `attacked this turn` / `not attacked this turn`**：按实例本回合的攻击次数筛选，
  用于"本回合中没有进行过攻击的进化前随从"。`validate.go` 的形状检查、
  `typed_ir.go` 的 `filterIR`、`ir/encode.go`/`decode.go` 的谓词白名单、
  `runner/execute.go` 的求值四处同步。
- 完成 3 张：10431120 流浪的家庭教师·斯芙拉玛尔（回合结束时按攻击力给所有手牌魔力增幅，
  写作 `repeat self.attack { spellboost own.hand 1; }`，无需新增动态增幅原语）、
  10434110 水之法则·瓦姆杜斯（魔力增幅时 +1/+1、入场曲增幅手牌、超进化模式二按攻击力分配伤害）、
  10464110 土之法则·伽莱翁（守护、无法攻击、超进化已解禁的回合结束时让未攻击过的进化前随从进化）。
- 新增 8 个场景（`tests/10004/batch-50-basics.wbotest`）；Go 单测
  `internal/project/attacked_filter_test.go`。
- **踩坑（重要）**：`filterIR` 的 `case "spellboost"` 漏掉 `j++` 会让编译卡包时死循环，
  而 `check` 只做语法/形状校验、不跑 `filterIR`，所以 `check` 全绿也发现不了——
  表现为 `wbo test` 卡死、`go run` 进程被 SIGKILL。教训：改 `filterIR` 这类带手工索引的
  循环后，必须至少跑一次 `wbo test`（或全量 `go test`），并给新分支补形状测试。
- 全量回归：`check` 0 错 0 警；`test` 736 全绿；`go test ./...` 全绿；语料快照更新为 736 个场景。

### 批次 49（语言扩展 S-45 / S-46 + 卡包 10004 第六批）

- **S-45 `add copies of <集合> to hand;`**：按目标当前的卡牌定义复制一张同名卡加入手牌，
  输出 `added`。可以复制已经被消失的实例（只读取卡牌身份）。
- **S-46 `summon <绑定>;`**：把手牌里的对象直接移动到战场，不发动入场曲，
  随从获得入场等待；仍然发出 `summoned` 事件。
- 完成 2 张：10443310 星晶兽吸收之力（选择敌方 1 张卡牌，使其消失并把同名卡加入手牌）、
  10412110 美妆少女·克洛伊（爆能强化 8：选择手牌中的 1 张随从召唤，本随从返回手牌）。
- 新增 5 个场景（`tests/10004/batch-49-basics.wbotest`）；Go 单测
  `internal/project/copies_and_hand_summon_test.go`。
- 全量回归：`check` 0 错 0 警；`test` 728 全绿；`go test ./...` 全绿；语料快照更新为 728 个场景。

### 批次 48（卡包 10004 第五批）

- 完成 9 张：10432310 能量外溢、10423110 真王之刃·黄金骑士、10434120 天才美少女炼金术士·卡莉奥丝特罗、
  10433110 缅怀之火·埃尔默特、10463210 蕾·菲耶的宝石、10454110 暗之法则·菲迪埃尔、
  10413310 亚尔夫海姆、10462210 骑驰天空之艇、10441120 梅格的挚友·玛丽亲。
- 这一批零新原语：【模式】三选一与"爆能强化_9 改为发动全部能力"
  （`enhance 9 replaces` 会跳过入场曲，因此模式选择不再出现）、
  敌方手牌全体 `buff`、纹章 + 回合开始触发、`engage 0` 启动（先预检 `require` 再破坏自身）、
  `necromancy` + 两次 `reanimate` 并 `evolve summoned silent`、
  "本随从超进化"模式（`superevolve self silent`）、手牌中的 `when own amulet engaged`
  减费与 `when own follower super_evolved while self in hand where base.cost == 3` 设费、
  回合结束强化超进化随从都直接可用。
- 新增 23 个场景（`tests/10004/batch-48-basics.wbotest`）。
- 教训：启动能力里 `require` 必须写在 `destroy self` 之前，否则合法性预检会因
  查询安全性降级而报 `unsupported_preflight`。
- 全量回归：`check` 0 错 0 警；`test` 723 全绿；`go test ./...` 全绿；语料快照更新为 723 个场景。

### 批次 47（语言扩展 S-43 / S-44 + 卡包 10004 第四批）

- **S-43 以纹章为效果目标**：`own.crests` / `oppo.crests` 成为可操作集合。运行时只有显式
  纹章集合才解析到纹章实体，其余路径（`self`、`field` 等）仍然看不到纹章；
  `destroy` 对纹章走 `expireCrest`（触发谢幕曲），IR 只接受 `zone:"crests"` 且
  `member:"card"` 的窄形式，避免把纹章当成普通卡移动或变身。
- **S-44 共享打出帧**：一次打出的外层效果、入场曲与爆能强化共用同一个 `frame`，
  后声明的块可以读取先声明块的输出；同时补登记了运行时早已存在的 `played` 事件绑定
  （`evolve played silent;`）。
- 完成 14 张：10442310 至爱狂轰、10463110 魔杖傍身的外科医生·缇可、10462110 沙神的巫女·莎拉、
  10442110 冰封的命运·伊什米尔、10452110 霸空武神·哪吒、10462120 赞恩教僧侣·索菲娅、
  10461120 克己复礼的修女·拉姆蕾达、10472110 轰雷闪狼·尤斯提斯、
  10424110 真红与群青·塞达&贝阿朵丽丝、10472120 严厉的教官·伊尔莎、10452130 元素共鸣·巴尔，
  以及三张纹章卡 10414120 调和的舞者·尤艾尔&苏丝雅、10453310 堕落、10454120 狡诈的堕天司·彼列。
- 新增 28 个场景（`tests/10004/batch-47-basics.wbotest`、`batch-47-crests.wbotest`）；
  Go 单测 `internal/project/crest_target_and_play_frame_test.go`、
  `internal/runner/enhance_play_frame_test.go`。
- 修正两处既有实现：`producedBindings` 漏掉 `add` 的 `added` 输出；检查器漏登记 `played` 绑定。
- 全量回归：`check` 0 错 0 警；`test` 700 全绿；`go test ./...` 全绿；语料快照更新为 700 个场景。

### 批次 46（语言扩展 S-42：伤害上限与主战者受伤 +1 + 卡包 10004 第三批）

- **新固有关键词 `damage_cap N`**：实例每次受到的伤害最多为 `N`，对应卡面
  "受到的 N+1 点或以上的伤害变为 N 点"。`internal/project/validate.go`（与 `damage_reduction` 同一形状）、
  `strict_validate.go`（固有能力形状表）、`typed_ir.go`（写入 `IntrinsicState`）、
  `internal/ir/decode.go`（固有状态白名单）、`internal/runner/runner.go`（`instance.damageCap`、
  `resetCardState`、`modifyDamage` 在减伤之后压上限）、`internal/runner/continuation.go`（`DamageCap` 快照与恢复）。
- **主战者关键词 `damage_taken_up`**："受到的伤害 +1"，在 `damageLeaderFrom` 与 `damageLeaders`
  的屏障判定前先加一（因此与【屏障】同时存在时下一次伤害仍为 0，符合官方 QA）。
  `internal/ir/decode.go` 的 `validKeyword`、`validate.go` 的 `abilities`、`strict_validate.go`
  的形状表三处同步；卡牌用 `add damage_taken_up to oppo.leader;` 赋予。
- 完成 4 张：10401110 驰骋天空的守护者·卡塔莉娜（奥义随机 2×5 + 守护 + 伤害上限）、
  10464120 威严的星晶骑士·薇拉（消失 2 个随从 + 解放奥义超进化 + 守护 + 伤害上限）、
  10474120 唯一王者·别西卜（选 2 个随从失去所有能力并各受 9 点伤害，对手主战者受伤 +1）、
  10444110 炎之法则·威尔纳斯（选 1 个随从 8 点伤害 + 威慑 + 进化时重复入场曲）。
- 新增 7 个场景（`tests/10004/batch-46-basics.wbotest`）；Go 单测
  `internal/runner/damage_cap_test.go` 覆盖伤害上限的三种输入、主战者受伤 +1、
  以及"受伤 +1 与屏障同时存在时下一次伤害为 0"。
- 全量回归：`check` 0 错 0 警；`test` 672 全绿；`go test ./...` 全绿；语料快照更新为 672 个场景。

### 批次 45（卡包 10004 第二批）

- 完成 18 张卡：10451110 憧憬的铁锤·阿尔梅达、10472310 身无长物唯有石、10422110 冰心霸王、
  10452120 爱的旅人·萨堤洛斯、10471110 夜王再起·翔、10431310 符文秘术、10403120 露莉亚、
  10402110 宙域使者·尤妮、10451120 不屈利刃·巴萨拉卡、10473310 混沌军势、
  10411120 可爱如琬似花·玛娜玛尔、10432110 报仇的占卜师、10432120 荆棘旅途、
  10453110 生与死之技·涅槃、10411310 彗星、10461210 莉莉艾的鼓舞，以及两张纹章卡
  10451310 妖异利刃、10412310 绮罗星（连击 5 获得纹章）。
- 这一批零新原语：爆能强化自行进化、`repeat` 随机伤害、`count(... where form evolved)`、
  `own.superevolve_unlocked`、纹章 + 吟唱 + 谢幕曲、`transform` 成护符、土之秘术让召唤物与本体
  一起进化都直接可用。
- 新增 25 个场景（`tests/10004/batch-45-basics.wbotest`、`batch-45-crests.wbotest`）。
- 全量回归：`check` 0 错 0 警；`test` 665 全绿；语料快照更新为 665 个场景。

### 批次 44（语言扩展 Skybound Art + 卡包 10004 / 90000 等）

- **新机制【奥义】/【解放奥义】**（官方术语表 Skybound Art / Super Skybound Art）：
  奥义槽 = 当前回合数 + 本卡牌在手牌中时己方随从进化过的次数。
  - 条件 `if skybound_art { … }`（≥10）与 `if super_skybound_art { … }`（≥15）；
  - 操作 `gain skybound 集合 N;`（"使自己的所有手牌的奥义槽 +1"）；
  - 实例新增 `skybound` 计数，己方随从每次进化（含超进化，只计一次）给手牌里的每张牌 +1，
    连击快照一并保存。
- 筛选器新增 `base.cost` / `base.attack` / `base.life` 比较（原始定义），与按当前值的 `cost` 区分。
- **修一个潜伏已久的解析顺序 bug**：`conditionIR` 里"`if 条件 { } else { }`"的 else 截断发生得
  太晚，导致带 else 的 `skybound_art` 之类的单 token 条件被当成 `overflow`。已把 else 截断提到
  函数开头；这也顺带让 `count(...)` 等条件与 else 组合时更稳。
- 修一个既有的编解码不一致：`gain own.ep N`（以及 sep/rally）能通过校验，但容器解码的白名单里
  没有它们，会在编译卡包时报 `invalid resource adjustment`。运行时与解码器都已补齐。
- 完成 9 张卡：10414110 风之法则·艾云尼亚、10403110 征服苍空的骑空士、10413110 幻彩弓手·丘比丹、
  10424120 十天众统领·希耶提、10433310 炼金炎爆、10442120 淳朴的钢铁之躯·无限、
  10471120 爆燃老大·翼、10443110 平平无奇的女孩·梅格、90034320 伟大之术。
- 新增 14 个场景（`tests/10004/batch-44-skybound-art.wbotest`）+ 1 个 Go 测试
  （奥义槽按手牌中的进化次数累加、阈值 10/15 的边界）。
- 全量回归：`check` 0 错 0 警；`test` 640 全绿；语料快照更新为 640 个场景。

### 批次 43（导入 10004 + 首批 15 张）

- 导入卡包 10004 的 75 张骨架，语料快照卡数 452 → 527。
- 完成 15 张卡：10422120 如火斗志·菲泽（白板）、10422130 绽放的肌肉、10441110 逐海之人·乔尔、
  10431110 不可思议的哲学家、10441310 破浪新月（纹章）、10421110 信念腿法·兰德尔、
  10471130 报恩工匠、10412120 绯焰舞姬、10421130 迷茫的狮子、10421120 决心之辉龙·亚瑟、
  10461110 英雄幻视·托路、10401120 亲爱的搭档·碧。这一批零新原语，用的都是既有的
  潜行/毁灭/守护/灵气/屏障、爆能强化、谢幕曲加入手牌、`when own amulet engaged` 监听
  与 `own.superevolve_unlocked` 条件。
- 新增 15 个场景（`tests/10004/batch-43-basics.wbotest`、`batch-43-crest.wbotest`）。
- 10004 里最显眼的新机制是【奥义】/【解放奥义】（全卡 18 张）：官方术语表写明
  奥义槽 = 当前回合数 + 本卡牌在手牌中时己方随从的进化次数，奥义槽 ≥10 触发【奥义】、
  ≥15 触发【解放奥义】（英文 Skybound Art / Super Skybound Art）——下一批实现。
- 全量回归：`check` 0 错 0 警；`test` 626 全绿；语料快照更新为 626 个场景。

### 批次 42（语言扩展 S-32 + 卡包 10003）

- **S-32 完成**：筛选器支持 `where card <绑定>`——匹配与绑定实例**同一卡牌定义**的对象。
  为了能在筛选里解析绑定，`filter`/`matches` 全程带上当前绑定帧（只有 6 处调用点）；
  配合既有的 `superevolve extends evolve`（两步共享绑定帧），超进化就能沿用普通进化
  那次选择的目标。
- 完成 1 张卡：10334110 真理的继承者·蓓哈丽雅（入场曲抽 1 张；进化时选择敌方 1 个随从
  使其消失；超进化时让与它同名的所有敌方随从一起消失）。
- 新增 3 个场景（`tests/10003/batch-42-same-card.wbotest`）+ 1 组 Go 测试
  （同名筛选形状 + `extends` 计划确实共享绑定帧）。
- 收益：全卡还有 12 张"同名"文本（10354110、10443310、10572310、90074320…）可用同一机制。
- 全量回归：`check` 0 错 0 警；`test` 611 全绿；语料快照更新为 611 个场景。

### 批次 41（语言扩展 S-34 + 卡包 10003）

- **S-34 完成**：筛选器新增 `where cost changed`。实例记录 `costChanged`，加费/减费/设置费用
  （`reduce`/`raise`/`set cost`/`halve cost`）都会置位，连击快照一并保存；
  配合上一批的"自己使用卡牌时"事件就能表达"自己使用费用发生变化的随从时"。
- 完成 1 张卡：10332210 真理的研究设施（吟唱 5；自己使用费用发生变化的随从时抽 1 张并让
  本护符的倒计数 -1；启动 1 给手牌随从 +1 费并 +1/+1）。
- 新增 2 个场景（`tests/10003/batch-41-cost-changed.wbotest`）+ 1 个 Go 测试
  （先用法术改费、再打出那个随从，验证触发抽牌与倒计数递减；场景一个动作做不了这两步）。
- 踩坑：新的谓词种类要在解码器与编码器两处白名单同时登记，否则会在编译卡包时才报
  `encode unknown field predicate kind`。
- 全量回归：`check` 0 错 0 警；`test` 608 全绿；语料快照更新为 608 个场景。

### 批次 40（语言扩展 S-40 + 卡包 10003 / 90000）

- **S-40 完成**：`set cost T N until turn ends|own turn ends|oppo turn ends;`。实例新增
  `temporaryCost`（按结束方记录差量），到期时在 `expireTurnEffects` 里按差量还原——
  这样到期前若又叠加了永久加减费也不会被覆盖。连击快照一并保存。
- 完成 2 张卡：10334120 绝尽的显现·莱奥（牌组所有随从 -3 费；随机 1 张手牌法术变身为
  绝尽的伪证并让它在回合结束前变成 0 费）、90034310 绝尽的伪证（手牌随从 +1 费；破坏敌方所有随从）。
- 新增 2 个场景（`tests/10003/batch-40-temporary-cost.wbotest`）+ 1 个 Go 测试
  （临时费用在回合结束时还原，场景只能写一个动作所以必须用 Go 覆盖）。
- 踩坑：10334120 是 9 费，测试里给 8 点能量点会直接 `illegal cost`；另外 `set cost` 的
  `until` 子句在编码器与解码器两处白名单都要放行，否则只改一半会在编译卡包时炸。
- 全量回归：`check` 0 错 0 警；`test` 606 全绿；语料快照更新为 606 个场景。

### 批次 38–39（语言扩展 S-36 + 卡包 10003）

- 批次 38：完成 10342120 海洋骑手（觉醒时召唤 2 个大海虎鲸而不是 1 个；己方海洋随从
  入场给守护），用已有原语即可，新增 2 个场景。
- 批次 39：**S-36 完成**——`mode N { … }`，【模式】选择 N 个能力发动。回放协议新增
  `SelectedOptionIDs`（多选响应）与测试写法 `mode 1, 3;`；请求的最小/最大数量等于 N，
  选项必须互不相同，按选项编号从小到大结算，连击快照沿用挂起请求里保存的数量。
  完成 10353310 叫唤与憎恶（四个选项里选两个：回复 1 点能量点、抽 1 张随从、
  抽 1 张费用 3 的法术、对随机敌方随从造成 4 点伤害）。
- 新增 5 个场景（`tests/10003/batch-38-marine-rider.wbotest`、`batch-39-multi-mode.wbotest`）
  + 2 个 Go 测试（多选请求跨连击快照恢复、重复选项被拒绝）。
- 踩坑：测试 DSL 的 `mode` 响应校验原本按"整数、逗号、整数"逐 token 检查，
  写成 `mode 1, 3` 时逗号会被当成编号，需要按步长 2 迭代；另外打出法术进墓场本身
  会加 1 点墓场资源，断言时不要漏算。
- 全量回归：`check` 0 错 0 警；`test` 604 全绿；语料快照更新为 604 个场景。

### 批次 37（语言扩展 S-24/S-26 + 卡包 10003）

- **S-24 完成**：`own|oppo.deck has [no] duplicates` 条件（按卡牌 ID 扫描牌组，空牌组视为没有重复）。
- **S-26 完成**：`banish duplicates in own|oppo.deck;`——按牌组顺序保留每种卡牌的第一张，
  其余移入消失区（普通区域移动，不是破坏，因此不触发破坏类监听）。
- 完成 2 张卡：10301310 至高的凌驾（抽 3 张，然后若牌组没有重复卡牌则回复 3 点能量点）、
  10303210 试炼的石板（入场曲给牌组去重；启动 1 抽 1 张）。
- 新增 4 个场景（`tests/10003/batch-37-deck-shape.wbotest`）+ 1 组 Go 测试。
  踩坑：至高的凌驾的条件是在**抽牌之后**求值的，我第一版把两张同名牌放在牌组顶部，
  抽 3 张后牌组空了（没有重复）反而满足条件——对照场景必须让抽牌之后仍留着一对同名牌。
- 顺带完成 10342120 海洋骑手（觉醒时召唤 2 个大海虎鲸而不是 1 个；己方海洋随从入场给守护）。
- 全量回归：`check` 0 错 0 警；`test` 601 全绿；语料快照更新为 601 个场景。

### 批次 36（语言扩展 S-27 + 卡包 10003）

- **S-27 完成**：两个身材事件——`when self stats increased`（本实例在战场上获得攻击力/生命值增加）
  与 `when own|oppo follower life decreased`（战场上随从生命值减少），都支持 `once per own turn`。
  实现上刻意分开：身材增加在 `buff_stats` 发独立事件（临时增益到期不算增加）；生命值减少
  减益时发独立事件，**伤害则让原有 `damaged` 事件一并匹配 `life_decreased` 监听**——
  第一版给伤害也补发了一条新事件，结果三处既有的"事件流精确断言"测试全红（伤害事实翻倍），
  改成"同一事件按两个种类匹配"后事件流保持不变。`set life` 的设置不算减少。
- 完成 2 张卡：10361120 圣骑士团员（获得身材增加时回复主战者 1 点）、
  10314110 不弑的继承者·库露露（入场曲让敌方全体 -0/-2；潜行；敌方随从掉血时每回合回 1 点；
  超进化让**对手**获得纹章，对手的随从入场时各 -1/-1）。
- 新增 6 个场景（`tests/10003/batch-36-stat-change-events.wbotest`）+ 1 组 Go 测试。
  场景设计上用了两条"有来源但条件不成立"的对照（鲸鱼骑兵在场但无超进化随从、两个敌方随从
  同时掉血只回 1 点），比单纯空场更能证明触发条件。
- 全量回归：`check` 0 错 0 警；`test` 595 全绿；语料快照更新为 595 个场景。

### 批次 35（语言扩展 S-39 + 卡包 10003）

- **S-39 完成**：`random … count <数值表达式>`。数量可以是"纹章数""其他卡牌张数"这类动态值，
  结算到该语句时求值，0 表示不选、超过候选数按候选数截断；续局恢复沿用挂起请求里保存的数量。
- 完成 2 张卡：10373110 破坏的团结者（随机破坏 X 个敌方随从并破坏自己的其他卡牌）、
  10364110 安息的继承者·妃花（超进化把敌方全体攻击力设为 4；纹章按纹章数随机给敌方随从
  「无法攻击」与「自己的回合结束消失」）。
- 新增 6 个场景（`tests/10003/batch-35-dynamic-random-count.wbotest`）+ 1 组 Go 测试。
  两处断言修正：被破坏的自己的卡牌当然不在场上；随机选择只能给"唯一候选"做确定性断言，
  所以测试里既用单候选锁定结果，也用攻击力 5 的随从验证筛选边界。
- 新登记 S-41（数值算术），10811110 需要"两个计数相减"。
- 全量回归：`check` 0 错 0 警；`test` 589 全绿；语料快照更新为 589 个场景。

### 批次 34（语言扩展 S-31/S-33 + 卡包 10003）

- **S-31 完成**：主战者级关键词。`add barrier to own.leader;` 让主战者的下一次伤害降为 0
  并消耗【屏障】（官方 QA 明确：与"受到的伤害 +1"同时存在时也是 0）。玩家状态新增
  `leaderAbilities`，主战者伤害路径与连击快照同步。
- **S-33 完成**：`reduce countdown T 数值引用`——"本护符的倒计数 -X，X 为自己的纹章数"
  可以直接写。`AdjustEffect` 新增 `DeltaExpr`（IR 里是 `deltaValue`），
  常量与动态增量共用同一条求值路径。
- 完成 2 张卡：10362210 安息的神殿、10363210 闪耀的失意。
- 新增 4 个场景（`tests/10003/batch-34-countdown-and-leader-barrier.wbotest`）+ 2 组 Go 测试。
  踩坑：护符的吟唱是在**持有者回合开始**递减的，用 `end_turn` 验证时必须把护符放在对手一侧
  （和自己回合的纹章测试相反）；主战者【屏障】本身没法在场景里直接观测，用 Go 测试覆盖。
- 全量回归：`check` 0 错 0 警；`test` 583 全绿；语料快照更新为 583 个场景。

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
