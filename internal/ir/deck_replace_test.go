package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDeckReplaceAndLeaderMaxLifeRoundTrip(t *testing.T) {
	base := NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}
	for _, effect := range []Effect{
		DeckReplaceEffect{NodeBase: base, Kind: "replace_deck", Owner: "own", Cards: []DeckEntry{{CardID: 12345678, Count: 3}, {CardID: 23456789, Count: 1}}},
		LeaderMaxLifeEffect{NodeBase: base, Kind: "set_leader_max_life", Side: "oppo", Amount: 1},
	} {
		data, err := json.Marshal(effect)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeEffect(data, map[string]bool{})
		if err != nil || !reflect.DeepEqual(effect, got) {
			t.Fatal(got, err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		badFields := []struct{ key, value string }{{"side", `"both"`}, {"amount", "0"}, {"amount", "65536"}, {"amount", "null"}}
		if effect.effectKind() == "replace_deck" {
			badFields = []struct{ key, value string }{
				{"owner", `"all"`}, {"cards", `[]`}, {"cards", `null`},
				{"cards", `[{"cardId":12345678,"count":0}]`},
				{"cards", `[{"cardId":12345678,"count":65535},{"cardId":23456789,"count":1}]`},
				{"cards", `[{"cardId":12,"count":1}]`},
				{"cards", `[{"cardId":12345678,"count":1,"extra":true}]`},
			}
		}
		for _, tc := range badFields {
			old := fields[tc.key]
			fields[tc.key] = json.RawMessage(tc.value)
			bad, _ := json.Marshal(fields)
			if _, err := decodeEffect(bad, map[string]bool{}); err == nil {
				t.Fatal("accepted malformed effect", string(bad))
			}
			fields[tc.key] = old
		}
	}
	card := Card{ID: 12345678, CardType: "spell", PlayEffects: []Effect{DeckReplaceEffect{Kind: "replace_deck", Owner: "own", Cards: []DeckEntry{{CardID: 23456789, Count: 1}}}}}
	if validateCardRefs(card, map[int]bool{12345678: true}, nil) == nil {
		t.Fatal("missing recipe dependency accepted")
	}
	if err := validateCardRefs(card, map[int]bool{12345678: true, 23456789: true}, nil); err != nil {
		t.Fatal(err)
	}
}
