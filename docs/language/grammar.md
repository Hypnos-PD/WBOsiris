# WBO DSL 形式文法 0.1 草案

本文给出 `.wbo` 卡牌文件与 `.wbotest` 规则测试文件的规范语法。本文仍标记为
0.1 草案；其目标是精确描述当前 75 张卡牌、15 个测试场景以及 [卡牌 DSL](cards.md)、
[测试 DSL](tests.md) 已承诺的表面语法，不改变现有作者语法。

本文使用扩展巴科斯范式。`=` 定义产生式，`|` 表示选择，`[...]` 表示可选，
`{...}` 表示重复零次或多次，`(...)` 表示分组，后缀 `-` 表示字符集差集。
双引号包围的内容是区分大小写的终结符；文法中的空格仅用于排版。

## 1. 词法

### 1.1 编码、空白与换行

文件必须是有效 UTF-8，可带或不带 UTF-8 BOM；BOM 只允许出现在文件开头，
并在词法分析前移除。除字符串外，空格、水平制表、回车和换行都是分隔用空白，
不具有缩进语义。实现必须接受 LF 与 CRLF，并可接受单独 CR；诊断位置应按
Unicode 标量值计算行列。除字符串和注释外，任意两个不能合并成同一词法单元的
词法单元之间都可出现空白或注释。

```ebnf
white_space       = " " | "\t" | "\r" | "\n" ;
digit             = "0" | "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9" ;
nonzero_digit     = "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9" ;
ascii_letter      = "A" | "B" | "C" | "D" | "E" | "F" | "G" | "H" | "I" | "J"
                  | "K" | "L" | "M" | "N" | "O" | "P" | "Q" | "R" | "S" | "T"
                  | "U" | "V" | "W" | "X" | "Y" | "Z"
                  | "a" | "b" | "c" | "d" | "e" | "f" | "g" | "h" | "i" | "j"
                  | "k" | "l" | "m" | "n" | "o" | "p" | "q" | "r" | "s" | "t"
                  | "u" | "v" | "w" | "x" | "y" | "z" ;
identifier        = (ascii_letter | "_") , {ascii_letter | digit | "_"} ;
integer           = "0" | nonzero_digit , {digit} ;
signed_integer    = ("+" | "-") , integer ;
version           = integer , "." , integer , "." , integer ;
card_id           = digit , digit , digit , digit , digit , digit , digit , digit ;
boolean           = "true" | "false" ;
```

`identifier` 采用 ASCII 是为了让规则标识在工具链、回放和跨平台文件中稳定；
中文及其他语言文字应放在字符串中。`integer` 不包含符号且除 `0` 外不得有前导
零。`version` 的三个分量分别为主、次、修订版本。`card_id` 恰为 8 个 ASCII
数字，前导零在词法上允许，具体编号策略由卡牌库检查器约束。

### 1.2 字符串

普通字符串不能跨物理行。它支持 `\"`、`\\`、`\n`、`\r`、`\t`、
`\u{hex}` 转义；其他反斜杠序列是错误。`\u{hex}` 含 1 至 6 个十六进制数字，
结果必须是 Unicode 标量值，不能是代理项。

```ebnf
hex_digit         = digit | "A" | "B" | "C" | "D" | "E" | "F"
                        | "a" | "b" | "c" | "d" | "e" | "f" ;
unicode_escape    = "\\u{" , hex_digit , {hex_digit} , "}" ;
escape            = "\\\"" | "\\\\" | "\\n" | "\\r" | "\\t" | unicode_escape ;
string_char       = ? 除 U+000A、U+000D、双引号、反斜杠外的 Unicode 标量值 ? ;
string            = '"' , {string_char | escape} , '"' ;
```

三引号字符串以 `"""` 开始和结束，可包含未转义的换行和普通双引号，但连续
三个双引号结束字符串。当前卡牌采用的去缩进规则如下：若开始定界符后紧接换行，
删除该换行；若结束定界符前只有空白至该行行首，删除该行的缩进和结束定界符前
的换行；以所有非空内容行的最小公共前导空白为基准，从每一非空行删除等量前导
空白，空白行清空。制表符按字符参与公共前缀比较，不展开为列。三引号内容按
UTF-8 原样保留，不处理普通字符串转义。

```ebnf
triple_string     = '"""' , {triple_char} , '"""' ;
triple_char       = ? 不会使当前位置开始形成结束定界符的任意 Unicode 标量值 ? ;
text_literal      = string | triple_string ;
```

### 1.3 注释与分隔

WBO 行注释从 `<<` 到行末。为兼容早期源码，词法器仍接受 `//`。块注释从 `/*`
到配对的 `*/`，允许嵌套。注释定界符在任一字符串内部没有特殊含义。未闭合的
字符串或注释必须报错。

```ebnf
line_comment      = ("<<" | "//") , { ? 除 U+000A、U+000D 外的 Unicode 标量值 ? } ;
block_comment     = "/*" , {block_comment | block_comment_char} , "*/" ;
block_comment_char = ? 不会在当前位置开始 "/*" 或 "*/" 的 Unicode 标量值 ? ;
separator         = white_space | line_comment | block_comment ;
```

下文产生式省略词法单元之间允许出现的 `separator`。

## 2. 卡牌文件

### 2.1 顶层结构

