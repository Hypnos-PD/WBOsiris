package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCountAmountRoundTrip(t *testing.T) {
	for _, kind := range []string{"damage", "heal"} {
		effect := TargetEffect{NodeBase: NodeBase{ID: strings.Repeat("b", 32), Origin: testOrigin()},
			Kind: kind, Target: LeaderRef{Kind: "leader", Side: "own", ValueType: "leader"},
			AmountExpr: &CountExpr{Kind: "count", Source: FilterRef{Kind: "filter",
				Source:    ZoneRef{Kind: "zone", Side: "own", Zone: "hand", Member: "follower"},
				Predicate: FieldPredicate{Kind: "has_trait", Trait: "golem"},
			}},
		}
		if kind == "damage" {
			effect.DamageType = "effect"
		}
		data, err := json.Marshal(effect)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeEffect(data, map[string]bool{})
		if err != nil || !reflect.DeepEqual(effect, decoded) {
			t.Fatalf("count effect changed: got=%#v err=%v", decoded, err)
		}
		effect.Amount = 1
		if _, err := json.Marshal(effect); err == nil {
			t.Fatal("encoded conflicting literal and count amounts")
		}
		effect.Amount, effect.Kind = 0, "set_attack_limit"
		if _, err := json.Marshal(effect); err == nil {
			t.Fatal("encoded count for unsupported effect")
		}
	}
}

func TestRejectMalformedIRAmounts(t *testing.T) {
	for _, amount := range []string{
		`null`, ` null `, `-1`, `1.5`, `"1"`, `true`, `[]`, `{}`, `{"kind":"value","value":1}`,
		`{"kind":"count"}`, `{"kind":"count","source":null}`,
		`{"kind":"count","source":{"kind":"leader","side":"own","valueType":"leader"}}`,
		`{"kind":"count","source":{"kind":"binding","name":""}}`,
		`{"kind":"count","source":{"kind":"zone","zone":"hand"}}`,
		`{"kind":"count","source":{"kind":"zone","side":"own","zone":"unknown"}}`,
		`{"kind":"count","source":{"kind":"zone","side":"own","zone":"hand"},"extra":1}`,
		`{"kind":"count","source":{"kind":"filter","source":{"kind":"self"},"predicate":{"kind":"has_trait","trait":"golem"}}}`,
	} {
		t.Run(amount, func(t *testing.T) {
			if _, _, err := decodeEffectAmount(json.RawMessage(amount)); err == nil {
				t.Fatal("accepted malformed amount")
			}
		})
	}
	base := TargetEffect{NodeBase: NodeBase{ID: strings.Repeat("b", 32), Origin: testOrigin()},
		Kind: "set_attack_limit", Amount: 1, Target: SelfRef{Kind: "self", ValueType: "entity"}}
	data, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	object["amount"] = json.RawMessage(`{"kind":"count","source":{"kind":"zone","side":"own","zone":"hand"}}`)
	data, err = json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeEffect(data, map[string]bool{}); err == nil {
		t.Fatal("decoded count for unsupported effect")
	}
}

func TestCountPredicateCardReferencesMustExist(t *testing.T) {
	for _, known := range []bool{false, true} {
		cards := map[int]bool{12345678: true}
		if known {
			cards[23456789] = true
		}
		card := Card{ID: 12345678, PlayEffects: []Effect{
			TargetEffect{Kind: "heal", Target: LeaderRef{Kind: "leader", Side: "own", ValueType: "leader"},
				AmountExpr: &CountExpr{Kind: "count", Source: FilterRef{Kind: "filter",
					Source: ZoneRef{Kind: "zone", Side: "own", Zone: "hand"},
					Predicate: AndPredicate{Kind: "and", Terms: []Predicate{
						FieldPredicate{Kind: "has_type", CardType: "follower"},
						FieldPredicate{Kind: "has_card", CardID: 23456789},
					}},
				}},
			},
		}}
		if err := validateCardRefs(card, cards, nil); (err == nil) != known {
			t.Fatalf("known=%v err=%v", known, err)
		}
	}
}
