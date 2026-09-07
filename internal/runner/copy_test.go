package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestRalmiaSelectionCopiesCurrentStateAndRestores(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		for _, spaces := range []int{0, 1, 4} {
			t.Run(fmt.Sprintf("%s/spaces%d", side, spaces), func(t *testing.T) {
				state := testState()
				state.Turn.Active = side
				p := state.Players[side]
				p.PP, p.MaxPP = 8, 8
				source := strings.Repeat("a", 32)
				p = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10174130, DeclaredType: "follower"})
				for n := 0; n < 4-spaces; n++ {
					p = withInstance(p, "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+20), CardID: 10001110, DeclaredType: "follower"})
				}
				ids := []string{}
				for n, card := range []int{90072110, 90072120, 90073110, 90073120, 90072110, 90071210, 10001110} {
					id := fmt.Sprintf("%032x", 10-n)
					kind := "follower"
					if n == 5 {
						kind = "amulet"
					}
					cost := 5
					if n == 4 {
						cost = 6
					}
					p = withInstance(p, "hand", ir.TestInstance{InstanceID: id, CardID: card, DeclaredType: kind, Overrides: ir.InstanceOverrides{Cost: &cost, Stats: &ir.Stats{Attack: n + 6, Life: n + 7}, Keywords: []string{"barrier"}}})
					if n < 4 {
						ids = append(ids, id)
					}
				}
				state.Players[side] = p
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				step := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "play", Source: source})
				if step.Status != StatusSuspended || len(step.Choice.Candidates) != 4 || step.Choice.MinSelections != 3 || step.Choice.MaxSelections != 3 {
					t.Fatal("incorrect current-cost copy choice", step)
				}
				for n, candidate := range step.Choice.Candidates {
					if candidate.InstanceID != ids[n] {
						t.Fatal("included expensive or wrong-type candidate", candidate)
					}
				}
				view, err := s.View(oppositeSide(side))
				if err != nil || view.PendingChoice != nil || len(view.Oppo.Hand) != 0 {
					t.Fatal("copy choice leaked hand", err)
				}
				response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision}
				for _, invalid := range [][]string{ids[:2], {ids[0], ids[0], ids[1]}, {ids[0], ids[1], fmt.Sprintf("%032x", 6)}} {
					response.SelectedInstanceIDs = invalid
					before := s.g.snapshot()
					if bad := s.Resume(response); bad.Status != StatusRejected || !reflect.DeepEqual(before, s.g.snapshot()) {
						t.Fatal("invalid copy selection changed state", bad)
					}
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
				response.SelectedInstanceIDs = []string{ids[3], ids[2], ids[1]}
				if r := s.Resume(response); r.Status != StatusCompleted {
					t.Fatal(r)
				}
				response.SelectedInstanceIDs = []string{ids[1], ids[2], ids[3]}
				if r := restored.Resume(response); r.Status != StatusCompleted || !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
					t.Fatal("copy continuation or client ordering diverged", r)
				}
				owner := s.g.player(side)
				if len(owner.hand) != 7 || len(owner.field) != 5-spaces+min(spaces, 3) || s.g.rng.Consumed() != 0 {
					t.Fatal("copy moved hand or mishandled field capacity")
				}
				for n := 0; n < min(spaces, 3); n++ {
					copy := owner.field[5-spaces+n]
					original := s.g.instances[ids[n+1]]
					if copy == original || copy.card != original.card || copy.cost != 5 || copy.attack != original.attack || copy.life != original.life || !copy.abilities["barrier"] || !copy.summoningSick || original.zone != "hand" {
						t.Fatal("copy lost current properties")
					}
					copy.removeKeyword("barrier")
					copy.life--
					if !original.abilities["barrier"] || copy.life == original.life {
						t.Fatal("copy shares mutable properties")
					}
				}
			})
		}
	}
}

