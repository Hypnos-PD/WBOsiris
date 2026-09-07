# WBO 类型化中间表示 0.2

本文定义 WBO 卡牌规则与 WBO 测试场景的语言无关中间表示。它是编译器、服务器
规则引擎和 WASM 规则引擎之间的稳定边界，不是新的卡牌编写语言。卡牌作者仍然
编写 `.wbo` 和 `.wbotest`；运行时只加载 `.wbos` 中经过验证的 `CardPack` 和
`TestPack`。外层编码和版本规则见 [WBOS 编译容器](wbos.md)。

本文使用代数数据类型记法描述模型：`A | B` 表示判别联合，`T?` 表示可选值，
`[T]` 表示有序列表，`Map<K, V>` 表示映射。所有判别联合在 JSON 中都使用 `kind`
字段；所有整数都必须经过范围检查，不允许依赖宿主语言的隐式溢出。

## 设计目标

- 完整表达当前 [卡牌 DSL](../language/cards.md)、[测试 DSL](../language/tests.md)、`cards/**/*.wbo` 和
  `tests/**/*.wbotest` 中出现的语法与语义。
- 解析、语义检查和语法糖展开只发生在编译期，运行时不解析源文本或本地化文本。
- IR 数据只包含定宽整数、布尔值、UTF-8 字符串、有序列表、映射和判别联合，便于
  Go、WASM 及其他实现共享。
- 同一规则集、卡牌包、初始状态、玩家指令、选择结果和随机种子在所有合规实现中
  产生相同结果。
- 每个可执行节点都可追溯到源文件，并具有跨重复编译稳定的标识。
- 未知的新类型不得被旧运行时静默执行。

## 编译管线

```text
Source -> AST -> 语义检查 -> 规范化 IR -> CardPack/TestPack -> .wbos
```

1. `Source` 阶段读取 UTF-8 文件，保留字节偏移、行列号和文件内容哈希。
2. `AST` 阶段只反映语法结构；此时允许名称引用、语法糖和 `replaces`、`extends`。
3. 语义检查阶段解析卡牌引用，推导表达式及绑定类型，检查作用域、目标区域、选项
   编号、能力组合和整数范围。
4. 规范化 IR 阶段展开语法糖与能力继承，生成稳定 ID、来源链和显式类型；IR 中不
   保留待运行时解释的源语言片段。
5. 包阶段按规范编码排序并计算哈希，写入依赖、编译器版本和 IR 版本。

测试文件先独立解析，再在 `use cards` 指向的卡牌集合上完成名称、卡牌 ID 和实例
类型检查。测试运行器不得绕过公开动作接口调用引擎内部函数。

## 基础类型

```text
CardId       = u32
PackId       = u32
OptionId     = u32
TurnNumber   = u32
Amount       = i32
NodeId       = 128 位小写十六进制字符串
SourceId     = 128 位小写十六进制字符串
InstanceId   = 128 位小写十六进制字符串
CommandId    = 128 位小写十六进制字符串
RequestId    = 128 位小写十六进制字符串
ContentHash  = "sha256:" + 64 位小写十六进制字符串
SemVer       = "主版本.次版本.修订版本"
Seed         = u64
```

`CardId`、`PackId` 和源语言中的数值卡牌编号在 JSON 中编码为无符号十进制数。
所有 `u64` 在线格式字段必须编码为带 `0x` 前缀的 16 位小写十六进制字符串，避免
WASM 宿主的 JSON 数值精度损失；源语言中的种子仍写十进制。运行时实例不得使用
内存地址作为 `InstanceId`。

### SourceSpan 与来源

```text
SourceFile = {
  sourceId: SourceId,
  path: string,
  contentHash: ContentHash
}

SourceSpan = {
  sourceId: SourceId,
  startByte: u32,
  endByte: u32,
  startLine: u32,
  startColumn: u32,
  endLine: u32,
  endColumn: u32
}

Origin = {
  primary: SourceSpan,
  expansion: [ExpansionFrame]
}

ExpansionFrame = {
  kind: "declared" | "extends" | "replaces" | "desugared",
  span: SourceSpan,
  fromNodeId: NodeId?
}
```

行列号从 1 开始，字节区间为左闭右开。诊断使用 `SourceSpan`；运行时事实使用
`NodeId`，需要调试时再通过包内来源表反查。

### 稳定 ID

`SourceId` 是规范化相对路径的 SHA-256 截断值。`NodeId` 由固定命名空间、卡牌
ID 或场景 ID、节点语义角色、父节点 ID、节点规范化语法指纹以及同指纹兄弟节点
序号计算。空白、本地化文本和源文件绝对路径不参与卡牌规则节点指纹。插入不同
指纹的兄弟节点不会改变既有 ID；修改节点语义只改变该节点及依赖其父 ID 的后代。

同一作用域出现完全相同的兄弟节点时，以它们在源中的相对顺序作为同指纹序号。
编译器检测到 128 位截断碰撞必须报错，不得自动换盐。测试场景 ID 由测试文件
`SourceId` 与场景名称生成；同一文件内场景名称必须唯一。

## 纹章定义

`Card.crest` 是可选的嵌套定义，包含 `counters`、`countdown`、`abilities`、五种
`locales` 与 `origin`。省略或为零的 `countdown` 表示永久纹章。能力 ID 在独立
`crest` 作用域生成，与所属卡牌能力不得碰撞。只允许独立关系的谢幕曲及事件能力，
事件来源不能指定手牌、战场或 `selfOnly`。

获得操作为 `{ kind: "gain_crest", owner: "own" | "oppo", cardId: CardId }`，连同
普通效果节点的稳定 ID 与来源字段编码。加载器必须确认引用卡牌声明了纹章。
运行时从嵌套定义派生 `cardType: "crest"` 实例，所属卡牌 ID 不变，实例 ID 独立。
公开状态的 `PlayerView.crests` 保留获得顺序，实体的 `crestLocales` 携带纹章本地化，
避免客户端误显示所属随从的能力。普通卡牌 `ZoneRef` 不接受 `crests`。
测试状态允许第七个区域 `crests`，其中实例 `declaredType` 必须为 `crest`。

## 卡牌定义

```text
Card = {
  crest?: CrestDefinition,
  counters?: { CounterName: nonnegative_i32 },
  id: CardId,
  cardType: CardType,
  cost: u16,
  stats: Stats?,
  traits: [TraitId],
  restrictions: [CardRestriction],
  intrinsic: [Keyword],
  intrinsicState: [IntrinsicState],
  abilities: [Ability],
  fusionAbilities: [FusionAbility],
  actionPlans: [ExecutionPlan],
  meta: Meta,
  locales: Map<LocaleCode, Locale>,
  origin: Origin
}

CardType = "follower" | "spell" | "amulet"
Stats = { attack: i16, life: i16 }
TraitId = string
Keyword = "ward" | "storm" | "rush" | "bane" | "drain" | "intimidate"
        | "barrier" | "stealth" | "aura" | "ability_target_guard"
        | "cannot_attack" | "cannot_attack_follower" | "cannot_attack_leader"

CardRestriction =
  Unplayable { kind: "unplayable" }

Meta = {
  pack: PackId,
  class: ClassId,
  rarity: Rarity
}

ClassId = "neutral" | "forestcraft" | "swordcraft" | "runecraft" |
          "dragoncraft" | "abysscraft" | "havencraft" | "portalcraft"
Rarity = "bronze" | "silver" | "gold" | "legendary"
LocaleCode = "chs" | "eng" | "jpn" | "kor" | "cht"
Locale = { name: string, text: string }
```

`follower` 必须有 `stats`，其他类型不得有 `stats`。`trait` 可重复声明，编译后按
源顺序去重。直接位于 `effect` 中的 `ward`、`storm`、`rush`、`bane`、`drain`、
`intimidate`、`barrier`、`stealth`、`aura`、`ability_target_guard`、`cannot_attack`、`cannot_attack_follower`、
`cannot_attack_leader`
编译为 `intrinsic`；它们不是进入战场时执行的 `AddKeyword`。
`locale` 只用于展示，必须按 `chs`、`eng`、`jpn`、`kor`、`cht` 规范顺序编码，
规则引擎不得读取 `Locale.text` 决定行为。

`intimidate` 的战斗合法性规则是敌方随从不能攻击该实体，但目标选择、能力伤害及
其他非攻击效果不受影响。同一实体同时具有 `intimidate` 与 `ward` 时，`ward` 不
参与攻击目标约束。`unplayable` 编译为 `CardRestriction`，只拒绝以该手牌实例为
来源的 `Play` 指令，不拒绝融合指令或规则效果。

`countdown N` 和 `earthsigil` 编译为 `IntrinsicState`，而不是关键词：

```text
IntrinsicState =
  Countdown  { kind: "countdown", initial: u16 }
| EarthSigil { kind: "earthsigil", initial: 1 }
```

护符实例在控制者回合开始时递减倒计时；变为零时立即执行破坏流程。

## 值、集合与绑定

`DestructionBatchRef { kind: "destruction_batch", targets: [SelfRef | BindingRef] }`
仅允许直接作为 `destroy.target`；至少两个引用，不允许重复引用、嵌套批次或附加
谓词。它在执行时合并绑定中的实例、去重后交给一次破坏操作，保持单个死亡批次。
源语言 `destroy ally, enemy;` 编译成一个带该目标的 `TargetEffect`，输出仍为 `destroyed`。

多个独立必选目标的来源限于区域查询、其静态筛选、排除 `self` 或混合角色集合。
选择仅改变绑定，不改变这些候选，因此可在同一指令支付前检查所有 `require`。
若前置操作改变了状态，或后续来源依赖选择结果，则仍返回 `unsupported_preflight`；
条件分支中的状态变更也会使后续前置查询失效。

