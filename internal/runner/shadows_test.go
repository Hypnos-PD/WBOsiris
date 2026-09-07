package runner

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/project"
)

func TestSpellGraveyardTimingAndContinuation(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		for _, initialShadows := range []int{3, 4} {
			t.Run(fmt.Sprintf("%s/shadows_%d", side, initialShadows), func(t *testing.T) {
				sourceID := strings.Repeat("1", 32)
				pay := func(id string, damage int) ir.Effect {
					return ir.PayResourceEffect{NodeBase: ir.NodeBase{ID: strings.Repeat(id, 32)}, Kind: "pay_resource", Resource: "shadows", Amount: 4, OnPaid: []ir.Effect{
						ir.TargetEffect{Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, Amount: damage},
					}}
				}
				pack := &ir.CardPack{Cards: []ir.Card{{ID: 66666671, CardType: "spell", Cost: 2, PlayEffects: []ir.Effect{
					pay("2", 2),
					ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("3", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{pay("4", 3)}}}},
				}}}}
				state := testState()
				state.Turn.Active = side
				actor := state.Players[side]
				actor.Shadows = initialShadows
				state.Players[side] = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 66666671, DeclaredType: "spell"})
				session, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				session.g.instances[sourceID].cost = 0
				before := session.g.snapshot()
				session.LegalActionsFor(side)
				if !reflect.DeepEqual(before, session.g.snapshot()) {
					t.Fatal("preflight changed graveyard state")
				}
				step := session.SubmitAs(strings.Repeat("5", 32), side, SimulatorCommand{Kind: "play", Source: sourceID})
				view, err := session.View(side)
				if err != nil {
					t.Fatal(err)
				}
				wantShadows, wantLife := initialShadows, 20
				if initialShadows == 4 {
					wantShadows, wantLife = 0, 18
				}
				if step.Status != StatusSuspended || view.Own.Shadows != wantShadows || view.Oppo.LeaderLife != wantLife || len(view.Own.Graveyard) != 0 || len(view.Own.Resolving) != 1 || view.Own.Resolving[0].InstanceID != sourceID {
					t.Fatalf("spell counted its graveyard before finishing: step=%#v view=%#v", step, view)
				}
				encoded, err := session.EncodeContinuation()
				if err != nil {
					t.Fatal(err)
				}
				saved, err := DecodeContinuation(encoded)
				if err != nil {
					t.Fatal(err)
				}
				restored, err := RestoreSession(pack, saved)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(session.g.snapshot(), restored.g.snapshot()) {
					t.Fatal("restoring a resolving spell changed its cost, owner, or graveyard")
				}
				choice := step.Choice
				response := ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedOptionID: 1}
				for _, current := range []*Session{session, restored} {
					if result := current.Resume(response); result.Status != StatusCompleted {
						t.Fatalf("spell failed to resume: %#v", result)
					}
					view, err := current.View(side)
					if err != nil {
						t.Fatal(err)
					}
					if view.Own.Shadows != wantShadows+1 || view.Oppo.LeaderLife != wantLife || view.Own.Combo != 1 || len(view.Own.Resolving) != 0 || len(view.Own.Graveyard) != 1 || view.Own.Graveyard[0].Cost != 0 {
						t.Fatalf("spell did not finish exactly once: %#v", view)
					}
					before := current.g.snapshot()
					if result := current.Resume(response); result.Status != StatusRejected || !reflect.DeepEqual(before, current.g.snapshot()) {
						t.Fatal("duplicate response changed graveyard state")
					}
				}
				if !reflect.DeepEqual(session.g.snapshot(), restored.g.snapshot()) {
					t.Fatal("restored and uninterrupted spell results diverged")
				}
			})
		}
	}
}

