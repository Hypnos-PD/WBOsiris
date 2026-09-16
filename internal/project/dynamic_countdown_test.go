package project

import (
	"testing"

	"wbo/internal/ir"
)

// `reduce countdown T 数值引用`：增量可以是标量表达式（例如"倒计数 -X，X 为纹章数"）。
func TestDynamicCountdownReduction(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			reduce countdown self own.crests;
			reduce countdown own.field.amulets 2;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	dynamic := pack.Cards[0].Abilities[0].Body[0].(ir.AdjustEffect)
	if dynamic.Kind != "adjust_entity_field" || dynamic.Field != "countdown" || dynamic.Delta != 0 || dynamic.DeltaExpr == nil {
		t.Fatalf("dynamic countdown not preserved: %#v", dynamic)
	}
	negate, ok := dynamic.DeltaExpr.(*ir.NegateExpr)
	if !ok {
		t.Fatalf("dynamic countdown must negate: %#v", dynamic.DeltaExpr)
	}
	scalar, ok := negate.Value.(*ir.Scalar)
	if !ok || scalar.Field != "crests" || scalar.Side != "own" {
		t.Fatalf("crest scalar not preserved: %#v", negate.Value)
	}
	constant := pack.Cards[0].Abilities[0].Body[1].(ir.AdjustEffect)
	if constant.DeltaExpr != nil || constant.Delta != -2 {
		t.Fatalf("constant countdown changed: %#v", constant)
	}
}

func TestDynamicCountdownRejectsBadShapes(t *testing.T) {
	for _, line := range []string{
		"reduce countdown self own.crests 1;",
		"reduce cost self own.crests minimum;",
	} {
		if _, ds := compile(t, validCard("fanfare { "+line+" }")); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
