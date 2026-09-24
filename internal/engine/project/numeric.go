package project

import (
	"wbo/internal/engine/ir"
	"wbo/internal/engine/syntax"
)

func numericIR(t []syntax.Token, i int) (int, ir.NumericExpr) {
	value, expr := numericOperandIR(t, i)
	end, ok := parseEffectOperand(t, i)
	for ok && end < len(t) && t[end].Value == "-" {
		nextEnd, nextOK := parseEffectOperand(t, end+1)
		if !nextOK {
			break
		}
		_, right := numericOperandIR(t, end+1)
		if expr == nil || right == nil {
			break
		}
		expr = &ir.DifferenceExpr{Kind: "difference", Left: expr, Right: right}
		end, ok = nextEnd, true
	}
	return value, expr
}

func numericOperandIR(t []syntax.Token, i int) (int, ir.NumericExpr) {
	if counterRef(t, i) {
		return 0, &ir.Scalar{Kind: "self_counter", Field: t[i+4].Value}
	}
	if t[i].Kind == syntax.Integer {
		return intToken(t[i]), nil
	}
	if t[i].Value == "count" || t[i].Value == "sum" {
		source := valueRefIR(t, i+2)
		next, _ := parseCountSource(t, i+2)
		if next < len(t) && t[next].Value == "other" {
			value, after := otherExclusion(t, next)
			source = ir.ExcludeRef{Kind: "exclude", Source: source, Value: value}
			next = after
		}
		if t[next].Value == "where" {
			var predicate ir.Predicate
			predicate, next = filterIR(t, next)
			source = ir.FilterRef{Kind: "filter", Source: source, Predicate: predicate}
		}
		if t[i].Value == "sum" {
			field := t[next+1].Value
			end := next + 2
			if field == "base" {
				field = "base_" + t[next+3].Value
				end = next + 4
			}
			// `sum(集合, base.cost) highest 3`：取最高的 N 张再求和。
			limit, direction := 0, ""
			if end+2 < len(t) && t[end].Value == ")" && set("highest", "lowest")[t[end+1].Value] && isUnsigned(t[end+2]) {
				limit = intToken(t[end+2])
				direction = t[end+1].Value
			}
			return 0, &ir.SumExpr{Kind: "sum", Source: source, Field: field, Limit: limit, Direction: direction}
		}
		return 0, &ir.CountExpr{Kind: "count", Source: source}
	}
	if t[i].Value == "self" {
		return 0, &ir.Scalar{Kind: "self_scalar", Field: t[i+2].Value}
	}
	if t[i].Value == "fused" {
		// `fused.cost` / `fused.distinct` 也可以直接当数值用（例如造成 X 点伤害，X 为融合种类）。
		return 0, &ir.Scalar{Kind: "fusion_material_scalar", Field: t[i+2].Value}
	}
	if t[i].Value != "own" && t[i].Value != "oppo" {
		// `<绑定>.attack|life|cost`：读取绑定实例当前的数值（例如"X 为选择的随从的攻击力"）。
		return 0, &ir.Scalar{Kind: "binding_scalar", Side: t[i].Value, Field: t[i+2].Value}
	}
	return 0, &ir.Scalar{Kind: "scalar", Side: t[i].Value, Field: t[i+2].Value}
}

func signedNumericIR(t []syntax.Token, i int) (int, ir.NumericExpr, int) {
	value, expr := numericIR(t, i+1)
	if t[i].Value == "-" {
		value = -value
		if expr != nil {
			expr = &ir.NegateExpr{Kind: "negate", Value: expr}
		}
	}
	end, _ := parseSignedAmount(t, i)
	return value, expr, end
}
