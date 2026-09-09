package project

import (
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func deckSummonIR(t []syntax.Token, base ir.NodeBase) (ir.DeckSummonEffect, bool) {
	e := ir.DeckSummonEffect{NodeBase: base, Kind: "summon_from_deck", Output: "summoned"}
	if len(t) < 9 || t[1].Value != "random" || !isUnsigned(t[2]) || t[3].Value != "from" {
		return e, false
	}
	e.Count = intToken(t[2])
	if e.Count < 1 || e.Count > 65535 {
		return e, false
	}
	var end int
	e.Source, end = setExprIR(t, 4)
	if !ir.ValidDeckSummonSource(e.Source) {
		return e, false
	}
	if end < len(t) && t[end].Value == "where" {
		next, ok := parseWhere(t, end)
		if !ok {
			return e, false
		}
		p, _ := filterIR(t, end)
		e.Source = ir.FilterRef{Kind: "filter", Source: e.Source, Predicate: p}
		end = next
	}
	if end+1 < len(t) && t[end].Value == "distinct" && t[end+1].Value == "names" {
		e.DistinctNames = true
		end += 2
	}
	return e, end == len(t)
}
