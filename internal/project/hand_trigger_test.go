package project

import (
	"os"
	"path/filepath"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestHandAndFusionTriggersRoundTrip(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`when own follower leaves field while self in hand where trait officer { reduce cost self 1 minimum 0; }
when oppo card fused { draw 1; }
when own follower leaves field { damage left 1; }`)
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
	if trigger.SourceZone != "hand" || trigger.Event != "follower_left" || trigger.Predicate.(ir.FieldPredicate).Trait != "officer" {
		t.Fatal(trigger)
	}
	fusion := decoded.Cards[0].Abilities[1].Trigger.(ir.EventTrigger)
	if fusion.Event != "card_fused" || fusion.Side != "oppo" || fusion.SourceZone != "" {
		t.Fatal(fusion)
	}
}

func TestHandTriggerRejectsInvalidZoneAndScope(t *testing.T) {
	for _, body := range []string{
		`when own follower leaves { draw 1; }`,
		`when own follower leaves hand { draw 1; }`,
		`when own follower leaves field while self in deck { draw 1; }`,
		`when own follower leaves field while oppo in hand { draw 1; }`,
		`when own follower leaves field while self hand { draw 1; }`,
		`when own card fused { destroy left; }`,
		`when own follower leaves field { draw 1; } fanfare { destroy left; }`,
		`grant self { when own turn ends while self in hand { draw 1; } }`,
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard(body)))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid listener", body)
		}
	}
}
