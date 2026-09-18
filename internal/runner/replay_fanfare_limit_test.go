package runner

import (
	"strings"
	"testing"

	"wbo/internal/ir"
)

// 官方 QA（0vrc9nkxfkx1）：『恐惧的象征·欧米伽奥提普』的（4）连续被选中时，
// "发动本随从的【入场曲】"最多 20 次，"本随从+4/+4"最多 21 次（含最初的 1 次）。
// 引擎用一个自引用的入场曲来固定这个上限：最初的 1 次 + 20 次重发 = 21 次强化。
func TestFanfareReplayIsCappedAtTwenty(t *testing.T) {
	cardID := 77881001
	pack := &ir.CardPack{Cards: []ir.Card{{
		ID: cardID, CardType: "follower", Cost: 1, Stats: &ir.Stats{Attack: 1, Life: 1},
		Abilities: []ir.Ability{{
			ID: strings.Repeat("a", 32), Trigger: ir.SimpleTrigger{Kind: "fanfare"},
			Body: []ir.Effect{
				ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "buff_stats", Target: ir.SelfRef{}, AttackDelta: 1, LifeDelta: 1},
				ir.ReplayFanfareEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("c", 32)}, Kind: "replay_fanfare", Target: ir.SelfRef{}},
			},
		}},
	}}}
	state := testState()
	sourceID := strings.Repeat("1", 32)
	actor := state.Players["own"]
	actor.PP, actor.MaxPP = 10, 10
	actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: cardID, DeclaredType: "follower"})
	state.Players["own"] = actor
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	result := session.SubmitAs(strings.Repeat("9", 32), "own", SimulatorCommand{Kind: "play", Source: sourceID})
	if result.Status != StatusCompleted {
		t.Fatal(result)
	}
	source := session.g.instances[sourceID]
	if source == nil {
		t.Fatal("source instance missing")
	}
	if source.fanfareReplays != fanfareReplayLimit {
		t.Fatalf("replays = %d, want the QA cap %d", source.fanfareReplays, fanfareReplayLimit)
	}
	if source.attack != 1+(fanfareReplayLimit+1) || source.life != 1+(fanfareReplayLimit+1) {
		t.Fatalf("stats = %d/%d, want %d/%d (initial fanfare plus 20 replays)",
			source.attack, source.life, 1+fanfareReplayLimit+1, 1+fanfareReplayLimit+1)
	}
}
