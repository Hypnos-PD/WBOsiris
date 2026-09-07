package project

import (
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestDiscardCompilationAndEventBinding(t *testing.T) {
	source := validCard(`when self discarded { buff self +1/+0; }
when own card discarded { buff discarded +1/+0; }
when oppo card discarded where type spell { draw 1; }
fanfare { choose target from own.hand; discard target; }`)
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	loaded := LoadWithRoot([]string{path}, true, root)
	pack, _, err := BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err, loaded.Diagnostics)
	}
	for n := 0; n < 3; n++ {
		trigger := pack.Cards[0].Abilities[n].Trigger.(ir.EventTrigger)
		if trigger.Event != "card_discarded" || trigger.SubjectType != "" || trigger.SelfOnly != (n == 0) {
			t.Fatal(trigger)
		}
	}
	data, err := ir.EncodeCardPack(*pack)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ir.DecodeCardPack(data); err != nil {
		t.Fatal(err)
	}
}

func TestDiscardRejectsMalformedTargetsAndEscapedBinding(t *testing.T) {
	for _, body := range []string{
		`fanfare { discard; }`, `fanfare { discard own.leader; }`, `fanfare { discard leaders; }`,
		`fanfare { discard missing; }`, `fanfare { discard own.hand extra; }`,
		`when self discarded where type spell { draw 1; }`, `fanfare { buff discarded +1/+0; }`,
	} {
		file, ds := syntax.Parse("12345678.wbo", []byte(validCard(body)))
		if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
			t.Fatal("accepted invalid discard", body)
		}
	}
}
