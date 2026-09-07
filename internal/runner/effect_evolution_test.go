package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestCompiledAbilityEvolutionGrantsStatsAndPreservesManualAllowance(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			state := testState()
			state.Turn.Active, state.Turn.Number = side, 5
			actor := state.Players[side]
			actor.PP, actor.MaxPP, actor.Combo, actor.EP = 4, 5, 2, 1
			sourceID, allyID, enemyID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10011130, DeclaredType: "follower"})
			state.Players[side] = withInstance(actor, "field", ir.TestInstance{InstanceID: allyID, CardID: 10001110, DeclaredType: "follower"})
			enemy := oppositeSide(side)
			state.Players[enemy] = withInstance(state.Players[enemy], "field", ir.TestInstance{InstanceID: enemyID, CardID: 10001110, DeclaredType: "follower"})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			if step := s.SubmitAs(strings.Repeat("4", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); step.Status != StatusCompleted {
				t.Fatal(step)
			}
			source := s.g.instances[sourceID]
			if source.attack != 6 || source.life != 6 || !source.evolved || source.superEvolved || s.g.player(side).ep != 1 || s.g.player(side).evolvedThisTurn {
				t.Fatal("ability evolution lost stats or spent manual allowance")
			}
			if s.g.preflightAttack(ir.AttackAction{Kind: "attack_entity", Actor: side, Attacker: sourceID, Defender: enemyID}) != "" {
				t.Fatal("evolved newcomer cannot attack follower")
			}
			if s.g.preflightAttack(ir.AttackAction{Kind: "attack_leader", Actor: side, Attacker: sourceID, Defender: enemy + ".leader"}) == "" {
				t.Fatal("evolution gave storm")
			}
			if step := s.SubmitAs(strings.Repeat("5", 32), side, SimulatorCommand{Kind: "evolve", Source: allyID}); step.Status != StatusCompleted {
				t.Fatal("manual evolution after ability evolution failed", step)
			}
		})
	}
}

func TestCompiledRemiEvolutionIsIdempotentButFollowingBuffStillApplies(t *testing.T) {
	pack := repeatCardPack(t)
	for _, evolved := range []bool{false, true} {
		state := testState()
		state.Turn.Number = 8
		own := state.Players["own"]
		own.SEP = 1
		sourceID, targetID := strings.Repeat("1", 32), strings.Repeat("2", 32)
		own = withInstance(own, "field", ir.TestInstance{InstanceID: sourceID, CardID: 10032110, DeclaredType: "follower"})
		state.Players["own"] = withInstance(own, "field", ir.TestInstance{InstanceID: targetID, CardID: 90031120, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Evolved: &evolved}})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		step := s.Submit(strings.Repeat("3", 32), SimulatorCommand{Kind: "superevolve", Source: sourceID})
		if step.Status != StatusSuspended {
			t.Fatal(step)
		}
		step = s.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{targetID}})
		want := 8
		if evolved {
			want = 6
		}
		if step.Status != StatusCompleted || s.g.instances[targetID].attack != want || s.g.instances[targetID].life != want {
			t.Fatal("Remi evolution did not preserve following +3/+3", step)
		}
	}
}