当前可执行 IR 的混合选择来源为 `CharacterSetRef { kind: "characters", side: Side }`，
仅允许直接用作选择节点的 `source`。它表示指定方的场上随从，随后附加该方主战者；
不能嵌入 `filter`、`exclude` 或附带 `extremum`。`side` 在此处相对能力来源解释。
普通绑定使用实例引用；混合绑定可同时包含实例与主战者。直接 `count(binding)`
统计全部目标，带卡牌筛选的计数仅统计实例。源语言限制混合绑定作为操作目标时
只能用于无筛选的伤害，避免把主战者传给卡牌专用操作。

### ValueRef

```text
ValueType = "entity" | "entity_set" | "player" | "leader" | "card" |
            "integer" | "boolean" | "zone" | "option"

ValueRef =
  SelfRef      { kind: "self", valueType: "entity" }
| OwnerRef     { kind: "owner", side: Side, valueType: "player" }
| LeaderRef    { kind: "leader", side: Side, valueType: "leader" }
| BindingRef   { kind: "binding", bindingId: NodeId, valueType: ValueType }
| CandidateRef { kind: "candidate", valueType: "entity" }
| LiteralInt   { kind: "integer", value: i32, valueType: "integer" }
| CardRef      { kind: "card", cardId: CardId, valueType: "card" }
| FieldRef     { kind: "field", base: ValueRef, field: FieldName,
                 valueType: ValueType }

Side = "own" | "oppo"
FieldName = "life" | "attack" | "cost" | "combo" | "pp" | "maxpp" |
            "ep" | "sep" | "shadows" | "countdown" | "earthsigil" |
            "zone" | "evolved" | "super_evolved" | "engaged"
```

`self` 在卡牌能力中指能力来源实体；法术结算期间指该法术实例。`own` 和 `oppo`
相对于能力控制者求值，不相对于当前回合玩家求值。`target`、`summoned`、`drawn`
不作为魔法字符串保留，而是在编译时解析成具体 `BindingRef`。

### SetExpr

```text
SetExpr =
  ZoneSet     { kind: "zone", side: Side?, zone: Zone, member: MemberKind }
| BindingSet  { kind: "binding_set", bindingId: NodeId }
| Singleton   { kind: "singleton", value: ValueRef }
| FilterSet   { kind: "filter", source: SetExpr, predicate: BoolExpr }
| ExcludeSet  { kind: "exclude", source: SetExpr, value: ValueRef }

Zone = "deck" | "hand" | "field" | "graveyard" | "banished" |
       "destroyed"
MemberKind = "card" | "follower" | "spell" | "amulet"
```

`field.followers` 的 `side` 为空，表示双方战场集合。`other` 编译为
`ExcludeSet(source, self)`，`where` 编译为 `FilterSet`。过滤谓词中的省略主语字段
统一绑定到 `CandidateRef`；例如 `where life <= 3` 比较当前候选的生命值。

牌组保持从顶到底顺序，手牌和单方战场保持槽位顺序。双方集合的拼接顺序、墓场与
消失区顺序由版本化规则集的 `OrderingPolicy` 决定，卡牌 IR 不得自行指定，也不得
依赖哈希映射迭代顺序。

### BoolExpr 与数值表达式

下列 `IntExpr` 是通用表达式模型；当前执行格式中的常量数值仍直接编码为 JSON
整数，不使用 `literal` 包装。`Damage.amount`、`Heal.amount`、`BuffStats.attackDelta`
和 `BuffStats.lifeDelta` 支持下方 `NumericExpr`。集合计数与求和来源支持区域集合、
当前回合破坏履历、集合绑定，以及对这些来源的一层筛选；区域省略 `side` 时只允许
`field`。求和字段限于卡牌定义中的原始攻击力、生命值和费用，筛选条件仍读取实例或
破坏履历快照。增益还支持单层 `negate`，不能嵌套取负或包装整数。
其他操作的数值字段仍只接受整数；尚未实现通用 `IntValue` 或一般算术求值。

```text
IntExpr =
  IntLiteral { kind: "literal", value: i32 }
| IntValue   { kind: "value", value: ValueRef }

BoolExpr =
  BoolLiteral { kind: "literal", value: bool }
| Compare     { kind: "compare", op: "eq" | "ne" | "lt" | "le" | "gt" | "ge",
                left: IntExpr, right: IntExpr }
| And         { kind: "and", terms: [BoolExpr] }
| Or          { kind: "or", terms: [BoolExpr] }
| Not         { kind: "not", term: BoolExpr }
| HasType     { kind: "has_type", value: ValueRef, cardType: CardType }
| HasClass    { kind: "has_class", value: ValueRef, class: ClassId }
| HasCard     { kind: "has_card", value: ValueRef, cardId: CardId }
| HasTrait    { kind: "has_trait", value: ValueRef, trait: TraitId }
| HasForm     { kind: "has_form", form: "unevolved" | "evolved" | "super_evolved" }
| HasSpellboost { kind: "has_spellboost" }
| HasKeyword  { kind: "has_keyword", value: ValueRef, keyword: Keyword }
| Overflow    { kind: "overflow", side: Side }
| SelfForm    { kind: "self_form", form: "unevolved" | "evolved" | "super_evolved" }
| EvolutionUnlocked { kind: "evolution_unlocked", side: Side, form: "evolved" | "super_evolved" }
```

当前 `where type follower and class swordcraft` 编译为两个谓词的 `And`；
`where card 90073120 or card 90073130` 编译为 `Or`。混合过滤器先将连续 `and`
编译为组，再以 `Or` 连接各组；每个逻辑节点至少包含两个子项。
`where life <= 3` 编译为 `Compare`；`if combo >= 3` 默认读取 `own.combo`；
`if overflow` 编译为 `Overflow(own)`。IR 不保留这些省略写法。
`if self form super_evolved` 编译为 `SelfForm`，不建立候选集合；它读取能力来源当前的
随从形态，`evolved` 包含超进化，非随从不匹配。`if own.superevolve_unlocked`
编译为 `EvolutionUnlocked(own, super_evolved)`；`evolve_unlocked` 使用 `evolved`。
解禁条件只比较对应玩家的先后手回合门槛，不检查点数、当前行动方或当回合已用次数。
条件在执行到 `If` 节点时求值，合法性预检使用相同规则。未知形态、非法玩家及额外字段
拒绝解码；暂停续局沿用既有的来源实例、形态、先手和回合记录，无需新增存档字段。

`HasForm` 使用当前候选随从的形态；`evolved` 包含超进化，非随从总是不匹配。
`HasSpellboost` 检查候选当前卡牌定义是否声明 `spellboost` 触发能力，不读取卡牌文本，
不检查职业或卡牌类型；没有额外字段。变身后按新的卡牌定义判断。
筛选器的 `compare` 节点支持 `field: "life" | "cost"`；`cost` 读取实例当前费用。
其 `value` 保留整数编码，并支持 `{kind:"scalar", side:"own"|"oppo", field:...}`，
字段限于 `combo`、`pp`、`maxpp`、`life`、`ep`、`sep`、`shadows`、`hand_count`、`earthsigils`。玩家引用相对于
能力来源求值，不能使用候选拥有者或抽牌接收者替代；测试断言中对应测试席位。
缺失值、`null`、自身字段、算术和聚合表达式均拒绝解码。筛选时读取玩家值；效果在
任何目标修改之前完成集合筛选，事件触发器在事件入队时完成筛选。暂停续局只保存既有
执行栈和玩家资源，尚未执行的筛选在恢复后求值，无需新增续局字段。
`when self evolved` 编译为 `{kind:"event", event:"evolved", side:"own", subjectType:"follower", selfOnly:true}`；
`when self super_evolved` 将 `event` 改为 `super_evolved`。`selfOnly` 必须按实例身份匹配，
上述进化事件只允许己方随从且不附带谓词。`when self summoned` 使用相同的自身匹配结构，
将 `event` 设为 `follower_summoned`；仅允许随从声明，不附带谓词或次数限制。
它在该实例通过打出或召唤进入战场时触发，其他实例入场或变身均不触发。
`when self discarded` 编译为
`{kind:"event", event:"card_discarded", side:"own", selfOnly:true}`，不声明 `subjectType`。
该弃牌触发器由已进入墓场的被舍弃实例派发，不依赖战场监听索引。
普通弃牌监听器省略 `selfOnly`，使用 `side` 匹配拥有者并用 `discarded` 绑定事件对象。

任何逐候选求值的谓词都会在自身作用域建立只读 `CandidateRef`，包括
`FilterSet.predicate`、事件对象过滤器、`Draw.predicate`、`ZoneCount.predicate`
以及测试中的 `all ... where`。`HasType`、`HasClass`、`HasCard`、`HasTrait` 及省略
主语的字段比较都以它为参数；候选引用不能逃逸到对应谓词之外，也不能被操作写入。

### Binding 与生命周期

```text
Binding = {
  id: NodeId,
  name: string,
  valueType: ValueType,
  cardinality: "zero_or_one" | "many" | "exactly_one",
  producerNodeId: NodeId,
  scopeId: NodeId,
  lifetime: "resolution",
  replacePrevious: bool,
  origin: Origin
}
```

每次效果结算建立一个绑定帧。`target` 的类型是 `entity`、基数为
`zero_or_one` 或 `exactly_one`；`summoned`、`drawn` 的类型是 `entity_set`、基数
为 `many`。集合仅包含操作实际成功产生或移动的实例，并保持产生顺序。对集合执行
操作时按该顺序逐个执行；对空目标或 `none` 执行操作不产生事实。

