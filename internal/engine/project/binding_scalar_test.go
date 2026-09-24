package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// S-50：`<绑定>.attack|life|cost` 作为数值（"X 为选择的随从的攻击力"）。
func TestBindingScalarCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			require target from own.hand.followers where trait artifact;
			damage oppo.field.followers target.attack;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	damage, ok := body[1].(ir.TargetEffect)
	if !ok || damage.Kind != "damage" {
		t.Fatalf("damage did not compile: %#v", body[1])
	}
	scalar, ok := damage.AmountExpr.(*ir.Scalar)
	if !ok || scalar.Kind != "binding_scalar" || scalar.Side != "target" || scalar.Field != "attack" {
		t.Fatalf("amount = %#v", damage.AmountExpr)
	}
	if _, ds := compile(t, validCard(`fanfare { damage oppo.field.followers target.maxpp; }`)); len(ds) == 0 {
		t.Fatal("binding scalar accepted an unsupported field")
	}
}

// S-53 / S-54：`raise countdown <集合> N` 与"让指定随从可以攻击两次"。
func TestRaiseCountdownAndTargetedAttackLimit(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			raise countdown own.crests 1;
			require target from own.field.followers;
			set_attack_limit target 2;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	raise, ok := body[0].(ir.AdjustEffect)
	if !ok || raise.Kind != "adjust_entity_field" || raise.Field != "countdown" || raise.Delta != 1 {
		t.Fatalf("raise countdown did not compile: %#v", body[0])
	}
	limit, ok := body[2].(ir.TargetEffect)
	if !ok || limit.Kind != "set_attack_limit" || limit.Amount != 2 {
		t.Fatalf("set_attack_limit did not compile: %#v", body[2])
	}
	if ref, ok := limit.Target.(ir.BindingRef); !ok || ref.Name != "target" {
		t.Fatalf("set_attack_limit lost its target: %#v", limit.Target)
	}
	if _, ds := compile(t, validCard(`fanfare { raise countdown own.crests; }`)); len(ds) == 0 {
		t.Fatal("raise countdown accepted a missing amount")
	}
}
