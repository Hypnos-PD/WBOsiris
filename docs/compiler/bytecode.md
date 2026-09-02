# WBO 规则字节码 0.1 设计

本文定义未来 WBO 规则字节码和虚拟机的语义边界。它是实现设计，不是当前可发布
格式；在验证器、测试向量和完整规则执行器完成前，`.wbos` 不得声明非空
`bytecodeVersion`。

## 目标与非目标

字节码负责把类型化 IR 降低为小型、确定、可验证的执行程序，使 Go 服务端和浏览器
WASM 使用相同行为。它不负责 UI、动画、网络、卡牌说明文本或宿主对象访问。

当前原型运行器解释严格类型化 IR，并已使用显式执行栈、请求/响应暂停、可跨进程恢复
的版本化 Continuation、确定性执行预算、TriggerIndex、FIFO 触发队列和死亡批次覆盖
现有测试纵向切片。它尚未实现全部规则动作，因此不构成字节码规范；字节码语义仍以
语言规范、类型化 IR 和 MatchRuleset 为准。

## 编译阶段

```text
.wbo / .wbotest
  -> syntax AST
  -> name and type checking
  -> normalized typed IR
  -> preflight and resolution plans
  -> linked module
  -> verified bytecode
  -> .wbos
```

编译器必须在降低前完成卡牌引用链接、绑定类型推导、`replaces`/`extends` 展开、目标
基数检查和触发器分类。字节码中不得保留需要再次解释的 DSL token。

## 虚拟机模型

采用带类型槽位的寄存器虚拟机。每个函数声明固定槽位类型；指令只能读写签名允许的
槽位。选择寄存器模型是因为规则结算需要序列化绑定、暂停等待选择，并在恢复后继续
使用同一执行帧。

```text
Value = None | Boolean | Integer | Player | Leader | Entity |
        EntitySet | Card | Zone | Event | Option
```

`EntitySet` 是不可变、有序的实例快照，不是宿主语言哈希集合。`None` 只允许出现在
显式可空槽位；对失效实例、错误字段或错误类型的访问是 VM fault，不能静默转换为空。

所有整数使用规范指定的定宽整数和显式溢出策略。VM 禁止浮点数、系统时间、文件、
网络、宿主随机数及依赖无序映射迭代的行为。

## 模块

```text
BytecodeModule = {
  bytecodeVersion: SemVer,
  requiredFeatures: [string],
  constants: [Constant],
  queries: [Query],
  predicates: [PredicateFunction],
  functions: [RuleFunction],
  actionPlans: [ActionPlan],
  triggerIndex: TriggerIndex,
  debugMap: DebugMap?
}
```

常量、函数和查询由模块内无符号索引引用。链接完成后，外部卡牌引用仍保留稳定
CardId，同时可附加包内索引作为加载优化；哈希和诊断不得依赖索引恰好相同。

## 函数与帧

打出效果、能力、融合块、谓词和嵌套控制块编译为有类型函数或基本块。规则函数具有
隐式上下文：

```text
self        能力来源实例
controller  能力控制者
event       触发事件，可空
action      当前玩家指令
frame       当前绑定帧
```

每次独立能力和触发实例创建新帧。只有编译后的 `frame=continue` 执行计划可以继续前一
帧，因此 `superevolve extends evolve` 能读取普通进化步骤产生的绑定；独立或替换步骤
不能意外共享 `target`、`summoned` 或 `drawn`。

源码绑定名称编译为有类型槽位。每个产生绑定的节点分配新槽位，即使源码复用了同一
名称。批量召唤和抽牌输出是有序 `EntitySet`，事件触发预绑定的主体与产生操作最终
输出必须使用不同槽位。

## 预检与结算

一个玩家动作包含两个入口：

```text
ActionPlan {
  preflight: FunctionId,
  resolve: FunctionId
}
```

预检在状态快照上检查阶段、所有权、费用、场地容量、使用限制、最高可支付爆能强化档、
启动限制和所有可达的 `require`。预检不得提交状态、事件、队列、实例 ID、请求 ID、
事实序号或 RNG 决策。

预检函数只能使用验证器标记为只读的指令。若后续必选目标依赖前序效果形成的临时状态，
编译器必须生成隔离的符号状态或可丢弃事务；不能在真实对局状态上执行后回滚。

结算阶段按顺序提交。后续普通操作因空集合或资源不足而跳过时，不回滚此前已经成功的
操作和资源支付；只有动作预检失败保持完整可序列化状态不变。

## 暂停与选择

