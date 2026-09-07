package runner

import (
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestFilteredKeywordsBothPlayersAndContinuation(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			state := testState()
			state.Turn.Active, state.Turn.Number = side, 6
			source, ally, neutral, enemy := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32)
			guild := strings.Repeat("5", 32)
			p := state.Players[side]
			p.SEP = 1
			p.PP, p.MaxPP = 1, 1
			p = withInstance(p, "field", ir.TestInstance{InstanceID: guild, CardID: 10002210, DeclaredType: "amulet"})
			p = withInstance(p, "field", ir.TestInstance{InstanceID: source, CardID: 10124120, DeclaredType: "follower"})
			p = withInstance(p, "field", ir.TestInstance{InstanceID: ally, CardID: 90021110, DeclaredType: "follower"})
			state.Players[side] = withInstance(p, "field", ir.TestInstance{InstanceID: neutral, CardID: 10001110, DeclaredType: "follower"})
			other := oppositeSide(side)
			state.Players[other] = withInstance(state.Players[other], "field", ir.TestInstance{InstanceID: enemy, CardID: 90021110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Keywords: []string{"barrier", "aura", "stealth"}}})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			before := s.g.snapshot()
			s.LegalActionsFor(side)
			if !reflect.DeepEqual(before, s.g.snapshot()) {
				t.Fatal("preflight granted keywords")
			}
			if step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "superevolve", Source: source}); step.Status != StatusCompleted {
				t.Fatal(step)
			}
			for _, id := range []string{source, ally, neutral, enemy} {
				if s.g.instances[id].abilities["barrier"] != (id == ally || id == enemy) {
					t.Fatal("incorrect filtered barrier target", id)
				}
			}
			step := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "engage", Source: guild})
			if step.Status != StatusSuspended {
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
			if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
				t.Fatal("keyword state changed during restore")
			}
			// A bulk removal affects a hidden enemy without creating a target choice.
			e := ir.TargetEffect{Kind: "remove_keyword", Keyword: "barrier", Target: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}, Predicate: ir.FieldPredicate{Kind: "has_class", Class: "swordcraft"}}
			for _, current := range []*Session{s, restored} {
				result := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{ally}})
				if result.Status != StatusCompleted || !current.g.instances[ally].abilities["rush"] {
					t.Fatal("restored selection did not complete", result)
				}
				current.g.execTargetEffect(e, current.g.instances[source], frame{})
				if current.g.instances[enemy].abilities["barrier"] || !current.g.instances[ally].abilities["barrier"] {
					t.Fatal("keyword removal applied to incorrect side")
				}
				e.Predicate = ir.FieldPredicate{Kind: "has_class", Class: "forestcraft"}
				before := current.g.snapshot()
				current.g.execTargetEffect(e, current.g.instances[source], frame{})
				if !reflect.DeepEqual(before, current.g.snapshot()) {
					t.Fatal("empty keyword filter changed state")
				}
				e.Predicate = ir.FieldPredicate{Kind: "has_class", Class: "swordcraft"}
			}
		})
	}
}

func TestAmaliaSummonedKeywordsAndAttackLegality(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active, state.Turn.Number = side, 8
		id, enemy := strings.Repeat("1", 32), strings.Repeat("2", 32)
		p := state.Players[side]
		p.PP, p.MaxPP = 8, 8
		state.Players[side] = withInstance(p, "hand", ir.TestInstance{InstanceID: id, CardID: 10123140, DeclaredType: "follower"})
		other := oppositeSide(side)
		state.Players[other] = withInstance(state.Players[other], "field", ir.TestInstance{InstanceID: enemy, CardID: 10001110, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		if step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: id}); step.Status != StatusCompleted {
			t.Fatal(step)
		}
		for _, i := range s.g.player(side).field {
			if i.id == id {
				if i.attack != 6 || i.abilities["rush"] || i.abilities["ward"] {
					t.Fatal("Amalia affected herself")
				}
				continue
			}
			if i.attack != 3 || i.life != 2 || !i.abilities["rush"] || !i.abilities["ward"] {
				t.Fatal("summoned Knight did not receive all effects")
			}
			canAttack := false
			for _, action := range s.LegalActionsFor(side) {
				if action.Source == i.id && action.Kind == "attack_entity" && action.Defender == enemy {
					canAttack = true
				}
				if action.Source == i.id && action.Kind == "attack_leader" {
					t.Fatal("Rush allowed attacking a leader on the summon turn")
				}
			}
			if !canAttack {
				t.Fatal("granted Rush did not enable an immediate follower attack")
			}
		}
	}
}
