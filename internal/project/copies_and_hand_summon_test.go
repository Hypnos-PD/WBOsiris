package project

import (
	"testing"

	"wbo/internal/ir"
)

// S-45 `add copies of 集合 to hand`：复制集合里的同名卡加入手牌，输出 `added`。
func TestAddCopiesCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			summon 1 card 12345678;
			add copies of summoned to hand;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	effect, ok := body[1].(ir.CardEffect)
	if !ok || effect.Kind != "add_copies" || effect.Destination != "hand" || effect.Output != "added" {
		t.Fatalf("add copies did not compile: %#v", body[1])
	}
	if ref, ok := effect.Target.(ir.BindingRef); !ok || ref.Name != "summoned" {
		t.Fatalf("add copies lost its target set: %#v", effect.Target)
	}
	if _, ds := compile(t, validCard(`fanfare { add copies of summoned to battlefield; }`)); len(ds) == 0 {
		t.Fatal("add copies accepted an unknown destination")
	}
}

// S-46 `summon target`：把已经存在于手牌的对象直接放到战场。
func TestSummonFromHandCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		enhance 8 {
			require target from own.hand.followers;
			summon target;
			return self to hand;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	effect, ok := body[1].(ir.CardEffect)
	if !ok || effect.Kind != "summon_from_hand" || effect.Output != "summoned" {
		t.Fatalf("summon target did not compile: %#v", body[1])
	}
	if _, ds := compile(t, validCard(`fanfare { summon missing; }`)); len(ds) == 0 {
		t.Fatal("summon accepted an undefined binding")
	}
}
