package runner

import (
	"fmt"

	"wbo/internal/engine/ir"
)

// summonFromPool 从几种指定卡中随机召唤：每次抽取各消费一次对局随机数。
func (g *game) summonFromPool(e ir.SummonPoolEffect, self *instance) []*instance {
	owner, _ := g.playerForSide(self, e.Owner)
	var batch []*instance
	for n := 0; n < e.Count && len(owner.field) < fieldLimit; n++ {
		if !g.chargeQueryVisits(len(e.Pool)) {
			return batch
		}
		index := 0
		if len(e.Pool) > 1 {
			index = g.rng.Index(len(e.Pool))
		}
		card := g.cards[e.Pool[index]]
		if card == nil {
			continue
		}
		if !g.reserveCreatedInstance() {
			break
		}
		g.serial++
		i := g.newInstance(card, fmt.Sprintf("summoned-%d", g.serial), fmt.Sprintf("@summoned%d", g.serial), "field")
		g.addToZone(owner, i, "field")
		g.mergeEarthSigil(i)
		i.summoningSick = card.CardType == "follower"
		batch = append(batch, i)
		g.triggerSummoned(i)
	}
	return batch
}
