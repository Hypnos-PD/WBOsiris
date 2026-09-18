package runner

import (
	"fmt"
	"wbo/internal/ir"
)

const crestLimit = 5

// placeFaith 在对局开始时按"初始牌组里出现的信仰定义"放置信仰实体
// （信仰与纹章共用主战者区域，因此同样放在 p.crests 里，卡牌类型为 faith，信仰值放在 counters["value"]）。
func (g *game) placeFaith(p *player, side string, cardID int) {
	card := g.cards[cardID]
	if card == nil || card.Faith == nil || len(p.crests) >= crestLimit {
		return
	}
	for _, faith := range p.crests {
		if faith.card != nil && faith.card.ID == cardID && faith.card.CardType == "faith" {
			return
		}
	}
	g.serial++
	i := g.newInstance(card.FaithCard(), fmt.Sprintf("faith-%d", g.serial), "", "crests")
	g.addToZone(p, i, "crests")
	g.emit(ir.RuntimeEvent{Kind: "faith_placed", Side: side, InstanceID: i.id, CardID: cardID})
}

// consumeFaith 支付「本卡牌定义的信仰」的信仰值；信仰不存在或余额不足时返回 false，
// 由调用方跳过整个效果块。
func (g *game) consumeFaith(self *instance, amount int) bool {
	if self == nil || self.card == nil {
		return false
	}
	owner := g.owner(self)
	for _, faith := range owner.crests {
		if faith.card == nil || faith.card.CardType != "faith" || faith.card.ID != self.card.ID {
			continue
		}
		value, ok := faith.counters["value"]
		if !ok || value < amount {
			return false
		}
		faith.counters["value"] = value - amount
		g.emit(ir.RuntimeEvent{Kind: "faith_consumed", Side: g.sideOf(faith), InstanceID: faith.id, CardID: faith.card.ID, Count: amount})
		return true
	}
	return false
}

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
			g.expireCrest(p, crest)
		}
	}
}

// expireCrest 让吟唱归零的纹章退场并触发谢幕曲。
func (g *game) expireCrest(p *player, crest *instance) {
	owner := g.sideOf(crest)
	g.detachEventSource(crest)
	g.remove(&p.crests, crest)
	g.addToZone(p, crest, "retired_crest")
	if !g.emit(ir.RuntimeEvent{Kind: "crest_destroyed", Side: owner, InstanceID: crest.id, CardID: crest.card.ID}) {
		return
	}
	g.queueSimpleTriggers(crest, "lastwords")
}
