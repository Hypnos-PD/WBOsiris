package project

import (
	"testing"

	"wbo/internal/ir"
)

// S-72：`return ... to deck` 把本次实际返回牌组的实例写入 `returned`，
// `draw count(returned)` 按这个集合绑定的张数抽牌。
func TestReturnedCountDrawCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			return own.hand to deck;
			draw count(returned);
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	ret, ok := body[0].(ir.TargetEffect)
	if !ok || ret.Kind != "return" || ret.Output != "returned" || ret.Destination != "deck" || ret.DeckInsertion != "uniform_random_position" {
		t.Fatalf("return output lost: %#v", body[0])
	}
	draw, ok := body[1].(ir.DrawEffect)
	if !ok {
		t.Fatalf("draw did not compile: %#v", body[1])
	}
	count, ok := draw.CountExpr.(*ir.CountExpr)
	if !ok {
		t.Fatalf("count(returned) did not compile: %#v", draw.CountExpr)
	}
	if ref, ok := count.Source.(ir.BindingRef); !ok || ref.Name != "returned" {
		t.Fatalf("draw count did not read the return output: %#v", count.Source)
	}
	if _, ds := compile(t, validCard(`
		fanfare {
			draw count(returned);
		}`)); len(ds) == 0 {
		t.Fatal("draw count(returned) without a preceding return was accepted")
	}
}
