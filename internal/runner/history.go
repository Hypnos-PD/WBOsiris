package runner

import (
	"sort"
	"wbo/internal/ir"
)

// DestructionRecord is a value snapshot, independent of the surviving instance.
type DestructionRecord struct {
	InstanceID    string   `json:"instanceId"`
	CardID        int      `json:"cardId"`
	Cost          int      `json:"cost"`
	Attack        int      `json:"attack"`
	Life          int      `json:"life"`
	Evolved       bool     `json:"evolved"`
	SuperEvolved  bool     `json:"superEvolved"`
	Departed      bool     `json:"departed,omitempty"`
	Keywords      []string `json:"keywords,omitempty"`
	EventSequence uint64   `json:"eventSequence"`
	TurnSide      string   `json:"turnSide,omitempty"`
	TurnNumber    int      `json:"turnNumber"`
}

func destructionRecord(i *instance, sequence uint64, turn ir.Turn) DestructionRecord {
	var keywords []string
	for keyword, enabled := range i.abilities {
		if enabled {
			keywords = append(keywords, keyword)
		}
	}
	sort.Strings(keywords)
	return DestructionRecord{InstanceID: i.id, CardID: i.card.ID, Cost: i.cost, Attack: i.attack, Life: i.life,
		Evolved: i.evolved, SuperEvolved: i.superEvolved, Departed: i.departed, Keywords: keywords, EventSequence: sequence, TurnSide: turn.Active, TurnNumber: turn.Number}
}

func cloneDestructionHistory(records []DestructionRecord) []DestructionRecord {
	cloned := append([]DestructionRecord{}, records...)
	for n := range cloned {
		cloned[n].Keywords = append([]string(nil), records[n].Keywords...)
	}
	return cloned
}

func historyInstances(history []DestructionRecord, cards map[int]*ir.Card) []*instance {
	items := make([]*instance, 0, len(history))
	for _, record := range history {
		i := &instance{id: record.InstanceID, zone: "destroyed"}
		resetCardState(i, cards[record.CardID])
		i.cost, i.attack, i.life = record.Cost, record.Attack, record.Life
		i.evolved, i.superEvolved, i.departed = record.Evolved, record.SuperEvolved, record.Departed
		i.abilities = map[string]bool{}
		for _, keyword := range record.Keywords {
			i.abilities[keyword] = true
		}
		items = append(items, i)
	}
	return items
}

func (g *game) effectTargets(ref ir.Ref, self *instance, bindings frame) []*instance {
	items := g.fromRef(ref, self, bindings)
	targets := make([]*instance, 0, len(items))
	for _, i := range items {
		if i != nil && i.zone != "destroyed" && (i.card == nil || i.card.CardType != "crest") {
			targets = append(targets, i)
		}
	}
	return targets
}
