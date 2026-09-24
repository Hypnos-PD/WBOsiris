package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// S-92：`add random N copies from <集合> to hand|deck` 支持对手手牌/牌组等任意集合。
func TestCopyRandomFromOpponentCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			add random 2 copies from oppo.hand to hand;
			reduce cost added 1 minimum 0;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	copyEffect, ok := body[0].(ir.CopyRandomEffect)
	if !ok {
		t.Fatalf("copy_random did not compile: %#v", body[0])
	}
	if copyEffect.Kind != "copy_random" || copyEffect.Owner != "own" || copyEffect.Count != 2 ||
		copyEffect.Destination != "hand" || copyEffect.Output != "added" {
		t.Fatalf("copy_random lost fields: %#v", copyEffect)
	}
	zone, ok := copyEffect.Source.(ir.ZoneRef)
	if !ok || zone.Side != "oppo" || zone.Zone != "hand" {
		t.Fatalf("copy_random lost its source: %#v", copyEffect.Source)
	}
	reduce, ok := body[1].(ir.AdjustEffect)
	if !ok || reduce.Field != "cost" || reduce.Delta != -1 || reduce.Minimum != 0 {
		t.Fatalf("added cost reduction lost: %#v", body[1])
	}
	for _, invalid := range []string{
		`fanfare { add random 0 copies from oppo.hand to hand; }`,
		`fanfare { add random 1 copies from oppo.hand to graveyard; }`,
	} {
		if _, ds := compile(t, validCard(invalid)); len(ds) == 0 {
			t.Fatalf("accepted invalid copy_random: %s", invalid)
		}
	}
}

// S-93：`transform … into random card from <集合>` 变身为随机一张卡的复制。
func TestTransformRandomCopyCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			require target from own.hand;
			transform target into random card from oppo.deck;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	transform, ok := body[1].(ir.CardEffect)
	if !ok || transform.Kind != "transform" || transform.CopySource == nil || transform.CardID != 0 {
		t.Fatalf("transform copy source lost: %#v", body[1])
	}
	if !transform.PreserveInstanceID || !transform.PreserveMaterials {
		t.Fatalf("transform preservation lost: %#v", transform)
	}
	zone, ok := transform.CopySource.(ir.ZoneRef)
	if !ok || zone.Side != "oppo" || zone.Zone != "deck" {
		t.Fatalf("transform copy source is wrong: %#v", transform.CopySource)
	}
}
