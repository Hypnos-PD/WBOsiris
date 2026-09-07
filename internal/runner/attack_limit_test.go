package runner

import (
	"testing"

	"wbo/internal/ir"
)

func TestSetAttackLimitEffect(t *testing.T) {
	g := &game{}
	i := &instance{id: "f", zone: "field", attackLimitValue: 1, abilities: map[string]bool{}}
	g.own.field = []*instance{i}
	g.instances = map[string]*instance{i.id: i}
	g.execTargetEffect(ir.TargetEffect{Kind: "set_attack_limit", Target: ir.SelfRef{Kind: "self"}, Amount: 2}, i, frame{})
	if got := attackLimit(i); got != 2 {
		t.Fatalf("attack limit = %d, want 2", got)
	}
}
