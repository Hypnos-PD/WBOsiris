package project

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"wbo/internal/engine/ir"
	"wbo/internal/engine/syntax"
)

func TestCompileHistorySummonAndBaseExtrema(t *testing.T) {
	source := validCard(`lastwords {
summon random 1 from own.destroyed.amulets highest base.cost;
summon random 2 from oppo.destroyed.followers this turn where keyword ward lowest base.attack;
summon random 2 from own.destroyed.amulets where base.cost <= 2 and lastwords distinct names;
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
	a, b, c := body[0].(ir.HistorySummonEffect), body[1].(ir.HistorySummonEffect), body[2].(ir.HistorySummonEffect)
	if a.Count != 1 || a.Extremum.Field != "base_cost" || a.Source.(ir.ZoneRef).Member != "amulet" || b.Count != 2 || b.Extremum.Field != "base_attack" || b.Source.(ir.FilterRef).Source.(ir.HistoryRef).Window != "this_turn" {
		t.Fatal(a, b)
	}
	if c.Count != 2 || !c.DistinctNames {
		t.Fatal("distinct names lost", c)
	}
	if body[4].(ir.SelectionEffect).Extremum.Field != "base_cost" || body[5].(ir.SelectionEffect).Extremum.Field != "base_life" {
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
		"summon random 1 from own.destroyed distinct names where base.cost <= 2;",
		"summon random 1 from own.destroyed distinct names distinct names;",
		"summon random 1 from own.destroyed names;",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+operation+" }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted malformed history summon", operation)
		}
	}
}

// S-64：`add random N copies from <破坏历史> … to hand|deck` 按记录复制同名卡。
func TestHistoryCopyCompilesAndKeepsDestination(t *testing.T) {
	body := compiledFanfare(t, `fanfare {
		add random 2 copies from own.destroyed.followers where trait artifact distinct names to hand;
		add random 1 copies from own.destroyed.followers highest base.cost to deck;
	}`)
	if len(body) != 2 {
		t.Fatalf("expected two effects, got %d", len(body))
	}
	toHand, ok := body[0].(ir.HistorySummonEffect)
	if !ok || toHand.Destination != "hand" || toHand.Output != "added" || toHand.Count != 2 || !toHand.DistinctNames {
		t.Fatalf("history copy to hand lost its shape: %#v", body[0])
	}
	toDeck, ok := body[1].(ir.HistorySummonEffect)
	if !ok || toDeck.Destination != "deck" || toDeck.Extremum == nil || toDeck.Extremum.Field != "base_cost" {
		t.Fatalf("history copy to deck lost its shape: %#v", body[1])
	}
}

func TestHistoryCopyRejectsBadShapes(t *testing.T) {
	for _, effect := range []string{
		"add random 1 copies from own.destroyed.followers;",
		"add random 0 copies from own.destroyed.followers to hand;",
		"add random 1 copies from own.hand to hand;",
		"add random 1 copies from own.destroyed.followers to graveyard;",
	} {
		if _, ds := compile(t, validCard(effect)); len(ds) == 0 {
			t.Fatalf("%q must not compile", effect)
		}
	}
}
