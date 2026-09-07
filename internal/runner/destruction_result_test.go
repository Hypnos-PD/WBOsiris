package runner

import (
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestDestructionResultExcludesProtectedDuplicateAndCollateralDeaths(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	ids := []string{strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32), strings.Repeat("5", 32)}
	p := state.Players["own"]
	for n, id := range ids {
		zone, cardID, kind := "field", 10001110, "follower"
		if n == 1 {
			cardID, kind = 90031210, "amulet"
		}
		if n == 4 {
			zone = "hand"
		}
		p = withInstance(p, zone, ir.TestInstance{InstanceID: id, CardID: cardID, DeclaredType: kind})
	}
	state.Players["own"] = p
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	i := func(n int) *instance { return s.g.instances[ids[n]] }
	i(1).earthsigil = 3
	i(2).superEvolved = true
	i(3).life = 0
	got := s.g.destroyByEffect([]*instance{i(2), i(1), i(0), nil, i(0), i(4)})
	if len(got) != 1 || got[0] != i(0) {
		t.Fatal("count included an unsuccessful or collateral destruction", got)
	}
	if i(1).zone != "field" || i(2).zone != "field" || i(3).zone != "graveyard" || i(4).zone != "hand" {
		t.Fatal("wrong destruction zones")
	}
	if got := s.g.destroyByEffect([]*instance{i(0)}); len(got) != 0 {
		t.Fatal("destroyed a graveyard instance twice")
	}
}

func TestDestructionBindingOrderAndEmptyOverwrite(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	ids := []string{strings.Repeat("1", 32), strings.Repeat("2", 32)}
	p := state.Players["own"]
	for _, id := range ids {
		p = withInstance(p, "field", ir.TestInstance{InstanceID: id, CardID: 10001210, DeclaredType: "amulet"})
	}
	state.Players["own"] = p
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	f := frame{"targets": bindEntities(s.g.instances[ids[1]], s.g.instances[ids[0]])}
	e := ir.TargetEffect{Kind: "destroy", Output: "destroyed", Target: ir.BindingRef{Kind: "binding", Name: "targets"}}
	s.g.execTargetEffect(e, s.g.instances[ids[0]], f)
	if len(f["destroyed"]) != 2 || f["destroyed"][0].InstanceID != ids[0] || f["destroyed"][1].InstanceID != ids[1] {
		t.Fatal("result lost field death order")
	}
	s.g.execTargetEffect(e, s.g.instances[ids[0]], f)
	if len(f["destroyed"]) != 0 {
		t.Fatal("empty destruction kept prior result")
	}
}

func TestSkullfaneResultContinuationBothPlayers(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		card := &pack.Cards[n]
		if card.ID == 10163110 {
			body := card.Abilities[0].Body
			card.Abilities[0].Body = append([]ir.Effect{body[0], ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1}, {ID: 2}}}}, body[1:]...)
		}
	}
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active = side
		id, first, second, enemy := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32)
		p := state.Players[side]
		p.PP, p.MaxPP = 6, 6
		p = withInstance(p, "hand", ir.TestInstance{InstanceID: id, CardID: 10163110, DeclaredType: "follower"})
		p = withInstance(p, "field", ir.TestInstance{InstanceID: first, CardID: 10001210, DeclaredType: "amulet"})
		p = withInstance(p, "field", ir.TestInstance{InstanceID: second, CardID: 10163210, DeclaredType: "amulet"})
		state.Players[side] = p
		state.Players[oppositeSide(side)] = withInstance(state.Players[oppositeSide(side)], "field", ir.TestInstance{InstanceID: enemy, CardID: 10001110, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		before := s.g.snapshot()
		s.LegalActionsFor(side)
		if !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("legal actions mutated destruction state")
		}
		step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: id})
		if step.Status != StatusSuspended || s.g.player(side).shadows != 2 || s.g.player(oppositeSide(side)).leaderLife != 20 || len(s.g.player(side).field) != 1 {
			t.Fatal("destruction did not pause before damage and lastwords", step)
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
		response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}
		for _, current := range []*Session{s, restored} {
			if r := current.Resume(response); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if current.g.player(oppositeSide(side)).leaderLife != 18 || current.g.instances[enemy].zone != "graveyard" || len(current.g.player(side).field) != 2 {
				t.Fatal("destruction result lost across death, lastwords or restore")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored destruction diverged")
		}
	}
}
