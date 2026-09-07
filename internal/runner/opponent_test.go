package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"wbo/internal/ir"
)

func TestMedusaAttacksThreeTimesBeforeClash(t *testing.T) {
	pack := repeatCardPack(t)
	pack.Cards = append(pack.Cards, ir.Card{ID: 88880001, CardType: "follower", Stats: &ir.Stats{Attack: 8, Life: 8}, Intrinsic: []string{"barrier", "aura", "ability_target_guard"}, Abilities: []ir.Ability{{
		ID: strings.Repeat("e", 32), Trigger: ir.SimpleTrigger{Kind: "clash"}, Body: []ir.Effect{ir.TargetEffect{Kind: "damage", Target: ir.BindingRef{Kind: "binding", Name: "opponent"}, Amount: 3}},
	}}})
	for _, side := range []string{"own", "oppo"} {
		for _, super := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/super%t", side, super), func(t *testing.T) {
				state := testState()
				state.Turn.Active = side
				source := strings.Repeat("a", 32)
				state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: source, CardID: 10154120, DeclaredType: "follower"})
				other := oppositeSide(side)
				for n := 1; n <= 4; n++ {
					state.Players[other] = withInstance(state.Players[other], "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n), CardID: 88880001, DeclaredType: "follower"})
				}
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				s.g.instances[source].superEvolved = super
				s.g.instances[source].summoningSick = true
				if r := s.Begin(strings.Repeat("b", 32), ir.AttackAction{Kind: "attack_leader", Actor: side, Attacker: source, Defender: other}); r.IllegalCode != "rush_cannot_attack_leader" {
					t.Fatal(r)
				}
				for n := 1; n <= 4; n++ {
					target := fmt.Sprintf("%032x", n)
					r := s.Begin(fmt.Sprintf("%032x", n+100), ir.AttackAction{Kind: "attack_entity", Actor: side, Attacker: source, Defender: target})
					if n <= 3 {
						if r.Status != StatusCompleted || s.g.instances[target].zone != "graveyard" || s.g.instances[source].life != 7 {
							t.Fatal("strike/clash order", r)
						}
					} else if r.Status != StatusIllegal || s.g.instances[target].zone != "field" {
						t.Fatal("fourth attack accepted", r)
					}
				}
				wantLife := 20
				if super {
					wantLife = 17
				}
				if s.g.player(other).leaderLife != wantLife {
					t.Fatal("super bonus", s.g.player(other).leaderLife)
				}
				for n, end := range []string{side, other} {
					if r := s.Begin(fmt.Sprintf("%032x", n+200), ir.SourceAction{Kind: "end_turn", Actor: end}); r.Status != StatusCompleted {
						t.Fatal(r)
					}
				}
				before := s.g.player(other).leaderLife
				if r := s.Begin(strings.Repeat("c", 32), ir.AttackAction{Kind: "attack_leader", Actor: side, Attacker: source, Defender: other}); r.Status != StatusCompleted || s.g.player(other).leaderLife != before-3 {
					t.Fatal("leader binding or attack reset", r)
				}
			})
		}
	}
}

func TestSuperAttackBonusRequiresDestruction(t *testing.T) {
	for _, phase := range []string{"attack", "clash_attacker", "clash_defender"} {
		for _, effect := range []string{"destroy", "return", "banish"} {
			t.Run(phase+"/"+effect, func(t *testing.T) {
				pack := &ir.CardPack{Cards: []ir.Card{
					{ID: 88880002, CardType: "follower", Stats: &ir.Stats{Attack: 3, Life: 7}},
					{ID: 88880003, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 8}},
				}}
				index, trigger := 0, "attack"
				var target ir.Ref = ir.BindingRef{Kind: "binding", Name: "opponent"}
				if phase != "attack" {
					trigger = "clash"
				}
				if phase == "clash_defender" {
					index, target = 1, ir.SelfRef{Kind: "self"}
				}
				pack.Cards[index].Abilities = []ir.Ability{{ID: strings.Repeat("a", 32), Trigger: ir.SimpleTrigger{Kind: trigger}, Body: []ir.Effect{ir.TargetEffect{Kind: effect, Target: target, Destination: "hand"}}}}
				s, attacker, defender := combatOpponentSession(t, pack)
				s.g.instances[attacker].superEvolved = true
				r := s.Begin(strings.Repeat("b", 32), ir.AttackAction{Kind: "attack_entity", Actor: "own", Attacker: attacker, Defender: defender})
				want := 20
				if effect == "destroy" {
					want = 19
				}
				if r.Status != StatusCompleted || s.g.oppo.leaderLife != want || s.g.instances[defender].zone == "field" {
					t.Fatal(r, s.g.oppo.leaderLife)
				}
			})
		}
	}
}

