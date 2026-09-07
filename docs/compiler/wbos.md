# WBOS 编译容器 0.1 草案

`.wbos` 是 WBO Serialized Package 的统一后缀，用于保存已经通过检查、可由工具或
运行时加载的编译产物。源文件仍使用 `.wbo` 和 `.wbotest`；`.wbos` 不作为作者语言，
运行时不得从中恢复并重新解释源文本。

## 设计目标

- 使用一个稳定后缀承载卡牌包、测试包和规则集。
- 分离容器版本、IR 版本、字节码版本与规则集版本。
- 支持当前规范 JSON 编码，并为后续二进制编码保留兼容边界。
- 对包类型、依赖、必需功能和内容哈希进行加载前验证。
- 保留稳定来源映射，但允许发布构建剥离源码内容和调试信息。
- 相同输入和编译选项必须生成逐字节相同的产物。

## 文件命名

推荐名称：

```text
base-cards.wbos
base-tests.wbos
standard-ruleset.wbos
```

后缀不表示包类型。加载器必须读取容器中的 `kind`，不得根据文件名猜测内容。

## 0.1 JSON 编码

0.1 使用 UTF-8 规范 JSON，顶层结构为：

```text
WBOS = {
  format: "wbos",
  containerVersion: SemVer,
  encoding: "canonical-json",
  kind: PackageKind,
  irVersion: SemVer?,
  bytecodeVersion: SemVer?,
  requiredFeatures: [string],
  compiler: CompilerIdentity?,
  ruleset: RulesetDependency?,
  sources: [SourceFile],
  cards: [Card]?,
  scenarios: [TestScenario]?,
  contentHash: ContentHash
}

PackageKind = "card-pack" | "test-pack" | "match-ruleset"
```

当前卡牌包必须包含 `cards`，测试包必须包含 `scenarios` 与 `ruleset`。不适用于该包
类型的负载字段必须省略，不能写成 `null`。0.1 尚不嵌入规则字节码，因此
`bytecodeVersion` 省略。

`requiredFeatures` 是运行时必须理解的语义功能标识。未知功能必须使加载失败，不能
静默忽略。列表按 UTF-8 字节序排序且不得重复。

## 规范编码

规范 JSON 遵循以下规则：

1. 文件编码为 UTF-8，无 BOM，使用两空格缩进并以一个换行结束。
2. 映射键按 UTF-8 字节序输出；哈希输入使用相同字段顺序的紧凑编码。
3. 数组保持规范定义的顺序；来源按规范路径排序，场景按文件路径及声明顺序排列，
   无语义顺序的集合数组在编译期显式排序。
4. 禁止浮点数、重复键和无意义空字段。
5. `u64` 使用 `0x` 前缀的 16 位小写十六进制字符串。
6. 内容哈希计算时先把顶层 `contentHash` 设为空字符串，再对紧凑规范 JSON 计算
   SHA-256，最终写成 `sha256:` 加 64 位小写十六进制。

当前加载器对单个 JSON WBOS 设置确定的 16 MiB 上限，并在结构体解码前递归扫描完整
JSON 值。非 UTF-8 输入、任意层级的重复对象键以及首个 JSON 值后的任何尾随值都会
被拒绝；这些错误不能由后出现的键覆盖，也不能进入内容哈希或运行时验证阶段。

当前 Go 实现的映射编码具有确定顺序，但还没有独立的跨语言规范 JSON 编码器；在
其他语言生产 `.wbos` 前，必须补齐并用金标准文件验证这一点。

## 版本边界

四种版本不能互相替代：

| 字段 | 控制内容 | 不兼容时的处理 |
| --- | --- | --- |
| `containerVersion` | 外层字段、编码和分区规则 | 未知主版本拒绝 |
| `irVersion` | 类型化 Card/Test 数据模型 | 未知主版本拒绝 |
| `bytecodeVersion` | 指令、验证和 VM 语义 | 未知版本拒绝 |
| `ruleset.contentHash` | RNG、排序和全局对局规则 | 哈希不符拒绝 |

