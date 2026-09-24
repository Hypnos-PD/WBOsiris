package nativeprofile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// State 是档案层的可变部分：牌组、样式偏好、战斗设置……
//
// 结构与非字段名照 delta 的 tools/local_practice_profile.py 的 seed()：它的存档格式
// 已经和原客户端来回验过，改字段名只会让两边对不上。
type State struct {
	Schema               int              `json:"schema"`
	SourceSHA256         string           `json:"sourceSHA256"`
	Revision             int64            `json:"revision"`
	NextID               int64            `json:"next_id"`
	Decks                []map[string]any `json:"decks"`
	LastEdit             map[string]int64 `json:"last_edit"`
	Styles               map[string]int64 `json:"styles"`
	BattleSettings       []map[string]any `json:"battle_settings"`
	DeckConfig           map[string]any   `json:"deck_config"`
	LastEditBattleFormat int64            `json:"last_edit_battle_format"`
	LeaderSkinSettings   []map[string]any `json:"leader_skin_settings"`
}

// store 管内存状态与落盘。档案层的每个请求都可能改状态，所以整层串行化——
// 客户端确实是并发发请求的，而"读-改-写"不是原子的。
type store struct {
	guard sync.Mutex
	path  string
	state *State
}

// Current 在锁里跑一段逻辑，结束后按需落盘。
func (s *store) Current(run func(state *State) error) error {
	s.guard.Lock()
	defer s.guard.Unlock()
	return run(s.state)
}

// save 原子落盘：先写 .pending 再改名，中断不会留下半个存档。
func (s *store) save() error {
	if s.path == "" {
		return nil
	}
	encoded, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	pending := s.path + ".pending"
	if err := os.WriteFile(pending, append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(pending, s.path)
}

// seedState 造一份全新的档案：每个职业一副基础牌组。
//
// 这些牌组是**本地样例**，不是官方预组；它们只由"已核实可收藏的基础卡"（set 10000、
// foil 0、同职业或中立）组成，每张 base 最多 3 张，正好 40 张。
func (p *Profile) seedState() (*State, error) {
	decks := make([]map[string]any, 0, 7)
	for class := int64(1); class <= 7; class++ {
		var pool []Card
		for _, card := range p.owned {
			if card.Foil != 0 || card.Set != 10000 {
				continue
			}
			if card.Class != 0 && int64(card.Class) != class {
				continue
			}
			pool = append(pool, card)
		}
		// 与 delta 相同的排序：本职业优先、费用升序、id 升序。
		sort.Slice(pool, func(i, j int) bool {
			left, right := pool[i], pool[j]
			leftOwn := left.Class == int(class)
			rightOwn := right.Class == int(class)
			if leftOwn != rightOwn {
				return leftOwn
			}
			if left.Cost != right.Cost {
				return left.Cost < right.Cost
			}
			return left.ID < right.ID
		})
		deck, err := p.copyTemplate("DeckData")
		if err != nil {
			return nil, err
		}
		deck["deck_id"] = class
		deck["deck_format"] = int64(1)
		deck["class_id"] = class
		deck["name"] = "离线基础·" + classLabel(class)
		deck["sleeve_id"] = int64(100011205)
		deck["leader_skin_id"] = int64(1000) + class
		deck["leader_skin_id_list"] = []any{int64(1000) + class}
		deck["error_state"] = p.enumValue("DeckError", "LessThanCardNum")
		cards := make([]any, 0, 40)
		used := map[int64]bool{}
		remaining := int64(40)
		for _, card := range pool {
			if used[card.Base] {
				continue
			}
			used[card.Base] = true
			count := int64(3)
			if remaining < 3 {
				count = remaining
			}
			entry, err := p.copyTemplate("DeckInCard")
			if err != nil {
				return nil, err
			}
			entry["id"] = card.ID
			entry["num"] = count
			cards = append(cards, entry)
			remaining -= count
			if remaining == 0 {
				break
			}
		}
		if remaining != 0 {
			return nil, fmt.Errorf("nativeprofile: 职业 %d 的基础卡不够组一副牌（还差 %d 张）", class, remaining)
		}
		deck["cards"] = cards
		deck["error_state"] = int64(0)
		decks = append(decks, deck)
	}
	deckConfig, err := p.copyTemplate("DeckConfig")
	if err != nil {
		return nil, err
	}
	state := &State{
		Schema:               1,
		SourceSHA256:         p.contract.SourceSHA256,
		Revision:             0,
		NextID:               8,
		Decks:                decks,
		LastEdit:             map[string]int64{},
		Styles:               map[string]int64{},
		BattleSettings:       []map[string]any{},
		DeckConfig:           deckConfig,
		LastEditBattleFormat: 1,
	}
	state.LeaderSkinSettings, err = p.defaultLeaderSettings()
	if err != nil {
		return nil, err
	}
	return state, nil
}

// defaultLeaderSettings 每个职业一个默认主战者（1000+职业）。
func (p *Profile) defaultLeaderSettings() ([]map[string]any, error) {
	settings := make([]map[string]any, 0, 7)
	for class := int64(1); class <= 7; class++ {
		setting, err := p.copyTemplate("LeaderSkinSetting")
		if err != nil {
			return nil, err
		}
		setting["class_id"] = class
		setting["leader_skin_id"] = 1000 + class
		setting["leader_skin_id_list"] = []any{1000 + class}
		settings = append(settings, setting)
	}
	return settings, nil
}

func classLabel(class int64) string {
	labels := map[int64]string{1: "精灵", 2: "皇家", 3: "巫师", 4: "龙族", 5: "梦魇", 6: "主教", 7: "超越者"}
	if label, ok := labels[class]; ok {
		return label
	}
	return fmt.Sprintf("职业%d", class)
}

// normalizeState 把从存档读出来的 JSON 数字归一化成 int64。
//
// State 里那些 map[string]any 的字段（牌组、战斗设置、牌组配置、主战者设置）解码后
// 全是 json.Number；它们最终要编给客户端，留着就会变成字符串。
func normalizeState(state *State) {
	for _, deck := range state.Decks {
		normalizeNumbers(deck)
	}
	for _, setting := range state.BattleSettings {
		normalizeNumbers(setting)
	}
	for _, setting := range state.LeaderSkinSettings {
		normalizeNumbers(setting)
	}
	normalizeNumbers(state.DeckConfig)
}

// enumValue 取契约里某个枚举成员的值。
func (p *Profile) enumValue(enum, member string) int64 {
	for name, value := range p.contract.Enums[enum] {
		if name == member {
			return int64(value)
		}
	}
	return 0
}
