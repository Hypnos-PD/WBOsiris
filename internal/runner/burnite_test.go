package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestBurniteDiscardCurrentCostAndContinuation(t *testing.T) {
	pack := repeatCardPack(t)
	// A visible callback event proves the discard listener waits for the entire fanfare.
	for n := range pack.Cards {
		if pack.Cards[n].ID == 10142110 {
			pack.Cards[n].Abilities[0].Body = append(pack.Cards[n].Abilities[0].Body, ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "own"}, Amount: 1})
		}
	}
	for _, owner := range []string{"own", "oppo"} {
		for _, cost := range []int{0, 2, 8} {
			t.Run(fmt.Sprintf("%s/%d", owner, cost), func(t *testing.T) {
				state := crestState(owner)
				source, held := strings.Repeat("a", 32), strings.Repeat("b", 32)
				p := withInstance(state.Players[owner], "hand", ir.TestInstance{InstanceID: source, CardID: 10144110, DeclaredType: "follower"})
				state.Players[owner] = withInstance(p, "hand", ir.TestInstance{InstanceID: held, CardID: 10142110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Cost: &cost}})
				for n := 0; n < 2; n++ {
					foe := oppositeSide(owner)
					state.Players[foe] = withInstance(state.Players[foe], "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: 1, Life: 10}}})
				}
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				step := s.SubmitAs(strings.Repeat("c", 32), owner, SimulatorCommand{Kind: "play", Source: source})
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
				response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{held}}
				for _, current := range []*Session{s, restored} {
					if r := current.Resume(response); r.Status != StatusCompleted {
						t.Fatal(r)
					}
					if current.g.instances[held].zone != "graveyard" || current.g.player(owner).shadows != 1 || current.g.instances[source].attack != 8 {
						t.Fatal("discard callback or destination missing")
					}
					for _, enemy := range current.g.player(oppositeSide(owner)).field {
						if enemy.life != 10-cost {
							t.Fatal("wrong current cost damage", enemy.life, cost)
						}
					}
					discarded, damaged, callback := -1, -1, -1
					for n, event := range current.g.events {
						if event.Kind == "card_discarded" {
							discarded = n
						}
						if event.Kind == "damaged" && event.Target != nil {
							if event.Target.Kind == "leader" {
								callback = n
							} else {
								damaged = n
							}
						}
					}
					if discarded < 0 || callback <= discarded || cost > 0 && (damaged <= discarded || callback <= damaged) {
						t.Fatal("discard callbacks ran before fanfare damage", current.g.events)
					}
				}
				if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
					t.Fatal("discard restore diverged")
				}
			})
		}
	}
}

func TestBurniteEmptyHandEvolutionAndPermanentOpponentCrest(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		for _, kind := range []string{"evolve", "superevolve"} {
			state := crestState(owner)
			id := strings.Repeat("a", 32)
			state.Players[owner] = withInstance(state.Players[owner], "hand", ir.TestInstance{InstanceID: id, CardID: 10144110, DeclaredType: "follower"})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			if r := s.SubmitAs(strings.Repeat("b", 32), owner, SimulatorCommand{Kind: "play", Source: id}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if r := s.SubmitAs(strings.Repeat("c", 32), owner, SimulatorCommand{Kind: kind, Source: id}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			foe := s.g.player(oppositeSide(owner))
			want := 0
			if kind == "superevolve" {
				want = 1
			}
			if len(foe.crests) != want || len(s.g.player(owner).crests) != 0 {
				t.Fatal("wrong crest owner/evolution")
			}
			if want == 0 {
				continue
			}
			crestID := foe.crests[0].id
			s.g.gainCrest(nil, oppositeSide(owner), 10144110)
			s.g.resolveDeathBatch([]*instance{s.g.instances[id]})
			if r := s.run(); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			for n := 0; n < 6; n++ {
				crestEndTurn(t, s)
			}
			if len(foe.crests) != 1 || foe.crests[0].id != crestID || foe.crests[0].countdown != 0 || foe.leaderLife != 17 {
				t.Fatal("permanent crest expired, duplicated or missed turn damage")
			}
		}
	}
}

func TestBurniteHealingOnlyOnceOnOwnerTurnAndOnlyActualHealing(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		state := crestState(oppositeSide(owner))
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		p := s.g.player(owner)
		s.g.gainCrest(nil, owner, 10144110)
		heal := func(amount int) {
			s.g.execTargetEffect(ir.TargetEffect{Kind: "heal", Target: ir.LeaderRef{Kind: "leader", Side: owner}, Amount: amount}, nil, nil)
			if r := s.run(); r.Status != StatusCompleted {
				t.Fatal(r)
			}
		}
		p.leaderLife = 10
		heal(2)
		if p.leaderLife != 12 {
			t.Fatal("crest fired outside owner turn")
		}
		crestEndTurn(t, s)
		if p.leaderLife != 11 {
			t.Fatal("turn start damage missing")
		}
		p.leaderLife = 20
		heal(2)
		p.leaderLife = 10
		heal(0)
		heal(3)
		if p.leaderLife != 12 {
			t.Fatal("zero/full heal consumed allowance or positive heal failed", p.leaderLife)
		}
		heal(2)
		if p.leaderLife != 14 {
			t.Fatal("crest fired twice")
		}
		crestEndTurn(t, s)
		crestEndTurn(t, s)
		heal(2)
		if p.leaderLife != 14 {
			t.Fatal("turn allowance did not reset", p.leaderLife)
		}
	}
}

