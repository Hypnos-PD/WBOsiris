package project

import (
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestSetLifeCompilation(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`fanfare { choose target from oppo.field.followers; set life target 1; set life own.field.followers count(own.hand); }`)
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	l := LoadWithRoot([]string{path}, true, root)
	p, _, err := BuildRuntimePacks(l)
	if err != nil {
		t.Fatal(err, l.Diagnostics)
	}
	body := p.Cards[0].Abilities[0].Body
	e := body[1].(ir.TargetEffect)
	if e.Kind != "set_life" || e.Amount != 1 {
		t.Fatal(e)
	}
	if body[2].(ir.TargetEffect).AmountExpr == nil {
		t.Fatal("lost numeric input")
	}
	data, err := ir.EncodeCardPack(*p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ir.DecodeCardPack(data); err != nil {
		t.Fatal(err)
	}
}

func TestRejectMalformedSetLife(t *testing.T) {
	for _, operation := range []string{"set life", "set life self", "set attack self 1", "set life unknown 1", "set life own.leader 1", "set life all.leaders 1", "set life leaders 1", "set life self -1", "set life self 1 extra", "set life self 1 where type follower"} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+operation+"; }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid set life", operation)
		}
	}
}
