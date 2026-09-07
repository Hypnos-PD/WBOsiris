package ir

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func testOrigin() Origin {
	return Origin{Primary: SourceSpan{
		SourceID: strings.Repeat("a", 32), StartByte: 0, EndByte: 1,
		StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 2,
	}}
}

func TestFilteredTargetEffectsRoundTrip(t *testing.T) {
	for _, kind := range []string{"buff_stats", "damage", "destroy", "banish"} {
		t.Run(kind, func(t *testing.T) {
			effect := TargetEffect{
				NodeBase: NodeBase{ID: strings.Repeat("b", 32), Origin: testOrigin()}, Kind: kind,
				Target:    ZoneRef{Kind: "zone", Side: "own", Zone: "hand", Member: "follower"},
				Predicate: FieldPredicate{Kind: "has_trait", Trait: "puppetry"},
			}
			if kind == "buff_stats" {
				effect.AttackDelta, effect.LifeDelta = 1, -2
			} else if kind == "damage" {
				effect.Amount, effect.DamageType = 2, "effect"
			}
			encoded, err := json.Marshal(effect)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodeEffect(encoded, map[string]bool{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(effect, decoded) {
				t.Fatalf("filtered effect changed on round trip: want=%#v got=%#v", effect, decoded)
			}
		})
	}
}

func TestEncodeCardPackCanonicalRoundTrip(t *testing.T) {
	pack := CardPack{
		Format: Format, ContainerVersion: ContainerVersion, Encoding: Encoding,
		Kind: "card-pack", IRVersion: IRVersion, SourceLanguageVersion: SourceLanguageVersion,
		RequiredFeatures: []string{}, UnresolvedReferences: []string{},
		Sources: []Source{}, Cards: []Card{}, ContentHash: "sha256:stale",
	}
	a, err := EncodeCardPack(pack)
	if err != nil {
		t.Fatal(err)
	}
	b, err := EncodeCardPack(pack)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("typed card-pack encoding is not deterministic")
	}
	if !bytes.HasSuffix(a, []byte("\n")) {
		t.Fatal("encoded WBOS does not end with a newline")
	}
	if !bytes.HasPrefix(a, []byte("{\n  \"cards\": []")) {
		t.Fatalf("top-level keys are not canonical: %s", a)
	}
	decoded, err := DecodeCardPack(a)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Kind != pack.Kind || len(decoded.Cards) != 0 || !strings.HasPrefix(decoded.ContentHash, "sha256:") {
		t.Fatalf("bad typed round trip: %#v", decoded)
	}
}

func TestEncodeTestPackCanonicalRoundTrip(t *testing.T) {
	pack := TestPack{
		Format: Format, ContainerVersion: ContainerVersion, Encoding: Encoding,
		Kind: "test-pack", IRVersion: IRVersion,
		RequiredFeatures: []string{}, UnresolvedReferences: []string{},
		Ruleset: RulesetDependency{
			ID: "test-ruleset", ContentHash: "sha256:" + strings.Repeat("0", 64),
			RNG: RNGPolicy{Algorithm: "splitmix64", Version: "1"},
			OrderingPolicy: OrderingPolicy{
				Collections: "a", ReplacementAbilities: "b",
				SimultaneousTriggers: "c", DeathBatchLastwords: "d",
			},
			ExecutionBudget: ExecutionBudgetPolicy{
				Instructions: 1, QueryVisits: 2, StackDepth: 3, Candidates: 4,
				Events: 5, Triggers: 6, CreatedInstances: 7, ContinuationBytes: 8,
			},
		},
		Sources: []Source{}, Scenarios: []Scenario{},
	}
	a, err := EncodeTestPack(pack)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeTestPack(a)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Ruleset != pack.Ruleset || len(decoded.Scenarios) != 0 {
		t.Fatalf("bad typed test-pack round trip: %#v", decoded)
	}

	pack.ContentHash = "sha256:" + strings.Repeat("f", 64)
	b, err := EncodeTestPack(pack)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("a stale package hash affected encoded output")
	}
}

func TestTaggedUnionEncodingPreservesRequiredZeroValues(t *testing.T) {
	base := NodeBase{ID: strings.Repeat("b", 32), Origin: testOrigin()}
	self := SelfRef{Kind: "self", ValueType: "entity"}
	effects := []Effect{
		DrawEffect{NodeBase: base, Kind: "draw", Owner: "own", SourceZone: "deck", Count: 0, Output: "drawn"},
		CardEffect{NodeBase: base, Kind: "add_card", Owner: "own", Count: 0, CardID: 12345678, Destination: "hand"},
		TargetEffect{NodeBase: base, Kind: "damage", DamageType: "effect", Target: self, Amount: 0},
		AdjustEffect{NodeBase: base, Kind: "adjust_entity_field", Field: "cost", Target: self, Delta: 0, Minimum: 0},
	}
	for _, effect := range effects {
		data, err := json.Marshal(effect)
		if err != nil {
			t.Fatalf("marshal %s: %v", EffectKind(effect), err)
		}
		if _, err := decodeEffect(data, map[string]bool{}); err != nil {
			t.Fatalf("decode %s: %v\n%s", EffectKind(effect), err, data)
		}
	}

	action, err := json.Marshal(SourceAction{Kind: "end_turn", Actor: "own"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeAction(action); err != nil {
		t.Fatalf("decode end_turn: %v\n%s", err, action)
	}

	predicate, err := json.Marshal(FieldPredicate{Kind: "compare", Field: "life", Op: "eq", Value: 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodePredicate(predicate); err != nil {
		t.Fatalf("decode zero predicate: %v\n%s", err, predicate)
	}

	assertion, err := json.Marshal(ZoneAssertion{Kind: "zone_count", Side: "own", Zone: "hand", CardID: 12345678, Op: "eq", Count: 0, Origin: testOrigin()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeAssertion(assertion); err != nil {
		t.Fatalf("decode zero assertion: %v\n%s", err, assertion)
	}
}
