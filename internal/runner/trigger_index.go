package runner

import "wbo/internal/ir"

type triggerIndex struct {
	byKind map[string]map[string][]runtimeAbility
}

// 索引只保存候选位置，实际顺序始终以当前场面为准。

func (x *triggerIndex) add(i *instance) {
	if i == nil || i.zone != "field" && i.zone != "hand" {
		return
	}
	if x.byKind == nil {
		x.byKind = map[string]map[string][]runtimeAbility{}
	}
	for ability := range i.triggeredAbilities() {
		zone := "field"
		if event, ok := ability.Trigger.(ir.EventTrigger); ok && event.SourceZone != "" {
			zone = event.SourceZone
		}
		if i.zone != zone {
			continue
		}
		key := indexedTriggerKind(ability.Trigger)
		if key == "" {
			continue
		}
		if x.byKind[key] == nil {
			x.byKind[key] = map[string][]runtimeAbility{}
		}
		x.byKind[key][i.id] = append(x.byKind[key][i.id], ability)
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

func (g *game) detachEventSource(i *instance) {
	g.triggerIndex.remove(i)
	blocks := map[string]bool{}
	for ability := range i.triggeredAbilities() {
		if _, ok := ability.Trigger.(ir.EventTrigger); ok {
			blocks[ability.blockID] = true
		}
	}
	kept := g.triggers[:0]
	for _, trigger := range g.triggers {
		if trigger.self != i || !blocks[trigger.blockID] {
			kept = append(kept, trigger)
		}
	}
	g.triggers = kept
}

func (x *triggerIndex) abilities(kind string, source *instance) []runtimeAbility {
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
	g.triggerIndex = triggerIndex{byKind: map[string]map[string][]runtimeAbility{}}
	for _, p := range []*player{&g.own, &g.oppo} {
		for _, i := range append(append([]*instance{}, p.field...), p.hand...) {
			g.triggerIndex.add(i)
		}
	}
}

func (g *game) queueEventTriggers(event ir.RuntimeEvent, subject *instance, binding string) bool {
	for _, side := range g.orderedSides() {
		for _, source := range append(append([]*instance{}, side.field...), g.player(side.name).hand...) {
			for _, ability := range g.triggerIndex.abilities(event.Kind, source) {
				if !g.chargeQueryVisits(1) {
					return false
				}
				trigger := ability.Trigger.(ir.EventTrigger)
				if trigger.SelfOnly && subject != source {
					continue
				}
				if !eventSideMatches(trigger.Side, side.name, event.Side) || subject != nil && !g.matches(subject, trigger.Predicate) || subject != nil && trigger.SubjectType != "" && subject.card.CardType != trigger.SubjectType {
					continue
				}
				bindings := frame{}
				if binding != "" && subject != nil {
					bindings[binding] = bindEntities(subject)
				}
				if !g.queueTrigger(triggerInvocation{body: ability.Body, blockID: ability.blockID, self: source, bindings: bindings}) {
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