```ebnf
wbo_file          = "wbo" , version , ";" , card_decl ;
card_decl         = "card" , card_id , "{" , card_body , "}" ;
card_body         = type_decl , cost_decl , [stats_decl] , {trait_decl} ,
                    effect_decl , meta_decl , locale_decl , {locale_decl} ;

type_decl         = "type" , card_type , ";" ;
card_type         = "follower" | "spell" | "amulet" ;
cost_decl         = "cost" , integer , ";" ;
stats_decl        = "stats" , stat_pair , ";" ;
stat_pair         = integer , "/" , integer ;
trait_decl        = "trait" , identifier , ";" ;

effect_decl       = "effect" , effect_block ;
effect_block      = "{" , {effect_statement} , "}" ;

meta_decl         = "meta" , "{" , pack_decl , class_decl , rarity_decl , "}" ;
pack_decl         = "pack" , integer , ";" ;
class_decl        = "class" , identifier , ";" ;
rarity_decl       = "rarity" , identifier , ";" ;

locale_decl       = "locale" , locale_id , "{" , name_decl , text_decl , "}" ;
locale_id         = "chs" | "eng" | "jpn" | "kor" | "cht" ;
name_decl         = "name" , text_literal , ";" ;
text_decl         = "text" , text_literal , ";" ;
```

顶层顺序是语义的一部分：`type`、`cost`、可选 `stats`、零个或多个 `trait`、
`effect`、`meta`、本地化块必须依次出现。`meta` 内固定为 `pack`、`class`、
`rarity`；每个本地化块固定为 `name`、`text`。本地化块按 `chs`、`eng`、
`jpn`、`kor`、`cht` 的相对顺序出现，不得重复；0.1 卡牌库要求五种全部存在。

### 2.2 效果语句总览

```ebnf
effect_statement  = intrinsic_statement
                  | counter_declaration
                  | ability_block
                  | fusion_block
                  | event_block
                  | replace_block
                  | selection_statement
                  | if_statement
                  | repeat_statement
                  | resource_block
                  | mode_block
                  | grant_block
                  | operation ;

counter_declaration = "counter" , counter_name , integer , ";" ;
counter_name      = identifier ; (* [a-z][a-z0-9_]{0,31}; unique, outermost effect only *)
counter_ref       = "self" , "." , "counter" , "." , counter_name ;

grant_block       = "grant" , value_ref , [where_clause] , "{" ,
                    {"label" , locale_id , text_literal , ";"} ,
                    granted_ability , "}" ;
granted_ability   = "lastwords" , effect_block
                  | "when" , participant , "turn" , ("starts" | "ends") , effect_block ;

intrinsic_statement = ability , ";"
                    | card_restriction , ";"
                    | "countdown" , integer , ";"
                    | "earthsigil" , ";" ;

ability            = "ward" | "storm" | "rush" | "bane" | "drain"
                   | "intimidate" | "barrier" | "stealth" | "aura"
                   | "ability_target_guard" | "cannot_attack"
                   | "cannot_attack_follower" | "cannot_attack_leader" ;
card_restriction   = "unplayable" ;

ability_block      = simple_ability_name , effect_block
                   | "superevolve" , [super_relation , "evolve"] , effect_block
                   | "engage" , integer , effect_block
                   | "enhance" , integer , effect_block ;
simple_ability_name = "fanfare" | "lastwords" | "attack" | "clash" | "evolve"
                     | "spellboost" ;
super_relation     = "replaces" | "extends" ;
```

`fanfare`、`lastwords`、`attack`、`clash`、`evolve`、`superevolve`、`spellboost` 是
触发能力；`engage` 是带能量点费用的启动能力；`enhance` 是强制替代打出费用的
能力。存在多个可支付强化档位时，必须采用费用最高的一档，不能使用原费用或更低
档位。支付最高可支付档位的费用后，发动该档位和以下的全部强化能力。
固有能力仅使用 `ability` 中的稳定英文标识。新增固有能力必须由新语言版本显式
登记，不能把任意 `identifier` 静默当作能力。

`intimidate` 禁止敌方随从以该随从为攻击目标，但不妨碍能力选择、能力伤害或其他
非攻击效果。它与 `ward` 同时存在于一个随从时，该随从的 `ward` 不生效。
`unplayable` 是卡牌固有限制：该手牌实例不能用于 `play` 指令，但仍可作为融合来源
或被其他规则引用。

### 2.3 融合

```ebnf
fusion_block       = "fusion" , "material" , "from" , target_set ,
                     [where_clause] , effect_block ;
```

融合能力只允许声明在可位于手牌的卡牌上；当前形式要求材料来源为 `own.hand`。
融合是独立玩家指令，一条指令只产生一次材料选择请求。候选集合在请求产生时求值，
并自动排除融合来源实例。玩家必须选择一个或多个互不相同的候选实例；选择结果为空、
重复或不属于候选集合时整条指令非法。合法材料同时附着到来源实例，融合块随后仅执行
一次，不按材料数重复。附着不是 `destroy`、`banish` 或普通区域移动，不产生谢幕曲、
墓场计数或对应事件。

### 2.4 选择、集合与过滤