玩家选择指令可以返回 `suspended`。Continuation 必须完整序列化：

```text
Continuation = {
  actionId,
  requestId,
  stateRevision,
  functionId,
  blockId,
  instructionIndex,
  callStack,
  frames,
  pendingEvents,
  pendingTriggers,
  budgetState
}
```

请求保存有序候选快照，不在恢复时重新计算。响应必须匹配动作、请求、状态修订、选择
类型和候选集合；过期、重复或越界响应不得改变状态。

当前 IR 解释器的 `Continuation 0.3.0` 使用稳定块 ID 保存每个根效果块和嵌套分支，
并保存共享绑定帧、栈 PC、当前及待处理触发、请求候选、实例、区域、事件、RNG、实例
计数器和状态修订。编码锁定卡牌包哈希与默认规则集哈希；恢复时重新建立块索引，并
拒绝块、节点、候选、绑定、卡包、规则集或修订不匹配。JSON 解码拒绝未知字段、重复键、
尾随数据和超过 1 MiB 的 Continuation。该解释器格式是当前运行时协议，不替代未来
字节码函数与基本块格式。

`choose` 的 optional 表示候选为空时绑定 `none` 并继续，不表示候选非空时玩家可以
跳过。`require` 空候选使整个动作在预检阶段非法。`random_choose` 不暂停，只在候选
非空时消费一次规则随机决策。

## 指令格式

设计阶段使用符号形式：

```text
OPCODE destination, operand...
```

控制流目标使用基本块 ID，不直接暴露字节偏移。最终二进制编码可以把基本块降低为
偏移，但验证和调试模型仍以函数、基本块和指令索引表示 PC。

### 常量、上下文与字段

```text
LOAD_CONST         dst, constant
LOAD_SELF          dst
LOAD_CONTROLLER    dst
LOAD_EVENT         dst
READ_PLAYER_FIELD  dst, player, field
READ_ENTITY_FIELD  dst, entity, field
READ_FUSION_FIELD  dst, entity, cost|distinct
```

### 集合与谓词

```text
QUERY_SET          dst, query
FILTER_SET         dst, source, predicate
EXCLUDE_ENTITY     dst, source, entity
COUNT_SET          dst, source
```

查询描述 side、zone、member type 和稳定遍历顺序。谓词是只读纯函数，不得暂停、
写状态、分配实例或消费 RNG。

### 比较与控制流

```text
COMPARE            dst, left, eq|ne|lt|le|gt|ge, right
BOOLEAN_NOT        dst, value
JUMP               block
JUMP_IF_TRUE       condition, block
JUMP_IF_FALSE      condition, block
CALL               function
RETURN
```

不支持不受限制的间接调用。第一版禁止递归，循环只允许编译器生成且必须按有限集合
迭代，由预算系统逐项计费。

### 选择与随机

```text
CHOOSE_OPTIONAL    dst, candidates, requestSpec
CHOOSE_REQUIRED    dst, candidates, requestSpec
CHOOSE_MODE        dst, optionTable, requestSpec
RANDOM_CHOOSE      dst, candidates
```

选择输出的槽位类型必须与候选基数一致。RNG 使用锁定 MatchRuleset 的算法；SplitMix64
v1 的取模映射属于版本语义，不能以拒绝采样作为无版本变化的优化。

### 资源与状态

```text
CHECK_RESOURCE     dst, player, resource, amount
PAY_RESOURCE       player, resource, amount
TRY_PAY_RESOURCE   dst, player, resource, amount
ADJUST_RESOURCE    player, resource, delta, minimum, maximum
ADJUST_FIELD       entity, field, delta, minimum, maximum
SET_FORM           entity, form, silent|triggered
```

`earthrite`、`necromancy` 和资源上限在降低后使用这些核心操作及规则集提供的资源策略，
VM 不解析源语言关键字。

### 卡牌与区域

```text
ADD_CARD           dst, player, card, count, destination
DRAW               dst, player, count|all, predicate
SUMMON             dst, player, card, count
MOVE_ENTITY        entity, destination, movePolicy
DESTROY            target
BANISH             target
RETURN_TO_DECK     target, insertionPolicy
REANIMATE          dst, player, maxCost, tiePolicy
ATTACH_MATERIALS   source, orderedMaterials
TRANSFORM          target, card, preserveFlags
```

`TRANSFORM` 保留 InstanceId；是否保留材料由已验证标志控制。融合材料附着不是普通区域
移动，不产生破坏、消失或谢幕曲。成功随机插回牌组总是消费一次规则决策。

