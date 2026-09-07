package ir_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/project"
)

func cardPackObject(t *testing.T) map[string]any {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, true, root)
	data, err := project.Compile(loaded, false)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	return object
}

func encodeCardPack(t *testing.T, object map[string]any) []byte {
	t.Helper()
	object["contentHash"] = ""
	canonical, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	object["contentHash"] = "sha256:" + hex.EncodeToString(sum[:])
	data, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testPackObject(t *testing.T) map[string]any {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "tests")}, true, root)
	data, err := project.Compile(loaded, false)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if json.Unmarshal(data, &object) != nil {
		t.Fatal("invalid test fixture")
	}
	return object
}

func encodeTestPack(t *testing.T, object map[string]any) []byte {
	t.Helper()
	object["contentHash"] = ""
	canonical, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	object["contentHash"] = "sha256:" + hex.EncodeToString(sum[:])
	data, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDecodeAllCards(t *testing.T) {
	pack, err := ir.DecodeCardPack(encodeCardPack(t, cardPackObject(t)))
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Cards) != 243 {
		t.Fatalf("cards=%d", len(pack.Cards))
	}
}

func TestDecodeRejectsUnknownCardTrait(t *testing.T) {
	object := cardPackObject(t)
	card := object["cards"].([]any)[0].(map[string]any)
	card["traits"] = []any{"bat"}
	if _, err := ir.DecodeCardPack(encodeCardPack(t, object)); err == nil {
		t.Fatal("card pack accepted an unknown trait")
	}
}

func TestDecodeAttackRestrictionKeywords(t *testing.T) {
	object := cardPackObject(t)
	cards := object["cards"].([]any)
	card := cards[0].(map[string]any)
	card["intrinsic"] = []any{"cannot_attack", "cannot_attack_follower", "cannot_attack_leader"}
	pack, err := ir.DecodeCardPack(encodeCardPack(t, object))
	if err != nil {
		t.Fatal(err)
	}
	if got := pack.Cards[0].Intrinsic; len(got) != 3 || got[0] != "cannot_attack" || got[1] != "cannot_attack_follower" || got[2] != "cannot_attack_leader" {
		t.Fatalf("attack restrictions were not decoded: %#v", got)
	}
}

func TestStrictCardPackRejections(t *testing.T) {
	t.Run("duplicate top-level key", func(t *testing.T) {
		if _, err := ir.DecodeCardPack([]byte(`{"format":"wbos","format":"wbos"}`)); err == nil {
			t.Fatal("accepted duplicate top-level key")
		}
	})
	t.Run("duplicate nested key", func(t *testing.T) {
		if _, err := ir.DecodeCardPack([]byte(`{"format":"wbos","nested":{"kind":"a","kind":"b"}}`)); err == nil {
			t.Fatal("accepted duplicate nested key")
		}
	})
	t.Run("trailing JSON", func(t *testing.T) {
		data := append(encodeCardPack(t, cardPackObject(t)), []byte(` {}`)...)
		if _, err := ir.DecodeCardPack(data); err == nil {
			t.Fatal("accepted trailing JSON")
		}
	})
	t.Run("invalid UTF-8", func(t *testing.T) {
		if _, err := ir.DecodeCardPack([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}); err == nil {
			t.Fatal("accepted invalid UTF-8")
		}
	})
	t.Run("oversized input", func(t *testing.T) {
		if _, err := ir.DecodeCardPack([]byte(strings.Repeat(" ", ir.MaxPackageSize+1))); err == nil {
			t.Fatal("accepted oversized input")
		}
	})
	t.Run("unknown field", func(t *testing.T) {
		o := cardPackObject(t)
		o["surprise"] = true
		if _, err := ir.DecodeCardPack(encodeCardPack(t, o)); err == nil {
			t.Fatal("accepted unknown field")
		}
	})
	t.Run("unknown executable kind", func(t *testing.T) {
		o := cardPackObject(t)
		var changed bool
		for _, raw := range o["cards"].([]any) {
			abilities := raw.(map[string]any)["abilities"].([]any)
			if len(abilities) == 0 {
				continue
			}
			body := abilities[0].(map[string]any)["body"].([]any)
			if len(body) == 0 {
				continue
			}
			body[0].(map[string]any)["kind"] = "future_effect"
			changed = true
			break
		}
		if !changed {
			t.Fatal("fixture has no executable effect")
		}
		if _, err := ir.DecodeCardPack(encodeCardPack(t, o)); err == nil {
			t.Fatal("accepted unknown executable kind")
		}
	})
	t.Run("unresolved executable pack", func(t *testing.T) {
		o := cardPackObject(t)
		o["unresolvedReferences"] = []any{"99999999"}
		if _, err := ir.DecodeCardPack(encodeCardPack(t, o)); err == nil {
			t.Fatal("accepted unresolved pack")
		}
	})
	t.Run("bad hash", func(t *testing.T) {
		data := encodeCardPack(t, cardPackObject(t))
		var o map[string]any
		_ = json.Unmarshal(data, &o)
		o["contentHash"] = "sha256:bad"
		bad, _ := json.Marshal(o)
		if _, err := ir.DecodeCardPack(bad); err == nil {
			t.Fatal("accepted malformed hash")
		}
	})
	t.Run("content hash mismatch", func(t *testing.T) {
		data := encodeCardPack(t, cardPackObject(t))
		var o map[string]any
		_ = json.Unmarshal(data, &o)
		o["contentHash"] = "sha256:" + strings.Repeat("0", 64)
		bad, _ := json.Marshal(o)
		if _, err := ir.DecodeCardPack(bad); err == nil {
			t.Fatal("accepted content hash mismatch")
		}
	})
	t.Run("bad card ID", func(t *testing.T) {
		o := cardPackObject(t)
		o["cards"].([]any)[0].(map[string]any)["id"] = 1
		if _, err := ir.DecodeCardPack(encodeCardPack(t, o)); err == nil {
			t.Fatal("accepted malformed card ID")
		}
	})
}

