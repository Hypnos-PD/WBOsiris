package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRepeatRoundTripAndReferences(t *testing.T) {
	for _, expr := range []NumericExpr{nil, &Scalar{Kind: "scalar", Side: "own", Field: "combo"}, &CountExpr{Kind: "count", Source: ZoneRef{Kind: "zone", Side: "own", Zone: "hand", Member: "follower"}}} {
		e := RepeatEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "repeat", TimesExpr: expr, Body: []Effect{
			CardEffect{NodeBase: NodeBase{ID: strings.Repeat("b", 32), Origin: testOrigin()}, Kind: "summon", Owner: "own", CardID: 23456789, Count: 1, Output: "summoned"},
		}}
		if expr == nil {
			e.Times = 2
		}
		data, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeEffect(data, map[string]bool{})
		if err != nil || !reflect.DeepEqual(decoded, e) {
			t.Fatalf("repeat changed: %#v %v", decoded, err)
		}
		card := Card{ID: 12345678, CardType: "follower", PlayEffects: []Effect{decoded}}
		if err := validateCardRefs(card, map[int]bool{12345678: true}); err == nil {
			t.Fatal("repeat body hid unresolved card")
		}
		if err := validateCardRefs(card, map[int]bool{12345678: true, 23456789: true}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRepeatRejectsInvalidTimesAndDuplicateNodes(t *testing.T) {
	e := RepeatEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "repeat", Times: 2, Body: []Effect{}}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	for _, times := range []string{`null`, `-1`, `1.5`, `"2"`, `{"kind":"negate","value":{"kind":"scalar","side":"own","field":"combo"}}`} {
		object["times"] = json.RawMessage(times)
		raw, _ := json.Marshal(object)
		if _, err := decodeEffect(raw, map[string]bool{}); err == nil {
			t.Fatalf("accepted times %s", times)
		}
	}
	e.Body = []Effect{RepeatEffect{NodeBase: e.NodeBase, Kind: "repeat", Times: 1, Body: []Effect{}}}
	data, _ = json.Marshal(e)
	if _, err := decodeEffect(data, map[string]bool{}); err == nil {
		t.Fatal("accepted duplicate nested node ID")
	}
	e.TimesExpr = &Scalar{Kind: "scalar", Side: "own", Field: "combo"}
	if _, err := json.Marshal(e); err == nil {
		t.Fatal("accepted conflicting repeat count")
	}
}
