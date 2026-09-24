package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// S-47：筛选器支持"本回合没有进行过攻击"，用于土之法则·伽莱翁这类"未攻击过的进化前随从"。
// 这条也顺带防住 filterIR 的索引回归（漏掉 `j++` 会让编译卡包时死循环）。
func TestAttackedThisTurnFilterCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			random ally from own.field.followers where form unevolved and not attacked this turn;
			evolve ally silent;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	choose, ok := pack.Cards[0].Abilities[0].Body[0].(ir.SelectionEffect)
	if !ok {
		t.Fatalf("selection did not compile: %#v", pack.Cards[0].Abilities[0].Body[0])
	}
	filter, ok := choose.Source.(ir.FilterRef)
	if !ok {
		t.Fatalf("selection lost its filter: %#v", choose.Source)
	}
	and, ok := filter.Predicate.(ir.AndPredicate)
	if !ok || len(and.Terms) != 2 {
		t.Fatalf("filter did not keep both terms: %#v", filter.Predicate)
	}
	second, ok := and.Terms[1].(ir.FieldPredicate)
	if !ok || second.Kind != "not_attacked_this_turn" {
		t.Fatalf("second term = %#v", and.Terms[1])
	}
}

func TestAttackedThisTurnFilterAcceptsBothPolarities(t *testing.T) {
	if _, ds := compile(t, validCard(`
		fanfare {
			random ally from own.field.followers where attacked this turn;
			damage ally 1;
		}`)); len(ds) != 0 {
		t.Fatal(ds)
	}
	if _, ds := compile(t, validCard(`
		fanfare {
			random ally from own.field.followers where not cost;
			damage ally 1;
		}`)); len(ds) == 0 {
		t.Fatal("filter accepted an unsupported `not` term")
	}
}