func TestGraveyardMovementAccounting(t *testing.T) {
	target := ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}
	for _, side := range []string{"own", "oppo"} {
		for _, tc := range []struct {
			name       string
			effect     ir.Effect
			hand       int
			gain       int
			victimZone string
			destroyed  int
		}{
			{"destroy", ir.TargetEffect{Kind: "destroy", Target: target}, 0, 1, "graveyard", 1},
			{"banish", ir.TargetEffect{Kind: "banish", Target: target}, 0, 0, "banished", 0},
			{"return_hand", ir.TargetEffect{Kind: "return", Target: target, Destination: "hand"}, 8, 0, "hand", 0},
			{"return_full_hand", ir.TargetEffect{Kind: "return", Target: target, Destination: "hand"}, 9, 1, "graveyard", 0},
			{"return_deck", ir.TargetEffect{Kind: "return", Target: target, Destination: "deck"}, 0, 0, "deck", 0},
			{"overdraw", ir.DrawEffect{Kind: "draw", Owner: "own", Count: 2}, 8, 1, "field", 0},
			{"add_full_hand", ir.CardEffect{Kind: "add_card", Owner: "own", CardID: 66666672, Count: 2}, 8, 1, "field", 0},
		} {
			t.Run(side+"/"+tc.name, func(t *testing.T) {
				sourceID, victimID := strings.Repeat("1", 32), strings.Repeat("2", 32)
				pack := &ir.CardPack{Cards: []ir.Card{
					{ID: 66666671, CardType: "amulet", Abilities: []ir.Ability{{ID: strings.Repeat("3", 32), Trigger: ir.CostTrigger{Kind: "engage"}, Body: []ir.Effect{tc.effect}}}},
					{ID: 66666672, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
				}}
				state := testState()
				state.Turn.Active = side
				actor := state.Players[side]
				actor.Shadows = 5
				actor = withInstance(actor, "field", ir.TestInstance{InstanceID: sourceID, CardID: 66666671, DeclaredType: "amulet"})
				actor = withInstance(actor, "field", ir.TestInstance{InstanceID: victimID, CardID: 66666672, DeclaredType: "follower"})
				actor = withInstance(actor, "graveyard", ir.TestInstance{InstanceID: strings.Repeat("4", 32), CardID: 66666672, DeclaredType: "follower"})
				for n := 0; n < tc.hand; n++ {
					actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+10), CardID: 66666672, DeclaredType: "follower"})
				}
				for n := 0; n < 2; n++ {
					actor = withInstance(actor, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+30), CardID: 66666672, DeclaredType: "follower"})
				}
				state.Players[side] = actor
				session, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				if session.g.player(side).shadows != 5 {
					t.Fatal("loading graveyard history recalculated spent shadows")
				}
				step := session.SubmitAs(strings.Repeat("5", 32), side, SimulatorCommand{Kind: "engage", Source: sourceID})
				view, err := session.View(side)
				if err != nil {
					t.Fatal(err)
				}
				if step.Status != StatusCompleted || view.Own.Shadows != 5+tc.gain || view.Oppo.Shadows != 0 || len(view.Own.Graveyard) != 1+tc.gain || view.Own.HandCount > handLimit || len(view.Own.Destroyed) != tc.destroyed || session.g.instances[victimID].zone != tc.victimZone {
					t.Fatalf("incorrect graveyard accounting: step=%#v view=%#v", step, view)
				}
			})
		}
	}
}

func TestDeathBatchFundsLastwordsNecromancy(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			sourceID := strings.Repeat("1", 32)
			pack := &ir.CardPack{Cards: []ir.Card{
				{ID: 66666671, CardType: "amulet", Abilities: []ir.Ability{{ID: strings.Repeat("2", 32), Trigger: ir.CostTrigger{Kind: "engage"}, Body: []ir.Effect{
					ir.TargetEffect{Kind: "destroy", Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}},
				}}}},
				{ID: 66666672, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{{ID: strings.Repeat("3", 32), Trigger: ir.SimpleTrigger{Kind: "lastwords"}, Body: []ir.Effect{
					ir.PayResourceEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("4", 32)}, Kind: "pay_resource", Resource: "shadows", Amount: 2, OnPaid: []ir.Effect{
						ir.TargetEffect{Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, Amount: 3},
					}},
				}}}},
			}}
			state := testState()
			state.Turn.Active = side
			actor := withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: sourceID, CardID: 66666671, DeclaredType: "amulet"})
			for n := 0; n < 2; n++ {
				actor = withInstance(actor, "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+10), CardID: 66666672, DeclaredType: "follower"})
			}
			state.Players[side] = actor
			session, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			step := session.SubmitAs(strings.Repeat("5", 32), side, SimulatorCommand{Kind: "engage", Source: sourceID})
			view, err := session.View(side)
			if err != nil {
				t.Fatal(err)
			}
			if step.Status != StatusCompleted || view.Own.Shadows != 0 || len(view.Own.Graveyard) != 2 || len(view.Own.Destroyed) != 2 || view.Oppo.LeaderLife != 17 {
				t.Fatalf("batch graves were not available to the first lastwords: step=%#v view=%#v", step, view)
			}
		})
	}
}

