package runner

import (
	"testing"

	"wbo/internal/ir"
)

// 主战者的【屏障】：下一次受到的伤害降为 0，然后消耗掉（官方 QA：与"受到伤害+1"同时存在时也是 0）。
func TestLeaderBarrierAbsorbsTheNextDamage(t *testing.T) {
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 70000001, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}}}}
	state := testState()
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	g := session.g
	before := g.own.leaderLife
	if g.own.leaderAbilities["barrier"] {
		t.Fatal("leader started with barrier")
	}
	g.setLeaderKeyword([]string{"own"}, "barrier", true)
	if got := g.damageLeaderFrom(nil, &g.own, "own", 5); got != 0 || g.own.leaderLife != before {
		t.Fatalf("barrier did not absorb: dealt=%d life=%d", got, g.own.leaderLife)
	}
	if g.own.leaderAbilities["barrier"] {
		t.Fatal("barrier was not consumed")
	}
	if got := g.damageLeaderFrom(nil, &g.own, "own", 5); got != 5 || g.own.leaderLife != before-5 {
		t.Fatalf("second hit was wrong: dealt=%d life=%d", got, g.own.leaderLife)
	}
}
