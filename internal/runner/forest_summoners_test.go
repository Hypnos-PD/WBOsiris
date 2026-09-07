package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"wbo/internal/ir"
)

func TestForestAmuletEngageRestoresAfterSourceDestruction(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		for _, card := range []int{10112210, 10113210} {
			t.Run(fmt.Sprintf("%s/%d", owner, card), func(t *testing.T) {
				state := testState()
				state.Turn.Active = owner
				source, follower, amulet, enemy := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32)
				p := withInstance(state.Players[owner], "field", ir.TestInstance{InstanceID: source, CardID: card, DeclaredType: "amulet"})
				p = withInstance(p, "field", ir.TestInstance{InstanceID: follower, CardID: 10001110, DeclaredType: "follower"})
				state.Players[owner] = withInstance(p, "field", ir.TestInstance{InstanceID: amulet, CardID: 10161210, DeclaredType: "amulet"})
				state.Players[oppositeSide(owner)] = withInstance(state.Players[oppositeSide(owner)], "field", ir.TestInstance{InstanceID: enemy, CardID: 10001110, DeclaredType: "follower"})
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				step := s.SubmitAs(strings.Repeat("a", 32), owner, SimulatorCommand{Kind: "engage", Source: source})
				if step.Status != StatusSuspended || s.g.instances[source].zone != "graveyard" || s.g.player(owner).shadows != 1 || s.g.player(owner).pp != 0 {
					t.Fatal("destruction did not precede selection", step)
				}
				wantCandidates := 1
				if card == 10113210 {
					wantCandidates = 2
				}
				if len(step.Choice.Candidates) != wantCandidates || step.Choice.Candidates[0].InstanceID != follower {
					t.Fatal("wrong engage candidates", step.Choice)
				}
				view, err := s.View(oppositeSide(owner))
				if err != nil || view.PendingChoice != nil {
					t.Fatal("choice leaked", err)
				}
				before := s.g.snapshot()
				response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{source}}
				if bad := s.Resume(response); bad.Status != StatusRejected || !reflect.DeepEqual(before, s.g.snapshot()) {
					t.Fatal("destroyed source was selectable", bad)
				}
				response.SelectedInstanceIDs = []string{enemy}
				if bad := s.Resume(response); bad.Status != StatusRejected || !reflect.DeepEqual(before, s.g.snapshot()) {
					t.Fatal("enemy was selectable", bad)
				}
				data, err := s.EncodeContinuation()
				if err != nil {
					t.Fatal(err)
				}
				checkpoint, err := DecodeContinuation(data)
				if err != nil {
					t.Fatal(err)
				}
				restored, err := RestoreSession(pack, checkpoint)
				if err != nil {
					t.Fatal(err)
				}
				response.SelectedInstanceIDs = []string{follower}
				if card == 10113210 {
					response.SelectedInstanceIDs = []string{amulet}
				}
				for _, current := range []*Session{s, restored} {
					if r := current.Resume(response); r.Status != StatusCompleted {
						t.Fatal(r)
					}
					if card == 10112210 && (current.g.instances[follower].attack != 3 || current.g.instances[follower].life != 3) {
						t.Fatal("buff lost destroyed source owner")
					}
					if card == 10113210 && current.g.instances[amulet].zone != "hand" {
						t.Fatal("return lost destroyed source owner")
					}
					for _, action := range current.LegalActions() {
						if action.Source == source {
							t.Fatal("destroyed source remains actionable")
						}
					}
				}
				if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
					t.Fatal("restored engage diverged")
				}
			})
		}
	}
}

