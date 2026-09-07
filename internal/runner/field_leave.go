package runner

import "wbo/internal/ir"

func (g *game) notifyFollowerLeaving(i *instance, destination string) {
	if i.card.CardType != "follower" {
		return
	}
	event := ir.RuntimeEvent{Kind: "follower_left", Side: g.sideOf(i), From: "field", To: destination,
		Subject: &ir.EventTarget{Kind: "instance", InstanceID: i.id, CardID: i.card.ID}}
	if g.emit(event) {
		g.queueEventTriggers(event, i, "left")
	}
}