同名输出采用最近生产者规则。每个 `Summon` 覆盖当前帧中的 `summoned`，每个
`Draw` 覆盖 `drawn`，每个选择节点覆盖 `target`。覆盖不是修改旧 `Binding`：编译器
创建新 ID，并把后续名称引用解析到新 ID，因此数据流静态可见。绑定在当前能力或
法术的完整结算结束后销毁；子块继承父块可见绑定，子块新建的绑定在该子块之后
仍可见于同一顺序块。不同触发实例、不同模式分支和不同事件不得共享绑定。

事件触发建立新帧，并按事件类型预置绑定。例如 `follower_summoned` 预置
`summoned: entity_set`，`card_drawn` 预置 `drawn: entity_set`，`amulet_engaged`
预置 `engaged: entity_set`。无法在所有控制流路径上确定存在且类型一致的绑定引用
属于编译错误。`extends evolve` 的附加块与被扩展块使用同一帧，因此可以读取普通
进化块最后产生的 `summoned` 或 `drawn`。

`attack`、`clash` 的新帧预置 `opponent`，使用普通 `BindingRef`，基数为零或一。
它绑定本次攻击的另一随从实例，防守方交战能力中的值是攻击方；攻击主战者时为空。
该引用不产生目标选择请求，也不会重新查询或替换为后来进入同一槽位的实例。

## 能力、触发与移动模式

```text
Ability = {
  id: NodeId,
  trigger: Trigger,
  condition: BoolExpr?,
  body: [EffectNode],
  origin: Origin
}

FusionAbility = {
  id: NodeId,
  materialFilter: MaterialFilter,
  body: [EffectNode],
  origin: Origin
}

MaterialFilter = {
  kind: "material_filter",
  source: SetExpr,
  predicate: BoolExpr?,
  excludeSource: true,
  minimum: 1,
  maximum: u16?,
  origin: Origin
}

ExecutionPlan = {
  action: "evolve" | "superevolve",
  steps: [{ abilityId: NodeId, frame: "new" | "continue" }]
}

Trigger =
  PlayTrigger          { kind: "play" }
| FanfareTrigger       { kind: "fanfare" }
| LastwordsTrigger     { kind: "lastwords" }
| AttackTrigger        { kind: "attack" }
| EvolveTrigger        { kind: "evolve" }
| SuperEvolveTrigger   { kind: "superevolve" }
| EngageTrigger        { kind: "engage", cost: u16 }
| EnhanceTrigger       { kind: "enhance", cost: u16 }
| SpellboostTrigger    { kind: "spellboost" }
| EventTrigger         { kind: "event", pattern: EventPattern, condition?: Condition }
| ReplacementTrigger   { kind: "replacement", pattern: MovePattern }
```

`fusion material from own.hand where P` 编译为 `FusionAbility`，其中
`MaterialFilter.source` 固定为己方手牌，`predicate` 是 `P`，来源实例始终排除。
`minimum=1` 表示一次融合必须选择至少一个材料；`maximum` 为空表示上限为当次请求
中的合法候选数。材料选择属于融合玩家命令，不是能力 `body` 内的 `Choose` 或
`Require` 节点；全部材料附着后，`body` 只执行一次。
每个实例每回合只能成功融合一次；次数在材料响应验证成功后消耗，非法响应不消耗。
变身会重置次数，手牌返回牌组不重置。合法动作枚举与执行使用同一入口选择规则：
按声明顺序取首个候选数满足 `minimum` 的能力，不把多个声明的材料自动合并。

`enhance N` 是打出卡牌时的强制替代费用能力。存在多个可支付档位时，合法性检查
选择费用最高的一档；存在可支付档位时，不能按原费用或更低档位打出。IR 中同一卡
牌的所有费用不超过所选档位的 `EnhanceTrigger` 都生效，并参与必选目标预检；
运行时不得把档位暴露成玩家模式选择。能力保留声明顺序，费用只决定是否生效。
零费档位与未发动强化必须分别表示，不能使用当前卡牌费用推断生效档位。
`engage N` 是带费用的启动能力，同一护符实体每个控制者回合最多成功启动一次。
`spellboost { ... }` 监听该手牌实例的魔力增幅。使用法术自动对控制者剩余手牌触发一次，
编译器不向每张法术的 `PlayTrigger` 插入重复增幅节点。运行时在法术离开手牌后确定
这一次增幅的实例集合并将能力入队，完成法术效果后再执行队列。队列绑定实例身份，
不会因该实例随后从手牌返回牌组而取消。暂停和恢复必须保存这项调度状态，不能再次
执行自动增幅。`fanfare` 只由正常打出并进入
战场触发；`reanimate` 创建的新实例不触发它。法术 `effect` 中不属于能力声明的
最外层节点编译到 `PlayTrigger`，在支付费用并完成必选目标预检后顺序执行。
`actionPlans` 只描述手动进化动作需要执行的能力；`frame=continue` 表示沿用上一步的
绑定帧，其他触发直接按 `Trigger` 入队。

```text
EventPattern = {
  event: EventKind,
  side: Side?,
  sourceZone: ("field" | "hand")?,
  oncePerTurn: ("own" | "oppo" | "any")?,
  subjectType: CardType?,
  zone: Zone?,
  predicate: BoolExpr?
}

EventKind = "follower_summoned" | "card_drawn" | "amulet_engaged" |
            "turn_started" | "turn_ended" | "attacked" | "damaged" |
            "healed" | "destroyed" | "banished" | "zone_moved" |
            "evolved" | "super_evolved" | "resource_changed" |
            "spellboosted" | "card_discarded" | "card_fused" | "follower_left"

MovePattern = {
  subject: ValueRef,
  from: Zone?,
  to: Zone?,
  phase: "before",
  predicate: BoolExpr?
}
```

`when own follower summoned where trait pixie` 对应 `event=follower_summoned`、
`side=own`、`subjectType=follower` 与 `HasTrait`。`when own turn ends` 对应
`turn_ended`。`replace self leaving field` 对应 `subject=self`、`from=field`、`to`
为空；空 `to` 表示任意离场目的地。

`while self in hand` 编译为 `EventTrigger.sourceZone=hand`；省略时默认为 `field`。
索引覆盖这两个来源区域，按回合玩家优先、战场在手牌之前、区域内顺序与声明顺序派发。
来源离开区域或变身时移除旧索引并取消待发动的事件能力。
`follower_left` 在状态重置前匹配事件对象并保存 `left` 绑定，包含离场前卡牌身份、
`from=field` 与目的区域；`card_fused` 匹配一次成功融合，使用融合前的来源卡筛选，
本体融合能力执行完毕后才排空外部监听队列。

`once per own turn`、`once per oppo turn` 与 `once per turn` 分别编译为
`EventTrigger.oncePerTurn=own|oppo|any`，省略表示不限次数。范围相对于来源持有者；
运行时按实例 ID 与印刷能力 ID 记录已用次数，在筛选通过且成功入队后标记，避免同一效果
连续产生事件时重复占用次数。换回合时先清空记录，再处理倒数与回合开始触发。
状态沙箱和 Continuation 复制记录；新卡牌副本不继承发动历史，进化保留记录，区域重置和变身清空。
解码器拒绝未知范围、自身专用触发及附加能力上的限次字段。

事件头部的 `if` 编译为可选的 `EventTrigger.condition`，使用与效果体相同的
`Condition` 节点。它在事件筛选通过后、消耗限次次数和入队之前求值；条件为假则不入队。
同一事件的其他能力执行后不重新判断，Continuation 恢复也只继续已有队列。
头部不能引用融合材料或事件绑定；自身专用事件和附加能力不接受此字段。
自身计数器必须已声明。缺失字段表示无条件，显式 `null` 或非法条件拒绝解码。
`hand_count` 与 `earthsigils` 分别读取手牌张数与土之印总层数，只能读取，不能作为资源修改目标。

替换能力在原移动提交前执行，并取消原移动。替换执行上下文记录已应用的
`ReplacementTrigger.id` 集合；同一移动因替换块产生后续移动时，不得再次应用同一
替换能力。多个匹配替换能力的优先级由规则集的 `OrderingPolicy` 决定，不能按
`NodeId` 猜测。替换造成的实际移动会产生正常事件事实。

## EffectNode 判别联合

所有节点共有以下字段：

```text
NodeBase = { id: NodeId, origin: Origin }
```

`amount` 为非负 `IntExpr`，除非节点说明允许负值。目标可为 `ValueRef` 或
`SetExpr` 的字段统一命名为 `target`；对集合逐个执行且不事务回滚。

### 选择与控制流

```text
Choose = NodeBase & {
  kind: "choose",
  policy: "optional",
  source: SetExpr,
  binding: Binding,
  count: UInt16?, // 1..65535; omitted means 1
  extremum?: SelectionExtremum
}

Require = NodeBase & {
  kind: "require",
  policy: "required",
  source: SetExpr,
  binding: Binding,
  count: UInt16?,
  extremum?: SelectionExtremum
}

RandomChoose = NodeBase & {
  kind: "random_choose",
  policy: "random",
  source: SetExpr,
  binding: Binding,
  count: UInt16?,
  extremum?: SelectionExtremum
}

SelectionExtremum = { direction: "highest" | "lowest", field: "attack" | "life" | "cost" }

If = NodeBase & {
  kind: "if",
  condition: BoolExpr,
  then: [EffectNode],
  else: [EffectNode]
}

Mode = NodeBase & {
  kind: "mode",
  options: [{ id: OptionId, body: [EffectNode], origin: Origin,
              labels?: { [LocaleId]: non-empty string } }]
}

Repeat = NodeBase & {
  kind: "repeat",
  times: EffectAmount,
  body: [EffectNode]
}

PayResource = NodeBase & {
  kind: "pay_resource",
  resource: "earthsigil" | "shadows",
  amount: IntExpr,
  onPaid: [EffectNode]
}
```

