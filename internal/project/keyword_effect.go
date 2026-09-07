package project

import (
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func keywordEffectIR(base ir.NodeBase, kind string, t []syntax.Token) ir.TargetEffect {
	end := valueRefEnd(t, 3)
	target := valueRefIR(t, 3)
	if end < len(t) && t[end].Value == "other" {
		target = ir.ExcludeRef{Kind: "exclude", Source: target, Value: ir.SelfRef{Kind: "self"}}
		end++
	}
	e := ir.TargetEffect{NodeBase: base, Kind: kind, Keyword: t[1].Value, Target: target}
	if end < len(t) && t[end].Value == "where" {
		e.Predicate, end = filterIR(t, end)
	}
	if end < len(t) {
		e.Until = effectDurationIR(t, end)
	}
	return e
}

func effectDurationIR(t []syntax.Token, start int) string {
	if t[start+1].Value == "turn" {
		return "turn_end"
	}
	return t[start+1].Value + "_turn_end"
}

func parseEffectDuration(t []syntax.Token, start int) (int, bool) {
	if start >= len(t) || t[start].Value != "until" {
		return start, false
	}
	i := start + 1
	if i < len(t) && (t[i].Value == "own" || t[i].Value == "oppo") {
		i++
	}
	return i + 2, i+2 == len(t) && t[i].Value == "turn" && t[i+1].Value == "ends"
}
