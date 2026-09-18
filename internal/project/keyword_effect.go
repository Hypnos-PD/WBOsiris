package project

import (
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func keywordEffectIR(base ir.NodeBase, kind string, t []syntax.Token) ir.TargetEffect {
	end := valueRefEnd(t, 3)
	target := valueRefIR(t, 3)
	if end < len(t) && t[end].Value == "other" {
		value, next := otherExclusion(t, end)
		target = ir.ExcludeRef{Kind: "exclude", Source: target, Value: value}
		end = next
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

// otherExclusion 解析 `other [绑定名]`：默认排除来源自身，给出绑定名时排除该绑定的实例。
func otherExclusion(t []syntax.Token, i int) (ir.Ref, int) {
	next := i + 1
	if next < len(t) && t[next].Kind == syntax.Identifier && !otherFollowers[t[next].Value] {
		return ir.BindingRef{Kind: "binding", Name: t[next].Value}, next + 1
	}
	return ir.SelfRef{Kind: "self", ValueType: "entity"}, next
}

// otherFollowers 是 `other` 之后可能出现的关键词，不算绑定名。
var otherFollowers = map[string]bool{
	"where": true, "highest": true, "lowest": true, "count": true, "until": true, "other": true,
	"into": true, "preserving": true, "minimum": true, "to": true, "from": true, "for": true,
}

// removeAbilityIR 解析 remove lastwords from … / remove all abilities from …：
// 目标从 start 开始，可选择 other 与 where，不支持 until。
func removeAbilityIR(base ir.NodeBase, ability string, t []syntax.Token, start int) ir.TargetEffect {
	end := valueRefEnd(t, start)
	target := valueRefIR(t, start)
	if end < len(t) && t[end].Value == "other" {
		value, next := otherExclusion(t, end)
		target = ir.ExcludeRef{Kind: "exclude", Source: target, Value: value}
		end = next
	}
	e := ir.TargetEffect{NodeBase: base, Kind: "remove_ability", Keyword: ability, Target: target}
	if end < len(t) && t[end].Value == "where" {
		e.Predicate, end = filterIR(t, end)
	}
	return e
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
