package ir

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeRejectsHistoryEffectTargetsButAcceptsCounts(t *testing.T) {
	history := ZoneRef{Kind: "zone", Side: "own", Zone: "destroyed", Member: "card"}
	base := NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}
	for _, effect := range []Effect{
		TargetEffect{NodeBase: base, Kind: "return", Target: history, Destination: "hand"},
		TargetEffect{NodeBase: base, Kind: "damage", DamageType: "effect", Target: FilterRef{Kind: "filter", Source: history, Predicate: FieldPredicate{Kind: "has_type", CardType: "follower"}}, Amount: 2},
		SelectionEffect{NodeBase: base, Kind: "choose", Policy: "optional", Binding: "old", Source: history},
		AdjustEffect{NodeBase: base, Kind: "adjust_entity_field", Field: "cost", Target: history, Delta: -1},
	} {
		data, err := json.Marshal(effect)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeEffect(data, map[string]bool{}); err == nil || err.Error() != "destroyed history is read-only" {
			t.Fatal("history target was not rejected by the read-only check", string(data), err)
		}
	}
	effect := TargetEffect{NodeBase: base, Kind: "damage", DamageType: "effect", Target: LeaderRef{Kind: "leader", Side: "oppo"}, AmountExpr: &CountExpr{Kind: "count", Source: history}}
	data, err := json.Marshal(effect)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeEffect(data, map[string]bool{}); err != nil {
		t.Fatal("rejected history count", err)
	}
}
