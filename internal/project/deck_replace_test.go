package project

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileDeckReplaceAndLeaderMaxLife(t *testing.T) {
	for _, cardType := range []string{"follower", "spell"} {
		body := `replace oppo.deck with shuffled 3 card 12345678, 1 card 12345678; set maxlife own.leader 30;`
		source := validCard("fanfare {" + body + "}")
		if cardType == "spell" {
			source = strings.Replace(validCard(body), "type follower; cost 1; stats 1/1;", "type spell; cost 1;", 1)
		}
		f, ds := syntax.Parse("12345678.wbo", []byte(source))
		if len(ds) != 0 {
			t.Fatal(ds)
		}
		formatted := syntax.Format(f)
		f, ds = syntax.Parse("12345678.wbo", formatted)
		if len(ds) != 0 || !bytes.Equal(formatted, syntax.Format(f)) {
			t.Fatal("unstable recipe formatting")
		}
		root := t.TempDir()
		path := filepath.Join(root, "12345", "12345678.wbo")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
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
		effects := decoded.Cards[0].PlayEffects
		if cardType == "follower" {
			effects = decoded.Cards[0].Abilities[0].Body
		}
		recipe := effects[0].(ir.DeckReplaceEffect)
		life := effects[1].(ir.LeaderMaxLifeEffect)
		if recipe.Owner != "oppo" || len(recipe.Cards) != 2 || recipe.Cards[0].Count != 3 || life.Side != "own" || life.Amount != 30 {
			t.Fatal(recipe, life)
		}
	}
}

func TestRejectMalformedDeckReplaceAndLeaderMaxLife(t *testing.T) {
	for _, op := range []string{
		"replace own.hand with shuffled 1 card 12345678;", "replace all.deck with shuffled 1 card 12345678;",
		"replace own.deck with 1 card 12345678;", "replace own.deck with shuffled;",
		"replace own.deck with shuffled 0 card 12345678;", "replace own.deck with shuffled 65535 card 12345678, 1 card 12345678;",
		"replace own.deck with shuffled 1 card 12345678,;", "replace own.deck with shuffled 1 card 12345678 1 card 12345678;",
		"replace own.deck with shuffled 1 card 12345678 {}", "replace own.deck with shuffled 1 card 12345678",
		"set maxlife self 1;", "set maxlife own.leader 0;", "set maxlife own.leader 65536;",
		"set maxlife all.leaders 1;", "set maxlife own.leader 1 extra;", "set maxlife own.leader count(own.hand);",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare {"+op+"}")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid operation", op)
		}
	}
	f, ds := syntax.Parse("12345678.wbo", []byte(validCard("replace own.deck with shuffled 1 card 12345678;")))
	if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
		t.Fatal("untimed follower deck replacement accepted")
	}
}
