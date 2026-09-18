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

「信仰」是卡片可以声明的永久实体（与纹章共用主战者区域，最多五个），
开局时按初始牌组里出现的信仰定义自动放置，信仰值放在 `counter value` 里：

```wbo
card 10664120 {
    effect {
        when own turn ends {
            faith 10 {            // 信仰值-10 后再执行；不足则整块跳过
                add 1 card 90064320 to hand;
            }
        }
    }

    faith {
        counter value 0;
        when own amulet destroyed {
            add 1 counter value;   // 信仰值 +1
        }
        locale chs { name "信仰：…"; text "…"; }
        // eng/jpn/kor/cht 同样必须齐全
    }
}
```

`faith N { … }` 与 `earthrite`/`necromancy` 同属资源支付块：信仰不存在或信仰值不足时
整块不执行（不会扣值，也不会产生块内效果）；信仰值由信仰自身的事件监听增长。
测试里可以用 `crests { faith 别名 = 卡牌ID { counter value N; } }` 直接放置信仰。
需要把信仰值当数值分配时用
`distribute faith <卡牌ID> { option 1 { … } option 2 { … } … }`：
信仰值会**逐点**独立等概率分给某个选项（每点消费一次随机数），再按选项编号顺序
为每个分配到的点执行一次能力；信仰值本身不消费。
「使自己的信仰获得「…」」写成 `grant faith { when … { … } }`：`faith` 指**本卡牌定义的信仰**，
附加的事件监听与随从的 `grant` 一致（可以是任意事件，含 `where` 筛选），之后的事件会照常触发它。

`damage_reduction N` 使该随从每次受到的伤害减少 `N`，实际伤害最低为零。
在效果块中使用 `set_damage_reduction TARGET N` 可在结算时动态设置目标的固定减伤值；
`N` 必须为非负整数，并从下一次伤害结算开始生效。
`damage_cap N` 给该随从加上单次伤害上限：每次受到的伤害最多为 `N`，对应卡面
"受到的 N+1 点或以上的伤害变为 N 点"（如"受到的 4 点或以上的伤害变为 3 点"写作
`damage_cap 3;`）。上限在伤害减免之后应用，最低仍为零；它只作用于该实例，不影响主战者。
主战者的"受到的1点或以上的伤害变为0点"写作给主战者关键词 `damage_to_zero`：

```wbo
enhance 10 {
    add storm to self;
    set maxlife own.leader 1;
    add damage_to_zero to own.leader until oppo turn ends;
}
```

主战者关键词同样支持 `until own|oppo turn ends`：到期时点在对应一方的回合结束时清除，
只影响该关键词，不会动到同一位主战者的其他关键词。【屏障】与 `damage_to_zero`
同时存在时，归零不会消耗【屏障】（伤害本来就是 0）。
`stealth` 使敌方效果的主动目标查询和攻击目标查询忽略该随从；范围效果和随机效果仍可作用于它。
拥有潜行的随从声明攻击，或通过能力造成实际伤害时解除潜行。
`aura` 仅使敌方效果的主动目标查询忽略该随从；它不会影响攻击，也可能成为随机效果的目标。
`attack_limit N` 使该随从每回合最多可以进行 `N` 次攻击，`N` 必须为正整数。
在效果块中使用 `set_attack_limit self N` 可在结算时动态设置自身上限，适合爆能强化或进化效果；
该值同样必须为正整数，并从下一次攻击合法性检查开始生效。
`cannot_attack` 使随从无法攻击；`cannot_attack_follower` 和 `cannot_attack_leader`
分别只禁止攻击随从或主战者。攻击限制会从合法动作集合中移除对应动作，直接提交时也会被拒绝。

`ability_destruction_guard` 表达「不会被能力破坏」：能力造成的破坏（`destroy` 操作、
附加的「回合结束时破坏本卡牌」等）会跳过该随从。战斗规则造成的破坏不受影响——
生命归零与【毁灭】照样让它离场。能力可通过
`add ability_destruction_guard to T` 与 `remove ability_destruction_guard from T` 修改。
主战者也可以获得关键词：`add barrier to own.leader;` 让本方主战者获得【屏障】——
下一次受到的伤害降为 0 并消耗掉（官方 QA 明确：与"受到的伤害 +1"同时存在时也是 0）。
`add damage_taken_up to oppo.leader;` 让该主战者获得「受到的伤害 +1」：之后每次受到的
伤害在其它修正之前先加一（包括随从攻击与效果伤害）；它与【屏障】同时存在时，下一次
伤害仍被【屏障】整体降为 0。

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
`add 1 counter NAME modulo M;` 在加上增量后按 M 取模，用于"按顺序循环发动以下 1 个能力"
这类每次触发前进一格、走完回到第一个的写法：