`choose` 候选为空时输出 `none` 且不产生 `ChoiceRequest`；非空时请求玩家选择
`min(count, 候选数)` 个不同实例，请求的最小与最大数量相同。
`require` 在动作预检阶段求候选集合，候选不足 `count` 使整个动作非法且状态、事件序号
和 RNG 均不改变；足够时产生数量为 `count` 的必选请求。
`extremum` 在来源集合筛选以及玩家目标限制之后计算；随机选择不应用玩家目标限制。
只保留当前数值等于极值的候选，维持原候选顺序；`attack`、`life` 排除非随从，
`cost` 使用不小于零的当前费用。它不表示取前 N 名，`count` 仅作用于极值并列集合。
候选查询预算不足时不返回部分结果；续局恢复用同一节点重新验证保存的候选。

`random_choose` 从当前候选不放回抽样，每个选中实例消费一次随机决策，最多选取
`count` 个。选择绑定保留候选顺序，不因随机抽取顺序或客户端提交顺序而变化。
`earthrite N` 和 `necromancy N` 分别编译为 `PayResource`；资源不足时不支付并跳过
`onPaid`，支付成功后不因后续失败退还。`Mode.options.id` 必须在能力内唯一且稳定，
模式是否可执行不取决于其块内资源支付能否成功。

`Repeat.times` 在进入块时读取一次，计算结果小于或等于零时跳过。块体不展开复制，
每次迭代使用独立绑定帧；子块继承当前迭代绑定，迭代结束后丢弃局部绑定。
块内生成的触发能力在整个重复块结束后处理，但每个效果立即更新实体和死亡状态。
嵌套次数、每次迭代的指令开销与查询均受执行预算约束，暂停后保存剩余次数。

### 卡牌与区域操作

```text
Draw = NodeBase & {
  kind: "draw",
  owner: Side,
  count: IntExpr?,
  all: bool,
  sourceZone: "deck",
  predicate: BoolExpr?,
  output: Binding
}

AddCard = NodeBase & {
  kind: "add_card",
  owner: Side,
  cardId: CardId,
  count: IntExpr,
  destination: "hand"
}

Summon = NodeBase & {
  kind: "summon",
  owner: Side,
  cardId: CardId,
  count: IntExpr,
  invokeFanfare: false,
  output: Binding
}

SummonCopies = NodeBase & {
  kind: "summon_copies",
  owner: Side,
  target: ValueRef | SetExpr,
  output: "summoned"
}

Destroy = NodeBase & {
  kind: "destroy",
  target: ValueRef | SetExpr,
  predicate?: Predicate,
  output?: "destroyed"
}

Banish = NodeBase & {
  kind: "banish",
  target: ValueRef | SetExpr
}

Discard = NodeBase & {
  kind: "discard",
  target: ValueRef | SetExpr,
  predicate?: Predicate
}

Transform = NodeBase & {
  kind: "transform",
  target: ValueRef | SetExpr,
  cardId: CardId,
  preserveInstanceId: true,
  preserveMaterials: true
}

Return = NodeBase & {
  kind: "return",
  target: ValueRef | SetExpr,
  destination: "hand" | "deck",
  deckInsertion: "uniform_random_position"?
}

Reanimate = NodeBase & {
  kind: "reanimate",
  owner: Side,
  maxCost: IntExpr,
  tieBreak: "random",
  invokeFanfare: false,
  output: Binding
}
```

`SummonCopies` 按输入集合顺序为手牌或战场中的随从、护符各创建一个独立副本，
保留原卡；忽略法术和其他区域对象，战场满时停止。继承当前卡牌状态、附加效果的
原有期限和独立的融合材料记录，重置已攻击次数与已启动标记，重新施加入场攻击限制。
不发动入场曲或进化能力；输出只包含成功召唤的实例。无需新增续局状态字段。

`Reanimate` 按破坏记录加权：原始费用满足上限且最高的每条记录均可被抽中，
同名卡的多条记录不去重。它创建原始状态的同名随从并在入场前添加亡者类型，
不继承旧实例的增益、费用变化、进化状态或额外关键词，也不触发入场曲。
亡者类型保存在运行时实例和恢复快照中，对外通过实体视图的 `traits` 返回；
卡牌定义的原始种族列表保持不变。

普通 `draw N` 使用牌组顶部且无筛选；`draw N from deck where P` 等概率、不放回地
抽取最多 N 个匹配实例，按抽中顺序处理，每张消费一次 RNG 决策。同名副本不合并为
单个候选；未抽中的实例保持牌组顺序。`draw all` 按牌组顺序取所有匹配项，不消费 RNG。
候选查询、候选预算和事件预算校验发生在抽选之前；失败时不消费 RNG 或移动卡牌。
筛选抽牌也发出公开的 `card_drawn` 数量事件，不附带身份；无候选时数量为零。
仅普通定量抽牌不足时产生牌组耗尽败北，筛选不足不产生败北。手牌或战场空间不足
时只把成功实例写入输出。`AddCard` 创建新实例；`Return` 移动既有实例。
`return ... to deck` 使用 `deckInsertion=uniform_random_position`：在牌组长度为
`n` 时从 `0..n` 共 `n+1` 个插入位置中等概率选择一个，不改变其他卡的相对顺序，
并消费一次规则层随机决策。返回手牌时 `deckInsertion` 为空且不消费随机数。
从战场返回手牌或牌组时，恢复当前卡牌定义的原始费用、身材、固有能力与初始状态，
清除伤害、进化和附加效果；从手牌返回牌组保留附加效果及已排队的魔力增幅。

`Destroy` 产生破坏事实、移入墓场并记录独立的破坏历史，随后排队谢幕曲；同批死亡
先形成一个批次。`Banish` 不产生破坏或谢幕曲。`Reanimate` 从已破坏随从历史选择
费用不超过上限且费用最高者，唯一最高者不消费 RNG；并列时才随机选择。它创建
全新实例，旧历史项不被移除。

历史记录与存续实例分开保存，包含破坏时的卡牌 ID、费用、攻击、生命、进化形态与亡者类型。
同一实例之后返回、变身或再度被破坏，不会改写旧记录。`destroyed` 区域引用仅用于只读
历史统计；解码器拒绝将它或其筛选集合用于选择和卡牌修改操作。`Destroy.output` 仍是
实际被破坏实例的绑定，不等同于历史记录。破坏事件的 `subject.cardId` 固定为破坏时身份。

`Transform` 将目标实例采用的卡牌定义改为 `cardId`，不分配新 `InstanceId`，并
保留目标上完整的材料序列。它不是区域移动，不触发离场、入场、破坏、消失或谢幕曲。
变身恢复新卡牌定义的原始费用、身材、固有能力及初始状态，不继承伤害、附加效果、
进化、攻击次数或启动状态。战场上的变身结果若为随从，重新进入入场等待状态。
当前源语法只允许融合能力块中的 `self` 作为目标。

```text
AttachedMaterial = {
  instanceId: InstanceId,
  cardId: CardId,
  state: bytes
}
```

运行时实体必须保存有序的 `materials: [AttachedMaterial]`。其中 `state` 是该 IR
版本规定的规范实例状态编码。材料实例从手牌附着后
不进入任何 `Zone`，但保留自身 `InstanceId` 和可序列化状态，以便回放和后续规则
检查；附着本身不产生 `zone_moved`、`destroyed` 或 `banished` 事实，也不增加墓场
计数。变身重置来源实例的卡牌状态，但不能清空、复制或重新创建材料。

### 数值、能力与状态操作

```text
HistorySet = { kind: "history", side: "own" | "oppo", window: "this_turn",
               member?: "card" | "follower" | "amulet" }
NumericExpr = Count { kind: "count", source: ZoneSet | HistorySet | BindingRef | FilterSet }
            | Sum { kind: "sum", source: ZoneSet | HistorySet | BindingRef | FilterSet,
                    field: "base_attack" | "base_life" | "base_cost" }
            | PlayerScalar { kind: "scalar", side: "own" | "oppo",
                             field: "combo" | "pp" | "maxpp" | "life" | "ep" | "sep" | "shadows" | "hand_count" | "earthsigils" }
            | SelfScalar { kind: "self_scalar", field: "attack" | "life" | "cost" }
            | SelfCounter { kind: "self_counter", field: CounterName }
EffectAmount = nonnegative_integer | NumericExpr
AdjustCounter = NodeBase & { kind: "adjust_counter", field: CounterName, delta: nonnegative_i32 }
StatDelta = i16 | NumericExpr | Negate { kind: "negate", value: NumericExpr }

Damage = NodeBase & {
  kind: "damage",
  target: ValueRef | SetExpr,
  amount: EffectAmount,
  damageType: "effect",
  distribution?: "field_entry_order",
  overflow?: LeaderRef,
  predicate?: Predicate
}

Heal = NodeBase & {
  kind: "heal",
  target: ValueRef | SetExpr,
  amount: EffectAmount,
  predicate?: Predicate
}

BuffStats = NodeBase & {
  kind: "buff_stats",
  target: ValueRef | SetExpr,
  attackDelta: StatDelta,
  lifeDelta: StatDelta,
  predicate?: Predicate,
  until?: "turn_end" | "own_turn_end" | "oppo_turn_end"
}

AddKeyword = NodeBase & {
  kind: "add_keyword",
  target: ValueRef | SetExpr,
  keyword: Keyword,
  predicate?: Predicate,
  until?: "turn_end" | "own_turn_end" | "oppo_turn_end"
}

RemoveKeyword = NodeBase & {
  kind: "remove_keyword",
  target: ValueRef | SetExpr,
  keyword: Keyword,
  predicate?: Predicate
}

SetAttackLimit = NodeBase & {
  kind: "set_attack_limit",
  target: ValueRef,
  amount: u16
}

SetLife = NodeBase & {
  kind: "set_life",
  target: ValueRef | SetExpr,
  amount: EffectAmount
}

SilentEvolve = NodeBase & {
  kind: "silent_evolve",
  target: ValueRef | SetExpr,
  form: "evolved" | "super_evolved"
}

AdjustResource = NodeBase & {
  kind: "adjust_resource",
  owner: Side,
  resource: "combo" | "maxpp" | "shadows",
  delta: i32,
  clamp: { minimum: i32?, maximum: i32? }
}

RestoreResource = NodeBase & {
  kind: "restore_resource",
  owner: Side,
  resource: "pp"
}

AdjustEarthSigil = NodeBase & {
  kind: "adjust_earthsigil",
  owner: Side,
  delta: i32
}

AdjustEntityField = NodeBase & {
  kind: "adjust_entity_field",
  target: ValueRef | SetExpr,
  field: "cost" | "countdown",
  delta: i32,
  minimum: i32?,
  maximum: i32?
}

Spellboost = NodeBase & {
  kind: "spellboost",
  target: SetExpr,
  times: IntExpr
}
```

