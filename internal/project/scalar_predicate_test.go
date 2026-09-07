package project

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompilePlayerScalarPredicates(t *testing.T) {
	source := validCard(`fanfare {
    draw 1 from deck where type follower and cost == own.combo;
    choose targets from oppo.field.followers where life <= own.life or cost != oppo.pp;
    damage targets count(own.hand where cost >= own.ep);
    buff own.field.followers +1/+1 where cost < oppo.sep;
    heal own.leader sum(own.destroyed this turn where cost > own.shadows, base.cost);
}
when oppo amulet summoned where cost <= own.life { draw 1; }
fusion material from own.hand where cost <= oppo.maxpp { draw 1; }`)
	f, ds := syntax.Parse("12345678.wbo", []byte(source))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	formatted := syntax.Format(f)
	f, ds = syntax.Parse("12345678.wbo", formatted)
	if len(ds) != 0 || !bytes.Equal(formatted, syntax.Format(f)) {
		t.Fatal("unstable scalar predicate formatting", ds)
	}
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	if err := os.WriteFile(path, formatted, 0600); err != nil {
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
	p := decoded.Cards[0].Abilities[0].Body[0].(ir.DrawEffect).Predicate.(ir.AndPredicate).Terms[1].(ir.FieldPredicate)
	if p.ValueScalar == nil || *p.ValueScalar != (ir.Scalar{Kind: "scalar", Side: "own", Field: "combo"}) {
		t.Fatal("predicate lost the ability controller", p)
	}
}

func TestRejectMalformedPlayerScalarPredicates(t *testing.T) {
	for _, right := range []string{"combo", "self.cost", "target.cost", "own.cost", "both.combo", "own", "own.", "own.combo + 1", "count(own.hand)", "-own.combo", "own.combo.combo"} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { draw 1 from deck where cost == "+right+"; }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted ambiguous scalar comparison", right)
		}
	}
	for _, body := range []string{
		"when oppo amulet summoned where life <= own.life { draw 1; }",
		"fusion material from own.hand where life <= own.life { draw 1; }",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard(body)))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("player life comparison bypassed candidate type checking", body)
		}
	}
}
