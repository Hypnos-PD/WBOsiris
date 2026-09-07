package project

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestKeywordEffectFilterRoundTrip(t *testing.T) {
	for _, operation := range []string{"add barrier to", "remove barrier from"} {
		for _, suffix := range []string{"", " other"} {
			t.Run(operation+suffix, func(t *testing.T) {
				root := t.TempDir()
				dir := filepath.Join(root, "12345")
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, "12345678.wbo")
				source := validCard(`fanfare { ` + operation + ` own.field.followers` + suffix + ` where class swordcraft and life != 3; }`)
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				loaded := LoadWithRoot([]string{path}, true, root)
				pack, _, err := BuildRuntimePacks(loaded)
				if err != nil {
					t.Fatal(err, loaded.Diagnostics)
				}
				e := pack.Cards[0].Abilities[0].Body[0].(ir.TargetEffect)
				_, excluded := e.Target.(ir.ExcludeRef)
				if excluded != (suffix != "") || e.Keyword != "barrier" || e.Predicate == nil {
					t.Fatalf("lost keyword target constraints: %#v", e)
				}
				data, err := ir.EncodeCardPack(*pack)
				if err != nil {
					t.Fatal(err)
				}
				decoded, err := ir.DecodeCardPack(data)
				if err != nil {
					t.Fatal(err)
				}
				if got := decoded.Cards[0].Abilities[0].Body[0]; !reflect.DeepEqual(e, got) {
					t.Fatalf("keyword effect changed on round trip: %#v", got)
				}
			})
		}
	}
}

func TestRejectMalformedKeywordFilters(t *testing.T) {
	for _, operation := range []string{"add barrier to", "remove barrier from"} {
		for _, target := range []string{"unknown other", "self other other", "own.field where", "own.field where class swordcraft other", "own.field where class invalid", "own.field where life >= -1"} {
			f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+operation+" "+target+"; }")))
			if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
				t.Fatal("accepted malformed keyword filter", operation, target)
			}
		}
	}
}
