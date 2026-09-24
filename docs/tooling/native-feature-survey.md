# 原版客户端功能可行性盘点

本文回答一个问题：原版客户端里那些「游戏功能」，我们离线接管服务端之后，哪些能做、
各自要做什么。结论来自三份证据，不靠猜：

1. **路由清单**：从运行中的客户端（1.9.1.18238）内存 dump 里抓出的 824 条 API 路由
   （`/Prefix/action`，过滤掉了类型名之类的噪音）。下面每条结论都指到具体路由。
2. **DTO 契约**：`native/proto/contracts/object-schemas.json`（181 个 schema，delta 提的）
   与 `dump.cs` 里的 `XxxRequest` / `XxxResponse` 字段表。
3. **资源侧事实**：客户端资源清单 `assetbundle.<语言>.manifest` 的表结构，以及
   `SerializedFile`/raw 资源的寻址方式。

## 两个贯穿全局的资源事实

先把这两条摆出来，后面每个功能都要用：

- **raw 资源是「名字寻址 + 只校验大小」**：`raw_assets` 有 5108 条，分四类
  （Sound 4769、Movie 318、Others 20、Master 1）。它们没有 `checksum` 字段，
  客户端拿名字在清单里查出文件名（26 位 base32）再去 `<前两位>/<名字>` 读文件。
  我们把清单和文件都控制在自己手上（`wbo native provision`），所以**理论上可以换内容**。
  待验证：这条还没实测过，而且文件的 26 位名字不是 `base32(MD5(内容))`
  （拿 `Master/mastermemory.bytes` 验过，两种哈希都对不上），换内容时要不要重算名字得先试一次。
- **prefab / 贴图在客户端自带的 `data.unity3d` 里**（478 MB，UnityFS / 2022.3.62f2）。
  标题画面的 `Prefab/UI/Title_<id>` 与它引用的贴图（`utx_title_chara_*`、
  `bg_title_11007_*` …）都在里面，清单里没有任何 `Prefab/` 条目。
  **不用 Unity 编辑器也能改**：UnityPy 能读出贴图、替换后回写 bundle，本机已实测
  （712×508 的贴图替换 → 回写 478 MB → 回读内容正确）。

还有一个已知未解：master 表（`Master/mastermemory.bytes`，201 张表）的**表体**是自定义
扩展编码（MessagePack ExtType(99)），看着像 `LibNative.LZ4.SimpleLZ4Frame`
压缩过的那套，标准 LZ4 block/frame 都解不开。要改 master（卡池、BGM 表、商店表都在里面）
得先把这层解掉——这是一块独立的、边界清楚的小逆向。

## 逐项结论

### 1. 登录界面

客户端侧是标题画面 →「点击开始游戏」→ 账号关联/数据引继 → 主界面，相关 prefab
（`UIDialogTitleMenu`、`UIDialogDataLinkage`、`UIDialogAccountLinkConfirm`…）都在包里。

服务端接口：

| 组 | 路由 |
| --- | --- |
| 引导链 | `/Version/info`、`/Session/start`、`/StartUp/{index,acceptAgreements,confirmParentConsent,confirmAppTrackingTransparency}`、`/Load/index`、`/Title/index` |
| 账号 | `/Account/{signUp,signIn?,getRandomNames,updateName,updateBirth,updateLanguage,updateResidentCountry,getCountryAgeGroup,getGameStartCountryAgeGroup,getEpicGamesLoginParams}` |
| 关联 | `/Account/{getAuthURL,getAuthToken,linkSocialAccount,getBySocialAccount,unlinkSocialAccount,getByIcloudAccount,updateByIcloudAccount}`、`/CygamesId/*`(6)、`/OneTimePassword/*`(8) |

**结论：能做，且是当前主线。** 引导链的 8 条已经跑通；剩下的是账号本身——按 WBCapture
那套设备码流程给 WBO 单开一组端点（`device/start` → 浏览器授权 → `device/poll`），
`wbo login` 拿令牌，`wbo native launch` 把它带给 broker，档案层按 WBArts 用户存。
另外 `/Load/index` 把 `usable_transition_account_exists` 置 true，「推荐进行账号关联」
那个弹窗就不再出现。

