package runner

import "wbo/internal/ir"

type triggerIndex struct {
	byKind map[string]map[string][]runtimeAbility
}

// 索引只保存候选位置，实际顺序始终以当前场面为准。

func (x *triggerIndex) add(i *instance) {
	if i == nil || i.zone != "field" && i.zone != "hand" && i.zone != "crests" && i.zone != "deck" {
		return
	}
	if x.byKind == nil {
		x.byKind = map[string]map[string][]runtimeAbility{}
	}
	for ability := range i.triggeredAbilities() {
		zone := "field"
		if i.card.CardType == "crest" || i.card.CardType == "faith" {
			zone = "crests"
		}
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
		for _, i := range append(append(append([]*instance{}, p.crests...), p.field...), p.hand...) {
			g.triggerIndex.add(i)
		}
	}
}

func (g *game) queueEventTriggers(event ir.RuntimeEvent, subject *instance, binding string) bool {
	return g.queueEventTriggersFor(event.Kind, event, subject, binding, "")
}

// queueEventTriggersFor 允许用另一个触发种类去匹配同一个事件：
// "生命值减少"既是独立的减益事件，也由伤害事件（damaged）一并触发。
func (g *game) queueEventTriggersFor(kind string, event ir.RuntimeEvent, subject *instance, binding, area string) bool {
	for _, side := range g.orderedSides() {
		p := g.player(side.name)
		for _, source := range append(append(append(append([]*instance{}, p.crests...), side.field...), p.hand...), p.deck...) {
			if area == "crests" && source.zone != "crests" || area == "cards" && source.zone == "crests" {
				continue
			}
			for _, ability := range g.triggerIndex.abilities(kind, source) {
				if !g.chargeQueryVisits(1) {
					return false
				}
				trigger := ability.Trigger.(ir.EventTrigger)
				if trigger.SourceZone != "" && source.zone != trigger.SourceZone {
					// "在牌组中发动"这类能力只在声明的区域生效；索引可能在移动后残留。
					continue
				}
				if trigger.SelfOnly && subject != source {
					continue
				}
				if trigger.ExcludeSelf && subject == source {
					continue
				}
				if trigger.Event == "damaged" && (subject == nil || subject.life <= 0 || subject.zone != "field") {
					continue
				}
				if trigger.DuringTurn != "" && !triggerTurnMatches(trigger.DuringTurn, side.name, g.turn.Active) {
					continue
				}
				if trigger.SubjectType == "leader" && (event.Target == nil || event.Target.Kind != "leader" || event.Actual <= 0) {
					continue
				}
				if trigger.TargetKind == "leader" && (event.Defender == nil || event.Defender.Kind != "leader") {
					continue
				}
				if !eventSideMatches(trigger.Side, side.name, event.Side) || subject != nil && !g.matches(subject, trigger.Predicate, source, frame{}) || subject != nil && trigger.SubjectType != "" && subject.card.CardType != trigger.SubjectType {
					continue
				}
				if trigger.OncePerTurn != "" && (source.usedTriggers[ability.ID] || !triggerTurnMatches(trigger.OncePerTurn, side.name, g.turn.Active)) {
					continue
				}
				if trigger.Condition != nil && !g.condition(trigger.Condition, source) {
					continue
				}
				bindings := frame{}
				if event.Kind == "attacked" && event.Defender != nil && event.Defender.Kind == "instance" {
					// `defender` 指向被攻击的随从；攻击主战者时不绑定（对应条件不成立）。
					if defender := g.instances[event.Defender.InstanceID]; defender != nil {
						bindings["defender"] = bindEntities(defender)
					}
				}
				if trigger.Event == "damaged" && !trigger.SelfOnly {
					bindings["damaged"] = bindEntities(subject)
				} else if binding != "" && subject != nil {
					bindings[binding] = bindEntities(subject)
				} else if binding == "healed" && event.Target != nil && event.Target.Kind == "leader" {
					bindings[binding] = []ir.EventTarget{*event.Target}
				}
				if !g.queueTrigger(triggerInvocation{body: ability.Body, blockID: ability.blockID, self: source, bindings: bindings, zone: source.zone}) {
					return false
				}
				if trigger.OncePerTurn != "" {
					if source.usedTriggers == nil {
						source.usedTriggers = map[string]bool{}
					}
					source.usedTriggers[ability.ID] = true
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
