package project

import (
	"os"
	"path/filepath"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestMixedTargetSelectionCompiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`fanfare { choose target from oppo.field.followers or oppo.leader; damage target 5; }
evolve { random target from own.field.followers or own.leader count 2; damage target 1; }`)
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
	for n, ability := range decoded.Cards[0].Abilities {
		selection := ability.Body[0].(ir.SelectionEffect)
		ref := selection.Source.(ir.CharacterSetRef)
		want := "oppo"
		if n == 1 {
			want = "own"
		}
		if ref.Kind != "characters" || ref.Side != want {
			t.Fatal(ref)
		}
	}
}

func TestMixedTargetSelectionRejectsAmbiguousSources(t *testing.T) {
	for _, source := range []string{
		`oppo.field.followers or own.leader`,
		`oppo.hand.followers or oppo.leader`,
		`oppo.field.amulets or oppo.leader`,
		`oppo.field.followers or oppo.leader highest cost`,
		`oppo.field.followers or oppo.leader where cost >= 1`,
		`oppo.field.followers or oppo.leader other`,
		`oppo.field.followers or oppo.leader or own.leader`,
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { choose target from "+source+"; damage target 5; }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid mixed target", source)
		}
	}
}

func TestMixedTargetBindingRejectsCardOnlyOperations(t *testing.T) {
	for _, operation := range []string{
		"destroy target;", "heal target 5;", "buff target 1/1;", "set life target 2;",
		"damage target 5 where cost >= 1;", "summon copies of target;",
		"add ward to target;", "repeat 2 { destroy target; }",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { choose target from oppo.field.followers or oppo.leader; "+operation+" }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted unsupported mixed operation", operation)
		}
	}
	for _, operation := range []string{
		"damage target count(target);", "repeat count(target) { damage target 1; }",
		"choose target from oppo.field.followers; destroy target;",
		"repeat 1 { choose target from oppo.field.followers; destroy target; } damage target 5;",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { choose target from oppo.field.followers or oppo.leader; "+operation+" }")))
		if len(ds) != 0 || hasErrors(ValidateFile(f)) {
			t.Fatal("rejected supported mixed operation", operation, ValidateFile(f))
		}
	}
}

func TestMixedTargetBindingTracksConditionalAssignments(t *testing.T) {
	for _, branch := range []string{
		"if own.pp >= 0 { choose target from oppo.field.followers or oppo.leader; }",
		"mode { option 1 { choose target from oppo.field.followers or oppo.leader; } option 2 { draw 1; } }",
		"necromancy 1 { choose target from oppo.field.followers or oppo.leader; }",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { choose target from oppo.field.followers; "+branch+" destroy target; }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("lost mixed type across a branch", branch)
		}
	}
	f, ds := syntax.Parse("12345678.wbo", []byte(validCard("superevolve extends evolve { destroy target; } evolve { choose target from oppo.field.followers or oppo.leader; }")))
	if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
		t.Fatal("lost mixed type across evolution continuation")
	}
}
