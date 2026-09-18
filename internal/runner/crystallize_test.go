package runner

import (
	"strings"
	"testing"

	"wbo/internal/ir"
)

// 【结晶】：以较低费用当作护符打出，卡面换成衍生护符；吟唱归零后按其谢幕曲召唤本体。
func TestCrystallizeCountsDownAndSummonsTheFollower(t *testing.T) {
	pack := repeatCardPack(t)
	state := crestState("own")
	card := strings.Repeat("1", 32)
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: card, CardID: 10661110, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if r := s.SubmitAs(strings.Repeat("a", 32), "own", SimulatorCommand{Kind: "crystallize", Source: card}); r.Status != StatusCompleted {
		t.Fatal(r)
	}
	i := s.g.instances[card]
	if i.zone != "field" || i.card.CardType != "amulet" || i.countdown != 3 {
		t.Fatalf("crystallize did not produce an amulet with countdown 3: %+v", i)
	}
	// 结界护符在控制者回合开始递减：结束三个回合后吟唱归零并结算谢幕曲。
	for n := 0; n < 6; n++ {
		crestEndTurn(t, s)
	}
	if s.g.instances[card].zone != "graveyard" {
		t.Fatalf("the crystallized amulet should have expired into the graveyard, got %s", s.g.instances[card].zone)
	}
	summoned := 0
	for _, field := range s.g.player("own").field {
		if field.card.ID == 10661110 {
			summoned++
		}
	}
	if summoned != 1 {
		t.Fatalf("expected one summoned follower after the countdown expired, got %d", summoned)
	}
}
