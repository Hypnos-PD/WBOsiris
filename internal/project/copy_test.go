package project

import (
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileCopiesWithCurrentCostAndOutputBindings(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`fanfare {
        choose targets from own.hand where type follower and trait artifact and cost <= 5 count 3;
        summon copies of targets;
        summon copies of summoned where cost != 0;
        buff summoned +1/+1;
    }`)
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
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
	selection := body[0].(ir.SelectionEffect)
	terms := selection.Source.(ir.FilterRef).Predicate.(ir.AndPredicate).Terms
	cost := terms[2].(ir.FieldPredicate)
	if selection.SelectionCount() != 3 || cost.Field != "cost" || cost.Op != "le" || cost.Value != 5 {
		t.Fatal("incorrect copy candidates", selection)
	}
	first, second := body[1].(ir.CardEffect), body[2].(ir.CardEffect)
	if first.Kind != "summon_copies" || first.Target.(ir.BindingRef).Name != "targets" || first.Output != "summoned" || second.Target.(ir.FilterRef).Source.(ir.BindingRef).Name != "summoned" {
		t.Fatal("copy bindings changed", first, second)
	}
}

func TestCopyRejectsMalformedSourceAndFilter(t *testing.T) {
	for _, body := range []string{
		"summon copies of missing;",
		"summon copies self;",
		"summon copies of;",
		"summon copies of own.hand where cost <=;",
		"summon copies of own.hand where cost <= -1;",
		"summon copies of own.hand where cost 5;",
		"summon copies of own.hand where cost <= 5 and;",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+body+" }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid copy", body)
		}
	}
}
