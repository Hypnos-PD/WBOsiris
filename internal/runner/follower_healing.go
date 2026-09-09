package runner

import "wbo/internal/ir"

// Damage is tracked separately from stat changes so healing cannot undo debuffs.
func (i *instance) maxLife() int {
	if i.card.CardType != "follower" {
		return 0
	}
	return max(0, i.life+i.damageTaken)
}

func (g *game) healFollower(target *instance, amount int) {
	if target == nil || target.zone != "field" || target.card.CardType != "follower" || target.life <= 0 {
		return
	}
	actual := min(max(0, amount), target.damageTaken)
	if actual == 0 {
		return
	}
	event := ir.RuntimeEvent{Kind: "healed", Side: g.sideOf(target), Actual: actual,
		Target: &ir.EventTarget{Kind: "instance", InstanceID: target.id, CardID: target.card.ID}}
	if g.emit(event) {
		target.life += actual
		target.damageTaken -= actual
		g.queueEventTriggers(event, target, "healed")
	}
}