func TestCopyInheritsEffectsAndFormButResetsActions(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active = side
		id := strings.Repeat("1", 32)
		state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: id, CardID: 10134120, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		original := s.g.instances[id]
		original.attack, original.life, original.cost = 7, 4, 1
		original.evolved, original.superEvolved, original.engaged, original.departed = true, true, true, true
		original.attacksUsed, original.attackLimitValue, original.damageReduction = 2, 3, 1
		original.buffStats(2, 3, side)
		original.addKeyword("barrier", side)
		original.addKeyword("storm", oppositeSide(side))
		f := frame{"summoned": bindEntities(original)}
		s.g.execCardEffect(ir.CardEffect{Kind: "summon_copies", Owner: "own", Target: ir.BindingRef{Kind: "binding", Name: "summoned"}, Output: "summoned"}, original, f)
		if len(f["summoned"]) != 1 {
			t.Fatal("copy cleared its input binding")
		}
		copy := s.g.instances[f["summoned"][0].InstanceID]
		if copy.attack != 9 || copy.life != 7 || copy.cost != 1 || !copy.evolved || !copy.superEvolved || !copy.departed || copy.damageReduction != 1 || copy.attacksUsed != 0 || copy.engaged || !copy.summoningSick || attackLimit(copy) != 3 {
			t.Fatal("copy lost form, effects, damage or action reset")
		}
		if len(s.g.events) != 1 || s.g.events[0].Kind != "follower_summoned" || len(s.g.triggers) != 0 {
			t.Fatal("copy invoked fanfare or evolution")
		}
		if err := s.g.preflightAttack(ir.AttackAction{Kind: "attack_leader", Actor: side, Attacker: copy.id, Defender: oppositeSide(side)}); err != "" {
			t.Fatal("copied Storm follower cannot attack", err)
		}
		s.g.expireTurnEffects(side)
		if copy.attack != 7 || copy.life != 4 || copy.abilities["barrier"] || !copy.abilities["storm"] || original.attack != 7 {
			t.Fatal("copy changed effect deadline")
		}
		copy.buffStats(1, 1, oppositeSide(side))
		if original.attack != 7 || len(original.temporaryStats) != 0 {
			t.Fatal("copy shares temporary stat map")
		}
		s.g.expireTurnEffects(oppositeSide(side))
		if copy.abilities["storm"] || original.abilities["storm"] || copy.attack != 7 {
			t.Fatal("opponent deadline did not expire independently")
		}
	}
}

func TestCopyFusionHistoryIsIndependentAndRestorable(t *testing.T) {
	pack := repeatCardPack(t)
	for _, limited := range []bool{false, true} {
		state := testState()
		source, target := strings.Repeat("1", 32), strings.Repeat("2", 32)
		p := state.Players["own"]
		p.PP, p.MaxPP = 8, 8
		p = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10174130, DeclaredType: "follower"})
		state.Players["own"] = withInstance(p, "hand", ir.TestInstance{InstanceID: target, CardID: 90073110, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		original := s.g.instances[target]
		material := s.g.newInstance(s.g.cards[90071210], strings.Repeat("3", 32), "", "attached")
		nested := s.g.newInstance(s.g.cards[10133310], strings.Repeat("4", 32), "", "attached")
		nested.counters["x"] = 3
		material.materials, original.materials = []*instance{nested}, []*instance{material}
		if limited {
			s.budgetPolicy.CreatedInstances = 2
		}
		step := s.Submit(strings.Repeat("a", 32), SimulatorCommand{Kind: "play", Source: source})
		if step.Status != StatusSuspended || step.Choice.MinSelections != 1 {
			t.Fatal(step)
		}
		step = s.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{target}})
		if limited {
			if step.Status != StatusFault || step.ErrorCode != executionBudgetExceeded || len(s.g.own.field) != 1 || len(s.g.instances) != 4 || original.materials[0] != material {
				t.Fatal("copy budget created an incomplete attachment tree", step)
			}
			continue
		}
		if step.Status != StatusCompleted || len(s.g.own.field) != 2 || len(s.g.instances) != 7 {
			t.Fatal("missing copy attachment history", step)
		}
		copy := s.g.own.field[1]
		if len(copy.materials) != 1 || copy.materials[0] == material || copy.materials[0].materials[0] == nested || copy.materials[0].materials[0].counters["x"] != 3 {
			t.Fatal("fusion history shares instances or lost counters")
		}
		copy.materials[0].materials[0].counters["x"] = 4
		if nested.counters["x"] != 3 {
			t.Fatal("copy shares counter map")
		}
		checkpoint := snapshotContinuationGame(s.g)
		restored, err := restoreGame(s.g.cards, checkpoint)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(checkpoint, snapshotContinuationGame(restored)) {
			t.Fatal("copy history changed on continuation restore")
		}
	}
}

func TestCopyAmuletStateAndIgnoreUnsupportedZones(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	for n, entry := range []struct {
		card       int
		kind, zone string
	}{
		{10161210, "amulet", "field"},
		{10131320, "spell", "hand"},
		{10001110, "follower", "graveyard"},
		{10001110, "follower", "deck"},
	} {
		state.Players["own"] = withInstance(state.Players["own"], entry.zone, ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: entry.card, DeclaredType: entry.kind})
	}
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	source := s.g.own.field[0]
	source.engaged, source.countdown = true, 2
	f := frame{"sources": bindEntities(source, s.g.own.hand[0], s.g.own.graveyard[0], s.g.own.deck[0])}
	e := ir.CardEffect{Kind: "summon_copies", Owner: "own", Target: ir.BindingRef{Kind: "binding", Name: "sources"}, Output: "summoned"}
	s.g.execCardEffect(e, source, f)
	if len(f["summoned"]) != 1 || s.g.instances[f["summoned"][0].InstanceID].engaged || s.g.instances[f["summoned"][0].InstanceID].countdown != 2 || len(s.g.events) != 0 {
		t.Fatal("amulet copy lost countdown, retained engage, or copied invalid sources")
	}
	f["sources"] = nil
	s.g.execCardEffect(e, source, f)
	if len(f["summoned"]) != 0 {
		t.Fatal("empty copy retained previous output")
	}
}
