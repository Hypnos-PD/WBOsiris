# WBO 卡牌 DSL 0.1 草案

本文档记录首批卡牌文件采用的语法与语义。当前版本仍是草案：真实卡牌是
语言的一致性样本，在完成这些卡牌的转换期间，语法仍可能调整。

## 待扩展语义

当前卡牌包中仍有部分文本保留为 `unplayable`，原因是其效果依赖尚未定型的规则对象，
包括纹章对象、复制随从和特殊启动费用。实现这些语义前，
编译器与运行器会明确拒绝相关效果，不以客户端表现推断规则。

随从效果可以声明固定伤害减免和潜行：

```wbo
effect {
    stealth;
    damage_reduction 1;
}
```

`damage_reduction N` 使该随从每次受到的伤害减少 `N`，实际伤害最低为零。
`stealth` 使敌方效果的主动目标查询和攻击目标查询忽略该随从；范围效果和随机效果仍可作用于它。
拥有潜行的随从声明攻击，或通过能力造成实际伤害时解除潜行。
`aura` 仅使敌方效果的主动目标查询忽略该随从；它不会影响攻击，也可能成为随机效果的目标。
`attack_limit N` 使该随从每回合最多可以进行 `N` 次攻击，`N` 必须为正整数。
在效果块中使用 `set_attack_limit self N` 可在结算时动态设置自身上限，适合爆能强化或进化效果；
该值同样必须为正整数，并从下一次攻击合法性检查开始生效。
`cannot_attack` 使随从无法攻击；`cannot_attack_follower` 和 `cannot_attack_leader`
分别只禁止攻击随从或主战者。攻击限制会从合法动作集合中移除对应动作，直接提交时也会被拒绝。

## 文件结构

```wbo
wbo 0.1.0;

card 10001120 {
    type follower;
    cost 2;
    stats 0/2;

    effect {
        ward;
        lastwords {
            draw 1;
        }
    }

    meta {
        pack 10000;
        class neutral;
        rarity bronze;
    }

    locale chs {
        name "...";
        text "...";
    }
}
```

本地化块固定按 `chs`、`eng`、`jpn`、`kor`、`cht` 排列。源数据中的纯显示
标记会被移除。本地化文本只用于展示，规则引擎绝不能将其解析为可执行规则。

## 卡牌计数器

卡牌文本中的 X 可声明为每张实例独立保存的命名计数器：

```wbo
effect {
    counter x 2;
    spellboost { add 1 counter x; }
    require target from oppo.field.followers;
    damage target self.counter.x;
}
```

`counter NAME INITIAL;` 只允许出现在 `effect` 最外层，同一名称只能声明一次。
名称匹配 `[a-z][a-z0-9_]{0,31}`，初始值、增加量及运行时值均为 `0..2147483647`。
`add N counter NAME;` 增加能力来源实例的值；`self.counter.NAME` 可用于伤害、
治疗、属性变化、重复次数及条件，例如 `if self.counter.x >= 5 { draw 1; }`。
读取和增加必须引用本卡已声明的名称，不从展示文本推断计数器。

创建实例时使用声明的初始值。从手牌返回牌组及再次抽取保留当前值；从战场返回
手牌或牌组会重置为初始值。进化、破坏和放逐本身不重置；变身按新卡身份重新初始化。
如果已排队能力的来源发生变身、已无原计数器，读取返回零，增加不执行。
超过上限会产生终止错误 `counter_overflow`，不回绕、不截断，后续效果不再执行。
计数器声明不消耗效果执行步骤；重复次数在进入块时读取一次，其余表达式在执行时读取。

融合能力与魔力增幅能力允许自身变身；其他变身触发时点仍待扩展。

## 来源与目标

- `self`：提供当前效果的卡牌或场上对象。
- `own`：当前控制者。
- `oppo`：当前控制者的对手。
- `target`：最近一次目标选择所绑定的值。
- `summoned`、`drawn`：最近一次对应操作成功产生的有序集合。
- `destroyed`：最近一次 `destroy` 操作实际破坏的目标集合。