```ebnf
selection_statement = selection_kind , binding_name , "from" , target_set ,
                      ["other"] , [where_clause] , [extremum_clause] , ["count" , integer] , ";"
                    | selection_kind , binding_name , "from" , character_set , ["count" , integer] , ";" ;
character_set       = "own.field.followers" , "or" , "own.leader"
                    | "oppo.field.followers" , "or" , "oppo.leader" ;
selection_kind      = "choose" | "require" | "random" ;
extremum_clause     = ("highest" | "lowest") , ("attack" | "life" | "cost") ;
binding_name        = identifier ;

target_set          = "field" , ["." , card_type_plural]
                     | participant , "." , zone , ["." , card_type_plural] ;
participant         = "own" | "oppo" ;
zone                = "deck" | "hand" | "field" | "graveyard" | "banished"
                    | "destroyed" ;
card_type_plural    = "followers" | "spells" | "amulets" ;

where_clause        = "where" , filter_expression ;
filter_expression   = filter_conjunction , {"or" , filter_conjunction} ;
filter_conjunction  = filter_term , {"and" , filter_term} ;
filter_term         = "card" , card_id
                    | "spellboost"
                    | "type" , card_type
                    | "class" , identifier
                    | "trait" , identifier
                    | "form" , ("unevolved" | "evolved" | "super_evolved")
                    | ("life" | "cost") , comparison_operator , integer ;
comparison_operator = "==" | "!=" | "<" | "<=" | ">" | ">=" ;
```

`field` 表示双方战场合并后的稳定有序集合；`own.field`、`oppo.field` 可包含随从
和护符。复数类型后缀缩窄集合类型。`other` 只排除与 `self` 同一实例的对象，
必须写在 `where` 前。`and` 的优先级高于 `or`；不支持括号过滤器。
`cost` 比较实例的当前费用，包括加费或降费效果，不比较原始费用。
`destroyed` 是随从与护符的破坏历史，不是区域，在集合语法中按只读历史集合处理。
每次破坏保存独立记录；同一实例被多次破坏时不去重。历史集合可用于 `count`，
不能作为选择或修改操作的目标。其筛选读取破坏时的卡牌身份与属性。

选择数量 `count` 必须为 1 至 65535 的整数，省略时为 1，且必须写在过滤条件之后。
`choose` 和 `random` 在空集合上将绑定设为 `none` 并继续；非空时选择
`min(count, 候选数)` 个不同实例。`random` 每抽取一个实例消费一次对局随机数。
`require` 在玩家指令合法性检查阶段要求至少 `count` 个候选，否则整条指令非法。
`choose` 与 `require` 必须选满请求数量，结果属于候选集合且不能重复；绑定保持候选
顺序，不使用客户端提交顺序。集合效果对这些目标使用同一次结算及死亡批次。

### 2.5 条件、模式与资源控制

```ebnf
if_statement      = "if" , condition , effect_block , ["else" , effect_block] ;
condition         = "overflow"
                  | "self" , "form" , follower_form
                  | participant , "." , evolution_unlock
                  | scalar_value , comparison_operator , integer ;
follower_form     = "unevolved" | "evolved" | "super_evolved" ;
evolution_unlock  = "evolve_unlocked" | "superevolve_unlocked" ;
scalar_value      = "combo" | participant , "." , scalar_field
                  | counter_ref
                  | "fused" , "." , fusion_scalar_field ;
scalar_field      = "life" | "pp" | "maxpp" | "ep" | "sep" | "combo"
                  | "shadows" ;
fusion_scalar_field = "cost" | "distinct" ;

resource_block    = resource_name , integer , effect_block ;
resource_name     = "earthrite" | "necromancy" ;

mode_block        = "mode" , "{" , option_decl , option_decl , {option_decl} , "}" ;
option_decl       = "option" , integer , "{" , {option_label} , {effect_statement} , "}" ;
option_label      = "label" , locale_id , string , ";" ;
```

`earthrite` 与 `necromancy` 仅在资源足够时支付并进入块，资源不足时跳过整个块；
成功支付后不因后续失败退还。`mode` 至少有两个选项，选项编号是回放协议的一部分。
`overflow` 的具体阈值属于规则引擎；`combo` 在入场曲结算前已经包含当前打出的卡。
`fused.cost` 是来源实例全部已附着材料的原始费用合计，`fused.distinct` 是这些材料按
卡牌 ID 计算的种类数；二者只允许在 `fusion_block` 内使用。

### 2.6 事件与替换

```ebnf
event_block       = "when" , event_pattern , [source_zone] , [turn_limit] , [where_clause] , effect_block ;
event_pattern     = participant , event_subject , event_verb
                  | participant , "follower" , "leaves" , "field"
                  | participant , "turn" , turn_boundary
                  | "self" , ("evolved" | "super_evolved" | "discarded") ;
event_subject     = "follower" | "amulet" | "card" ;
event_verb        = "summoned" | "engaged" | "discarded" | "fused" ;
source_zone       = "while" , "self" , "in" , ("hand" | "field") ;
turn_limit        = "once" , "per" , [participant] , "turn" ;
turn_boundary     = "starts" | "ends" ;

replace_block     = "replace" , "self" , "leaving" , "field" , effect_block ;
```

