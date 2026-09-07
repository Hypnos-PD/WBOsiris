package runner

import (
	"fmt"
	"wbo/internal/ir"
)

const crestLimit = 5

func (g *game) gainCrest(self *instance, side string, cardID int) {
	p, owner := g.playerForSide(self, side)
	card := g.cards[cardID]
	if card == nil || card.Crest == nil || len(p.crests) >= crestLimit {
		return
	}
	for _, crest := range p.crests {
		if crest.card.ID == cardID {
			return
		}
	}
	if !g.reserveCreatedInstance() {
		return
	}
	g.serial++
	i := g.newInstance(card.CrestCard(), fmt.Sprintf("crest-%d", g.serial), "", "crests")
	g.addToZone(p, i, "crests")
	g.emit(ir.RuntimeEvent{Kind: "crest_gained", Side: owner, InstanceID: i.id, CardID: cardID})
}

func (g *game) tickCrests(p *player) {
	for _, crest := range append([]*instance(nil), p.crests...) {
		if !g.chargeQueryVisits(1) {
			return
		}
		if crest.countdown <= 0 {
			continue
		}
		crest.countdown--
		if !g.emit(ir.RuntimeEvent{Kind: "crest_countdown", Side: g.sideOf(crest), InstanceID: crest.id, CardID: crest.card.ID, Count: crest.countdown}) {
			return
		}
		if crest.countdown == 0 {
			owner := g.sideOf(crest)
			g.detachEventSource(crest)
			g.remove(&p.crests, crest)
			g.addToZone(p, crest, "retired_crest")
			if !g.emit(ir.RuntimeEvent{Kind: "crest_destroyed", Side: owner, InstanceID: crest.id, CardID: crest.card.ID}) {
				return
			}
			g.queueSimpleTriggers(crest, "lastwords")
		}
	}
}
