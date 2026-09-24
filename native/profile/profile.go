// Package nativeprofile 是"玩家的档案"这一层：收藏、牌组、领袖皮肤、卡垫。
//
// 它回答的是客户端在**进入主界面之后**问的那批问题（/Card/getList、/Deck/getList、
// /Mypage/index……）。这些响应不依赖规则引擎，但也不能是空壳：客户端会拿收藏里的
// 卡片去建牌组、拿牌组去开练习对局，空收藏会让它退回标题界面。
//
// 这一层是 delta 的 tools/local_practice_profile.py 的 Go 移植。契约（路由、模板、枚举、
// 卡表）来自 delta 从原客户端 CardMaster 提取的那份，逐字节拷进 contracts/；语义
// （谁能进收藏、牌组怎么校验）照 delta 的判定搬，不重新发明。
package nativeprofile

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

//go:embed contracts/*.json
var contractFiles embed.FS

// Card 是 CardMaster 里的一行，只保留这一层判断需要的字段。
type Card struct {
	ID     int64 `json:"id"`
	Base   int64 `json:"base"`
	Set    int64 `json:"set"`
	Foil   int   `json:"foil"`
	Normal int64 `json:"normal"`
	Token  bool  `json:"token"`
	Class  int   `json:"class"`
	Type   int   `json:"type"`
	Cost   int   `json:"cost"`
	Attack int   `json:"attack"`
	Life   int   `json:"life"`
}

// Route 是契约里的一条档案路由：data 是"该路由成功响应的空壳"，真值由本包按请求
// 算出来（见 Handle）。
type Route struct {
	Request  map[string]any `json:"request"`
	Data     map[string]any `json:"data"`
	Evidence map[string]any `json:"evidence,omitempty"`
}

// Contract 是 delta 那份 native-practice 契约。
type Contract struct {
	Schema           int                       `json:"schema"`
	SourceSHA256     string                    `json:"sourceSHA256"`
	CardMasterSHA256 string                    `json:"cardMasterSHA256"`
	Routes           map[string]Route          `json:"routes"`
	Templates        map[string]map[string]any `json:"templates"`
	// Enums 是"枚举名 → 成员名 → 值"。delta 的 Python 取的是值（例如
	// DeckError.LessThanCardNum == 3），所以这里也要留着值。
	Enums map[string]map[string]int `json:"enums"`
	Cards []Card                    `json:"cards"`
}

// alternateReference 只用到 printings 的键：那些是"异画/加速/结晶"的印刷体，
// 它们不算玩家收藏里的普通卡（照 delta 的判定）。
type alternateReference struct {
	CardMasterSHA256 string                     `json:"cardMasterSHA256"`
	Printings        map[string]json.RawMessage `json:"printings"`
}

// Profile 是这一层的内存状态。它现在只做"收藏"，牌组状态随后按同一套契约补上。
type Profile struct {
	contract *Contract
	// cards 是按 id 索引的整张卡表；owned 是"玩家持有"的那一部分。
	cards map[int64]Card
	owned map[int64]Card
	// ownedBase 是收藏里出现过的 base 卡 id，升序——就是 /Load/index 要的那份。
	ownedBase []int64
	// styles 管卡面样式，leaders 是"资源齐备"的主战者。
	styles  *CardStyles
	leaders map[int64]Leader
	store   *store
	// battleURL 是 /Practice/battleStart 交给客户端的对局通道地址（见 battle.go）。
	battleURL string
}

// Leader 是一个可用主战者。只收 nativeResourceReady 的那些——客户端要用的 prefab、
// 语音、立绘得有资源，否则选到就是黑屏或崩溃。
type Leader struct {
	ID        int64 `json:"id"`
	CardClass int   `json:"cardClass"`
	Ready     bool  `json:"nativeResourceReady"`
}

// Load 读内置契约并算出收藏。
// Load 读内置契约并算出收藏。
func Load() (*Profile, error) {
	return LoadWithState("")
}

