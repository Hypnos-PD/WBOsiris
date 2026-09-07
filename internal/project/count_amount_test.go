package project

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileCountAmountKeepsSourceAndTargetFiltersSeparate(t *testing.T) {
	source := validCard(`fanfare {
		damage oppo.field.followers count(own.hand.followers where trait golem and life != 4) where trait officer;
		heal own.leader count(own.hand);
	}`)
	file, ds := syntax.Parse("12345678.wbo", []byte(source))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	formatted := syntax.Format(file)
	if !strings.Contains(string(formatted), "count(own.hand.followers where trait golem and life != 4)") {
		t.Fatalf("count syntax formatting changed: %s", formatted)
	}
	file, ds = syntax.Parse("12345678.wbo", formatted)
	if len(ds) != 0 || !bytes.Equal(formatted, syntax.Format(file)) {
		t.Fatal("count formatting is not stable")
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
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	damage := pack.Cards[0].Abilities[0].Body[0].(ir.TargetEffect)
	want := &ir.CountExpr{Kind: "count", Source: ir.FilterRef{Kind: "filter",
		Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand", Member: "follower"},
		Predicate: ir.AndPredicate{Kind: "and", Terms: []ir.Predicate{
			ir.FieldPredicate{Kind: "has_trait", Trait: "golem"},
			ir.FieldPredicate{Kind: "compare", Field: "life", Op: "ne", Value: 4},
		}},
	}}
	if !reflect.DeepEqual(damage.AmountExpr, want) || !reflect.DeepEqual(damage.Predicate, ir.FieldPredicate{Kind: "has_trait", Trait: "officer"}) {
		t.Fatalf("mixed count and target filters: %#v", damage)
	}
	heal := pack.Cards[0].Abilities[0].Body[1].(ir.TargetEffect)
	if !reflect.DeepEqual(heal.AmountExpr, &ir.CountExpr{Kind: "count", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand", Member: "card"}}) {
		t.Fatalf("lost unfiltered count: %#v", heal)
	}
}

func TestRejectMalformedCountAmounts(t *testing.T) {
	for _, amount := range []string{
		"count", "count()", "count(own.hand", "count own.hand)", "count(own.hand))",
		"count(own.leader)", "count(self)", "count(target)", "count(all.leaders)",
		"count(own.hand where)", "count(own.hand where trait unknown)",
		"count(own.hand where trait golem and)", "count(count(own.hand))",
		"count(own.hand) + 1", "count(own.hand) where", "-1",
	} {
		t.Run(amount, func(t *testing.T) {
			file, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { heal own.leader "+amount+"; }")))
			if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
				t.Fatal("accepted malformed count")
			}
		})
	}
}
