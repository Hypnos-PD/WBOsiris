package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestCompiledSylviaSelectsDistinctTargetsAndRestores(t *testing.T) {
	pack := repeatCardPack(t)
	sourceID := strings.Repeat("a", 32)
	for _, side := range []string{"own", "oppo"} {
		for _, kind := range []string{"evolve", "superevolve"} {
			for candidates := 0; candidates <= 3; candidates++ {
				t.Run(fmt.Sprintf("%s/%s/%d", side, kind, candidates), func(t *testing.T) {
					state := testState()
					state.Turn.Active, state.Turn.Number = side, 8
					actor := state.Players[side]
					actor.EP, actor.SEP = 1, 1
					state.Players[side] = withInstance(actor, "field", ir.TestInstance{InstanceID: sourceID, CardID: 10173120, DeclaredType: "follower"})
					enemy := oppositeSide(side)
					ids := []string{}
					for n := 0; n < candidates; n++ {
						id := fmt.Sprintf("%032x", 10-n)
						ids = append(ids, id)
						state.Players[enemy] = withInstance(state.Players[enemy], "field", ir.TestInstance{InstanceID: id, CardID: 10151130, DeclaredType: "follower"})
					}
					for n, keyword := range []string{"stealth", "aura"} {
						state.Players[enemy] = withInstance(state.Players[enemy], "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+30), CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Keywords: []string{keyword}}})
					}
					s, err := NewSession(pack, state, 1)
					if err != nil {
						t.Fatal(err)
					}
					step := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: kind, Source: sourceID})
					if candidates == 0 {
						if step.Status != StatusCompleted {
							t.Fatal(step)
						}
						return
					}
					count := 1
					if kind == "superevolve" {
						count = min(2, candidates)
					}
					if step.Status != StatusSuspended || step.Choice.MinSelections != count || step.Choice.MaxSelections != count || len(step.Choice.Candidates) != candidates {
						t.Fatalf("wrong selection request: %#v", step)
					}
					data, err := s.EncodeContinuation()
					if err != nil {
						t.Fatal(err)
					}
					saved, err := DecodeContinuation(data)
					if err != nil {
						t.Fatal(err)
					}
					restored, err := RestoreSession(pack, saved)
					if err != nil {
						t.Fatal(err)
					}
					saved.Pending.Request.MinSelections = 0
					if _, err := RestoreSession(pack, saved); err == nil {
						t.Fatal("accepted skippable target checkpoint")
					}
					response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision}
					for _, invalid := range [][]string{nil, ids[:count-1], {sourceID}, {ids[0], ids[0]}, append(append([]string{}, ids...), sourceID)} {
						response.SelectedInstanceIDs = invalid
						before := s.g.snapshot()
						if rejected := s.Resume(response); rejected.Status != StatusRejected || !reflect.DeepEqual(before, s.g.snapshot()) {
							t.Fatal("invalid selection changed state")
						}
					}
					response.SelectedInstanceIDs = append([]string{}, ids[:count]...)
					step = s.Resume(response)
					for i, j := 0, len(response.SelectedInstanceIDs)-1; i < j; i, j = i+1, j-1 {
						response.SelectedInstanceIDs[i], response.SelectedInstanceIDs[j] = response.SelectedInstanceIDs[j], response.SelectedInstanceIDs[i]
					}
					other := restored.Resume(response)
					if step.Status != StatusCompleted || !reflect.DeepEqual(step, other) || !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) || s.BudgetState() != restored.BudgetState() {
						t.Fatal("restoration or client order changed resolution")
					}
					var batch uint64
					destroyed := 0
					for _, event := range s.g.events {
						if event.Kind == "destroyed" {
							if batch != 0 && event.BatchID != batch {
								t.Fatal("selected targets died in separate batches")
							}
							batch = event.BatchID
							destroyed++
						}
						if event.Kind == "follower_summoned" && destroyed != count {
							t.Fatal("lastwords ran before all targets died")
						}
					}
					if destroyed != count || s.g.rng.Consumed() != 0 {
						t.Fatal("wrong selected destruction count")
					}
				})
			}
		}
	}
}

