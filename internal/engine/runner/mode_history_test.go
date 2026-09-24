package runner

import (
	"strings"
	"testing"

	"wbo/internal/engine/ir"
)

// S-74：`mode random N history <名字>` 只在尚未发动过的选项里随机；用过的选项不再发动。
func TestModeHistorySkipsActivatedOptions(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	state.Turn.Active, state.Turn.Number = "own", 3
	source := strings.Repeat("1", 32)
	held := strings.Repeat("2", 32)
	owner := state.Players["own"]
	owner.Leader = ir.Leader{Life: 10, MaxLife: 20}
	owner = withInstance(owner, "field", ir.TestInstance{InstanceID: source, CardID: 10574110, DeclaredType: "follower"})
	owner = withInstance(owner, "hand", ir.TestInstance{InstanceID: held, CardID: 10012310, DeclaredType: "spell"})
	state.Players["own"] = owner
	s, err := NewSession(pack, state, 5)
	if err != nil {
		t.Fatal(err)
	}
	card := s.g.instances[source]
	hand := s.g.instances[held]
	changed := func() int {
		count := 0
		if s.g.own.leaderLife != 10 {
			count++
		}
		if card.attack != 0 || card.life != 2 {
			count++
		}
		if hand.cost != 1 {
			count++
		}
		return count
	}
	for turn := 1; turn <= 3; turn++ {
		if r := s.Advance(ir.AdvanceAction{Kind: "advance", Timing: "turn_start", Side: "own"}); r.Status != StatusCompleted {
			t.Fatalf("advance %d: %+v", turn, r)
		}
		if changed() != turn {
			t.Fatalf("after %d activations %d effects applied", turn, changed())
		}
	}
	// 三个能力都用过后再发动不会有任何变化。
	before := changed()
	if r := s.Advance(ir.AdvanceAction{Kind: "advance", Timing: "turn_start", Side: "own"}); r.Status != StatusCompleted {
		t.Fatalf("fourth advance: %+v", r)
	}
	if changed() != before {
		t.Fatal("an already activated option fired again")
	}
}
