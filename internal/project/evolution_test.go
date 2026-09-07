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

func TestCompileEffectEvolutionAndSelfEvents(t *testing.T) {
	source := validCard(`
		fanfare {
			choose target from own.field.followers other where form unevolved;
			superevolve target silent;
		}
		when self evolved { gain own.maxpp 1; }
		when self super_evolved { draw 1; }
		evolve { draw 2; }
	`)
	file, ds := syntax.Parse("12345678.wbo", []byte(source))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	formatted := syntax.Format(file)
	file, ds = syntax.Parse("12345678.wbo", formatted)
	if len(ds) != 0 || !bytes.Equal(formatted, syntax.Format(file)) {
		t.Fatal("unstable evolution formatting")
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
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	card := pack.Cards[0]
	selection := card.Abilities[0].Body[0].(ir.SelectionEffect)
	filter := selection.Source.(ir.FilterRef)
	_, excludesSelf := filter.Source.(ir.ExcludeRef)
	if filter.Predicate.(ir.FieldPredicate).Form != "unevolved" || !excludesSelf {
		t.Fatal("lost evolution target filter")
	}
	if effect := card.Abilities[0].Body[1].(ir.TargetEffect); effect.Kind != "silent_evolve" || effect.Form != "super_evolved" {
		t.Fatal("lost effect evolution form")
	}
	for _, ability := range card.Abilities[1:3] {
		if trigger := ability.Trigger.(ir.EventTrigger); !trigger.SelfOnly || trigger.Side != "own" || trigger.SubjectType != "follower" {
			t.Fatal("self event lost identity constraint")
		}
	}
	if len(card.ActionPlans) != 2 || card.ActionPlans[1].Action != "superevolve" || card.ActionPlans[1].Steps[0].AbilityID != card.Abilities[3].ID {
		t.Fatal("super evolution did not inherit ordinary keyword ability")
	}
}

func TestRejectMalformedEvolutionSyntax(t *testing.T) {
	for _, effect := range []string{
		`when self evolve { draw 1; }`,
		`when self evolved where trait golem { draw 1; }`,
		`when oppo evolved { draw 1; }`,
		`fanfare { superevolve self; }`,
		`fanfare { superevolve self silent extra; }`,
		`fanfare { choose target from own.field.followers where form unknown; }`,
		`fanfare { choose target from own.field.followers where form evolved == true; }`,
	} {
		file, ds := syntax.Parse("12345678.wbo", []byte(validCard(effect)))
		if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
			t.Fatalf("accepted %s", effect)
		}
	}
	source := strings.Replace(validCard(`when self evolved { draw 1; }`), "type follower; cost 1; stats 1/1;", "type spell; cost 1;", 1)
	file, ds := syntax.Parse("12345678.wbo", []byte(source))
	if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
		t.Fatal("spell accepted a self evolution trigger")
	}
}