func TestBurniteSalefaAndCombatDrain(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		for _, drain := range []bool{false, true} {
			state := crestState(owner)
			id, card, zone := strings.Repeat("a", 32), 10164130, "hand"
			override := ir.InstanceOverrides{}
			if drain {
				card, zone = 10001110, "field"
				override.Keywords = []string{"drain"}
				override.Stats = &ir.Stats{Attack: 3, Life: 3}
			}
			p := state.Players[owner]
			p.Leader.Life = 10
			state.Players[owner] = withInstance(p, zone, ir.TestInstance{InstanceID: id, CardID: card, DeclaredType: "follower", Overrides: override})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			s.g.gainCrest(nil, owner, 10144110)
			command := SimulatorCommand{Kind: "play", Source: id}
			if drain {
				command = SimulatorCommand{Kind: "attack", Source: id}
			}
			if r := s.SubmitAs(strings.Repeat("b", 32), owner, command); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if s.g.player(owner).leaderLife != 12 {
				t.Fatal("healing did not fire crest once", drain, s.g.player(owner).leaderLife)
			}
		}
	}
}

func TestBurniteLethalTurnStartPrecedesDraw(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		state := crestState(oppositeSide(owner))
		p := state.Players[owner]
		p.Leader.Life = 1
		state.Players[owner] = p
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		s.g.gainCrest(nil, owner, 10144110)
		crestEndTurn(t, s)
		if !s.g.gameOver || len(s.g.player(owner).hand) != 0 || len(s.LegalActionsFor(owner)) != 0 {
			t.Fatal("lethal crest allowed turn actions/draw")
		}
		if r := s.SubmitAs(strings.Repeat("e", 32), owner, SimulatorCommand{Kind: "end_turn"}); r.Status != StatusRejected || r.ErrorCode != "game_over" {
			t.Fatal("dead leader can act", r)
		}
	}
}

func TestLeaderHealedBindingAndOnceStateSurviveContinuation(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID == 10031310 {
			pack.Cards[n].PlayEffects = []ir.Effect{
				ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("c", 32)}, Kind: "heal", Target: ir.LeaderRef{Kind: "leader", Side: "own"}, Amount: 2},
				ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "heal", Target: ir.LeaderRef{Kind: "leader", Side: "own"}, Amount: 2},
			}
		}
		if pack.Cards[n].ID != 10144110 {
			continue
		}
		a := &pack.Cards[n].Crest.Abilities[1]
		a.Body = []ir.Effect{
			ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}},
			ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "damage", Target: ir.BindingRef{Kind: "binding", Name: "healed"}, Amount: 1},
		}
	}
	for _, owner := range []string{"own", "oppo"} {
		state := crestState(owner)
		id := strings.Repeat("a", 32)
		state.Players[owner] = withInstance(state.Players[owner], "hand", ir.TestInstance{InstanceID: id, CardID: 10031310, DeclaredType: "spell"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		s.g.gainCrest(nil, owner, 10144110)
		s.g.player(owner).leaderLife = 10
		step := s.SubmitAs(strings.Repeat("b", 32), owner, SimulatorCommand{Kind: "play", Source: id})
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
			response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}
			if r := current.Resume(response); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if current.g.player(owner).leaderLife != 13 {
				t.Fatal("leader binding or queued once limit lost")
			}
			current.g.healLeader(current.g.player(owner), owner, 1)
			if r := current.run(); r.Status != StatusCompleted || current.g.player(owner).leaderLife != 14 {
				t.Fatal("restored trigger allowance lost", r)
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("healing restore diverged")
		}
	}
}

func TestSumCurrentAttributesAndHistoricalSnapshots(t *testing.T) {
	pack := repeatCardPack(t)
	state := crestState("own")
	id, spell := strings.Repeat("a", 32), strings.Repeat("b", 32)
	cost := 8
	p := withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: id, CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Cost: &cost, Stats: &ir.Stats{Attack: 7, Life: 9}}})
	state.Players["own"] = withInstance(p, "hand", ir.TestInstance{InstanceID: spell, CardID: 10031310, DeclaredType: "spell"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	bindings := frame{"chosen": bindEntities(s.g.instances[id], s.g.instances[spell])}
	for _, tc := range []struct {
		field string
		want  int
	}{{"cost", 9}, {"attack", 7}, {"life", 9}, {"base_cost", 3}, {"base_attack", 2}, {"base_life", 2}} {
		if got := s.g.numericValue(&ir.SumExpr{Kind: "sum", Source: ir.BindingRef{Kind: "binding", Name: "chosen"}, Field: tc.field}, nil, bindings); got != tc.want {
			t.Fatal(tc, got)
		}
		if got := s.g.numericValue(&ir.SumExpr{Kind: "sum", Source: ir.BindingRef{Kind: "binding", Name: "empty"}, Field: tc.field}, nil, bindings); got != 0 {
			t.Fatal("empty sum", got)
		}
	}
	s.g.resolveDeathBatch([]*instance{s.g.instances[id]})
	s.g.instances[id].cost, s.g.instances[id].attack, s.g.instances[id].life = 0, 1, 1
	for _, tc := range []struct {
		field string
		want  int
	}{{"cost", 8}, {"attack", 7}, {"life", 9}} {
		if got := s.g.numericValue(&ir.SumExpr{Kind: "sum", Source: ir.HistoryRef{Kind: "history", Side: "own"}, Field: tc.field}, nil, nil); got != tc.want {
			t.Fatal("history snapshot changed", tc, got)
		}
	}
}
