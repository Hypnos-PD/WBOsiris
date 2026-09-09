package ir

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDamageTakenOverrideRejectsImpossibleWounds(t *testing.T) {
	for _, tc := range []struct {
		kind   string
		damage int
		valid  bool
	}{
		{"follower", 0, true}, {"follower", 4, true}, {"follower", -1, false},
		{"follower", 6, false}, {"follower", 65536, false}, {"amulet", 0, false}, {"spell", 1, false},
	} {
		value := TestInstance{InstanceID: strings.Repeat("1", 32), Alias: "source", CardID: 10411110, DeclaredType: tc.kind,
			Overrides: InstanceOverrides{Stats: &Stats{Attack: 8, Life: 6}, DamageTaken: &tc.damage}}
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeInstance(data)
		if (err == nil) != tc.valid {
			t.Fatal(tc, err)
		}
		if tc.valid && (decoded.Overrides.DamageTaken == nil || *decoded.Overrides.DamageTaken != tc.damage) {
			t.Fatal("damage override lost")
		}
	}
}
