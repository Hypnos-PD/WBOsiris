package project

import (
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileExtremumSelection(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`fanfare {
random targets from oppo.field.followers where life >= 2 highest attack count 2;
choose cheap from own.hand lowest cost;
require wounded from own.field.followers other lowest life;
}`)
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	l := LoadWithRoot([]string{path}, true, root)
	p, _, err := BuildRuntimePacks(l)
	if err != nil {
		t.Fatal(err, l.Diagnostics)
	}
	for n, want := range []ir.SelectionExtremum{{Direction: "highest", Field: "attack"}, {Direction: "lowest", Field: "cost"}, {Direction: "lowest", Field: "life"}} {
		got := p.Cards[0].Abilities[0].Body[n].(ir.SelectionEffect)
		if got.Extremum == nil || *got.Extremum != want {
			t.Fatal(got)
		}
	}
	data, err := ir.EncodeCardPack(*p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ir.DecodeCardPack(data); err != nil {
		t.Fatal(err)
	}
}

func TestRejectMalformedExtremumSelection(t *testing.T) {
	for _, suffix := range []string{"highest", "highest mana", "highest self.attack", "highest attack lowest life", "count 2 highest attack", "highest attack where life > 2", "highest attack count 0"} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { random target from oppo.field.followers "+suffix+"; }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted malformed ranking", suffix)
		}
	}
}