`Destroy.output` 由编译器写为 `destroyed`。运行器按死亡批次顺序记录本条操作实际
破坏的目标，不记录受保护目标、重复目标或同批其他死亡；空结果覆盖此前绑定。
旧 IR 可省略输出，此时只执行破坏。`Count.source` 可引用区域集合或当前帧的集合绑定，
`FilterSet` 的计数来源也可为这两者；绑定定义与作用域由源语言校验。
选择暂停的存档通过既有帧绑定保存这些实例 ID，不增加独立计数器。

`Damage.distribution` 省略时，对每个目标造成完整的 `amount` 伤害。
`SetLife` 对随从直接设置当前生命值，不使用伤害管线、不消费屏障、不产生伤害或回复事件。
数值在修改任何目标前确定；零生命值的战场随从随后统一进入死亡批次。
此节点拒绝主战者引用，不携带伤害类型、过滤器、增益量或输出绑定；非随从实例不受影响。
`AddKeyword` 与 `RemoveKeyword` 的 `predicate` 在修改任何目标前应用；`other` 编译为
以 `self` 为排除值的 `exclude` 引用。它们不创建玩家选择，也不应用敌方选择限制。
`AddKeyword` 和 `BuffStats` 支持 `until`。它在执行时将相对期限解析为绝对玩家，记录临时赋予；
省略表示永久。到期只撤销临时赋予，不删除原生或后来永久获得的同名能力。
不同玩家的到期期限独立保存，同一截止回合的重复赋予合并；显式移除或消耗会清除
该能力的全部赋予记录。其他效果携带 `until` 时拒绝解码。
`BuffStats` 按绝对玩家合计已经求值的攻击力、生命值增量；到期撤销相应增量，
保留之后的永久修改和伤害。`SetLife` 清除先前临时增益中的生命值部分，保留攻击力部分。
所有到期属性一起更新，再统一检查死亡并处理谢幕曲，完成后才进入下一回合。
数值表达式在该效果实际执行时读取，目标修改前同时确定伤害量或两项增益量。
`self_scalar` 使用能力来源实例的当前数值，攻击力和生命值仅适用于随从；`scalar` 的玩家
相对能力控制者解析。增益保留负数，伤害和回复的动态数值以零为下限。计数查询超出预算时
不执行该数值操作。数值引用本身不消耗随机决策。
值为 `field_entry_order` 时，`target` 必须为单侧 `field` 中的 `follower` 区域集合，
`amount` 表示本次分配总额。可选 `overflow` 必须引用同侧主战者，且仅能与
`distribution` 一起出现。筛选后按原出场顺序分配，屏障和减伤不退还额度。
未指定主战者时余量并入最后一个随从的单次伤害；指定主战者时余量交给主战者。
所有分配额度先确定，再造成伤害并统一处理死亡；存档恢复后继续使用区域槽位顺序。

`buff self +1/+1` 的两个增量允许为负。`gain own.maxpp 1` 编译为
`AdjustResource(maxpp, +1, maximum=10)`；`add combo 1` 编译为 `AdjustResource`。
`restore own.pp` 编译为 `RestoreResource(owner=own, resource=pp)`，执行时增加
`max(0, maxpp - pp)`，保留能量上限和超出上限的额外能量。事件 `pp_restored`
的 `Actual` 记录实际回复量，包括零；该节点不携带增量、目标或次数字段。
`add 1 earthsigil` 编译为 `AdjustEarthSigil`，操作当前控制者战场上的土之印实体。
若没有土之印，则召唤 90031210 并设置层数；满场时不创建实例。正数增量隐式依赖
该衍生物，源文件引用检查与 IR 解码都拒绝缺失依赖的可执行卡包。零增量无此依赖。
`reduce countdown self 2` 编译为
`AdjustEntityField(delta=-2, minimum=0)`；`reduce cost self 1 minimum 0` 显式保留
下限。到达上限或下限后按最终截断值继续结算。

`evolve target silent` 与 `superevolve target silent` 只作用于场上的未进化随从，分别增加
+2/+2、+3/+3 并改变形态，不支付 EP/SEP、不占用手动次数，也不执行目标卡面的进化关键词能力。
普通形态变化发出 `evolved`，超进化依次发出 `evolved`、`super_evolved`；重复进化不再增加身材或发出事件。
手动普通进化与超进化属于玩家动作，不表示为此节点；超进化默认执行规范化后的普通进化能力，
再执行超进化附加能力。没有独立超进化块时，编译器仍为普通进化块生成超进化 ActionPlan。

### 当前语法到节点的完整映射

`GrantAbility = NodeBase & { kind: "grant_ability", target: ValueRef | SetExpr,
ability: Ability, labels?: map<LocaleId, string> }` 附加一个独立触发能力。
`ability` 必须为 `independent` 的谢幕曲或回合开始/结束事件，能力体不得含 `require`。
编译器为能力体建立新绑定作用域，随从自身数值按受赋予者求值；不捕获施法者局部状态。
实例按顺序保存赋予节点的 ID，重复 ID 表示多次赋予，恢复时必须在相同卡包内找到节点。
执行栈以 `node:<grant-id>:granted` 引用能力体，支持其中的选择、模式及重复块暂停恢复。

| WBO 构造 | IR |
|---|---|
| `choose target from S` | `Choose` |
| `require target from S` | `Require` |
| `random target from S` | `RandomChoose` |
| `if C { A } else { B }` | `If` |
| `mode`、`option N` | `Mode` |
| `earthrite N`、`necromancy N` | `PayResource` |
| `draw N`、`draw all from deck where P` | `Draw` |
| `add N card C to hand` | `AddCard` |
| `summon N card C` | `Summon` |
| `summon copies of S` | `SummonCopies` |
| `grant T { 能力 }` | `GrantAbility` |
| `damage T N`、`heal T N` | `Damage`、`Heal` |
| `set life T N` | `SetLife` |
| `buff T +A/+L [where P] [until [own/oppo] turn ends]` | `BuffStats`，可带 `predicate` 和 `until` |
| `destroy T`、`banish T`、`discard T` | `Destroy`、`Banish`、`Discard` |
| `transform T into card C [preserving materials] [where P]` | `Transform`，筛选编译为 `FilterRef` |
| `return T to hand/deck` | `Return` |
| `add K to T`、`remove K from T` | `AddKeyword`、`RemoveKeyword` |
| `evolve T silent` | `SilentEvolve` |
| `superevolve T silent` | `SilentEvolve(form=super_evolved)` |
| `reanimate N` | `Reanimate` |
| `gain own.maxpp N`、`add combo N` | `AdjustResource` |
| `restore own.pp` | `RestoreResource` |
| `add N earthsigil` | `AdjustEarthSigil` |
| `reduce countdown T N`、`reduce cost T N minimum M` | `AdjustEntityField` |
| `spellboost S N` | `Spellboost` |
| `ward`、`storm`、`rush`、`bane`、`drain`、`intimidate` | `Card.intrinsic` |
| `unplayable` | `Card.restrictions` 中的 `Unplayable` |
| `fusion material from S where P { ... }` | `FusionAbility(MaterialFilter)` |
| `countdown N`、`earthsigil` | `IntrinsicState` |
| `fanfare`、`lastwords`、`attack`、`clash`、`evolve`、`superevolve` | 对应 `Trigger` |
| `engage N`、`enhance N`、`spellboost {}` | 对应专用 `Trigger` |
| `when ...` | `EventTrigger(EventPattern)` |
| `replace ...` | `ReplacementTrigger(MovePattern)` |

## replaces 与 extends 的编译期展开

AST 暂存下列声明形式：

```text
DeclaredAbility = {
  trigger: Trigger,
  relation: "independent" | "replaces" | "extends",
  relatedTrigger: Trigger?,
  body: ASTBlock,
  origin: Origin
}
```

规范化按卡牌内声明顺序执行：

1. 普通 `evolve` 编译为基础块 `E`。
2. 独立 `superevolve` 编译为块 `S`。普通进化计划为 `E(new)`；手动超进化计划为
   `E(new), S(new)`，独立能力不继承普通进化块的输出绑定。
3. `superevolve replaces evolve` 编译为超进化块 `R`，手动超进化计划只含 `R`；
   普通进化仍使用 `E`。
4. `superevolve extends evolve` 编译为扩展块 `X`，超进化计划为
   `E(new), X(continue)`，两段共享绑定帧；普通进化计划仍只含 `E(new)`。
5. 缺失被引用能力、重复替换、关系目标不是 `evolve` 或形成环都属于编译错误。

