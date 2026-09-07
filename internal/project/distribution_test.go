package project

import (
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileDistributedDamageWithCountAndLeaderOverflow(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`fanfare { damage oppo.field.followers count(own.hand where type follower) distributed overflow oppo.leader where trait officer; }`)
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
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
	if effect.Distribution != "field_entry_order" || effect.AmountExpr == nil || effect.Predicate == nil || effect.Overflow != (ir.LeaderRef{Kind: "leader", Side: "oppo", ValueType: "leader"}) {
		t.Fatalf("lost distribution or overflow: %#v", effect)
	}
}

func TestRejectInvalidDistributedDamageSyntax(t *testing.T) {
	for _, text := range []string{
		"heal own.field.followers 3 distributed",
		"damage self 3 distributed", "damage oppo.leader 3 distributed",
		"damage field.followers 3 distributed", "damage oppo.hand.followers 3 distributed",
		"damage oppo.field 3 distributed", "damage oppo.field.amulets 3 distributed",
		"damage oppo.field.followers 3 distributed overflow own.leader",
		"damage oppo.field.followers 3 overflow oppo.leader",
		"damage oppo.field.followers 3 distributed overflow",
		"damage oppo.field.followers 3 distributed overflow oppo.field",
		"damage oppo.field.followers 3 distributed twice",
		"damage oppo.field.followers 3 distributed distributed",
	} {
		t.Run(text, func(t *testing.T) {
			file, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+text+"; }")))
			if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
				t.Fatal("accepted invalid distribution")
			}
		})
	}
}
