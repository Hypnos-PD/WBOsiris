package ir

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestKeywordDurationRejectsUnsupportedEffects(t *testing.T) {
	e := TargetEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "add_keyword", Keyword: "storm", Target: SelfRef{Kind: "self"}, Until: "oppo_turn_end"}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ field, value string }{{"until", `"forever"`}, {"kind", `"remove_keyword"`}, {"kind", `"damage"`}, {"until", `1`}} {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatal(err)
		}
		raw[tc.field] = json.RawMessage(tc.value)
		bad, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeEffect(bad, map[string]bool{}); err == nil {
			t.Fatal("accepted malformed duration", tc)
		}
	}
	e.Kind = "remove_keyword"
	if _, err := json.Marshal(e); err == nil {
		t.Fatal("encoded unsupported temporary removal")
	}
}
