package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSumRoundTripAndInvalidShapes(t *testing.T) {
	for _, field := range []string{"base_attack", "base_life", "base_cost", "attack", "life", "cost"} {
		expr := &SumExpr{Kind: "sum", Field: field, Source: HistoryRef{Kind: "history", Side: "oppo", Window: "this_turn", Member: "follower"}}
		data, _ := json.Marshal(expr)
		_, decoded, err := decodeEffectAmount(data)
		if err != nil || !reflect.DeepEqual(expr, decoded) {
			t.Fatal(err, decoded)
		}
	}
	for _, raw := range []string{
		`{"kind":"sum","source":{"kind":"leader","side":"own"},"field":"base_attack"}`,
		`{"kind":"sum","source":{"kind":"zone","side":"own","zone":"hand"},"field":"missing"}`,
		`{"kind":"sum","source":{"kind":"zone","side":"own","zone":"hand"},"field":"base_attack","value":1}`,
		`{"kind":"count","source":{"kind":"history","side":"own","window":"last_turn"}}`,
		`{"kind":"count","source":{"kind":"history","side":"both","window":"this_turn"}}`,
	} {
		if _, _, err := decodeEffectAmount([]byte(raw)); err == nil {
			t.Fatal("accepted invalid aggregate", raw)
		}
	}
	e := SelectionEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "choose", Policy: "optional", Binding: "old", Source: HistoryRef{Kind: "history", Side: "own", Window: "this_turn"}}
	data, _ := json.Marshal(e)
	if _, err := decodeEffect(data, map[string]bool{}); err == nil {
		t.Fatal("current-turn history became selectable")
	}
}
