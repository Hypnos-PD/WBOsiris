package runner

import (
	"path/filepath"
	"reflect"
	"testing"

	"wbo/internal/engine/ir"
	"wbo/internal/engine/project"
)

func formatPack(cardID, pack int, class string) ir.Card {
	card := ir.Card{ID: cardID, CardType: "follower", Cost: 2, Stats: &ir.Stats{Attack: 2, Life: 2}}
	card.Meta.Pack, card.Meta.Class = pack, class
	return card
}

// 指定模式 = 基础卡牌 + 最新的 6 个已发布扩展包（与 WBArts 的 js/deck.js 一致）。
func TestRotationUsesCoreAndLatestSixExpansions(t *testing.T) {
	cards := &ir.CardPack{}
	for n := 0; n <= 8; n++ {
		cards.Cards = append(cards.Cards, formatPack(10000000+n, 10000+n, "neutral"))
	}
	cards.Cards = append(cards.Cards, formatPack(90000001, 90000, "neutral"))
	formats := Formats(cards)
	if len(formats) != 2 || formats[0].ID != FormatRotation || formats[1].ID != FormatUnlimited {
		t.Fatalf("unexpected formats: %#v", formats)
	}
	wantRotation := []int{10000, 10003, 10004, 10005, 10006, 10007, 10008}
	if !reflect.DeepEqual(formats[0].Packs, wantRotation) {
		t.Fatalf("rotation packs = %v, want %v", formats[0].Packs, wantRotation)
	}
	if len(formats[1].Packs) != 9 {
		t.Fatalf("unlimited packs = %v, want all nine published packs", formats[1].Packs)
	}
	// 附属卡包永远不在任何赛制里。
	for _, format := range formats {
		if format.AllowsPack(90000) {
			t.Fatalf("%s must not allow the token pack: %v", format.ID, format.Packs)
		}
	}
}

func TestValidateDeckForFormatRejectsOutOfRotationCards(t *testing.T) {
	cards := &ir.CardPack{}
	for n := 0; n <= 8; n++ {
		for k := 0; k < 3; k++ {
			cards.Cards = append(cards.Cards, formatPack(10000000+n*10+k, 10000+n, "swordcraft"))
		}
	}
	rotation, err := FormatByID(cards, FormatRotation)
	if err != nil {
		t.Fatal(err)
	}
	unlimited, _ := FormatByID(cards, FormatUnlimited)
	deck := make([]int, 0, 40)
	// 13 张指定模式内的卡各 3 张 = 39 张，再塞 1 张已轮换出去的卡。
	for n := 0; n < 13; n++ {
		pack := rotation.Packs[1+n/3] // 10000 之外的卡包
		id := 10000000 + (pack-10000)*10 + n%3
		for copy := 0; copy < 3; copy++ {
			deck = append(deck, id)
		}
	}
	old := 10000010 // pack 10001：已轮换出去（n=1 起每包 3 张、编号从 10000010 开始）
	deck = append(deck, old)
	if err := ValidateDeckForFormat(cards, deck, rotation); err == nil {
		t.Fatal("old pack card was accepted in rotation")
	}
	if err := ValidateDeckForFormat(cards, deck, unlimited); err != nil {
		t.Fatalf("unlimited rejected a published pack: %v", err)
	}
	// 结构错误（张数）仍然由结构校验报出来。
	if err := ValidateDeckForFormat(cards, deck[:20], unlimited); err == nil {
		t.Fatal("short deck was accepted")
	}
}

// 用真实卡池固定住当前规则：最新包是 10009，所以指定模式是 10000 + 10004–10009。
func TestRealCardPoolRotationPacks(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	cards, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	rotation, err := FormatByID(cards, FormatRotation)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{10000, 10004, 10005, 10006, 10007, 10008, 10009}
	if !reflect.DeepEqual(rotation.Packs, want) {
		t.Fatalf("rotation packs = %v, want %v", rotation.Packs, want)
	}
	if err := ValidateDeckForFormat(cards, PracticeDeck(), rotation); err != nil {
		t.Fatalf("practice deck must be rotation legal: %v", err)
	}
}
