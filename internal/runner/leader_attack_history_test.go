package runner

import (
	"strings"
	"testing"

	"wbo/internal/ir"
)

// S-87：`own.attacked_leader_last_turn` 在进入自己的回合时结转本回合是否攻击过主战者，
// 随后清空本回合标记。
func TestLeaderAttackHistoryRollsIntoTheNextOwnTurn(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		state := crestState(side)
		state.FirstPlayer = side
		attacker := strings.Repeat("1", 32)
		// 10962120 奇迹独角兔自带【疾驰】，当回合就能攻击主战者。
		p := withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: attacker, CardID: 10962120, DeclaredType: "follower"})
		state.Players[side] = p
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		// Defender 为空表示攻击主战者。
		if r := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "attack", Source: attacker}); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		if !s.g.player(side).leaderAttackedThisTurn || s.g.player(side).leaderAttackedLastTurn {
			t.Fatal("leader attack was not recorded for the current turn")
		}
		// 结束双方各一个回合：回到自己的回合开始时结转。
		crestEndTurn(t, s)
		crestEndTurn(t, s)
		if !s.g.player(side).leaderAttackedLastTurn || s.g.player(side).leaderAttackedThisTurn {
			t.Fatal("leader attack history did not roll over")
		}
		restored, err := restoreGame(s.g.cards, snapshotContinuationGame(s.g))
		if err != nil || !restored.player(side).leaderAttackedLastTurn {
			t.Fatal("restored history lost the rolled-over flag", err)
		}
	}
}
