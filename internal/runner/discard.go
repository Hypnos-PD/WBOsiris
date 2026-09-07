package runner

import "wbo/internal/ir"

func (g *game) discardCards(targets []*instance) {
	for _, item := range targets {
		if item == nil || item.zone != "hand" {
			continue
		}
		event := ir.RuntimeEvent{Kind: "card_discarded", Side: g.sideOf(item), From: "hand", To: "graveyard",
			Subject: &ir.EventTarget{Kind: "instance", InstanceID: item.id, CardID: item.card.ID}}
		if !g.emit(event) {
			return
		}
		g.move(item, "graveyard")
		if !g.queueEventTriggers(event, item, "discarded") {
			return
		}
		// The discarded card is no longer a field source, so dispatch its own ability explicitly.
		for _, ability := range item.card.Abilities {
			trigger, ok := ability.Trigger.(ir.EventTrigger)
			if ok && trigger.SelfOnly && trigger.Event == "card_discarded" {
				if !g.queueTrigger(triggerInvocation{body: ability.Body, blockID: abilityBlockID(item.card.ID, ability.ID), self: item, bindings: frame{"discarded": bindEntities(item)}}) {
					return
				}
			}
		}
	}
}