事件模式中的筛选器作用于事件产生的对象。`summoned`、`engaged`、`discarded` 分别绑定该事件
的对象；回合边界事件不建立对象绑定。监听器默认包含 Token。`replace self
leaving field` 在原区域移动前执行并取消原移动，同一替换块不会因自己产生的区域
移动再次触发。

`when self evolved` 与 `when self super_evolved` 只允许在随从上声明，不附带 `where`。
它们匹配自身的形态变化，前者也包含超进化；不同于仅在支付点数后执行的进化关键词能力。
`when self discarded` 允许用于任何卡牌种类且不附带 `where`，只由被舍弃实例自身发动。
`when own card discarded` 与 `when oppo card discarded` 由战场监听器按弃牌拥有者匹配。
`card fused` 匹配成功融合的来源卡，每次操作只触发一次。`follower leaves field` 绑定 `left`。
`source_zone` 只用于玩家侧事件，不用于 `when self ...` 或 `grant` 内的附加能力；
省略时来源位于战场。区域条件约束来源，`where` 则约束事件对象。
`turn_limit` 同样只用于玩家侧事件，不用于 `when self ...` 或 `grant`。
`once per own turn` 只在持有者回合限一次，`once per oppo turn` 只在对方回合限一次，
`once per turn` 则在双方各自的每个回合限一次。次数按来源实例与能力分别记录，成功入队时消耗。

### 2.7 操作

```ebnf
operation         = draw_operation
                  | add_operation
                  | summon_operation
                  | numeric_operation
                  | object_operation
                  | ability_operation
                  | return_operation
                  | evolve_operation
                  | reanimate_operation
                  | reduce_operation
                  | spellboost_operation
                  | set_attack_limit_operation
                  | transform_operation ;

draw_operation    = "draw" , draw_amount , ["from" , "deck" , where_clause] , ";" ;
draw_amount       = integer | "all" ;

add_operation     = "add" , integer , "card" , card_id , "to" , "hand" , ";"
                  | "add" , "combo" , integer , ";"
                  | "add" , integer , "earthsigil" , ";"
                  | "add" , integer , "counter" , counter_name , ";"
                  | "add" , ability , "to" , value_ref , ["other"] , [where_clause] , [effect_duration] , ";" ;
effect_duration   = "until" , [participant] , "turn" , "ends" ;

summon_operation  = "summon" , integer , "card" , card_id , ";"
                  | "summon" , "copies" , "of" , value_ref , [where_clause] , ";" ;
numeric_operation = "damage" , value_ref , effect_amount , [damage_distribution] , [where_clause] , ";"
                  | "heal" , value_ref , effect_amount , [where_clause] , ";"
                  | "set" , "life" , value_ref , effect_amount , ";"
                  | "buff" , value_ref , ["other"] , signed_amount , "/" , signed_amount , [where_clause] , [effect_duration] , ";"
                  | "gain" , scalar_ref , integer , ";"
                  | "restore" , participant , "." , "pp" , ";" ;

effect_amount     = integer | counter_ref | "count" , "(" , count_source , [where_clause] , ")"
                  | participant , "." , ("combo" | "pp" | "maxpp" | "life" | "ep" | "sep" | "shadows")
                  | "self" , "." , ("cost" | "attack" | "life") ;
signed_amount     = ("+" | "-") , effect_amount ;
count_source      = target_set | binding_name ; (* binding must already be defined in this scope *)
repeat_statement  = "repeat" , effect_amount , effect_block ;
damage_distribution = "distributed" , ["overflow" , participant , "." , "leader"] ;

object_operation  = ("destroy" | "banish" | "discard") , value_ref , [where_clause] , ";"
                  | "destroy" , batch_target , "," , batch_target , {"," , batch_target} , ";" ;
batch_target      = binding_name | "self" ;
ability_operation = "remove" , ability , "from" , value_ref , ["other"] , [where_clause] , ";" ;
return_operation  = "return" , value_ref , "to" , ("hand" | "deck") , ";" ;
evolve_operation  = ("evolve" | "superevolve") , value_ref , "silent" , ";" ;
reanimate_operation = "reanimate" , integer , ";" ;

reduce_operation  = "reduce" , "countdown" , value_ref , integer , ";"
                  | "reduce" , "cost" , value_ref , integer , "minimum" , integer , ";" ;
spellboost_operation = "spellboost" , value_ref , integer , ";" ;
set_attack_limit_operation = "set_attack_limit" , "self" , positive_integer , ";" ;
transform_operation = "transform" , value_ref , "into" , "card" , card_id ,
                      ["preserving" , "materials"] , [where_clause] , ";" ;

positive_integer  = integer ; (* semantic constraint: value >= 1 *)
value_ref         = binding_name
                  | "self" | "target" | "summoned" | "drawn"
                  | participant , "." , "leader"
                  | target_set ;
scalar_ref        = participant , "." , scalar_field ;
```

`draw all` 必须带过滤器；`draw integer` 可省略来源，等价于从当前控制者牌组顶部
抽取该数量。带过滤器的定量抽牌从匹配实例中等概率、不放回抽取，每张消费一次随机
决策（包括只剩一个候选时），并按抽中顺序进入手牌；未抽中的牌保持相对顺序。
`draw all` 按牌组顺序取全部匹配实例，不消费随机决策。匹配不足时只抽取现有候选，
不会因筛选失败触发牌组耗尽败北。
`summon` 按数量依次创建实例并覆盖 `summoned`；`draw` 按本次抽取顺序移动实例并
覆盖 `drawn`。`add card ... to hand` 创建实例但不视为抽牌。批量操作只把
实际成功进入目标区域的实例写入输出绑定。
`destroy` 覆盖 `destroyed`，只记录本条操作实际破坏的目标；空结果也覆盖。
`count(destroyed)` 统计该结果，其他伤害或独立触发能力不会改写它。

