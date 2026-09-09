package project

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
)

func TestInitialDamageOverrideValidationAndRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cards", "12345"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cards", "12345", "12345678.wbo"), []byte(validCard("")), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "healing.wbotest")
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{"stats 8/6; damage_taken 4;", true}, {"damage_taken 4; stats 8/6;", true},
		{"stats 8/6; damage_taken 0;", true}, {"stats 8/6; damage_taken 6;", false},
		{"stats 8/6; damage_taken -1;", false}, {"stats 8/6; damage_taken 65536;", false},
		{"stats 8/6; damage_taken 1; damage_taken 2;", false},
	} {
		source := fmt.Sprintf(`wbotest 0.1.0; use cards "cards";
scenario "wounded" { seed 50; state { player own { field { follower source = 12345678 { %s } } } }
action { attack source into oppo.leader; } expect { legal; } }`, tc.body)
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		loaded := LoadWithRoot([]string{path}, true, root)
		if loaded.HasErrors() == tc.valid {
			t.Fatal(tc.body, loaded.Diagnostics)
		}
		if !tc.valid {
			continue
		}
		_, pack, err := BuildRuntimePacks(loaded)
		if err != nil {
			t.Fatal(err)
		}
		data, err := ir.EncodeTestPack(*pack)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := ir.DecodeTestPack(data)
		if err != nil {
			t.Fatal(err)
		}
		o := decoded.Scenarios[0].InitialState.Players["own"].Zones["field"][0].Overrides
		if o.DamageTaken == nil || *o.DamageTaken != *pack.Scenarios[0].InitialState.Players["own"].Zones["field"][0].Overrides.DamageTaken {
			t.Fatal("damage override lost during IR roundtrip")
		}
	}
}
