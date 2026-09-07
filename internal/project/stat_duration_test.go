package project

import (
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileStatDurations(t *testing.T) {
	for _, tc := range []struct{ suffix, until string }{{"turn ends", "turn_end"}, {"own turn ends", "own_turn_end"}, {"oppo turn ends", "oppo_turn_end"}} {
		root := t.TempDir()
		dir := filepath.Join(root, "12345")
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "12345678.wbo")
		source := validCard(`fanfare { buff own.field.followers other +own.combo/-2 where life >= 3 until ` + tc.suffix + `; }`)
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
		e := decoded.Cards[0].Abilities[0].Body[0].(ir.TargetEffect)
		if e.Until != tc.until || e.Predicate == nil || e.AttackExpr == nil || e.LifeDelta != -2 {
			t.Fatal("lost duration, dynamic amount or filter", e)
		}
		if _, ok := e.Target.(ir.ExcludeRef); !ok {
			t.Fatal("lost exclusion")
		}
	}
}

func TestRejectMalformedStatDurations(t *testing.T) {
	for _, operation := range []string{
		"buff self +1/+0 until", "buff self +1/+0 until own", "buff self +1/+0 until turn",
		"buff self +1/+0 until enemy turn ends", "buff self +1/+0 until own turn starts",
		"buff self +1/+0 until turn ends extra", "buff self +1/+0 until turn ends where life == 1",
		"buff self until turn ends +1/+0", "buff self +1/+0 where until turn ends",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+operation+"; }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted malformed duration", operation)
		}
	}
}
