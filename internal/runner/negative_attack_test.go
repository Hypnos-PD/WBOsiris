package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestNegativeAttackQueriesClampEachFollowerBeforeReading(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	for n := 1; n <= 3; n++ {
		state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n), CardID: 10001110, DeclaredType: "follower"})
	}
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	a, b, c := s.g.own.field[0], s.g.own.field[1], s.g.own.field[2]
	a.buffStats(-5, 0, "") // -3, not zero: later buffs must still pay off this deficit.
	b.buffStats(-2, 0, "")
	ref := ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}
	if got := s.g.numericValue(&ir.SumExpr{Kind: "sum", Source: ref, Field: "attack"}, a, nil); got != 2 {
		t.Fatal("negative attack subtracted from another follower in a sum", got)
	}
	if got := s.g.numericValue(&ir.Scalar{Kind: "self_scalar", Field: "attack"}, a, nil); got != 0 {
		t.Fatal("self.attack exposed its signed modifier total", got)
	}
	if got := s.g.numericValue(&ir.SumExpr{Kind: "sum", Source: ref, Field: "base_attack"}, a, nil); got != 6 {
		t.Fatal("base attack was modified", got)
	}
	for _, tc := range []struct {
		direction string
		want      []*instance
	}{{"lowest", []*instance{a, b}}, {"highest", []*instance{c}}} {
		if got := s.g.extremumCandidates(s.g.own.field, &ir.SelectionExtremum{Direction: tc.direction, Field: "attack"}); !reflect.DeepEqual(got, tc.want) {
			t.Fatal("wrong zero-attack ties", tc.direction)
		}
	}
	c.buffStats(-2, 0, "")
	if got := s.g.extremumCandidates(s.g.own.field, &ir.SelectionExtremum{Direction: "highest", Field: "attack"}); !reflect.DeepEqual(got, s.g.own.field) {
		t.Fatal("negative and zero attack were not tied for highest")
	}
	// A buff amount read from another follower is zero, not its signed total.
	s.g.execTargetEffect(ir.TargetEffect{Kind: "buff_stats", Target: ir.BindingRef{Kind: "binding", Name: "target"}, AttackExpr: &ir.Scalar{Kind: "self_scalar", Field: "attack"}}, a, frame{"target": bindEntities(b)})
	if b.attack != 0 || a.attack != -3 {
		t.Fatal("numeric buff leaked or erased negative attack")
	}
	a.buffStats(2, 0, "own")
	copy := s.g.copyInstance(a)
	s.g.addToZone(&s.g.own, copy, "field")
	copy.buffStats(2, 0, "")
	if a.attack != -1 || copy.attack != 1 {
		t.Fatal("copy lost or shared the signed modifier total")
	}
	s.g.expireTurnEffects("own")
	if a.attack != -3 || copy.attack != -1 {
		t.Fatal("temporary buff expiry lost negative attack")
	}
	for _, viewer := range []string{"own", "oppo"} {
		view, _ := s.View(viewer)
		field := view.Own.Field
		if viewer == "oppo" {
			field = view.Oppo.Field
		}
		for _, entity := range field {
			if entity.Attack != 0 {
				t.Fatal("public view exposed negative attack", entity)
			}
		}
	}
	s.g.destroyByEffect([]*instance{copy})
	if len(s.g.own.destroyed) != 1 || s.g.own.destroyed[0].Attack != -1 {
		t.Fatal("destruction history discarded the signed total")
	}
	restored, err := restoreGame(s.g.cards, snapshotContinuationGame(s.g))
	if err != nil {
		t.Fatal(err)
	}
	if restored.instances[a.id].attack != -3 || restored.own.destroyed[0].Attack != -1 {
		t.Fatal("restoration clamped stored attack")
	}
	view, _ := s.View("own")
	if view.Own.Destroyed[0].Attack != 0 || view.Own.Graveyard[0].Attack != 0 {
		t.Fatal("graveyard or history view exposed negative attack")
	}
	s.g.returnCard(a, "hand")
	if a.attack != 2 {
		t.Fatal("field return retained the old signed modifier")
	}
}

func TestNegativeAttackCombatRestoresAndDealsZeroWithoutUsingBarrier(t *testing.T) {
	pack := repeatCardPack(t)
	pack.Cards = append(pack.Cards, ir.Card{ID: 77779501, CardType: "follower", Stats: &ir.Stats{Attack: 2, Life: 5}, Intrinsic: []string{"drain"}, Abilities: []ir.Ability{{
		ID: strings.Repeat("d", 32), Trigger: ir.SimpleTrigger{Kind: "attack"}, Body: []ir.Effect{
			ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}},
		},
	}}})
	for _, side := range []string{"own", "oppo"} {
		state := crestState(side)
		a, b := strings.Repeat("1", 32), strings.Repeat("2", 32)
		p := state.Players[side]
		p.Leader.Life = 10
		state.Players[side] = withInstance(p, "field", ir.TestInstance{InstanceID: a, CardID: 77779501, DeclaredType: "follower"})
		other := oppositeSide(side)
		state.Players[other] = withInstance(state.Players[other], "field", ir.TestInstance{InstanceID: b, CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Keywords: []string{"barrier"}}})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		s.g.instances[a].buffStats(-5, 0, "")
		s.g.instances[b].buffStats(-4, 0, "")
		step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "attack", Source: a, Defender: b})
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
		old, _ := DecodeContinuation(data)
		old.Version = "0.35.0"
		if _, err := RestoreSession(pack, old); err == nil {
			t.Fatal("accepted old negative-attack query semantics")
		}
		for _, current := range []*Session{s, restored} {
			if r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if current.g.instances[a].life != 5 || current.g.instances[b].life != 2 || !current.g.instances[b].abilities["barrier"] || current.g.player(side).leaderLife != 10 {
				t.Fatal("zero combat consumed Barrier, healed Drain, or changed life")
			}
			damage := 0
			for _, event := range current.Events() {
				if event.Kind == "damaged" {
					damage++
					if event.Actual != 0 {
						t.Fatal("negative combat caused damage", event)
					}
				}
			}
			if damage != 2 {
				t.Fatal("zero combat did not emit both damage facts")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored negative-attack combat diverged")
		}
	}
}
