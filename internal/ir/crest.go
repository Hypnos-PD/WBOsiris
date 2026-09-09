package ir

import (
	"encoding/json"
	"fmt"
)

type CrestDefinition struct {
	Counters  map[string]int    `json:"counters,omitempty"`
	Countdown int               `json:"countdown,omitempty"`
	Abilities []Ability         `json:"abilities"`
	Locales   map[string]Locale `json:"locales"`
	Origin    Origin            `json:"origin"`
}

// The owner card ID identifies the crest definition, not its live instance.
func (c Card) CrestCard() *Card {
	if c.Crest == nil {
		return nil
	}
	d := c.Crest
	crest := &Card{ID: c.ID, CardType: "crest", Counters: d.Counters, Abilities: d.Abilities, Locales: d.Locales, Meta: c.Meta, Origin: d.Origin}
	if d.Countdown > 0 {
		crest.IntrinsicState = []IntrinsicState{{Kind: "countdown", Initial: d.Countdown}}
	}
	return crest
}

func decodeCrest(data []byte, abilityIDs, nodeIDs map[string]bool) (*CrestDefinition, error) {
	var raw struct {
		Counters  map[string]int    `json:"counters"`
		Countdown int               `json:"countdown"`
		Abilities []json.RawMessage `json:"abilities"`
		Locales   map[string]Locale `json:"locales"`
		Origin    Origin            `json:"origin"`
	}
	if err := strict(data, &raw); err != nil {
		return nil, err
	}
	if !validOrigin(raw.Origin) || !ValidCounters(raw.Counters) || raw.Countdown < 0 || raw.Countdown > 65535 || len(raw.Abilities) == 0 || len(raw.Locales) != 5 {
		return nil, fmt.Errorf("invalid crest definition")
	}
	c := &CrestDefinition{Counters: raw.Counters, Countdown: raw.Countdown, Locales: raw.Locales, Origin: raw.Origin}
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
