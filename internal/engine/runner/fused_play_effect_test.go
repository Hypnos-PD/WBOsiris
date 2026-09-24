package runner

import (
	"path/filepath"
	"strings"
	"testing"

	"wbo/internal/engine/ir"
	"wbo/internal/engine/project"
)

// 花园的指引（10213310）：打出时若已融合，则改为抽 2 张。
// 融合指令与打出指令是两个独立命令，场景测试只能各写一个，所以这条链路由 Go 测试覆盖。
func TestFusedSpellReadsMaterialsAtResolution(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	loaded := project.LoadWithRoot([]string{
		filepath.Join(root, "cards", "10002", "10213310.wbo"),
		filepath.Join(root, "cards", "10001", "10111110.wbo"),
		filepath.Join(root, "cards", "10000", "10001110.wbo"),
	}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	sourceID, materialID, firstDraw, secondDraw := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32)
	for _, fused := range []bool{false, true} {
		name := "未融合时只抽一张"
		if fused {
			name = "融合后改为抽两张"
		}
		t.Run(name, func(t *testing.T) {
			state := testState()
			actor := state.Players["own"]
			actor.PP, actor.MaxPP = 4, 4
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10213310, DeclaredType: "spell"})
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: materialID, CardID: 10111110, DeclaredType: "follower"})
			actor = withInstance(actor, "deck", ir.TestInstance{InstanceID: firstDraw, CardID: 10001110, DeclaredType: "follower"})
			actor = withInstance(actor, "deck", ir.TestInstance{InstanceID: secondDraw, CardID: 10001110, DeclaredType: "follower"})
			state.Players["own"] = actor
			session, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			if fused {
				result := session.Begin(strings.Repeat("a", 32), ir.FusionAction{Kind: "fusion", Actor: "own", Source: sourceID})
				if result.Status != StatusSuspended || result.Choice == nil || result.Choice.Kind != "fusion_material" {
					t.Fatalf("fusion did not request materials: %#v", result)
				}
				choice := session.PendingChoice()
				result = session.Resume(ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{materialID}})
				if result.Status != StatusCompleted {
					t.Fatalf("fusion did not complete: %#v", result)
				}
				if len(session.g.instances[sourceID].materials) != 1 {
					t.Fatalf("materials = %#v", session.g.instances[sourceID].materials)
				}
			}
			result := session.Begin(strings.Repeat("b", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID})
			if result.Status != StatusCompleted {
				t.Fatalf("play failed: %#v", result)
			}
			drawn := 0
			for _, card := range session.g.own.hand {
				if card.card.ID == 10001110 {
					drawn++
				}
			}
			want := 1
			if fused {
				want = 2
			}
			if drawn != want {
				t.Fatalf("drawn = %d, want %d", drawn, want)
			}
			if session.g.instances[sourceID].zone != "graveyard" {
				t.Fatalf("spell zone = %s", session.g.instances[sourceID].zone)
			}
		})
	}
}
