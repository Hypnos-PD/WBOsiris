# WBO 卡牌 DSL 0.1 草案

本文档记录首批卡牌文件采用的语法与语义。当前版本仍是草案：真实卡牌是
语言的一致性样本，在完成这些卡牌的转换期间，语法仍可能调整。

## 使用限制

`unplayable` 表达卡面明确规定的“无法使用”，不阻止手牌中的融合能力。
当前卡牌包中的未来核心和过往核心使用这一限制；它们通过融合变身后采用新卡牌的规则。

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

`ability_target_guard` 表达洛伊德的“对手能力只能选择本卡牌”。拥有该能力的随从在场时，
敌方能力对该玩家战场的主动选择只能选择仍满足原筛选条件的此类随从；多个来源均可成为
候选。筛选不匹配或全部来源拥有潜行、灵气时，不会退回选择其他卡牌。能力来源即使无法
被选择仍维持限制，直到它离场或该能力被移除。

此限制不影响能力控制者对自己卡牌的选择，也不影响攻击、随机、全体或直接指定对象的
效果；它不会将伤害转移给洛伊德。`require` 没有候选时拒绝使用并保留费用，`choose`
没有候选时继续结算。限制在极值比较前应用，并在暂停恢复时重新核验。能力可以通过
`add ability_target_guard to T` 与 `remove ability_target_guard from T` 修改。

## 纹章

在卡牌的 `effect` 之后、`meta` 之前可以声明一个 `crest` 块。纹章以所属卡牌 ID
引用，运行时使用独立实例，不占用战场位置，也不进入牌组、手牌或破坏卡牌历史。

```wbo
effect {
    fanfare { gain own crest 10153140; }
}
crest {
    countdown 4;
    when own turn ends { damage all.leaders 1; }
    locale chs { name "纹章：地下赏金猎人·巴尔特"; text "吟唱 4。自己的回合结束时，对所有主战者造成1点伤害。"; }
    // 依次补齐 eng、jpn、kor、cht 本地化。
}
```

`gain own|oppo crest ID;` 的玩家相对能力来源解释。每位玩家最多拥有五个不同纹章，
同一定义重复获得时不新增实例，也不刷新吟唱。来源随从离场不移除纹章。
省略 `countdown` 表示永久保留；显式吟唱必须为 `1..65535`，在控制者回合开始时减少，
归零后离开主战者区域并结算谢幕曲，不增加墓场数。

纹章块允许命名 `counter`、`countdown`、`lastwords` 和 `when` 事件能力，之后按固定
顺序声明五种本地化。至少声明一个能力。纹章监听来源固定为主战者区域，不能使用
`when self ...` 或 `while self in ...`。入场监听可使用 `summoned`，例如
`when own follower summoned where trait pixie { add storm to summoned; }`。
同类纹章按获得顺序触发；回合能力先于战场卡牌，开始阶段的纹章能力先于普通抽牌。
当前普通卡牌选择器不能查询或修改主战者区域，信仰及纹章消失效果尚未实现。

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
- `opponent`：`attack`、`clash` 能力中本次交战的另一随从；攻击主战者时为空。

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

同一方的随从和主战者可组成一次混合选择，例如触手撕咬：

```wbo
require target from oppo.field.followers or oppo.leader;
damage target 5;
heal own.leader 5;
```

两侧必须同为 `own` 或同为 `oppo`，候选按战场随从顺序排列，最后是主战者。
同样支持 `choose`、`random` 和末尾的 `count N`，不支持在混合来源上追加
`other`、`where` 或极值筛选。敌方随从仍受潜行、不能被选中等保护；敌方存在
`ability_target_guard` 时，主战者也不能成为玩家选择的目标。随机选择不受这些选择保护限制。
混合绑定支持不带筛选的 `damage` 和 `heal`；`count(target)` 包含主战者，
`count(target where ...)` 仅统计满足卡牌筛选的实例。破坏、增益及复制等
操作不能直接使用混合绑定，编译器会报告类型错误。

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

### 临时关键词

`add` 可在末尾追加 `until turn ends`、`until own turn ends` 或 `until oppo turn ends`。
前者持续到当前回合结束，后两者持续到能力控制者或对手的下一个回合结束；期限在赋予
时确定，不依赖能力来源之后是否还在战场。回合结束触发能力全部处理完毕后才到期。

