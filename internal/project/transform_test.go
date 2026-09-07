package project

import (
	"os"
	"path/filepath"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestFilteredTransformCompilesAndRoundTrips(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`fanfare {
    transform own.hand into card 12345678 where class forestcraft and cost <= 2;
    choose targets from oppo.field.followers count 2;
    transform targets into card 12345678;
    transform own.deck into card 12345678 preserving materials where type amulet;
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
	first := body[0].(ir.CardEffect)
	filter := first.Target.(ir.FilterRef)
	terms := filter.Predicate.(ir.AndPredicate).Terms
	if first.CardID != 12345678 || !first.PreserveInstanceID || !first.PreserveMaterials || filter.Source.(ir.ZoneRef).Zone != "hand" || terms[0].(ir.FieldPredicate).Class != "forestcraft" || terms[1].(ir.FieldPredicate).Value != 2 {
		t.Fatal(first)
	}
	if body[2].(ir.CardEffect).Target.(ir.BindingRef).Name != "targets" || body[3].(ir.CardEffect).Target.(ir.FilterRef).Source.(ir.ZoneRef).Zone != "deck" {
		t.Fatal(body)
	}
}

func TestTransformRejectsInvalidTargetsAndModifiers(t *testing.T) {
	for _, body := range []string{
		`transform missing into card 12345678;`,
		`transform own.leader into card 12345678;`,
		`transform all.leaders into card 12345678;`,
		`transform own.destroyed into card 12345678;`,
		`transform own.graveyard into card 12345678;`,
		`transform own.banished into card 12345678;`,
		`transform own.hand into card 12345678 where cost <=;`,
		`transform own.hand into card 12345678 where cost <= 2 and;`,
		`transform own.hand where cost <= 2 into card 12345678;`,
		`transform own.hand into card 12345678 preserving;`,
		`transform own.hand into card 12345678 where cost <= 2 preserving materials;`,
		`choose target from oppo.field.followers or oppo.leader; transform target into card 12345678;`,
	} {
		file, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+body+" }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
			t.Fatal("accepted invalid transformation", body)
		}
	}
}