对 `summoned` 或 `drawn` 执行操作时，会按顺序对集合中的每个实例执行。手牌
或战场空间不足时，集合只包含实际成功进入目标区域的实例。新的召唤或抽牌
操作会替换此前的同名绑定。

存在候选对象时，`choose` 必须选择目标。默认选择一个，也可在语句末尾用
`count N` 指定正整数数量；不足 N 个时选择全部可用目标，空集合时绑定 `none`
并继续结算。对 `none` 执行的操作不产生效果；非空候选不能主动跳过选择。

```wbo
choose target from oppo.field.followers;
choose targets from oppo.field.followers count 2;
destroy targets;
```

`require` 表示在验证玩家指令时进行必选目标选择。候选数量不足 `count`（默认 1）时，
该卡牌或启动能力不能使用，不支付费用也不改变状态。数量要求依据卡牌文本：
[官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#bxi2b429pl7)
明确说明，需要选择两张手牌的法术在只有一张合适手牌时无法使用。

```wbo
require target from own.field.followers;
```

筛选条件写在目标集合之后，`other` 用于排除 `self`。

```wbo
choose target from own.field.followers other where trait golem;
require target from oppo.field.followers where life <= 3;
```

`buff` 也支持在数值后追加 `where` 过滤器。过滤先于操作执行，只处理匹配实例，
并保持目标集合的稳定顺序；未匹配或空集合不会产生效果。`and` 表示条件同时成立，
`or` 表示任意一组条件成立，且 `and` 优先于 `or`。同一实例匹配多个分支时只出现一次。
`!=` 表示不等于。数值必须包含攻击力与生命值两部分，即使增量为零也不能省略。

```wbo
buff own.hand.followers +1/+0 where trait puppetry;
```

关键词的添加与移除同样支持 `other` 和 `where`：

```wbo
add barrier to own.field.followers other where class swordcraft;
remove ward from oppo.field.followers where life <= 3;
```

筛选在关键词修改前完成；`other` 只排除当前能力来源实例，其他同名实例仍可受影响。
这些操作不要求玩家选择目标，不受潜行或灵气的选择限制；空集合直接继续结算。
赋予关键词是对实例的一次修改，不会持续影响以后入场的随从。要响应之后的入场事件，
使用 `when own follower summoned`，并在块内操作该次事件的 `summoned` 绑定。

`random ... count N` 从当前候选中等概率、不重复地抽取最多 N 个实例，每取一个
消耗一次对局随机数；空集合不消耗。同名卡牌的不同实例仍是不同候选。选中集合按
原候选顺序绑定，后续一次 `damage` 或 `destroy` 同时处理全部目标。

```wbo
random target from oppo.field.followers;
random targets from oppo.field.followers count 3;
damage targets 3;
```

玩家多选也按候选顺序绑定，不以点击顺序决定伤害或死亡顺序；同一实例不能重复选择。
`count N` 是一次多选，`repeat N` 则会每次重新求候选，同一存活目标可以再次被选中。

选择可以在筛选器之后、`count` 之前使用 `highest attack`、`lowest life` 或
`highest cost` 等修饰符，只保留该字段达到极值的全部并列候选：

```wbo
random target from oppo.field.followers highest attack;
destroy target;
damage oppo.field.followers 1;
choose cheapest from own.hand lowest cost;
```

`highest` 与 `lowest` 均支持 `attack`、`life`、`cost`，读取实例当前数值；费用以零
为下限，攻击力和生命值只对随从有意义，其他卡牌不参加这两类比较。先应用 `other`
和 `where`；玩家选择还会先移除不能被选择的目标，再求极值。随机效果不受潜行、
灵气的目标选择限制。极值查询本身不消费随机数；后续 `random` 只在极值候选中抽取。
`highest attack count 2` 最多取两个并列最高者，不会补选攻击力次高者。
暂停时保存极值候选，恢复会重新核对候选合法性；查询预算不足不会使用部分结果。

## 效果操作

`repeat 数值 { ... }` 按顺序重复执行块内效果。次数可以是非负整数、数值引用或
`count(...)`，进入重复块时只读取一次；零或负的计算结果不执行块体。每次循环重新执行
块内选择，因而随机伤害会从该次仍在场的随从中抽选目标，候选为空时不消耗随机决策。

```wbo
evolve {
    repeat count(own.hand.followers where trait pixie) {
        random target from oppo.field.followers;
        damage target 1;
    }
}
```

每次伤害分别应用屏障、伤害减免和死亡判定；重复期间新产生的触发能力排队等待，
直到整个重复块结束。例如，剩余随机伤害不会命中该次伤害触发的谢幕曲召唤物。
此顺序见 [官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#7g3t2088i)。

重复块可以嵌套。每次循环都继承进入该块时的外层绑定，并有独立的绑定作用域；
`target`、`drawn`、`summoned` 等块内绑定不会覆盖外层同名绑定，也不会传给下一次循环。
块内允许 `choose` 和 `mode` 暂停；恢复继续当前次数，已完成的次数不会重做。
前置校验 `require` 不允许出现在重复块内，因为它不能预先验证后续循环变化后的目标。
次数和块体均受执行预算约束，大量空循环也会计费；执行器不会预先展开全部次数。

`gain own.shadows N` 或 `gain oppo.shadows N` 只增加墓场资源计数，不创建墓场卡牌或
被破坏记录；与 `necromancy` 的资源支付共用同一个计数。

`spellboost S N` 使集合 `S` 中每张手牌的魔力增幅能力按声明顺序发动 `N` 次。费用等
实例状态修改会保留在手牌实例上，并在费用降低时以 `minimum` 指定的下限截断。

正常使用法术时，规则执行器自动使控制者当时剩余的每张手牌魔力增幅一次；卡牌作者
不需要为此添加 `spellboost own.hand 1`。法术自身和随后抽到的卡牌不参与这一次增幅，
但随后抽到的卡牌可以参与法术文本显式产生的增幅。

增幅能力在触发时入队，等待当前法术的效果结算完成后执行。已入队能力绑定原来的
手牌实例；即使它在法术结算中返回牌组，能力仍会执行。从手牌返回牌组会保留附加效果。
例如，原费用为 6、每次增幅费用减 1 的手牌，被法术返回牌组后若未抽回，法术结束时
费用变为 5；若抽回并再增幅 5 次，则最终为 0。该时序参见
[官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#gzffm2_g3y)。

效果从上到下依次执行。某个操作失败时，不会回滚此前已经执行的操作或支付。
法术 `effect` 中最外层的操作会在使用法术时执行；随从和护符通过能力块声明
对应的执行时点。

法术完成自身效果后才进入墓场并增加一点墓场资源，随后结算等待中的触发能力。
作者无需显式加入这一点资源；法术正文中的 `necromancy N` 只能使用此前已取得的
墓场资源，不能提前消费这张法术尚未提供的一点。`necromancy` 支付仅扣除资源计数，
不删除墓场卡牌或被破坏卡牌历史。

带筛选的 `draw N from deck where ...` 按实例等概率、不放回抽取，符合
[官方抽选概率 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#q0bo1yysvucb)：
同名卡牌的每个副本均占一个抽选机会。手牌按抽中顺序排列，未抽中的牌保留相对顺序。
每抽中一张消费一次随机决策，空集合不消费；`draw all` 按牌组顺序取全部匹配项，
不消费随机决策。普通 `draw N` 仍从牌组顶部抽取。
筛选不足只抽取现有匹配项，不因未找到目标而败北。所有抽牌事件公开数量，隐藏手牌
身份；超出手牌上限的卡牌进入墓场，但不加入供后续能力使用的 `drawn` 绑定。

```wbo
draw 2;
draw all from deck where card 10022120;
draw 1 from deck where type follower;
add 2 card 90011110 to hand;
summon 1 card 90021110;
damage oppo.leader 3;
heal own.leader 2;
buff self +1/+1;
buff own.hand +1/+0 where trait puppetry;
destroy target;
banish target;
return target to hand;
return target to deck;
add ward to target;
remove ward from target;
```

`damage`、`heal` 的数值可写为 `count(集合 [where 筛选])`，例如：

```wbo
fanfare {
    draw 1;
    heal own.leader count(own.hand);
}
```

上述回复量在抽牌完成后读取；打出的随从已离开手牌，不计入手牌张数。
主战者生命值不会超过上限。空集合计数为零。计数支持区域集合与类型后缀，
例如 `count(own.field.followers where trait golem)`；括号内的 `where` 筛选计数来源，
括号外的 `where` 筛选效果目标。一次效果只读取一次计数，对全部目标使用同一数值。
也可统计已定义的集合绑定，如 `count(drawn)`、`count(summoned)`、`count(targets)`，
以及 `count(destroyed where type amulet)`。绑定必须在当前作用域内先定义再使用。
计数不消耗随机决策；查询预算耗尽时不使用部分结果执行效果。
数值也可直接读取 `own`/`oppo` 的 `combo`、`pp`、`maxpp`、`life`、`ep`、`sep`、`shadows`，
或者读取 `self.cost`；随从还可读取 `self.attack` 和 `self.life`。
`own` 始终相对能力控制者，`self` 为发动能力的卡牌实例。费用支付和使用卡牌的连击计数
在入场曲之前发生，因此入场曲中的 `own.pp` 已扣除费用，`own.combo` 包含本卡牌。

```wbo
fanfare {
    buff self +own.combo/+0;
}
```

`buff` 的两项增量均可使用数值引用或 `count(...)`，正负号必须显式写出：

```wbo
buff self +count(own.hand.followers where trait pixie)/+count(own.hand.followers where trait pixie);
buff own.field.followers +self.attack/-count(oppo.hand);
damage target self.attack;
```

两项增量在修改任何目标前分别读取一次；即使目标包含 `self`，也不会因为先修改攻击力
而改变同一次操作的生命值增量，或使后面的目标读取到已修改值。统计查询超出预算时，
整个操作不修改目标。选择操作暂停并恢复后，在数值操作实际开始时读取状态。

攻击力内部允许保留负值，增益计算保留该负值；伤害和回复量的下限为零。
负攻击力参与后续增益计算的说明见 [官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#2mrx5ewte1)。
当前不支持计数嵌套、一般算术、绑定对象的数值字段，或将动态数值用于抽牌数量等其他操作。

分配伤害在数值后写 `distributed`：

```wbo
damage oppo.field.followers 7 distributed;
damage oppo.field.followers count(own.hand) distributed;
damage oppo.field.followers 4 distributed overflow oppo.leader;
```

分配目标必须为 `own.field.followers` 或 `oppo.field.followers`，可在最后加
`where` 筛选。按目标随从进入战场的先后顺序分配，每个随从先分到不超过当前生命值
的额度；未指定 `overflow` 时，全部余量并入最后一个目标的这一次伤害。
指定 `overflow` 时，余量交给目标一侧的主战者；没有随从时全部交给该主战者。
未指定主战者且没有随从时，不造成伤害。不能把另一侧主战者写作溢出目标。

分配额度与实际伤害分开计算：屏障、减伤和伤害免疫不会退回额度重新分配。
一次分配不使用随机决策，全部伤害完成后统一处理死亡和谢幕曲；连续多个分配效果
则各自重新读取当时的战场。潜行、灵气不会排除分配目标，因为这不是玩家选择。

`destroy` 会产生破坏和谢幕曲事件，`banish` 不会。战场已满时，召唤失败，
但此前已经执行的操作不会回滚。

`return ... to deck` 会在从牌组顶部之前到牌组底部之后的所有插入位置中等概率
选择一个位置，只移动目标实例，不改变其他卡牌的相对顺序。候选插入位置始终
非空，因此每次成功返回牌组都会消费一次对局随机决策。

区域移动替换使用 `replace`。替换块在原区域移动发生前执行，原移动被取消；
同一个替换块不会对自己产生的区域移动再次触发。

```wbo
replace self leaving field {
    banish self;
}
```

## 能力

固有关键词直接写成语句，触发能力和启动能力使用代码块。

```wbo
ward;
storm;
rush;
bane;
drain;
intimidate;
barrier;

fanfare { ... }
lastwords { ... }
attack { ... }
clash { ... }
evolve { ... }
superevolve { ... }
superevolve replaces evolve { ... }
superevolve extends evolve { ... }
engage 1 { ... }
enhance 7 { ... }
```

手动进化会触发 `evolve`，手动超进化默认也会触发 `evolve`。普通
`superevolve` 表示超进化时额外触发的独立能力。

卡牌文本使用“改为”时，必须显式替换普通进化能力：

```wbo
evolve {
    heal own.leader 2;
}
superevolve replaces evolve {
    heal own.leader 4;
}
```

卡牌文本在普通进化能力上追加效果时，使用 `extends`。扩展块可以访问普通
进化块产生的 `summoned`、`drawn` 等输出绑定：

```wbo
evolve {
    summon 2 card 90051130;
}
superevolve extends evolve {
    add drain to summoned;
}
```

由其他效果引起的进化必须显式标记：

```wbo
evolve target silent;
superevolve target silent;
```

两者只作用于场上未进化的随从，分别增加 +2/+2 和 +3/+3，不支付 EP/SEP、
不受手动进化解禁回合限制，也不占用本回合手动进化次数。已进化或超进化的目标不再改变，
但后续语句仍会执行。随从保留原有的关键词、身材变化和触发能力，获得对应形态的攻击资格与保护。
`silent` 表示不发动目标卡面上消耗点数才触发的 `evolve`、`superevolve` 能力，不表示没有进化事件。
手动超进化即使没有独立 `superevolve` 块，也会执行该卡牌的 `evolve` 块。

文本中的“本随从进化时”与关键词【进化时】不同，使用事件监听：

```wbo
when self evolved {
    gain own.maxpp 1;
}
```

`when self evolved` 响应自身的手动或能力进化，也响应超进化；
`when self super_evolved` 只响应自身超进化。同名的其他实例不会触发该监听器。
能力产生的事件等待当前动作能力完整执行后结算，选择暂停与恢复不重复发送事件。
来源离场后，其尚未开始执行的场上事件监听不再发动；谢幕曲不受此限制。

选择未进化随从可使用 `where form unevolved`。另支持 `form evolved`（包含超进化）
及 `form super_evolved`，可与其他条件通过 `and` 组合。非随从不匹配任何形态条件。
例如奥莉薇的能力使用 `choose target from own.field.followers other where form unevolved;`。

依据：关键词“进化”“超进化”“进化时”“超进化时”，以及
[能力超进化与进化关键词 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#dt7oehzyd)、
[入场曲与场上监听器 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#2hv4yt1j8)。

`intimidate`（威慑）是固有关键词。敌方随从不能以具有威慑的随从为攻击目标；
能力仍可选择该随从，能力伤害及其他非攻击效果也照常作用。若同一随从同时具有
`intimidate` 与 `ward`，其 `ward` 不生效：威慑只禁止敌方随从攻击它，不迫使敌方
随从改为攻击它，也不限制攻击主战者或其他合法目标。

`unplayable` 是卡牌固有限制，表示该手牌实例不能作为 `play` 玩家指令的来源。
它不阻止该实例成为融合来源，也不阻止其他效果引用、移动或变身该实例。

卡牌种族使用稳定英文标识声明，可在目标筛选和事件监听中引用：

```wbo
trait pixie;
trait departed;
```

种族声明与 `where trait` 使用同一词表，未知标识会被拒绝：

| 标识 | 种族 |
|---|---|
| `officer` | 士兵 |
| `luminous` | 鲁米那斯 |
| `levin` | 雷维翁 |
| `pixie` | 妖精 |
| `departed` | 亡者 |
| `earthsigil` | 土之印 |
| `mysteria` | 玛纳利亚 |
| `golem` | 巨像 |
| `shikigami` | 式神 |
| `artifact` | 创造物 |
| `puppetry` | 人偶 |
| `marine` | 海洋 |
| `loot` | 财宝 |
| `encroacher` | 侵蚀者 |
| `anathema` | 安纳提玛 |

种族与同名卡牌不能互换：文本指定某张卡牌时使用 `where card 卡牌ID`，
只有按种族筛选时才使用 `where trait`。`trait earthsigil` 是卡牌身份，
不会替代 `earthsigil` 固有状态或增加土之印数值。

每个护符实体在控制者的每个回合中只能 `engage` 一次，进入战场的当回合也
可以启动。`engage` 后的整数是启动所需的能量点费用。

爆能强化是强制替代费用。存在一个或多个可支付档位时，必须采用费用最高的
可支付档位，玩家不能选择按原费用或更低强化档位打出。支付一次该档位费用后，
发动该档位及以下的所有爆能强化能力，不只发动最高档位。原本的打出效果和入场曲
仍然执行；每个生效强化块中的必选目标也参与打出前的合法性检查。

例如同时声明 `enhance 3` 和 `enhance 5` 时，剩余 5 点能量点会支付 5 点并发动
两个能力块；剩余 3 点时只支付 3 点并发动 `enhance 3`。卡牌当前费用的增减不修改
强化档位。`enhance 0` 也是可支付档位，不能与未发动强化混同。

多个强化档位累计发动的判例见[官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#e03skkdj6s6)。

## 设置生命值

`set life T N;` 将目标随从的当前生命值设为 N，数值也可使用 `self.life`、
`count(...)` 等数值表达式。所有目标使用操作开始时读取的同一个值，负的计算结果按零处理。

```wbo
fanfare {
    if own.combo >= 3 {
        choose target from oppo.field.followers;
        set life target 1;
    }
}
```

这不是伤害或回复：不消费屏障，不应用伤害减免，也不产生伤害或回复事件。
零生命值的战场随从随后按同一死亡批次破坏，正常触发谢幕曲；手牌中的随从不因此
产生战场破坏。非随从目标忽略，主战者引用会在编译时被拒绝。空目标不产生数值变化。
后续增益与进化基于设置后的数值；从战场返回手牌或牌组时按通常规则恢复卡牌基础状态。

## 破坏结果

每条 `destroy` 操作将实际破坏的目标写入 `destroyed`，包括空结果；后续 `destroy`
会替换同名绑定。集合按照本批破坏的顺序排列，同一实例最多出现一次。
已离开战场、土之印或因保护而未被破坏的目标不计入；其他随从因生命值归零而在同批
离场，也不会加入本操作的结果。伤害和后续谢幕曲产生的破坏不会改写当前块的绑定。

```wbo
fanfare {
    destroy own.field.amulets;
    damage oppo.field.followers count(destroyed);
    damage oppo.leader count(destroyed);
}
```

这里两条伤害读取相同的破坏张数，新触发的谢幕曲等待入场曲结束后结算。
`destroyed` 保存实例集合，不是墓场总数；`own.destroyed` 仍表示已有的破坏历史区域。
结果不会跨越独立能力块，选择暂停与恢复保留当前块内的结果。

## 舍弃与弃牌事件

`discard T;` 舍弃目标集合中仍在手牌的卡牌；支持 `where` 筛选。每张成功舍弃的
卡牌进入其拥有者墓场并使墓场数量加一，不触发谢幕曲，也不计入已破坏随从历史。
空集合、已离开手牌或重复的实例不重复舍弃。手牌溢出、破坏和放逐不产生弃牌事件。

```wbo
engage 3 {
    summon 1 card 90043110;
    choose target from own.hand;
    discard target;
}
```

此处召唤先执行，再选择舍弃卡牌；手牌为空时 `choose` 跳过，启动及召唤仍可完成。
需要“舍弃自身时”发动的能力可写为：

```wbo
when self discarded {
    random target from own.field.followers;
    buff target +1/+0;
}
```

此能力只在该实例实际被舍弃时入队，结算时自身位于墓场；不会在其他卡牌被舍弃时发动。
`when own card discarded` 和 `when oppo card discarded` 则由战场上的来源监听，
可用 `where type spell` 等筛选。事件块内的 `discarded` 绑定指向本次被舍弃的实例。
全部新触发能力等待当前能力结束后执行，选择暂停与存档不会重复舍弃或丢失等待能力。
选择前手牌保持隐藏，舍弃后该卡牌及事件身份向双方公开，录像保存同样的可见性。

## 融合与变身

魔力增幅块可在条件满足时变身为另一张卡牌：

```wbo
counter x 0;
spellboost {
    add 1 counter x;
    if self.counter.x >= 5 {
        transform self into card 90033310;
    }
}
```

同一实例保留区域、拥有者和手牌或牌组中的位置，新身份使用新卡牌的费用、能力与
计数器初始值。魔力增幅发动时按当时的卡牌定义将能力入队；结算途中变身不会把已
入队的旧能力替换为新卡牌的能力。原计数器在新身份中不存在时，旧能力的增加操作
不执行、读取为零，因此跨过阈值后的剩余增幅不会让此卡再次变身。
原卡正常使用时只执行其抽牌效果，不因使用自身发动魔力增幅。

手牌或牌组中的变身事件只向拥有者公开卡牌身份，录像保持相同可见性。
变身后的能力与费用立即可用于下一条玩家指令。变身不触发入场或谢幕曲，
参见[官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#7wzi4bb1a)。

融合能力声明可作为材料的己方手牌集合，并在一次融合成功后执行能力块：

```wbo
fusion material from own.hand where type amulet and trait artifact {
    transform self into card 90072110 preserving materials;
}
```

融合是独立的玩家指令，不是打出卡牌。指令以手牌中的融合来源实例为对象，并产生
一次命令级材料选择请求。玩家必须一次选择一个或多个互不相同的合法手牌实例；
来源实例本身自动排除在候选外，重复实例、空选择或候选外实例均使响应非法。所有
已选材料作为有序材料附着到来源实例，不执行普通区域移动，不触发破坏、消失、
谢幕曲或墓场计数；附着后材料不再属于手牌，也不能被普通区域集合选中。
每个来源实例每回合最多成功融合一次；同名的另一实例独立计算。非法材料响应不会
消耗次数。从手牌返回牌组再抽回不会刷新本回合次数，进入下一回合或变身时重置。

可用一个材料集合表达多个候选种类，使玩家一次选择任意组合：

```wbo
fusion material from own.hand where card 90073120 or card 90073130 {
    if fused.distinct == 2 {
        transform self into card 90074110 preserving materials;
    }
}
```

多个融合声明按源码顺序选取首个满足最少材料数的入口，各声明不自动合并；同一次
材料选择需要覆盖多个种类时，应在一个声明中使用 `or`。

融合能力块在全部材料附着后结算一次，不按材料数量重复结算。`transform self into
card C preserving materials` 将来源实例的卡牌定义改为 `C`，保留来源的
`InstanceId` 和全部已有及本次附着的材料；该操作不会为变身结果创建新实例，也
不会把材料转移到区域中。
变身后的费用、身材、固有能力及初始状态取自新卡牌定义；旧卡牌的伤害、附加效果、
进化、攻击次数和启动状态不继承。变身不会触发入场能力。

融合变身链已经完整定义到 `90074110`。融合块可读取 `fused.cost` 和
`fused.distinct`：前者为全部已附着材料的原始费用合计，后者为按卡牌 ID 计算的
材料种类数。变身保留材料，因此这两个值跨变身连续累积。

## 条件与资源

`restore own.pp;` 将自己的能量回复至执行时的上限；`restore oppo.pp;` 作用于对手。
它不改变能量上限，也不会扣除已经高于上限的额外能量。固定数量的回复使用
`gain own.pp N;`，同样只增加至能量上限，不扣除已有的额外能量。

例如“超越次元”的效果依次为：

```wbo
return own.hand to deck;
draw 5;
spellboost own.hand 5;
restore own.pp;
```

使用中的法术已经离开手牌，不会被自身返回牌组。使用法术时已触发的魔力增幅
保留在原手牌实例上，即使该实例随后返回牌组也会结算；新抽到的牌只获得后续
显式发动的五次增幅。

```wbo
if combo >= 3 { ... }
if overflow { ... } else { ... }
earthrite 1 { ... }
necromancy 4 { ... }
```

卡牌在入场曲结算前已经计入连击。土之秘术和唤灵会在资源充足时自动支付，
资源不足时跳过对应代码块。支付成功后，即使后续操作失败也不会退还资源。

土之印是护符实体。新的土之印进入战场时，会取得已有土之印的全部层数并保留
新实例；被合并的旧实例移入消失区，不触发破坏、谢幕曲或墓场计数。
`add N earthsigil;` 增加已有护符的土之印；没有土之印护符时，召唤层数为 N 的
大地之魔片（90031210），满场则无法召唤，N 为零时不创建实例。因此，包含正数
土之印增加效果的卡包必须同时包含该衍生物，编译器和运行包解码器都会检查依赖。
土之印不能被对手的能力指定，也不能被能力破坏；土之秘术将层数消耗至零时，
护符被破坏并增加墓场，谢幕曲按通常顺序结算，不影响同场的普通随从或护符。

魔力增幅记录在每一张手牌实例上，费用降低的下限为零。

亡者召还会选择费用不超过指定值且费用最高的已破坏随从；最高费用相同时，
使用对局随机数决定。它会创建一个全新的实例，并且不触发入场曲。

需要玩家选择其中一项的模式使用带稳定编号的 `option`。编号会写入玩家指令
和回放，不能依赖代码块的临时排列位置。

```wbo
mode {
    option 1 {
        label chs "抽取1张随从。";
        label eng "Draw a follower.";
        draw 1 from deck where type follower;
    }
    option 2 {
        label chs "发动【亡者召还_2】。";
        reanimate 2;
    }
}
```

`label 语言 字符串;` 是可选的模式选项文本，只能写在对应 `option` 的所有效果之前。
语言为 `chs`、`eng`、`jpn`、`kor` 或 `cht`，每种语言至多一条，文本不能为空；
支持普通字符串和多行字符串。标签属于当前选项，因此嵌套模式或不同能力可以复用
选项编号而保留各自的说明。标签不执行效果、不消耗指令预算、不决定分支；
玩家响应仍只提交稳定的选项编号。编译器不会从整段卡牌文本中猜测或生成标签。
未写标签的旧卡牌仍可运行，界面显示编号；当前中文网页优先显示简体标签，缺失时使用英文标签。

## 吟唱与事件

吟唱倒计数在其控制者的回合开始时减少。倒计数变为零后，护符立即被破坏，
并可以触发谢幕曲。

标准对局中，每名玩家的战场上限为 5 个对象，最大能量点上限为 10。达到上限
后继续增加对应数值会被截断，但后续效果仍根据截断后的最终值继续结算。

除非筛选条件另有规定，事件监听器也会响应被召唤的 Token。同时死亡的对象
会先收集为同一个死亡批次，再将谢幕曲加入队列。

事件监听统一使用 `when`。事件产生的对象通过与操作输出相同的名称绑定：

```wbo
when own follower summoned {
    buff summoned +1/+1;
}

when own amulet engaged {
    add storm to self;
}

when own turn ends {
    draw 1;
}
```

## 确定性

规则不能读取文件、网络状态、系统时间或外部随机源。所有随机操作都必须使用
规则引擎持有的、带种子的随机数生成器。在锁定相同规则集版本的前提下，玩家
指令、选择结果和初始随机种子必须足以完整重放一场对局。规则集必须锁定随机
算法及排序策略；只有种子而没有规则集标识不足以保证跨实现重放。