`return T to deck` 在 `0..牌组长度` 的插入位置中等概率选择一个位置，只插入目标
而不改变其他牌的相对顺序。每次成功返回牌组消费一次规则层随机决策；返回手牌不
消费随机数。

`distributed` 只允许修饰 `damage`，目标必须为单侧的 `field.followers`。
可选的 `overflow` 主战者必须与该目标集合属于同一侧。先按出场顺序和当前生命值
分配额度，再应用屏障等伤害规则；余量交给显式指定的主战者，否则并入最后一个
随从的伤害。一次分配完成后统一处理死亡，分配不消耗随机决策。

`damage`、`heal`、`buff`、`destroy`、`banish`、能力增删等操作可作用于单值或
集合；集合按稳定顺序逐实例执行。对 `none` 的操作无效果。`destroy` 产生破坏、
死亡与谢幕曲相关事件，`banish` 不产生这些事件。`evolve ... silent` 是效果引起
的能力进化，增加 +2/+2；`superevolve ... silent` 增加 +3/+3 并赋予超进化形态。
两者均只改变场上未进化的随从，不支付点数或占用手动次数，不执行目标卡面的进化关键词能力，
但会发出一般进化事件供 `when self evolved` 等监听器在当前能力完成后处理。

`reanimate N` 从当前控制者的已破坏随从历史中选择原始费用不超过 N 且费用最高者，
创建原始状态的新实例、不触发入场曲，并在入场事件前赋予 `departed`（亡者）类型。
每条破坏记录各占一个等概率候选，同名卡牌不会去重；最高费用并列时消费一次随机
决策。无候选或战场已满时不创建实例、不消费随机数，`summoned` 绑定清空。
唯一候选无需随机数；召还不会消耗破坏记录，也不会从墓场移动旧实例。
`reduce cost` 不能低于
`minimum`。标准规则中最大能量点与战场容量分别为 10 和 5；截断或部分成功不
回滚先前操作。

`transform T into card C preserving materials` 将目标实例的卡牌定义替换为 `C`，
保留同一 `InstanceId`、区域位置和全部附着材料，不创建新实例。目标允许 `self`、
已定义的卡牌绑定，以及手牌、牌组、战场集合；不接受主战者、墓场、消失区或破坏历史集合。
操作可出现在入场曲等正常效果块及其条件、模式和重复块中。融合块内的自身变身必须显式写出
`preserving materials`；其他位置允许省略，省略不改变保留实例与材料的语义。
可选 `where` 写在目标卡牌 ID 和材料短语之后，在变身前按当前属性一次性筛选。
失效绑定中的非存续卡牌会被跳过；目标位于战场且结果为法术时同样不执行变身。

## 3. 测试文件

### 3.1 顶层与场景

```ebnf
wbotest_file      = "wbotest" , version , ";" , use_decl , scenario_decl ,
                    {scenario_decl} ;
use_decl          = "use" , "cards" , string , ";" ;
scenario_decl     = "scenario" , text_literal , "{" , seed_decl , state_decl ,
                    action_decl , expect_decl , "}" ;
seed_decl         = "seed" , integer , ";" ;
state_decl        = "state" , "{" , {state_statement} , "}" ;
state_statement   = "turn" , participant , integer , ";"
                  | "phase" , phase , ";"
                  | player_decl ;
phase             = "main" ;
player_decl       = "player" , participant , "{" , {player_statement} , "}" ;
```

`use cards` 的普通字符串按测试文件目录解析为卡牌根目录。每个场景相互隔离，
必须显式声明种子。0.1 场景固定依次包含 `seed`、`state`、`action`、`expect`。

### 3.2 玩家状态与实例

```ebnf
player_statement  = player_scalar_decl | zone_decl ;
player_scalar_decl = "leader" , stat_pair , ";"
                   | "pp" , stat_pair , ";"
                   | ("ep" | "sep" | "combo" | "shadows") , integer , ";" ;

zone_decl         = "deck" , "top" , instance_block
                  | zone_name , instance_block ;
zone_name         = "hand" | "field" | "graveyard" | "banished" | "destroyed" ;
instance_block    = "{" , {instance_decl} , "}" ;
instance_decl     = card_type , alias , "=" , card_id ,
                    (";" | instance_override_block) ;
alias             = identifier ;
instance_override_block = "{" , {instance_override} , "}" ;
instance_override = "stats" , stat_pair , ";"
                  | "counter" , counter_name , integer , ";"
                  | "cost" , integer , ";"
                  | "evolved" , ";"
                  | "super_evolved" , ";"
                  | ability , ";"
                  | "earthsigil" , integer , ";"
                  | "countdown" , integer , ";"
                  | "engaged" , boolean , ";" ;
```

