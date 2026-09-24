package runner

import (
	"testing"

	"wbo/internal/engine/ir"
)

// "胜利的卡牌"：牌组耗尽时改为胜利（10304110 的纹章）。
func TestEmptyDeckOutcomeVictoryWinsInsteadOfLosing(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	state.Turn.Active = "own"
	owner := state.Players["own"]
	owner.Leader = ir.Leader{Life: 20, MaxLife: 20}
	state.Players["own"] = owner
	s, err := NewSession(pack, state, 3)
	if err != nil {
		t.Fatal(err)
	}
	// 常规规则：牌组为空时抽牌即败北。
	s.g.firstPlayer = "own"
	s.g.draw(ir.DrawEffect{Kind: "draw", Owner: "own", Count: 1}, nil, frame{})
	if !s.g.gameOver || s.g.winner != "oppo" {
		t.Fatalf("empty deck did not lose: over=%t winner=%s", s.g.gameOver, s.g.winner)
	}
	// 拥有"胜利的卡牌"时改为自己胜利。
	session, err := NewSession(pack, state, 3)
	if err != nil {
		t.Fatal(err)
	}
	session.g.emit(ir.RuntimeEvent{Kind: "empty_deck_outcome_set", Side: "own"})
	session.g.firstPlayer = "own"
	session.g.own.deckOutcome = "victory"
	session.g.draw(ir.DrawEffect{Kind: "draw", Owner: "own", Count: 1}, nil, frame{})
	if !session.g.gameOver || session.g.winner != "own" {
		t.Fatalf("victory card did not win: over=%t winner=%s", session.g.gameOver, session.g.winner)
	}
}
