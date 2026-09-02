# WBO 卡牌 DSL 0.1 草案

本文档记录首批卡牌文件采用的语法与语义。当前版本仍是草案：真实卡牌是
语言的一致性样本，在完成这些卡牌的转换期间，语法仍可能调整。

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

## 来源与目标

- `self`：提供当前效果的卡牌或场上对象。
- `own`：当前控制者。
- `oppo`：当前控制者的对手。
- `target`：最近一次目标选择所绑定的值。
- `summoned`、`drawn`：最近一次对应操作成功产生的有序集合。

对 `summoned` 或 `drawn` 执行操作时，会按顺序对集合中的每个实例执行。手牌
或战场空间不足时，集合只包含实际成功进入目标区域的实例。新的召唤或抽牌
操作会替换此前的同名绑定。

存在候选对象时，`choose` 绑定一个目标。候选集合为空时，它绑定 `none` 并
继续结算；对 `none` 执行的操作不产生效果。

```wbo
choose target from oppo.field.followers;
```

`require` 表示在验证玩家指令时进行必选目标选择。不存在候选对象时，该卡牌
或启动能力不能使用。

```wbo
require target from own.field.followers;
```

筛选条件写在目标集合之后，`other` 用于排除 `self`。

```wbo
choose target from own.field.followers other where trait golem;
require target from oppo.field.followers where life <= 3;
```

只有候选集合非空时，随机选择才会消耗一次对局随机数。

```wbo
random target from oppo.field.followers;
```

## 效果操作

效果从上到下依次执行。某个操作失败时，不会回滚此前已经执行的操作或支付。
法术 `effect` 中最外层的操作会在使用法术时执行；随从和护符通过能力块声明
对应的执行时点。

```wbo
draw 2;
draw all from deck where card 10022120;
draw 1 from deck where type follower;
add 2 card 90011110 to hand;
summon 1 card 90021110;
damage oppo.leader 3;
heal own.leader 2;
buff self +1/+1;
destroy target;
banish target;
return target to hand;
return target to deck;
add ward to target;
remove ward from target;
```

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

由其他效果引起的进化必须显式标记，并且不会触发目标随从卡面上的
`evolve` 能力：

```wbo
evolve target silent;
```

随从进化后保留原有的关键词和触发能力。

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

每个护符实体在控制者的每个回合中只能 `engage` 一次，进入战场的当回合也
可以启动。`engage` 后的整数是启动所需的能量点费用。

爆能强化是强制替代费用。存在一个或多个可支付档位时，必须采用费用最高的
可支付档位，玩家不能选择按原费用或更低强化档位打出。

## 融合与变身

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

融合能力块在全部材料附着后结算一次，不按材料数量重复结算。`transform self into
card C preserving materials` 将来源实例的卡牌定义改为 `C`，保留来源的
`InstanceId` 和全部已有及本次附着的材料；该操作不会为变身结果创建新实例，也
不会把材料转移到区域中。

融合变身链已经完整定义到 `90074110`。融合块可读取 `fused.cost` 和
`fused.distinct`：前者为全部已附着材料的原始费用合计，后者为按卡牌 ID 计算的
材料种类数。变身保留材料，因此这两个值跨变身连续累积。

## 条件与资源

```wbo
if combo >= 3 { ... }
if overflow { ... } else { ... }
earthrite 1 { ... }
necromancy 4 { ... }
```

卡牌在入场曲结算前已经计入连击。土之秘术和唤灵会在资源充足时自动支付，
资源不足时跳过对应代码块。支付成功后，即使后续操作失败也不会退还资源。

土之印是护符实体。新的土之印进入战场时，会取得已有土之印的全部层数并保留
新实例；被合并的旧实例移入消失区，不触发破坏、谢幕曲或墓场计数。魔力增幅
记录在每一张手牌实例上，费用降低的下限为零。

亡者召还会选择费用不超过指定值且费用最高的已破坏随从；最高费用相同时，
使用对局随机数决定。它会创建一个全新的实例，并且不触发入场曲。

需要玩家选择其中一项的模式使用带稳定编号的 `option`。编号会写入玩家指令
和回放，不能依赖代码块的临时排列位置。

```wbo
mode {
    option 1 {
        draw 1 from deck where type follower;
    }
    option 2 {
        reanimate 2;
    }
}
```

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
