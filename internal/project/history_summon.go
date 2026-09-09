package project

import (
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func parseExtremum(t []syntax.Token, i int) (*ir.SelectionExtremum, int, bool) {
	if i >= len(t) || !set("highest", "lowest")[t[i].Value] {
		return nil, i, true
	}
	direction := t[i].Value
	i++
	prefix := ""
	if i+1 < len(t) && t[i].Value == "base" && t[i+1].Value == "." {
		prefix = "base_"
		i += 2
	}
	if i >= len(t) || !set("attack", "life", "cost")[t[i].Value] {
		return nil, i, false
	}
	return &ir.SelectionExtremum{Direction: direction, Field: prefix + t[i].Value}, i + 1, true
}

func historySummonIR(t []syntax.Token, base ir.NodeBase) (ir.HistorySummonEffect, bool) {
	e := ir.HistorySummonEffect{NodeBase: base, Kind: "summon_from_history", Owner: "own", Output: "summoned"}
	if len(t) < 7 || t[1].Value != "random" || !isUnsigned(t[2]) || t[3].Value != "from" {
		return e, false
	}
	e.Count = intToken(t[2])
	if e.Count < 1 || e.Count > 65535 {
		return e, false
	}
	var end int
	e.Source, end = setExprIR(t, 4)
	if !ir.ValidHistorySummonSource(e.Source) {
		return e, false
	}
	if end < len(t) && t[end].Value == "where" {
		next, ok := parseWhere(t, end)
		if !ok {
			return e, false
		}
		predicate, _ := filterIR(t, end)
		e.Source = ir.FilterRef{Kind: "filter", Source: e.Source, Predicate: predicate}
		end = next
	}
	var ok bool
	e.Extremum, end, ok = parseExtremum(t, end)
	return e, ok && end == len(t)
}
