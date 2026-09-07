package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"wbo/internal/ir"
)

func TestJunoUsesSigilLayersWithoutSpendingAndRestoresSelection(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		for _, layers := range []int{0, 1, 4} {
			state := crestState(owner)
			source, target := strings.Repeat("a", 32), strings.Repeat("b", 32)
			state.Players[owner] = withInstance(state.Players[owner], "hand", ir.TestInstance{InstanceID: source, CardID: 10133110, DeclaredType: "follower"})
			state.Players[oppositeSide(owner)] = withInstance(state.Players[oppositeSide(owner)], "field", ir.TestInstance{InstanceID: target, CardID: 10163130, DeclaredType: "follower"})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			if layers > 0 {
				s.g.execAdjust(ir.AdjustEffect{Kind: "adjust_earthsigil", Owner: owner, Delta: layers}, nil, frame{})
			}
			step := s.SubmitAs(strings.Repeat("c", 32), owner, SimulatorCommand{Kind: "play", Source: source})
			if step.Status != StatusSuspended {
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
			for _, current := range []*Session{s, restored} {
				r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{target}})
				if r.Status != StatusCompleted || current.g.instances[target].life != 6-layers {
					t.Fatal("wrong Juno damage", r)
				}
				if n := current.g.numericValue(&ir.Scalar{Kind: "scalar", Side: "own", Field: "earthsigils"}, current.g.instances[source], nil); n != layers {
					t.Fatal("Juno spent sigils", n)
				}
			}
			if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
				t.Fatal("Juno continuation diverged")
			}
		}
	}
}

func TestJunoCrestPaysRiteBeforeCheckingFieldCapacity(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		for _, layers := range []int{0, 1, 2} {
			s, err := NewSession(pack, crestState(owner), 1)
			if err != nil {
				t.Fatal(err)
			}
			s.g.gainCrest(nil, owner, 10133110)
			if layers > 0 {
				s.g.execAdjust(ir.AdjustEffect{Kind: "adjust_earthsigil", Owner: owner, Delta: layers}, nil, frame{})
			}
			s.g.summonFor(nil, owner, 5, 10001110, false)
			crestEndTurn(t, s)
			p := s.g.player(owner)
			golems := 0
			for _, i := range p.field {
				if i.card.ID == 90031120 {
					golems++
					if !i.abilities["ward"] {
						t.Fatal("golem missing Ward")
					}
				}
			}
			want := 0
			if layers == 1 {
				want = 1
			}
			if golems != want || len(p.field) != 5 || p.crests[0].countdown != 3 {
				t.Fatal("wrong full-field rite", layers, golems)
			}
			if n := s.g.numericValue(&ir.Scalar{Kind: "scalar", Side: owner, Field: "earthsigils"}, nil, nil); n != max(0, layers-1) {
				t.Fatal("wrong rite payment", n)
			}
		}
	}
}

func TestEudieThresholdIsExclusiveAtTurnEnd(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		for _, size := range []int{0, 5, 6, 9} {
			state := crestState(owner)
			p := state.Players[owner]
			p.Leader.Life = 17
			for n := 0; n < size; n++ {
				p = withInstance(p, "hand", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 10001110, DeclaredType: "follower"})
			}
			state.Players[owner] = p
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			s.g.gainCrest(nil, owner, 10174110)
			crestEndTurn(t, s)
			wantHand, wantLife := size, 18
			if size <= 5 {
				wantHand, wantLife = size+1, 17
			}
			if p := s.g.player(owner); len(p.hand) != wantHand || p.leaderLife != wantLife {
				t.Fatal("Eudie threshold", owner, size, len(p.hand), p.leaderLife)
			}
		}
	}
}

