package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"wbo/internal/ir"
)

func TestRaveningTentaclesSelectsFollowerOrLeaderForBothSides(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		for _, kind := range []string{"leader", "follower", "barrier", "guard", "hidden_guard"} {
			t.Run(side+"/"+kind, func(t *testing.T) {
				state := testState()
				state.Turn.Active = side
				other := oppositeSide(side)
				source, target := strings.Repeat("1", 32), strings.Repeat("2", 32)
				owner := state.Players[side]
				owner.PP, owner.MaxPP, owner.Leader.Life = 7, 7, 17
				state.Players[side] = withInstance(owner, "hand", ir.TestInstance{InstanceID: source, CardID: 10123310, DeclaredType: "spell"})
				if kind != "leader" {
					cardID := 10113130
					keywords := []string{}
					if kind == "guard" || kind == "hidden_guard" {
						cardID = 90074120
					}
					if kind == "barrier" {
						keywords = []string{"barrier"}
					}
					if kind == "hidden_guard" {
						keywords = []string{"aura"}
					}
					state.Players[other] = withInstance(state.Players[other], "field", ir.TestInstance{InstanceID: target, CardID: cardID, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Keywords: keywords}})
				}
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				before := s.g.snapshot()
				step := s.Begin(strings.Repeat("3", 32), ir.SourceAction{Kind: "play", Actor: side, Source: source})
				if kind == "hidden_guard" {
					if step.Status != StatusIllegal || step.IllegalCode != "target_required" || !reflect.DeepEqual(before, s.g.snapshot()) {
						t.Fatal("guard failed preflight", step)
					}
					return
				}
				wantCandidates := 2
				if kind == "leader" || kind == "guard" {
					wantCandidates = 1
				}
				if step.Status != StatusSuspended || len(step.Choice.Candidates) != wantCandidates {
					t.Fatal(step)
				}
				before = s.g.snapshot()
				response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedLeaderSides: []string{side}}
				if r := s.Resume(response); r.Status != StatusRejected || !reflect.DeepEqual(before, s.g.snapshot()) {
					t.Fatal("accepted another leader", r)
				}
				response.SelectedLeaderSides = []string{other, other}
				if r := s.Resume(response); r.Status != StatusRejected || !reflect.DeepEqual(before, s.g.snapshot()) {
					t.Fatal("accepted repeated leader", r)
				}
				response.SelectedLeaderSides = nil
				if kind == "leader" {
					response.SelectedLeaderSides = []string{other}
				} else {
					response.SelectedInstanceIDs = []string{target}
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
				for _, current := range []*Session{s, restored} {
					if r := current.Resume(response); r.Status != StatusCompleted {
						t.Fatal(r)
					}
					if current.g.player(side).pp != 0 || current.g.player(side).leaderLife != 20 {
						t.Fatal("wrong payment or healing")
					}
					switch kind {
					case "leader":
						if current.g.player(other).leaderLife != 15 {
							t.Fatal("leader not damaged")
						}
					case "follower":
						if current.g.instances[target].zone != "graveyard" {
							t.Fatal("follower survived")
						}
					case "barrier":
						if current.g.instances[target].life != 4 || current.g.instances[target].abilities["barrier"] {
							t.Fatal("barrier interaction")
						}
					case "guard":
						if current.g.instances[target].life != 1 {
							t.Fatal("guard not damaged")
						}
					}
				}
				if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
					t.Fatal("continuation diverged")
				}
			})
		}
	}
}

func TestMixedBindingSurvivesNestedPauseAndContinuation(t *testing.T) {
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 88880007, CardType: "spell", PlayEffects: []ir.Effect{
		ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("a", 32)}, Kind: "require", Policy: "required", Binding: "target", Source: ir.CharacterSetRef{Kind: "characters", Side: "oppo"}},
		ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{
			ir.RepeatEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("c", 32)}, Kind: "repeat", Times: 2, Body: []ir.Effect{
				ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "damage", Target: ir.BindingRef{Kind: "binding", Name: "target"}, AmountExpr: &ir.CountExpr{Kind: "count", Source: ir.BindingRef{Kind: "binding", Name: "target"}}},
			}},
		}}}},
	}}}}
	state := testState()
	source := strings.Repeat("1", 32)
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: source, CardID: 88880007, DeclaredType: "spell"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := s.Begin(strings.Repeat("2", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: source})
	step = s.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedLeaderSides: []string{"oppo"}})
	if step.Status != StatusSuspended || step.Choice.Kind != "mode" {
		t.Fatal(step)
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
	for _, current := range []*Session{s, restored} {
		r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1})
		if r.Status != StatusCompleted || current.g.oppo.leaderLife != 18 {
			t.Fatal("leader binding lost through nested scopes", r)
		}
	}
	if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
		t.Fatal("nested restoration diverged")
	}
	for _, f := range saved.BindingFrames {
		for name, values := range f.Values {
			if len(values) > 0 && values[0].Kind == "leader" {
				f.Values[name][0].Side = "invalid"
			}
		}
	}
	if _, err := RestoreSession(pack, saved); err == nil {
		t.Fatal("accepted invalid bound leader")
	}
}

