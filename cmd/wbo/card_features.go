package main

import (
	"encoding/json"
	"sort"
	"strings"

	"wbo/internal/engine/ir"
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
		Abilities      []ir.Ability        `json:"abilities"`
		Fusions        []ir.FusionAbility  `json:"fusions"`
		PlayEffects    []ir.Effect         `json:"playEffects"`
		IntrinsicState []ir.IntrinsicState `json:"intrinsicState"`
		Restrictions   []ir.Restriction    `json:"restrictions"`
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
			Effects:  cardEffectTokens(card, maxEffectTokens),
		})
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].ID < pool[j].ID })
	return pool
}

// envEffectToken 是**效果程序**的一个步骤。
//
// 设计目标是"无损"：不手挑字段，而是把节点上的**所有数值字段与字符串字段**（按字段名排序）
// 原样带上，结构用 `Parent`（父 token 下标）表达，模式分支用 `Option` 表达。
// 训练侧再把 kind/字段名哈希进固定桶 —— 新机制、新字段都能表达，不需要改 schema。
//
// `target` 是唯一被特殊处理的部分（它是选择器而不是效果本身）：抽成 Target* 字段，
// 不再往里递归，否则会产出 leader/zone 这类噪声 token。
type envEffectToken struct {
	Kind    string    `json:"kind"`
	Trigger string    `json:"trigger,omitempty"`
	Parent  int       `json:"parent"`
	Depth   int       `json:"depth"`
	Option  int       `json:"option,omitempty"`
	NumKeys []string  `json:"numKeys,omitempty"`
	Nums    []float64 `json:"nums,omitempty"`
	StrKeys []string  `json:"strKeys,omitempty"`
	Strs    []string  `json:"strs,omitempty"`

	TargetKind   string `json:"targetKind,omitempty"`
	TargetSide   string `json:"targetSide,omitempty"`
	TargetZone   string `json:"targetZone,omitempty"`
	TargetMember string `json:"targetMember,omitempty"`
	All          bool   `json:"all,omitempty"`
}

// : 每张卡最多发多少条效果 token（超出截断，训练侧靠截断标记识别）。
const maxEffectTokens = 24

// cardEffectTokens 把一张卡的效果树展平成 token 序列。
//
// 顺序 = 求值顺序（abilities → fusion → playEffects，内部按 body 顺序深度优先），
// 分支（mode.option N）用 `Option` 标注，深度用 `Depth` 标注，触发时机用 `Trigger` 标注。
func cardEffectTokens(card *ir.Card, limit int) []envEffectToken {
	if card == nil {
		return nil
	}
	if limit <= 0 {
		limit = maxEffectTokens
	}
	tokens := make([]envEffectToken, 0, limit)
	add := func(token envEffectToken) {
		if len(tokens) >= limit {
			return
		}
		tokens = append(tokens, token)
	}
	var walk func(value any, trigger string, parent, depth, option int)
	walk = func(value any, trigger string, parent, depth, option int) {
		switch node := value.(type) {
		case map[string]any:
			if len(tokens) >= limit {
				return
			}
			index := parent
			if kind, _ := node["kind"].(string); kind != "" {
				token := makeEffectToken(kind, trigger, node, parent, depth, option)
				add(token)
				index = len(tokens) - 1
			}
			// "mode" 的 options 是二选一分支：给每个分支编号，模型才分得清"不是都能做"。
			if options, ok := node["options"].([]any); ok {
				for position, child := range options {
					walk(child, trigger, index, depth+1, position+1)
				}
				return
			}
			for key, child := range node {
				if key == "origin" || key == "id" || key == "kind" || key == "target" {
					continue // 位置信息 / 已有专门字段 / 选择器噪声
				}
				walk(child, trigger, index, depth+1, option)
			}
		case []any:
			for _, child := range node {
				walk(child, trigger, parent, depth+1, option)
			}
		}
	}
	// IR 的效果节点是接口/结构体，先转成通用 JSON 再按顺序遍历（顺序 = 求值顺序）。
	walkBody := func(trigger string, body any) {
		if body == nil {
			return
		}
		blob, err := json.Marshal(body)
		if err != nil {
			return
		}
		var generic any
		if err := json.Unmarshal(blob, &generic); err != nil {
			return
		}
		if list, ok := generic.([]any); ok && len(list) == 0 {
			return // 这一类没有效果（例如法术没有 abilities）
		}
		add(envEffectToken{Kind: "trigger", Trigger: trigger, Parent: -1})
		walk(generic, trigger, len(tokens)-1, 1, 0)
	}
	for index := range card.Abilities {
		ability := &card.Abilities[index]
		trigger := triggerKindOf(ability)
		if trigger == "" {
			trigger = "ability"
		}
		walkBody(trigger, ability.Body)
	}
	for index := range card.FusionAbilities {
		walkBody("fusion", card.FusionAbilities[index].Body)
	}
	walkBody("play", card.PlayEffects)
	return tokens
}

// triggerKindOf 取触发时机的 kind：`triggerKind()` 未导出，用 JSON 里的 kind 字段。
func triggerKindOf(ability *ir.Ability) string {
	if ability == nil {
		return ""
	}
	blob, err := json.Marshal(ability.Trigger)
	if err != nil {
		return ""
	}
	var generic map[string]any
	if err := json.Unmarshal(blob, &generic); err != nil {
		return ""
	}
	kind, _ := generic["kind"].(string)
	return kind
}

// makeEffectToken 把一个效果节点**原样**转成 token：
// 所有数值字段、所有字符串字段都收进来（按键名排序，保证可复现）。
func makeEffectToken(kind, trigger string, node map[string]any, parent, depth, option int) envEffectToken {
	token := envEffectToken{Kind: kind, Trigger: trigger, Parent: parent, Depth: depth, Option: option}
	numericKeys := make([]string, 0, 4)
	stringKeys := make([]string, 0, 4)
	for key, value := range node {
		switch key {
		case "id", "origin", "kind", "target", "options":
			continue // id/origin 是位置信息；kind/target 已有专门字段；options 由 walk 处理
		}
		switch typed := value.(type) {
		case float64, int:
			numericKeys = append(numericKeys, key)
		case string:
			if typed != "" {
				stringKeys = append(stringKeys, key)
			}
		case bool:
			if typed {
				stringKeys = append(stringKeys, key+"=true")
			}
		}
	}
	sort.Strings(numericKeys)
	sort.Strings(stringKeys)
	for _, key := range numericKeys {
		token.NumKeys = append(token.NumKeys, key)
		token.Nums = append(token.Nums, numberField(node, key))
	}
	for _, key := range stringKeys {
		if strings.HasSuffix(key, "=true") {
			token.StrKeys = append(token.StrKeys, strings.TrimSuffix(key, "=true"))
			token.Strs = append(token.Strs, "true")
			continue
		}
		token.StrKeys = append(token.StrKeys, key)
		text, _ := node[key].(string)
		token.Strs = append(token.Strs, text)
	}
	if target, ok := node["target"].(map[string]any); ok {
		token.TargetKind, _ = target["kind"].(string)
		token.TargetSide, _ = target["side"].(string)
		token.TargetZone, _ = target["zone"].(string)
		token.TargetMember, _ = target["member"].(string)
		token.All, _ = target["all"].(bool)
	}
	if !token.All {
		token.All, _ = node["all"].(bool)
	}
	return token
}

func numberField(node map[string]any, key string) float64 {
	switch value := node[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	default:
		return 0
	}
}