### 2. 背景音乐

曲目是 raw 资源（`sound/Windows/**/*.pck`，4769 条，CRI 容器）。选谁播由两处决定：
标题画面看 `/Title/index` 的 `TitleData.sound_id`（对应 `mx_title_*`）；场景内看
master 的 `HomeBgmMaster`（场景 → cue）与用户设置（`bgm_id`/`bgm_list`/
`shuffle_bgm_list`/`is_shuffle_bgm`，走 `/Metaverse/updateBgm` 这类路由）。

**结论：**

- **在官方曲目里换/加**：简单。改 `/Title/index` 的 `sound_id`、把 BGM 设置路由照实回。
- **换成自己的曲子**：要产出 CRI 的 `.pck`（自制音频得先解决容器），再走 raw 资源的
  替换（上面那条待验证的机制）。比贴图那条路更麻烦，除非我们只针对已有曲目做重排。

### 3. 界面更换（主界面插图 / 背景）

客户端侧：大厅背景是「主界面插图」（Spine 立绘 + 背景图），可用集合来自
`/HomeIllustration/getList`（`HomeIllustration{illustration_id}`），槽位/购买/扩展/
排序/保存走 `/ArtCollection/*`(9)。主战者、卡背、卡面样式 WBO 已经实现了
（`/LeaderSkin/*`、`/Sleeve/*`、`/CardStyle/getSettings`）。

**结论：**

- **在官方已有插图里任意更换**：几乎零成本——`/HomeIllustration/getList` 回「全部拥有」，
  `/ArtCollection/*` 按契约应答。
- **换成自己的图**：和标题图同一类问题（资源在客户端包里），走已经验证过的 UnityPy
  改贴图回写那条路。

### 4. 抽卡模拟

客户端侧整台演出机器都在（`CardPackOpen*` 系列 prefab 与特效）。服务端接口是
`/CardPack/{info,open,detail,readGacha,getCeilingRewardList,exchange,exchangeCeilingReward,
getExchangeableItemList,declineTreasureBoxChance,receiveTreasureBoxChance,
updateWelcomeCardPackId}`。

形状很清楚：`/CardPack/open` 的请求只有 `gacha_id`、`cost_type`、`purchase_num`，
响应给 `card_pack_result`（外加保底、宝箱、赠送奖励那几项）；
`/CardPack/info` 给 `gacha_list`、硬币与免费次数。

**结论：完全能做，而且是最划算的一块。** 抽到什么本来就是服务端说了算；客户端只负责
演出。卡表我们有（`cards/` + `data-source/`），缺的只是「卡池定义 + 结果」这两层
DTO（现在的 `object-schemas.json` 不含这批，用 dumper 的 DTO 直接生成即可）。
顺带一提，客户端里还留着一整套调试路由（`/Debug/Debug/addAllCards`、
`drawDebugGacha`、`finishTutorial`、`addAllCoins`…），自测时可能用得上。

### 5. 商店界面

接口分三层：商店 `/Shop/{shopList,readProducts,purchase,purchaseSetProduct,
createPremiumSupply,bnr,utx}`；支付 `/Payment/{productList,coinInfo,purchaseLimitation,
start,finish,cancel,readProducts,steamMicroTxnInit,sendLog}`；还有
`/WorldShop/*`(4)、`/ItemPurchase/{info,purchase}`、`/ExchangeTicket/*`、
`/Dlc/*`、`/Gift/*`。

**结论：**

- **浏览 + 用游戏内货币/道具兑换**：能做。商店列表、价格、购买结果都是服务端给的，
  我们自己定义一套「本地商店」完全可行。
- **真金充值**：不做。`/Payment/steamMicroTxnInit` 要走 Steam 微交易、`/Payment/finish`
  要官方收据，我们没有也不应伪造；这部分要么隐藏、要么明确回「离线不可用」。

### 6. 网络对战