`leader A/B` 表示当前生命与生命上限，`pp A/B` 表示当前与最大能量点。牌组实例
按 `deck top` 中从上到下的顺序排列。`graveyard`、`banished` 是当前区域，
`destroyed` 是本局随从与护符的破坏历史。实例覆盖只改变运行时状态，不改卡牌定义。

未声明状态采用 [测试 DSL](tests.md) 的默认值：双方主战者为 `20/20`，数值资源为零，
所有区域和历史为空，行动方为 `own`，阶段为 `main`。显式 `turn` 和 `phase`
覆盖对应默认值。

### 3.3 动作

```ebnf
action_decl       = "action" , "{" , primary_action , {action_response} , "}" ;
primary_action    = "play" , alias , ";"
                  | "engage" , alias , ";"
                  | "evolve" , alias , ";"
                  | "superevolve" , alias , ";"
                  | "attack" , alias , "into" , attack_target , ";"
                  | "end_turn" , ";"
                  | "advance" , advance_point , participant , ";" ;
attack_target     = alias | participant , "." , "leader" ;
advance_point     = "turn_start" | "turn_end" ;
action_response   = "select" , selected_target , {"," , selected_target} , ";" | "mode" , integer , ";" ;
selected_target   = alias | participant , "." , "leader" ;
```

一个 `action` 恰有一个玩家指令或测试驱动 `advance`，其后按引擎请求顺序给出
`select`、`mode` 响应。`attack`、`evolve`、`superevolve`、`end_turn` 均是正式
玩家指令；`advance` 只从指定规则时点驱动队列，不进入正式回放。没有对应请求、
缺少响应或存在多余响应都使场景格式或执行失败，而不是被静默忽略。

### 3.4 断言

```ebnf
expect_decl       = "expect" , "{" , {expect_statement} , "}" ;
expect_statement  = legality_assertion
                  | unchanged_assertion
                  | comparison_assertion
                  | ability_assertion
                  | count_assertion
                  | all_assertion
                  | order_assertion
                  | event_assertion ;

legality_assertion = "legal" , ";"
                   | "illegal" , identifier , ";" ;
unchanged_assertion = "unchanged" , ";" ;

comparison_assertion = assertion_ref , "==" , assertion_value , ";" ;
assertion_ref     = scalar_ref
                  | participant , "." , "leader" , "." , ("life" | "maxlife")
                  | alias , "." , instance_field
                  | alias , "." , "counter" , "." , counter_name
                  | "rng" , "." , "consumed" ;
instance_field    = "zone" | "stats" | "evolved" | "super_evolved"
                  | "earthsigil" | "countdown" | "engaged" ;
assertion_value   = integer | stat_pair | boolean | zone_value ;
zone_value        = "deck" | "hand" | "field" | "graveyard" | "banished" ;

ability_assertion = alias , ("has" | "lacks") , ability , ";" ;
count_assertion   = participant , "." , zone , "count" , "card" , card_id ,
                    "==" , integer , ";" ;
all_assertion     = "all" , participant , "." , zone , where_clause ,
                    "have" , ability , ";" ;

order_assertion   = participant , "." , "deck" , "top" , "==" , alias_list , ";"
                  | participant , "." , zone , "contains" , alias_list ,
                    "ordered" , ";" ;
alias_list        = "[" , [alias , {"," , alias}] , "]" ;

event_assertion   = "events" , event_matcher , event_fact_block ;
event_matcher     = "contains" , "ordered" | "excludes" | "exact" ;
event_fact_block  = "{" , {event_fact} , "}" ;
```

`events exact` 要求规则层事实序列与块内事实逐项、逐参数完全相等；
`events contains ordered` 只要求给定事实按顺序构成实际流的子序列；
`events excludes` 要求块中每个事实均不匹配实际流中的任何事实。空的 `exact`
断言表示实际事实流必须为空。区域移动、资源支付等未列事实仍会影响 `exact`，因此
运行器必须使用统一的规则事实模型，不能使用界面动画或本地化日志。

### 3.5 事件事实

```ebnf
event_fact        = damage_fact | heal_fact | draw_fact | summon_fact
                  | object_fact | return_fact | evolve_fact | combat_fact
                  | engage_fact | turn_fact | zone_fact | resource_fact ;

damage_fact       = "damage" , fact_object , integer , ";" ;
heal_fact         = "heal" , fact_object , integer , ";" ;
draw_fact         = "draw" , participant , integer , ";" ;
summon_fact       = "summon" , (alias | "card" , card_id , "count" , integer) , ";" ;
object_fact       = ("destroy" | "banish" | "discard") , fact_object , ";" ;
return_fact       = "return" , fact_object , "to" , ("hand" | "deck") , ";" ;
evolve_fact       = ("evolve" | "superevolve") , alias , ";" ;
combat_fact       = "attack" , alias , "into" , attack_target , ";" ;
engage_fact       = "engage" , alias , ";" ;
turn_fact         = ("turn_start" | "turn_end" | "game_end") , participant , ";" ;
zone_fact         = "move" , alias , "to" , zone_value , ";" ;
resource_fact     = ("gain" | "spend") , scalar_ref , integer , ";" ;
fact_object       = alias | participant , "." , "leader" | "card" , card_id ;
```

