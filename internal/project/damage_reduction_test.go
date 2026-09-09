package project

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileSetDamageReduction(t *testing.T) {
	for _, tc := range []struct {
		name, body    string
		index, amount int
		target        ir.Ref
	}{
		{"self", "set_damage_reduction self 0;", 0, 0, ir.SelfRef{Kind: "self", ValueType: "entity"}},
		{"binding", "choose target from own.field.followers; set_damage_reduction target 3;", 1, 3, ir.BindingRef{Kind: "binding", Name: "target"}},
		{"own field", "set_damage_reduction own.field.followers 3;", 0, 3, ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}},
		{"opposing field", "set_damage_reduction oppo.field.followers 7;", 0, 7, ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+tc.body+" }")))
			if len(ds) != 0 {
				t.Fatal(ds)
			}
			if ds := ValidateFile(f); hasErrors(ds) {
				t.Fatal(ds)
			}
			formatted := syntax.Format(f)
			again, ds := syntax.Parse("12345678.wbo", formatted)
			if len(ds) != 0 || !bytes.Equal(formatted, syntax.Format(again)) {
				t.Fatal("unstable formatting", ds)
			}
			root := t.TempDir()
			path := filepath.Join(root, "12345", "12345678.wbo")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
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
			e := pack.Cards[0].Abilities[0].Body[tc.index].(ir.TargetEffect)
			if e.Kind != "set_damage_reduction" || e.Amount != tc.amount || !reflect.DeepEqual(e.Target, tc.target) {
				t.Fatalf("compiled effect = %#v", e)
			}
			data, err := ir.EncodeCardPack(*pack)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := ir.DecodeCardPack(data)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(e, decoded.Cards[0].Abilities[0].Body[tc.index]) {
				t.Fatal("effect changed during IR round trip")
			}
		})
	}
}

func TestRejectMalformedDamageReduction(t *testing.T) {
	for _, body := range []string{
		"set_damage_reduction;", "set_damage_reduction self;", "set_damage_reduction self -1;",
		"set_damage_reduction self 65536;", "set_damage_reduction missing 1;",
		"set_damage_reduction own.field.followers;", "set_damage_reduction self 1 extra;",
		"set_damage_reduction own.leader 1;", "set_damage_reduction all.leaders 1;", "set_damage_reduction leaders 1;",
		"choose target from own.field.followers or own.leader; set_damage_reduction target 1;",
	} {
		t.Run(body, func(t *testing.T) {
			f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+body+" }")))
			if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
				t.Fatal("accepted malformed damage reduction")
			}
		})
	}
}