```wbo
counter step 0;
when own amulet destroyed {
    if self.counter.step == 0 {
        random victims from oppo.field.followers count 2;
        damage victims 3;
    } else {
        if self.counter.step == 1 {
            heal own.leader 2;
        } else {
            summon 1 card 90061110;
        }
    }
    add 1 counter step modulo 3;
}
```
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
- `added`：最近一次 `add 1 card X to hand` 实际进入手牌的实例（手牌已满被丢弃的不计入），
  用于"加入手牌后立即修改"的文本，例如 `add 1 card 90071110 to hand; buff added +3/+0;`。
- `destroyed`：最近一次 `destroy` 操作实际破坏的目标集合。
- `returned`：最近一次 `return` 操作实际返回 `hand`/`deck` 的实例集合，
  用于"抽取X张卡牌，X为因本能力返回牌组的张数"这类文本（`draw count(returned);`）。
  本来就已经在目标区域、没有真正移动的实例不计入。
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

`first <绑定> from <集合> [where …] [count N]` 不询问玩家、也不消费随机决策，
按集合顺序取前 N 个（`count` 省略时为 1）。集合顺序就是区域顺序：战场按入场顺序、
手牌按获得顺序，因此它正好表达"从左起的第 N 个／前 N 张"：

```wbo
first ally from own.field.followers where class swordcraft;
set_attack_limit ally 2;

first copies from own.hand count 3;
add copies of copies to hand;
```

候选不足 N 个时只绑定现有的那些，空集合绑定空集合；操作对象为空则什么都不做。

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
双方的战场随从加双方主战者写作 `field.followers [other] or leaders`，
其中 `other` 排除来源实例（"其他随从或自己的主战者或对手的主战者"）：

```wbo
repeat 3 {
    random victim from field.followers other or leaders;
    damage victim 7;
}
```

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

