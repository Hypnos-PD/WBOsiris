package runner

import (
	"fmt"

	"wbo/internal/ir"
)

func (g *game) condition(c ir.Condition) bool {
	switch x := c.(type) {
	case ir.OverflowCondition:
		return g.own.maxpp >= 7
	case ir.CompareCondition:
		n := 0
		switch x.Left.Field {
		case "combo":
			n = g.own.combo
		case "maxpp":
			n = g.own.maxpp
		case "life":
			n = g.own.leaderLife
		}
		switch x.Op {
		case "eq":
			return n == x.Right
		case "ne":
			return n != x.Right
		case "lt":
			return n < x.Right
		case "le":
			return n <= x.Right
		case "gt":
			return n > x.Right
		case "ge":
			return n >= x.Right
		}
	}
	return false
}
func (g *game) fromRef(ref ir.Ref, self *instance, f frame) []*instance {
	switch r := ref.(type) {
	case ir.SelfRef:
		return []*instance{self}
	case ir.BindingRef:
		return f[r.Name]
	case ir.ZoneRef:
		var out []*instance
		if r.Side == "oppo" {
			out = g.zone(&g.oppo, r.Zone)
		} else if r.Side == "own" {
			out = g.zone(&g.own, r.Zone)
		} else {
			out = append(append([]*instance{}, g.own.field...), g.oppo.field...)
		}
		return g.filter(out, r.Member, nil)
	case ir.FilterRef:
		return g.filter(g.fromRef(r.Source, self, f), "", r.Predicate)
	case ir.ExcludeRef:
		out := g.fromRef(r.Source, self, f)
		excluded := g.fromRef(r.Value, self, f)
		var result []*instance
		for _, i := range out {
			if !g.chargeQueryVisits(1) {
				break
			}
			if !contains(excluded, i) {
				result = append(result, i)
			}
		}
		return result
	}
	return nil
}
func (g *game) zone(p *player, z string) []*instance {
	switch z {
	case "deck":
		return p.deck
	case "hand":
		return p.hand
	case "field":
		return p.field
	case "graveyard":
		return p.graveyard
	case "banished":
		return p.banished
	case "destroyed":
		return p.destroyed
	}
	return nil
}
func (g *game) filter(items []*instance, member string, p ir.Predicate) []*instance {
	var out []*instance
	for _, i := range items {
		if !g.chargeQueryVisits(1) {
			break
		}
		if member != "" && member != "card" && i.card.CardType != member {
			continue
		}
		if g.matches(i, p) {
			out = append(out, i)
		}
	}
	return out
}
func (g *game) matches(i *instance, p ir.Predicate) bool {
	if p == nil {
		return true
	}
	switch x := p.(type) {
	case ir.FieldPredicate:
		switch x.Kind {
		case "has_card":
			return i.card.ID == x.CardID
		case "has_type":
			return i.card.CardType == x.CardType
		case "has_class":
			return i.card.Meta.Class == x.Class
		case "has_trait":
			for _, t := range i.card.Traits {
				if t == x.Trait {
					return true
				}
			}
			return false
		case "compare":
			switch x.Op {
			case "le":
				return i.life <= x.Value
			case "lt":
				return i.life < x.Value
			case "eq":
				return i.life == x.Value
			case "ge":
				return i.life >= x.Value
			case "gt":
				return i.life > x.Value
			}
		}
	case ir.AndPredicate:
		for _, t := range x.Terms {
			if !g.matches(i, t) {
				return false
			}
		}
		return true
	}
	return false
}
func (g *game) draw(e ir.DrawEffect, f frame) {
	count := e.Count
	if e.All {
		count = len(g.own.deck)
	}
	var drawn, kept []*instance
	for _, i := range g.own.deck {
		if !g.chargeQueryVisits(1) {
			return
		}
		if len(drawn) < count && g.matches(i, e.Predicate) {
			drawn = append(drawn, i)
		} else {
			kept = append(kept, i)
		}
	}
	if e.Predicate == nil {
		if !g.emit(ir.RuntimeEvent{Kind: "card_drawn", Side: "own", Count: len(drawn)}) {
			return
		}
	}
	for _, i := range drawn {
		i.zone = "hand"
		g.own.hand = append(g.own.hand, i)
	}
	g.own.deck = kept
	f[e.Output] = drawn
}
func (g *game) execCardEffect(e ir.CardEffect, self *instance, f frame) {
	switch e.Kind {
	case "add_card":
		for n := 0; n < e.Count; n++ {
			c := g.cards[e.CardID]
			if c == nil {
				continue
			}
			if !g.reserveCreatedInstance() {
				break
			}
			g.serial++
			i := g.newInstance(c, fmt.Sprintf("added-%d", g.serial), fmt.Sprintf("@added%d", g.serial), "hand")
			g.own.hand = append(g.own.hand, i)
		}
	case "summon":
		f[e.Output] = g.summon(e.Count, e.CardID)
	case "reanimate":
		var best *ir.Card
		for _, dead := range g.own.destroyed {
			if !g.chargeQueryVisits(1) {
				break
			}
			if dead.card.CardType == "follower" && dead.card.Cost <= e.MaxCost && (best == nil || dead.card.Cost > best.Cost) {
				best = dead.card
			}
		}
		if best != nil && (g.budget == nil || !g.budget.exceeded) {
			f[e.Output] = g.summon(1, best.ID)
		}
	case "transform":
		targets := g.fromRef(e.Target, self, f)
		if g.budget != nil && g.budget.exceeded {
			return
		}
		for _, i := range targets {
			if c := g.cards[e.CardID]; c != nil {
				i.card = c
			}
		}
	}
}
func (g *game) execTargetEffect(e ir.TargetEffect, self *instance, f frame) {
	targets := g.fromRef(e.Target, self, f)
	if g.budget != nil && g.budget.exceeded {
		return
	}
	switch e.Kind {
	case "destroy":
		for _, i := range targets {
			g.destroy(i)
		}
	case "banish":
		for _, i := range targets {
			g.move(i, "banished")
		}
	case "return":
		for _, i := range targets {
			g.returnCard(i, e.Destination)
		}
	case "heal":
		if r, ok := e.Target.(ir.LeaderRef); ok && r.Side == "own" {
			life := g.own.leaderLife + e.Amount
			if g.own.leaderMax > 0 && life > g.own.leaderMax {
				life = g.own.leaderMax
			}
			actual := life - g.own.leaderLife
			t := ir.EventTarget{Kind: "leader", Side: "own"}
			if g.emit(ir.RuntimeEvent{Kind: "healed", Actual: actual, Target: &t}) {
				g.own.leaderLife = life
			}
		}
	case "buff_stats":
		for _, i := range targets {
			i.attack += e.AttackDelta
			i.life += e.LifeDelta
		}
	case "add_keyword":
		for _, i := range targets {
			i.abilities[e.Keyword] = true
		}
	case "remove_keyword":
		for _, i := range targets {
			delete(i.abilities, e.Keyword)
		}
	case "silent_evolve":
		for _, i := range targets {
			i.evolved = true
		}
	case "damage":
		for _, i := range targets {
			i.life -= e.Amount
		}
	}
}
func (g *game) execAdjust(e ir.AdjustEffect, self *instance, f frame) {
	switch e.Kind {
	case "adjust_resource":
		if e.Resource == "combo" {
			g.own.combo += e.Delta
		} else if e.Resource == "maxpp" {
			g.own.maxpp += e.Delta
			if g.own.maxpp > 10 {
				g.own.maxpp = 10
			}
		}
	case "adjust_entity_field":
		targets := g.fromRef(e.Target, self, f)
		if g.budget != nil && g.budget.exceeded {
			return
		}
		for _, i := range targets {
			if e.Field == "countdown" {
				i.countdown += e.Delta
				if i.countdown <= 0 {
					g.destroy(i)
				}
			}
		}
	case "adjust_earthsigil":
		for _, i := range g.own.field {
			if !g.chargeQueryVisits(1) {
				return
			}
			if i.earthsigil > 0 {
				i.earthsigil += e.Delta
				break
			}
		}
	}
}
func (g *game) triggerSummoned(s *instance) {
	event := ir.RuntimeEvent{Kind: "follower_summoned", Side: "own", InstanceID: s.id, CardID: s.card.ID, Count: 1}
	if !g.emit(event) {
		return
	}
	field := append([]*instance{}, g.own.field...)
	for _, source := range field {
		if !g.chargeQueryVisits(1) {
			break
		}
		for _, a := range source.card.Abilities {
			t, ok := a.Trigger.(ir.EventTrigger)
			if !ok || t.Event != "follower_summoned" || !g.matches(s, t.Predicate) {
				continue
			}
			if !g.queueTrigger(triggerInvocation{body: a.Body, blockID: abilityBlockID(source.card.ID, a.ID), self: source, bindings: frame{"summoned": {s}}}) {
				return
			}
		}
	}
}
func (g *game) summon(count, id int) []*instance {
	var batch []*instance
	for n := 0; n < count && len(g.own.field) < fieldLimit; n++ {
		c := g.cards[id]
		if c == nil {
			break
		}
		if !g.reserveCreatedInstance() {
			break
		}
		g.serial++
		i := g.newInstance(c, fmt.Sprintf("summoned-%d", g.serial), fmt.Sprintf("@summoned%d", g.serial), "field")
		g.own.field = append(g.own.field, i)
		batch = append(batch, i)
		g.triggerSummoned(i)
		if g.budget != nil && g.budget.exceeded {
			break
		}
	}
	return batch
}
func (g *game) mergeEarthSigil(i *instance) {
	if i.earthsigil == 0 || i.zone != "field" {
		return
	}
	if !g.chargeQueryVisits(len(g.own.field)) {
		return
	}
	for _, old := range append([]*instance{}, g.own.field...) {
		if old != i && old.earthsigil > 0 {
			i.earthsigil += old.earthsigil
			g.move(old, "banished")
		}
	}
}
func (g *game) consumeEarthSigil(n int) bool {
	total := 0
	for _, i := range g.own.field {
		if !g.chargeQueryVisits(1) {
			return false
		}
		total += i.earthsigil
	}
	if total < n {
		return false
	}
	visits := 0
	remaining := n
	for _, i := range g.own.field {
		visits++
		remaining -= min(remaining, i.earthsigil)
		if remaining == 0 {
			break
		}
	}
	if !g.chargeQueryVisits(visits) {
		return false
	}
	for _, i := range append([]*instance{}, g.own.field...) {
		used := n
		if used > i.earthsigil {
			used = i.earthsigil
		}
		i.earthsigil -= used
		n -= used
		if i.earthsigil == 0 {
			g.move(i, "banished")
		}
		if n == 0 {
			break
		}
	}
	return true
}
func (g *game) destroy(i *instance) {
	if i.zone != "field" {
		return
	}
	subject := ir.EventTarget{Kind: "instance", InstanceID: i.id}
	if !g.emit(ir.RuntimeEvent{Kind: "destroyed", Subject: &subject}) {
		return
	}
	g.move(i, "graveyard")
	g.own.destroyed = append(g.own.destroyed, i)
	for _, a := range i.card.Abilities {
		if ir.TriggerKind(a.Trigger) == "lastwords" {
			if !g.queueTrigger(triggerInvocation{body: a.Body, blockID: abilityBlockID(i.card.ID, a.ID), self: i, bindings: frame{}}) {
				return
			}
		}
	}
}
func (g *game) returnCard(i *instance, z string) {
	if z != "deck" {
		g.move(i, z)
		return
	}
	p := g.owner(i)
	g.removeFromPlayer(p, i)
	pos := g.rng.Index(len(p.deck) + 1)
	p.deck = append(p.deck, nil)
	copy(p.deck[pos+1:], p.deck[pos:])
	p.deck[pos] = i
	i.zone = "deck"
}
func (g *game) move(i *instance, z string) {
	p := g.owner(i)
	g.removeFromPlayer(p, i)
	g.addToZone(p, i, z)
}
func (g *game) owner(i *instance) *player {
	for _, z := range [][]*instance{g.oppo.field, g.oppo.hand, g.oppo.deck, g.oppo.graveyard, g.oppo.banished} {
		if contains(z, i) {
			return &g.oppo
		}
	}
	return &g.own
}
func (g *game) removeFromPlayer(p *player, i *instance) {
	g.remove(&p.deck, i)
	g.remove(&p.hand, i)
	g.remove(&p.field, i)
	g.remove(&p.graveyard, i)
	g.remove(&p.banished, i)
}
func (g *game) remove(v *[]*instance, target *instance) {
	for n, i := range *v {
		if i == target {
			*v = append((*v)[:n], (*v)[n+1:]...)
			return
		}
	}
}
func contains(v []*instance, target *instance) bool {
	for _, i := range v {
		if i == target {
			return true
		}
	}
	return false
}
