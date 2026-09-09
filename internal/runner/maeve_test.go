package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/ruleset"
)

func TestMaeveChoosesBaseCostAndRestoresPrintedAmulet(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		state := crestState(owner)
		maeve, high, low := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)
		p := withInstance(state.Players[owner], "field", ir.TestInstance{InstanceID: maeve, CardID: 10162130, DeclaredType: "follower"})
		zero, ten, one := 0, 10, 1
		p = withInstance(p, "field", ir.TestInstance{InstanceID: high, CardID: 10022210, DeclaredType: "amulet", Overrides: ir.InstanceOverrides{Cost: &zero, Countdown: &one, Keywords: []string{"ward"}}})
		state.Players[owner] = withInstance(p, "destroyed", ir.TestInstance{InstanceID: low, CardID: 10062210, DeclaredType: "amulet", Overrides: ir.InstanceOverrides{Cost: &ten}})
		state.Players[oppositeSide(owner)] = withInstance(state.Players[oppositeSide(owner)], "destroyed", ir.TestInstance{InstanceID: strings.Repeat("d", 32), CardID: 10163220, DeclaredType: "amulet"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !s.g.instances[maeve].abilities["ward"] {
			t.Fatal("Maeve lost Ward")
		}
		// Returning and transforming the actual card must not change its historical identity.
		original := s.g.instances[high]
		s.g.destroyByEffect([]*instance{original})
		s.g.returnCard(original, "hand")
		s.g.execCardEffect(ir.CardEffect{Kind: "transform", Target: ir.SelfRef{Kind: "self"}, CardID: 10163220}, original, nil)
		s.g.destroyByEffect([]*instance{s.g.instances[maeve]})
		if r := s.run(); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		field := s.g.player(owner).field
		if len(field) != 1 || field[0].card.ID != 10022210 || field[0].cost != 4 || field[0].countdown != 4 || field[0].abilities["ward"] || field[0].departed || field[0].id == high {
			t.Fatal("resurrected modified state or wrong card", field)
		}
		if original.zone != "hand" || original.card.ID != 10163220 || len(s.g.player(owner).destroyed) != 3 || s.g.rng.Consumed() != 0 {
			t.Fatal("history summon moved the original, consumed history or RNG")
		}
		for _, viewer := range []string{"own", "oppo"} {
			view, _ := s.View(viewer)
			visible := view.Own
			if viewer != owner {
				visible = view.Oppo
			}
			if len(visible.Field) != 1 || visible.Field[0].CardID != 10022210 {
				t.Fatal("missing public summon")
			}
		}
	}
}

func TestMaeveWeightsEachDestructionRecordAndIgnoresOtherTypes(t *testing.T) {
	all := repeatCardPack(t)
	pack := &ir.CardPack{}
	for _, c := range all.Cards {
		if c.ID == 10162130 || c.ID == 10061210 || c.ID == 10022210 || c.ID == 10144110 {
			pack.Cards = append(pack.Cards, c)
		}
	}
	for _, owner := range []string{"own", "oppo"} {
		for seed := uint64(0); seed < 32; seed++ {
			state := testState()
			state.Turn.Active = owner
			maeve := strings.Repeat("a", 32)
			p := withInstance(state.Players[owner], "field", ir.TestInstance{InstanceID: maeve, CardID: 10162130, DeclaredType: "follower"})
			for n, id := range []int{10061210, 10022210, 10022210, 10022210, 10144110} {
				kind := "amulet"
				if id == 10144110 {
					kind = "follower"
				}
				p = withInstance(p, "destroyed", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: id, DeclaredType: kind})
			}
			state.Players[owner] = p
			s, err := NewSession(pack, state, seed)
			if err != nil {
				t.Fatal(err)
			}
			s.g.destroyByEffect([]*instance{s.g.instances[maeve]})
			if r := s.run(); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			want := []int{10061210, 10022210, 10022210, 10022210}[ruleset.NewRNG(seed).Index(4)]
			if len(s.g.player(owner).field) != 1 || s.g.player(owner).field[0].card.ID != want || s.g.rng.Consumed() != 1 {
				t.Fatal("weighted history lottery diverged", owner, seed, want)
			}
		}
	}
}