这条限制只适用于**法术**与**启动能力**（`engage`）。随从与护符即使没有可选对象
也能正常打出，能力照常结算——[官方 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#q2lo-vv80cvw)
写明"随从和护符在没有可选择手牌时依旧可以使用，能力也会发动；法术则无法使用"，
而[卡西乌斯那条 QA](https://shadowverse-wb.com/chs/usersupport/?tab=2#49odoxq3_z)
进一步给出随从的例子：手牌里没有创造物·随从时照样能打出『向往天空的回归者·卡西乌斯』，
对对手战场造成 0 点伤害。因此随从、护符的入场曲、爆能强化、进化时、超进化时、
谢幕曲与触发能力里的选择一律写 `choose`：有候选时必须选，没有候选时绑定 `none`
并继续结算后面的语句。法术顶层的必选目标仍写 `require`，没有目标时整张卡不能打出。

`if skybound_art { … }` 是【奥义】条件（奥义槽 ≥ 10），`if super_skybound_art { … }` 是
【解放奥义】（奥义槽 ≥ 15）。奥义槽 = 当前回合数 + 本卡牌在手牌中时己方随从进化过的次数
（官方术语表：Skybound Art / Super Skybound Art）。`gain skybound 集合 N;` 让集合里的
卡牌奥义槽 +N（"使自己的所有手牌的奥义槽 +1"）。

筛选器里的 `where base.cost == 2`（`base.cost`/`base.attack`/`base.life`）按原始定义比较，
与按当前值比较的 `cost` 区分开。

`where card <绑定>` 匹配与那个绑定实例**同一卡牌定义**的对象（绑定名在运行时解析；
写错名字不会匹配到任何目标，检查器不校验筛选里的绑定名）："使对手场上与其同名的所有随从消失"
写作 `banish oppo.field.followers where card target;`，配合 `superevolve extends evolve`
就能沿用普通进化那次选择的目标（两步共享同一绑定帧）。

筛选器里的 `where cost changed` 匹配"费用被效果改过"的卡牌（加费、减费与设置费用都算），
配合 `when own card played` 表达"自己使用费用发生变化的随从时"。

`where attacked this turn` 与 `where not attacked this turn` 按实例本回合已经进行的攻击
筛选（回合开始时清零），用于"本回合中没有进行过攻击的进化前随从"这类文本。
`where not <词条>` 可以取反单个筛选词条，目前支持 `trait`、`type`、`class`、`form`、
`keyword` 与 `damaged`（例如"非侵蚀者随从"写作 `where not trait encroacher`）；
其它否定写法仍会被检查器拒绝。

`if own.hand has 4 same cost { … }` 判断某区域里是否存在 4 张以上**当前费用**相同的卡牌
（`hand` 与 `deck` 可用），用于"若自己的手牌中有4张或以上费用相同的卡牌"。

数值位置除了 `self.attack`、`count(...)`、`sum(...)` 与玩家数值，还可以写
`<绑定>.attack|life|cost`，读取该绑定里第一个实例的当前数值，例如
"X 为选择的随从的攻击力"写作 `require target from own.hand.followers where trait artifact;
damage oppo.field.followers target.attack;`。绑定名写错不会报错，求值为 0。
加 `base.` 读取卡牌定义里的原始数值：`<绑定>.base.attack|life|cost`，
例如"X 为使用的卡牌的原始费用"写作 `played.base.cost`（`played` 是
`when own card played` 监听里指向被打出卡牌的绑定）。比较右侧同样可用这些标量，
因此"战场上存在另一张与该卡牌原始费用相同的卡牌"写作
`count(field other played where base.cost == played.base.cost) >= 1`。
`raise countdown <集合> N` 与 `reduce countdown <集合> N` 互为反向，可用来推进或延后
护符与纹章的吟唱；`set_attack_limit <目标> N` 把"1回合可以攻击 N 次"给到任意随从，
写 `self` 时就是本随从。

`own.deck has no duplicates` / `oppo.deck has duplicates` 判断牌组里是否有重复的卡牌定义；
`banish duplicates in own.deck;` 让牌组中的重复卡牌消失，只保留每种的第一张。

纹章不在战场上，普通的目标解析（`self`、`field.followers` 等）看不到它们。
需要操作纹章时必须显式写 `own.crests` / `oppo.crests`：例如
`destroy own.crests where card 10453310;` 破坏自己的某个纹章（走纹章退场并触发谢幕曲），
`reduce countdown own.crests 1;` 推进自己所有纹章的吟唱。显式集合之外的写法则
仍然解析不到纹章，纹章也不会被当成普通卡移动或变身。

`count` 后面可以跟数值表达式（例如 `count own.crests`、`count(own.field other)`）：
动态数量在结算到该语句时求值，为 0 时不选任何目标。

筛选条件写在目标集合之后，`other` 用于排除 `self`；写成 `other 绑定名` 时改为排除
该绑定指向的实例，例如【攻击时】里的"非交战对手的随从"写作
`random victim from oppo.field.followers other opponent;`。

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
主战者关键词的期限独立于随从：`add damage_to_zero to own.leader until oppo turn ends;`
在对手的回合结束时清除，不会影响同一位主战者上的其他关键词。

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

末尾加 `distinct names` 表示"抽取 N 种…"：每抽中一张就排除同卡名的其余候选，
同名副本不会被抽第二次，因此实际张数上限是候选里不同卡名的数量。

```wbo
superevolve {
    draw 2 from deck where type spell and cost == 1 distinct names;
}
```

它只能与筛选搭配（不能写 `draw all … distinct names` 或裸 `draw 2 distinct names`）。

`draw N for oppo` 让对方抽 N 张，从对方的牌组顶部抽取并产生对方的抽牌事件；
省略 `for` 时等价于 `for own`。"让对方抽牌"必须用这条语句表达：`add ... to hand`
创建实例但不产生抽牌，两者的触发与牌组耗尽行为不同。
筛选不足只抽取现有匹配项，不因未找到目标而败北。所有抽牌事件公开数量，隐藏手牌
身份；超出手牌上限的卡牌进入墓场，但不加入供后续能力使用的 `drawn` 绑定。

抽牌张数也可以是数值表达式，例如按本次返回牌组的张数抽牌写作
`return own.hand to deck; draw count(returned);`（`returned` 见[来源与目标](#来源与目标)）。
动态数量与 `draw N` 形状一致：仍可接 `for own|oppo` 与 `from deck where …`。

```wbo
draw 2;
draw all from deck where card 10022120;
draw 1 from deck where type follower;
draw 1 for oppo;
draw 2 for oppo from deck where type spell;
draw count(returned);
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
remove lastwords from summoned;
remove all abilities from targets;
```

`remove lastwords from 集合` 让选中的实例失去【谢幕曲】，`remove all abilities from 集合`
让它同时失去固有关键词、附加能力与卡面声明的触发能力。失去的能力只属于该实例：
同一个卡牌定义召唤出的其它实例不受影响，离场后也不复原。需要阻止衍生体无限触发
谢幕曲时（例如腐臭的僵尸），在召唤同一个块里对它执行本条操作。

`summon copies of 集合` 为手牌或战场中的每个随从、护符召唤一个独立副本，原卡保留。
可用绑定、区域集合或 `self`，并可附加 `where` 筛选；法术和其他区域的对象不产生副本。
复制只接受仍留在手牌或战场的目标，因此"使其消失，召唤对应数量的复制随从"要写成
`summon copies of target; banish target;`（先复制再消失，净效果与文本一致）。
多个对象按集合的稳定顺序处理，战场满后停止。`summoned` 只包含实际创建的副本，
没有创建时也会清空，且可作为后续效果或下一次复制的输入。

`summon <绑定>;` 把已经存在于手牌中的对象直接放到战场：它不发动入场曲，随从获得入场等待，
仍然发出 `summoned` 事件并消耗一个战场空位。典型写法是
`choose target from own.hand.followers; summon target;`（"召唤手牌中的该随从"）。
`add N card C to deck;` 把新卡以随机位置插入牌组（不视为抽牌，输出仍是 `added`）；
`add copies of <集合> to deck` 同理。`transform <集合> other into card C;` 可以排除来源实例，
用于"使战场上的其他所有随从变身"。
`summon random 1 card A or card B [for own|oppo];` 从几种指定卡牌定义中随机召唤
（每次抽取消费一次对局随机数），用于"召唤随机1个『A』或『B』"这类文本；
推导卡不发动入场曲，池内至少两种、至多十六种互不相同的卡牌。
一次结算里发生的所有召唤都会累加到 `summoned_all`（`summoned` 仍然只保留最近一次），
因此"爆能强化_7：使其获得【疾驰】"指向同一次入场曲召唤的多个衍生体时写作
`enhance 7 { add storm to summoned_all; }`。
`destroy` 与 `banish` 分别把本次操作实际处理的实例写入 `destroyed` 与 `banished`，
可用 `count(banished)` 读取"因本能力消失的卡牌的张数"（例如作为分配伤害的总额）。
`add copies of <集合> to hand;` 则按每个目标当前的卡牌定义复制一张同名卡加入手牌
（手牌满时按过抽处理，不计入 `added`）；复制可以作用于已经消失的实例，
因为只读取卡牌身份，所以"使其消失，将1张同名的卡牌加入自己的手牌"写作
`banish target; add copies of target to hand;`。

```wbo
choose targets from own.hand where type follower and trait artifact and cost <= 5 count 3;
summon copies of targets;
buff summoned +1/+1;
```

`summon N card X for own|oppo;` 在指定一方的战场上召唤（省略 `for` 时是 `own`）。
"在对手的战场上召唤2个『骑士』"写作 `summon 2 card 90021110 for oppo;`；
召唤出的随从属于那一方，入场事件、战场容量与 `summoned` 绑定都按该方结算。

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

`summon random N from 破坏历史 [where ...] [highest|lowest 属性] [distinct names]` 从历史中抽取记录，
按对应卡牌定义召唤同名的新卡。新卡使用原始费用、身材、吟唱和固有能力，
不继承伤害、费用修正、额外关键词、计数器、进化或附加能力，也不获得亡者类型。
原实例和破坏记录保留。支持双方的随从与护符历史，以及可选的 `this turn` 回合窗口。

把同一条记录**复制**到别的区域写作
`add random N copies from 破坏历史 [where ...] [highest|lowest 属性] [distinct names] to hand|deck;`：
抽选方式与历史召唤一致（每次消费一次随机数、`distinct names` 排除同名记录），
但生成的是放进手牌或牌组的新实例，而不是召唤到战场，输出绑定是 `added`。
与牌组召唤一样，`distinct names` 要求本次召唤的卡名两两不同：每抽到一张后，
后续抽取都排除同名的其余记录（同名记录不消耗、也不重复召唤），候选耗尽时少召唤。
官方 QA 对这种"随机 2 种各 1 张"的写法给出的结算顺序正是"先等概率抽第 1 张，
再从与它不同种类的剩余候选中抽第 2 张"，因此重名卡越多越容易被选中。

```wbo
lastwords {
    summon random 2 from own.destroyed.amulets where base.cost <= 2 and lastwords distinct names;
}
```

同一个写法也接受**任意集合**作为来源，用于"复制对手手牌/牌组里的随机几张"：

```wbo
add random 1 copies from oppo.hand to hand;
add random 5 copies from oppo.deck to hand;
```

抽选方式与历史复制一致（每条候选消费一次随机决策、不放回，只剩一条候选时不消费），
但候选是来源区域里的**实例**，复制的是它们当前的卡牌定义：不继承伤害、费用修正、
额外关键词或附加能力，也不会移动来源实例。这类复制是**非公开**的——只读取卡牌定义，
不产生带来源身份的公开事件，对手只能看到手牌张数。加进手牌后可以照常用
`reduce cost added N` 之类的语句修改复制体。

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
两个数值表达式之间还可以相减，用于"X 为对手的战场上的随从数减去自己的战场上的随从数"：

```wbo
fanfare {
    random victims from oppo.field.followers count count(oppo.field.followers) - count(own.field.followers);
    destroy victims;
}
```

减法可以连写（从左向右结合），操作数必须是数值表达式：字面量直接写数字，
`count(A) - 1` 这样的写法会在检查阶段报错。结果可能为负，取用它的操作
（选择数量、伤害量等）按零处理。
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
数值也可直接读取 `own`/`oppo` 的 `combo`、`rally`、`crests`、`pp`、`maxpp`、`life`、`ep`、`sep`、`shadows`、`hand_count`、`earthsigils`、`entered_artifacts`，
或者读取 `self.cost`；随从还可读取 `self.attack` 和 `self.life`。
`own` 始终相对能力控制者，`self` 为发动能力的卡牌实例。费用支付和使用卡牌的连击计数
在入场曲之前发生，因此入场曲中的 `own.pp` 已扣除费用，`own.combo` 包含本卡牌。
`hand_count` 是手牌张数；`earthsigils` 是己方或对方战场上土之印的总层数，
不是土之印实例数。二者均只读，不能用 `gain` 修改；读取土之印层数不会消耗土之印。
`entered_artifacts` 是"本场对战中进入过该玩家战场的创造物·随从的**种类**数"
（按卡牌 ID 去重，读作 `own.entered_artifacts`），用于"若本次对战中进入战场的自己的
创造物·随从的种类为3种或以上"这类条件与伤害量；只读，也不会随随从离场而回退。

`own.entered` / `oppo.entered` 是"本场对战中进入过该玩家战场的随从与护符"的**集合**
（按入场顺序，包含打出的卡牌、召唤的衍生体与变身进场的实例，不因离场而移除），
只能用在 `count(...)` 里：

```wbo
when self summoned {
    if count(own.entered other where card 10931110) >= 5 { ... }
}
buff self +count(own.entered other where card 10844110)/+count(own.entered other where card 10844110);
```

`other` 在计数时排除正在结算的来源实例，用来表达"本次对战中进入战场的自己的**其他**
『同名卡』的张数"。计数在入场事件派发前已经记入，因此来源实例自己一定在集合里；
`where` 与其他集合一样支持 `card`、`type`、`class`、`trait` 等筛选。
负值不会出现：与 `count(...)` 一样返回张数。

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
`own.attacked_leader_last_turn` / `oppo.attacked_leader_last_turn` 判断"该玩家的随从在
**自己的上一回合**中攻击过主战者"（同样支持 `not`）：进入某个玩家的回合时，会把
"上一回合是否攻击过主战者"结转下来并清空本回合标记，用于
「若自己的随从在自己的上一回合中攻击过主战者，则…」。场景测试可以在玩家状态里写
`attacked_leader_last_turn;` 直接摆出这个历史。

`set attack T N;` 直接设置目标随从的当前攻击力（与 `set life` 对称）。

`pp N { … }` 是支付能量点的块：当前能量点不足 N 时整块跳过，够则先扣 N 再结算块内效果
（与 `earthrite N { … }`、`necromancy N { … }`、`faith N { … }` 同族）。
用于"与本卡牌【融合】时，消耗 2 点能量点，…"这类写法。

【激奏】写作 `accelerate N { … }`：以 N 点能量点打出时只结算这个块，
**本体不进入战场、也不发动入场曲**，结算完成后按法术流程进入墓场；
测试与模拟器的动作用 `accelerate <别名>;`（`SimulatorCommand{Kind: "accelerate"}`）。
它和 `enhance N { … }` 的区别是：爆能强化仍然正常入场，只替换/追加效果。

```wbo
effect {
    fanfare { draw 3; }
    accelerate 2 {
        summon 1 card 10671110;
    }
}
```

【结晶】写作 `crystallize N { … }`：以 N 点能量点**当作护符**打出。打出时卡面换成
衍生护符（类型变成护符、费用 N、带块内声明的吟唱/谢幕曲/事件监听），本体的入场曲、
进化等能力都不会发动；测试与模拟器的动作用 `crystallize <别名>;`
（`SimulatorCommand{Kind: "crystallize"}`）。块内允许 `counter`、`countdown`、
`lastwords`、`when` 与 `engage`：

```wbo
effect {
    bane;
    ward;
}
crystallize 2 {
    countdown 3;
    lastwords {
        summon 1 card 10661110;
    }
}
```

`count(集合 other)` 统计时排除来源实例自身，`destroy 集合 other` 同理，
因此"X 为自己的战场上的其他卡牌张数"写作 `count(own.field other)`。

`self.damage_taken` 读本实例已经受到的伤害，用于"回复至上限"这类文本：
`heal own.leader self.damage_taken; heal self self.damage_taken;`。

`damage` 可以在数值之后接极值筛选：`damage field.followers 5 highest life;` 只打击生命值最大的
那些随从（并列全中），`damage all.leaders 3 highest life;` 打击生命值最大的主战者。

`fused.cost` 与 `fused.distinct` 除了做条件，也能直接当数值用，例如
`damage oppo.field.followers fused.distinct;`（X 为融合的材料种类数）。

`self.cost`、`self.attack`、`self.life` 也可以直接作为条件左侧，例如
`if self.cost != 2 { heal own.leader 3; }` 或 `if self.life <= 3 { ... }`；
读取的是结算到该语句时来源实例的当前数值。

`rally >= N` 是【协作】条件：`rally` 统计本场对战中进入过自己战场的随从数量，
法术与护符不计入，能力召唤的随从立即计入。打出的随从在本次结算**之后**才计入，
因此"协作 19 时打出吉尔达利娅不发动【协作_20】"——官方 QA 明确要求先达到 20 再打出。
数值读取用 `own.rally` / `oppo.rally`。

```wbo
when own turn ends if not own.attacked_this_turn {
    random target from own.field.followers;
    buff target -2/-0;
    add ward to target;
}
```

合法攻击一经声明即计入，包括攻击力为零、攻击主战者以及攻击者随后离场的情况。

`<绑定> damaged` 判断绑定实例当前是否生命值受损，用于【攻击时】这类需要读取交战对象的文本：

```wbo
attack {
    if opponent damaged {
        destroy opponent;
    }
}
```

条件在结算到该语句时读取绑定，因此此前造成的伤害会影响结果；攻击主战者时
`opponent` 为空，条件为假。
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
enhance 7 replaces { ... }
```

"在牌组中发动"的能力写 `when … while self in deck { … }`，只允许回合开始与回合结束
两个时点；满足条件时用 `invoke self;` 瞬念召唤本卡牌：

```wbo
// 在牌组中发动：自己的回合开始时，本场对战中自己的随从进化过6次以上则瞬念召唤。
when own turn starts while self in deck {
    if own.evolutions >= 6 {
        invoke self;
    }
}
// 被【瞬念召唤】时：获得纹章并返回手牌。
when self invoked {
    gain own crest 10404110;
    return self to hand;
}
```

`invoke <目标>;` 把牌组里的该实例移到战场：不支付费用、不增加连击、不算抽牌、
也不发动入场曲（与亡者召还、牌组召唤一致），但会产生正常的入场事件，
随后派发"被瞬念召唤时"事件供 `when self invoked` 使用。官方 QA 明确了时点顺序：
纹章的回合开始能力先结算，其次才是牌组中发动的能力，最后是回合开始的抽牌。
`own.evolutions` / `oppo.evolutions` 是"本场对战中该玩家随从进化过的次数"（含超进化）。

"发动本随从的【入场曲】"写作 `replay fanfare self;`：按当前卡牌定义重新执行本实例的
`fanfare` 能力（包含其中的 `mode random` 等结构，重新随机）。同一实例的重发次数有 12 次
的安全上限，避免"随机模式里包含重发自身"这类卡牌无限递归：

```wbo
fanfare {
    mode random 2 {
        option 1 { random victim from oppo.field.followers; destroy victim; }
        option 2 { damage oppo.leader 2; }
        option 3 { gain own.pp 2; }
        option 4 { buff self +4/+4; replay fanfare self; }
    }
}
superevolve {
    replay fanfare self;
}
```

`enhance N` 在支付该档费用时**追加**执行；文本写"改为"的卡用 `enhance N replaces`，
支付该档时改为**只执行这个块**：本次打出的基础效果与入场曲都不再发动。它和
`superevolve replaces evolve` 是同一套替换语义。例：焰火占卜普通打出时对随机 1 个
敌方随从造成 4 点伤害，`enhance 4 replaces` 改为随机 3 个。

一次打出的外层效果、入场曲与爆能强化按声明顺序共用同一个**打出帧**：
后声明的块可以读取先声明块的输出（`summoned`、`added`、`drawn`、`destroyed` 与选择绑定），
例如"爆能强化_6：使其获得【毁灭】"写作 `enhance 6 { add bane to summoned; }`，
其中 `summoned` 来自同一次打出里先声明的入场曲。反过来把爆能强化写在入场曲之前时，
该绑定仍未定义，检查器会报错。

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

`raise maxlife own|oppo.leader N;` 与 `reduce maxlife own|oppo.leader N;` 按增量修改
生命上限，N 为 1..65535；上限夹在 1..65535，当前生命高于新上限时同样降至上限。
例如纹章『漫步的《愚者》·琳库露』写作 `reduce maxlife oppo.leader 2;`。

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

`set cost T N until …;` 是临时的费用修改（`until turn ends` / `until own turn ends` /
`until oppo turn ends`）：到期时按差量还原，"回合结束前使其费用变为 0"就用它。

`raise cost T N;` 给目标的当前费用加 N（没有上限，卡牌文本没有写上限时按字面处理）；
它和 `reduce cost` 也可以带期限：`raise cost oppo.hand 1 until oppo turn ends;`
表示"对手的回合结束前，使对手的所有手牌的费用+1"，到期只撤销这次差量，
不会覆盖期间发生的永久加减费。
只做减费时继续用 `reduce cost T N minimum M`。
带期限的 `reduce cost` 可以省略 `minimum`（下限默认为 0）：
`reduce cost self 1 until turn ends;`。
`reduce countdown T X` 的增量也可以是数值引用，例如"本护符的倒计数 -X，X 为自己的纹章数"
写作 `reduce countdown self own.crests;`。

`set cost T N;` 把目标卡牌的当前费用设为 N（N 为 0..65535，可用数值表达式），
可以指向手牌或牌组里的实例。需要"费用变为 1"这类精确表述时用它；只做相对调整时
用 `reduce cost`。

`halve cost T;` 把目标的当前费用变为一半，奇数向上取整（官方 FAQ：9 → 5）。
它和 `reduce cost T N minimum M` 都接受集合，因此"使自己的牌组中的所有卡牌的费用
变为一半"写成 `halve cost own.deck;`，"使自己牌组中的所有随从的费用 -3"写成
`reduce cost own.deck.followers 3 minimum 0;`。两者都按**当前**费用结算：重复发动
基于已经改变的费用，只影响结算那一刻在集合里的实例——之后从手牌返回牌组的卡牌
不会追溯生效（官方 FAQ 对『绚丽凤凰·小凤』的说明）。

`double stats T;` 让集合中的每个随从按**自己**的当前数值翻倍：攻击力与生命值都 ×2，
已受伤害也一并翻倍，因此上限与当前生命同时变化。护符等非随从目标被忽略。
例如『帕梅拉的舞蹈』的纹章写作 `double stats own.field.followers;`。
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
`when self stats increased` 在**本实例**于战场上获得攻击力/生命值增加时发动；
`when own|oppo follower life decreased` 在战场上的随从生命值减少时发动：减益发独立事件，
伤害则由 `damaged` 事件一并匹配（事件流里只保留原来的伤害事实，`set life` 的设置不算减少）。
两者都支持 `once per own turn` 限制次数。

`when own card played` / `when oppo card played` 监听卡牌被打出，监听对象绑定成 `played`，
可用 `where` 筛选（类型、trait、费用…），末尾的 `other` 排除本卡牌自身。它在打出后入队，
按事件顺序在本次效果结算之后发动。`when own card discarded` 和 `when oppo card discarded` 则由战场上的来源监听，
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
变身的来源也可以是集合里的随机一张卡牌的复制：

```wbo
engage 0 {
    require target from own.hand;
    transform target into random card from oppo.deck;
}
```

`transform <目标> into random card from <集合>;` 在结算时从集合里等概率取一张
（只有一条候选时不消费随机决策），把目标的身份换成它的卡牌定义；来源为空时不变身。
多个目标时每个目标各自抽一次（"分别变身为…的复制"），候选集合只查询一次；
抽选来源同样可以带 `where` 筛选。
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

声明了 `fusion` 的卡牌在其它效果块里也能读这两个值。例如法术的正文写在
`effect` 最外层，与融合块分开：

```wbo
effect {
    fusion material from own.hand where class forestcraft {
    }
    if fused.distinct >= 1 {
        draw 2;
    } else {
        draw 1;
    }
}
```

融合读到的数值也可以用于伤害/回复的目标数量之外的效果块；`fused.cost` 是材料原始费用合计，
`fused.distinct` 是按卡牌 ID 计算的种类数。融合块本身留空是允许的：材料选择由融合指令完成，正文在打出时按当时已附着的
材料数分支。`fused` 读到的是来源实例的已有材料，因此同一条判断也可以出现在
`fanfare` 等结算时点；没有声明 `fusion` 的卡牌读到 `fused` 会被检查器拒绝。

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

条件里的数值也可以直接统计集合，写法与伤害/回复的数量一致：

```wbo
if count(own.field.followers where form super_evolved) >= 1 { ... }
if count(own.field.amulets) >= 2 { ... }
if count(oppo.hand) <= 5 { ... }
```

支持 `where` 筛选（`form`、`type`、`class`、`trait`、`life`、`cost`、`keyword`、`card`），
也支持 `sum(...)` 求和。判断发生在执行到该语句时，因此同一能力之前造成的变化会影响结果。
比较的右侧同样可以是数值表达式，例如"若自己的主战者的生命值大于对手的主战者的生命值"
写作 `if own.life > oppo.life { ... }`。左侧保持标量写法：集合计数之间的比较
（`count(A) > count(B)`）还没有实现，写了会在检查阶段报错而不是静默按 0 结算。

`own.played has costs N to M` 判断"本场对战中该玩家使用过的卡牌的**原始费用**"
是否涵盖 N 到 M 的所有数值（"费用包含1到8所有数值"），只统计从手牌打出的卡牌，
召唤与【瞬念召唤】不计入；爆能强化、激奏、结晶仍按卡牌定义的原始费用记录：

```wbo
when own turn ends {
    if own.played has costs 1 to 8 {
        destroy self;
    }
}
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
`mode random N { … }` 表示由引擎随机发动 N 个互不相同的选项（"从以下能力中随机发动2个能力"）：
不向玩家提问，结算顺序仍按选项编号升序，因此回放确定。
`mode random N history <名字> { … }` 在此基础上只从**尚未发动过**的选项里随机
（"从以下未发动的能力中随机发动1个能力"）：发动过的选项按实例记录在该名字下，
跨回合与续局保留；全部选项都发动过时该模式什么都不做。写在纹章里的记录属于纹章实例，
与来源卡牌实例互不影响。
`mode N { … }` 表示【模式】选择 N 个能力发动（省略数量等于 1）：玩家一次选择 N 个不同选项，
按选项编号从小到大依次结算，响应使用选项编号列表（测试里写作 `mode 1, 3;`）。
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

`when own card played` 把本次打出的卡绑定为 `played`
（例如纹章"自己使用随从时使其进化"写作 `evolve played silent;`）；
`when own follower summoned` 绑定 `summoned`、`when own amulet engaged` 绑定 `engaged`、
`when own card discarded` 绑定 `discarded`、`when own follower destroyed` 绑定 `destroyed`、
`when own follower leaves field` 绑定 `left`，与对应操作的输出同名。

事件头部可以加 `during own turn` / `during oppo turn` 把监听限制在特定玩家的回合
（`self survives damage during own turn`、`when own leader healed during own turn`），
用于"若为自己的回合"这类条件。
入场监听也能带回合限制：`when own follower summoned during own turn where trait departed { … }`
对应"自己的亡者·随从进入战场时，若为自己的回合…"；`during own turn` 只在自己的回合触发，
`during oppo turn` 只在对方回合触发。

`when own earthrite [while self in hand] { … }` 在自己成功支付【土之秘术】的土之印时触发，
用于"手牌中发动：自己发动【土之秘术】时，使本卡牌的费用-1"这类写法：

```wbo
when own earthrite while self in hand {
    reduce cost self 1 minimum 0;
}
```

事件在土之印实际扣除之后派发（层数不足、整块跳过时不触发），
监听可以挂在手牌里的卡牌上；`oppo earthrite` 对应对手发动。

抽牌事件写作 `when own card drawn [during own turn] { … }`，绑定名 `drawn` 指向被抽到的实例
（可以当数值用，例如 `damage oppo.field.followers drawn.cost;`）。
"抽到本卡牌时"写作 `when self drawn { set cost self 3 until turn ends; }`：
监听在该实例进入手牌后触发，每次抽到都会按张结算。
其他随从的宣告攻击用 `when own|oppo follower attacks [leader] [where …] { … }` 表达，
绑定 `attacker` 指向攻击方；加 `attacks leader` 只监听攻击主战者的那部分
（例如"对手拥有【疾驰】的随从攻击主战者时，使其 -3/-0"写作
`when oppo follower attacks leader where keyword storm { buff attacker -3/-0 until turn ends; }`）。

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

`where lastwords` 单独判断卡牌定义里是否带【谢幕曲】能力，用于"拥有【谢幕曲】的
护符"这类筛选；它只关心能力是否存在，不等待该能力真正触发。目前的判定读取
卡牌定义的固有能力，运行中通过 `grant` 临时获得的【谢幕曲】还不算在内。

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
