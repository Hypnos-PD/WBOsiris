package project

import (
	"os"
	"path/filepath"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestEventConditionsAndResourceCountsRoundTrip(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`counter x 0;
fanfare { damage oppo.leader own.earthsigils; }
when own turn ends if own.hand_count <= 5 { draw 1; }
when own follower summoned once per own turn where trait pixie if self.counter.x == 0 { add 1 counter x; }
when oppo amulet summoned where cost <= own.hand_count if oppo.earthsigils >= 2 { draw 1; }`)
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
	abilities := decoded.Cards[0].Abilities
	if abilities[0].Body[0].(ir.TargetEffect).AmountExpr.(*ir.Scalar).Field != "earthsigils" {
		t.Fatal("wrong sigil scalar")
	}
	for n, field := range []string{"hand_count", "x", "earthsigils"} {
		trigger := abilities[n+1].Trigger.(ir.EventTrigger)
		if trigger.Condition.(ir.CompareCondition).Left.Field != field {
			t.Fatal("condition lost", trigger)
		}
		if n == 1 && (trigger.OncePerTurn != "own" || trigger.Predicate == nil) {
			t.Fatal("condition lost other event modifiers")
		}
	}
}

func TestEventConditionsRejectAmbiguousOrOutOfScopeExpressions(t *testing.T) {
	for _, body := range []string{
		`when own turn ends if { draw 1; }`,
		`when own turn ends if own.hand_count { draw 1; }`,
		`when own turn ends if own.hand_count <= 5 extra { draw 1; }`,
		`when own turn ends if fused.cost >= 3 { draw 1; }`,
		`when own turn ends if self.counter.missing == 0 { draw 1; }`,
		`when self summoned if own.hand_count == 0 { draw 1; }`,
		`when own follower summoned if own.hand_count == 0 where trait pixie { draw 1; }`,
		`gain own.hand_count 1;`, `fanfare { gain own.earthsigils 2; }`,
		`grant self { when own turn ends if own.hand_count == 0 { draw 1; } }`,
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard(body)))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid condition", body)
		}
	}
}