### 数值与能力

```text
DAMAGE             target, amount, damageType
HEAL               target, amount
BUFF_STATS         target, attackDelta, lifeDelta
ADD_KEYWORD        target, keyword
REMOVE_KEYWORD     target, keyword
SPELLBOOST         target, times
```

规则操作自动产生规范 EventFact。普通卡牌字节码不得任意伪造事实；事件入队、死亡批次、
移动替换、谢幕曲和同时触发排序由 VM 与 MatchRuleset 共同执行。

## 示例

源码：

```wbo
require target from own.field.followers where life <= 3;
damage target 2;
```

降低结果：

```text
preflight:
  QUERY_SET          r3, query#12
  REQUIRE_NONEMPTY   r3, target_required
  RETURN

resolve:
  QUERY_SET          r3, query#12
  CHOOSE_REQUIRED    r4, r3, request#4
  LOAD_CONST         r5, 2
  DAMAGE             r4, r5, effect
  RETURN
```

`REQUIRE_NONEMPTY` 仅允许出现在只读预检函数；`CHOOSE_REQUIRED` 使用预检建立的请求
契约并可以暂停。

## 触发与调度

模块预编译 TriggerIndex，以事件类型定位候选能力。事件发生后：

1. 创建不可变 EventFact 并分配事实序号。
2. 从 TriggerIndex 找到候选能力并求值过滤器。
3. 按规则集的行动方、区域实例顺序和声明顺序排序。
4. 为每个触发实例创建独立帧并入队。
5. 在规则检查点排空队列；新事实继续按相同流程处理。

破坏和同时死亡必须先形成死亡批次，再按规则集顺序安排谢幕曲。立即递归调用来源能力
不是合规实现。

## 验证器

加载字节码前必须验证：

- 版本、必需功能、opcode 和所有索引有效。
- 指令边界、基本块入口和跳转目标有效。
- 槽位在读取前赋值，类型在所有分支汇合点一致。
- 函数签名、调用上下文和 `frame=continue` 兼容。
- 预检不含写状态、请求提交、分配、事实或 RNG 指令。
- 谓词保持纯函数限制。
- 绑定不逃出帧，候选只由对应请求消费。
- 卡牌、能力、查询、选项和触发引用存在且唯一。
- 不存在递归、不可达非法块或无预算的无限控制流。
- 调试 PC 范围只引用有效函数与指令。

未知 opcode 或语义功能必须拒绝，不能作为 no-op 跳过。

## 确定性预算

VM 使用规则步数而不是真实时间限制执行。规则集版本必须固定以下预算及计费方式：

```text
执行指令数
谓词访问实例数
调用和帧深度
事件、触发与替换链深度
队列和事实数量
候选集合长度
新建实例数量
Continuation 编码长度
```

每次指令和集合元素访问按规范计费。预算耗尽产生确定性的
`execution_budget_exceeded` 对局错误，不根据设备性能改变结果，也不能被视为普通卡牌
效果失败后继续结算。

当前 IR 解释器已按 `wbo-standard-0.2.0.executionBudget` 对效果节点、查询访问、最大
栈深度、请求候选、事件、触发和新实例计费，并限制 Continuation JSON 字节数。每个
玩家动作开始时重置结算计数；暂停和恢复保留计数。预检沙箱使用独立且不提交的查询、
候选和嵌套深度计数，预检超限同样终止对局。扣费发生在对应操作之前，超限操作不执行，
Session 随即进入终止性的 `fault`，后续动作和选择响应返回同一错误。

## 调试映射

开发构建为每个 PC 范围保存：

```text
FunctionId
NodeId
SourceSpan
语法糖展开来源链
能力与卡牌 ID
绑定槽位的源名称
```

运行时诊断和 EventFact 可携带 NodeId、ActionId 与 InstanceId，但不得依赖本地化文本。
剥离调试信息不能改变字节码行为。

## 实施顺序

严格 Go IR lowering、`wbo test` 的 WBOS 执行边界，以及当前语料所需的请求暂停、
跨进程 Continuation、沙箱预检、TriggerIndex、FIFO 触发队列与死亡批次已经完成。
后续顺序为：

1. 实现字节码验证器及文本汇编表示，建立测试向量。
2. 将完整规则场景同时运行于 IR 解释器和字节码 VM，做差分验证。
3. 语义稳定后分配 opcode 数字并设计 WBOS 二进制分区。

在差分验证长期稳定前，字节码属于实验功能，不应作为线上回放或卡包兼容边界。