节点不因展开复制生成第二套身份。`ExecutionPlan` 保存能力 ID 和绑定帧关系，并为
关系能力增加 `ExpansionFrame`：`extends` 帧指向扩展声明，`replaces` 帧指向替换
声明。这样运行时事实既能指出实际执行节点，也能解释它为何出现在超进化计划中。
包中不保留需要运行时判断的 `relation`。

## 玩家选择协议

```text
PlayerCommand =
  PlayCommand        { kind: "play", commandId: CommandId, actor: Side,
                       source: InstanceId }
| FusionCommand      { kind: "fusion", commandId: CommandId, actor: Side,
                       source: InstanceId }
| EngageCommand      { kind: "engage", commandId: CommandId, actor: Side,
                       source: InstanceId }
| EvolveCommand      { kind: "evolve", commandId: CommandId, actor: Side,
                       source: InstanceId, super: bool }
| AttackCommand      { kind: "attack", commandId: CommandId, actor: Side,
                       source: InstanceId, target: InstanceId | Side }
| EndTurnCommand     { kind: "end_turn", commandId: CommandId, actor: Side }

ChoiceRequest = {
  requestId: RequestId,
  actionId: CommandId,
  nodeId: NodeId,
  kind: "target" | "mode" | "fusion_material",
  minSelections: u16,
  maxSelections: u16,
  candidates: [ChoiceCandidate],
  stateRevision: u64,
  publicTo: Side
}

ChoiceCandidate =
  EntityCandidate { kind: "entity", instanceId: InstanceId }
| LeaderCandidate { kind: "leader", leaderSide: Side }
| OptionCandidate { kind: "option", optionId: OptionId,
                    labels?: { [LocaleId]: non-empty string } }

ChoiceResponse = {
  requestId: RequestId,
  actionId: CommandId,
  stateRevision: u64,
  selectedInstanceIds: [InstanceId]?,
  selectedLeaderSides: [Side]?,
  selectedOptionId: OptionId?
}
```

`FusionCommand.source` 必须是行动方手牌中具有 `FusionAbility` 的实例；
`Unplayable` 不会使该命令非法。命令完成前，材料请求及其响应与同一个
`CommandId` 关联，不能拆成多条命令，也不能用多个单选响应累积材料。

`leaderSide` 与 `selectedLeaderSides` 是对局固定席位，不相对请求接收者转换；
客户端以 `state.viewer` 判断“你的主战者”和“对手主战者”。目标响应合计实例与
主战者数量，拒绝重复或候选外的值，再按原候选顺序保存。模式与融合响应禁止
携带主战者。候选列表和选中绑定仅对选择方可见，录像遵循相同的可见范围。

模式候选的 `labels` 来自当前执行的 `ModeOption`，与卡牌当前区域或变身后的身份无关。
其语言键只允许 `chs/eng/jpn/kor/cht`，缺省兼容无标签卡牌；不增加可执行节点。
候选和标签只发给选择所属玩家，导出的请求与存档使用独立副本。Continuation 恢复时
逐项校验编号、执行块及全部标签，拒绝替换、删除或添加标签的存档。

候选顺序由 `SetExpr` 的规范顺序决定。服务端不得接受候选列表之外的值，也不得接受
旧 `stateRevision` 的响应。`requestId` 由动作 ID、节点 ID、该动作内请求序号确定性
生成，不使用 RNG。执行暂停点属于对局状态，不写入卡牌 IR；服务器与 WASM 使用
相同的请求和响应结构恢复执行。

融合命令只产生一个 `kind=fusion_material` 的命令级请求。其 `nodeId` 是
`FusionAbility.id`，`minSelections=1`，`maxSelections` 等于候选数；候选为过滤后的
己方手牌实例并排除来源。`selectedInstanceIds` 必须含一个或多个互不相同的候选，
响应顺序就是材料附着顺序。请求验证通过后，所有材料作为一次状态提交附着到来源，
再执行融合能力块一次；验证失败不得移除手牌、附着材料、生成事实或部分结算。

## 确定性随机数

默认规则集 `wbo-standard-0.3.0` 固定使用 SplitMix64 v1。卡牌 IR 只声明何时
需要随机决策；对局、测试包、服务器和 WASM 必须持久化并校验规则集内容哈希。

```text
MatchRuleset = {
  id: string,
  version: SemVer,
  contentHash: ContentHash,
  rngAlgorithm: AlgorithmRef,
  orderingPolicy: AlgorithmRef
}

AlgorithmRef = {
  id: string,
  version: SemVer,
  contentHash: ContentHash
}
```

默认排序策略先处理当前行动方再处理对方；每一方内部按区域中的实例顺序，再按
卡面能力声明顺序。双方集合拼接、多个替换能力、同时触发能力和死亡批次谢幕曲均
使用这套稳定顺序。任何对局、回放或可执行测试包都必须锁定完整规则集哈希。

```text
RngSpec = {
  algorithm: string,
  algorithmVersion: SemVer,
  seed: Seed,
  opaqueState: bytes,
  decisionsConsumed: u64
}
```

SplitMix64 以场景 `u64` 种子作为初始状态，每次规则决策推进一次并将输出对候选数
取模；此有界映射是 v1 规则的一部分。算法或映射规则变化必须提升算法版本。
`decisionsConsumed` 统计规则层已经完成的
随机决策，不统计算法内部读取的随机字数量：一次随机目标、一次并列打破或一次
随机插入各计为一。候选为空、非法动作、唯一最高费用亡者召还及无需随机打破的
确定性路径不得开始随机决策。合法性预检在 RNG 快照上运行且不得提交状态。

`opaqueState` 只由同算法及版本的实现解释，但必须稳定序列化。RNG 状态、事件队列、
实例 ID 分配计数器均属于可序列化对局状态。新实例 ID 由对局 ID、创建事实序号和
卡牌 ID 确定性生成，不读取 RNG。

## 事件事实

事件事实是规则层不可变记录，用于触发匹配、重放校验和测试断言，不是 UI 文本。

当前运行器的 `card_fused` 记录来源实例、融合前卡牌 ID 及本次材料数；
`card_transformed` 记录同一实例变身前后的卡牌 ID。它们是操作记录，不触发入场曲。
卡牌 ID 在事件发生时固定，后续变身不改写旧记录。手牌、牌组与附着材料区域中的
这些事件使用 `privateTo` 指定持有者。客户端事件视图保留事件种类、所属方、数量与
序号，但向另一方隐藏实例编号和卡牌身份；过滤不会删除事件或改变录像帧边界。

```text
EventFact = {
  sequence: u64,
  batchId: u64?,
  kind: EventKind,
  sourceNodeId: NodeId?,
  sourceInstanceId: InstanceId?,
  controller: Side?,
  subjects: [InstanceId],
  payload: EventPayload
}

EventPayload =
  ZoneMovedPayload { kind: "zone_moved", from: Zone, to: Zone, reason: MoveReason }
| AmountPayload    { kind: "amount", requested: i32, actual: i32 }
| StatsPayload     { kind: "stats", attackDelta: i16, lifeDelta: i16 }
| KeywordPayload   { kind: "keyword", keyword: Keyword, added: bool }
| DrawPayload      { kind: "draw", owner: Side, count: u16 }
| SummonPayload    { kind: "summon", owner: Side, cardId: CardId, count: u16 }
| DestroyPayload   { kind: "destroy" }
| EmptyPayload     { kind: "empty" }

MoveReason = "draw" | "play" | "summon" | "destroy" | "banish" |
             "return" | "earthsigil_merge" | "rule"
```

每个事实使用全局递增 `sequence`。批量伤害或同时死亡共享 `batchId`；先收集死亡
批次，再按规则集 `OrderingPolicy` 指定的顺序把谢幕曲加入队列。`destroy` 至少
产生 `destroyed` 和对应 `zone_moved` 事实；`banish` 只产生 `banished` 与移动事实。
土之印合并使用 `earthsigil_merge`，旧实例移入 `banished`，不产生 `destroyed`。

测试中的 `heal own.leader 4`、`draw own 1`、`destroy source`、
`summon card C count N` 编译为 `EventMatcher`，只匹配结构化字段。`contains ordered`
是事件流的有序子序列，`excludes` 要求无匹配项，`exact` 要求过滤前的完整规则事实
序列完全相同。

## CardPack

```text
CardPack = {
  format: "wbos",
  containerVersion: SemVer,
  encoding: "canonical-json",
  kind: "card-pack",
  irVersion: SemVer,
  schemaHash: ContentHash,
  packId: PackId,
  packVersion: SemVer,
  compilerVersion: SemVer,
  sourceLanguageVersion: SemVer,
  requiredFeatures: [string],
  dependencies: [PackDependency],
  sources: [SourceFile],
  cards: [Card],
  origins: Map<NodeId, Origin>,
  contentHash: ContentHash
}

PackDependency = {
  packId: PackId,
  versionRange: string,
  contentHash: ContentHash,
  requiredCards: [CardId]
}
```

卡牌按 `Card.id` 升序，依赖按 `packId` 升序，来源按规范相对路径升序，映射键按
UTF-8 字节序编码。`contentHash` 是将自身字段置空后，对规范 JSON 编码计算的
SHA-256；规范编码禁止浮点数、重复键和无意义空字段。依赖同时锁定版本范围与内容
哈希，加载器必须拒绝缺失依赖、哈希不符、重复卡牌 ID 和依赖环。

`schemaHash` 标识生成该包的精确 IR 模式。相同 `packId` 与 `packVersion` 不得对应
不同内容哈希；发布后修改内容必须提升 `packVersion`。

`90071210`、`90071220` 开始的变身目标已完整解析至 `90074110`。卡包编译必须继续
拒绝其他未解析引用；开发期显式生成的部分语义产物必须在依赖中列出缺失卡牌 ID。

## Test IR

