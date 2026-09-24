package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// `self.cost` / `self.attack` / `self.life` 可作为条件左侧；`raise cost T N` 加费；
// `damage 集合 other N` 排除来源自身。
func TestSelfScalarConditionRaiseCostAndDamageOther(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			if self.cost != 2 {
				heal own.leader 3;
			}
			damage field.followers other 3;
			raise cost self 1;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	condition, ok := body[0].(ir.IfEffect).Condition.(ir.CompareCondition)
	if !ok || condition.Left.Kind != "self_scalar" || condition.Left.Field != "cost" || condition.Op != "ne" || condition.Right != 2 {
		t.Fatalf("self scalar condition not preserved: %#v", body[0])
	}
	damage := body[1].(ir.TargetEffect)
	exclude, ok := damage.Target.(ir.ExcludeRef)
	if !ok {
		t.Fatalf("damage other did not exclude self: %#v", damage.Target)
	}
	if _, ok := exclude.Value.(ir.SelfRef); !ok {
		t.Fatalf("damage other must exclude self: %#v", exclude.Value)
	}
	raise := body[2].(ir.AdjustEffect)
	if raise.Kind != "adjust_entity_field" || raise.Field != "cost" || raise.Delta != 1 {
		t.Fatalf("raise cost did not compile: %#v", raise)
	}
}

func TestSelfScalarAndRaiseCostRejectBadShapes(t *testing.T) {
	for _, line := range []string{
		"if self.cost { draw 1; }",
		"if self.keyword != 2 { draw 1; }",
		"raise cost;",
		"raise cost self;",
		"raise countdown self;",
		"raise ward self 1;",
	} {
		if _, ds := compile(t, validCard("fanfare { "+line+" }")); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
