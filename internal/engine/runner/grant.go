package runner

import (
	"fmt"
	"iter"
	"maps"
	"sort"

	"wbo/internal/engine/ir"
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
		if i.suppressAll {
			return
		}
		for _, ability := range i.card.Abilities {
			if i.suppresses(ir.TriggerKind(ability.Trigger)) {
				continue
			}
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

// suppresses 报告本实例是否已经失去某一类触发能力（例如 remove lastwords from …）。
func (i *instance) suppresses(kind string) bool {
	return i != nil && kind != "" && i.suppressed[kind]
}

// removeAbility 让实例失去一类触发能力；all 表示"失去所有能力"。
// 失去的能力不会因为重新索引或能力查询而恢复，离开战场后随实例一起消失。
//
// 官方 QA（mxtwkh_pd）：失去能力只影响之后的能力查询，已经进入队列的触发
// 不受影响，仍然会结算完（『唯一王者·别西卜』的【入场曲】让『雷维翁超越者·尤里乌斯』
// 失去所有能力后，尤里乌斯的能力依旧完成结算）。
func (g *game) removeAbility(i *instance, ability string) {
	if i == nil || i.card.CardType != "follower" && i.card.CardType != "amulet" {
		return
	}
	if ability == "all" {
		i.suppressAll = true
		i.abilities = map[string]bool{}
		i.temporaryKeywords = nil
		i.grants = nil
	} else {
		if i.suppressed == nil {
			i.suppressed = map[string]bool{}
		}
		i.suppressed[ability] = true
	}
	g.triggerIndex.remove(i)
	g.triggerIndex.add(i)
}

func (g *game) grantAbility(e ir.GrantEffect, self *instance, bindings frame) {
	for _, target := range g.effectTargets(e.Target, self, bindings) {
		if !g.chargeQueryVisits(1) {
			return
		}
		if target.card.CardType != "follower" && target.card.CardType != "faith" {
			continue
		}
		if target.card.CardType == "follower" && target.zone != "field" && target.zone != "hand" {
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

func suppressedAbilities(i *instance) []string {
	if len(i.suppressed) == 0 {
		return nil
	}
	names := make([]string, 0, len(i.suppressed))
	for name, on := range i.suppressed {
		if on {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
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
