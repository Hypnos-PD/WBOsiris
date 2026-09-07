package project

import (
	"os"
	"path/filepath"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestTriggerLimitsRoundTrip(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`when own follower summoned once per own turn where trait puppetry { add bane to summoned; }
when oppo follower summoned once per oppo turn { draw 1; }
when own follower leaves field while self in hand once per turn where trait officer { reduce cost self 1 minimum 0; }`)
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
	for n, scope := range []string{"own", "oppo", "any"} {
		trigger := decoded.Cards[0].Abilities[n].Trigger.(ir.EventTrigger)
		if trigger.OncePerTurn != scope {
			t.Fatal(trigger)
		}
		if n == 2 && (trigger.SourceZone != "hand" || trigger.Predicate.(ir.FieldPredicate).Trait != "officer") {
			t.Fatal(trigger)
		}
	}
}

func TestTriggerLimitsRejectAmbiguousModifiers(t *testing.T) {
	for _, body := range []string{
		`when own follower summoned once turn { draw 1; }`,
		`when own follower summoned once per { draw 1; }`,
		`when own follower summoned once per any turn { draw 1; }`,
		`when own follower summoned once per 2 turn { draw 1; }`,
		`when own follower summoned once per own turn once per turn { draw 1; }`,
		`when own follower summoned once per turn while self in hand { draw 1; }`,
		`when own follower summoned where trait puppetry once per turn { draw 1; }`,
		`when self evolved once per turn { draw 1; }`,
		`when self discarded once per turn { draw 1; }`,
		`grant self { when own turn ends once per turn { draw 1; } }`,
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard(body)))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid limit", body)
		}
	}
}