```wbo
require target from oppo.field.followers;
set life target 1;
add cannot_attack to target until oppo turn ends;
```

临时赋予与永久能力独立：重复赋予不会覆盖另一个回合的期限，到期不会删除原生或
后来永久获得的同名能力。显式 `remove`、屏障抵消伤害或潜行解除时清除该能力的全部
赋予记录，旧期限不会影响随后新获得的能力。从战场返回手牌或牌组会清除临时记录；
手牌返回牌组则保留，到期仍会移除。设置生命值本身不随攻击限制一起到期。

期限修饰也适用于 `buff`；临时移除关键词仍会被显式拒绝。

### 临时属性增益

`buff` 使用相同的期限后缀，写在可选的 `where` 之后：

```wbo
when own follower summoned where trait officer {
    buff self +1/+0 until turn ends;
}
buff own.field.followers other +2/+1 where life <= 3 until own turn ends;
```

增量在执行时求值并累加；到期撤销该期限内的增量，不会将属性还原成某个旧快照，
因此永久增益、进化增益及之后受到的伤害会保留。不同玩家的期限独立处理。
`set life` 覆盖先前临时增益中的生命值部分，但不影响攻击力部分和之后的新增益。
返回与变身沿用卡牌状态重置规则；手牌返回牌组保留的临时增益仍按期失效。
到期不是伤害，不消耗屏障；所有到期属性统一更新后，生命值不大于零的战场随从
一并破坏，其谢幕曲处理完毕后才切换回合。结束阶段中的玩家选择会暂停整个流程。

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

`highest` 与 `lowest` 均支持 `attack`、`life`、`cost`，读取实例当前数值；费用和攻击力以零
为下限，攻击力和生命值只对随从有意义，其他卡牌不参加这两类比较。先应用 `other`
和 `where`；玩家选择还会先移除不能被选择的目标，再求极值。随机效果不受潜行、
灵气的目标选择限制。极值查询本身不消费随机数；后续 `random` 只在极值候选中抽取。
`highest attack count 2` 最多取两个并列最高者，不会补选攻击力次高者。
暂停时保存极值候选，恢复会重新核对候选合法性；查询预算不足不会使用部分结果。
`base.attack`、`base.life`、`base.cost` 则比较卡牌定义的原始数值，例如
`highest base.cost` 忽略费用修正；历史来源使用破坏时的卡牌身份，不受原实例后来变身影响。

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

`spellboost S N` 使集合或绑定 `S` 中每张手牌的魔力增幅能力按声明顺序发动 `N` 次。费用等
实例状态修改会保留在手牌实例上，并在费用降低时以 `minimum` 指定的下限截断。

`where spellboost` 匹配当前卡牌定义中声明了 `spellboost { ... }` 的对象，可与
`type`、`class` 等过滤器组合。它不匹配仅在入场曲中让其他手牌增幅的卡，也不匹配
已经变身为没有魔力增幅能力的卡牌。指定对象增幅可写为：

```wbo
require target from own.hand where spellboost;
spellboost target 1;
draw 1;
```

