package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// S-75：`where not <词条>` 取反单个筛选词条（"非侵蚀者随从"）。
func TestNegatedFilterCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			damage field.followers 2 where not trait encroacher;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	damage, ok := pack.Cards[0].Abilities[0].Body[0].(ir.TargetEffect)
	if !ok || damage.Predicate == nil {
		t.Fatalf("damage did not keep its predicate: %#v", pack.Cards[0].Abilities[0].Body[0])
	}
	not, ok := damage.Predicate.(ir.NotPredicate)
	if !ok || not.Kind != "not" {
		t.Fatalf("negation was not preserved: %#v", damage.Predicate)
	}
	inner, ok := not.Term.(ir.FieldPredicate)
	if !ok || inner.Kind != "has_trait" || inner.Trait != "encroacher" {
		t.Fatalf("negated term = %#v", not.Term)
	}
	// 原有的 `not attacked this turn` 特例不受影响。
	attack, ds := compile(t, validCard(`
		fanfare {
			damage field.followers 1 where not attacked this turn;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	predicate := attack.Cards[0].Abilities[0].Body[0].(ir.TargetEffect).Predicate
	if inner, ok := predicate.(ir.FieldPredicate); !ok || inner.Kind != "not_attacked_this_turn" {
		t.Fatalf("attacked-this-turn negation regressed: %#v", predicate)
	}
	if _, ds := compile(t, validCard(`fanfare { damage field.followers 1 where not life == 3; }`)); len(ds) == 0 {
		t.Fatal("negation accepted an unsupported term")
	}
}
