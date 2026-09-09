package runner

import (
	"fmt"

	"wbo/internal/ir"
)

func (g *game) condition(c ir.Condition, self *instance) bool {
	switch x := c.(type) {
	case ir.AttackHistoryCondition:
		p, side := g.playerForSide(self, x.Side)
		return (side == g.turn.Active && p.attackedThisTurn) == x.Attacked
	case ir.SelfFormCondition:
		return self != nil && g.matches(self, ir.FieldPredicate{Kind: "has_form", Form: x.Form}, self)
	case ir.EvolutionUnlockedCondition:
		_, side := g.playerForSide(self, x.Side)
		return g.evolutionUnlocked(side, x.Form == "super_evolved")
	case ir.OverflowCondition:
		p, _ := g.playerForSide(self, x.Side)
		return p.maxpp >= 7
	case ir.CompareCondition:
		n := 0
		if x.Left.Kind == "self_counter" || x.Left.Kind == "scalar" {
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
		items = g.filter(items, "", filter.Predicate, source)
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
	event := ir.RuntimeEvent{Kind: "card_fused", Side: side, PrivateTo: side, From: "hand", Count: len(materials),
		Subject: &ir.EventTarget{Kind: "instance", InstanceID: source.id, CardID: source.card.ID}}
	if !g.emit(event) {
		return false
	}
	source.fusedThisTurn = true
	for _, material := range materials {
		g.remove(&owner.hand, material)
		material.zone = "attached"
		source.materials = append(source.materials, material)
	}
	return g.queueEventTriggers(event, source, "")
}
func (g *game) fromRef(ref ir.Ref, self *instance, f frame) []*instance {
	own, oppo, _ := g.relativePlayers(self)
	switch r := ref.(type) {
	case ir.HistoryRef:
		p, _ := g.playerForSide(self, r.Side)
		var records []DestructionRecord
		for _, record := range p.destroyed {
			if !g.chargeQueryVisits(1) {
				return nil
			}
			if record.TurnSide == g.turn.Active && record.TurnNumber == g.turn.Number {
				records = append(records, record)
			}
		}
		return g.filter(historyInstances(records, g.cards), r.Member, nil, self)
	case ir.SelfRef:
		if self != nil && self.zone == "retired_deck" {
			return nil
		}
		return []*instance{self}
	case ir.BindingRef:
		return g.boundInstances(f[r.Name])
	case ir.DestructionBatchRef:
		var targets []*instance
		seen := map[*instance]bool{}
		for _, ref := range r.Targets {
			for _, target := range g.fromRef(ref, self, f) {
				if !g.chargeQueryVisits(1) {
					return nil
				}
				if target != nil && !seen[target] {
					seen[target] = true
					targets = append(targets, target)
				}
			}
		}
		return targets
	case ir.ZoneRef:
		var out []*instance
		if r.Side == "oppo" {
			out = g.zone(oppo, r.Zone)
		} else if r.Side == "own" {
			out = g.zone(own, r.Zone)
		} else {
			out = append(append([]*instance{}, g.own.field...), g.oppo.field...)
		}
		return g.filter(out, r.Member, nil, self)
	case ir.FilterRef:
		return g.filter(g.fromRef(r.Source, self, f), "", r.Predicate, self)
	case ir.ExcludeRef:
		out := g.fromRef(r.Source, self, f)
		excluded := g.fromRef(r.Value, self, f)
		excludedIDs := map[string]bool{}
		for _, i := range excluded {
			if i != nil {
				excludedIDs[i.id] = true
			}
		}
		var result []*instance
		for _, i := range out {
			if !g.chargeQueryVisits(1) {
				break
			}
			if i != nil && !excludedIDs[i.id] {
				result = append(result, i)
			}
		}
		return result
	}
	return nil
}

// Target restrictions apply to player choices, not to set queries or random effects.
func (g *game) selectionCandidates(e ir.SelectionEffect, self *instance, f frame) []*instance {
	items := g.effectTargets(e.Source, self, f)
	if e.Kind == "random_choose" {
		return g.extremumCandidates(items, e.Extremum)
	}
	controller := g.sideOf(self)
	guarded := false
	for _, i := range g.player(oppositeSide(controller)).field {
		if !g.chargeQueryVisits(1) {
			return nil
		}
		if i.abilities["ability_target_guard"] {
			guarded = true
		}
	}
	out := make([]*instance, 0, len(items))
	for _, i := range items {
		if !g.chargeQueryVisits(1) {
			break
		}
		if i.zone == "field" && (i.abilities["stealth"] || i.abilities["aura"] || i.earthsigil > 0) && g.sideOf(i) != controller {
			continue
		}
		if guarded && i.zone == "field" && g.sideOf(i) != controller && !i.abilities["ability_target_guard"] {
			continue
		}
		out = append(out, i)
	}
	return g.extremumCandidates(out, e.Extremum)
}

func (g *game) extremumCandidates(items []*instance, extremum *ir.SelectionExtremum) []*instance {
	if extremum == nil {
		return items
	}
	var out []*instance
	best := 0
	for _, item := range items {
		if !g.chargeQueryVisits(1) {
			return nil
		}
		if item == nil || extremum.Field != "cost" && extremum.Field != "base_cost" && item.card.CardType != "follower" {
			continue
		}
		value := max(0, item.cost)
		switch extremum.Field {
		case "attack":
			value = item.currentAttack()
		case "life":
			value = item.life
		case "base_cost":
			value = item.card.Cost
		case "base_attack":
			value = item.card.Stats.Attack
		case "base_life":
			value = item.card.Stats.Life
		}
		if len(out) == 0 || extremum.Direction == "highest" && value > best || extremum.Direction == "lowest" && value < best {
			best, out = value, out[:0]
		}
		if value == best {
			out = append(out, item)
		}
	}
	return out
}
func (g *game) zone(p *player, z string) []*instance {
	switch z {
	case "crests":
		return p.crests
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
		return historyInstances(p.destroyed, g.cards)
	}
	return nil
}
func (g *game) filter(items []*instance, member string, p ir.Predicate, self *instance) []*instance {
	var out []*instance
	for _, i := range items {
		if !g.chargeQueryVisits(1) {
			break
		}
		if member != "" && member != "card" && i.card.CardType != member {
			continue
		}
		if g.matches(i, p, self) {
			out = append(out, i)
		}
	}
	return out
}
func (g *game) matches(i *instance, p ir.Predicate, self *instance) bool {
	if p == nil {
		return true
	}
	switch x := p.(type) {
	case ir.FieldPredicate:
		switch x.Kind {
		case "has_keyword":
			return i.abilities[x.Keyword]
		case "has_spellboost":
			for _, ability := range i.card.Abilities {
				if ir.TriggerKind(ability.Trigger) == "spellboost" {
					return true
				}
			}
			return false
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
			right := x.Value
			if x.ValueScalar != nil {
				right = g.numericValue(x.ValueScalar, self, nil)
			}
			value := i.life
			if x.Field == "cost" {
				value = i.cost
			}
			switch x.Op {
			case "le":
				return value <= right
			case "lt":
				return value < right
			case "eq":
				return value == right
			case "ne":
				return value != right
			case "ge":
				return value >= right
			case "gt":
				return value > right
			}
		}
	case ir.AndPredicate:
		for _, t := range x.Terms {
			if !g.matches(i, t, self) {
				return false
			}
		}
		return true
	case ir.OrPredicate:
		for _, t := range x.Terms {
			if g.matches(i, t, self) {
				return true
			}
		}
		return false
	}
	return false
}
func (g *game) draw(e ir.DrawEffect, self *instance, f frame) {
	own, ownSide := g.playerForSide(self, e.Owner)
	candidates := g.filter(own.deck, "", e.Predicate, self)
	if g.budget != nil && g.budget.exceeded {
		return
	}
	count := min(max(0, e.Count), len(candidates))
	if e.All {
		count = len(candidates)
	}
	if e.Predicate != nil && !e.All && g.budget != nil && !g.budget.chargeCandidates(uint64(len(candidates))) {
		return
	}
	event := ir.RuntimeEvent{Kind: "card_drawn", Side: ownSide, Count: count}
	if !g.emit(event) || !g.queueEventTriggers(event, nil, "") {
		return
	}
	var drawn []*instance
	if e.Predicate != nil && !e.All {
		for n := 0; n < count; n++ {
			index := g.rng.Index(len(candidates))
			drawn = append(drawn, candidates[index])
			candidates = append(candidates[:index], candidates[index+1:]...)
		}
	} else {
		drawn = candidates[:count]
	}
	selected := make(map[*instance]bool, len(drawn))
	f[e.Output] = nil
	for _, i := range drawn {
		selected[i] = true
		g.putInHandOrOverdraw(own, i)
		if i.zone == "hand" {
			f[e.Output] = append(f[e.Output], bindEntities(i)...)
		}
	}
	kept := own.deck[:0]
	for _, i := range own.deck {
		if !selected[i] {
			kept = append(kept, i)
		}
	}
	own.deck = kept
	if g.firstPlayer != "" && e.Predicate == nil && !e.All && count < e.Count {
		g.finishGame(oppositeSide(ownSide))
	}
}

func (g *game) putInHandOrOverdraw(owner *player, card *instance) {
	if len(owner.hand) < handLimit {
		card.zone = "hand"
		owner.hand = append(owner.hand, card)
		g.triggerIndex.add(card)
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
	case "gain_crest":
		g.gainCrest(self, e.Owner, e.CardID)
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
		f[e.Output] = bindEntities(g.summonFor(self, e.Owner, e.Count, e.CardID, false)...)
	case "summon_copies":
		f[e.Output] = bindEntities(g.summonCopies(self, e.Owner, g.effectTargets(e.Target, self, f))...)
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
			card := g.cards[dead.CardID]
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
		f[e.Output] = bindEntities(g.summonFor(self, e.Owner, 1, candidates[selected].ID, true)...)
	case "transform":
		targets := g.effectTargets(e.Target, self, f)
		if g.budget != nil && g.budget.exceeded {
			return
		}
		for _, i := range targets {
			if c := g.cards[e.CardID]; c != nil && (i.zone == "hand" || i.zone == "deck" || i.zone == "field" && c.CardType != "spell") {
				event := ir.RuntimeEvent{Kind: "card_transformed", Side: g.sideOf(i), From: i.zone,
					Subject: &ir.EventTarget{Kind: "instance", InstanceID: i.id, CardID: i.card.ID},
					Target:  &ir.EventTarget{Kind: "instance", InstanceID: i.id, CardID: c.ID}}
				if i.zone == "hand" || i.zone == "deck" || i.zone == "attached" {
					event.PrivateTo = event.Side
				}
				if !g.emit(event) {
					return
				}
				if i.zone == "field" || i.zone == "hand" {
					g.detachEventSource(i)
				}
				resetCardState(i, c)
				if i.zone == "field" {
					i.summoningSick = c.CardType == "follower"
				}
				g.triggerIndex.add(i)
			}
		}
	}
}
func (g *game) execTargetEffect(e ir.TargetEffect, self *instance, f frame) {
	_, _, ownSide := g.relativePlayers(self)
	targets := g.effectTargets(e.Target, self, f)
	leaders := g.boundLeaderSides(e.Target, self, f)
	if e.Predicate != nil {
		targets = g.filter(targets, "", e.Predicate, self)
		leaders = nil
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
	case "set_life":
		for _, item := range targets {
			if item != nil && item.card.CardType == "follower" {
				item.life = e.Amount
				item.damageTaken = 0
				item.clearTemporaryLife()
			}
		}
		g.resolveDeathBatch(nil)
	case "set_attack_limit":
		for _, i := range targets {
			i.attackLimitValue = e.Amount
		}
	case "set_damage_reduction":
		for _, i := range targets {
			i.damageReduction = e.Amount
		}
	case "destroy":
		destroyed := g.destroyByEffect(targets)
		if e.Output != "" {
			f[e.Output] = bindEntities(destroyed...)
		}
	case "discard":
		g.discardCards(targets)
	case "banish":
		for _, i := range targets {
			g.move(i, "banished")
		}
	case "return":
		for _, i := range targets {
			g.returnCard(i, e.Destination)
		}
	case "heal":
		for _, target := range targets {
			g.healFollower(target, e.Amount)
		}
		for _, side := range leaders {
			g.healLeader(g.player(side), side, e.Amount)
		}
	case "buff_stats":
		endingSide := g.effectEndingSide(e.Until, ownSide)
		for _, i := range targets {
			i.buffStats(e.AttackDelta, e.LifeDelta, endingSide)
		}
		g.resolveDeathBatch(nil)
	case "add_keyword":
		endingSide := g.effectEndingSide(e.Until, ownSide)
		for _, i := range targets {
			i.addKeyword(e.Keyword, endingSide)
		}
	case "remove_keyword":
		for _, i := range targets {
			i.removeKeyword(e.Keyword)
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
		for _, i := range targets {
			g.damageInstanceFrom(self, i, e.Amount, "effect")
		}
		for _, side := range leaders {
			g.damageLeaderFrom(self, g.player(side), side, e.Amount)
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
		targets := g.effectTargets(e.Target, self, f)
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
			for _, sigil := range g.summonFor(self, e.Owner, 1, ir.MagicSedimentCardID, false) {
				sigil.earthsigil = e.Delta
			}
		}
	case "spellboost":
		targets := g.effectTargets(e.Target, self, f)
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
	if s.card.CardType != "follower" && s.card.CardType != "amulet" {
		return
	}
	event := ir.RuntimeEvent{Kind: "follower_summoned", Side: g.sideOf(s), InstanceID: s.id, CardID: s.card.ID, Count: 1}
	if s.card.CardType == "amulet" {
		event.Kind = "amulet_summoned"
	}
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
	abilities []runtimeAbility
}

func (g *game) resolveDeathBatch(explicit []*instance) []*instance {
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
				abilities := append([]runtimeAbility(nil), g.triggerIndex.abilities("lastwords", i)...)
				deaths = append(deaths, deathRecord{instance: i, owner: p, abilities: abilities})
			}
		}
	}
	if len(deaths) == 0 {
		return nil
	}
	triggerCount := 0
	for _, death := range deaths {
		triggerCount += len(death.abilities)
	}
	if g.budget != nil && (!g.budget.chargeEvents(uint64(len(deaths))) || !g.budget.chargeTriggers(uint64(triggerCount))) {
		return nil
	}
	g.deathBatchSerial++
	batchID := g.deathBatchSerial
	for _, death := range deaths {
		g.move(death.instance, "graveyard")
		if g.attack != nil && death.instance.id == g.attack.defender {
			g.attack.defenderDestroyed = true
		}
	}
	for n, death := range deaths {
		death.owner.destroyed = append(death.owner.destroyed, destructionRecord(death.instance, g.eventSequence+uint64(n)+1, g.turn))
	}
	for _, death := range deaths {
		g.eventSequence++
		subject := ir.EventTarget{Kind: "instance", InstanceID: death.instance.id, CardID: death.instance.card.ID}
		event := ir.RuntimeEvent{Kind: "destroyed", Side: g.sideOf(death.instance), Subject: &subject, Sequence: g.eventSequence, BatchID: batchID}
		g.events = append(g.events, event)
		g.queueEventTriggers(event, death.instance, "destroyed")
	}
	for _, death := range deaths {
		for _, ability := range death.abilities {
			g.triggers = append(g.triggers, triggerInvocation{body: ability.Body, blockID: ability.blockID, self: death.instance, bindings: frame{}})
		}
	}
	destroyed := make([]*instance, 0, len(deaths))
	for _, death := range deaths {
		destroyed = append(destroyed, death.instance)
	}
	return destroyed
}
func (g *game) returnCard(i *instance, z string) {
	if i.zone == "retired_deck" {
		return
	}
	if i.zone == "graveyard" || i.zone == "banished" {
		resetCardState(i, i.card)
	}
	if z != "deck" {
		g.move(i, z)
		return
	}
	p := g.owner(i)
	if i.zone == "field" {
		g.notifyFollowerLeaving(i, "deck")
		g.detachEventSource(i)
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
	if i.zone == "retired_deck" {
		return
	}
	if i.zone == z {
		return
	}
	p := g.owner(i)
	if i.zone == "field" {
		g.notifyFollowerLeaving(i, z)
		g.detachEventSource(i)
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
	for _, z := range [][]*instance{g.oppo.field, g.oppo.hand, g.oppo.deck, g.oppo.graveyard, g.oppo.banished, g.oppo.resolving, g.oppo.crests, g.oppo.retiredCrests, g.oppo.retiredDeck} {
		if contains(z, i) {
			return &g.oppo
		}
	}
	for _, record := range g.oppo.destroyed {
		if record.InstanceID == i.id {
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
			if i.zone == "hand" {
				g.detachEventSource(i)
			}
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
