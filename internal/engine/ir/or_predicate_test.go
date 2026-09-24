package ir

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestOrPredicateRoundTripAndReferenceValidation(t *testing.T) {
	want := OrPredicate{Kind: "or", Terms: []Predicate{
		FieldPredicate{Kind: "has_card", CardID: 90073120},
		AndPredicate{Kind: "and", Terms: []Predicate{
			FieldPredicate{Kind: "has_card", CardID: 90073130},
			FieldPredicate{Kind: "has_trait", Trait: "artifact"},
		}},
	}}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodePredicate(data)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("lost disjunction or nested conjunction", err)
	}
	if err := validatePredicateCardRefs(got, map[int]bool{90073120: true}); err == nil {
		t.Fatal("missing reference in second branch was accepted")
	}
	if err := validatePredicateCardRefs(got, map[int]bool{90073120: true, 90073130: true}); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"kind":"or","terms":[]}`,
		`{"kind":"or","terms":[{"kind":"has_card","cardId":90073120}]}`,
		`{"kind":"or","terms":[null,null]}`,
		`{"kind":"or","terms":[{"kind":"has_card","cardId":90073120},{"kind":"has_card","cardId":90073130}],"extra":true}`,
	} {
		if _, err := decodePredicate([]byte(raw)); err == nil {
			t.Fatalf("accepted malformed disjunction %s", raw)
		}
	}
}
