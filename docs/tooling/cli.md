# WBO Go 工具

`wbo` 是 WBO DSL 的检查、格式化、编译、规则场景执行和交互模拟工具。当前实现使用 Go
标准库。

## 运行

开发期间可以直接运行：

```bash
go run ./cmd/wbo check cards tests
```

也可以构建本地二进制：

```bash
go build -o wbo ./cmd/wbo
./wbo check cards tests
```

## 检查

```bash
wbo check [--strict-references] [--source-root PATH] PATH...
```

`check` 会递归读取 `.wbo` 和 `.wbotest`，检查词法、语法、字段顺序、效果结构、
绑定作用域、测试状态、动作、断言、卡牌类型和引用。

默认情况下，尚未提供定义的卡牌引用产生警告，命令仍返回成功。发布或 CI 应启用
严格引用：

```bash
wbo check --strict-references cards tests
```

退出码：

- `0`：没有错误，可以有非严格引用警告。
- `1`：存在语法、语义、引用或文件错误。
- `2`：命令或参数用法错误。

## 格式化

查看单个文件的规范格式：

```bash
wbo format cards/10000/10001110.wbo
```

原子改写一个或多个文件：

```bash
wbo format --write cards/10000/10001110.wbo tests/10000/base-rules.wbotest
```

格式器只处理能够完整解析并通过单文件语义检查的文件。格式化结果具有幂等性。
格式器会保留并规范缩进 `<<` 与兼容的 `//` 行注释、移除块注释，并拒绝改写符号链接。

## 编译

卡牌和测试必须分别编译，不能混在同一个包中：

```bash
wbo compile --source-root . --output cards.wbos cards
wbo compile --source-root . --output tests.wbos tests
```

默认编译要求所有引用都能解析。开发未完成的卡包可以显式允许未解析依赖：

```bash
wbo compile \
    --source-root . \
    --allow-unresolved \
    --output cards.wbos \
    cards
```

此时输出顶层的 `unresolvedReferences` 会列出缺失卡牌 ID，产物不能视为可发布的
完整卡包。

`--source-root` 决定包内规范相对路径和稳定 `SourceId`。CI 与本地构建应始终传入
同一个项目根目录，避免因调用位置不同改变来源标识。

编译输出统一使用 `.wbos` 后缀；省略 `--output` 时仍可把内容写到标准输出。0.1 容器
使用两空格缩进的确定性 JSON：

- `format`：`wbos`
- `containerVersion`：`0.1.0`
- `encoding`：`canonical-json`
- `kind`：`card-pack` 或 `test-pack`

完整容器约定见 [WBOS 编译容器](../compiler/wbos.md)。

卡牌效果会转换为类型化节点，例如目标集合、过滤器、触发能力、超进化执行计划、
融合能力和变身操作。测试场景会物化初始状态、实例 ID、动作和断言。输出不包含
需要运行时再次解释的源 token 数组。

## 验证

执行测试场景：

```bash
wbo test --ruleset wbo-standard-0.2.0 --source-root . tests
```

`test` 会加载测试声明的卡牌依赖，把依赖卡牌和测试分别编译为内存中的 WBOS
`CardPack` 与 `TestPack`，再经过严格 JSON 解码后执行类型化 IR；执行器不读取源 AST。
它按场景种子执行动作并检查全部断言。默认规则集使用 SplitMix64 v1；随机目标和
牌组随机插入仅在实际发生决策时推进一次。失败场景、不兼容 WBOS 或不支持的规则集
返回退出码 `1`。

每个场景的首个动作通过只读沙箱预检后开始结算；后续 `select` 和 `mode` 仅在执行器
实际产生对应请求时消费。缺失、多余、过期、类型不符或不在候选快照中的响应都会使
场景失败。非空玩家选择会暂停显式执行栈，响应验证通过后从同一绑定帧继续。

当前执行器覆盖现有 15 个基础场景所需的打出、启动、超进化、模式/目标选择、触发、
资源、区域移动、回合结束准备、测试驱动的 `advance` 和断言。攻击及融合玩家指令
尚未进入执行器范围。

## 交互模拟

```bash
wbo simulate --source-root . --scenario "场景名称或 ID" cards tests
```

`simulate` 使用场景的初始状态和种子建立会话，先向 stdout 输出一行 JSON 状态，随后
从 stdin 按行读取 JSON 命令，每条命令后再输出完整状态。状态只展示当前观察者的手牌，
对手手牌和双方牌组只公开数量。输出同时包含当前合法动作和能力声明，界面不能把值为
`false` 的动作显示为可用功能。

```json
{"kind":"play","source":"5d951ae351ad8760d582585950e16c89"}
{"kind":"select","selectedInstanceIds":["目标实例 ID"]}
{"kind":"select_mode","selectedOptionId":1}
```

省略主动作 `actionId` 时，命令行会按输入顺序生成确定性 ID。选择响应可以省略请求 ID、
动作 ID 和状态修订，命令行会使用当前待处理请求；网络服务仍应显式传递并核对这些字段。
当前模拟接口正式支持打出、启动、超进化、结束回合、目标选择和模式选择。攻击、普通进化
和融合会返回 `unsupported_feature`，不会用未成文规则猜测结果。结束回合完成后会切换到
对方回合；当前动作入口仍只允许 `own` 提交指令。

## 开发验证

```bash
go test -count=1 ./...
go vet ./...
```

集成测试会检查当前全部卡牌和测试场景、严格引用、畸形输入拒绝、格式化幂等、
类型化 IR、确定性编译和固定源根下的来源 ID 稳定性。

## 当前限制

- 尚未执行攻击和融合玩家指令。
- 浏览器 WASM 批量接口尚未实现。
