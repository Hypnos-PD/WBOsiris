package runner

import (
	"testing"

	"wbo/internal/engine/ir"
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

// 顺序契约：外发的必须是**多重集**，不能带上抽牌顺序。
//
// 牌库顺序就是"接下来会抽到什么"——连自己都不知道。以前这里是照 deck 的遍历顺序发的，
// 于是谁拿到 view 谁就知道未来几张牌（训练侧只按多重集分桶，所以一直没暴露；
// 客户端拿它做提示就是作弊）。这条钉住"顺序被打掉、张数与内容不变"。
func TestPlayerViewDeckRemainingHasNoDrawOrder(t *testing.T) {
	ids := []int{10003110, 10001110, 10002110, 10001110}
	cards := map[int]*ir.Card{}
	deck := make([]*instance, 0, len(ids))
	for index, id := range ids {
		if cards[id] == nil {
			cards[id] = &ir.Card{ID: id}
		}
		deck = append(deck, &instance{id: string(rune('a' + index)), card: cards[id], zone: "deck"})
	}
	subject := &player{deck: deck}
	view := playerView(subject, true, 1, "own", "own", cards)
	if len(view.DeckRemaining) != len(ids) {
		t.Fatalf("张数必须保持：期望 %d，实际 %d", len(ids), len(view.DeckRemaining))
	}
	for index := 1; index < len(view.DeckRemaining); index++ {
		if view.DeckRemaining[index-1] > view.DeckRemaining[index] {
			t.Fatalf("外发的牌组必须是排序后的多重集（不能是抽牌顺序），实际 %v", view.DeckRemaining)
		}
	}
	// 多重集（内容与张数）必须与牌库一致。
	counts := map[int]int{}
	for _, item := range view.DeckRemaining {
		counts[item]++
	}
	want := map[int]int{10001110: 2, 10002110: 1, 10003110: 1}
	for id, count := range want {
		if counts[id] != count {
			t.Fatalf("多重集对不上：%d 期望 %d 张，实际 %d（%v）", id, count, counts[id], view.DeckRemaining)
		}
	}
	// 重新洗牌后的同一副牌必须得到**同一个** view（顺序不再泄漏任何信息）。
	shuffled := []*instance{deck[2], deck[0], deck[3], deck[1]}
	reshuffled := &player{deck: shuffled}
	other := playerView(reshuffled, true, 1, "own", "own", cards)
	if len(other.DeckRemaining) != len(view.DeckRemaining) {
		t.Fatalf("张数不一致")
	}
	for index := range view.DeckRemaining {
		if view.DeckRemaining[index] != other.DeckRemaining[index] {
			t.Fatalf("同一副牌的不同顺序必须给出同一个 view，实际 %v vs %v",
				view.DeckRemaining, other.DeckRemaining)
		}
	}
}
