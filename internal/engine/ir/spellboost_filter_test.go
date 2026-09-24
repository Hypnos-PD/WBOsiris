package ir

import "testing"

func TestSpellboostPredicateRejectsExtraFields(t *testing.T) {
	for _, raw := range []string{
		`{"kind":"has_spellboost","value":1}`,
		`{"kind":"has_spellboost","keyword":"storm"}`,
		`{"kind":"has_spellboost","side":"own"}`,
	} {
		if _, err := decodePredicate([]byte(raw)); err == nil {
			t.Fatal("accepted malformed spellboost predicate", raw)
		}
	}
}