事实中的别名优先指向场景已有实例；对效果新建实例，可用卡牌 ID 与数量形式聚合
匹配。`summon card ... count ...` 只聚合同一次召唤操作的成功实例。实现可记录更多
事实种类，但在 0.1 的 `events exact` 中必须把这些事实按上述规范映射为稳定序列，
无法映射的内部日志不得泄漏到规则事实流。

## 4. EBNF 之外的检查器规则

以下规则不能仅靠上下文无关文法完整表达，解析器、卡牌库检查器或测试运行器必须
实施。

1. `.wbo` 恰含一张卡；文件名去扩展名后应等于卡牌 ID，同一卡牌根目录内 ID
   唯一。独立语法检查允许引用尚未随当前语料提供的外部卡牌；可执行加载时，所有
   实际可达的 `card` 引用都必须能从所加载卡牌根目录或显式依赖中解析，包括
   Token。
2. `follower` 必须且只能声明一次 `stats`；`spell`、`amulet` 不得声明
   `stats`。`countdown`、`earthsigil`、`engage` 只允许用于语义上适合的护符；
   随从专属能力和进化操作必须满足对象类型。
3. `cost`、费用、数量、资源、倒计数、选项号、回合号及种子必须落入实现声明的
   非负整数范围，解析过程必须检测溢出。基础攻击和生命值必须为非负；增减只使用
   显式带符号数。`minimum` 不得大于操作前允许的费用上界。
4. `meta.pack` 必须与卡牌所属包一致；`class`、`rarity`、`trait` 必须来自当前
   卡牌库登记表。0.1 已出现的职业与稀有度标识不能因本地化名称改变。
5. `effect`、能力块及所有控制块中的语句严格从上到下执行。操作失败只使该操作
   无效果或部分成功，不回滚此前操作、费用或资源支付。触发能力按引擎规定进入
   队列；同时死亡先形成同一死亡批次，再将谢幕曲排队。
6. 法术 `effect` 最外层操作在使用时执行；随从和护符的执行时点由能力块声明。
   检查器应拒绝在卡牌类型或作用域中无执行时点的最外层语句，而不是猜测时点。
7. 同一 `effect` 中各类唯一能力的重复规则由能力登记表决定。一个
   `superevolve replaces evolve` 或 `extends evolve` 必须有同一 `effect`
   中的普通 `evolve`。普通 `superevolve` 在手动超进化时与普通 `evolve`
   分别触发；`replaces` 不触发普通块；`extends` 先完整执行普通块再执行扩展块。
8. `self` 在能力或法术结算期间指效果来源；`own` 是其当前控制者，`oppo` 是
   对手。绑定具有所在效果调用帧的词法作用域：内层块可读外层绑定；同名新绑定
   覆盖当前帧旧值，离开帧后恢复外层值。
9. `choose`、`require`、`random` 的 `binding_name` 建立或覆盖绑定；当前语料
   使用稳定名 `target`。操作输出 `summoned`、`drawn`、`destroyed` 和事件输出同样是绑定。
   使用前必须在所有可达控制流上已定义，`superevolve extends evolve` 是允许
   读取普通进化块输出绑定的特例。绑定值必须满足操作所需的单值、集合和类型。
10. 新的召唤、抽牌或破坏操作即使成功数为零，也以对应空集合替换旧的 `summoned`、
    `drawn` 或 `destroyed`。事件绑定只在对应监听器调用帧有效。实例离开原区域后，绑定和测试
    别名仍指向同一实例。
11. `where` 的字段必须适用于候选类型；例如 `life` 只适用于随从，`trait` 只
    匹配具有该种族的卡牌。应用于操作的 `where` 先过滤其紧邻集合操作数，再按
    稳定集合顺序执行。对单实例附加 `where` 是类型错误。
12. `replace` 只允许作为具有场上实体的卡牌能力，且来源必须是 `self`。替换块
    执行期间应设置重入标记；它产生的离场不能再次进入同一替换块。原离场取消后，
    替换块造成的新移动仍正常产生其自身允许的事件。
13. 手动进化支付并检查 `ep`，手动超进化支付并检查 `sep`；静默进化不支付且
    不触发目标的进化能力。进化后的随从保留已有关键词和触发能力。每个护符实体
    每个控制者回合最多成功 `engage` 一次，入场回合可启动。
14. 吟唱在控制者回合开始时减少；变为零立即破坏护符并可触发谢幕曲。土之印
    合并保留新实例、累加层数，旧实例移入 `banished`，不产生破坏、谢幕曲或墓场
    计数。魔力增幅计数属于手牌实例，费用下限为零。
15. 测试场景内所有别名全局唯一，包括双方及所有初始区域。别名声明的
    `card_type` 必须与卡牌定义一致；同一实体不能同时出现在两个当前区域。
    `destroyed` 历史项可与当前区域中的实例不同，但历史别名仍不得重复。
16. `deck top` 是完整声明的初始牌组，不是未列尾部的前缀。列表和区域顺序断言
    使用实例身份，不按卡牌 ID 合并。新实例的确定顺序是产生操作内的创建顺序。
17. 一个场景的 `legal` 与 `illegal` 只能出现其一且不得重复。`unchanged` 仅可
    与 `illegal` 联用，比较动作前后全部可序列化状态及随机数生成器状态。非法
    指令不得支付费用、发出事实、消费随机数或改变请求队列。
