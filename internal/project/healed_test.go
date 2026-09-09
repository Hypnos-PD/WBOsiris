package project

import (
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestLeaderHealedEventRoundTrip(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`when own leader healed once per own turn if own.hand_count <= 5 { damage healed 1; }
when oppo leader healed while self in hand { heal own.leader 1; }`)
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
	a := decoded.Cards[0].Abilities
	first, second := a[0].Trigger.(ir.EventTrigger), a[1].Trigger.(ir.EventTrigger)
	if first.Event != "healed" || first.SubjectType != "leader" || first.Side != "own" || first.OncePerTurn != "own" || first.Condition == nil || second.Side != "oppo" || second.SourceZone != "hand" {
		t.Fatal(first, second)
	}
	if a[0].Body[0].(ir.TargetEffect).Target.(ir.BindingRef).Name != "healed" {
		t.Fatal("missing leader binding")
	}
}

func TestRejectMalformedLeaderHealedEvents(t *testing.T) {
	for _, body := range []string{
		`when own follower healed { draw 1; }`,
		`when self healed { draw 1; }`,
		`when own leader summoned { draw 1; }`,
		`when own leader healed where keyword ward { draw 1; }`,
		`when own leader healed where card 12345678 { draw 1; }`,
		`fanfare { damage healed 1; }`,
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard(body)))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid leader event", body)
		}
	}
}
