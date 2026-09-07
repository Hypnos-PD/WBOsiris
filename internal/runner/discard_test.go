package runner

import (
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestFanDiscardContinuationAndKitBuffBothPlayers(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active = side
		sourceID, kitID, otherID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
		actor := state.Players[side]
		actor.PP, actor.MaxPP = 3, 3
		actor = withInstance(actor, "field", ir.TestInstance{InstanceID: sourceID, CardID: 10143210, DeclaredType: "amulet"})
		actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: kitID, CardID: 10142110, DeclaredType: "follower"})
		state.Players[side] = withInstance(actor, "hand", ir.TestInstance{InstanceID: otherID, CardID: 10001120, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		before := s.g.snapshot()
		s.LegalActionsFor(side)
		if !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("preflight discarded cards")
		}
		step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "engage", Source: sourceID})
		if step.Status != StatusSuspended || len(s.g.player(side).field) != 2 || s.g.instances[kitID].zone != "hand" {
			t.Fatal("discard did not wait for choice", step)
		}
		foe, _ := s.View(oppositeSide(side))
		if foe.PendingChoice != nil || len(foe.Oppo.Hand) != 0 {
			t.Fatal("private discard choice leaked")
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
		response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{kitID}}
		for _, current := range []*Session{s, restored} {
			if result := current.Resume(response); result.Status != StatusCompleted {
				t.Fatal(result)
			}
			owner := current.g.player(side)
			guard := owner.field[1]
			if owner.pp != 0 || owner.shadows != 1 || len(owner.destroyed) != 0 || guard.card.ID != 90043110 || guard.attack != 3 || guard.life != 2 || !guard.abilities["storm"] || !guard.abilities["ward"] || len(owner.hand) != 1 {
				t.Fatal("wrong discard result")
			}
			public, _ := current.View(oppositeSide(side))
			if public.Oppo.Graveyard[0].CardID != 10142110 {
				t.Fatal("discarded card is not public")
			}
			count := 0
			for _, e := range current.EventsFor(oppositeSide(side)) {
				if e.Kind == "card_discarded" {
					count++
					if e.Subject == nil || e.Subject.CardID != 10142110 {
						t.Fatal("discard identity lost")
					}
				}
			}
			if count != 1 {
				t.Fatal("duplicate discard event")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored discard diverged")
		}
	}
}

func TestDiscardOnlyHandAndNeverLastwords(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	handID, fieldID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	actor := withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: handID, CardID: 10001120, DeclaredType: "follower"})
	state.Players["own"] = withInstance(actor, "field", ir.TestInstance{InstanceID: fieldID, CardID: 10001120, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	s.g.discardCards([]*instance{s.g.instances[handID], s.g.instances[handID], nil, s.g.instances[fieldID]})
	if len(s.g.events) != 1 || s.g.own.shadows != 1 || len(s.g.triggers) != 0 || len(s.g.own.destroyed) != 0 || s.g.instances[fieldID].zone != "field" {
		t.Fatal("discard duplicated or triggered destruction")
	}
}

func TestDiscardObserversAndSelfTriggerAreDistinct(t *testing.T) {
	pack := repeatCardPack(t)
	watcher := ir.Card{ID: 77772001, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{
		{ID: strings.Repeat("a", 32), Trigger: ir.EventTrigger{Kind: "event", Event: "card_discarded", Side: "own"}, Body: []ir.Effect{ir.TargetEffect{Kind: "buff_stats", Target: ir.SelfRef{Kind: "self"}, AttackDelta: 2}}},
	}}
	pack.Cards = append(pack.Cards, watcher)
	state := testState()
	id, watcherID, enemyID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	actor := withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: id, CardID: 10142110, DeclaredType: "follower"})
	state.Players["own"] = withInstance(actor, "field", ir.TestInstance{InstanceID: watcherID, CardID: watcher.ID, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: enemyID, CardID: watcher.ID, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	s.g.discardCards([]*instance{s.g.instances[id]})
	if len(s.g.triggers) != 2 {
		t.Fatal("wrong observer or self trigger count")
	}
	if result := s.run(); result.Status != StatusCompleted {
		t.Fatal(result)
	}
	if s.g.instances[watcherID].attack != 4 || s.g.instances[enemyID].attack != 1 {
		t.Fatal("discard observer side or own discarded trigger failed")
	}
}

func TestEventSubjectMatcherAllowsAnIdentitySnapshot(t *testing.T) {
	want := &ir.EventTarget{Kind: "instance", InstanceID: strings.Repeat("1", 32)}
	got := *want
	got.CardID = 10142110
	if !targetEqual(want, &got) {
		t.Fatal("identity snapshot prevented instance match")
	}
	want.CardID = 10001120
	if targetEqual(want, &got) {
		t.Fatal("explicit card identity ignored")
	}
}

func TestDiscardedSelfTriggerSurvivesContinuation(t *testing.T) {
	pack := repeatCardPack(t)
	spell := ir.Card{ID: 77772002, CardType: "spell", PlayEffects: []ir.Effect{
		ir.TargetEffect{Kind: "discard", Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand"}},
		ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1}, {ID: 2}}},
	}}
	pack.Cards = append(pack.Cards, spell)
	state := testState()
	id, kitID, allyID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	actor := withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: id, CardID: spell.ID, DeclaredType: "spell"})
	actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: kitID, CardID: 10142110, DeclaredType: "follower"})
	state.Players["own"] = withInstance(actor, "field", ir.TestInstance{InstanceID: allyID, CardID: 10001110, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := s.Submit(strings.Repeat("a", 32), SimulatorCommand{Kind: "play", Source: id})
	if step.Status != StatusSuspended || s.g.instances[kitID].zone != "graveyard" || s.g.instances[allyID].attack != 2 || len(s.g.triggers) != 1 {
		t.Fatal("discard trigger did not wait for enclosing spell")
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
	for _, mutation := range []string{"hidden", "from", "subject"} {
		bad, _ := DecodeContinuation(data)
		for n := range bad.Game.Events {
			e := &bad.Game.Events[n]
			if e.Kind != "card_discarded" {
				continue
			}
			switch mutation {
			case "hidden":
				e.PrivateTo = "own"
			case "from":
				e.From = "field"
			case "subject":
				e.Subject = nil
			}
		}
		if _, err := RestoreSession(pack, bad); err == nil {
			t.Fatal("restored corrupt discard event", mutation)
		}
	}
	response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}
	for _, current := range []*Session{s, restored} {
		if result := current.Resume(response); result.Status != StatusCompleted || current.g.instances[allyID].attack != 3 || current.g.own.shadows != 2 {
			t.Fatal("discarded self trigger failed after restore", result)
		}
	}
	if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
		t.Fatal("discard trigger restore changed result")
	}
}
