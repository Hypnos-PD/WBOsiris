package project

import (
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestDestructionResultCompilation(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`fanfare {
destroy own.field.amulets;
damage oppo.leader count(destroyed where type amulet);
draw 1;
heal own.leader count(drawn);
random targets from oppo.field.followers count 2;
repeat count(targets) { heal own.leader 1; }
}`)
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	l := LoadWithRoot([]string{path}, true, root)
	p, _, err := BuildRuntimePacks(l)
	if err != nil {
		t.Fatal(err, l.Diagnostics)
	}
	body := p.Cards[0].Abilities[0].Body
	if body[0].(ir.TargetEffect).Output != "destroyed" {
		t.Fatal("missing result")
	}
	count := body[1].(ir.TargetEffect).AmountExpr.(*ir.CountExpr)
	if count.Source.(ir.FilterRef).Source.(ir.BindingRef).Name != "destroyed" {
		t.Fatal(count)
	}
	data, err := ir.EncodeCardPack(*p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ir.DecodeCardPack(data); err != nil {
		t.Fatal(err)
	}
}

func TestDestructionResultScope(t *testing.T) {
	for _, body := range []string{
		`fanfare { damage oppo.leader count(destroyed); }`,
		`fanfare { destroy own.field.amulets; } lastwords { damage oppo.leader count(destroyed); }`,
		`fanfare { if overflow { destroy own.field.amulets; } damage oppo.leader count(destroyed); }`,
		`fanfare { repeat count(missing) { draw 1; } }`,
		`fanfare { destroy own.field.amulets; heal own.leader count(self); }`,
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard(body)))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted undefined result", body)
		}
	}
}
