package runner

import (
	"strings"
	"testing"

	"wbo/internal/engine/ir"
)

// 【奥义】/【解放奥义】的奥义槽 = 当前回合数 + 本卡牌在手牌中时己方随从的进化次数
// （官方术语表）。这里验证"随从进化给手牌累加奥义槽"，再用真实卡牌验证阈值。
func TestSkyboundArtGaugeCountsEvolutionsInHand(t *testing.T) {
	pack := loadCardsForTest(t,
		"10004/10414110", "10004/10471120", "10000/10001110")
	heldID, firstID, secondID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	state := testState()
	state.Turn.Active, state.Turn.Number = "own", 8
	own := state.Players["own"]
	own.PP, own.MaxPP = 4, 4
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: heldID, CardID: 10414110, DeclaredType: "follower"})
	own = withInstance(own, "field", ir.TestInstance{InstanceID: firstID, CardID: 10001110, DeclaredType: "follower"})
	own = withInstance(own, "field", ir.TestInstance{InstanceID: secondID, CardID: 10001110, DeclaredType: "follower"})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := session.g.instances[heldID].skybound; got != 0 {
		t.Fatalf("gauge started at %d", got)
	}
	// 两次能力进化让手牌里的卡牌各记 1 点奥义槽；8 回合 + 2 = 10 →【奥义】成立。
	session.g.execTargetEffect(ir.TargetEffect{Kind: "silent_evolve", Target: ir.BindingRef{Kind: "binding", Name: "targets"}, Form: "evolved"}, session.g.instances[firstID], frame{"targets": bindEntities(session.g.instances[firstID])})
	session.g.execTargetEffect(ir.TargetEffect{Kind: "silent_evolve", Target: ir.BindingRef{Kind: "binding", Name: "targets"}, Form: "evolved"}, session.g.instances[secondID], frame{"targets": bindEntities(session.g.instances[secondID])})
	if got := session.g.instances[heldID].skybound; got != 2 {
		t.Fatalf("gauge = %d, want 2", got)
	}
	if !session.g.condition(ir.SkyboundArtCondition{Kind: "skybound_art", Level: 10}, session.g.instances[heldID]) {
		t.Fatal("skybound art should be active at 8 turns + 2 evolutions")
	}
	if session.g.condition(ir.SkyboundArtCondition{Kind: "skybound_art", Level: 15}, session.g.instances[heldID]) {
		t.Fatal("super skybound art must not be active before 15")
	}
}