次版本只能增加由 `requiredFeatures` 保护的可选能力。任何改变既有字段或指令语义的
修改都必须提升对应主版本。加载器不得通过猜测版本来修复产物。

## 依赖与链接

卡牌引用在写入完整 CardPack 前必须解析。测试包必须锁定：

- 精确规则集 ID 和内容哈希。
- 所需卡牌包 ID、版本和内容哈希。
- 测试所引用的全部卡牌 ID。

链接器必须拒绝缺失引用、重复卡牌 ID、依赖环和同版本不同哈希。开发构建可以显式
产生带 `unresolvedReferences` 的分析包，但这种包必须标记为不可执行，运行时必须
拒绝加载。

## 调试与来源

开发包可保存：

```text
SourceFile
SourceSpan
NodeId
展开来源链
字节码 PC 到 NodeId 的映射
```

发布包可以剥离源文件正文，但必须保留稳定 NodeId、卡牌 ID、能力 ID 和足够的运行时
诊断信息。调试信息是否剥离属于编译选项，必须进入内容哈希。

## 未来二进制编码

二进制编码使用同一 `.wbos` 后缀，并以固定文件头识别：

```text
offset  size  content
0       4     ASCII "WBOS"
4       2     container major, little-endian
6       2     container minor, little-endian
8       1     package kind
9       1     encoding
10      2     flags
12      4     section count
16      32    SHA-256
48      ...   section directory
```

计划分区：

```text
MANIFEST  STRINGS  SOURCES  CARDS  CONSTANTS
FUNCTIONS BYTECODE TRIGGERS DEBUG   TESTS
```

每个分区目录项必须包含类型、标志、偏移、长度和分区哈希。未知可选分区可以跳过；
未知必需分区必须拒绝。二进制版冻结前需要先确定整数编码、对齐、分区排序和规范哈希，
0.1 实现不得提前输出看似稳定的实验二进制文件。

## 安全加载顺序

加载器按以下顺序处理：

1. 限制文件大小并识别编码。
2. 解析容器头，验证包类型和版本。
3. 验证内容哈希及依赖哈希。
4. 验证必需功能、IR 或字节码。
5. 建立卡牌和触发索引。
6. 验证规则集完全匹配后才允许创建对局。

任何失败都不能产生部分可用的规则包。

## 当前实现与迁移

当前编译器输出 WBOS 0.1 的规范 JSON 形态。降低阶段直接构造严格 Go `CardPack`、
`TestPack` 和上下文相关的判别联合类型，再由统一编码器计算规范内容哈希并序列化。
`wbo test` 会把卡牌依赖与场景编译为内存 WBOS，完成序列化、严格解码、跨包链接和
精确规则集依赖校验后才执行，不再读取项目或语法 AST。当前 IR 解释器通过 Session
边界执行只读沙箱预检，并在非空目标或模式节点返回请求；测试响应只会交给当前请求，
候选快照、动作 ID 和状态修订不匹配时不会修改状态。暂停状态可通过
`EncodeContinuation` 输出版本化 JSON，由 `DecodeContinuation` 严格解码并通过
`RestoreSession` 在相同卡牌包和规则集上恢复；执行栈使用卡牌、能力和节点稳定 ID，
不保存 Go 内存引用。当前 Continuation 版本为 `0.15.0`，完整保存卡牌命名计数器、模式选项标签、事件可见范围、本回合融合状态、攻击次数、入场等待、攻击阶段、
玩家回合攻击历史、正在结算的法术和重复块检查点；旧版本不做猜测性迁移。默认规则集
`wbo-standard-0.3.0` 还固定执行预算，TestPack 的
规则集依赖必须包含全部预算字段；预算计数随 Continuation 保存，耗尽后产生终止性的
`execution_budget_exceeded`。字节码加入后，类型化 IR 仍作为编译器中层表示，不要求
在发布包中长期保留。