接口：`/RoomMatch/{createRoom,enterRoom,enterRoomAsWatcher,acceptRecovery,
declineRecovery,battleFinish,getShareMessage}`、`/PrivateLobby/*`(17)、
以及服务端内部的 `/Internal/InternalRoomMatch/{create,enterAsWatcher,battleStart,
close,leave,updateSettings,watchBattle,…}`。

真实形态是 MagicOnion（HTTP/2 + 推送 hub）：建房/进房之后客户端维持一条长连接，
房间状态靠推送，开打时拿 `battle_url` 转去对局通道。delta 把这套逆完了
（`research/native-room-contract/contract.json` 536 KB、
`protocol-contract/battle-service-contract.json`、`object-schemas.json`），
WBO 目前只搬了 schema，**房间契约还没搬**。

**结论：能做，是继登录之后的下一大步。** 三步：搬房间契约 → 实现房间/匹配/推送
（Go 侧新包）→ 把对局挂到 WBO 自己的引擎（`internal/engine/runner`）。
delta 的 `local_native_room*.py`(5) + `local_native_duel*.py`(2) +
`local_native_battle.py` 是可对着读的参考实现。

### 7. 广场赛（以及同一族的赛程/广场）

这里其实是三个不同量级的东西，得分开看：

| 名称 | 路由 | 是什么 | 结论 |
| --- | --- | --- | --- |
| 大厅锦标赛 | `/LobbyTournament/{entry,cancelEntry,getInfo,getJoinedTournamentInfo,retire,battleFinish}` + `/PrivateLobbyTournament/*`(5) | 大厅里报名的赛事：报名、赛程、轮次、弃权，对局走同一套对局通道 | 能做，工作量 = 一个赛程/轮次模型 |
| 官方大会 / Grand Prix | `/OfficialTournament/*`(17)、`/Grandprix/*`(11)、`/GrandprixTwoPick/*`(8)、`/ArenaTwoPick/*`(18) | 赛程 + 匹配 + 双选选牌，同样是纯服务端逻辑 | 能做，但属于「一个赛程引擎」，建议在大厅锦标赛之后 |
| 广场本体（Metaverse / 自室） | `/Metaverse/*`(25)、`/MetaverseLobby/*`、`/MtvPrivateLobby/*`、`/House/*`、`/Ajito/*` | 3D 广场、化身、家具、BGM、NPC 与看板 | 另一个大模块：客户端资源都在包里，服务端只要给「进哪个广场、有谁、什么状态」；建议排在网络对战之后 |

## 建议顺序

1. **登录 + 账号一体**（已经在做）：WBArts 单开 device 端点 → `wbo login` → broker 绑身份 →
   档案层按用户存 → 关掉账号关联弹窗。
2. **抽卡模拟**：纯服务端逻辑、演出现成、不碰资源，能最快看到「像原版一样」的效果。
3. **网络对战**：搬房间契约、实现房间与推送、接上自己的引擎——这是把整个东西变成
   「能玩」的关键一步。
4. **商店（不含充值）+ 界面更换（官方集合内）**：都是补丁级工作量。
5. **赛程类（大厅锦标赛 / Grand Prix / 双选）**：等对局通道稳了再上，复用同一套对局。
6. **广场本体**：独立模块，最后做。

## 待办的小逆向（做上面任何一条之前都值得先落地）

- raw 资源替换实测：换一个 `sound/*.pck` 或 `Master/mastermemory.bytes` 的内容，同步清单
  的「名字 + 大小」，看客户端是否照单全收。这条决定「自制音频/自定义数据」能不能走通。
- master 表体解码：ExtType(99) 的 LZ4 帧（线索：`LibNative.LZ4.SimpleLZ4Frame`，
  RVA 可以在 `dump.cs` 里查到）。解开之后就能读/改卡池、BGM 表、商店表。
- DTO 批量导出：把 `dump.cs` 里的 `XxxRequest/Response` 字段表批量转成 schema，
  就不用一条路由一条路由地手抄（现在 181 个 schema 只覆盖对局那一片）。
