package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// S-81：`sum(集合, base.cost) highest N` 取最高的 N 张求和，并支持两侧都是表达式的比较。
func TestTopSumComparisonCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		evolve {
			if sum(own.hand, base.cost) highest 3 > sum(oppo.hand, base.cost) highest 3 {
				draw 1;
			}
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	branch, ok := pack.Cards[0].Abilities[0].Body[0].(ir.IfEffect)
	if !ok {
		t.Fatalf("if did not compile: %#v", pack.Cards[0].Abilities[0].Body[0])
	}
	comparison, ok := branch.Condition.(ir.CompareCondition)
	if !ok || comparison.Op != "gt" || comparison.LeftExpr == nil || comparison.RightExpr == nil {
		t.Fatalf("expression comparison lost: %#v", branch.Condition)
	}
	left, ok := comparison.LeftExpr.(*ir.SumExpr)
	if !ok || left.Field != "base_cost" || left.Limit != 3 || left.Direction != "highest" {
		t.Fatalf("top sum lost: %#v", comparison.LeftExpr)
	}
	if _, ok := comparison.RightExpr.(*ir.SumExpr); !ok {
		t.Fatalf("right sum lost: %#v", comparison.RightExpr)
	}
	// 不带最高/最低的旧写法保持原样。
	plain, ds := compile(t, validCard(`fanfare { heal own.leader sum(own.hand, base.cost); }`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	heal, ok := plain.Cards[0].Abilities[0].Body[0].(ir.TargetEffect)
	if !ok {
		t.Fatalf("heal did not compile: %#v", plain.Cards[0].Abilities[0].Body[0])
	}
	if sum, ok := heal.AmountExpr.(*ir.SumExpr); !ok || sum.Limit != 0 || sum.Direction != "" {
		t.Fatalf("plain sum changed: %#v", heal.AmountExpr)
	}
	for _, invalid := range []string{
		`fanfare { heal own.leader sum(own.hand, base.cost) highest 0; }`,
		`fanfare { heal own.leader sum(own.hand, base.cost) middle 2; }`,
	} {
		if _, ds := compile(t, validCard(invalid)); len(ds) == 0 {
			t.Fatalf("accepted invalid top sum: %s", invalid)
		}
	}
}
