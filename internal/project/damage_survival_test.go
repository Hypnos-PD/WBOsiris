package project

import (
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestDamageSurvivalTriggerRoundTrip(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`when self survives damage during own turn { draw 1; }
when self survives damage once per own turn if own.hand_count <= 5 { draw 1; }
when self survives damage during oppo turn once per turn { draw 1; }
when own follower survives damage once per own turn { buff damaged +0/+1; }
when oppo follower survives damage during oppo turn while self in hand once per turn where class dragoncraft if own.hand_count <= 5 { draw 1; }`)
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
		trigger := ability.Trigger.(ir.EventTrigger)
		if trigger.Event != "damaged" || trigger.SelfOnly != (n < 3) || trigger.SubjectType != "follower" || (trigger.Side == "own") != (n < 4) {
			t.Fatal(trigger)
		}
		if n == 0 && (trigger.DuringTurn != "own" || trigger.OncePerTurn != "") || n == 1 && (trigger.OncePerTurn != "own" || trigger.Condition == nil) || n == 2 && (trigger.DuringTurn != "oppo" || trigger.OncePerTurn != "any") {
			t.Fatal(trigger)
		}
		if n == 3 && (trigger.OncePerTurn != "own" || ability.Body[0].(ir.TargetEffect).Target.(ir.BindingRef).Name != "damaged") || n == 4 && (trigger.DuringTurn != "oppo" || trigger.SourceZone != "hand" || trigger.Predicate == nil || trigger.Condition == nil) {
			t.Fatal(trigger)
		}
	}
	f, ds := syntax.Parse(path, []byte(source))
	formatted := syntax.Format(f)
	f2, more := syntax.Parse(path, formatted)
	if len(ds)+len(more) != 0 || string(syntax.Format(f2)) != string(formatted) {
		t.Fatal("unstable formatting")
	}
}

func TestRejectMalformedDamageSurvivalTriggers(t *testing.T) {
	for _, body := range []string{
		`when self damaged { draw 1; }`, `when self survives { draw 1; }`,
		`when own amulet survives damage { draw 1; }`,
		`when own leader survives damage { draw 1; }`,
		`when own follower survives { draw 1; }`,
		`when self survives damage { heal damaged 1; }`,
		`when self survives damage during turn { draw 1; }`,
		`when self survives damage during any turn { draw 1; }`,
		`when self survives damage while self in hand { draw 1; }`,
		`when self survives damage where life <= 3 { draw 1; }`,
		`when self survives damage once per own { draw 1; }`,
		`when self survives damage once per turn during own turn { draw 1; }`,
		`when own follower summoned during own turn { draw 1; }`,
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard(body)))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid damage listener", body)
		}
	}
}
