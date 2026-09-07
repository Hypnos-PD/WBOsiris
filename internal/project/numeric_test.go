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

func TestCompileNumericEffects(t *testing.T) {
	source := validCard(`fanfare {
		buff self +own.combo/-count(own.hand.followers where trait golem);
		damage oppo.field.followers self.attack distributed;
		heal own.leader oppo.shadows;
		buff own.field.followers other +count(own.hand)/+self.life where trait officer;
	}`)
	file, ds := syntax.Parse("12345678.wbo", []byte(source))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	formatted := syntax.Format(file)
	file, ds = syntax.Parse("12345678.wbo", formatted)
	if len(ds) != 0 || !bytes.Equal(formatted, syntax.Format(file)) {
		t.Fatal("numeric formatting is unstable")
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
	body := pack.Cards[0].Abilities[0].Body
	buff := body[0].(ir.TargetEffect)
	if !reflect.DeepEqual(buff.AttackExpr, &ir.Scalar{Kind: "scalar", Side: "own", Field: "combo"}) {
		t.Fatal(buff)
	}
	if _, ok := buff.LifeExpr.(*ir.NegateExpr); !ok {
		t.Fatal(buff)
	}
	damage := body[1].(ir.TargetEffect)
	if !reflect.DeepEqual(damage.AmountExpr, &ir.Scalar{Kind: "self_scalar", Field: "attack"}) || damage.Distribution != "field_entry_order" {
		t.Fatal(damage)
	}
	filtered := body[3].(ir.TargetEffect)
	if _, ok := filtered.Target.(ir.ExcludeRef); !ok || filtered.Predicate == nil || filtered.AttackExpr == nil || filtered.LifeExpr == nil {
		t.Fatal(filtered)
	}
}

func TestRejectAmbiguousNumericSyntax(t *testing.T) {
	for _, operation := range []string{
		"buff self own.combo/+0", "buff self +own.combo", "buff self +1/+", "buff self +1/+1 extra",
		"buff self +self.unknown/+0", "buff self +own.combo+1/+0", "buff self +count(self)/+0",
		"buff self +count(own.hand)/-count(own.hand where)", "damage oppo.leader target.attack",
		"damage oppo.leader self.attack + 1", "damage oppo.leader own.field", "heal own.leader -own.combo",
	} {
		t.Run(operation, func(t *testing.T) {
			file, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+operation+"; }")))
			if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
				t.Fatal("accepted invalid numeric syntax")
			}
		})
	}
	source := strings.Replace(validCard("damage oppo.leader self.attack;"), "type follower;", "type spell;", 1)
	source = strings.Replace(source, "stats 1/1;", "", 1)
	file, _ := syntax.Parse("12345678.wbo", []byte(source))
	diagnostics := ValidateFile(file)
	if len(diagnostics) != 1 || diagnostics[0].Code != "WBO-E008-TYPE-MISMATCH" {
		t.Fatalf("expected a follower-stat type error, got %#v", diagnostics)
	}
}
