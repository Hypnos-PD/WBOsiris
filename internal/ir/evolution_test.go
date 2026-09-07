package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestEvolutionShapesRoundTripAndRejectInvalidData(t *testing.T) {
	for _, form := range []string{"evolved", "super_evolved"} {
		effect := TargetEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "silent_evolve", Target: SelfRef{Kind: "self"}, Form: form}
		data, err := json.Marshal(effect)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeEffect(data, map[string]bool{})
		if err != nil || !reflect.DeepEqual(got, effect) {
			t.Fatalf("effect roundtrip: %s %v", data, err)
		}
		trigger := EventTrigger{Kind: "event", Event: form, Side: "own", SubjectType: "follower", SelfOnly: true}
		data, err = json.Marshal(trigger)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeTrigger(data)
		if err != nil || !reflect.DeepEqual(decoded, trigger) {
			t.Fatalf("trigger roundtrip: %s %v", data, err)
		}
	}
	for _, form := range []string{"unevolved", "evolved", "super_evolved"} {
		predicate := FieldPredicate{Kind: "has_form", Form: form}
		data, _ := json.Marshal(predicate)
		got, err := decodePredicate(data)
		if err != nil || !reflect.DeepEqual(got, predicate) {
			t.Fatal("form predicate roundtrip", err)
		}
	}
	for _, data := range []string{
		`{"kind":"event","event":"evolved","side":"oppo","subjectType":"follower","selfOnly":true}`,
		`{"kind":"event","event":"evolved","side":"own","subjectType":"amulet","selfOnly":true}`,
		`{"kind":"event","event":"turn_ended","side":"own","subjectType":"follower","selfOnly":true}`,
		`{"kind":"event","event":"evolved","side":"own","subjectType":"follower","selfOnly":true,"predicate":{"kind":"has_type","cardType":"follower"}}`,
	} {
		if _, err := decodeTrigger([]byte(data)); err == nil {
			t.Fatalf("accepted trigger %s", data)
		}
	}
	for _, data := range []string{`{"kind":"has_form"}`, `{"kind":"has_form","form":"unknown"}`, `{"kind":"has_form","form":"evolved","value":true}`} {
		if _, err := decodePredicate([]byte(data)); err == nil {
			t.Fatalf("accepted predicate %s", data)
		}
	}
}
