package project

import (
	"testing"

	"wbo/internal/ir"
)

// S-96：`own.played has costs N to M` 与"使用卡牌的原始费用"绑定标量。
func TestPlayedCostsConditionCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		when own card played other {
			if count(field other played where base.cost == played.base.cost) >= 1 {
				draw 1;
			}
		}
		fanfare {
			if own.played has costs 1 to 8 {
				draw 1;
			}
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	listener, ok := body[0].(ir.IfEffect)
	if !ok {
		t.Fatalf("if did not compile: %#v", body[0])
	}
	condition, ok := listener.Condition.(ir.CountCondition)
	if !ok || condition.Op != "ge" || condition.Right != 1 {
		t.Fatalf("count condition lost: %#v", listener.Condition)
	}
	filter, ok := condition.Source.(ir.FilterRef)
	if !ok {
		t.Fatalf("filter lost: %#v", condition.Source)
	}
	exclude, ok := filter.Source.(ir.ExcludeRef)
	if !ok {
		t.Fatalf("played exclusion lost: %#v", filter.Source)
	}
	if binding, ok := exclude.Value.(ir.BindingRef); !ok || binding.Name != "played" {
		t.Fatalf("exclusion should target the played binding: %#v", exclude.Value)
	}
	predicate, ok := filter.Predicate.(ir.FieldPredicate)
	if !ok || predicate.Field != "base_cost" || predicate.Op != "eq" {
		t.Fatalf("base cost comparison lost: %#v", filter.Predicate)
	}
	scalar := predicate.ValueScalar
	if scalar == nil || scalar.Kind != "binding_scalar" || scalar.Side != "played" || scalar.Field != "base_cost" {
		t.Fatalf("played base cost scalar lost: %#v", predicate.ValueScalar)
	}
	fanfare, ok := pack.Cards[0].Abilities[1].Body[0].(ir.IfEffect)
	if !ok {
		t.Fatalf("fanfare if did not compile: %#v", pack.Cards[0].Abilities[1].Body[0])
	}
	played, ok := fanfare.Condition.(ir.PlayedCostsCondition)
	if !ok || played.Side != "own" || played.From != 1 || played.To != 8 {
		t.Fatalf("played costs condition lost: %#v", fanfare.Condition)
	}
	for _, invalid := range []string{
		`fanfare { if own.played has costs 8 to 1 { draw 1; } }`,
		`fanfare { if own.played has costs 1 to 65536 { draw 1; } }`,
	} {
		if _, ds := compile(t, validCard(invalid)); len(ds) == 0 {
			t.Fatalf("accepted invalid played costs condition: %s", invalid)
		}
	}
}
