package project

import (
	"testing"

	"wbo/internal/ir"
)

// `fused.cost` / `fused.distinct` 也可以直接当数值用（造成 X 点伤害，X 为融合种类）。
func TestFusedScalarAsAmount(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fusion material from own.hand where trait loot {
		}
		fanfare {
			damage oppo.field.followers fused.distinct;
			damage oppo.leader fused.cost;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	distinct := body[0].(ir.TargetEffect)
	scalar, ok := distinct.AmountExpr.(*ir.Scalar)
	if !ok || scalar.Kind != "fusion_material_scalar" || scalar.Field != "distinct" {
		t.Fatalf("fused.distinct amount not preserved: %#v", distinct.AmountExpr)
	}
	total := body[1].(ir.TargetEffect)
	cost, ok := total.AmountExpr.(*ir.Scalar)
	if !ok || cost.Kind != "fusion_material_scalar" || cost.Field != "cost" {
		t.Fatalf("fused.cost amount not preserved: %#v", total.AmountExpr)
	}
}

func TestFusedAmountRejectsUnknownField(t *testing.T) {
	for _, line := range []string{
		"damage oppo.field.followers fused.types;",
		"damage oppo.field.followers fused;",
	} {
		if _, ds := compile(t, validCard("fanfare { "+line+" }")); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
