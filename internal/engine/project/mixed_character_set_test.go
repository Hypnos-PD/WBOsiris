package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// S-19/S-69：`field.followers other or leaders` = 双方战场的其他随从 + 双方主战者。
func TestMixedCharacterSetCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		when own turn ends {
			random victim from field.followers other or leaders;
			damage victim 7;
		}
		fanfare {
			random victim from oppo.field.followers or oppo.leader;
			damage victim 2;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	both, ok := pack.Cards[0].Abilities[0].Body[0].(ir.SelectionEffect)
	if !ok {
		t.Fatalf("mixed selection did not compile: %#v", pack.Cards[0].Abilities[0].Body[0])
	}
	characters, ok := both.Source.(ir.CharacterSetRef)
	if !ok || characters.Side != "" || !characters.ExcludeSelf {
		t.Fatalf("both-sides character set lost: %#v", both.Source)
	}
	oneSide, ok := pack.Cards[0].Abilities[1].Body[0].(ir.SelectionEffect)
	if !ok {
		t.Fatalf("single-side mixed selection did not compile: %#v", pack.Cards[0].Abilities[1].Body[0])
	}
	if characters, ok := oneSide.Source.(ir.CharacterSetRef); !ok || characters.Side != "oppo" || characters.ExcludeSelf {
		t.Fatalf("single-side character set lost: %#v", oneSide.Source)
	}
	// 两侧必须同方；双方混合只允许 `leaders` 这种写法。
	for _, invalid := range []string{
		`fanfare { random victim from own.field.followers or oppo.leader; }`,
		`fanfare { random victim from field.followers or leader; }`,
	} {
		if _, ds := compile(t, validCard(invalid)); len(ds) == 0 {
			t.Fatalf("accepted invalid mixed set: %s", invalid)
		}
	}
}