func TestMixedRandomSelectionIncludesLeaderAndIgnoresTargetGuard(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	for n, card := range []int{90074120, 10001110} {
		state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: card, DeclaredType: "follower"})
	}
	s, err := NewSession(pack, state, 9)
	if err != nil {
		t.Fatal(err)
	}
	e := ir.SelectionEffect{Kind: "random_choose", Policy: "random", Binding: "targets", Source: ir.CharacterSetRef{Kind: "characters", Side: "oppo"}, Count: 3}
	f := frame{}
	s.execute(e, nil, f)
	if len(f["targets"]) != 3 || f["targets"][2].Kind != "leader" || f["targets"][2].Side != "oppo" || s.g.rng.Consumed() != 3 {
		t.Fatal("random mixed selection", f)
	}
}

func TestMixedChoiceContinuationRejectsMalformedCandidates(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	source := strings.Repeat("1", 32)
	player := state.Players["own"]
	player.PP, player.MaxPP = 7, 7
	state.Players["own"] = withInstance(player, "hand", ir.TestInstance{InstanceID: source, CardID: 10123310, DeclaredType: "spell"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := s.Begin(strings.Repeat("2", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: source})
	if step.Status != StatusSuspended {
		t.Fatal(step)
	}
	data, err := s.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []ChoiceCandidate{
		{Kind: "leader", LeaderSide: "own"},
		{Kind: "leader", LeaderSide: "invalid"},
		{Kind: "leader", LeaderSide: "oppo", InstanceID: source},
		{Kind: "leader", LeaderSide: "oppo", OptionID: 1},
		{Kind: "leader", LeaderSide: "oppo", Labels: map[string]string{"chs": "extra"}},
		{Kind: "entity", InstanceID: source},
	} {
		saved, err := DecodeContinuation(data)
		if err != nil {
			t.Fatal(err)
		}
		saved.Pending.Request.Candidates[0] = candidate
		if _, err := RestoreSession(pack, saved); err == nil {
			t.Fatal("accepted malformed leader candidate", candidate)
		}
	}
}

func TestMixedMultipleSelectionCountsAndDamagesBothKinds(t *testing.T) {
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 88880007, CardType: "spell", PlayEffects: []ir.Effect{
			ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("a", 32)}, Kind: "require", Policy: "required", Binding: "targets", Source: ir.CharacterSetRef{Kind: "characters", Side: "oppo"}, Count: 2},
			ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "damage", Target: ir.BindingRef{Kind: "binding", Name: "targets"}, AmountExpr: &ir.CountExpr{Kind: "count", Source: ir.BindingRef{Kind: "binding", Name: "targets"}}},
		}},
		{ID: 88880008, CardType: "follower", Stats: &ir.Stats{Attack: 2, Life: 2}},
	}}
	state := testState()
	source, target := strings.Repeat("1", 32), strings.Repeat("2", 32)
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: source, CardID: 88880007, DeclaredType: "spell"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: target, CardID: 88880008, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := s.Begin(strings.Repeat("3", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: source})
	if step.Status != StatusSuspended || step.Choice.MinSelections != 2 {
		t.Fatal(step)
	}
	response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedLeaderSides: []string{"oppo", "oppo"}}
	if r := s.Resume(response); r.Status != StatusRejected || r.ErrorCode != "invalid_candidate" {
		t.Fatal("accepted duplicate leader", r)
	}
	response.SelectedLeaderSides = []string{"oppo"}
	response.SelectedInstanceIDs = []string{target}
	if r := s.Resume(response); r.Status != StatusCompleted {
		t.Fatal(r)
	}
	if s.g.oppo.leaderLife != 18 || s.g.instances[target].zone != "graveyard" {
		t.Fatal("mixed batch damage failed")
	}
}
