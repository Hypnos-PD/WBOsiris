package runner

import "wbo/internal/engine/ir"

func candidateForValue(value ir.EventTarget) ChoiceCandidate {
	if value.Kind == "leader" {
		return ChoiceCandidate{Kind: "leader", LeaderSide: value.Side}
	}
	return ChoiceCandidate{Kind: "entity", InstanceID: value.InstanceID}
}

func bindEntities(instances ...*instance) []ir.EventTarget {
	var values []ir.EventTarget
	for _, i := range instances {
		if i != nil {
			values = append(values, ir.EventTarget{Kind: "instance", InstanceID: i.id})
		}
	}
	return values
}

// bindSummoned 记录召唤输出：`summoned` 仍只保留最近一次操作的结果，
// `summoned_all` 则累计本次结算里由各次召唤成功入场的全部实例
// （用于"爆能强化使其获得【疾驰】"这类同时指向多个衍生体的文本）。
func bindSummoned(f frame, output string, batch []*instance) {
	entities := bindEntities(batch...)
	if output != "" {
		f[output] = entities
	}
	if len(entities) > 0 {
		f["summoned_all"] = append(f["summoned_all"], entities...)
	}
}

func (g *game) boundInstances(values []ir.EventTarget) []*instance {
	var instances []*instance
	for _, value := range values {
		if value.Kind == "instance" && g.instances[value.InstanceID] != nil && g.instances[value.InstanceID].zone != "retired_deck" {
			instances = append(instances, g.instances[value.InstanceID])
		}
	}
	return instances
}

func (g *game) selectionValues(e ir.SelectionEffect, self *instance, f frame) []ir.EventTarget {
	characters, mixed := e.Source.(ir.CharacterSetRef)
	if !mixed {
		return bindEntities(g.selectionCandidates(e, self, f)...)
	}
	e.Source = ir.ZoneRef{Kind: "zone", Side: characters.Side, Zone: "field", Member: "follower"}
	values := bindEntities(g.selectionCandidates(e, self, f)...)
	if characters.ExcludeSelf && self != nil {
		kept := values[:0]
		for _, value := range values {
			if value.Kind == "instance" && value.InstanceID == self.id {
				continue
			}
			kept = append(kept, value)
		}
		values = kept
	}
	sides := []string{"own", "oppo"}
	if characters.Side != "" {
		_, side := g.playerForSide(self, characters.Side)
		sides = []string{side}
		if e.Kind != "random_choose" && side != g.sideOf(self) {
			for _, i := range g.player(side).field {
				if !g.chargeQueryVisits(1) {
					return nil
				}
				if i.abilities["ability_target_guard"] {
					return values
				}
			}
		}
	}
	for _, side := range sides {
		values = append(values, ir.EventTarget{Kind: "leader", Side: side})
	}
	return values
}

func (g *game) boundLeaderSides(ref ir.Ref, self *instance, f frame) []string {
	switch r := ref.(type) {
	case ir.LeaderRef:
		_, side := g.playerForSide(self, r.Side)
		return []string{side}
	case ir.BindingRef:
		var sides []string
		for _, value := range f[r.Name] {
			if value.Kind == "leader" {
				sides = append(sides, value.Side)
			}
		}
		return sides
	}
	return nil
}