func TestCompiledOliviaFiltersTargetsRestoresAndSkipsKeywordAbilities(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active, state.Turn.Number = side, 8
		actor := state.Players[side]
		actor.SEP = 1
		ids := []string{strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32)}
		evolved := true
		for n, id := range []int{10104110, 10001120, 10001110, 10001110} {
			card := ir.TestInstance{InstanceID: ids[n], CardID: id, DeclaredType: "follower"}
			if n >= 2 {
				card.Overrides.Evolved = &evolved
			}
			if n == 3 {
				card.Overrides.SuperEvolved = &evolved
			}
			actor = withInstance(actor, "field", card)
		}
		state.Players[side] = withInstance(actor, "deck", ir.TestInstance{InstanceID: strings.Repeat("5", 32), CardID: 10001110, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		step := s.SubmitAs(strings.Repeat("6", 32), side, SimulatorCommand{Kind: "superevolve", Source: ids[0]})
		if step.Status != StatusSuspended || len(step.Choice.Candidates) != 1 || step.Choice.Candidates[0].InstanceID != ids[1] {
			t.Fatalf("wrong Olivia targets: %#v", step)
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
		choice := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{ids[1]}}
		for _, session := range []*Session{s, restored} {
			if result := session.Resume(choice); result.Status != StatusCompleted {
				t.Fatal(result)
			}
			target := session.g.instances[ids[1]]
			if !target.superEvolved || !target.evolved || target.attack != 3 || target.life != 5 || !target.abilities["ward"] {
				t.Fatal("wrong effect super evolution state")
			}
			if len(session.g.player(side).hand) != 0 || len(session.g.player(side).deck) != 1 || session.g.player(side).sep != 0 {
				t.Fatal("effect evolution triggered keyword ability or consumed points")
			}
			before := len(session.g.events)
			session.g.applyEvolution(target, false)
			session.g.applyEvolution(target, true)
			if len(session.g.events) != before || target.attack != 3 {
				t.Fatal("repeated evolution changed state")
			}
			session.g.damageInstance(target, 5)
			session.g.destroyByEffect([]*instance{target})
			if target.life != 5 || target.zone != "field" {
				t.Fatal("effect super evolution lost own-turn protection")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) || !reflect.DeepEqual(s.Events(), restored.Events()) {
			t.Fatal("restored evolution diverged")
		}
	}
}

func TestCompiledSelfEvolutionEventsObserveOnlyTheirOwnInstance(t *testing.T) {
	pack := repeatCardPack(t)
	for _, id := range []int{10143120, 10133130, 10153130} {
		for _, method := range []string{"play", "evolve", "superevolve"} {
			for _, side := range []string{"own", "oppo"} {
				t.Run(fmt.Sprintf("%d/%s/%s", id, method, side), func(t *testing.T) {
					state := testState()
					state.Turn.Active, state.Turn.Number = side, 8
					actor := state.Players[side]
					actor.PP, actor.MaxPP, actor.EP, actor.SEP, actor.Shadows = 7, 7, 1, 1, 8
					sourceID := strings.Repeat("1", 32)
					zone := "field"
					if method == "play" {
						zone = "hand"
					}
					actor = withInstance(actor, zone, ir.TestInstance{InstanceID: sourceID, CardID: id, DeclaredType: "follower"})
					actor = withInstance(actor, "field", ir.TestInstance{InstanceID: strings.Repeat("2", 32), CardID: id, DeclaredType: "follower"})
					state.Players[side] = actor
					s, err := NewSession(pack, state, 1)
					if err != nil {
						t.Fatal(err)
					}
					if method == "play" && id == 10133130 {
						sigil := s.g.summonFor(s.g.instances[sourceID], "own", 1, 10031210, false)
						if len(sigil) != 1 {
							t.Fatal("missing earth sigil")
						}
						sigil[0].earthsigil = 2
					}
					step := s.SubmitAs(strings.Repeat("3", 32), side, SimulatorCommand{Kind: method, Source: sourceID})
					if step.Status != StatusCompleted || !s.g.instances[sourceID].evolved {
						t.Fatal(step)
					}
					p := s.g.player(side)
					if id == 10143120 && p.maxpp != 8 {
						t.Fatal("same-name evolution observer also fired")
					}
					if id == 10133130 {
						want := 7
						if method == "play" {
							want = 5
						}
						if p.pp != want {
							t.Fatalf("wrong spellcaster PP: %d", p.pp)
						}
					}
					if id == 10153130 {
						ghosts := 0
						for _, follower := range p.field {
							if follower.card.ID == 90051130 {
								ghosts++
								if !follower.abilities["bane"] || !follower.abilities["storm"] {
									t.Fatal("ghost lacks granted bane")
								}
							}
						}
						if ghosts != 1 {
							t.Fatal("self evolution triggered another instance")
						}
					}
				})
			}
		}
	}
}

func TestEvolutionEventWaitsForFanfareAndSurvivesContinuation(t *testing.T) {
	for _, super := range []bool{false, true} {
		form, attack := "evolved", 8
		if super {
			form, attack = "super_evolved", 9
		}
		sourceID, targetID := strings.Repeat("1", 32), strings.Repeat("2", 32)
		self := ir.SelfRef{Kind: "self"}
		pack := &ir.CardPack{Cards: []ir.Card{
			{ID: 66666601, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{
				{ID: strings.Repeat("a", 32), Trigger: ir.SimpleTrigger{Kind: "fanfare"}, Body: []ir.Effect{
					ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "silent_evolve", Target: self, Form: form},
					ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("c", 32)}, Kind: "choose", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}},
					ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "buff_stats", Target: self, AttackDelta: 5},
				}},
				{ID: strings.Repeat("e", 32), Trigger: ir.EventTrigger{Kind: "event", Event: "evolved", Side: "own", SubjectType: "follower", SelfOnly: true}, Body: []ir.Effect{
					ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "own"}, AmountExpr: &ir.Scalar{Kind: "self_scalar", Field: "attack"}, DamageType: "effect"},
				}},
			}},
			{ID: 66666602, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
		}}
		state := testState()
		state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, CardID: 66666601, DeclaredType: "follower"})
		state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: targetID, CardID: 66666602, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		step := s.Submit(strings.Repeat("3", 32), SimulatorCommand{Kind: "play", Source: sourceID})
		if step.Status != StatusSuspended || s.g.own.leaderLife != 20 || len(s.g.triggers) != 1 {
			t.Fatalf("evolution trigger interrupted fanfare: %#v", step)
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
		choice := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{targetID}}
		for _, session := range []*Session{s, restored} {
			if result := session.Resume(choice); result.Status != StatusCompleted || session.g.own.leaderLife != 20-attack {
				t.Fatal("wrong deferred evolution event", result)
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) || !reflect.DeepEqual(s.Events(), restored.Events()) {
			t.Fatal("queued evolution restoration diverged")
		}
	}
}

