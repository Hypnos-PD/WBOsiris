package ir

import "testing"

func TestDecodeEvolutionConditionsRejectsInvalidFields(t *testing.T) {
	for _, raw := range []string{
		`{"kind":"self_form"}`, `{"kind":"self_form","form":"normal"}`,
		`{"kind":"self_form","form":"evolved","side":"own"}`,
		`{"kind":"self_form","form":1}`,
		`{"kind":"evolution_unlocked","side":"own","form":"unevolved"}`,
		`{"kind":"evolution_unlocked","side":"enemy","form":"evolved"}`,
		`{"kind":"evolution_unlocked","form":"super_evolved"}`,
		`{"kind":"evolution_unlocked","side":"own","form":"super_evolved","turn":6}`,
	} {
		if _, err := decodeCondition([]byte(raw)); err == nil {
			t.Fatal("accepted malformed condition", raw)
		}
	}
}
