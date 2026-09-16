package runner

import (
	"strconv"
	"strings"
	"testing"

	"wbo/internal/ir"
)

// 「对手的随从超进化时」的手牌触发：本回合由对手行动，场景测试只能由 own 行动，
// 所以这类卡用 Go 测试验证（对着手牌的实例断言获得的关键词）。
func TestHandTriggersOnOpponentSuperEvolution(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cardID  int
		keyword string
	}{
		{"抗拒叹息之人获得毁灭", 10302110, "bane"},
		{"充满勇气之人获得疾驰", 10303110, "storm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pack := loadCardsForTest(t, "10003/"+strconv.Itoa(tc.cardID), "10000/10001110")
			heldID, evolvedID := strings.Repeat("1", 32), strings.Repeat("2", 32)
			state := testState()
			state.Turn.Active, state.Turn.Number = "oppo", 7
			own := state.Players["own"]
			own = withInstance(own, "hand", ir.TestInstance{InstanceID: heldID, CardID: tc.cardID, DeclaredType: "follower"})
			state.Players["own"] = own
			oppo := state.Players["oppo"]
			oppo.SEP = 1
			oppo = withInstance(oppo, "field", ir.TestInstance{InstanceID: evolvedID, CardID: 10001110, DeclaredType: "follower"})
			state.Players["oppo"] = oppo
			session, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			if held := session.g.instances[heldID]; held.abilities[tc.keyword] {
				t.Fatalf("%s was already present", tc.keyword)
			}
			if result := session.SubmitAs(strings.Repeat("a", 32), "oppo", SimulatorCommand{Kind: "superevolve", Source: evolvedID}); result.Status != StatusCompleted {
				t.Fatalf("opponent super evolution failed: %#v", result)
			}
			if held := session.g.instances[heldID]; !held.abilities[tc.keyword] {
				t.Fatalf("hand card did not gain %s: %#v", tc.keyword, held.abilities)
			}
		})
	}
}
