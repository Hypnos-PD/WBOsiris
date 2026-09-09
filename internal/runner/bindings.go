package runner

import "wbo/internal/ir"

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
	_, side := g.playerForSide(self, characters.Side)
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
	return append(values, ir.EventTarget{Kind: "leader", Side: side})
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
