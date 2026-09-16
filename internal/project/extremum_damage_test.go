package project

import (
	"testing"

	"wbo/internal/ir"
)

// `damage 集合 数值 highest|lowest [base.] attack|life|cost`：只打击极值目标；
// `own.crests` 读取本方纹章数量。
func TestExtremumDamageAndCrestScalar(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			damage field.followers 5 highest life;
			if overflow {
				damage all.leaders 3 highest life;
			}
			damage oppo.field.followers own.crests distributed overflow oppo.leader;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	first := body[0].(ir.TargetEffect)
	if first.Extremum == nil || first.Extremum.Direction != "highest" || first.Extremum.Field != "life" {
		t.Fatalf("follower extremum not preserved: %#v", first.Extremum)
	}
	leaders := body[1].(ir.IfEffect).Then[0].(ir.TargetEffect)
	if _, ok := leaders.Target.(ir.LeaderSetRef); !ok || leaders.Extremum == nil {
		t.Fatalf("leader extremum not preserved: %#v", leaders)
	}
	distributed := body[2].(ir.TargetEffect)
	if distributed.Distribution != "field_entry_order" || distributed.AmountExpr == nil {
		t.Fatalf("distributed crest damage not preserved: %#v", distributed)
	}
	scalar, ok := distributed.AmountExpr.(*ir.Scalar)
	if !ok || scalar.Field != "crests" || scalar.Side != "own" {
		t.Fatalf("crest scalar not preserved: %#v", distributed.AmountExpr)
	}
}

func TestExtremumDamageRejectsBadShapes(t *testing.T) {
	for _, line := range []string{
		"damage field.followers 5 highest;",
		"damage field.followers 5 highest keyword;",
		"buff field.followers +1/+1 highest life;",
	} {
		if _, ds := compile(t, validCard("fanfare { "+line+" }")); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
