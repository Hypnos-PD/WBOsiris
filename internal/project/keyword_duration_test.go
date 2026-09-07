package project

import (
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileKeywordDurations(t *testing.T) {
	for _, tc := range []struct{ suffix, until string }{{"turn ends", "turn_end"}, {"own turn ends", "own_turn_end"}, {"oppo turn ends", "oppo_turn_end"}} {
		root := t.TempDir()
		dir := filepath.Join(root, "12345")
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "12345678.wbo")
		text := validCard(`fanfare { add barrier to own.field.followers other where life <= 3 until ` + tc.suffix + `; }`)
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
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
		if e.Until != tc.until || e.Predicate == nil {
			t.Fatal("lost duration or filter", e)
		}
		if _, ok := e.Target.(ir.ExcludeRef); !ok {
			t.Fatal("lost other exclusion")
		}
	}
}

func TestRejectMalformedKeywordDurations(t *testing.T) {
	for _, operation := range []string{
		"add storm to self until", "add storm to self until own", "add storm to self until turn",
		"add storm to self until enemy turn ends", "add storm to self until own turn starts",
		"add storm to self until turn ends extra", "add storm to self until turn ends where life == 1",
		"remove storm from self until turn ends", "buff self +1/+0 until turn ends",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+operation+"; }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid duration", operation)
		}
	}
}