func TestHistorySummonSnapshotCountsCapacityAndBudget(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	id := strings.Repeat("a", 32)
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: id, CardID: 10002210, DeclaredType: "amulet"})
	s, err := NewSession(pack, state, 3)
	if err != nil {
		t.Fatal(err)
	}
	original := s.g.instances[id]
	for n := 0; n < 3; n++ {
		if n > 0 {
			s.g.returnCard(original, "hand")
			s.g.remove(&s.g.own.hand, original)
			s.g.addToZone(&s.g.own, original, "field")
		}
		s.g.destroyByEffect([]*instance{original})
	}
	history := cloneDestructionHistory(s.g.own.destroyed)
	e := ir.HistorySummonEffect{Kind: "summon_from_history", Owner: "own", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "destroyed", Member: "amulet"}, Count: 5, Output: "summoned"}
	out := s.g.summonFromHistory(e, nil, nil)
	if len(out) != 3 || len(s.g.own.field) != 3 || !reflect.DeepEqual(history, s.g.own.destroyed) || original.zone != "graveyard" {
		t.Fatal("repeated instance records collapsed or were consumed")
	}
	if len(s.g.own.hand) != 0 || s.g.gameOver {
		t.Fatal("summon incorrectly executed fanfare draw")
	}
	for _, i := range out {
		if i.card.ID != 10002210 || i.id == id {
			t.Fatal("not a fresh same-name card")
		}
	}
	// Only two slots remain; no third random choice is made for an impossible summon.
	before := s.g.rng.Consumed()
	out = s.g.summonFromHistory(e, nil, nil)
	if len(out) != 2 || s.g.rng.Consumed()-before != 2 {
		t.Fatal("capacity consumed extra choices")
	}
	before = s.g.rng.Consumed()
	if len(s.g.summonFromHistory(e, nil, nil)) != 0 || s.g.rng.Consumed() != before {
		t.Fatal("full field consumed randomness")
	}
	// A partial query must not spawn a card or consume a random choice.
	s.g.own.field = nil
	policy := ruleset.Default().ExecutionBudget
	policy.QueryVisits = 1
	s.g.budget = &budgetTracker{}
	s.g.budget.reset(policy)
	if len(s.g.summonFromHistory(e, nil, nil)) != 0 || !s.g.budget.exceeded || s.g.rng.Consumed() != before {
		t.Fatal("partial history query performed a summon")
	}
	policy = ruleset.Default().ExecutionBudget
	policy.Candidates = 1
	s.g.budget.reset(policy)
	if len(s.g.summonFromHistory(e, nil, nil)) != 0 || !s.g.budget.exceeded || s.g.rng.Consumed() != before {
		t.Fatal("history summon bypassed the candidate budget")
	}
}

func TestMaeveLastWordsAndSummonedBindingSurviveContinuation(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID == 10162130 {
			a := &pack.Cards[n].Abilities[0]
			pause := ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}}
			a.Body = append(a.Body, pause, ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "return", Target: ir.BindingRef{Kind: "binding", Name: "summoned"}, Destination: "hand"})
		}
	}
	for _, owner := range []string{"own", "oppo"} {
		state := crestState(owner)
		maeve, enemy := strings.Repeat("a", 32), strings.Repeat("b", 32)
		state.Players[owner] = withInstance(state.Players[owner], "field", ir.TestInstance{InstanceID: maeve, CardID: 10162130, DeclaredType: "follower"})
		state.Players[owner] = withInstance(state.Players[owner], "destroyed", ir.TestInstance{InstanceID: strings.Repeat("c", 32), CardID: 10061210, DeclaredType: "amulet"})
		state.Players[oppositeSide(owner)] = withInstance(state.Players[oppositeSide(owner)], "field", ir.TestInstance{InstanceID: enemy, CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: 7, Life: 9}}})
		s, err := NewSession(pack, state, 4)
		if err != nil {
			t.Fatal(err)
		}
		step := s.SubmitAs(strings.Repeat("d", 32), owner, SimulatorCommand{Kind: "attack", Source: maeve, Defender: enemy})
		if step.Status != StatusSuspended {
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
			if r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			p := current.g.player(owner)
			if len(p.field) != 0 || len(p.hand) != 1 || p.hand[0].card.ID != 10061210 || len(p.destroyed) != 2 {
				t.Fatal("summoned binding or history lost on resume")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("history summon continuation diverged")
		}
	}
}

func TestHistorySummonTurnWindowAndPreSummonRestore(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID != 10031310 {
			continue
		}
		pack.Cards[n].PlayEffects = []ir.Effect{
			ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}},
			ir.HistorySummonEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "summon_from_history", Owner: "own", Source: ir.HistoryRef{Kind: "history", Side: "own", Window: "this_turn", Member: "amulet"}, Count: 1, Extremum: &ir.SelectionExtremum{Direction: "highest", Field: "base_cost"}, Output: "summoned"},
		}
	}
	for _, owner := range []string{"own", "oppo"} {
		state := crestState(owner)
		spell, old, current := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)
		p := withInstance(state.Players[owner], "hand", ir.TestInstance{InstanceID: spell, CardID: 10031310, DeclaredType: "spell"})
		p = withInstance(p, "destroyed", ir.TestInstance{InstanceID: old, CardID: 10163220, DeclaredType: "amulet"})
		state.Players[owner] = withInstance(p, "field", ir.TestInstance{InstanceID: current, CardID: 10022210, DeclaredType: "amulet"})
		s, err := NewSession(pack, state, 12)
		if err != nil {
			t.Fatal(err)
		}
		i := s.g.instances[current]
		s.g.destroyByEffect([]*instance{i})
		s.g.returnCard(i, "hand")
		s.g.remove(&s.g.player(owner).hand, i)
		s.g.addToZone(s.g.player(owner), i, "field")
		s.g.destroyByEffect([]*instance{i})
		before := s.g.snapshot()
		s.LegalActionsFor(owner)
		if !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("preflight mutated history or RNG")
		}
		step := s.SubmitAs(strings.Repeat("d", 32), owner, SimulatorCommand{Kind: "play", Source: spell})
		if step.Status != StatusSuspended || s.g.rng.Consumed() != 0 || len(s.g.player(owner).field) != 0 {
			t.Fatal("summoned before pause", step)
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
		for _, session := range []*Session{s, restored} {
			if r := session.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			field := session.g.player(owner).field
			if len(field) != 1 || field[0].card.ID != 10022210 || session.g.rng.Consumed() != 1 {
				t.Fatal("turn window, duplicate history or random state lost")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("pre-summon restore diverged")
		}
	}
}