// LoadWithState 与 Load 相同，但把可变状态落到 statePath（空则只放在内存里）。
func LoadWithState(statePath string) (*Profile, error) {
	raw, err := contractFiles.ReadFile("contracts/practice.json")
	if err != nil {
		return nil, fmt.Errorf("nativeprofile: 读契约失败: %w", err)
	}
	var contract Contract
	// 契约里的模板是 JSON 字面量，数字必须先归一化再进 MessagePack（见 contract.go）。
	if err := decodeInto(raw, &contract); err != nil {
		return nil, fmt.Errorf("nativeprofile: 解析契约失败: %w", err)
	}
	if contract.Schema != 1 {
		return nil, fmt.Errorf("nativeprofile: 契约 schema 是 %d，不认识", contract.Schema)
	}
	// 契约里的模板与路由基线是 JSON 字面量，解码后仍是 json.Number：它们是**要编给
	// 客户端**的值，必须归一化成整数（否则客户端收到的是字符串）。
	for _, template := range contract.Templates {
		normalizeNumbers(template)
	}
	for _, route := range contract.Routes {
		normalizeNumbers(route.Data)
	}
	alternateRaw, err := contractFiles.ReadFile("contracts/alternates.json")
	if err != nil {
		return nil, fmt.Errorf("nativeprofile: 读异画参考失败: %w", err)
	}
	var alternates alternateReference
	if err := json.Unmarshal(alternateRaw, &alternates); err != nil {
		return nil, fmt.Errorf("nativeprofile: 解析异画参考失败: %w", err)
	}
	// 两份数据必须描述同一份 CardMaster，否则"谁算收藏"这件事就无从谈起。
	if alternates.CardMasterSHA256 != contract.CardMasterSHA256 {
		return nil, fmt.Errorf("nativeprofile: 异画参考与契约不是同一份 CardMaster（%s != %s）",
			alternates.CardMasterSHA256, contract.CardMasterSHA256)
	}
	profile := &Profile{
		contract: &contract,
		cards:    make(map[int64]Card, len(contract.Cards)),
		owned:    map[int64]Card{},
	}
	for _, card := range contract.Cards {
		profile.cards[card.ID] = card
	}
	bases := map[int64]struct{}{}
	for _, card := range contract.Cards {
		if !profile.collectible(card, alternates) {
			continue
		}
		profile.owned[card.ID] = card
		bases[card.Base] = struct{}{}
	}
	for base := range bases {
		profile.ownedBase = append(profile.ownedBase, base)
	}
	sort.Slice(profile.ownedBase, func(i, j int) bool { return profile.ownedBase[i] < profile.ownedBase[j] })
	if len(profile.ownedBase) == 0 {
		return nil, fmt.Errorf("nativeprofile: 契约里没有算出任何收藏卡")
	}
	stylesRaw, err := contractFiles.ReadFile("contracts/card-styles.json")
	if err != nil {
		return nil, fmt.Errorf("nativeprofile: 读卡面样式表失败: %w", err)
	}
	styles, err := loadStyles(stylesRaw, contract.GameAssemblySHA256())
	if err != nil {
		return nil, err
	}
	profile.styles = styles
	leadersRaw, err := contractFiles.ReadFile("contracts/leaders.json")
	if err != nil {
		return nil, fmt.Errorf("nativeprofile: 读主战者表失败: %w", err)
	}
	var leaders struct {
		Leaders []Leader `json:"leaders"`
	}
	if err := decodeInto(leadersRaw, &leaders); err != nil {
		return nil, fmt.Errorf("nativeprofile: 解析主战者表失败: %w", err)
	}
	profile.leaders = map[int64]Leader{}
	for _, leader := range leaders.Leaders {
		if leader.Ready {
			profile.leaders[leader.ID] = leader
		}
	}
	if len(profile.leaders) == 0 {
		return nil, fmt.Errorf("nativeprofile: 主战者表里没有资源齐备的条目")
	}
	state, err := profile.loadState(statePath)
	if err != nil {
		return nil, err
	}
	profile.store = &store{path: statePath, state: state}
	return profile, nil
}

// GameAssemblySHA256 是契约对应的客户端 DLL 哈希；卡面样式表要对得上才用。
func (c *Contract) GameAssemblySHA256() string { return c.SourceSHA256 }

// loadState 读已有档案；没有就现造一份（seed）。
func (p *Profile) loadState(path string) (*State, error) {
	if path != "" {
		raw, err := os.ReadFile(path)
		if err == nil {
			var state State
			if err := decodeInto(raw, &state); err != nil {
				return nil, fmt.Errorf("nativeprofile: 解析档案失败: %w", err)
			}
			if state.Schema != 1 || state.SourceSHA256 != p.contract.SourceSHA256 {
				// 版本对不上就拒绝加载，**不要**覆盖用户已有的档案。
				return nil, fmt.Errorf("nativeprofile: 档案版本与契约不符（%s），已保留原文件", path)
			}
			normalizeState(&state)
			if state.LeaderSkinSettings == nil {
				settings, err := p.defaultLeaderSettings()
				if err != nil {
					return nil, err
				}
				state.LeaderSkinSettings = settings
			}
			return &state, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	state, err := p.seedState()
	if err != nil {
		return nil, err
	}
	// 第一次就把档案写下去：之后重启游戏，牌组还是这些。
	pending := &store{path: path, state: state}
	if err := pending.save(); err != nil {
		return nil, err
	}
	return state, nil
}

// collectible 判断一行 CardMaster 是不是"玩家收藏里的普通卡"。
//
// 判据照 delta：不是异画/加速/结晶的印刷体、不是 token、foil 在 0..2、type 在
// 1..4（随从/护符/倒计时护符/法术）、set < 80000（80000 起是特殊模式与活动卡）。
// CardMaster 里还有纹章、恩泽这些非收藏对象，这条筛子把它们排除掉。
func (p *Profile) collectible(card Card, alternates alternateReference) bool {
	if card.Token {
		return false
	}
	if _, isAlternate := alternates.Printings[fmt.Sprint(card.ID)]; isAlternate {
		return false
	}
	if card.Foil < 0 || card.Foil > 2 {
		return false
	}
	if card.Type < 1 || card.Type > 4 {
		return false
	}
	return card.Set < 80000
}

// OwnedBaseCardIDs 是收藏里出现过的 base 卡 id（升序）。/Load/index 会给客户端这份
// 列表——实测空列表会让它回到标题界面。
func (p *Profile) OwnedBaseCardIDs() []int64 {
	out := make([]int64, len(p.ownedBase))
	copy(out, p.ownedBase)
	return out
}

// Owned 返回收藏（只读副本），供上层按需查询。
func (p *Profile) Owned() map[int64]Card {
	out := make(map[int64]Card, len(p.owned))
	for id, card := range p.owned {
		out[id] = card
	}
	return out
}

// SourceSHA256 是这份契约提取自哪个 GameAssembly.dll。
func (p *Profile) SourceSHA256() string { return p.contract.SourceSHA256 }