func findKind(value any, kind string) map[string]any {
	switch x := value.(type) {
	case map[string]any:
		if x["kind"] == kind {
			return x
		}
		for _, child := range x {
			if found := findKind(child, kind); found != nil {
				return found
			}
		}
	case []any:
		for _, child := range x {
			if found := findKind(child, kind); found != nil {
				return found
			}
		}
	}
	return nil
}

func TestEffectVariantInvariants(t *testing.T) {
	cases := []struct {
		name, kind string
		mutate     func(map[string]any)
	}{
		{"selection policy", "choose", func(v map[string]any) { v["policy"] = "random" }},
		{"transform preservation", "transform", func(v map[string]any) { v["preserveMaterials"] = false }},
		{"return insertion", "return", func(v map[string]any) { v["destination"] = "deck"; v["deckInsertion"] = "top" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := cardPackObject(t)
			v := findKind(o, tc.kind)
			if v == nil {
				t.Fatalf("fixture has no %s", tc.kind)
			}
			tc.mutate(v)
			if _, err := ir.DecodeCardPack(encodeCardPack(t, o)); err == nil {
				t.Fatalf("accepted malformed %s", tc.kind)
			}
		})
	}
	t.Run("ability relation", func(t *testing.T) {
		o := cardPackObject(t)
		var ability map[string]any
		for _, c := range o["cards"].([]any) {
			a := c.(map[string]any)["abilities"].([]any)
			if len(a) > 0 {
				ability = a[0].(map[string]any)
				break
			}
		}
		if ability == nil {
			t.Fatal("fixture has no ability")
		}
		ability["relation"] = "future"
		if _, err := ir.DecodeCardPack(encodeCardPack(t, o)); err == nil {
			t.Fatal("accepted malformed relation")
		}
	})
}

func TestRuntimePackCardLinking(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "tests")}, true, root)
	cards, tests, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	var referenced int
	for _, p := range tests.Scenarios[0].InitialState.Players {
		for _, zone := range p.Zones {
			if len(zone) > 0 {
				referenced = zone[0].CardID
				break
			}
		}
		if referenced != 0 {
			break
		}
	}
	filtered := cards.Cards[:0]
	for _, c := range cards.Cards {
		if c.ID != referenced {
			filtered = append(filtered, c)
		}
	}
	cards.Cards = filtered
	if err := ir.ValidateRuntimePacks(cards, tests); err == nil {
		t.Fatal("accepted unknown initial-state card")
	}
	cards, tests, err = project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	changed := false
	for si := range tests.Scenarios {
		for ai, a := range tests.Scenarios[si].Assertions {
			if z, ok := a.(ir.ZoneAssertion); ok && z.Kind == "zone_count" {
				z.CardID = 99999999
				tests.Scenarios[si].Assertions[ai] = z
				changed = true
				break
			}
		}
		if changed {
			break
		}
	}
	if !changed {
		t.Fatal("fixture has no zone_count assertion")
	}
	if err := ir.ValidateRuntimePacks(cards, tests); err == nil {
		t.Fatal("accepted unknown zone_count card")
	}
}

func TestTestPackRejectsUnknownInstanceReferences(t *testing.T) {
	t.Run("action", func(t *testing.T) {
		o := testPackObject(t)
		sc := o["scenarios"].([]any)[0].(map[string]any)
		sc["actions"].([]any)[0].(map[string]any)["source"] = strings.Repeat("f", 32)
		if _, err := ir.DecodeTestPack(encodeTestPack(t, o)); err == nil {
			t.Fatal("accepted unknown action instance")
		}
	})
	t.Run("assertion", func(t *testing.T) {
		o := testPackObject(t)
		var changed bool
		for _, raw := range o["scenarios"].([]any) {
			for _, a := range raw.(map[string]any)["assertions"].([]any) {
				m := a.(map[string]any)
				if m["kind"] == "has_keyword" {
					m["target"] = strings.Repeat("f", 32)
					changed = true
					break
				}
			}
			if changed {
				break
			}
		}
		if !changed {
			t.Fatal("fixture has no instance assertion")
		}
		if _, err := ir.DecodeTestPack(encodeTestPack(t, o)); err == nil {
			t.Fatal("accepted unknown assertion instance")
		}
	})
	t.Run("primary after response", func(t *testing.T) {
		o := testPackObject(t)
		sc := o["scenarios"].([]any)[0].(map[string]any)
		actions := sc["actions"].([]any)
		sc["actions"] = append([]any{map[string]any{"kind": "select", "target": strings.Repeat("f", 32)}}, actions...)
		if _, err := ir.DecodeTestPack(encodeTestPack(t, o)); err == nil {
			t.Fatal("accepted response as primary action")
		}
	})
}
