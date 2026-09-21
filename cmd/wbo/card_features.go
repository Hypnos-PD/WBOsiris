package main

import (
	"encoding/json"
	"sort"
	"strings"

	"wbo/internal/ir"
)

// semanticEffectKinds 是**真正描述"这张牌做什么"**的效果种类白名单。
// 其余 kind（zone/self/leader/event/filter/if/compare/require/mode/choose…）是
// 选择器与结构节点，不表达语义，不进标签。
var semanticEffectKinds = map[string]string{
	"damage":              "damage",
	"destroy":             "destroy",
	"banish":              "banish",
	"heal":                "heal",
	"draw":                "draw",
	"summon":              "summon",
	"add_card":            "add_card",
	"buff_stats":          "buff_stats",
	"add_keyword":         "add_keyword",
	"gain_crest":          "gain_crest",
	"adjust_resource":     "adjust_resource",
	"pay_resource":        "pay_resource",
	"adjust_entity_field": "adjust_entity_field",
	"random_choose":       "random_choose",
	"spellboost":          "spellboost",
	"enhance":             "enhance",
	"repeat":              "repeat",
	"transform":           "transform",
	"return_to_hand":      "return_to_hand",
	"grant_ability":       "grant_ability",
}

// triggerTags 把触发时机映射成标签。`triggerKind` 未导出，所以这里用 JSON 里的 kind 字符串。
var triggerTags = map[string]string{
	"fanfare":     "fanfare",
	"evolve":      "evolve_effect",
	"superevolve": "superevolve_effect",
	"lastwords":   "lastwords",
	"attack":      "attack_trigger",
	"clash":       "clash_trigger",
	"spellboost":  "spellboost_trigger",
	"engage":      "engage_trigger",
	"enhance":     "enhance_trigger",
	"turn_ended":  "turn_end_trigger",
}

// cardFeatureTags 抽取一张卡的语义标签（稳定、固定词表、可枚举）。
//
// 训练侧把它当结构特征用：`damage`/`destroy`/`summon`/`add_keyword`/`spellboost` 这类标签
// 才是"这张牌具体能干什么"，比只有费用/身材/类型强得多，而且新卡上线时引擎会自动给出。
func cardFeatureTags(card *ir.Card) []string {
	if card == nil {
		return nil
	}
	seen := map[string]bool{}
	add := func(tag string) {
		if tag != "" {
			seen[tag] = true
		}
	}
	if card.Crystallize != nil {
		add("crystallize")
	}
	if len(card.FusionAbilities) > 0 {
		add("fusion")
	}
	for _, trait := range card.Traits {
		if trait != "" {
			add("trait:" + strings.ToLower(trait))
		}
	}
	blob, err := json.Marshal(struct {
		Abilities       []ir.Ability       `json:"abilities"`
		Fusions         []ir.FusionAbility `json:"fusions"`
		PlayEffects     []ir.Effect        `json:"playEffects"`
		IntrinsicState  []ir.IntrinsicState `json:"intrinsicState"`
		Restrictions    []ir.Restriction   `json:"restrictions"`
	}{card.Abilities, card.FusionAbilities, card.PlayEffects, card.IntrinsicState, card.Restrictions})
	if err != nil {
		return nil
	}
	var generic any
	if err := json.Unmarshal(blob, &generic); err != nil {
		return nil
	}
	var walk func(value any)
	walk = func(value any) {
		switch node := value.(type) {
		case map[string]any:
			if kind, ok := node["kind"].(string); ok {
				if tag, exists := semanticEffectKinds[kind]; exists {
					add(tag)
				}
				if tag, exists := triggerTags[kind]; exists {
					add(tag)
				}
			}
			for _, child := range node {
				walk(child)
			}
		case []any:
			for _, child := range node {
				walk(child)
			}
		}
	}
	walk(generic)
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}
// cardStats 返回基础攻击/生命；法术与护符没有身材时返回 0/0。
func cardStats(card *ir.Card) [2]int {
	if card == nil || card.Stats == nil {
		return [2]int{0, 0}
	}
	return [2]int{card.Stats.Attack, card.Stats.Life}
}

// cardPool 是 "card_pool" 命令的清单：每张卡的赛制归属 + 结构化卡面特征。
func cardPool(cards *ir.CardPack) []envCardInfo {
	pool := make([]envCardInfo, 0, len(cards.Cards))
	for n := range cards.Cards {
		card := &cards.Cards[n]
		stats := cardStats(card)
		pool = append(pool, envCardInfo{
			ID: card.ID, Pack: card.Meta.Pack, Class: card.Meta.Class,
			Type: card.CardType, Name: card.Locales["chs"].Name,
			Keywords: intrinsicKeywords(card),
			Rarity:   card.Meta.Rarity,
			Cost:     card.Cost,
			Attack:   stats[0],
			Life:     stats[1],
			Traits:   card.Traits,
			Tags:     cardFeatureTags(card),
		})
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].ID < pool[j].ID })
	return pool
}
