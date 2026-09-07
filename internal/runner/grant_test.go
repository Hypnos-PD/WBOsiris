package runner

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"wbo/internal/ir"
)

func TestResurgenceRequiresTwoAndOnlyCopiesExpire(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		for _, count := range []int{0, 1, 2, 3} {
			state := testState()
			state.Turn.Active = side
			p := state.Players[side]
			p.PP, p.MaxPP = 5, 5
			source := strings.Repeat("a", 32)
			p = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10172320, DeclaredType: "spell"})
			ids := []string{}
			for n := 0; n < count; n++ {
				id := fmt.Sprintf("%032x", n+1)
				ids = append(ids, id)
				p = withInstance(p, "hand", ir.TestInstance{InstanceID: id, CardID: 90072120, DeclaredType: "follower"})
			}
			state.Players[side] = p
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			step := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "play", Source: source})
			if count < 2 {
				if step.Status != StatusIllegal || step.IllegalCode != "target_required" || s.g.player(side).pp != 5 || s.g.instances[source].zone != "hand" {
					t.Fatal("missing preflight", step)
				}
				continue
			}
			if step.Status != StatusSuspended || step.Choice.MinSelections != 2 {
				t.Fatal(step)
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
			response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: ids[:2]}
			for _, current := range []*Session{s, restored} {
				if r := current.Resume(response); r.Status != StatusCompleted {
					t.Fatal(r)
				}
				owner := current.g.player(side)
				if len(owner.field) != 2 || len(owner.hand) != count {
					t.Fatal("wrong copies")
				}
				for _, copy := range owner.field {
					if len(copy.grants) != 1 {
						t.Fatal("copy missing grant")
					}
				}
				for _, original := range owner.hand {
					if len(original.grants) != 0 {
						t.Fatal("granted to original")
					}
				}
				if r := current.Advance(ir.AdvanceAction{Timing: "turn_end", Side: side}); r.Status != StatusCompleted || len(owner.field) != 2 {
					t.Fatal("early expiry", r)
				}
				if r := current.Advance(ir.AdvanceAction{Timing: "turn_end", Side: oppositeSide(side)}); r.Status != StatusCompleted || len(owner.field) != 0 || owner.shadows != 3 {
					t.Fatal("late expiry", r)
				}
			}
			if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
				t.Fatal("restored grant diverged")
			}
		}
	}
}

func TestLiamGrantedLastwordsStackPersistAndCopy(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active = side
		state.Turn.Number = 8
		source, target := strings.Repeat("a", 32), strings.Repeat("b", 32)
		p := state.Players[side]
		p.EP, p.SEP = 1, 1
		p = withInstance(p, "field", ir.TestInstance{InstanceID: source, CardID: 10173130, DeclaredType: "follower"})
		state.Players[side] = withInstance(p, "field", ir.TestInstance{InstanceID: target, CardID: 90071120, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		if r := s.SubmitAs(strings.Repeat("c", 32), side, SimulatorCommand{Kind: "evolve", Source: source}); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		puppet := s.g.instances[target]
		if len(puppet.grants) != 1 || !puppet.abilities["ward"] {
			t.Fatal("Liam grant missing")
		}
		view, _ := s.View(side)
		if len(view.Own.Field[1].GrantedAbilities) != 1 || !slices.Contains(view.Own.Field[1].Keywords, "lastwords") {
			t.Fatal("missing grant view")
		}
		grant := puppet.grants[0]
		s.g.grantAbility(grant, puppet, frame{})
		if len(puppet.grants) != 2 {
			t.Fatal("grant did not stack")
		}
		copies := s.g.summonCopies(puppet, "own", []*instance{puppet})
		if len(copies) != 1 || len(copies[0].grants) != 2 {
			t.Fatal("copy lost grants")
		}
		s.g.returnCard(s.g.instances[source], "hand")
		checkpoint := snapshotContinuationGame(s.g)
		restored, err := restoreGame(s.g.cards, checkpoint)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(checkpoint, snapshotContinuationGame(restored)) {
			t.Fatal("grants lost in saved state")
		}
		for n := range checkpoint.Instances {
			if checkpoint.Instances[n].ID == target {
				checkpoint.Instances[n].Grants = []string{strings.Repeat("f", 32)}
			}
		}
		if _, err := restoreGame(s.g.cards, checkpoint); err == nil {
			t.Fatal("accepted unknown grant")
		}
		if r := s.Advance(ir.AdvanceAction{Timing: "turn_end", Side: oppositeSide(side)}); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		if len(s.g.player(side).field) != 0 || s.g.player(oppositeSide(side)).leaderLife != 12 {
			t.Fatal("stacked lastwords did not survive source departure and copy")
		}
	}
}

func TestGrantedAbilityResetAndDepartureCancellation(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	id := strings.Repeat("a", 32)
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: id, CardID: 90072120, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	i := s.g.instances[id]
	grant := s.g.cards[10172320].PlayEffects[2].(ir.GrantEffect)
	grant.Target = ir.SelfRef{Kind: "self"}
	s.g.grantAbility(grant, i, frame{})
	s.g.queueEventTriggers(ir.RuntimeEvent{Kind: "turn_ended", Side: "oppo"}, nil, "")
	if len(s.g.triggers) != 1 {
		t.Fatal("grant not indexed")
	}
	s.g.returnCard(i, "hand")
	if len(i.grants) != 0 || len(s.g.triggers) != 0 {
		t.Fatal("return kept grant or pending field listener")
	}
	s.g.grantAbility(grant, i, frame{})
	if len(i.grants) != 1 {
		t.Fatal("hand grant missing")
	}
	resetCardState(i, i.card)
	if len(i.grants) != 0 {
		t.Fatal("transformation retained grant")
	}
}

func TestGrantedLastwordsChoiceRestoresAfterDeath(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID != 10173130 {
			continue
		}
		grant := pack.Cards[n].Abilities[1].Body[1].(ir.GrantEffect)
		grant.Ability.Body = []ir.Effect{
			ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "choose", Policy: "optional", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}},
			ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "damage", Target: ir.BindingRef{Kind: "binding", Name: "target"}, Amount: 3},
		}
		pack.Cards[n].Abilities[1].Body[1] = grant
	}
	state := testState()
	state.Turn.Number = 8
	source, target, enemy := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)
	p := state.Players["own"]
	p.EP = 1
	p = withInstance(p, "field", ir.TestInstance{InstanceID: source, CardID: 10173130, DeclaredType: "follower"})
	state.Players["own"] = withInstance(p, "field", ir.TestInstance{InstanceID: target, CardID: 90071120, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: enemy, CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: 5, Life: 5}}})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if r := s.Submit(strings.Repeat("d", 32), SimulatorCommand{Kind: "evolve", Source: source}); r.Status != StatusCompleted {
		t.Fatal(r)
	}
	s.g.returnCard(s.g.instances[source], "hand")
	step := s.Advance(ir.AdvanceAction{Timing: "turn_end", Side: "oppo"})
	if step.Status != StatusSuspended || s.g.instances[target].zone != "graveyard" {
		t.Fatal("lastwords did not suspend after death", step)
	}
	data, err := s.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	c, err := DecodeContinuation(data)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(pack, c)
	if err != nil {
		t.Fatal(err)
	}
	response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{enemy}}
	for _, current := range []*Session{s, restored} {
		if r := current.Resume(response); r.Status != StatusCompleted || current.g.instances[enemy].life != 2 {
			t.Fatal(r)
		}
	}
	if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
		t.Fatal("granted lastwords continuation diverged")
	}
}
