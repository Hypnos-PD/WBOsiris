package runner

import (
	"fmt"
	"wbo/internal/ir"
)

func (g *game) replaceDeck(e ir.DeckReplaceEffect, self *instance) {
	owner, side := g.playerForSide(self, e.Owner)
	total := 0
	for _, entry := range e.Cards {
		if g.cards[entry.CardID] == nil {
			return
		}
		total += entry.Count
	}
	// Reserve the complete replacement before changing the old deck or RNG.
	if !g.chargeQueryVisits(len(owner.deck)+total) || g.budget != nil && !g.budget.chargeCreatedInstances(uint64(total)) {
		return
	}
	cards := make([]*ir.Card, 0, total)
	for _, entry := range e.Cards {
		for range entry.Count {
			cards = append(cards, g.cards[entry.CardID])
		}
	}
	for n := len(cards) - 1; n > 0; n-- {
		j := g.rng.Index(n + 1)
		cards[n], cards[j] = cards[j], cards[n]
	}
	for _, old := range owner.deck {
		g.detachEventSource(old)
		g.addToZone(owner, old, "retired_deck")
	}
	owner.deck = nil
	for _, card := range cards {
		g.serial++
		i := g.newInstance(card, fmt.Sprintf("added-%d", g.serial), "", "deck")
		g.addToZone(owner, i, "deck")
	}
	// Only the new count is public; neither deck's identities or order are exposed.
	g.emit(ir.RuntimeEvent{Kind: "deck_replaced", Side: side, Count: total})
}

func (g *game) setLeaderMaxLife(e ir.LeaderMaxLifeEffect, self *instance) {
	p, side := g.playerForSide(self, e.Side)
	p.leaderMax = e.Amount
	p.leaderLife = min(p.leaderLife, p.leaderMax)
	g.emit(ir.RuntimeEvent{Kind: "leader_max_life_set", Side: side, Count: p.leaderMax, Actual: p.leaderLife})
}