func TestFanfareCancelsDepartedFieldListenerButKeepsLastwords(t *testing.T) {
	sourceID, victimID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	heal := func(amount int) []ir.Effect {
		return []ir.Effect{ir.TargetEffect{Kind: "heal", Target: ir.LeaderRef{Kind: "leader", Side: "own"}, Amount: amount}}
	}
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 66666603, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{{ID: strings.Repeat("a", 32), Trigger: ir.SimpleTrigger{Kind: "fanfare"}, Body: []ir.Effect{ir.TargetEffect{Kind: "destroy", Target: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}}}}}},
		{ID: 66666604, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{
			{ID: strings.Repeat("b", 32), Trigger: ir.EventTrigger{Kind: "event", Event: "follower_summoned", Side: "oppo", SubjectType: "follower"}, Body: heal(5)},
			{ID: strings.Repeat("c", 32), Trigger: ir.SimpleTrigger{Kind: "lastwords"}, Body: heal(1)},
		}},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, CardID: 66666603, DeclaredType: "follower"})
	oppo := state.Players["oppo"]
	oppo.Leader.Life = 10
	state.Players["oppo"] = withInstance(oppo, "field", ir.TestInstance{InstanceID: victimID, CardID: 66666604, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result := s.Submit(strings.Repeat("3", 32), SimulatorCommand{Kind: "play", Source: sourceID}); result.Status != StatusCompleted || s.g.oppo.leaderLife != 11 {
		t.Fatal("fanfare did not precede entry listeners or lost lastwords", result)
	}
}