```text
TestPack = {
  format: "wbos",
  containerVersion: SemVer,
  encoding: "canonical-json",
  kind: "test-pack",
  irVersion: SemVer,
  requiredFeatures: [string],
  ruleset: RulesetDependency,
  cardDependencies: [PackDependency],
  scenarios: [TestScenario],
  contentHash: ContentHash
}

RulesetDependency = {
  id: string,
  version: SemVer,
  contentHash: ContentHash,
  rng: RNGPolicy,
  orderingPolicy: OrderingPolicy,
  executionBudget: {
    instructions: u64,
    queryVisits: u64,
    stackDepth: u64,
    candidates: u64,
    events: u64,
    triggers: u64,
    createdInstances: u64,
    continuationBytes: u64
  }
}

TestScenario = {
  id: NodeId,
  name: string,
  seed: Seed,
  initialState: TestState,
  actions: [TestAction],
  assertions: [Assertion],
  origin: Origin
}

TestState = {
  turn: { active: Side, number: TurnNumber },
  phase: "main" | "turn_start" | "turn_end",
  players: Map<Side, TestPlayer>,
  aliases: Map<string, InstanceId>
}

TestPlayer = {
  leader: { life: i16, maxLife: i16 },
  pp: u16,
  maxpp: u16,
  ep: u16,
  sep: u16,
  combo: u16,
  shadows: u16,
  zones: Map<Zone, [TestInstance]>
}

TestInstance = {
  instanceId: InstanceId,
  alias: string,
  cardId: CardId,
  declaredType: CardType,
  overrides: InstanceOverrides
}

InstanceOverrides = {
  counters?: { CounterName: nonnegative_i32 },
  cost: u16?,
  stats: Stats?, evolved: bool?, superEvolved: bool?,
  keywords: [Keyword]?, countdown: u16?, engaged: bool?, earthsigil: u16?
}
```

`CounterName` 匹配 `[a-z][a-z0-9_]{0,31}`；初始值、覆盖值与增加量的范围是
`0..2147483647`。`self_counter` 同时可作为比较条件的 `left`。
解码器递归校验数值、条件和嵌套效果内的计数器引用，名称必须存在于当前卡牌的 `counters`。
测试断言 `alias.counter.x` 使用 `{ kind: "instance_counter", instanceId: ID, field: "x" }`。
存档实例保存完整计数器表，恢复时要求名称集合与当前卡身份完全一致；所有导出值独立复制。
手牌计数器遵循玩家视角隐藏，公开战场实例和对应录像帧保留当前值。

`.wbotest` 源文件暂不声明规则集；编译 TestPack 时必须由命令行、项目清单或调用方
显式提供，编译器随后把精确依赖写入 `ruleset`。缺少规则集时可以执行纯语法检查，
但不得生成可执行 TestPack 或声称测试结果可跨实现重现。

缺省状态在编译 Test IR 时物化：主战者 `20/20`，所有资源为零，区域为空，行动方
为 `own`，阶段为 `main`。牌组按顶部到牌底保存。别名必须在场景内唯一，实例移动
后 `InstanceId` 与别名不变。覆盖值只写入测试初始实例，不修改 `Card`。

```text
TestAction =
  Play         { kind: "play", actor: Side, source: InstanceId }
| Engage       { kind: "engage", actor: Side, source: InstanceId }
| Evolve       { kind: "evolve", actor: Side, source: InstanceId }
| SuperEvolve  { kind: "superevolve", actor: Side, source: InstanceId }
| AttackEntity { kind: "attack_entity", actor: Side, attacker: InstanceId,
                 defender: InstanceId }
| AttackLeader { kind: "attack_leader", actor: Side, attacker: InstanceId,
                 defender: Side }
| EndTurn      { kind: "end_turn", actor: Side }
| Select       { kind: "select", target: InstanceId }
| SelectMany   { kind: "select", targets: [InstanceId] }
| SelectMixed  { kind: "select", target?: InstanceId, targets?: [InstanceId], leaderSides: [Side] }
| SelectMode   { kind: "select_mode", optionId: OptionId }
| Advance      { kind: "advance", timing: "turn_start" | "turn_end", side: Side }
```

`Select`、`SelectMany` 与 `SelectMode` 必须紧跟实际产生的请求。`target` 与 `targets`
互斥，多选列表非空且实例 ID 不能重复；全部引用都必须存在。`Advance` 标记为测试驱动动作，不得
写入正式对局回放。

```text
Assertion =
  Legal             { kind: "legal" }
| Illegal           { kind: "illegal", code: DiagnosticCode }
| Unchanged         { kind: "unchanged" }
| CompareValue      { kind: "compare_value", left: TestValueRef,
                      op: "eq" | "ne" | "lt" | "le" | "gt" | "ge",
                      right: TestLiteral }
| HasKeywordAssert  { kind: "has_keyword", target: InstanceId,
                      keyword: Keyword, expected: bool }
| ZoneCount         { kind: "zone_count", side: Side, zone: Zone,
                      predicate: BoolExpr, op: "eq", count: u32 }
| AllHaveKeyword    { kind: "all_have_keyword", source: SetExpr, keyword: Keyword }
| OrderedInstances  { kind: "ordered_instances", source: SetExpr,
                      expected: [InstanceId], containment: "exact" | "subsequence" }
| EventsAssert      { kind: "events", mode: "contains_ordered" | "excludes" | "exact",
                      expected: [EventMatcher] }
| RngConsumed       { kind: "rng_consumed", op: "eq", count: u64 }

TestValueRef =
  PlayerFieldRef    { kind: "player_field", side: Side,
                      field: "leader.life" | "leader.max_life" | "pp" | "maxpp" |
                             "ep" | "sep" | "combo" | "shadows" }
| PlayerPpPairRef   { kind: "player_pp_pair", side: Side }
| InstanceFieldRef  { kind: "instance_field", instanceId: InstanceId,
                      field: "zone" | "attack" | "life" | "stats" | "evolved" |
                             "super_evolved" | "engaged" | "countdown" |
                             "earthsigil" }

TestLiteral =
  IntegerLiteral { kind: "integer", value: i32 }
| BooleanLiteral { kind: "boolean", value: bool }
| StatsLiteral   { kind: "stats", attack: i16, life: i16 }
| PpLiteral      { kind: "pp", current: u16, maximum: u16 }
| ZoneLiteral    { kind: "zone", value: Zone }

EventMatcher =
  HealMatcher    { kind: "healed", target: TestEventTarget, actual: i32? }
| DamageMatcher  { kind: "damaged", target: TestEventTarget, actual: i32? }
| DrawMatcher    { kind: "card_drawn", side: Side, count: u16? }
| DestroyMatcher { kind: "destroyed", subject: TestEventSubject }
| BanishMatcher  { kind: "banished", subject: TestEventSubject }
| SummonMatcher  { kind: "follower_summoned", side: Side?,
                    instanceId: InstanceId?, cardId: CardId?, count: u16? }
| MoveMatcher    { kind: "zone_moved", instanceId: InstanceId?, from: Zone?, to: Zone? }
| ReturnMatcher  { kind: "zone_moved", reason: "return",
                   subject: TestEventSubject, destination: "hand" | "deck" }
| EvolveMatcher  { kind: "evolved" | "super_evolved", instanceId: InstanceId }
| AttackMatcher  { kind: "attacked", attacker: InstanceId,
                   defender: InstanceId | Side }
| EngageMatcher  { kind: "amulet_engaged", instanceId: InstanceId }
| TurnMatcher    { kind: "turn_started" | "turn_ended", side: Side }
| ResourceMatcher { kind: "resource_changed", side: Side,
                    resource: FieldName, direction: "gain" | "spend", amount: i32 }

TestEventSubject =
  InstanceSubject { kind: "instance", instanceId: InstanceId }
| CardSubject     { kind: "card", cardId: CardId }

TestEventTarget =
  LeaderTarget   { kind: "leader", side: Side }
| InstanceTarget { kind: "instance", instanceId: InstanceId }
| CardTarget     { kind: "card", cardId: CardId }
```

`Unchanged` 比较动作前后的完整可序列化状态，包括 RNG、事实序号、实例计数器和队列。
非法动作不得生成事件或选择请求。`own.pp == 3/10` 编译为 `PlayerPpPairRef` 与
`PpLiteral` 的比较；单独的 `own.pp` 或 `own.maxpp` 使用 `PlayerFieldRef`。实例别名在
编译期解析为 `InstanceId`，不进入运行时断言求值器。事件中的 `count` 匹配同一批次
与同一卡牌的事实数量，而不是 UI 合并日志数量。

`RngConsumed` 比较 `RngSpec.decisionsConsumed` 的动作前后差值，不比较算法内部状态
推进次数或读取的随机字数量。

## JSON 示例：卡牌 10002110

下例省略包级来源表和本地化的其他语言，但节点结构是有效的规范形状。`origin`
中的行列用于说明来源，实际哈希由编译器计算。

