package runner

import (
	"fmt"
	"maps"
	"wbo/internal/ir"
)

func (g *game) summonCopies(self *instance, side string, sources []*instance) []*instance {
	owner, _ := g.playerForSide(self, side)
	var summoned []*instance
	for _, source := range sources {
		if len(owner.field) >= fieldLimit || !g.chargeQueryVisits(1) {
			break
		}
		if source.zone != "hand" && source.zone != "field" || source.card.CardType == "spell" {
			continue
		}
		copy := g.copyInstance(source)
		if copy == nil {
			break
		}
		copy.zone = "field"
		copy.attacksUsed, copy.engaged = 0, false
		copy.summoningSick = copy.card.CardType == "follower"
		g.addToZone(owner, copy, "field")
		g.mergeEarthSigil(copy)
		summoned = append(summoned, copy)
		g.triggerSummoned(copy)
	}
	return summoned
}

// Attached fusion history is copied into independent instances as well.
func (g *game) copyInstance(source *instance) *instance {
	sources := []*instance{source}
	copies := []*instance{}
	for next := 0; next < len(sources); next++ {
		original := sources[next]
		if !g.chargeQueryVisits(1+len(original.materials)+len(original.counters)+len(original.abilities)+len(original.grants)+len(original.temporaryKeywords)+len(original.temporaryStats)) || !g.reserveCreatedInstance() {
			return nil
		}
		g.serial++
		copy := *original
		copy.grants = append([]ir.GrantEffect(nil), original.grants...)
		copy.id, copy.alias = fmt.Sprintf("summoned-%d", g.serial), fmt.Sprintf("@summoned%d", g.serial)
		copy.counters = maps.Clone(original.counters)
		copy.abilities = maps.Clone(original.abilities)
		copy.temporaryKeywords = maps.Clone(original.temporaryKeywords)
		copy.temporaryStats = maps.Clone(original.temporaryStats)
		copy.materials = nil
		copies = append(copies, &copy)
		sources = append(sources, original.materials...)
	}
	next := 1
	for n, original := range sources {
		copy := copies[n]
		if count := len(original.materials); count != 0 {
			copy.materials = copies[next : next+count : next+count]
			next += count
		}
		g.instances[copy.id] = copy
	}
	return copies[0]
}
