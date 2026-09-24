package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestMixedSelectionSourceDecoding(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		e := SelectionEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "require", Policy: "required", Binding: "target", Source: CharacterSetRef{Kind: "characters", Side: side}}
		data, _ := json.Marshal(e)
		decoded, err := decodeEffect(data, map[string]bool{})
		if err != nil || !reflect.DeepEqual(e, decoded) {
			t.Fatal(decoded, err)
		}
		e.Extremum = &SelectionExtremum{Direction: "highest", Field: "cost"}
		data, _ = json.Marshal(e)
		if _, err := decodeEffect(data, map[string]bool{}); err == nil {
			t.Fatal("accepted extrema for leaders")
		}
	}
	for _, source := range []string{
		`{"kind":"characters","side":"all"}`,
		`{"kind":"characters","side":"oppo","member":"spell"}`,
		`{"kind":"filter","source":{"kind":"characters","side":"oppo"},"predicate":{"kind":"has_type","cardType":"follower"}}`,
		`{"kind":"exclude","source":{"kind":"characters","side":"oppo"},"value":{"kind":"self"}}`,
	} {
		if _, err := decodeSelectionSource([]byte(source)); err == nil {
			t.Fatal("accepted malformed character set", source)
		}
	}
	if _, err := decodeRef([]byte(`{"kind":"characters","side":"own"}`)); err == nil {
		t.Fatal("accepted character set outside a selection source")
	}
}

func TestMixedSelectionResponseDecoding(t *testing.T) {
	for _, action := range []SelectAction{
		{Kind: "select", LeaderSides: []string{"oppo"}},
		{Kind: "select", Target: strings.Repeat("a", 32), LeaderSides: []string{"own", "oppo"}},
	} {
		data, _ := json.Marshal(action)
		decoded, err := decodeAction(data)
		if err != nil || !reflect.DeepEqual(action, decoded) {
			t.Fatal(decoded, err)
		}
	}
	for _, sides := range [][]string{{"own", "own"}, {"other"}, {""}} {
		data, _ := json.Marshal(SelectAction{Kind: "select", LeaderSides: sides})
		if _, err := decodeAction(data); err == nil {
			t.Fatal("accepted invalid leader selection", sides)
		}
	}
}