```json
{
  "id": 10002110,
  "cardType": "follower",
  "cost": 3,
  "stats": { "attack": 3, "life": 3 },
  "traits": [],
  "intrinsic": [],
  "intrinsicState": [],
  "abilities": [
    {
      "id": "6d50bff8290cfb3c3e50356d77552701",
      "trigger": { "kind": "evolve" },
      "body": [
        {
          "id": "218fa67ef5b2f27f57851dd223f9c179",
          "kind": "heal",
          "target": { "kind": "leader", "side": "own", "valueType": "leader" },
          "amount": { "kind": "literal", "value": 2 },
          "origin": {
            "primary": {
              "sourceId": "b843d24770b14fca8b4de092f890f44b",
              "startByte": 115,
              "endByte": 133,
              "startLine": 10,
              "startColumn": 13,
              "endLine": 10,
              "endColumn": 31
            },
            "expansion": [{ "kind": "declared", "span": {
              "sourceId": "b843d24770b14fca8b4de092f890f44b",
              "startByte": 115,
              "endByte": 133,
              "startLine": 10,
              "startColumn": 13,
              "endLine": 10,
              "endColumn": 31
            }}]
          }
        }
      ],
      "origin": {
        "primary": {
          "sourceId": "b843d24770b14fca8b4de092f890f44b",
          "startByte": 88,
          "endByte": 143,
          "startLine": 9,
          "startColumn": 9,
          "endLine": 11,
          "endColumn": 10
        },
        "expansion": []
      }
    },
    {
      "id": "bb7c6d9a8a76a9a43f670559abc969e8",
      "trigger": { "kind": "superevolve" },
      "body": [
        {
          "id": "317d479010f5a295a2cd9b12cf5e860d",
          "kind": "heal",
          "target": { "kind": "leader", "side": "own", "valueType": "leader" },
          "amount": { "kind": "literal", "value": 4 },
          "origin": {
            "primary": {
              "sourceId": "b843d24770b14fca8b4de092f890f44b",
              "startByte": 191,
              "endByte": 209,
              "startLine": 13,
              "startColumn": 13,
              "endLine": 13,
              "endColumn": 31
            },
            "expansion": [{
              "kind": "replaces",
              "span": {
                "sourceId": "b843d24770b14fca8b4de092f890f44b",
                "startByte": 153,
                "endByte": 219,
                "startLine": 12,
                "startColumn": 9,
                "endLine": 14,
                "endColumn": 10
              }
            }]
          }
        }
      ],
      "origin": {
        "primary": {
          "sourceId": "b843d24770b14fca8b4de092f890f44b",
          "startByte": 153,
          "endByte": 219,
          "startLine": 12,
          "startColumn": 9,
          "endLine": 14,
          "endColumn": 10
        },
        "expansion": []
      }
    }
  ],
  "actionPlans": [
    {
      "action": "evolve",
      "steps": [{
        "abilityId": "6d50bff8290cfb3c3e50356d77552701",
        "frame": "new"
      }]
    },
    {
      "action": "superevolve",
      "steps": [{
        "abilityId": "bb7c6d9a8a76a9a43f670559abc969e8",
        "frame": "new"
      }]
    }
  ],
  "meta": { "pack": 10000, "class": "neutral", "rarity": "silver" },
  "locales": {
    "chs": {
      "name": "煌响使者·亨莉雅妲",
      "text": "【进化时】回复自己的主战者2点生命值。\n【超进化时】改为回复4点。"
    }
  }
}
```

此卡的超进化执行计划只引用第二个能力，普通 `evolve` 节点不会出现在该计划中，
因此测试中的 2 点治疗事实不会产生。

## JSON 示例：现有 wbotest 场景

下例对应“超进化替换普通进化治疗”。为突出 Test IR，使用缩短的示例 ID；正式
输出必须使用完整 128 位 ID，并物化双方所有默认字段。

```json
{
  "id": "b40b3299cdf5c99510a55f9367e9db6e",
  "name": "超进化替换普通进化治疗",
  "seed": "0x00000000000003eb",
  "initialState": {
    "turn": { "active": "own", "number": 6 },
    "phase": "main",
    "players": {
      "own": {
        "leader": { "life": 10, "maxLife": 20 },
        "pp": 0,
        "maxpp": 0,
        "ep": 0,
        "sep": 1,
        "combo": 0,
        "shadows": 0,
        "zones": {
          "deck": [],
          "hand": [],
          "field": [{
            "instanceId": "1d57fc2a30d3841001d5b227693c905e",
            "alias": "healer",
            "cardId": 10002110,
            "declaredType": "follower",
            "overrides": {}
          }],
          "graveyard": [],
          "banished": [],
          "destroyed": []
        }
      },
      "oppo": {
        "leader": { "life": 20, "maxLife": 20 },
        "pp": 0,
        "maxpp": 0,
        "ep": 0,
        "sep": 0,
        "combo": 0,
        "shadows": 0,
        "zones": {
          "deck": [], "hand": [], "field": [], "graveyard": [],
          "banished": [], "destroyed": []
        }
      }
    },
    "aliases": { "healer": "1d57fc2a30d3841001d5b227693c905e" }
  },
  "actions": [{
    "kind": "superevolve",
    "actor": "own",
    "source": "1d57fc2a30d3841001d5b227693c905e"
  }],
  "assertions": [
    { "kind": "legal" },
    {
      "kind": "compare_value",
      "left": { "kind": "player_field", "side": "own", "field": "leader.life" },
      "op": "eq",
      "right": { "kind": "integer", "value": 14 }
    },
    {
      "kind": "events",
      "mode": "contains_ordered",
      "expected": [{
        "kind": "healed",
        "target": { "kind": "leader", "side": "own" },
        "actual": 4
      }]
    },
    {
      "kind": "events",
      "mode": "excludes",
      "expected": [{
        "kind": "healed",
        "target": { "kind": "leader", "side": "own" },
        "actual": 2
      }]
    }
  ],
  "origin": {
    "primary": {
      "sourceId": "df77021718885c1c05b5812c6c041c6d",
      "startByte": 953,
      "endByte": 1741,
      "startLine": 65,
      "startColumn": 1,
      "endLine": 99,
      "endColumn": 2
    },
    "expansion": []
  }
}
```

## 诊断

```text
Diagnostic = {
  code: DiagnosticCode,
  severity: "error" | "warning",
  message: string,
  primary: SourceSpan,
  related: [{ message: string, span: SourceSpan }]
}
```

`message` 可本地化，自动化工具只依赖稳定 `code`。首批代码如下：

| 代码 | 含义 |
|---|---|
| `WBO-E001-SYNTAX` | 语法错误 |
| `WBO-E002-VERSION` | 不支持的源语言版本 |
| `WBO-E003-DUPLICATE-CARD` | 卡牌 ID 重复 |
| `WBO-E004-UNKNOWN-CARD` | 引用的卡牌不存在 |
| `WBO-E005-INVALID-CARD-SHAPE` | 卡牌类型、数值或必需字段不合法 |
| `WBO-E006-DUPLICATE-LOCALE` | 本地化代码重复或顺序非法 |
| `WBO-E007-UNKNOWN-NAME` | 名称、字段、关键词、种族或职业无法解析 |
| `WBO-E008-TYPE-MISMATCH` | 值、集合、目标或字段类型不匹配 |
| `WBO-E009-BINDING-SCOPE` | 绑定未定义、越界或并非所有路径可用 |
| `WBO-E010-DUPLICATE-OPTION` | 模式选项编号重复 |
| `WBO-E011-INVALID-RELATION` | `replaces` 或 `extends` 目标非法、重复或成环 |
| `WBO-E012-INVALID-TRIGGER` | 能力与卡牌类型或上下文不兼容 |
| `WBO-E013-INVALID-FILTER` | 筛选字段不适用于集合成员 |
| `WBO-E014-INTEGER-RANGE` | 整数超出 IR 定宽范围 |
| `WBO-E015-ID-COLLISION` | 稳定 ID 发生碰撞 |
| `WBO-E016-PACK-DEPENDENCY` | 包依赖缺失、冲突、成环或哈希不符 |
| `WBO-E017-IR-INCOMPATIBLE` | IR 主版本或模式不受支持 |
| `WBT-E001-DUPLICATE-SCENARIO` | 同文件场景名称重复 |
| `WBT-E002-DUPLICATE-ALIAS` | 测试实例别名重复 |
| `WBT-E003-CARD-TYPE` | 测试声明类型与卡牌定义不一致 |
| `WBT-E004-INVALID-OVERRIDE` | 运行时覆盖不适用于该实例 |
| `WBT-E005-ACTION-ORDER` | `select` 或 `mode` 没有对应请求 |
| `WBT-E006-UNKNOWN-ALIAS` | 测试引用未知别名 |
| `WBT-E007-INVALID-ASSERTION` | 断言字段、集合或事件模式不合法 |
| `target_required` | 运行时必选目标为空，动作非法 |
| `insufficient_resource` | 运行时动作费用不足 |
| `invalid_phase` | 运行时动作时点非法 |
| `stale_choice` | 选择请求已过期或状态版本不符 |
| `invalid_choice` | 选择结果不在候选集合中 |

编译错误阻止产包。运行时非法原因沿用稳定代码，但不带源码 `SourceSpan` 时应附带
相关动作、实例 ID 和节点 ID。

## IR 版本与兼容

`irVersion` 使用语义化版本：

- 主版本变化表示删除字段、改变既有字段含义、改变执行顺序或确定性算法；运行时
  必须拒绝未知主版本。
- 次版本变化只允许增加可选字段、判别联合成员或诊断代码。运行时只有在声明支持
  所有实际出现的 `kind` 和 `requiredFeatures` 时才能加载，不能跳过未知效果节点。
- 修订版本变化用于不改变数据语义的模式澄清和诊断修正。

加载器先检查主版本，再检查 `schemaHash`、`requiredFeatures`、所有判别值和依赖。
未知可选元数据可保留并忽略；未知 `EffectNode`、`Trigger`、`BoolExpr`、事件载荷或
RNG 算法必须拒绝。升级工具应执行显式的 `旧版本 -> 新版本` 纯数据迁移，并重新
计算内容哈希，不允许运行时根据字段缺失猜测旧语义。

服务器与 WASM 应由同一份机器可读模式生成各自数据结构和编解码器。Go 实现可用
带显式判别字段的结构体联合，但本文不规定具体语言类层次；规则行为来自 IR 执行器，
不得把 TypeScript 或任何宿主语言源码作为卡牌规则实现或包内容。
