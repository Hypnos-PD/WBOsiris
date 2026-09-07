package project

import (
	"os"
	"path/filepath"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestDestructionTriggersAndKeywordFiltersRoundTrip(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`counter discarded 0;
when own follower destroyed where keyword ward and cost <= 3 if self.counter.discarded == 0 {
    return destroyed to hand;
    buff self +count(own.destroyed.followers where keyword ward)/+0;
}
when oppo amulet destroyed while self in hand once per turn if own.hand_count <= 5 { draw 1; }
fanfare { choose target from own.field.followers where keyword barrier or keyword ward; heal target 1; }
`)
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
	trigger := decoded.Cards[0].Abilities[0].Trigger.(ir.EventTrigger)
	if trigger.Event != "destroyed" || trigger.SubjectType != "follower" || trigger.Predicate.(ir.AndPredicate).Terms[0].(ir.FieldPredicate).Keyword != "ward" {
		t.Fatal(trigger)
	}
	second := decoded.Cards[0].Abilities[1].Trigger.(ir.EventTrigger)
	if second.Event != "destroyed" || second.SubjectType != "amulet" || second.SourceZone != "hand" || second.OncePerTurn != "any" {
		t.Fatal(second)
	}
}

func TestDestructionTriggerRejectsUnsupportedSubjectsAndKeywords(t *testing.T) {
	for _, body := range []string{
		`when own card destroyed { draw 1; }`,
		`when own spell destroyed { draw 1; }`,
		`when self destroyed { draw 1; }`,
		`when own follower destroyed where keyword { draw 1; }`,
		`when own follower destroyed where keyword lastwords { draw 1; }`,
		`when own follower destroyed where keyword unknown { draw 1; }`,
		`fanfare { return destroyed to hand; }`,
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard(body)))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid destruction listener", body)
		}
	}
}
