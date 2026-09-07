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
	if end < len(t) {
		e.Predicate, _ = filterIR(t, end)
	}
	return e
}
