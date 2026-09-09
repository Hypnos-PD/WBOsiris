package runner

import (
	"testing"

	"wbo/internal/ir"
)

func TestSetDamageReductionEffect(t *testing.T) {
	g := &game{}
	first := &instance{id: "first", zone: "field", damageReduction: 1, life: 10, abilities: map[string]bool{}}
	second := &instance{id: "second", zone: "field", damageReduction: 4, life: 10, abilities: map[string]bool{}}
	g.own.field = []*instance{first, second}
	g.instances = map[string]*instance{"first": first, "second": second}
	g.execTargetEffect(ir.TargetEffect{Kind: "set_damage_reduction", Target: ir.BindingRef{Kind: "binding", Name: "target"}, Amount: 3}, first, frame{"target": bindEntities(second)})
	if first.damageReduction != 1 {
		t.Fatalf("source damage reduction = %d, want 1", first.damageReduction)
	}
	if second.damageReduction != 3 {
		t.Fatalf("target damage reduction = %d, want 3", second.damageReduction)
	}
	if got := g.damageInstance(second, 5); got != 2 || second.life != 8 {
		t.Fatalf("damage after reduction = %d, life=%d; want 2/8", got, second.life)
	}
	g.execTargetEffect(ir.TargetEffect{Kind: "set_damage_reduction", Target: ir.BindingRef{Kind: "binding", Name: "target"}, Amount: 0}, first, frame{"target": bindEntities(second)})
	if second.damageReduction != 0 {
		t.Fatalf("target damage reduction reset = %d, want 0", second.damageReduction)
	}
	if got := g.damageInstance(second, 3); got != 3 || second.life != 5 {
		t.Fatalf("damage after reset = %d, life=%d; want 3/5", got, second.life)
	}
}
