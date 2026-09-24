package ir

import (
	"encoding/json"
	"fmt"
)

type CrestDefinition struct {
	Counters  map[string]int    `json:"counters,omitempty"`
	Countdown int               `json:"countdown,omitempty"`
	Abilities []Ability         `json:"abilities"`
	// Passives 是"持续性规则改动"（例如"自己的随从的【入场曲】不发动"），
	// 不是事件监听，因此单独列出。
	Passives []string `json:"passives,omitempty"`
	Locales   map[string]Locale `json:"locales"`
	Origin    Origin            `json:"origin"`
}

// The owner card ID identifies the crest definition, not its live instance.
func (c Card) CrestCard() *Card {
	if c.Crest == nil {
		return nil
	}
	d := c.Crest
	crest := &Card{ID: c.ID, CardType: "crest", Counters: d.Counters, Abilities: d.Abilities, Passives: d.Passives, Locales: d.Locales, Meta: c.Meta, Origin: d.Origin}
	if d.Countdown > 0 {
		crest.IntrinsicState = []IntrinsicState{{Kind: "countdown", Initial: d.Countdown}}
	}
	return crest
}

// FaithCard 派生信仰实例使用的卡牌定义（关卡类型为 faith，与纹章共用能力与本地化结构）。
func (c Card) FaithCard() *Card {
	if c.Faith == nil {
		return nil
	}
	d := c.Faith
	return &Card{ID: c.ID, CardType: "faith", Counters: d.Counters, Abilities: d.Abilities, Passives: d.Passives, Locales: d.Locales, Meta: c.Meta, Origin: d.Origin}
}

// CrystallizeDefinition 是【结晶】形态：以 Cost 点费用当作护符打出时使用的卡面。
type CrystallizeDefinition struct {
	Cost           int              `json:"cost"`
	Counters       map[string]int   `json:"counters,omitempty"`
	Intrinsic      []string         `json:"intrinsic,omitempty"`
	IntrinsicState []IntrinsicState `json:"intrinsicState,omitempty"`
	// Abilities 是谢幕曲/事件监听等触发能力；PlayEffects 是吟唱等打出时结算的效果。
	Abilities   []Ability `json:"abilities"`
	PlayEffects []Effect  `json:"playEffects"`
	Locales     map[string]Locale `json:"locales"`
	Origin      Origin            `json:"origin"`
}

// CrystallizeCard 派生【结晶】打出后进入战场的护符卡面。
func (c Card) CrystallizeCard() *Card {
	if c.Crystallize == nil {
		return nil
	}
	d := c.Crystallize
	return &Card{ID: c.ID, CardType: "amulet", Cost: d.Cost, Counters: d.Counters,
		Intrinsic: d.Intrinsic, IntrinsicState: d.IntrinsicState, Abilities: d.Abilities,
		PlayEffects: d.PlayEffects, Locales: d.Locales, Meta: c.Meta, Origin: d.Origin}
}

