package project

import (
	"wbo/internal/engine/ir"
	"wbo/internal/engine/syntax"
)

func deckReplaceIR(t []syntax.Token, base ir.NodeBase) (ir.DeckReplaceEffect, bool) {
	e := ir.DeckReplaceEffect{NodeBase: base, Kind: "replace_deck"}
	if len(t) < 9 || t[0].Value != "replace" || !set("own", "oppo")[t[1].Value] || values(t[2:6]) != ". deck with shuffled" {
		return e, false
	}
	e.Owner = t[1].Value
	for n := 6; n < len(t); n += 4 {
		if n+3 > len(t) || !isUnsigned(t[n]) || t[n+1].Value != "card" || !isCardID(t[n+2]) {
			return e, false
		}
		e.Cards = append(e.Cards, ir.DeckEntry{CardID: intToken(t[n+2]), Count: intToken(t[n])})
		if n+3 < len(t) && (t[n+3].Value != "," || n+4 == len(t)) {
			return e, false
		}
	}
	return e, ir.ValidDeckRecipe(e.Cards)
}

func leaderMaxLifeIR(t []syntax.Token, base ir.NodeBase) (ir.LeaderMaxLifeEffect, bool) {
	e := ir.LeaderMaxLifeEffect{NodeBase: base, Kind: "set_leader_max_life"}
	if len(t) != 6 || !set("own", "oppo")[t[2].Value] || values(t[3:5]) != ". leader" || !isUnsigned(t[5]) {
		return e, false
	}
	amount := intToken(t[5])
	if amount < 1 || amount > 65535 {
		return e, false
	}
	e.Side, e.Amount = t[2].Value, amount
	switch values(t[:2]) {
	case "set maxlife":
		return e, true
	case "raise maxlife":
		e.Kind, e.Delta = "adjust_leader_max_life", true
		return e, true
	case "reduce maxlife":
		// `reduce maxlife own|oppo.leader N`：把生命上限降低 N，最低为 1。
		e.Kind, e.Amount, e.Delta = "adjust_leader_max_life", -amount, true
		return e, true
	}
	return e, false
}
