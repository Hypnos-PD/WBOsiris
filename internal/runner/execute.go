package runner

import (
	"fmt"

	"wbo/internal/ir"
)

func (g *game) condition(c ir.Condition, self *instance) bool {
	switch x := c.(type) {
	case ir.OverflowCondition:
		p, _ := g.playerForSide(self, x.Side)
		return p.maxpp >= 7
	case ir.CompareCondition:
		p, _ := g.playerForSide(self, x.Left.Side)
		n := 0
		if x.Left.Kind == "self_counter" {
			n = g.numericValue(&x.Left, self, nil)
		} else {
			switch x.Left.Field {
			case "cost", "distinct":
				if self == nil {
					return false
				}
				seen := map[int]bool{}
				for _, material := range self.materials {
					n += material.card.Cost
					seen[material.card.ID] = true
				}
				if x.Left.Field == "distinct" {
					n = len(seen)
				}
			case "combo":
				n = p.combo
			case "pp":
				n = p.pp
			case "maxpp":
				n = p.maxpp
			case "life":
				n = p.leaderLife
			case "ep":
				n = p.ep
			case "sep":
				n = p.sep
			case "shadows":
				n = p.shadows
			}
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

func (g *game) fusionCandidates(source *instance, ability *ir.FusionAbility) []*instance {
	if source == nil || ability == nil {
		return nil
	}
	filter := &ability.MaterialFilter
	items := g.fromRef(filter.Source, source, frame{})
	if filter.Predicate != nil {
		items = g.filter(items, "", filter.Predicate)
	}
	out := make([]*instance, 0, len(items))
	for _, item := range items {
		if item != source && item.zone == "hand" && !contains(out, item) {
			out = append(out, item)
		}
	}
	return out
}

func (g *game) availableFusion(source *instance) (*ir.FusionAbility, []*instance) {
	if source == nil || source.zone != "hand" || source.fusedThisTurn {
		return nil, nil
	}
	for n := range source.card.FusionAbilities {
		ability := &source.card.FusionAbilities[n]
		items := g.fusionCandidates(source, ability)
		if len(items) > 0 && len(items) >= ability.MaterialFilter.Minimum {
			return ability, items
		}
	}
	return nil, nil
}

func (g *game) commitFusion(source *instance, ability *ir.FusionAbility, materials []*instance) bool {
	if source == nil || source.zone != "hand" || source.fusedThisTurn || len(materials) == 0 {
		return false
	}
	candidates := g.fusionCandidates(source, ability)
	for _, material := range materials {
		if !contains(candidates, material) {
			return false
		}
	}
	owner := g.owner(source)
	side := g.sideOf(source)
	if !g.emit(ir.RuntimeEvent{Kind: "card_fused", Side: side, PrivateTo: side, From: "hand", Count: len(materials),
		Subject: &ir.EventTarget{Kind: "instance", InstanceID: source.id, CardID: source.card.ID}}) {
		return false
	}
	source.fusedThisTurn = true
	for _, material := range materials {
		g.remove(&owner.hand, material)
		material.zone = "attached"
		source.materials = append(source.materials, material)
	}
	return true
}
func (g *game) fromRef(ref ir.Ref, self *instance, f frame) []*instance {
	own, oppo, _ := g.relativePlayers(self)
	switch r := ref.(type) {
	case ir.SelfRef:
		return []*instance{self}
	case ir.BindingRef:
		return f[r.Name]
	case ir.ZoneRef:
		var out []*instance
		if r.Side == "oppo" {
			out = g.zone(oppo, r.Zone)
		} else if r.Side == "own" {
			out = g.zone(own, r.Zone)
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

// Target restrictions apply to player choices, not to set queries or random effects.
func (g *game) selectionCandidates(e ir.SelectionEffect, self *instance, f frame) []*instance {
	items := g.fromRef(e.Source, self, f)
	if e.Kind == "random_choose" {
		return items
	}
	controller := g.sideOf(self)
	out := make([]*instance, 0, len(items))
	for _, i := range items {
		if !g.chargeQueryVisits(1) {
			break
		}
		if i.zone == "field" && (i.abilities["stealth"] || i.abilities["aura"] || i.earthsigil > 0) && g.sideOf(i) != controller {
			continue
		}
		out = append(out, i)
	}
	return out
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
		case "has_form":
			if i.card.CardType != "follower" {
				return false
			}
			switch x.Form {
			case "unevolved":
				return !i.evolved && !i.superEvolved
			case "evolved":
				return i.evolved || i.superEvolved
			case "super_evolved":
				return i.superEvolved
			}
		case "has_trait":
			if x.Trait == "departed" && i.departed {
				return true
			}
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
			case "ne":
				return i.life != x.Value
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
	case ir.OrPredicate:
		for _, t := range x.Terms {
			if g.matches(i, t) {
				return true
			}
		}
		return false
	}
	return false
}
func (g *game) draw(e ir.DrawEffect, self *instance, f frame) {
	own, ownSide := g.playerForSide(self, e.Owner)
	count := e.Count
	if e.All {
		count = len(own.deck)
	}
	var drawn, kept []*instance
	for _, i := range own.deck {
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
		event := ir.RuntimeEvent{Kind: "card_drawn", Side: ownSide, Count: len(drawn)}
		if !g.emit(event) {
			return
		}
		g.queueEventTriggers(event, nil, "")
	}
	for _, i := range drawn {
		g.putInHandOrOverdraw(own, i)
	}
	own.deck = kept
	f[e.Output] = drawn
	if g.firstPlayer != "" && e.Predicate == nil && !e.All && len(drawn) < count {
		g.finishGame(oppositeSide(ownSide))
	}
}

func (g *game) putInHandOrOverdraw(owner *player, card *instance) {
	if len(owner.hand) < handLimit {
		card.zone = "hand"
		owner.hand = append(owner.hand, card)
		return
	}
	g.putInGraveyard(owner, card)
	event := ir.RuntimeEvent{Kind: "zone_moved", Side: g.sideOf(card), InstanceID: card.id}
	if g.emit(event) {
		g.queueEventTriggers(event, card, "")
	}
}

// Initial-state loading uses addToZone directly: shadows may already be spent.
func (g *game) putInGraveyard(owner *player, card *instance) {
	g.addToZone(owner, card, "graveyard")
	owner.shadows++
}

func (g *game) execCardEffect(e ir.CardEffect, self *instance, f frame) {
	own, _ := g.playerForSide(self, e.Owner)
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
			g.putInHandOrOverdraw(own, i)
		}
	case "summon":
		f[e.Output] = g.summonFor(self, e.Owner, e.Count, e.CardID, false)
	case "reanimate":
		f[e.Output] = nil
		if len(own.field) >= fieldLimit {
			return
		}
		var candidates []*ir.Card
		bestCost := -1
		for _, dead := range own.destroyed {
			if !g.chargeQueryVisits(1) {
				return
			}
			card := dead.card
			if card.CardType != "follower" || card.Cost > e.MaxCost || card.Cost < bestCost {
				continue
			}
			if card.Cost > bestCost {
				candidates = candidates[:0]
				bestCost = card.Cost
			}
			// Each destruction is a ticket, including repeated card identities.
			candidates = append(candidates, card)
		}
		if len(candidates) == 0 || g.budget != nil && !g.budget.chargeCandidates(uint64(len(candidates))) {
			return
		}
		selected := 0
		if len(candidates) > 1 {
			selected = g.rng.Index(len(candidates))
		}
		f[e.Output] = g.summonFor(self, e.Owner, 1, candidates[selected].ID, true)
	case "transform":
		targets := g.fromRef(e.Target, self, f)
		if g.budget != nil && g.budget.exceeded {
			return
		}
		for _, i := range targets {
			if c := g.cards[e.CardID]; c != nil {
				event := ir.RuntimeEvent{Kind: "card_transformed", Side: g.sideOf(i), From: i.zone,
					Subject: &ir.EventTarget{Kind: "instance", InstanceID: i.id, CardID: i.card.ID},
					Target:  &ir.EventTarget{Kind: "instance", InstanceID: i.id, CardID: c.ID}}
				if i.zone == "hand" || i.zone == "deck" || i.zone == "attached" {
					event.PrivateTo = event.Side
				}
				if !g.emit(event) {
					return
				}
				if i.zone == "field" {
					g.detachFieldSource(i)
				}
				resetCardState(i, c)
				if i.zone == "field" {
					i.summoningSick = c.CardType == "follower"
					g.triggerIndex.add(i)
				}
			}
		}
	}
}
func (g *game) execTargetEffect(e ir.TargetEffect, self *instance, f frame) {
	own, oppo, ownSide := g.relativePlayers(self)
	targets := g.fromRef(e.Target, self, f)
	if e.Predicate != nil {
		targets = g.filter(targets, "", e.Predicate)
	}
	// Snapshot all numeric inputs before any target or resulting event changes them.
	if e.AmountExpr != nil {
		e.Amount = max(0, g.numericValue(e.AmountExpr, self, f))
	}
	if e.AttackExpr != nil {
		e.AttackDelta = g.numericValue(e.AttackExpr, self, f)
	}
	if e.LifeExpr != nil {
		e.LifeDelta = g.numericValue(e.LifeExpr, self, f)
	}
	if g.budget != nil && g.budget.exceeded {
		return
	}
	switch e.Kind {
	case "set_attack_limit":
		for _, i := range targets {
			i.attackLimitValue = e.Amount
		}
	case "destroy":
		g.destroyByEffect(targets)
	case "banish":
		for _, i := range targets {
			g.move(i, "banished")
		}
	case "return":
		for _, i := range targets {
			g.returnCard(i, e.Destination)
		}
	case "heal":
		if r, ok := e.Target.(ir.LeaderRef); ok {
			target, side := own, ownSide
			if r.Side == "oppo" {
				target, side = oppo, oppositeSide(ownSide)
			}
			life := target.leaderLife + e.Amount
			if target.leaderMax > 0 && life > target.leaderMax {
				life = target.leaderMax
			}
			actual := life - target.leaderLife
			t := ir.EventTarget{Kind: "leader", Side: side}
			if g.emit(ir.RuntimeEvent{Kind: "healed", Actual: actual, Target: &t}) {
				target.leaderLife = life
			}
		}
	case "buff_stats":
		for _, i := range targets {
			i.attack += e.AttackDelta
			i.life += e.LifeDelta
		}
		g.resolveDeathBatch(nil)
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
			g.applyEvolution(i, e.Form == "super_evolved")
		}
	case "damage":
		if e.Distribution == "field_entry_order" {
			g.distributeDamage(self, targets, e.Amount, e.Overflow)
			return
		}
		if _, ok := e.Target.(ir.LeaderSetRef); ok {
			g.damageLeaders(self, e.Amount)
			return
		}
		if r, ok := e.Target.(ir.LeaderRef); ok {
			target, side := own, ownSide
			if r.Side == "oppo" {
				target, side = oppo, oppositeSide(ownSide)
			}
			g.damageLeaderFrom(self, target, side, e.Amount)
			return
		}
		for _, i := range targets {
			g.damageInstanceFrom(self, i, e.Amount, "effect")
		}
		g.resolveDeathBatch(nil)
	}
}
func (g *game) execAdjust(e ir.AdjustEffect, self *instance, f frame) {
	own, side := g.playerForSide(self, e.Owner)
	switch e.Kind {
	case "restore_resource":
		amount := max(0, own.maxpp-own.pp)
		if e.Resource == "pp" && g.emit(ir.RuntimeEvent{Kind: "pp_restored", Side: side, Actual: amount}) {
			own.pp += amount
		}
	case "adjust_resource":
		if e.Resource == "pp" {
			if e.Delta >= 0 {
				own.pp += min(e.Delta, max(0, own.maxpp-own.pp))
			} else {
				own.pp = max(0, own.pp+e.Delta)
			}
		} else if e.Resource == "combo" {
			own.combo += e.Delta
		} else if e.Resource == "shadows" {
			own.shadows = max(0, own.shadows+e.Delta)
		} else if e.Resource == "maxpp" {
			own.maxpp += e.Delta
			if own.maxpp > 10 {
				own.maxpp = 10
			}
		}
	case "adjust_entity_field":
		targets := g.fromRef(e.Target, self, f)
		if g.budget != nil && g.budget.exceeded {
			return
		}
		var expired []*instance
		for _, i := range targets {
			if e.Field == "cost" {
				i.cost = max(e.Minimum, i.cost+e.Delta)
			} else if e.Field == "countdown" {
				i.countdown += e.Delta
				if i.countdown <= 0 {
					expired = append(expired, i)
				}
			}
		}
		g.resolveDeathBatch(expired)
	case "adjust_earthsigil":
		for _, i := range own.field {
			if !g.chargeQueryVisits(1) {
				return
			}
			if i.earthsigil > 0 {
				i.earthsigil += e.Delta
				return
			}
		}
		if e.Delta > 0 {
			for _, sigil := range g.summonFor(self, "own", 1, ir.MagicSedimentCardID, false) {
				sigil.earthsigil = e.Delta
			}
		}
	case "spellboost":
		targets := g.fromRef(e.Target, self, f)
		for _, target := range targets {
			if target == nil || target.zone != "hand" {
				continue
			}
			for n := 0; n < e.Times; n++ {
				for _, ability := range target.card.Abilities {
					if ir.TriggerKind(ability.Trigger) == "spellboost" {
						g.queueTrigger(triggerInvocation{body: ability.Body, blockID: abilityBlockID(target.card.ID, ability.ID), self: target, bindings: frame{}})
					}
				}
			}
		}
	}
}
func (g *game) triggerSummoned(s *instance) {
	if s.card.CardType != "follower" {
		return
	}
	event := ir.RuntimeEvent{Kind: "follower_summoned", Side: g.sideOf(s), InstanceID: s.id, CardID: s.card.ID, Count: 1}
	if !g.emit(event) {
		return
	}
	g.queueEventTriggers(event, s, "summoned")
}

func (g *game) triggerEngaged(engaged *instance) {
	event := ir.RuntimeEvent{Kind: "amulet_engaged", Side: g.sideOf(engaged), InstanceID: engaged.id, CardID: engaged.card.ID, Count: 1}
	if !g.emit(event) {
		return
	}
	g.queueEventTriggers(event, engaged, "engaged")
}

func (g *game) summon(count, id int) []*instance {
	return g.summonFor(nil, "own", count, id, false)
}

func (g *game) summonFor(self *instance, side string, count, id int, departed bool) []*instance {
	own, _ := g.playerForSide(self, side)
	var batch []*instance
	for n := 0; n < count && len(own.field) < fieldLimit; n++ {
		c := g.cards[id]
		if c == nil {
			break
		}
		if !g.reserveCreatedInstance() {
			break
		}
		g.serial++
		i := g.newInstance(c, fmt.Sprintf("summoned-%d", g.serial), fmt.Sprintf("@summoned%d", g.serial), "field")
		i.departed = departed
		g.addToZone(own, i, "field")
		g.mergeEarthSigil(i)
		i.summoningSick = i.card.CardType == "follower"
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
	owner := g.owner(i)
	if !g.chargeQueryVisits(len(owner.field)) {
		return
	}
	for _, old := range append([]*instance{}, owner.field...) {
		if old != i && old.earthsigil > 0 {
			i.earthsigil += old.earthsigil
			g.move(old, "banished")
		}
	}
}
func (g *game) consumeEarthSigil(self *instance, n int) bool {
	if n == 0 {
		return true
	}
	own, _, _ := g.relativePlayers(self)
	total := 0
	for _, i := range own.field {
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
	for _, i := range own.field {
		visits++
		remaining -= min(remaining, i.earthsigil)
		if remaining == 0 {
			break
		}
	}
	if !g.chargeQueryVisits(visits) {
		return false
	}
	var exhausted []*instance
	for _, i := range append([]*instance{}, own.field...) {
		if i.earthsigil == 0 {
			continue
		}
		used := n
		if used > i.earthsigil {
			used = i.earthsigil
		}
		i.earthsigil -= used
		n -= used
		if i.earthsigil == 0 {
			exhausted = append(exhausted, i)
		}
		if n == 0 {
			break
		}
	}
	g.resolveDeathBatch(exhausted)
	return true
}

type deathRecord struct {
	instance  *instance
	owner     *player
	abilities []int
}

func (g *game) resolveDeathBatch(explicit []*instance) {
	// 先收齐本批死亡对象，避免谢幕曲看到只离场了一半的状态。
	marked := map[*instance]bool{}
	for _, i := range explicit {
		marked[i] = true
	}
	var deaths []deathRecord
	for _, side := range g.orderedSides() {
		p := g.player(side.name)
		for _, i := range p.field {
			if marked[i] || i.card.CardType == "follower" && i.life <= 0 {
				abilities := append([]int(nil), g.triggerIndex.abilities("lastwords", i)...)
				deaths = append(deaths, deathRecord{instance: i, owner: p, abilities: abilities})
			}
		}
	}
	if len(deaths) == 0 {
		return
	}
	triggerCount := 0
	for _, death := range deaths {
		triggerCount += len(death.abilities)
	}
	if g.budget != nil && (!g.budget.chargeEvents(uint64(len(deaths))) || !g.budget.chargeTriggers(uint64(triggerCount))) {
		return
	}
	g.deathBatchSerial++
	batchID := g.deathBatchSerial
	for _, death := range deaths {
		g.move(death.instance, "graveyard")
		death.owner.destroyed = append(death.owner.destroyed, death.instance)
	}
	for _, death := range deaths {
		g.eventSequence++
		subject := ir.EventTarget{Kind: "instance", InstanceID: death.instance.id}
		event := ir.RuntimeEvent{Kind: "destroyed", Side: g.sideOf(death.instance), Subject: &subject, Sequence: g.eventSequence, BatchID: batchID}
		g.events = append(g.events, event)
		g.queueEventTriggers(event, death.instance, "")
	}
	for _, death := range deaths {
		for _, abilityIndex := range death.abilities {
			ability := death.instance.card.Abilities[abilityIndex]
			g.triggers = append(g.triggers, triggerInvocation{body: ability.Body, blockID: abilityBlockID(death.instance.card.ID, ability.ID), self: death.instance, bindings: frame{}})
		}
	}
}
func (g *game) returnCard(i *instance, z string) {
	if z != "deck" {
		g.move(i, z)
		return
	}
	p := g.owner(i)
	if i.zone == "field" {
		g.detachFieldSource(i)
		resetCardState(i, i.card)
	}
	g.removeFromPlayer(p, i)
	pos := g.rng.Index(len(p.deck) + 1)
	p.deck = append(p.deck, nil)
	copy(p.deck[pos+1:], p.deck[pos:])
	p.deck[pos] = i
	i.zone = "deck"
}
func (g *game) move(i *instance, z string) {
	if i.zone == z {
		return
	}
	p := g.owner(i)
	if i.zone == "field" {
		g.detachFieldSource(i)
		if z == "hand" || z == "deck" {
			resetCardState(i, i.card)
		} else {
			resetCombatState(i)
		}
	}
	g.removeFromPlayer(p, i)
	if z == "graveyard" {
		g.putInGraveyard(p, i)
	} else if z == "hand" {
		g.putInHandOrOverdraw(p, i)
	} else {
		g.addToZone(p, i, z)
	}
}

func resetCombatState(i *instance) {
	i.attacksUsed = 0
	i.summoningSick = false
}
func (g *game) owner(i *instance) *player {
	for _, source := range g.instances {
		if contains(source.materials, i) {
			return g.owner(source)
		}
	}
	for _, z := range [][]*instance{g.oppo.field, g.oppo.hand, g.oppo.deck, g.oppo.graveyard, g.oppo.banished, g.oppo.resolving, g.oppo.destroyed} {
		if contains(z, i) {
			return &g.oppo
		}
	}
	return &g.own
}

func (g *game) sideOf(i *instance) string {
	if i != nil && g.owner(i) == &g.oppo {
		return "oppo"
	}
	return "own"
}

func (g *game) relativePlayers(self *instance) (own, oppo *player, ownSide string) {
	if g.sideOf(self) == "oppo" {
		return &g.oppo, &g.own, "oppo"
	}
	return &g.own, &g.oppo, "own"
}

func (g *game) playerForSide(self *instance, side string) (*player, string) {
	own, oppo, ownSide := g.relativePlayers(self)
	if side == "oppo" {
		return oppo, oppositeSide(ownSide)
	}
	return own, ownSide
}

func oppositeSide(side string) string {
	if side == "oppo" {
		return "own"
	}
	return "oppo"
}
func (g *game) removeFromPlayer(p *player, i *instance) {
	g.remove(&p.deck, i)
	g.remove(&p.hand, i)
	g.remove(&p.field, i)
	g.remove(&p.graveyard, i)
	g.remove(&p.banished, i)
	g.remove(&p.resolving, i)
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
