package runner

import (
	"maps"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestTemporaryKeywordGrantsRemainIndependent(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	id := strings.Repeat("1", 32)
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: id, CardID: 10001110, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	i := s.g.instances[id]
	i.addKeyword("storm", "own")
	i.addKeyword("storm", "own")
	i.addKeyword("storm", "oppo")
	s.g.expireKeywords("own")
	if !i.abilities["storm"] {
		t.Fatal("expired the other player's grant")
	}
	s.g.expireKeywords("oppo")
	if i.abilities["storm"] || len(i.temporaryKeywords) != 0 {
		t.Fatal("temporary grant survived both deadlines")
	}
	for _, permanentFirst := range []bool{true, false} {
		i.removeKeyword("storm")
		if permanentFirst {
			i.addKeyword("storm", "")
		}
		i.addKeyword("storm", "own")
		if !permanentFirst {
			i.addKeyword("storm", "")
		}
		s.g.expireKeywords("own")
		if !i.abilities["storm"] {
			t.Fatal("expiration removed a permanent grant")
		}
	}
	i.addKeyword("barrier", "own")
	s.g.execTargetEffect(ir.TargetEffect{Kind: "damage", Target: ir.SelfRef{Kind: "self"}, Amount: 1}, i, frame{})
	if i.abilities["barrier"] || i.temporaryKeywords["barrier"].OwnTurnEnd {
		t.Fatal("consumed barrier kept an expiration record")
	}
	i.addKeyword("barrier", "")
	s.g.expireKeywords("own")
	if !i.abilities["barrier"] {
		t.Fatal("old deadline removed a newly granted barrier")
	}
	i.addKeyword("cannot_attack", "oppo")
	s.g.returnCard(i, "hand")
	if i.abilities["cannot_attack"] || len(i.temporaryKeywords) != 0 {
		t.Fatal("return did not reset temporary state")
	}
	i.addKeyword("cannot_attack", "oppo")
	s.g.returnCard(i, "deck")
	if !i.abilities["cannot_attack"] {
		t.Fatal("hand to deck lost attached effects")
	}
	s.g.expireKeywords("oppo")
	if i.abilities["cannot_attack"] {
		t.Fatal("effect did not expire in deck")
	}
}

func TestSnowmanAttackLockBothPlayers(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active, state.Turn.Number = side, 8
		source, target := strings.Repeat("1", 32), strings.Repeat("2", 32)
		p := state.Players[side]
		p.PP, p.MaxPP = 8, 8
		state.Players[side] = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10132320, DeclaredType: "spell"})
		opponent := oppositeSide(side)
		state.Players[opponent] = withInstance(state.Players[opponent], "field", ir.TestInstance{InstanceID: target, CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Keywords: []string{"barrier"}}})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: source})
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
		for _, current := range []*Session{s, restored} {
			if r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{target}}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			i := current.g.instances[target]
			if i.life != 1 || !i.abilities["barrier"] || !i.abilities["cannot_attack"] {
				t.Fatal("incorrect Snowman effect")
			}
			if r := current.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "end_turn"}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if !i.abilities["cannot_attack"] {
				t.Fatal("lock expired at caster turn end")
			}
			for _, action := range current.LegalActionsFor(opponent) {
				if action.Source == target && (action.Kind == "attack_leader" || action.Kind == "attack_entity") {
					t.Fatal("locked follower can attack")
				}
			}
			if r := current.SubmitAs(strings.Repeat("c", 32), opponent, SimulatorCommand{Kind: "attack", Source: target}); r.Status != StatusIllegal || r.IllegalCode != "attack_restricted" {
				t.Fatal(r)
			}
			if r := current.SubmitAs(strings.Repeat("d", 32), opponent, SimulatorCommand{Kind: "end_turn"}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if i.abilities["cannot_attack"] || i.life != 1 {
				t.Fatal("expiration changed life or kept lock")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored spell diverged")
		}
	}
}

func TestTemporaryKeywordsWaitForEndTriggerAndRestore(t *testing.T) {
	pack := repeatCardPack(t)
	pack.Cards = append(pack.Cards, ir.Card{ID: 77775001, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 2}, Abilities: []ir.Ability{{
		ID: strings.Repeat("7", 32), Trigger: ir.EventTrigger{Kind: "event", Event: "turn_ended", Side: "own"}, Body: []ir.Effect{
			ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("8", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1}, {ID: 2}}},
		},
	}}})
	for _, advance := range []bool{false, true} {
		state := testState()
		id := strings.Repeat("1", 32)
		state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: id, CardID: 77775001, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		s.g.instances[id].addKeyword("cannot_attack", "own")
		cloned := s.g.clone()
		cloned.instances[id].removeKeyword("cannot_attack")
		if len(s.g.instances[id].temporaryKeywords) != 1 {
			t.Fatal("clone aliases temporary state")
		}
		var step StepResult
		if advance {
			step = s.Advance(ir.AdvanceAction{Timing: "turn_end", Side: "own"})
		} else {
			step = s.Submit(strings.Repeat("a", 32), SimulatorCommand{Kind: "end_turn"})
		}
		if step.Status != StatusSuspended || !s.g.instances[id].abilities["cannot_attack"] {
			t.Fatal("expired before end trigger completed", step)
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
		for _, current := range []*Session{s, restored} {
			if r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if current.g.instances[id].abilities["cannot_attack"] || current.g.endingSide != "" {
				t.Fatal("expiration did not finish")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("end trigger restore diverged")
		}
		for _, bad := range []map[string]KeywordExpiry{{"cannot_attack": {}}, {"unknown": {OwnTurnEnd: true}}, {"barrier": {OwnTurnEnd: true}}} {
			c.Game.Instances[0].TemporaryKeywords = maps.Clone(bad)
			if _, err := RestoreSession(pack, c); err == nil {
				t.Fatal("accepted malformed temporary keyword")
			}
		}
	}
}
