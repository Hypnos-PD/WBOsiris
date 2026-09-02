# WBO 规则测试 DSL 0.1 草案

`.wbotest` 用于描述可重复执行的规则场景。它只声明初始状态、玩家指令和
可观察结果，不允许直接调用规则引擎内部函数。

测试文件必须使用 UTF-8。关键字区分大小写，语句以分号结束。

## 文件结构

```wbotest
wbotest 0.1.0;
use cards "../../cards";

scenario "场景名称" {
    seed 1;

    state {
        turn own 5;
        phase main;

        player own { ... }
        player oppo { ... }
    }

    action { ... }
    expect { ... }
}
```

`use cards` 的路径相对于测试文件所在目录。测试运行器必须加载该目录下用到的
卡牌及其引用卡牌。一个文件可以包含多个 `scenario`，场景之间完全隔离。

## 默认状态

未声明的玩家字段采用以下默认值：

- 主战者生命值为 `20/20`。
- 当前能量点和最大能量点均为 `0`。
- 进化点和超进化点均为 `0`。
- 连击、墓场数及其他职业资源均为 `0`。
- 牌组、手牌、战场、墓场、消失区和已破坏历史均为空。
- `own` 是当前行动玩家，阶段为 `main`。

涉及某项规则的字段应当显式写出，避免测试依赖无关默认值。

## 玩家状态

```wbotest
player own {
    leader 10/20;
    pp 3/10;
    ep 1;
    sep 1;
    combo 2;
    shadows 4;

    deck top {
        follower first = 10001110;
        spell second = 10031310;
    }

    hand {
        follower source = 10002110;
    }

    field {
        amulet sigil = 10031210 {
            earthsigil 2;
        }
    }

    graveyard {
        spell old_spell = 10031310;
    }

    banished {
        follower old_unit = 10001110;
    }

    destroyed {
        follower destroyed_unit = 10001110;
    }
}
```

`pp 3/10` 的左值是当前可用能量点，右值是最大能量点。断言中分别使用
`own.pp` 和 `own.maxpp`；组合形式 `own.pp == 3/10` 会同时比较两者。

牌组在 `deck top` 中按照从牌组顶部到底部的顺序排列。`destroyed` 是本局已被
破坏随从的历史，不等同于当前墓场内容，亡者召还读取该历史。

每个实例必须声明场景内唯一的别名，例如 `source`。实例移动区域后别名保持
不变。规则效果创建的新实例通过卡牌 ID、产生顺序或事件输出进行断言。

实例可以覆盖运行时状态：

```wbotest
follower unit = 10001110 {
    stats 5/3;
    evolved;
    ward;
}

follower super_unit = 10002110 {
    super_evolved;
}

amulet amulet = 10061210 {
    countdown 2;
    engaged false;
}
```

这些覆盖只改变该实例的运行时状态，不会修改卡牌定义。

## 动作

`action` 按顺序描述一条玩家指令及其后续选择：

```wbotest
action {
    play source;
    select target;
    mode 2;
}
```

基础动作包括：

```wbotest
play source;
engage source;
evolve source;
superevolve source;
attack attacker into defender;
attack attacker into oppo.leader;
end_turn;
```

仅当引擎产生选择请求时，才能使用 `select` 或 `mode`。目标集合为空且规则使用
`choose` 时，不产生玩家选择请求；效果以空目标继续结算。`require` 没有候选
目标时，整个玩家指令非法。

用于验证回合时点的测试可以使用测试驱动动作：

```wbotest
advance turn_start own;
advance turn_end own;
```

`advance` 不是玩家指令，不应进入正式对局回放；它只用于从指定时点开始执行
规则队列。

未显式声明 `turn` 时，初始行动方为 `own`，回合号为 `1`。

## 结果断言

合法性断言：

```wbotest
expect {
    legal;
}
```

```wbotest
expect {
    illegal target_required;
    unchanged;
}
```

非法指令必须保持整个状态和随机数生成器不变。`unchanged` 比较动作执行前后的
完整可序列化状态。

字段与区域断言：

```wbotest
expect {
    own.leader.life == 14;
    own.pp == 0/10;
    source.zone == graveyard;
    source.evolved == true;
    source.super_evolved == true;
    source has ward;
    source lacks storm;

    own.hand count card 10001110 == 2;
    own.field count card 90051130 == 1;
    all own.field where card 90051130 have drain;
}
```

牌组顺序断言：

```wbotest
expect {
    own.deck top == [first, second];
    own.hand contains [drawn_a, drawn_b] ordered;
}
```

事件断言使用规则层事实，不使用 UI 动画或日志文本：

```wbotest
expect {
    events contains ordered {
        heal own.leader 4;
        draw own 1;
    }

    events excludes {
        heal own.leader 2;
    }
}
```

`contains ordered` 允许实际事件流中夹杂区域移动等其他事件，但列出的事件必须
按顺序出现。需要验证完整事件序列时使用 `events exact`。

事件可以引用实例别名：

```wbotest
events contains ordered {
    destroy source;
    summon card 90061130 count 1;
    game_end own;
}
```

## 随机数断言

每个场景必须显式声明 `seed`。默认规则集 `wbo-standard-0.3.0` 使用 SplitMix64
v1，可以根据种子断言具体随机结果，同时应断言是否消费随机数：

```wbotest
expect {
    rng.consumed == 0;
}
```

候选集合为空、非法指令和无需打破并列的确定性选择都不能消费随机数。

`rng.consumed` 统计规则层随机决策次数，不统计具体算法内部读取了多少随机字。
随机目标、并列亡者召还和成功返回牌组各消费一次随机决策；非法动作和空候选
随机目标不消费。

## 场景编写原则

- 一个场景只验证一个主要规则结论。
- 初始状态只包含证明该结论所需的对象。
- 优先断言可观察状态和规则事件，不断言执行器内部栈结构。
- 对非法动作使用 `unchanged`，避免只检查手牌或能量点。
- 对替换效果同时断言新效果存在、旧效果不存在。
- 对部分成功的批量操作断言实际输出数量及输出实例获得的能力。
- 测试文件中的中文名称仅用于阅读，卡牌引用一律使用稳定数字 ID。