func TestCompiledRandomMultiDamageUsesDistinctSnapshot(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		for _, combo := range []int{0, 2} {
			for enemies := 0; enemies <= 5; enemies++ {
				t.Run(fmt.Sprintf("%s/combo%d/enemies%d", side, combo, enemies), func(t *testing.T) {
					state := testState()
					state.Turn.Active = side
					actor := state.Players[side]
					actor.PP, actor.MaxPP, actor.Combo = 4, 4, combo
					sourceID := strings.Repeat("a", 32)
					state.Players[side] = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10111150, DeclaredType: "follower"})
					enemy := oppositeSide(side)
					for n := 0; n < enemies; n++ {
						state.Players[enemy] = withInstance(state.Players[enemy], "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 10151130, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Keywords: []string{"stealth", "aura"}}})
					}
					s, err := NewSession(pack, state, 19)
					if err != nil {
						t.Fatal(err)
					}
					if step := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); step.Status != StatusCompleted {
						t.Fatal(step)
					}
					want := min(1, enemies)
					if combo == 2 {
						want = min(3, enemies)
					}
					seen := map[string]bool{}
					var batch uint64
					for _, event := range s.g.events {
						if event.Kind == "damaged" {
							id := event.Target.InstanceID
							if seen[id] || s.g.instances[id].card.ID != 10151130 || event.Actual != 2 {
								t.Fatal("random multi-target repeated or hit a new summon")
							}
							seen[id] = true
						}
						if event.Kind == "destroyed" {
							if batch != 0 && batch != event.BatchID {
								t.Fatal("random multi-target split death batch")
							}
							batch = event.BatchID
						}
					}
					if len(seen) != want || s.g.rng.Consumed() != uint64(want) {
						t.Fatalf("hit %d want %d, rng %d", len(seen), want, s.g.rng.Consumed())
					}
				})
			}
		}
	}
}

func TestRequireMultipleTargetsPreflightsBeforePayment(t *testing.T) {
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 77775001, CardType: "spell", Cost: 2, PlayEffects: []ir.Effect{
			ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("1", 32)}, Kind: "require", Policy: "required", Binding: "targets", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}, Count: 2},
			ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("2", 32)}, Kind: "destroy", Target: ir.BindingRef{Kind: "binding", Name: "targets"}},
		}},
		{ID: 77775002, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
	}}
	for _, side := range []string{"own", "oppo"} {
		for enemies := 0; enemies <= 2; enemies++ {
			state := testState()
			state.Turn.Active = side
			actor := state.Players[side]
			actor.PP, actor.MaxPP = 2, 2
			sourceID := strings.Repeat("a", 32)
			state.Players[side] = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 77775001, DeclaredType: "spell"})
			other := oppositeSide(side)
			for n := 0; n < enemies; n++ {
				state.Players[other] = withInstance(state.Players[other], "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 77775002, DeclaredType: "follower"})
			}
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			before := s.g.snapshot()
			legal := hasLegalAction(s.LegalActionsFor(side), "play", sourceID, "")
			if legal != (enemies == 2) || !reflect.DeepEqual(before, s.g.snapshot()) {
				t.Fatal("preflight changed state or ignored count")
			}
			step := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "play", Source: sourceID})
			if enemies < 2 {
				if step.Status != StatusIllegal || step.IllegalCode != "target_required" || !reflect.DeepEqual(before, s.g.snapshot()) {
					t.Fatal("insufficient targets paid costs")
				}
			} else if step.Status != StatusSuspended || step.Choice.MinSelections != 2 || s.g.player(side).pp != 0 {
				t.Fatal("valid multi-target spell failed")
			}
		}
	}
}