func TestCompiledSpellFundsFollowingNecromancy(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{
		filepath.Join(root, "cards", "10000", "10031310.wbo"),
		filepath.Join(root, "cards", "10000", "10051130.wbo"),
	}, false, root)
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	spellID, followerID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	state := testState()
	actor := state.Players["own"]
	actor.Shadows, actor.PP, actor.MaxPP = 3, 10, 10
	actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: spellID, CardID: 10031310, DeclaredType: "spell"})
	state.Players["own"] = withInstance(actor, "hand", ir.TestInstance{InstanceID: followerID, CardID: 10051130, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	for n, id := range []string{spellID, followerID} {
		if step := session.Submit(fmt.Sprintf("%032x", n+10), SimulatorCommand{Kind: "play", Source: id}); step.Status != StatusCompleted {
			t.Fatalf("card play failed: %#v", step)
		}
	}
	if session.g.own.shadows != 0 || !session.g.instances[followerID].abilities["storm"] || len(session.g.own.graveyard) != 1 {
		t.Fatal("completed spell did not fund the next card's necromancy")
	}
}

func TestSpellGraveyardPrecedesQueuedSpellboost(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			sourceID, heldID := strings.Repeat("1", 32), strings.Repeat("2", 32)
			pack := &ir.CardPack{Cards: []ir.Card{
				{ID: 66666671, CardType: "spell"},
				{ID: 66666672, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{{ID: strings.Repeat("3", 32), Trigger: ir.SimpleTrigger{Kind: "spellboost"}, Body: []ir.Effect{
					ir.PayResourceEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("4", 32)}, Kind: "pay_resource", Resource: "shadows", Amount: 1, OnPaid: []ir.Effect{
						ir.TargetEffect{Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, Amount: 1},
					}},
				}}}},
			}}
			state := testState()
			state.Turn.Active = side
			actor := withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: sourceID, CardID: 66666671, DeclaredType: "spell"})
			state.Players[side] = withInstance(actor, "hand", ir.TestInstance{InstanceID: heldID, CardID: 66666672, DeclaredType: "follower"})
			session, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			step := session.SubmitAs(strings.Repeat("5", 32), side, SimulatorCommand{Kind: "play", Source: sourceID})
			view, err := session.View(side)
			if err != nil {
				t.Fatal(err)
			}
			if step.Status != StatusCompleted || view.Own.Shadows != 0 || len(view.Own.Graveyard) != 1 || len(view.Own.Resolving) != 0 || view.Oppo.LeaderLife != 19 {
				t.Fatalf("spellboost ran before the completed spell entered the graveyard: step=%#v view=%#v", step, view)
			}
		})
	}
}

func TestLethalSpellLeavesNoResolvingCard(t *testing.T) {
	sourceID := strings.Repeat("1", 32)
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 66666671, CardType: "spell", PlayEffects: []ir.Effect{
		ir.TargetEffect{Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, Amount: 20},
	}}}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, CardID: 66666671, DeclaredType: "spell"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := session.Submit(strings.Repeat("2", 32), SimulatorCommand{Kind: "play", Source: sourceID})
	if step.Status != StatusCompleted || !session.g.gameOver || session.g.winner != "own" || session.g.own.shadows != 1 || len(session.g.own.resolving) != 0 || len(session.g.own.graveyard) != 1 {
		t.Fatalf("lethal spell left an unfinished cast: %#v", session.g.snapshot())
	}
}

func TestRestoreRejectsInvalidSpellCheckpoint(t *testing.T) {
	pack, state, sourceID, targetID := continuationTargetFixture()
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if step := session.Submit(strings.Repeat("f", 32), SimulatorCommand{Kind: "play", Source: sourceID}); step.Status != StatusSuspended {
		t.Fatalf("spell did not suspend: %#v", step)
	}
	encoded, err := session.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Continuation)
	}{
		{"old_version", func(c *Continuation) { c.Version = "0.6.0" }},
		{"early_triggers", func(c *Continuation) { c.DrainingTrigger = false }},
		{"missing_spell", func(c *Continuation) { c.Game.Own.Resolving = nil }},
		{"duplicate_spell", func(c *Continuation) { c.Game.Own.Resolving = append(c.Game.Own.Resolving, sourceID) }},
		{"wrong_owner", func(c *Continuation) { c.Game.Oppo.Resolving, c.Game.Own.Resolving = c.Game.Own.Resolving, nil }},
		{"wrong_frame_source", func(c *Continuation) { c.Stack[0].SelfInstanceID = targetID }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := DecodeContinuation(encoded)
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(c)
			if _, err := RestoreSession(pack, c); err == nil {
				t.Fatal("invalid spell checkpoint was accepted")
			}
		})
	}
}
