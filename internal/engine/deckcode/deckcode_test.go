package deckcode

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/engine/project"
	"wbo/internal/engine/runner"
)

// 编码规则要和官网 hash 一致：格式.职业.卡牌base64…
func TestEncodeMatchesOfficialHashShape(t *testing.T) {
	// 10001110 在官方字母表里是 c9hM（10001110 = 38·64³ + 9·64² + 43·64 + 22）。
	code, err := Encode(runner.FormatRotation, "neutral", []int{10001110, 10001110, 10001111})
	if err != nil {
		t.Fatal(err)
	}
	if code != "1.0.c9hM.c9hM.c9hN" {
		t.Fatalf("code = %q", code)
	}
	unlimited, err := Encode(runner.FormatUnlimited, "swordcraft", []int{10001110})
	if err != nil {
		t.Fatal(err)
	}
	if unlimited != "2.2.c9hM" {
		t.Fatalf("unlimited code = %q", unlimited)
	}
}

func TestRoundTrip(t *testing.T) {
	deck := []int{10001110, 10001120, 10002120, 10001110, 70000123}
	class := ClassForDeck(deck, func(id int) string { return "forestcraft" })
	if class != "forestcraft" {
		t.Fatalf("class = %q", class)
	}
	code, err := Encode(runner.FormatRotation, class, deck)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(code)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{10001110, 10001110, 10001120, 10002120, 70000123}
	if decoded.Format != runner.FormatRotation || decoded.Class != "forestcraft" || !reflect.DeepEqual(decoded.Cards, want) {
		t.Fatalf("decoded = %#v, want %v", decoded, want)
	}
}

// 官方卡组详情链接里的 hash 也要能直接解析。
func TestDecodeAcceptsOfficialDeckLink(t *testing.T) {
	decoded, err := Decode("https://shadowverse-wb.com/chs/deck/detail/?hash=1.1.c9hM.c9hN&x=1")
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Format != runner.FormatRotation || decoded.Class != "forestcraft" {
		t.Fatalf("decoded = %#v", decoded)
	}
	if !reflect.DeepEqual(decoded.Cards, []int{10001110, 10001111}) {
		t.Fatalf("cards = %v", decoded.Cards)
	}
}

func TestDecodeRejectsMalformedCodes(t *testing.T) {
	for name, code := range map[string]string{
		"empty":      "",
		"single":     "1",
		"bad format": "9.1.2vG",
		"bad class":  "1.9.2vG",
		"bad chars":  "1.1.***",
		"bad card":   "1.1.0",
		"too many":   "1.1." + strings.Repeat("c9hM.", 300) + "c9hM",
	} {
		if _, err := Decode(code); err == nil {
			t.Fatalf("%s: malformed code accepted", name)
		}
	}
}

// 真实卡池的往返：用默认练习卡组编码再解码，结果必须一致且仍然是合法构筑。
func TestPracticeDeckRoundTrip(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	cards, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	classOf := func(id int) string {
		for n := range cards.Cards {
			if cards.Cards[n].ID == id {
				return cards.Cards[n].Meta.Class
			}
		}
		return ""
	}
	deck := runner.PracticeDeck()
	code, err := Encode(runner.FormatRotation, ClassForDeck(deck, classOf), deck)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(code)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Cards) != 40 {
		t.Fatalf("decoded %d cards", len(decoded.Cards))
	}
	format, err := runner.FormatByID(cards, decoded.Format)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.ValidateDeckForFormat(cards, decoded.Cards, format); err != nil {
		t.Fatalf("decoded deck is not legal: %v", err)
	}
}
