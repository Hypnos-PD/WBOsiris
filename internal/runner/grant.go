package runner

import (
	"fmt"
	"iter"
	"maps"
	"wbo/internal/ir"
)

type runtimeAbility struct {
	ir.Ability
	blockID string
}

type GrantedAbilityView struct {
	Kind   string            `json:"kind"`
	Labels map[string]string `json:"labels,omitempty"`
}

func grantedAbilityViews(i *instance) []GrantedAbilityView {
	var views []GrantedAbilityView
	for _, grant := range i.grants {
		views = append(views, GrantedAbilityView{Kind: ir.TriggerKind(grant.Ability.Trigger), Labels: maps.Clone(grant.Labels)})
	}
	return views
}

func (i *instance) triggeredAbilities() iter.Seq[runtimeAbility] {
	return func(yield func(runtimeAbility) bool) {
		for _, ability := range i.card.Abilities {
			if !yield(runtimeAbility{Ability: ability, blockID: abilityBlockID(i.card.ID, ability.ID)}) {
				return
			}
		}
		for _, grant := range i.grants {
			if !yield(runtimeAbility{Ability: grant.Ability, blockID: nestedBlockID(grant.ID, "granted")}) {
				return
			}
		}
	}
}

func (g *game) grantAbility(e ir.GrantEffect, self *instance, bindings frame) {
	for _, target := range g.fromRef(e.Target, self, bindings) {
		if !g.chargeQueryVisits(1) {
			return
		}
		if target.card.CardType != "follower" || target.zone != "field" && target.zone != "hand" {
			continue
		}
		if !g.chargeQueryVisits(len(target.card.Abilities) + len(target.grants) + 1) {
			return
		}
		target.grants = append(target.grants, e)
		g.triggerIndex.remove(target)
		g.triggerIndex.add(target)
	}
}

func grantIDs(i *instance) []string {
	if len(i.grants) == 0 {
		return nil
	}
	ids := make([]string, len(i.grants))
	for n, grant := range i.grants {
		ids[n] = grant.ID
	}
	return ids
}

func indexGrants(cards map[int]*ir.Card) (map[string]ir.GrantEffect, error) {
	blocks, err := indexBlocks(cards)
	if err != nil {
		return nil, err
	}
	grants := map[string]ir.GrantEffect{}
	for _, body := range blocks {
		for _, effect := range body {
			if grant, ok := effect.(ir.GrantEffect); ok {
				if !ir.ValidGrantedTrigger(grant.Ability.Trigger) {
					return nil, fmt.Errorf("invalid granted trigger")
				}
				grants[grant.ID] = grant
			}
		}
	}
	return grants, nil
}