func TestCerberusBuffsNewCompanionsAfterOrderedSummons(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		for _, shadows := range []int{5, 6} {
			for _, occupied := range []int{0, 3, 4} {
				t.Run(fmt.Sprintf("%s/shadows%d/occupied%d", owner, shadows, occupied), func(t *testing.T) {
					state := testState()
					state.Turn.Active = owner
					p := state.Players[owner]
					p.PP, p.MaxPP, p.Shadows = 8, 8, shadows
					source := strings.Repeat("a", 32)
					p = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10154110, DeclaredType: "follower"})
					for n := 0; n < occupied; n++ {
						p = withInstance(p, "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 10001110, DeclaredType: "follower"})
					}
					state.Players[owner] = p
					s, err := NewSession(pack, state, 1)
					if err != nil {
						t.Fatal(err)
					}
					if r := s.SubmitAs(strings.Repeat("b", 32), owner, SimulatorCommand{Kind: "play", Source: source}); r.Status != StatusCompleted {
						t.Fatal(r)
					}
					boost := 0
					if shadows == 6 {
						boost = 2
					}
					wantShadows := shadows
					if shadows == 6 {
						wantShadows = 0
					}
					own := s.g.player(owner)
					if own.shadows != wantShadows || own.pp != 0 || len(own.field) != min(5, occupied+3) {
						t.Fatal("wrong summons or resource payment")
					}
					for n, i := range own.field {
						if n < occupied && (i.attack != 2+boost || i.life != 2) {
							t.Fatal("existing ally not buffed")
						}
						if n == occupied && (i.id != source || i.attack != 6 || i.life != 6) {
							t.Fatal("Cerberus buffed itself")
						}
						if n == occupied+1 && (i.card.ID != 90054110 || i.attack != 2+boost || i.life != 1 || !i.abilities["rush"]) {
							t.Fatal("Mimi order or buff")
						}
						if n == occupied+2 && (i.card.ID != 90054120 || i.attack != 1+boost || i.life != 2 || !i.abilities["rush"]) {
							t.Fatal("Coco order or buff")
						}
					}
				})
			}
		}
	}
}

func TestCerberusRepeatedReanimateRestoresWithDepartedListeners(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID == 10153130 {
			for a := range pack.Cards[n].Abilities {
				trigger, ok := pack.Cards[n].Abilities[a].Trigger.(ir.EventTrigger)
				if ok && trigger.Event == "follower_summoned" {
					pause := ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}}
					pack.Cards[n].Abilities[a].Body = append([]ir.Effect{pause}, pack.Cards[n].Abilities[a].Body...)
				}
			}
		}
	}
	for _, owner := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active, state.Turn.Number = owner, 6
		p := state.Players[owner]
		p.SEP = 1
		source := strings.Repeat("a", 32)
		p = withInstance(p, "field", ir.TestInstance{InstanceID: source, CardID: 10154110, DeclaredType: "follower"})
		p = withInstance(p, "field", ir.TestInstance{InstanceID: strings.Repeat("b", 32), CardID: 10153130, DeclaredType: "follower"})
		for n, id := range []int{90051110, 90054110, 90054120, 10001110} {
			p = withInstance(p, "destroyed", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: id, DeclaredType: "follower"})
		}
		state.Players[owner] = p
		s, err := NewSession(pack, state, 4)
		if err != nil {
			t.Fatal(err)
		}
		step := s.SubmitAs(strings.Repeat("c", 32), owner, SimulatorCommand{Kind: "superevolve", Source: source})
		if step.Status != StatusSuspended || len(s.g.player(owner).field) != 4 || s.g.rng.Consumed() != 2 {
			t.Fatal("listeners interrupted repeated summon or wrong reanimate cost", step)
		}
		data, err := s.EncodeContinuation()
		if err != nil {
			t.Fatal(err)
		}
		checkpoint, err := DecodeContinuation(data)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := RestoreSession(pack, checkpoint)
		if err != nil {
			t.Fatal(err)
		}
		for _, current := range []*Session{s, restored} {
			result := step
			for count := 0; result.Status == StatusSuspended && count < 3; count++ {
				result = current.Resume(ChoiceResponse{RequestID: result.Choice.RequestID, ActionID: result.Choice.ActionID, StateRevision: result.Choice.StateRevision, SelectedOptionID: 1})
			}
			if result.Status != StatusCompleted {
				t.Fatal(result)
			}
			for _, i := range current.g.player(owner).field[2:] {
				if i.card.Cost != 1 || !i.departed || !i.abilities["bane"] || !i.abilities["rush"] || i.attack != i.card.Stats.Attack || i.life != i.card.Stats.Life {
					t.Fatal("reanimate lost printed stats or listener keywords")
				}
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("reanimate continuation diverged")
		}
	}
}
