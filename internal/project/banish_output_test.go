package project

import (
	"testing"

	"wbo/internal/ir"
)

// S-66：`banish` 输出本次实际消失的实例，可用 `count(banished)` 读取数量。
func TestBanishOutputCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			banish own.deck where cost == 1 or cost == 3;
			damage oppo.field.followers count(banished) distributed;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	banish, ok := body[0].(ir.TargetEffect)
	if !ok || banish.Kind != "banish" || banish.Output != "banished" {
		t.Fatalf("banish output lost: %#v", body[0])
	}
	damage, ok := body[1].(ir.TargetEffect)
	if !ok || damage.Distribution != "field_entry_order" {
		t.Fatalf("distributed damage lost: %#v", body[1])
	}
	count, ok := damage.AmountExpr.(*ir.CountExpr)
	if !ok {
		t.Fatalf("count(banished) did not compile: %#v", damage.AmountExpr)
	}
	if ref, ok := count.Source.(ir.BindingRef); !ok || ref.Name != "banished" {
		t.Fatalf("count did not read the banish output: %#v", count.Source)
	}
}
