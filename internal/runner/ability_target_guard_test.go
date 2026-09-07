package runner

import (
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestAbilityTargetGuardCandidates(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			state := testState()
			state.Turn.Active = side
			source, first, second, other := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32)
			state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: source, CardID: 10001110, DeclaredType: "follower"})
			opponent := oppositeSide(side)
			p := state.Players[opponent]
			for _, id := range []string{first, second} {
				p = withInstance(p, "field", ir.TestInstance{InstanceID: id, CardID: 90074120, DeclaredType: "follower"})
			}
			state.Players[opponent] = withInstance(p, "field", ir.TestInstance{InstanceID: other, CardID: 10001110, DeclaredType: "follower"})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			e := ir.SelectionEffect{Kind: "choose", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}}
			check := func(want ...string) {
				t.Helper()
				got := s.g.selectionCandidates(e, s.g.instances[source], frame{})
				if len(got) != len(want) {
					t.Fatalf("candidates=%v, want=%v", instanceIDs(got), want)
				}
				for n := range want {
					if got[n].id != want[n] {
						t.Fatal(instanceIDs(got), want)
					}
				}
			}
			check(first, second)
			e.Extremum = &ir.SelectionExtremum{Direction: "highest", Field: "attack"}
			check(first, second)
			e.Extremum = nil
			e.Source = ir.FilterRef{Kind: "filter", Source: e.Source, Predicate: ir.FieldPredicate{Kind: "compare", Field: "cost", Op: "lt", Value: 3}}
			check()
			e.Source = ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}
			e.Kind = "random_choose"
			check(first, second, other)
			e.Kind = "choose"
			e.Source = ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}
			check(source)
			e.Source = ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}
			s.g.instances[first].abilities["aura"] = true
			check(second)
			s.g.instances[second].abilities["stealth"] = true
			check()
			delete(s.g.instances[first].abilities, "aura")
			s.g.execTargetEffect(ir.TargetEffect{Kind: "remove_keyword", Keyword: "ability_target_guard", Target: ir.BindingRef{Kind: "binding", Name: "target"}}, s.g.instances[source], frame{"target": bindEntities(s.g.instances[first])})
			check()
			s.g.returnCard(s.g.instances[second], "hand")
			check(first, other)
			s.g.execTargetEffect(ir.TargetEffect{Kind: "add_keyword", Keyword: "ability_target_guard", Target: ir.BindingRef{Kind: "binding", Name: "target"}}, s.g.instances[source], frame{"target": bindEntities(s.g.instances[other])})
			check(other)
			s.g.budget = &budgetTracker{policy: s.budgetPolicy}
			s.g.budget.policy.QueryVisits = 2
			check()
			if !s.g.budget.exceeded {
				t.Fatal("guard query did not account for budget")
			}
		})
	}
}

func TestLloydChoiceContinuationAndProtectionRemoval(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active = side
		source, lloyd, other := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
		p := state.Players[side]
		p.PP, p.MaxPP = 4, 4
		state.Players[side] = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10071310, DeclaredType: "spell"})
		opponent := oppositeSide(side)
		p = withInstance(state.Players[opponent], "field", ir.TestInstance{InstanceID: lloyd, CardID: 90074120, DeclaredType: "follower"})
		state.Players[opponent] = withInstance(p, "field", ir.TestInstance{InstanceID: other, CardID: 10174120, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		before := s.g.snapshot()
		s.LegalActionsFor(side)
		if !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("preflight changed state")
		}
		step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: source})
		if step.Status != StatusSuspended || len(step.Choice.Candidates) != 1 || step.Choice.Candidates[0].InstanceID != lloyd {
			t.Fatal(step)
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
		response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{other}}
		if r := restored.Resume(response); r.Status != StatusRejected {
			t.Fatal("accepted protected follower", r)
		}
		response.SelectedInstanceIDs = []string{lloyd}
		for _, current := range []*Session{s, restored} {
			if r := current.Resume(response); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if current.g.instances[lloyd].zone != "graveyard" || current.g.instances[other].zone != "field" {
				t.Fatal("destroyed the wrong follower")
			}
			e := ir.SelectionEffect{Kind: "choose", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}}
			candidates := current.g.selectionCandidates(e, current.g.instances[source], frame{})
			if len(candidates) != 1 || candidates[0].id != other {
				t.Fatal("destroyed Lloyd still restricts targets")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored guard choice diverged")
		}
		for n := range c.Game.Instances {
			if c.Game.Instances[n].ID == lloyd {
				c.Game.Instances[n].Abilities = []string{"ward"}
			}
		}
		if _, err := RestoreSession(pack, c); err == nil {
			t.Fatal("restored stale candidates after the guard ability was removed")
		}
	}
}

func TestOrchisPuppetsAttackAndExpireBothPlayers(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active, state.Turn.Number = side, 7
		source, enemy := strings.Repeat("1", 32), strings.Repeat("2", 32)
		p := state.Players[side]
		p.PP, p.MaxPP, p.SEP = 8, 8, 1
		state.Players[side] = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10174120, DeclaredType: "follower"})
		opponent := oppositeSide(side)
		state.Players[opponent] = withInstance(state.Players[opponent], "field", ir.TestInstance{InstanceID: enemy, CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: 2, Life: 10}}})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		for n, kind := range []string{"play", "superevolve"} {
			if step := s.SubmitAs(strings.Repeat(string(rune('a'+n)), 32), side, SimulatorCommand{Kind: kind, Source: source}); step.Status != StatusCompleted {
				t.Fatal(step)
			}
		}
		lloyd := ""
		puppets := 0
		for _, i := range s.g.player(side).field {
			if i.id == source {
				continue
			}
			if !i.abilities["storm"] || !i.abilities["bane"] {
				t.Fatal("entering puppet missed granted keywords")
			}
			if i.card.ID == 90074120 {
				lloyd = i.id
			} else if i.card.ID == 90071120 {
				puppets++
			}
			canAttackLeader := false
			for _, action := range s.LegalActionsFor(side) {
				if action.Kind == "attack_leader" && action.Source == i.id {
					canAttackLeader = true
				}
			}
			if !canAttackLeader {
				t.Fatal("granted Storm did not enable immediate attack")
			}
		}
		if lloyd == "" || puppets != 2 {
			t.Fatal("Orchis summon counts are incorrect")
		}
		if step := s.SubmitAs(strings.Repeat("c", 32), side, SimulatorCommand{Kind: "attack", Source: lloyd, Defender: enemy}); step.Status != StatusCompleted {
			t.Fatal(step)
		}
		if s.g.instances[enemy].zone != "graveyard" || s.g.instances[lloyd].life != 4 {
			t.Fatal("Lloyd did not destroy the defender through granted Bane")
		}
		for n, actor := range []string{side, opponent} {
			if step := s.SubmitAs(strings.Repeat(string(rune('d'+n)), 32), actor, SimulatorCommand{Kind: "end_turn"}); step.Status != StatusCompleted {
				t.Fatal(step)
			}
		}
		if len(s.g.player(side).field) != 2 || s.g.instances[lloyd].zone != "field" || len(s.g.player(side).graveyard) != 2 {
			t.Fatal("puppets did not expire at the end of the opponent's turn")
		}
	}
}
