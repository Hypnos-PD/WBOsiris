package project

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileHistorySummonAndBaseExtrema(t *testing.T) {
	source := validCard(`lastwords {
summon random 1 from own.destroyed.amulets highest base.cost;
summon random 2 from oppo.destroyed.followers this turn where keyword ward lowest base.attack;
buff summoned +1/+1;
random target from own.hand highest base.cost;
choose target from own.field.followers lowest base.life;
}`)
	f, ds := syntax.Parse("12345678.wbo", []byte(source))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	formatted := syntax.Format(f)
	f, ds = syntax.Parse("12345678.wbo", formatted)
	if len(ds) != 0 || !bytes.Equal(formatted, syntax.Format(f)) {
		t.Fatal("unstable history summon formatting")
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
	loaded := LoadWithRoot([]string{path}, true, root)
	pack, _, err := BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err, loaded.Diagnostics)
	}
	data, err := ir.EncodeCardPack(*pack)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ir.DecodeCardPack(data)
	if err != nil {
		t.Fatal(err)
	}
	body := decoded.Cards[0].Abilities[0].Body
	a, b := body[0].(ir.HistorySummonEffect), body[1].(ir.HistorySummonEffect)
	if a.Count != 1 || a.Extremum.Field != "base_cost" || a.Source.(ir.ZoneRef).Member != "amulet" || b.Count != 2 || b.Extremum.Field != "base_attack" || b.Source.(ir.FilterRef).Source.(ir.HistoryRef).Window != "this_turn" {
		t.Fatal(a, b)
	}
	if body[3].(ir.SelectionEffect).Extremum.Field != "base_cost" || body[4].(ir.SelectionEffect).Extremum.Field != "base_life" {
		t.Fatal("base extrema lost")
	}
}

func TestRejectMalformedHistorySummons(t *testing.T) {
	for _, operation := range []string{
		"summon random 0 from own.destroyed;", "summon random 65536 from own.destroyed;",
		"summon random 1 from own.hand;", "summon random 1 from own.graveyard;",
		"summon random 1 from own.destroyed.spells;", "summon random 1 from old;",
		"summon random 1 from own.destroyed other;", "summon random 1 from own.destroyed highest base;",
		"summon random 1 from own.destroyed highest base.mana;", "summon random 1 from own.destroyed highest cost where type amulet;",
		"summon random 1 from own.destroyed last turn;", "summon random 1 from own.destroyed where;",
		"summon random from own.destroyed;", "summon random 1 from own.destroyed count 2;",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+operation+" }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted malformed history summon", operation)
		}
	}
}
