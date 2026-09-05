package runner

import "wbo/internal/ir"

type triggerIndex struct {
	byKind map[string]map[string][]int
}

// 索引只保存候选位置，实际顺序始终以当前场面为准。

func (x *triggerIndex) add(i *instance) {
	if i == nil || i.zone != "field" {
		return
	}
	if x.byKind == nil {
		x.byKind = map[string]map[string][]int{}
	}
	for abilityIndex, ability := range i.card.Abilities {
		key := indexedTriggerKind(ability.Trigger)
		if key == "" {
			continue
		}
		if x.byKind[key] == nil {
			x.byKind[key] = map[string][]int{}
		}
		x.byKind[key][i.id] = append(x.byKind[key][i.id], abilityIndex)
	}
}

func (x *triggerIndex) remove(i *instance) {
	if i == nil {
		return
	}
	for key, sources := range x.byKind {
		delete(sources, i.id)
		if len(sources) == 0 {
			delete(x.byKind, key)
		}
	}
}

func (x *triggerIndex) abilities(kind string, source *instance) []int {
	if x == nil || source == nil {
		return nil
	}
	return x.byKind[kind][source.id]
}

func indexedTriggerKind(trigger ir.Trigger) string {
	switch t := trigger.(type) {
	case ir.EventTrigger:
		return t.Event
	case ir.SimpleTrigger:
		if t.Kind == "lastwords" {
			return t.Kind
		}
	}
	return ""
}

func (g *game) rebuildTriggerIndex() {
	g.triggerIndex = triggerIndex{byKind: map[string]map[string][]int{}}
	for _, p := range []*player{&g.own, &g.oppo} {
		for _, i := range p.field {
			g.triggerIndex.add(i)
		}
	}
}

func (g *game) queueEventTriggers(event ir.RuntimeEvent, subject *instance, binding string) bool {
	for _, side := range g.orderedSides() {
		for _, source := range side.field {
			for _, abilityIndex := range g.triggerIndex.abilities(event.Kind, source) {
				if !g.chargeQueryVisits(1) {
					return false
				}
				ability := source.card.Abilities[abilityIndex]
				trigger := ability.Trigger.(ir.EventTrigger)
				if !eventSideMatches(trigger.Side, side.name, event.Side) || subject != nil && !g.matches(subject, trigger.Predicate) || subject != nil && trigger.SubjectType != "" && subject.card.CardType != trigger.SubjectType {
					continue
				}
				bindings := frame{}
				if binding != "" && subject != nil {
					bindings[binding] = []*instance{subject}
				}
				if !g.queueTrigger(triggerInvocation{body: ability.Body, blockID: abilityBlockID(source.card.ID, ability.ID), self: source, bindings: bindings}) {
					return false
				}
			}
		}
	}
	return true
}

func (g *game) orderedSides() []struct {
	name  string
	field []*instance
} {
	if g.turn.Active == "oppo" {
		return []struct {
			name  string
			field []*instance
		}{{"oppo", g.oppo.field}, {"own", g.own.field}}
	}
	return []struct {
		name  string
		field []*instance
	}{{"own", g.own.field}, {"oppo", g.oppo.field}}
}

func eventSideMatches(want, sourceSide, eventSide string) bool {
	if want == "" || eventSide == "" {
		return true
	}
	if want == "own" {
		return sourceSide == eventSide
	}
	return sourceSide != eventSide
}
