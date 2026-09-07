package project

import (
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func numericIR(t []syntax.Token, i int) (int, ir.NumericExpr) {
	if counterRef(t, i) {
		return 0, &ir.Scalar{Kind: "self_counter", Field: t[i+4].Value}
	}
	if t[i].Kind == syntax.Integer {
		return intToken(t[i]), nil
	}
	if t[i].Value == "count" {
		source := valueRefIR(t, i+2)
		next, _ := parseCountSource(t, i+2)
		if t[next].Value == "where" {
			predicate, _ := filterIR(t, next)
			source = ir.FilterRef{Kind: "filter", Source: source, Predicate: predicate}
		}
		return 0, &ir.CountExpr{Kind: "count", Source: source}
	}
	if t[i].Value == "self" {
		return 0, &ir.Scalar{Kind: "self_scalar", Field: t[i+2].Value}
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
