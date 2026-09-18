package runner

import (
	"fmt"

	"wbo/internal/ir"
)

func (g *game) summonFromHistory(e ir.HistorySummonEffect, self *instance, bindings frame) []*instance {
	owner, _ := g.playerForSide(self, e.Owner)
	if e.Destination != "" {
		return g.copyFromHistory(e, owner, self, bindings)
	}
	if len(owner.field) >= fieldLimit {
		return nil
	}
	candidates := g.extremumCandidates(g.fromRef(e.Source, self, bindings), e.Extremum)
	if g.budget != nil && (g.budget.exceeded || !g.budget.chargeCandidates(uint64(len(candidates)))) {
		return nil
	}
	var out []*instance
	for n := 0; n < e.Count && len(candidates) > 0 && len(owner.field) < fieldLimit; n++ {
		if !g.chargeQueryVisits(1) {
			break
		}
		index := 0
		if len(candidates) > 1 {
			index = g.rng.Index(len(candidates))
		}
		cardID := candidates[index].card.ID
		remaining := candidates[:0]
		for _, candidate := range candidates {
			if candidate != candidates[index] && (!e.DistinctNames || candidate.card.ID != cardID) {
				remaining = append(remaining, candidate)
			}
		}
		candidates = remaining
		out = append(out, g.summonFor(self, e.Owner, 1, cardID, false)...)
	}
	return out
}

// copyFromHistory 按破坏历史复制同名卡加入手牌或牌组：每次抽选消费一次随机数，
// `distinctNames` 抽到后排除同名记录。复制体是全新的实例（原始卡面状态）。
func (g *game) copyFromHistory(e ir.HistorySummonEffect, owner *player, self *instance, bindings frame) []*instance {
	candidates := g.extremumCandidates(g.fromRef(e.Source, self, bindings), e.Extremum)
	if g.budget != nil && (g.budget.exceeded || !g.budget.chargeCandidates(uint64(len(candidates)))) {
		return nil
	}
	var out []*instance
	for n := 0; n < e.Count && len(candidates) > 0; n++ {
		if !g.chargeQueryVisits(1) {
			break
		}
		index := 0
		if len(candidates) > 1 {
			index = g.rng.Index(len(candidates))
		}
		picked := candidates[index]
		remaining := candidates[:0]
		for _, candidate := range candidates {
			if candidate != picked && (!e.DistinctNames || candidate.card.ID != picked.card.ID) {
				remaining = append(remaining, candidate)
			}
		}
		candidates = remaining
		if picked.card == nil || picked.card.CardType == "crest" || !g.reserveCreatedInstance() {
			continue
		}
		g.serial++
		i := g.newInstance(picked.card, fmt.Sprintf("added-%d", g.serial), fmt.Sprintf("@added%d", g.serial), e.Destination)
		if e.Destination == "deck" {
			pos := g.rng.Index(len(owner.deck) + 1)
			owner.deck = append(owner.deck, nil)
			copy(owner.deck[pos+1:], owner.deck[pos:])
			owner.deck[pos] = i
			out = append(out, i)
			continue
		}
		g.putInHandOrOverdraw(owner, i)
		if i.zone == "hand" {
			out = append(out, i)
		}
	}
	return out
}

// copyRandomFrom 从任意集合随机抽选若干张，按卡牌定义各复制 1 张加入手牌或牌组
//（"将对手的手牌中的随机1张卡牌的复制卡牌以非公开形式加入自己的手牌"）。
// 抽选不放回，每条候选消费一次随机决策（只剩一条候选时不消费）；只读取卡牌定义，
// 来源区的身份不会出现在事件流里。
func (g *game) copyRandomFrom(e ir.CopyRandomEffect, self *instance, bindings frame) []*instance {
	owner, _ := g.playerForSide(self, e.Owner)
	candidates := g.fromRef(e.Source, self, bindings)
	if g.budget != nil && (g.budget.exceeded || !g.budget.chargeCandidates(uint64(len(candidates)))) {
		return nil
	}
	var out []*instance
	for n := 0; n < e.Count && len(candidates) > 0; n++ {
		if !g.chargeQueryVisits(1) {
			break
		}
		index := 0
		if len(candidates) > 1 {
			index = g.rng.Index(len(candidates))
		}
		picked := candidates[index]
		candidates = append(candidates[:index], candidates[index+1:]...)
		if picked.card == nil || picked.card.CardType == "crest" || !g.reserveCreatedInstance() {
			continue
		}
		g.serial++
		i := g.newInstance(picked.card, fmt.Sprintf("added-%d", g.serial), fmt.Sprintf("@added%d", g.serial), e.Destination)
		if e.Destination == "deck" {
			pos := g.rng.Index(len(owner.deck) + 1)
			owner.deck = append(owner.deck, nil)
			copy(owner.deck[pos+1:], owner.deck[pos:])
			owner.deck[pos] = i
			out = append(out, i)
			continue
		}
		g.putInHandOrOverdraw(owner, i)
		if i.zone == "hand" {
			out = append(out, i)
		}
	}
	if e.Output != "" && bindings != nil {
		bindings[e.Output] = bindEntities(out...)
	}
	return out
}

// randomCardFrom 从集合里等概率取一张的卡牌定义（用于"变身为随机一张卡的复制"）。
// 空集合返回 nil；多条候选时消费一次随机决策。
func (g *game) randomCardFrom(ref ir.Ref, self *instance, bindings frame) *ir.Card {
	candidates := g.fromRef(ref, self, bindings)
	if g.budget != nil && (g.budget.exceeded || !g.budget.chargeCandidates(uint64(len(candidates)))) {
		return nil
	}
	if len(candidates) == 0 {
		return nil
	}
	index := 0
	if len(candidates) > 1 {
		index = g.rng.Index(len(candidates))
	}
	return candidates[index].card
}
