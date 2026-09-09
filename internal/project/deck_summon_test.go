package project

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileDeckSummonAndAmuletEntry(t *testing.T) {
	source := validCard(`fanfare {
summon random 3 from own.deck.amulets where cost <= 3 distinct names;
summon random 2 from own.deck.followers;
add storm to summoned;
}
when own amulet summoned once per own turn { heal own.leader 1; }
when oppo amulet summoned while self in hand where cost <= 3 { reduce cost self 1 minimum 0; }`)
	f, ds := syntax.Parse("12345678.wbo", []byte(source))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	formatted := syntax.Format(f)
	f, ds = syntax.Parse("12345678.wbo", formatted)
	if len(ds) != 0 || !bytes.Equal(formatted, syntax.Format(f)) {
		t.Fatal("unstable deck summon format")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	if err := os.WriteFile(path, formatted, 0600); err != nil {
		t.Fatal(err)
	}
	l := LoadWithRoot([]string{path}, true, root)
	pack, _, err := BuildRuntimePacks(l)
	if err != nil {
		t.Fatal(err, l.Diagnostics)
	}
	data, err := ir.EncodeCardPack(*pack)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ir.DecodeCardPack(data)
	if err != nil {
		t.Fatal(err)
	}
	a := decoded.Cards[0].Abilities
	first, second := a[0].Body[0].(ir.DeckSummonEffect), a[0].Body[1].(ir.DeckSummonEffect)
	if first.Count != 3 || !first.DistinctNames || second.DistinctNames || second.Count != 2 || first.Source.(ir.FilterRef).Source.(ir.ZoneRef).Member != "amulet" {
		t.Fatal(first, second)
	}
	for _, ability := range a[1:] {
		tr := ability.Trigger.(ir.EventTrigger)
		if tr.Event != "amulet_summoned" || tr.SubjectType != "amulet" {
			t.Fatal(tr)
		}
	}
}

func TestRejectMalformedDeckSummon(t *testing.T) {
	for _, operation := range []string{
		"summon random 1 from own.deck;", "summon random 1 from own.deck.spells;", "summon random 1 from oppo.deck.amulets;",
		"summon random 0 from own.deck.amulets;", "summon random 65536 from own.deck.amulets;",
		"summon random 1 from own.deck.amulets distinct;", "summon random 1 from own.deck.amulets names;",
		"summon random 1 from own.deck.amulets distinct names where cost <= 3;",
		"summon random 1 from own.deck.amulets highest cost;", "summon random 1 from own.deck.amulets this turn;",
		"summon random 1 from own.destroyed.amulets distinct names;",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+operation+" }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted malformed deck summon", operation)
		}
	}
}
