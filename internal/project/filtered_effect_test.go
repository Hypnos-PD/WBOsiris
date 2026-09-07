package project

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"wbo/internal/ir"
)

func TestCompileBuffPreservesFilterAndSignedStats(t *testing.T) {
	for _, other := range []bool{false, true} {
		name, exclude := "all", ""
		if other {
			name, exclude = "other", " other"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "12345")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "12345678.wbo")
			text := validCard(`fanfare { buff own.field.followers` + exclude + ` -2/+3 where trait puppetry and life != 4; }`)
			if err := os.WriteFile(path, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			loaded := LoadWithRoot([]string{path}, true, root)
			if loaded.HasErrors() {
				t.Fatal(loaded.Diagnostics)
			}
			pack, _, err := BuildRuntimePacks(loaded)
			if err != nil {
				t.Fatal(err)
			}
			effect := pack.Cards[0].Abilities[0].Body[0].(ir.TargetEffect)
			want := ir.AndPredicate{Kind: "and", Terms: []ir.Predicate{
				ir.FieldPredicate{Kind: "has_trait", Trait: "puppetry"},
				ir.FieldPredicate{Kind: "compare", Field: "life", Op: "ne", Value: 4},
			}}
			if effect.AttackDelta != -2 || effect.LifeDelta != 3 || !reflect.DeepEqual(effect.Predicate, want) {
				t.Fatalf("lost signed stats or filter during compilation: %#v", effect)
			}
			_, excluded := effect.Target.(ir.ExcludeRef)
			if excluded != other {
				t.Fatalf("incorrect other exclusion: %#v", effect.Target)
			}
		})
	}
}
