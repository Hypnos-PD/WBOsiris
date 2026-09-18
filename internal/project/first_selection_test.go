package project

import (
	"testing"

	"wbo/internal/ir"
)

// S-94：`first <绑定> from <集合> [where …] [count N]` 按集合顺序取前 N 个。
func TestFirstSelectionCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			first ally from own.field.followers where class swordcraft;
			set_attack_limit ally 2;
			first copies from own.hand count 3;
			add copies of copies to hand;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	single, ok := body[0].(ir.SelectionEffect)
	if !ok || single.Kind != "first" || single.Policy != "first" || single.Binding != "ally" {
		t.Fatalf("first selection did not compile: %#v", body[0])
	}
	if single.SelectionCount() != 1 {
		t.Fatalf("first selection should default to one candidate: %#v", single)
	}
	limit, ok := body[1].(ir.TargetEffect)
	if !ok || limit.Kind != "set_attack_limit" || limit.Amount != 2 {
		t.Fatalf("attack limit grant lost: %#v", body[1])
	}
	multiple, ok := body[2].(ir.SelectionEffect)
	if !ok || multiple.Kind != "first" || multiple.SelectionCount() != 3 {
		t.Fatalf("first selection count lost: %#v", body[2])
	}
	add, ok := body[3].(ir.CardEffect)
	if !ok || add.Kind != "add_copies" || add.Destination != "hand" {
		t.Fatalf("copy of the first cards lost: %#v", body[3])
	}
	if _, ds := compile(t, validCard(`fanfare { first ally from own.field.followers count 0; }`)); len(ds) == 0 {
		t.Fatal("first selection accepted count 0")
	}
}