这张法术仍会自动使原有手牌增幅一次，因此选中的卡累计增幅两次，其他原有手牌
增幅一次，随后抽到的卡不参与这两次增幅。没有可选对象时，`require` 在支付费用前
拒绝出牌，参见[无法使用法术的官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#lm9y3xju9y0z)。

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

`summon copies of 集合` 为手牌或战场中的每个随从、护符召唤一个独立副本，原卡保留。
可用绑定、区域集合或 `self`，并可附加 `where` 筛选；法术和其他区域的对象不产生副本。
多个对象按集合的稳定顺序处理，战场满后停止。`summoned` 只包含实际创建的副本，
没有创建时也会清空，且可作为后续效果或下一次复制的输入。

```wbo
choose targets from own.hand where type follower and trait artifact and cost <= 5 count 3;
summon copies of targets;
buff summoned +1/+1;
```

复制继承当前费用、攻击力和生命值（包括已受伤害）、进化形态、关键词、计数器、
附加效果及其原有到期回合、融合材料记录；副本的可变状态与原卡相互独立。
本回合攻击次数和启动记录清零，随从重新受到入场当回合的攻击限制。
复制不会发动入场曲或进化能力，但会产生正常的进入战场事件。
超进化形态继承也符合[官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#2e2l68z79011)。
选择条件中的 `cost` 是当前费用：费用增加到 6 的创造物不满足 `cost <= 5`，
参见[当前费用筛选的官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#bxi2b429pl7)。

`summon random N from own.deck.followers|amulets [where ...] [distinct names]`
从自己的牌组召唤已有卡牌。`where` 读取当前属性，`distinct names` 要求本次召唤
各不相同的卡名；省略时允许同名的多张卡。

```wbo
fanfare {
    choose target from own.hand;
    discard target;
    summon random 3 from own.deck.amulets where cost <= 3 distinct names;
}
```

每张实体卡起初等概率参与抽选。选中一个名字后，`distinct names` 才从后续候选中
排除该名字的其他副本；这些未选副本仍留在牌组。若有 A 三张、B 两张、C 一张，
首次选中 A 的概率为 3/6；选中 A 后，下一次选中 B 的概率为 2/3。
这是[官方关于随机不同种类的 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#9jbr88t6fy6j)
给出的抽选方式。

操作开始时按战场空位限制数量，固定候选并完成抽选后，按抽选顺序将实体移到战场。
保留实例身份、费用和身材修正、附加能力及吟唱；剩余牌组顺序不变。随从重新进入
入场等待。它不支付费用、不增加连击、不算抽牌，也不发动入场曲；候选不足不导致
牌组耗尽败北。公开事件只显示成功入场的卡牌，不透露剩余牌组身份。
`summoned` 绑定成功入场的有序实例，空结果也覆盖旧绑定。只有候选数大于一时才消费
随机数，满场不抽选。入场监听在当前效果块完成后结算。

随从和护符分别产生自己的入场事件：`when own follower summoned` 不响应护符，
`when own amulet summoned` 响应使用、创建、复制、历史召唤和牌组召唤的护符。
护符启动是独立的 `engaged` 事件。

`replace own.deck|oppo.deck with shuffled 数量 card ID [, 数量 card ID ...];`
将指定玩家的整副牌组替换为列出的卡牌，并洗牌。每项数量与总数量均为 1..65535，
卡牌 ID 必须已定义；同一卡牌可重复列出。必须显式写 `shuffled`。

```wbo
fanfare {
    replace own.deck with shuffled 3 card 90004110, 3 card 90004120, 3 card 90004130, 1 card 90004310;
}
```

这个配方构成十张启示录牌组：沉默的魔将、深渊之主的仆从、边狱的邪祟各三张，
阿斯塔罗特的宣判一张。替换创建原始状态的新实例，不继承原牌组的费用、身材或附加能力。
它不算抽牌、召唤、破坏、消失或舍弃，不增加墓场资源，也不发动入场曲或谢幕曲。
手牌、战场和既有破坏记录保留；空牌组同样可以替换。

原牌组实体退出可操作区域，其已有绑定不再解析为可修改的目标，也不能返回手牌。
运行器仅在存档内部保留这些实体，用于核对旧引用和破坏记录。双方只收到新牌组数量，
不获得新旧牌组的实例列表或牌序。按配方展开后使用确定性的 Fisher–Yates 洗牌，
N 张牌消费 N−1 次随机决策；十张启示录牌组消费九次，单张牌组无需随机数。
新实例预算与查询预算先整体核验，预算不足不替换原牌组，也不消费随机数。

`summon random N from 破坏历史 [where ...] [highest|lowest 属性]` 从历史中抽取记录，
按对应卡牌定义召唤同名的新卡。新卡使用原始费用、身材、吟唱和固有能力，
不继承伤害、费用修正、额外关键词、计数器、进化或附加能力，也不获得亡者类型。
原实例和破坏记录保留。支持双方的随从与护符历史，以及可选的 `this turn` 回合窗口。

```wbo
lastwords {
    summon random 1 from own.destroyed.amulets highest base.cost;
}
```

每条破坏记录各占一个候选，同一实例被多次破坏也分别计入。同一条操作先固定筛选和
极值候选，再无放回地逐次抽选、召唤，`summoned` 按实际召唤顺序绑定。
记录不足或战场已满时停止，不补选低于极值的记录；空结果也会清空 `summoned`。
没有候选、仅剩一个候选或战场已满时不消耗随机数。它不发动入场曲，会产生正常入场事件，
监听能力在当前效果块完成后结算。历史不能作为普通选择或修改操作的目标。

`life` 与 `cost` 的比较右侧还可读取 `own`/`oppo` 的 `combo`、`pp`、`maxpp`、
`life`、`ep`、`sep`、`shadows`。必须显式写出玩家，且不支持算术或聚合表达式。
`own` 始终指能力来源的控制者；即使筛选对手牌组或监听对手入场，也不随候选对象改变。
没有能力来源的测试断言按测试席位解释。以下入场曲的连击包含正在使用的卡牌：

```wbo
fanfare {
    draw 1 from deck where type follower and cost == own.combo;
}
```

数值在该集合被筛选时读取。效果先确定完整目标集合，再执行修改；效果期间的资源变化
不会追加目标。事件监听在事件发生时筛选并决定是否入队；已入队的能力不会因后续资源
变化而取消。暂停后尚未执行的筛选则在恢复执行到该效果时读取最新数值。

`grant 对象 [where ...] { 能力 }` 给手牌或战场中的每个随从附加一个能力。
当前支持谢幕曲和回合开始、结束触发。每次赋予独立叠加，在原生能力之后按赋予顺序
发动；后来入场的随从不会自动获得。可用 `label` 提供卡牌详情中的本地化效果文本。

```wbo
require targets from own.hand where type follower and trait artifact and cost <= 5 count 2;
summon copies of targets;
grant summoned {
    label chs "对手的回合结束时，破坏本卡牌。";
    when oppo turn ends {
        destroy self;
    }
}
```

附加能力中的 `self` 是获得能力的随从，`own`、`oppo` 按该随从的控制者解释。
能力使用独立绑定作用域，不捕获赋予时的 `target`、`summoned` 或其他局部变量，
不能使用前置合法性检查 `require`，执行时需要选目标则使用 `choose`。
效果赋予后不依赖施法者继续在场。副本继承已获能力；变身和从战场返回手牌、牌组
会清除它们。重新召唤同名卡牌只使用原始定义，不继承旧实例的附加能力。
离场后取消尚未发动的场上回合监听；已经因破坏进入队列的谢幕曲仍会执行。

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

`sum(集合 [where 筛选], base.attack)` 对原始攻击力求和；`base.life` 和 `base.cost`
分别求原始生命值与费用之和。没有身材的卡牌对攻击力、生命值的贡献为零，主战者不参与求和。
空集合结果为零，每条历史记录独立参与。筛选读取当前实例属性或破坏记录属性，求和投影则
读取对应身份的卡牌定义，不计强化、伤害、进化加成或费用修正。
省略 `base.` 的 `sum(集合, attack|life|cost)` 改为读取效果执行时的当前属性，
包括强化、伤害、进化和费用修正；费用最低按零计算。历史集合读取破坏时保存的属性。
例如以下入场曲读取所舍弃卡牌的当前费用；空手牌跳过选择，空绑定之和为零。
舍弃产生的监听能力在整段入场曲结束后才执行。

```wbo
fanfare {
    choose target from own.hand;
    discard target;
    damage oppo.field.followers sum(target, cost);
}
```

```wbo
when self summoned {
    buff self +sum(own.destroyed.followers this turn where trait shikigami, base.attack)/+sum(own.destroyed.followers this turn where trait shikigami, base.life);
}
```

`this turn` 位于历史集合之后、筛选之前，只统计当前回合发生的破坏；回合开始阶段的
倒计时和谢幕曲结算也属于新回合。上例只在本随从入场时触发，包括由能力召唤入场；
其他随从入场以及战场上的变身都不会触发。两项增量在本条强化操作开始时分别读取，
暂停恢复保留破坏记录的回合归属。测试初始历史不计入本回合。
计数不消耗随机决策；查询预算耗尽时不使用部分结果执行效果。
数值也可直接读取 `own`/`oppo` 的 `combo`、`pp`、`maxpp`、`life`、`ep`、`sep`、`shadows`、`hand_count`、`earthsigils`，
或者读取 `self.cost`；随从还可读取 `self.attack` 和 `self.life`。
`own` 始终相对能力控制者，`self` 为发动能力的卡牌实例。费用支付和使用卡牌的连击计数
在入场曲之前发生，因此入场曲中的 `own.pp` 已扣除费用，`own.combo` 包含本卡牌。
`hand_count` 是手牌张数；`earthsigils` 是己方或对方战场上土之印的总层数，
不是土之印实例数。二者均只读，不能用 `gain` 修改；读取土之印层数不会消耗土之印。

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

攻击力内部允许保留负值，后续增减、进化和临时效果到期都在该值上计算。
例如内部攻击力为 -2 时，获得 +1/+1 后内部攻击力为 -1，显示仍为 0。
`self.attack`、`sum(..., attack)`、攻击力极值选择、战斗伤害和网页显示均以零为下限；
求和先对每个随从取不小于零的攻击力，负值不会抵消其他随从的贡献。复制和续局保留内部值。
伤害和回复量的下限同样为零。
负攻击力参与后续增益计算的说明见 [官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#2mrx5ewte1)。
当前不支持计数嵌套、一般算术、绑定对象的数值字段，或将动态数值用于抽牌数量等其他操作。

`own.attacked_this_turn` 和 `oppo.attacked_this_turn` 可直接作为 `if` 条件，
在前面加 `not` 表示本回合尚未有对应玩家的随从宣告攻击。这里的 `own` 始终指能力控制者。

```wbo
when own turn ends if not own.attacked_this_turn {
    random target from own.field.followers;
    buff target -2/-0;
    add ward to target;
}
```

合法攻击一经声明即计入，包括攻击力为零、攻击主战者以及攻击者随后离场的情况。
被拒绝的攻击不计入；不沿用上一位玩家回合的记录，玩家下次回合开始时重置。
该条件也可放在效果块的 `if ... { ... } else { ... }` 中。

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

`attack` 在宣告攻击后执行，随后才依次处理攻击方、防守方的 `clash`，最后造成战斗伤害。
`opponent` 是该次攻击绑定的实例，无需再次选择，不受效果目标选择保护影响；
在防守方的 `clash` 中，它指向攻击方。攻击主战者时，`attack` 中的该绑定为空，且不发动 `clash`。
例如“攻击随从时，破坏交战对手”可写为：

```wbo
attack {
    destroy opponent;
}
```

若防守随从已被该能力破坏，则不再发动其交战能力，也不互相造成战斗伤害，
参见[攻击时与交战时的官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#o74bzd_mnn)。
绑定可供能力内的条件、重复或模式块使用，并随暂停续局保存；不会自动传给其他独立能力。

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

条件判断可直接读取能力来源的当前形态：

```wbo
when own turn ends {
    if self form super_evolved {
        heal own.leader 4;
        add barrier to self;
    } else {
        heal own.leader 2;
    }
}
```

`self form unevolved` 只匹配进化前随从，`self form evolved` 包含普通进化和超进化，
`self form super_evolved` 只匹配超进化；非随从不匹配任何形态。条件在执行到 `if` 时
求值，因此同一能力之前造成的进化会影响后续判断。

判断是否到了进化解禁回合使用 `own.evolve_unlocked`、`own.superevolve_unlocked`，
也可将 `own` 换成 `oppo`。两者只检查对应玩家的回合门槛，分别是先手第 5/7 回合、
后手第 4/6 回合；即使点数用尽或本回合已经进化，解禁条件仍然成立。

```wbo
fanfare {
    if own.superevolve_unlocked {
        add bane to self;
    }
}
```

这里的条件只在入场曲执行时判断一次，不会在以后到达门槛时自动赋予能力。
规则测试未提供先手信息时，解禁条件按 `own` 为先手、`oppo` 为后手计算；
测试中手动进化动作原有的门槛豁免不会令卡牌条件提前成立。

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

`set maxlife own.leader|oppo.leader N;` 设置指定主战者的生命上限，N 为 1..65535 的整数。
当前生命高于新上限时降至上限；提高上限不会增加当前生命。这不是伤害或回复，不触发
对应监听，也不导致主战者在上限变为 1 时直接败北。之后的回复仍受新上限约束。
例如阿斯塔罗特的宣判写作 `set maxlife oppo.leader 1;`。

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

多个已选定目标可合并为一次破坏，例如死神挥刀：

```wbo
require ally from own.field.followers;
require enemy from oppo.field.followers;
destroy ally, enemy;
```

连续的独立 `require` 会在支付费用前一起检查；任一候选不足，整个动作非法。
实际选择依次请求玩家响应，两次选择之间没有破坏。逗号列表只允许已定义的实例绑定
或 `self`，不能附带筛选、区域集合或混合主战者绑定；同名目标不可重复书写。
不同绑定包含同一实例时自动去重。所有有效目标进入同一死亡批次，依当前回合方、
另一方的战场顺序处理，`destroyed` 保存该批实际被破坏的目标。
超进化保护仅阻止破坏，不阻止选择：自己的回合中可选自己的超进化随从并保留它，
对方被选中的随从照常破坏。

```wbo
fanfare {
    destroy own.field.amulets;
    damage oppo.field.followers count(destroyed);
    damage oppo.leader count(destroyed);
}
```

这里两条伤害读取相同的破坏张数，新触发的谢幕曲等待入场曲结束后结算。
`destroyed` 保存实例集合，不是墓场总数；`own.destroyed` 表示只读的破坏历史集合。
历史按每次破坏独立保存卡牌身份、当时费用、攻击、生命、进化形态、亡者类型和关键词。
原实例返回手牌、变身或再次被破坏，不会改写旧记录；历史也不会泄露该实例后来的隐藏身份。
`count(own.destroyed where type follower)` 按历史记录统计，重复破坏不会去重。
`count(own.destroyed.followers where keyword ward)` 统计破坏时拥有守护的随从记录，
包括当时仍有效的临时守护；之后到期或移除关键词不改写历史。
历史集合不能用于 `choose`、`require`、`random` 或修改卡牌的操作；需要移动墓场中的
存续实例时应使用 `own.graveyard`。本条 `destroy` 产生的 `destroyed` 绑定仍指向实际实例。
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

普通效果也可使卡牌集合或选择后的绑定变身：

```wbo
fanfare {
    transform own.hand into card 90014310 where class forestcraft and cost <= 2;
}
```

筛选在变身前按当前职业、种类与费用一次性求值。例中包含满足条件的随从、护符和法术，
不影响其他区域或对手手牌；减费后的高费卡可满足条件，加费后超过门槛的卡不会变身。
`transform target into card C;` 使用既有绑定；需要玩家选择时先写 `choose` 或 `require`。
可以变身手牌、牌组与战场上的存续卡牌；主战者与历史区域不是合法集合目标，
已经离开这些区域的绑定会被跳过。战场卡牌不能变身为法术。

变身保留实例身份、区域顺序和附着材料，重置费用、伤害、附加能力、进化形态、命名计数器、
攻击与启动记录、每回合触发次数。新的场上随从重新获得入场等待，疾驰或突进仍可赋予攻击资格。
旧的场上或手牌监听从索引中移除，尚未开始的事件监听取消；新身份的监听立即登记，
但变身本身不产生入场或破坏事件。暂停恢复保留新身份与后续指令位置。

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

不同身份的召唤按卡牌文本顺序分别书写，后续强化使用区域查询覆盖已在场和新召唤的随从：

```wbo
fanfare {
    summon 1 card 90054110;
    summon 1 card 90054120;
    necromancy 6 {
        buff own.field.followers other +2/+0;
    }
}
superevolve {
    repeat 2 {
        reanimate 1;
    }
}
```

只剩一个空位时，先写的召唤占用该位置；召唤未能入场不会中止后续效果。
`other` 仅排除能力来源实例。亡者召还不消耗破坏记录，两次结算可召唤同一身份；
每次按原始费用选择，每条破坏记录各占一个随机候选，场地已满时不消耗随机数。
原始费用来自每条记录的破坏时卡牌定义，不使用当时的费用修正或该实例后来变身后的定义。
新实例具有原始身材与固有能力并获得亡者类型，其入场监听在当前能力完整结束后依次执行。

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

场上事件监听也可以显式限制为在手牌中发动：

```wbo
when own follower leaves field while self in hand {
    reduce cost self 1 minimum 0;
}

when own card fused {
    random target from oppo.field.followers;
    damage target 2;
}
```

`while self in hand` 指监听来源必须位于手牌；省略时为战场，也可写
`while self in field`。该条件写在事件模式之后、`where` 之前。`where` 仍筛选事件对象。
监听来源离开指定区域或变身后，取消其尚未开始执行的监听能力；恢复续局会重建这些监听。
每位玩家先按战场顺序、再按手牌顺序收集，回合玩家优先，同一卡牌内按能力声明顺序收集。

`follower leaves field` 包含破坏、消失和返回手牌或牌组，不包含变身，也不包含护符离场。
离场前按当时的区域、种族与属性收集监听，`left` 绑定该实例；随后的状态重置不会重新筛选。
当前能力完成前不执行新监听，批量破坏中每个随从分别触发一次。
返回手牌会触发减费，参见[官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#4vvbwaghp)。

`card fused` 每次成功融合触发一次，不按素材张数重复。本体融合能力先完整执行，再执行监听；
非法素材响应和重复融合不触发。融合来源和材料的身份仍按手牌信息隐藏。

## 生命回复

`heal` 可回复主战者或场上存活的随从，例如 `heal own.field.followers 3;`。
回复只消除已受到的伤害，最多回复至当前生命上限；满生命、零回复量及非战场随从不会产生
回复事实。生命增益、减益、进化和临时数值到期会同时调整当前生命与上限，回复不能撤销减益。
`set life` 同时重设当前生命与上限；变身、从战场返回手牌也会清除旧伤害，复制则保留独立的
伤害状态。致命伤害后生命已为零的随从不能通过回复逃过破坏。

随从回复使用同一 `healed` 事实，目标为实例，`Actual` 是实际回复量；它不会触发主战者回复监听。
回复多个随从时按集合顺序记录事实，完成当前能力后再结算监听。攻击时的回复先于交战伤害。

## 主战者回复事件

`when own leader healed` 在己方主战者实际恢复生命值后触发，`oppo` 对应对手。
能力回复与虹吸共用此事件，每次回复只产生一次事件，不按回复点数重复。
零回复或已达生命上限时不产生事件，也不消耗每回合限次。
`healed` 绑定此次接受回复的主战者，仅在对应监听器的效果体内有效。
事件头部可使用来源区域、每回合限次和 `if` 条件；不接受卡牌 `where` 筛选。

```wbo
when own leader healed once per own turn {
    damage healed 1;
}
```

上例仅在能力持有者自己的回合响应，其他回合的回复不消耗次数。
成功入队即占用次数，因此同一能力连续回复两次也只触发一次。
正在执行的能力先完整结算，再处理回复监听；暂停恢复保留主战者绑定与已用次数。
纹章的回合开始伤害先于普通抽牌结算；若主战者因此败北，不再抽牌或接受行动，
参见[官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#lbe2tcmqi4)。

## 破坏事件与关键词筛选

`when own follower destroyed` 监听己方随从的实际破坏；`oppo` 对应对手，
`amulet destroyed` 对应护符。战斗、伤害与效果造成的破坏都适用；消失、返回、变身、
舍弃和手牌溢出不会触发。`when self destroyed` 不受支持，自身谢幕曲使用 `lastwords`。

```wbo
when own follower destroyed where keyword ward {
    buff self +1/+1;
}
```

`where keyword K` 检查实例当前拥有的关键词，包括临时或额外赋予的关键词。
它可用于事件、选择、集合操作和历史统计，并可与 `and`、`or` 组合。
`K` 使用固有关键词的标识，例如 `ward`、`barrier`、`storm`；入场曲、谢幕曲、
吟唱和土之印层数不是这个筛选器的关键词。

同批死亡先整体离场，再逐个产生破坏监听并排入谢幕曲。随从在同批死亡中已经离场，
不能靠自己的破坏监听强化而存活。事件块的 `destroyed` 绑定本次被破坏的实例，
其含义不同于 `own.destroyed` 历史集合。入队时完成筛选，后续关键词变化不重新筛选；
监听来源离场或变身会取消其尚未开始执行的能力。
被破坏实例通过绑定从墓场返回手牌或牌组时，恢复原始费用、身材与固有关键词，
清除进化、临时状态和额外赋予；消失区返回同样处理。原有破坏记录保持不变。

## 事件触发条件

事件头部可在 `where` 之后添加 `if`，在事件产生、能力入队时判断条件：

```wbo
when own turn ends if own.hand_count <= 5 {
    draw 1;
}
when own turn ends if own.hand_count >= 6 {
    heal own.leader 1;
}
```

结束回合时有五张手牌，只会排入抽牌能力；抽到第六张后不会追加回复能力。
同一事件先执行的其他纹章即使改变手牌，也不会改变已经确定的触发资格。
暂停和恢复保留已排队能力，不重新判断头部条件。能力体内的 `if` 则在执行到该语句时判断。
结束回合时统一判定触发资格的机制，参见[官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#75wdwp8ysf)。

头部条件支持玩家数值、已声明的自身计数器、溢出、进化形态和进化解禁条件；
不支持事件对象绑定或 `fused` 数值。自身事件中仅 `when self survives damage` 接受头部条件；
其他自身事件与 `grant` 内的事件暂不接受头部条件。条件不成立时不消耗每回合发动次数。

## 受到伤害后存活

```wbo
when self survives damage during own turn {
    draw 1 from deck where class dragoncraft and type follower;
}
when self survives damage once per own turn {
    random target from oppo.field.followers count 1;
    damage target 3;
}
when own follower survives damage once per own turn {
    add 1 card 90044320 to hand;
}
```

`when self survives damage` 仅用于随从。伤害应用后该随从仍有生命时，将能力排入队列；
当前能力或战斗检查点先完成，来源在能力发动前离场会取消等待中的监听。致命伤害及
本次交战中被必杀破坏的随从不发动此能力。

攻击力为 0 的交战也产生伤害事实；伤害因屏障、超进化保护或减免变为 0 时，同样可满足
存活事件。原始伤害为 0 时不消耗屏障。参见[零攻击交战 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#10nvv2q1zaop)
与[超进化保护 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#v-6amcbgd2vc)。

`during own turn` 仅允许自己的回合，`during oppo turn` 仅允许对方回合，省略则双方回合
均可发动。此范围不会限制次数；需要限次时使用既有的 `once per own turn` 等语法。
后者同时限定回合与次数，因此通常无需再写相同的 `during`。也可在末尾用 `if` 声明
事件发生时检查的条件。自身模式不接受 `where` 或来源区域声明，正文直接使用 `self`。

`when own follower survives damage` 与 `when oppo follower survives damage` 分别监听己方与对方
随从，可声明在纹章上，正文用 `damaged` 引用受伤随从。它们支持玩家侧事件的来源区域和
`where` 筛选器，筛选读取受伤后的状态；存活条件检查受伤随从，而非监听器来源。
每回合次数按监听器实例与能力记录，因此同一纹章看到多名随从同时存活也只生成一次卡牌，
多名嘉尔缪则各自保有一次反击。纹章优先于场上随从排队；反击在执行时随机选择仍在场的敌方
随从，没有目标仍消耗本次发动次数。

## 每回合限次

事件能力可在区域条件之后、筛选器之前声明每回合限次：

```wbo
when own follower summoned once per own turn where trait puppetry {
    add bane to summoned;
}
```

`once per own turn` 仅在持有者回合发动，每个持有者回合限一次；对方回合不发动也不消耗次数。
`once per oppo turn` 仅在对方回合发动；`once per turn` 在任一玩家的每个回合各限一次。
不写限次条件则保留每次事件都可触发的行为。其他自身专用事件及 `grant` 暂不接受限次条件。

次数按卡牌实例与能力分别记录，在事件通过区域、玩家侧与对象筛选并成功入队时消耗。
同一效果先后召唤多个符合条件的随从时，只有第一个事件占用该能力的次数；当前效果完整执行后
才结算监听。多个来源或多条限次能力分别消耗，效果落空也不返还次数。
“自己的每回合”仅包含持有者回合，参见[官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#a7wdp3yujp)。

换回合时，在倒数处理与回合开始能力前清空次数。进化保留记录，返回手牌或牌组、变身重置记录；
新复制实例有独立的未使用次数。暂停、保存和恢复保留次数，不会因为重新连接而再次发动。
对局与录像卡牌详情显示回合范围和已发动次数；隐藏手牌的记录不会发送给对手。

## 确定性

规则不能读取文件、网络状态、系统时间或外部随机源。所有随机操作都必须使用
规则引擎持有的、带种子的随机数生成器。在锁定相同规则集版本的前提下，玩家
指令、选择结果和初始随机种子必须足以完整重放一场对局。规则集必须锁定随机
算法及排序策略；只有种子而没有规则集标识不足以保证跨实现重放。
