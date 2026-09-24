package ir

import (
	"encoding/json"
	"testing"
)

func TestKeywordPredicateStrictDecode(t *testing.T) {
	p := FieldPredicate{Kind: "has_keyword", Keyword: "ward"}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodePredicate(data)
	if err != nil || decoded.(FieldPredicate).Keyword != "ward" {
		t.Fatal(decoded, err)
	}
	for _, raw := range []string{
		`{"kind":"has_keyword"}`, `{"kind":"has_keyword","keyword":null}`,
		`{"kind":"has_keyword","keyword":"unknown"}`, `{"kind":"has_keyword","keyword":"ward","extra":1}`,
	} {
		if _, err := decodePredicate([]byte(raw)); err == nil {
			t.Fatal("accepted invalid keyword predicate", raw)
		}
	}
	for _, subject := range []string{"follower", "amulet"} {
		raw := []byte(`{"kind":"event","event":"destroyed","side":"own","subjectType":"` + subject + `","predicate":{"kind":"has_keyword","keyword":"ward"}}`)
		if _, err := decodeTrigger(raw); err != nil {
			t.Fatal(err)
		}
	}
	for _, subject := range []string{"", "spell", "card"} {
		raw := []byte(`{"kind":"event","event":"destroyed","side":"own","subjectType":"` + subject + `"}`)
		if _, err := decodeTrigger(raw); err == nil {
			t.Fatal("accepted unsupported destroyed subject", subject)
		}
	}
}
