package runner

import (
	"fmt"

	"wbo/internal/ir"
)

// conditionIn 求值条件；带绑定名字的条件（例如 `opponent damaged`）需要当前帧。
func (g *game) conditionIn(c ir.Condition, self *instance, bindings frame) bool {
	switch x := c.(type) {
	case ir.IsDamagedCondition:
		for _, i := range g.boundInstances(bindings[x.Name]) {
			if i != nil && i.card.CardType == "follower" && i.damageTaken > 0 {
				return true
			}
		}
		return false
	case ir.CountCondition:
		// 计数条件可以引用绑定（例如 `count(own.field.amulets where card target) >= 1`），
		// 因此必须带上当前帧求值。
		return compareCount(g.numericValue(&ir.CountExpr{Kind: "count", Source: x.Source}, self, bindings), x.Op, x.Right)
	}
	return g.condition(c, self)
}

func compareCount(n int, op string, right int) bool {
	switch op {
	case "eq":
		return n == right
	case "ne":
		return n != right
	case "lt":
		return n < right
	case "le":
		return n <= right
	case "gt":
		return n > right
	case "ge":
		return n >= right
	}
	return false
}

func (g *game) condition(c ir.Condition, self *instance) bool {
	switch x := c.(type) {
	case ir.AttackHistoryCondition:
		p, side := g.playerForSide(self, x.Side)
		if x.LeaderLastTurn {
			return p.leaderAttackedLastTurn == x.Attacked
		}
		return (side == g.turn.Active && p.attackedThisTurn) == x.Attacked
	case ir.SkyboundArtCondition:
		return self != nil && g.turn.Number+self.skybound >= x.Level
	case ir.DeckDuplicatesCondition:
		p, _ := g.playerForSide(self, x.Side)
		seen := map[int]bool{}
		for _, card := range p.deck {
			if !g.chargeQueryVisits(1) {
				return false
			}
			if seen[card.card.ID] {
				return !x.Unique
			}
			seen[card.card.ID] = true
		}
		return x.Unique
	case ir.SameCostCondition:
		p, _ := g.playerForSide(self, x.Side)
		zone := p.hand
		if x.Zone == "deck" {
			zone = p.deck
		}
		counts := map[int]int{}
		for _, card := range zone {
			if !g.chargeQueryVisits(1) {
				return false
			}
			counts[card.cost]++
			if counts[card.cost] >= x.Count {
				return true
			}
		}
		return false
	case ir.SelfFormCondition:
		return self != nil && g.matches(self, ir.FieldPredicate{Kind: "has_form", Form: x.Form}, self, nil)
	case ir.EvolutionUnlockedCondition:
		_, side := g.playerForSide(self, x.Side)
		return g.evolutionUnlocked(side, x.Form == "super_evolved")
	case ir.OverflowCondition:
		p, _ := g.playerForSide(self, x.Side)
		return p.maxpp >= 7
	case ir.CountCondition:
		return compareCount(g.numericValue(&ir.CountExpr{Kind: "count", Source: x.Source}, self, nil), x.Op, x.Right)
	case ir.PlayedCostsCondition:
		p, _ := g.playerForSide(self, x.Side)
		for cost := x.From; cost <= x.To; cost++ {
			if !p.playedCosts[cost] {
				return false
			}
		}
		return true
	case ir.CompareCondition:
		n := 0
		if x.Left.Kind == "self_counter" || x.Left.Kind == "scalar" || x.Left.Kind == "self_scalar" {
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
		right := x.Right
		if x.RightExpr != nil {
			right = g.numericValue(x.RightExpr, self, nil)
		}
		switch x.Op {
		case "eq":
			return n == right
		case "ne":
			return n != right
		case "lt":
			return n < right
		case "le":
			return n <= right
		case "gt":
			return n > right
		case "ge":
			return n >= right
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
		items = g.filter(items, "", filter.Predicate, source, frame{})
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
		return g.filter(historyInstances(records, g.cards), r.Member, nil, self, f)
	case ir.SelfRef:
		if self != nil && self.zone == "retired_deck" {
			return nil
		}
		return []*instance{self}
	case ir.FaithRef:
		if self == nil || self.card == nil {
			return nil
		}
		for _, faith := range g.owner(self).crests {
			if faith.card != nil && faith.card.CardType == "faith" && faith.card.ID == self.card.ID {
				return []*instance{faith}
			}
		}
		return nil
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
		return g.filter(out, r.Member, nil, self, f)
	case ir.FilterRef:
		return g.filter(g.fromRef(r.Source, self, f), "", r.Predicate, self, f)
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
	if e.Kind == "random_choose" || e.Kind == "first" {
		// 随机与"从左起"都不是玩家指定目标，不受潜行/不可选中那类保护影响。
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
	case "entered":
		return p.entered
	}
	return nil
}
func (g *game) filter(items []*instance, member string, p ir.Predicate, self *instance, bindings frame) []*instance {
	var out []*instance
	for _, i := range items {
		if !g.chargeQueryVisits(1) {
			break
		}
		if member != "" && member != "card" && i.card.CardType != member {
			continue
		}
		if g.matches(i, p, self, bindings) {
			out = append(out, i)
		}
	}
	return out
}
func (g *game) matches(i *instance, p ir.Predicate, self *instance, bindings frame) bool {
	if p == nil {
		return true
	}
	switch x := p.(type) {
	case ir.NotPredicate:
		return !g.matches(i, x.Term, self, bindings)
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
		case "is_damaged":
			return i.card.CardType == "follower" && i.damageTaken > 0
		case "has_lastwords":
			for _, ability := range i.card.Abilities {
				if trigger, ok := ability.Trigger.(ir.SimpleTrigger); ok && trigger.Kind == "lastwords" {
					return true
				}
			}
			return false
		case "attacked_this_turn":
			return i.attacksUsed > 0
		case "not_attacked_this_turn":
			return i.attacksUsed == 0
		case "was_enhanced":
			return i.enhancedPlay
		case "cost_changed":
			return i.costChanged
		case "has_card":
			return i.card.ID == x.CardID
		case "same_card":
			for _, other := range g.fromRef(x.CardRef, self, bindings) {
				if other != nil && other.card != nil && i.card != nil && other.card.ID == i.card.ID {
					return true
				}
			}
			return false
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
				right = g.numericValue(x.ValueScalar, self, bindings)
			}
			value := i.life
			switch x.Field {
			case "cost":
				value = i.cost
			case "attack":
				value = i.currentAttack()
			case "base_cost":
				value = i.card.Cost
			case "base_attack":
				value = 0
				if i.card.Stats != nil {
					value = i.card.Stats.Attack
				}
			case "base_life":
				value = 0
				if i.card.Stats != nil {
					value = i.card.Stats.Life
				}
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
			if !g.matches(i, t, self, bindings) {
				return false
			}
		}
		return true
	case ir.OrPredicate:
		for _, t := range x.Terms {
			if g.matches(i, t, self, bindings) {
				return true
			}
		}
		return false
	}
	return false
}
func (g *game) draw(e ir.DrawEffect, self *instance, f frame) {
	own, ownSide := g.playerForSide(self, e.Owner)
	candidates := g.filter(own.deck, "", e.Predicate, self, f)
	if g.budget != nil && g.budget.exceeded {
		return
	}
	want := e.Count
	if e.CountExpr != nil {
		want = g.numericValue(e.CountExpr, self, f)
	}
	count := min(max(0, want), len(candidates))
	if e.DistinctNames {
		// "抽取 N 种…"：每种卡名最多抽一张，所以张数上限是候选里不同卡名的数量。
		count = min(count, distinctNameCount(candidates))
	}
	if e.All {
		count = len(candidates)
	}
	if e.Predicate != nil && !e.All && g.budget != nil && !g.budget.chargeCandidates(uint64(len(candidates))) {
		return
	}
	event := ir.RuntimeEvent{Kind: "card_drawn", Side: ownSide, Count: count}
	if !g.emit(event) {
		return
	}
	var drawn []*instance
	if e.Predicate != nil && !e.All {
		for n := 0; n < count && len(candidates) > 0; n++ {
			index := g.rng.Index(len(candidates))
			picked := candidates[index]
			drawn = append(drawn, picked)
			if e.DistinctNames {
				remaining := candidates[:0]
				for _, candidate := range candidates {
					if candidate != picked && candidate.card.ID != picked.card.ID {
						remaining = append(remaining, candidate)
					}
				}
				candidates = remaining
			} else {
				candidates = append(candidates[:index], candidates[index+1:]...)
			}
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
			g.triggerDrawn(i)
		}
	}
	kept := own.deck[:0]
	for _, i := range own.deck {
		if !selected[i] {
			kept = append(kept, i)
		}
	}
	own.deck = kept
	if g.firstPlayer != "" && e.Predicate == nil && !e.All && count < want {
		g.finishGame(oppositeSide(ownSide))
	}
}

// distinctNameCount 返回候选里不同卡牌定义的数量，用于"抽取 N 种…"的张数上限。
func distinctNameCount(cards []*instance) int {
	seen := make(map[int]bool, len(cards))
	for _, card := range cards {
		if card != nil {
			seen[card.card.ID] = true
		}
	}
	return len(seen)
}

// triggerDrawn 按"抽到的每一张卡"派发抽牌监听（公开事实仍然只记一次聚合事件）。
func (g *game) triggerDrawn(card *instance) {
	if card == nil || card.zone != "hand" {
		return
	}
	event := ir.RuntimeEvent{Kind: "card_drawn", Side: g.sideOf(card), Count: 1,
		Subject: &ir.EventTarget{Kind: "instance", InstanceID: card.id, CardID: card.card.ID}}
	g.queueEventTriggers(event, card, "drawn")
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
	case "banish_duplicates":
		owner, _ := g.playerForSide(self, e.Owner)
		seen := map[int]bool{}
		for _, card := range append([]*instance(nil), owner.deck...) {
			if !g.chargeQueryVisits(1) {
				return
			}
			if seen[card.card.ID] {
				g.move(card, "banished")
				continue
			}
			seen[card.card.ID] = true
		}
	case "add_card":
		added := []*instance{}
		for n := 0; n < e.Count; n++ {
			c := g.cards[e.CardID]
			if c == nil {
				continue
			}
			if !g.reserveCreatedInstance() {
				break
			}
			g.serial++
			if e.Destination == "deck" {
				// "将一张卡加入牌组"：在随机位置插入，不视为抽牌。
				i := g.newInstance(c, fmt.Sprintf("added-%d", g.serial), fmt.Sprintf("@added%d", g.serial), "deck")
				pos := g.rng.Index(len(own.deck) + 1)
				own.deck = append(own.deck, nil)
				copy(own.deck[pos+1:], own.deck[pos:])
				own.deck[pos] = i
				added = append(added, i)
				continue
			}
			i := g.newInstance(c, fmt.Sprintf("added-%d", g.serial), fmt.Sprintf("@added%d", g.serial), "hand")
			g.putInHandOrOverdraw(own, i)
			if i.zone == "hand" {
				added = append(added, i)
			}
		}
		if e.Output != "" {
			f[e.Output] = bindEntities(added...)
		}
	case "add_copies":
		// `add copies of 集合 to hand`：按每个目标当前的卡牌定义复制一张同名卡加入手牌。
		added := []*instance{}
		for _, target := range g.effectTargets(e.Target, self, f) {
			if target.card == nil || target.card.CardType == "crest" {
				continue
			}
			if !g.reserveCreatedInstance() {
				break
			}
			g.serial++
			if e.Destination == "deck" {
				i := g.newInstance(target.card, fmt.Sprintf("added-%d", g.serial), fmt.Sprintf("@added%d", g.serial), "deck")
				pos := g.rng.Index(len(own.deck) + 1)
				own.deck = append(own.deck, nil)
				copy(own.deck[pos+1:], own.deck[pos:])
				own.deck[pos] = i
				added = append(added, i)
				continue
			}
			i := g.newInstance(target.card, fmt.Sprintf("added-%d", g.serial), fmt.Sprintf("@added%d", g.serial), "hand")
			g.putInHandOrOverdraw(own, i)
			if i.zone == "hand" {
				added = append(added, i)
			}
		}
		if e.Output != "" {
			f[e.Output] = bindEntities(added...)
		}
	case "summon":
		bindSummoned(f, e.Output, g.summonFor(self, e.Owner, e.Count, e.CardID, false))
	case "summon_from_hand":
		// `summon target`：把已经存在于手牌的对象直接放到战场，不发动入场曲。
		batch := []*instance{}
		for _, target := range g.effectTargets(e.Target, self, f) {
			if target == nil || target.zone != "hand" || target.card == nil || len(own.field) >= fieldLimit {
				continue
			}
			if target.card.CardType != "follower" && target.card.CardType != "amulet" {
				continue
			}
			g.move(target, "field")
			target.summoningSick = target.card.CardType == "follower"
			g.mergeEarthSigil(target)
			batch = append(batch, target)
			g.triggerSummoned(target)
		}
		bindSummoned(f, e.Output, batch)
	case "summon_copies":
		bindSummoned(f, e.Output, g.summonCopies(self, e.Owner, g.effectTargets(e.Target, self, f)))
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
		bindSummoned(f, e.Output, g.summonFor(self, e.Owner, 1, candidates[selected].ID, true))
	case "transform":
		targets := g.effectTargets(e.Target, self, f)
		if g.budget != nil && g.budget.exceeded {
			return
		}
		if e.CopySource == nil {
			card := g.cards[e.CardID]
			for _, i := range targets {
				g.transformTarget(i, card)
			}
			return
		}
		// `transform <目标> into random card from <集合>`：每个目标各自等概率取一张
		// （"分别变身"），候选集合只查询一次；只剩一条候选时不消费随机决策。
		candidates := g.fromRef(e.CopySource, self, f)
		if g.budget != nil && (g.budget.exceeded || !g.budget.chargeCandidates(uint64(len(candidates)))) {
			return
		}
		if len(candidates) == 0 {
			return
		}
		for _, i := range targets {
			index := 0
			if len(candidates) > 1 {
				index = g.rng.Index(len(candidates))
			}
			g.transformTarget(i, candidates[index].card)
		}
	}
}

// transformTarget 把实例 i 的身份换成卡牌 c（保留实例身份、区域位置与附着材料）。
// 手牌、牌组与战场上的存续卡牌都可以变身；战场卡牌不能变身为法术。
func (g *game) transformTarget(i *instance, c *ir.Card) {
	if c == nil || i == nil {
		return
	}
	if i.zone != "hand" && i.zone != "deck" && (i.zone != "field" || c.CardType == "spell") {
		return
	}
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
func (g *game) execTargetEffect(e ir.TargetEffect, self *instance, f frame) {
	_, _, ownSide := g.relativePlayers(self)
	targets := g.effectTargets(e.Target, self, f)
	leaders := g.boundLeaderSides(e.Target, self, f)
	if e.Predicate != nil {
		targets = g.filter(targets, "", e.Predicate, self, f)
		leaders = nil
	}
	if e.Extremum != nil {
		targets = g.extremumCandidates(targets, e.Extremum)
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
	case "set_attack":
		for _, item := range targets {
			if item != nil && item.card.CardType == "follower" {
				item.attack = e.Amount
			}
		}
	case "set_cost":
		endingSide := g.effectEndingSide(e.Until, ownSide)
		for _, i := range targets {
			if i == nil {
				continue
			}
			if endingSide != "" {
				// "回合结束前，使其费用变为 0"：记录差量，到期时按差量还原。
				if i.temporaryCost == nil {
					i.temporaryCost = map[string]int{}
				}
				i.temporaryCost[endingSide] += max(e.Amount, 0) - i.cost
			}
			if i.cost != e.Amount {
				i.costChanged = true
			}
			i.cost = max(e.Amount, 0)
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
		banished := []*instance{}
		for _, i := range targets {
			g.move(i, "banished")
			banished = append(banished, i)
		}
		if e.Output != "" {
			// "因本能力消失的卡牌的张数"用 count(banished) 读取。
			f[e.Output] = bindEntities(banished...)
		}
	case "return":
		returned := []*instance{}
		for _, i := range targets {
			before := i.zone
			g.returnCard(i, e.Destination)
			if i.zone != before {
				returned = append(returned, i)
			}
		}
		if e.Output != "" {
			f[e.Output] = bindEntities(returned...)
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
		// 「在战场上获得攻击力或生命值增加时」「生命值在战场上被减少时」按本次增减入队。
		for _, i := range targets {
			if e.AttackDelta > 0 || e.LifeDelta > 0 {
				g.triggerStatsIncreased(i)
			}
			if e.LifeDelta < 0 {
				g.triggerLifeDecreased(i)
			}
		}
		g.resolveDeathBatch(nil)
	case "add_keyword":
		endingSide := g.effectEndingSide(e.Until, ownSide)
		for _, i := range targets {
			i.addKeyword(e.Keyword, endingSide)
		}
		g.setLeaderKeyword(leaders, e.Keyword, true, endingSide)
	case "remove_keyword":
		for _, i := range targets {
			i.removeKeyword(e.Keyword)
		}
		g.setLeaderKeyword(leaders, e.Keyword, false, "")
	case "remove_ability":
		for _, i := range targets {
			g.removeAbility(i, e.Keyword)
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
			if e.Extremum != nil {
				g.damageExtremumLeaders(self, e.Amount, e.Extremum)
			} else {
				g.damageLeaders(self, e.Amount)
			}
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
		} else if e.Resource == "ep" {
			// 官方 QA：回复进化点/超进化点不会超过上限（都是 2 点）。
			own.ep = min(2, max(0, own.ep+e.Delta))
		} else if e.Resource == "sep" {
			own.sep = min(2, max(0, own.sep+e.Delta))
		} else if e.Resource == "rally" {
			own.rally = max(0, own.rally+e.Delta)
		}
	case "adjust_entity_field":
		targets := g.effectTargets(e.Target, self, f)
		if len(targets) == 0 && self != nil && self.card != nil && self.card.CardType == "crest" {
			// 纹章用自己的 `reduce countdown self 1` 推进吟唱：纹章不在战场上，需要显式纳入。
			if _, ok := e.Target.(ir.SelfRef); ok {
				targets = []*instance{self}
			}
		}
		if g.budget != nil && g.budget.exceeded {
			return
		}
		var expired []*instance
		delta := e.Delta
		if e.DeltaExpr != nil {
			// 动态增量（例如"倒计数 -X"，X 为纹章数）在结算到该语句时求值。
			delta = g.numericValue(e.DeltaExpr, self, f)
		}
		for _, i := range targets {
			if e.Field == "cost" {
				if delta != 0 {
					i.costChanged = true
				}
				if endingSide := g.effectEndingSide(e.Until, side); endingSide != "" {
					// "到对手回合结束前，使其费用+1"：记录差量，到期按差量还原。
					if i.temporaryCost == nil {
						i.temporaryCost = map[string]int{}
					}
					i.temporaryCost[endingSide] += delta
				}
				i.cost = max(e.Minimum, i.cost+delta)
			} else if e.Field == "countdown" {
				i.countdown += delta
				if i.countdown <= 0 {
					if i.card.CardType == "crest" {
						g.expireCrest(g.owner(i), i)
						continue
					}
					expired = append(expired, i)
				}
			}
		}
		g.resolveDeathBatch(expired)
	case "halve_cost":
		// 当前费用向上取整的一半；重复发动基于已经改变的费用（官方 FAQ：9 → 5）。
		for _, i := range g.effectTargets(e.Target, self, f) {
			if i != nil && i.cost > 0 {
				i.cost = (i.cost + 1) / 2
				i.costChanged = true
			}
		}
	case "double_stats":
		// 每个目标按自己的当前数值翻倍；已受的伤害也翻倍，因此上限与当前生命一起变化。
		for _, i := range g.effectTargets(e.Target, self, f) {
			if i == nil || i.card.CardType != "follower" {
				continue
			}
			i.attack *= 2
			i.life *= 2
			i.damageTaken *= 2
		}
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
	case "adjust_skybound":
		for _, i := range g.effectTargets(e.Target, self, f) {
			if i != nil {
				i.skybound += e.Times
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
	if s.card.CardType == "follower" {
		g.countRally(s)
		g.trackEnteredArtifact(s)
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

// trackEnteredArtifact 记录"本场对战中进入过战场的创造物·随从"的卡牌种类，
// 读作 `own|oppo.entered_artifacts`（"X 为本次对战中进入战场的自己的创造物·随从的种类"）。
func (g *game) trackEnteredArtifact(s *instance) {
	if s == nil {
		return
	}
	recordEnteredArtifact(g.owner(s), s.card)
}

// recordEnteredArtifact 按卡牌定义记录种类；同一卡牌只算一种。
func recordEnteredArtifact(p *player, card *ir.Card) {
	if p == nil || card == nil || card.CardType != "follower" {
		return
	}
	artifact := false
	for _, trait := range card.Traits {
		if trait == "artifact" {
			artifact = true
			break
		}
	}
	if !artifact {
		return
	}
	if p.enteredArtifacts == nil {
		p.enteredArtifacts = map[int]bool{}
	}
	p.enteredArtifacts[card.ID] = true
}

// countRally 记录随从进入战场：打出的随从先挂起，其它召唤立即计入。
func (g *game) countRally(s *instance) {
	owner := g.owner(s)
	if owner == nil {
		return
	}
	if s.rallyPending {
		owner.pendingRally = append(owner.pendingRally, s)
		return
	}
	owner.rally++
}

// creditRally 在本次结算的效果执行完之后、触发队列结算之前，把打出的随从计入协作。
func (g *game) creditRally() {
	for _, p := range []*player{&g.own, &g.oppo} {
		for _, i := range p.pendingRally {
			if i != nil {
				p.rally++
				i.rallyPending = false
			}
		}
		p.pendingRally = nil
	}
}

func (g *game) triggerEngaged(engaged *instance) {
	event := ir.RuntimeEvent{Kind: "amulet_engaged", Side: g.sideOf(engaged), InstanceID: engaged.id, CardID: engaged.card.ID, Count: 1}
	if !g.emit(event) {
		return
	}
	g.queueEventTriggers(event, engaged, "engaged")
}

// triggerStatsIncreased 在随从于战场上获得攻击力或生命值增加时发出事件。
func (g *game) triggerStatsIncreased(target *instance) {
	if target == nil || target.card == nil || target.zone != "field" || target.card.CardType != "follower" {
		return
	}
	event := ir.RuntimeEvent{Kind: "stats_increased", Side: g.sideOf(target), InstanceID: target.id, CardID: target.card.ID, Count: 1}
	if g.emit(event) {
		g.queueEventTriggers(event, target, "")
	}
}

// triggerLifeDecreased 在随从于战场上生命值被减少时发出事件（伤害、减益、设置更低生命）。
func (g *game) triggerLifeDecreased(target *instance) {
	if target == nil || target.card == nil || target.zone != "field" || target.card.CardType != "follower" {
		return
	}
	event := ir.RuntimeEvent{Kind: "life_decreased", Side: g.sideOf(target), InstanceID: target.id, CardID: target.card.ID, Count: 1}
	if g.emit(event) {
		g.queueEventTriggers(event, target, "")
	}
}

// triggerPlayed 在卡牌被打出后发出 card_played 事件，监听者用 `when own card played` 声明。
func (g *game) triggerPlayed(played *instance) {
	if p := g.owner(played); p != nil && played.card != nil {
		// 记下"使用的卡牌的原始费用"，供"费用包含1到8所有数值"这类条件使用。
		if p.playedCosts == nil {
			p.playedCosts = map[int]bool{}
		}
		p.playedCosts[played.card.Cost] = true
	}
	event := ir.RuntimeEvent{Kind: "card_played", Side: g.sideOf(played), InstanceID: played.id, CardID: played.card.ID, Count: 1}
	if !g.emit(event) {
		return
	}
	g.queueEventTriggers(event, played, "played")
}

// invokeInstance 瞬念召唤：把牌组里的实例移到战场。不支付费用、不增加连击、
// 不算抽牌，也不发动入场曲；产生正常入场事件后派发"被瞬念召唤时"事件。
func (g *game) invokeInstance(i *instance) {
	if i == nil || i.zone != "deck" || i.card == nil {
		return
	}
	p := g.owner(i)
	if p == nil || len(p.field) >= fieldLimit {
		return
	}
	for index, deck := range p.deck {
		if deck != i {
			continue
		}
		p.deck = append(p.deck[:index], p.deck[index+1:]...)
		break
	}
	if i.zone != "deck" {
		return
	}
	g.detachEventSource(i)
	g.addToZone(p, i, "field")
	i.summoningSick = i.card.CardType == "follower"
	g.triggerSummoned(i)
	event := ir.RuntimeEvent{Kind: "card_invoked", Side: g.sideOf(i), InstanceID: i.id, CardID: i.card.ID, Count: 1}
	if !g.emit(event) {
		return
	}
	g.queueEventTriggers(event, i, "invoked")
}

// setLeaderKeyword 给主战者加上/移除关键词（【屏障】、"受到的伤害变为0"这类主战者级状态）。
// endingSide 非空时记录到期时点（"到对手的回合结束为止"）。
func (g *game) setLeaderKeyword(sides []string, keyword string, enabled bool, endingSide string) {
	for _, side := range sides {
		p := g.player(side)
		if p == nil {
			continue
		}
		if enabled {
			if p.leaderAbilities == nil {
				p.leaderAbilities = map[string]bool{}
			}
			p.leaderAbilities[keyword] = true
			if endingSide != "" {
				expiry := p.leaderTemporary[keyword]
				expiry.Permanent = false
				if endingSide == "own" {
					expiry.OwnTurnEnd = true
				} else {
					expiry.OppoTurnEnd = true
				}
				if p.leaderTemporary == nil {
					p.leaderTemporary = map[string]KeywordExpiry{}
				}
				p.leaderTemporary[keyword] = expiry
			}
		} else if p.leaderAbilities != nil {
			delete(p.leaderAbilities, keyword)
			delete(p.leaderTemporary, keyword)
		}
	}
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
