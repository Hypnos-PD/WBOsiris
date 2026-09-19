package runner

import (
	"testing"

	"wbo/internal/ir"
)

// 视角契约：自己能看到自己牌组剩下的卡（多重集），对手看不到。
func TestPlayerViewExposesOwnDeckRemainingOnly(t *testing.T) {
	cards := map[int]*ir.Card{10001110: {ID: 10001110}}
	first := &instance{id: "1", card: cards[10001110], zone: "deck"}
	second := &instance{id: "2", card: cards[10001110], zone: "deck"}
	player := &player{deck: []*instance{first, second}}

	own := playerView(player, true, 1, "own", "own", cards)
	if len(own.DeckRemaining) != 2 {
		t.Fatalf("自己的牌组剩余张数应为 2，实际 %d", len(own.DeckRemaining))
	}
	if own.DeckRemaining[0] != 10001110 || own.DeckRemaining[1] != 10001110 {
		t.Fatalf("牌组剩余应逐张给出卡牌 ID，实际 %v", own.DeckRemaining)
	}
	hidden := playerView(player, false, 1, "oppo", "own", cards)
	if len(hidden.DeckRemaining) != 0 {
		t.Fatalf("对手视角不能看到牌组内容，实际 %v", hidden.DeckRemaining)
	}
	if hidden.DeckCount != 2 {
		t.Fatalf("对手仍应看到牌组张数，实际 %d", hidden.DeckCount)
	}
}