18. `select` 必须匹配下一个目标请求且别名属于其候选集合；`mode` 必须匹配下
    一个模式请求且编号存在。`require` 无候选时，主动作在任何状态变更前判定
    非法。测试动作结束时不得遗留未响应请求。
19. 断言左右值必须类型一致。`own.pp == A/B` 同时比较 `pp` 与 `maxpp`；
    `alias.stats`、`leader` 使用攻击/生命对，区域、布尔、整数不得混比。
    `count`、`all` 和顺序断言必须引用允许查询的区域。
20. 所有随机决定使用场景种子初始化的对局随机数生成器。空候选、非法指令、
    无需打破并列的确定性选择不得消费随机决策。随机插入牌组、非空随机目标和
    需要打破并列的选择各消费一次规则层随机决策。`wbo-standard-0.3.0` 使用
    SplitMix64 v1；同一规则集、种子和有序候选集合必须得到相同结果。
21. 本地化 `name`、`text` 只用于展示，规则引擎不得解析其内容。规范检查器可以
    比较展示文本与规则，但比较结果不能改变规则执行。
22. `fusion_block` 的材料过滤器必须适用于手牌卡牌，来源必须是 `own.hand`。一次
    融合选择至少一个互不相同的实例；材料按选择响应中的顺序附着到来源。附着实例
    不进入 `graveyard` 或 `banished`，不触发普通区域移动、破坏、消失或谢幕曲。
    变身保留来源 `InstanceId` 和完整材料序列。
23. `90071210`、`90071220` 开始的融合变身链必须完整解析至 `90074110`。变身保留
    材料，因此后续 `fused.cost` 和 `fused.distinct` 包含变身前已经附着的材料。

## 5. 歧义消解

1. 词法分析采用最长匹配；`<=`、`>=`、`==`、`!=` 优先于单字符符号。关键字
   仅在完整 `identifier` 边界上匹配，区分大小写；例如 `attacker` 不是
   `attack` 加后缀。
2. `"""` 在可开始字符串的位置优先于 `"`。注释识别发生在字符串识别之外；
   字符串中的 `//`、`/*`、`*/` 都是内容。
3. `where` 绑定到它左侧最近的 `target_set` 或集合操作数；`and` 表示交集，
   `or` 表示并集，`and` 优先于 `or`。当前过滤器不支持括号分组。
   `other` 在过滤前应用。没有隐式跨语句过滤器。
4. `else` 绑定到同一 `if_statement` 中紧邻且尚无 `else` 的 `if`。由于各分支
   必须带花括号，不允许悬空语句形式。
5. `add` 后接整数时解析为卡牌或资源操作，后接 `ability` 时解析为能力操作；
   `remove` 只有能力操作。不得用任意标识符猜测新能力。
6. `spellboost` 后接 `{` 时是能力块，后接 `value_ref integer ;` 时是操作。
   `evolve` 后接 `{` 时是能力块，在 `.wbotest` 动作上下文是玩家动作，在效果
   上下文后接 `value_ref silent ;` 时是静默进化操作。
7. `attack` 在卡牌 `effect` 中后接 `{` 时是能力块，在测试 `action` 中后接
   `alias into` 时是动作，在事件事实块中是事实。解析器必须以所在非终结符上下文
   决定，不进行语义回溯。
8. `all` 在 `draw` 后是数量，在 `expect` 语句开头是全称断言。`exact` 只在
   `events` 后是事件匹配器。`graveyard`、`banished` 在状态块中是区域声明，
   在路径或断言值中是区域名。
9. 点号路径作为一个左结合的语法结构解析，不允许省略点或插入隐式所有者。
   `field.followers` 是双方战场集合，`own.field.followers` 是己方集合，两者不可
   根据上下文互换。
10. 花括号始终由其引入关键字配对；分号只终结声明、操作、动作、事实和断言，
    不跟在块声明之后。解析器不得自动插入分号。

## 6. 版本与向后兼容

`.wbo` 与 `.wbotest` 分别以 `wbo version;`、`wbotest version;` 声明语言版本，
两者版本独立协商。本文定义 `0.1.x` 系列：主版本为 `0`，次版本为 `1`。修订号
用于不改变既有合法文件语义的勘误、诊断改进和兼容性澄清。

0.1 解析器必须精确接受其声明支持的 `0.1.x`，不得把未知关键字、字段、能力、
事件、过滤器或断言当作普通 `identifier` 后继续执行。实现可接受不高于自身支持
修订号的同次版本文件；面对更高修订号时，只有在实现声明该修订向后兼容时才可
执行，否则必须在规则执行前拒绝。

任何新增可执行语句、改变操作顺序、绑定生命周期、随机数消费、事件事实序列或
既有语法含义的改动，至少提升次版本。删除或不兼容地重解释既有 0.1 语法时必须
提升主版本。新版本可以增加显式迁移工具，但不得在未声明版本转换的情况下猜测
旧文件意图。

卡牌文件、测试文件和运行器应在加载阶段校验版本组合。测试运行器加载卡牌时，
必须确保所有参与场景的卡牌版本均受支持；版本错误、缺失引用和静态语义错误应在
执行任何场景前报告。0.1 不承诺未来草案接受所有当前错误写法，但承诺本文列出的
合法表面语法不会在同一 `0.1.x` 系列中被静默赋予不同语义。