func TestConditionalCrestDecisionSurvivesEarlierAbilityAndContinuation(t *testing.T) {
	for _, discard := range []bool{false, true} {
		pack := repeatCardPack(t)
		for n := range pack.Cards {
			if pack.Cards[n].ID != 10114110 {
				continue
			}
			a := &pack.Cards[n].Crest.Abilities[0]
			a.Trigger = ir.EventTrigger{Kind: "event", Event: "turn_ended", Side: "own"}
			change := ir.Effect(ir.DrawEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "draw", Owner: "own", SourceZone: "deck", Output: "drawn", Count: 1})
			if discard {
				change = ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "discard", Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand"}}
			}
			a.Body = []ir.Effect{change, ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}}}
		}
		for _, owner := range []string{"own", "oppo"} {
			state := crestState(owner)
			p := state.Players[owner]
			p.Leader.Life = 17
			size := 5
			if discard {
				size = 6
			}
			for n := 0; n < size; n++ {
				p = withInstance(p, "hand", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 10001110, DeclaredType: "follower"})
			}
			state.Players[owner] = p
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			s.g.gainCrest(nil, owner, 10114110)
			s.g.gainCrest(nil, owner, 10174110)
			step := crestEndTurn(t, s)
			if step.Status != StatusSuspended || len(s.g.triggers) != 1 {
				t.Fatal("wrong queued decision", step, len(s.g.triggers))
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
				r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1})
				if r.Status != StatusCompleted {
					t.Fatal(r)
				}
				wantHand, wantLife := 7, 17
				if discard {
					wantHand, wantLife = 0, 18
				}
				if p := current.g.player(owner); len(p.hand) != wantHand || p.leaderLife != wantLife {
					t.Fatal("condition reevaluated after queue", len(p.hand), p.leaderLife)
				}
			}
			if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
				t.Fatal("conditional crest continuation diverged")
			}
		}
	}
}

func TestEventConditionIsCheckedBeforeConsumingTurnLimit(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID != 10114110 {
			continue
		}
		a := &pack.Cards[n].Crest.Abilities[0]
		trigger := a.Trigger.(ir.EventTrigger)
		trigger.OncePerTurn = "own"
		trigger.Condition = ir.CompareCondition{Kind: "compare", Left: ir.Scalar{Kind: "scalar", Side: "own", Field: "hand_count"}, Op: "ge", Right: 1}
		a.Trigger = trigger
	}
	s, err := NewSession(pack, crestState("own"), 1)
	if err != nil {
		t.Fatal(err)
	}
	s.g.gainCrest(nil, "own", 10114110)
	first := s.g.summonFor(nil, "own", 1, 90011110, false)[0]
	if len(s.g.triggers) != 0 || len(s.g.own.crests[0].usedTriggers) != 0 {
		t.Fatal("false condition consumed limit")
	}
	s.g.execCardEffect(ir.CardEffect{Kind: "add_card", Owner: "own", Count: 1, CardID: 10001110, Destination: "hand"}, nil, frame{})
	second := s.g.summonFor(nil, "own", 1, 90011110, false)[0]
	third := s.g.summonFor(nil, "own", 1, 90011110, false)[0]
	if r := s.run(); r.Status != StatusCompleted {
		t.Fatal(r)
	}
	if first.abilities["storm"] || !second.abilities["storm"] || third.abilities["storm"] {
		t.Fatal("wrong conditional turn limit")
	}
}

func TestFieldTurnStartConditionAndEffectsResolveBeforeRuleDraw(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID != 10133110 {
			continue
		}
		pack.Cards[n].Abilities = []ir.Ability{{ID: strings.Repeat("d", 32), Trigger: ir.EventTrigger{Kind: "event", Event: "turn_started", Side: "own", Condition: ir.CompareCondition{Kind: "compare", Left: ir.Scalar{Kind: "scalar", Side: "own", Field: "hand_count"}, Op: "eq", Right: 1}}, Body: []ir.Effect{
			ir.AdjustEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "adjust_entity_field", Field: "cost", Delta: 1, Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand"}},
			ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}},
		}}}
	}
	for _, owner := range []string{"own", "oppo"} {
		state := crestState(oppositeSide(owner))
		state.Players[owner] = withInstance(state.Players[owner], "field", ir.TestInstance{InstanceID: strings.Repeat("a", 32), CardID: 10133110, DeclaredType: "follower"})
		state.Players[owner] = withInstance(state.Players[owner], "hand", ir.TestInstance{InstanceID: strings.Repeat("b", 32), CardID: 10001110, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		step := crestEndTurn(t, s)
		if step.Status != StatusSuspended || s.g.turnTransition != "starting_draw" || len(s.g.player(owner).hand) != 1 || s.g.player(owner).hand[0].cost != 3 {
			t.Fatal("field listener ran after draw", step)
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
			r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1})
			p := current.g.player(owner)
			if r.Status != StatusCompleted || len(p.hand) != 2 || p.hand[1].cost != 2 {
				t.Fatal("ordinary draw occurred at wrong phase", r)
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("turn-start restore diverged")
		}
	}
}