func TestDefendingClashBindsAttacker(t *testing.T) {
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 88880002, CardType: "follower", Stats: &ir.Stats{Attack: 3, Life: 7}},
		{ID: 88880003, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 8}, Abilities: []ir.Ability{{ID: strings.Repeat("a", 32), Trigger: ir.SimpleTrigger{Kind: "clash"}, Body: []ir.Effect{ir.TargetEffect{Kind: "destroy", Target: ir.BindingRef{Kind: "binding", Name: "opponent"}}}}}},
	}}
	s, attacker, defender := combatOpponentSession(t, pack)
	r := s.Begin(strings.Repeat("b", 32), ir.AttackAction{Kind: "attack_entity", Actor: "own", Attacker: attacker, Defender: defender})
	if r.Status != StatusCompleted || s.g.instances[attacker].zone != "graveyard" || s.g.instances[defender].life != 8 {
		t.Fatal("defensive clash bound wrong instance", r)
	}
}

func combatOpponentSession(t *testing.T, pack *ir.CardPack) (*Session, string, string) {
	t.Helper()
	attacker, defender := strings.Repeat("1", 32), strings.Repeat("2", 32)
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: attacker, CardID: pack.Cards[0].ID, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: defender, CardID: pack.Cards[1].ID, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	return s, attacker, defender
}

func TestCombatOpponentAndDestructionSurviveContinuation(t *testing.T) {
	for _, after := range []bool{false, true} {
		for _, phase := range []string{"attack", "clash"} {
			t.Run(fmt.Sprintf("%s/after%t", phase, after), func(t *testing.T) {
				destroy := ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("a", 32)}, Kind: "destroy", Target: ir.BindingRef{Kind: "binding", Name: "opponent"}}
				mode := ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}}
				body := []ir.Effect{mode, destroy}
				if after {
					body = []ir.Effect{destroy, mode}
				}
				pack := &ir.CardPack{Cards: []ir.Card{
					{ID: 88880002, CardType: "follower", Stats: &ir.Stats{Attack: 3, Life: 7}, Abilities: []ir.Ability{{ID: strings.Repeat("c", 32), Trigger: ir.SimpleTrigger{Kind: phase}, Body: body}}},
					{ID: 88880003, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 8}},
				}}
				s, attacker, defender := combatOpponentSession(t, pack)
				s.g.instances[attacker].superEvolved = true
				step := s.Begin(strings.Repeat("d", 32), ir.AttackAction{Kind: "attack_entity", Actor: "own", Attacker: attacker, Defender: defender})
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
				if saved.Game.Attack.DefenderDestroyed != after {
					t.Fatal("lost destruction flag")
				}
				restored, err := RestoreSession(pack, saved)
				if err != nil {
					t.Fatal(err)
				}
				saved.Game.Attack.DefenderDestroyed = !after
				if _, err := RestoreSession(pack, saved); err == nil {
					t.Fatal("accepted modified destruction flag")
				}
				response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}
				for _, current := range []*Session{s, restored} {
					if r := current.Resume(response); r.Status != StatusCompleted || current.g.oppo.leaderLife != 19 || current.g.instances[defender].zone != "graveyard" {
						t.Fatal(r)
					}
				}
				if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
					t.Fatal("continuation diverged")
				}
			})
		}
	}
}