// decodeCrystallize 解码【结晶】形态（效果、能力与本地化）。
func decodeCrystallize(data []byte, abilityIDs, nodeIDs map[string]bool) (*CrystallizeDefinition, error) {
	var raw struct {
		Cost           int               `json:"cost"`
		Counters       map[string]int    `json:"counters,omitempty"`
		Intrinsic      []string          `json:"intrinsic,omitempty"`
		IntrinsicState []json.RawMessage `json:"intrinsicState,omitempty"`
		Abilities      []json.RawMessage `json:"abilities"`
		PlayEffects    []json.RawMessage `json:"playEffects"`
		Locales        map[string]Locale `json:"locales"`
		Origin         Origin            `json:"origin"`
	}
	if err := strict(data, &raw); err != nil {
		return nil, err
	}
	if !validOrigin(raw.Origin) || !ValidCounters(raw.Counters) || raw.Cost < 0 || raw.Cost > 65535 || len(raw.Abilities) == 0 {
		return nil, fmt.Errorf("invalid crystallize definition")
	}
	d := &CrystallizeDefinition{Cost: raw.Cost, Counters: raw.Counters, Intrinsic: raw.Intrinsic, Locales: raw.Locales, Origin: raw.Origin}
	for _, x := range raw.IntrinsicState {
		type state struct {
			Kind    string `json:"kind"`
			Initial int    `json:"initial"`
		}
		var s state
		if err := strict(x, &s); err != nil || s.Kind != "countdown" || s.Initial < 0 || s.Initial > 65535 {
			return nil, fmt.Errorf("invalid crystallize state")
		}
		d.IntrinsicState = append(d.IntrinsicState, IntrinsicState{s.Kind, s.Initial})
	}
	var err error
	if d.PlayEffects, err = decodeEffects(raw.PlayEffects, nodeIDs); err != nil {
		return nil, err
	}
	for _, data := range raw.Abilities {
		a, err := decodeAbility(data, nodeIDs)
		if err != nil {
			return nil, err
		}
		if abilityIDs[a.ID] {
			return nil, fmt.Errorf("duplicate ability ID %s", a.ID)
		}
		abilityIDs[a.ID] = true
		d.Abilities = append(d.Abilities, a)
	}
	return d, nil
}

func decodeCrest(data []byte, abilityIDs, nodeIDs map[string]bool) (*CrestDefinition, error) {
	var raw struct {
		Counters  map[string]int    `json:"counters"`
		Countdown int               `json:"countdown"`
		Abilities []json.RawMessage `json:"abilities"`
		Passives  []string          `json:"passives"`
		Locales   map[string]Locale `json:"locales"`
		Origin    Origin            `json:"origin"`
	}
	if err := strict(data, &raw); err != nil {
		return nil, err
	}
	if !validOrigin(raw.Origin) || !ValidCounters(raw.Counters) || raw.Countdown < 0 || raw.Countdown > 65535 || len(raw.Abilities) == 0 && len(raw.Passives) == 0 || len(raw.Locales) != 5 {
		return nil, fmt.Errorf("invalid crest definition")
	}
	c := &CrestDefinition{Counters: raw.Counters, Countdown: raw.Countdown, Locales: raw.Locales, Origin: raw.Origin}
	seenPassives := map[string]bool{}
	for _, name := range raw.Passives {
		if !ValidPassive(name) || seenPassives[name] {
			return nil, fmt.Errorf("invalid crest passive")
		}
		seenPassives[name] = true
		c.Passives = append(c.Passives, name)
	}
	for _, code := range []string{"chs", "eng", "jpn", "kor", "cht"} {
		if c.Locales[code].Name == "" || c.Locales[code].Text == "" {
			return nil, fmt.Errorf("missing crest locale %s", code)
		}
	}
	for _, data := range raw.Abilities {
		a, err := decodeAbility(data, nodeIDs)
		if err != nil {
			return nil, err
		}
		if abilityIDs[a.ID] || a.Relation != "independent" {
			return nil, fmt.Errorf("invalid crest ability")
		}
		abilityIDs[a.ID] = true
		switch trigger := a.Trigger.(type) {
		case SimpleTrigger:
			if trigger.Kind != "lastwords" {
				return nil, fmt.Errorf("unsupported crest trigger")
			}
		case EventTrigger:
			if trigger.SelfOnly || trigger.SourceZone != "" {
				return nil, fmt.Errorf("crest listeners always use the leader area")
			}
		default:
			return nil, fmt.Errorf("unsupported crest trigger")
		}
		c.Abilities = append(c.Abilities, a)
	}
	return c, nil
}

func ValidCrestOverrides(o InstanceOverrides, countdown int) bool {
	return o.Cost == nil && o.Stats == nil && o.DamageTaken == nil && o.Earthsigil == nil && o.DamageReduction == nil && o.Engaged == nil && o.Evolved == nil && o.SuperEvolved == nil && len(o.Keywords) == 0 &&
		(o.Countdown == nil || *o.Countdown > 0 && *o.Countdown <= countdown)
}
