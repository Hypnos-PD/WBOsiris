package project

import (
	"testing"

	"wbo/internal/ir"
)

// S-59：`add N card C to deck` 把新卡插入牌组（随机位置），与加入手牌共用 `added` 输出。
func TestAddCardToDeckCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`fanfare { add 1 card 12345678 to deck; }`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	effect, ok := pack.Cards[0].Abilities[0].Body[0].(ir.CardEffect)
	if !ok || effect.Kind != "add_card" || effect.Destination != "deck" || effect.Output != "added" {
		t.Fatalf("add to deck did not compile: %#v", pack.Cards[0].Abilities[0].Body[0])
	}
	if _, ds := compile(t, validCard(`fanfare { add 1 card 12345678 to graveyard; }`)); len(ds) == 0 {
		t.Fatal("add accepted an unknown destination")
	}
}
