package runner

import "wbo/internal/ir"

func (g *game) summonFromDeck(e ir.DeckSummonEffect, self *instance, bindings frame) []*instance {
	owner, _ := g.playerForSide(self, "own")
	count := min(e.Count, fieldLimit-len(owner.field))
	if count <= 0 {
		return nil
	}
	candidates := g.fromRef(e.Source, self, bindings)
	if g.budget != nil && (g.budget.exceeded || !g.budget.chargeCandidates(uint64(len(candidates)))) {
		return nil
	}
	var selected []*instance
	for len(selected) < count && len(candidates) > 0 {
		if !g.chargeQueryVisits(len(candidates)) {
			return nil
		}
		index := 0
		if len(candidates) > 1 {
			index = g.rng.Index(len(candidates))
		}
		chosen := candidates[index]
		selected = append(selected, chosen)
		remaining := candidates[:0]
		for _, candidate := range candidates {
			if candidate != chosen && (!e.DistinctNames || candidate.card.ID != chosen.card.ID) {
				remaining = append(remaining, candidate)
			}
		}
		candidates = remaining
	}
	if len(selected) == 0 || !g.chargeQueryVisits(len(owner.deck)) {
		return nil
	}
	chosen := map[*instance]bool{}
	for _, i := range selected {
		chosen[i] = true
	}
	kept := owner.deck[:0]
	for _, i := range owner.deck {
		if !chosen[i] {
			kept = append(kept, i)
		}
	}
	owner.deck = kept
	var summoned []*instance
	for _, i := range selected {
		resetCombatState(i)
		i.summoningSick = i.card.CardType == "follower"
		i.engaged = false
		g.addToZone(owner, i, "field")
		g.mergeEarthSigil(i)
		summoned = append(summoned, i)
		g.triggerSummoned(i)
	}
	return summoned
}
