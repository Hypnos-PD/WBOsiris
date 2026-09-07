package runner

import (
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestChooseOtherExcludesSourceAndBuffsSelectedFollower(t *testing.T) {
	sourceID, allyID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	card := ir.Card{ID: 77770001, CardType: "follower", Cost: 1, Stats: &ir.Stats{Attack: 2, Life: 2}, PlayEffects: []ir.Effect{
		ir.SelectionEffect{Kind: "choose", Policy: "required", Binding: "target", Source: ir.ExcludeRef{Kind: "exclude", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}, Value: ir.SelfRef{Kind: "self"}}},
		ir.TargetEffect{Kind: "buff_stats", Target: ir.BindingRef{Kind: "binding", Name: "target"}, AttackDelta: 1, LifeDelta: 1},
	}}
	state := testState()
	own := state.Players["own"]
	own.PP, own.MaxPP = 1, 1
	state.Players["own"] = own
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, CardID: card.ID, DeclaredType: "follower"})
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: allyID, CardID: card.ID, DeclaredType: "follower"})
	session, err := NewSession(&ir.CardPack{Cards: []ir.Card{card}}, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	result := session.Submit(strings.Repeat("3", 32), SimulatorCommand{Kind: "play", Source: sourceID})
	if result.Status != StatusSuspended || result.Choice == nil || len(result.Choice.Candidates) != 1 || result.Choice.Candidates[0].InstanceID != allyID {
		t.Fatalf("unexpected choice: %#v", result)
	}
	choice := result.Choice
	result = session.Resume(ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{allyID}})
	if result.Status != StatusCompleted || session.g.instances[allyID].attack != 3 || session.g.instances[allyID].life != 3 || session.g.instances[sourceID].attack != 2 {
		t.Fatalf("other target buff failed: %#v source=%d/%d ally=%d/%d", result, session.g.instances[sourceID].attack, session.g.instances[sourceID].life, session.g.instances[allyID].attack, session.g.instances[allyID].life)
	}
}
