package runner

import "wbo/internal/ir"

func (g *game) summonFromHistory(e ir.HistorySummonEffect, self *instance, bindings frame) []*instance {
	owner, _ := g.playerForSide(self, e.Owner)
	if len(owner.field) >= fieldLimit {
		return nil
	}
	candidates := g.extremumCandidates(g.fromRef(e.Source, self, bindings), e.Extremum)
	if g.budget != nil && (g.budget.exceeded || !g.budget.chargeCandidates(uint64(len(candidates)))) {
		return nil
	}
	var out []*instance
	for n := 0; n < e.Count && len(candidates) > 0 && len(owner.field) < fieldLimit; n++ {
		if !g.chargeQueryVisits(1) {
			break
		}
		index := 0
		if len(candidates) > 1 {
			index = g.rng.Index(len(candidates))
		}
		cardID := candidates[index].card.ID
		candidates[index] = candidates[len(candidates)-1]
		candidates = candidates[:len(candidates)-1]
		out = append(out, g.summonFor(self, e.Owner, 1, cardID, false)...)
	}
	return out
}
