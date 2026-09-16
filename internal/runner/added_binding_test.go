package runner

import (
	"strings"
	"testing"

	"wbo/internal/ir"
)

// 猫偶：`add 1 card 90071110 to hand; buff added +3/+0;` 只能强化这次加入的那一张。
// 场景测试只能数手牌张数，数值断言放在这里。
func TestAddedBindingBuffsOnlyTheNewCard(t *testing.T) {
	pack := loadCardsForTest(t, "10002/10271120", "90000/90071110", "10000/10001110")
	sourceID, evolvedID, oldID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	state := testState()
	own := state.Players["own"]
	own.PP, own.MaxPP = 1, 1
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10271120, DeclaredType: "follower"})
	// 手里已经有一张同名的悬丝傀儡，用来验证强化不会连带旧卡。
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: oldID, CardID: 90071110, DeclaredType: "follower"})
	superEvolved := true
	own = withInstance(own, "field", ir.TestInstance{InstanceID: evolvedID, CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{SuperEvolved: &superEvolved}})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID}); result.Status != StatusCompleted {
		t.Fatalf("playing the cat doll failed: %#v", result)
	}
	puppets := 0
	for _, card := range session.g.own.hand {
		if card.card.ID != 90071110 {
			continue
		}
		puppets++
		attack, life := card.currentAttack(), card.life
		if card.id == oldID {
			if attack != 1 || life != 1 {
				t.Fatalf("existing puppet was modified: %d/%d", attack, life)
			}
			continue
		}
		if attack != 4 || life != 1 {
			t.Fatalf("added puppet stats = %d/%d, want 4/1", attack, life)
		}
	}
	if puppets != 2 {
		t.Fatalf("puppets in hand = %d, want 2", puppets)
	}
}
