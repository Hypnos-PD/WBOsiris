package runner

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/project"
)

func TestCompiledCardTypeSemantics(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	// Pin the non-follower identities independently of the importer.
	expected := map[string][]int{
		"spell": {10101310, 10103310, 10111310, 10121310, 10122310, 10123310,
			10131310, 10131320, 10132310, 10132320, 10133310, 10133320, 10134310,
			10141310, 10142310, 10151310, 10153310, 10161310, 10171310, 10171320, 10172310, 10172320},
		"amulet": {10112210, 10113210, 10143210, 10152210, 10161210,
			10162210, 10162220, 10163210, 10163220, 10173210},
	}
	byID := map[int]ir.Card{}
	for _, card := range pack.Cards {
		byID[card.ID] = card
	}
	for kind, ids := range expected {
		for _, id := range ids {
			if card := byID[id]; card.CardType != kind {
				t.Errorf("card %d type=%s, want %s", id, card.CardType, kind)
			}
		}
	}
	sourceID, heldID, allyID, enemyID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32)
	newState := func(side string, cardID int) ir.State {
		state := testState()
		state.Turn.Active = side
		actor := state.Players[side]
		actor.PP, actor.MaxPP = 10, 10
		actor.Leader.Life = 10
		state.Players[side] = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: cardID, DeclaredType: byID[cardID].CardType})
		return state
	}
	newSession := func(t *testing.T, state ir.State) *Session {
		t.Helper()
		session, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		return session
	}
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			t.Run("spell_on_full_field", func(t *testing.T) {
				state := newState(side, 10111310)
				actor := withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: heldID, CardID: 10133320, DeclaredType: "spell"})
				for i := 0; i < 5; i++ {
					actor = withInstance(actor, "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", 100+i), CardID: 90001110, DeclaredType: "follower"})
				}
				state.Players[side] = actor
				session := newSession(t, state)
				if result := session.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); result.Status != StatusCompleted {
					t.Fatalf("full-field spell: %#v", result)
				}
				view, _ := session.View(side)
				if len(view.Own.Field) != 5 || len(view.Own.Hand) != 3 || view.Own.PP != 9 || session.g.instances[sourceID].zone != "graveyard" || session.g.instances[heldID].cost != 6 {
					t.Fatalf("spell did not resolve, leave play, or spellboost: %#v", view.Own)
				}
			})
			t.Run("summon_five", func(t *testing.T) {
				session := newSession(t, newState(side, 10101310))
				if result := session.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				view, _ := session.View(side)
				if len(view.Own.Field) != 5 || session.g.instances[sourceID].zone != "graveyard" {
					t.Fatalf("spell occupied a summon slot: %#v", view.Own.Field)
				}
			})
			t.Run("simultaneous_damage", func(t *testing.T) {
				state := newState(side, 10141310)
				state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: allyID, CardID: 10151130, DeclaredType: "follower"})
				opponent := oppositeSide(side)
				state.Players[opponent] = withInstance(state.Players[opponent], "field", ir.TestInstance{InstanceID: enemyID, CardID: 10151130, DeclaredType: "follower"})
				session := newSession(t, state)
				if result := session.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				batches := map[string]uint64{}
				for _, event := range session.g.events {
					if event.Kind == "destroyed" && event.Subject != nil {
						batches[event.Subject.InstanceID] = event.BatchID
					}
				}
				view, _ := session.View(side)
				if batches[allyID] == 0 || batches[allyID] != batches[enemyID] || len(view.Own.Field) != 2 || len(view.Oppo.Field) != 2 {
					t.Fatalf("damage split death batches or hit last-words summons: batches=%v view=%#v", batches, view)
				}
			})
			for _, cardID := range []int{10121310, 10172310} {
				t.Run(fmt.Sprintf("required_target_%d", cardID), func(t *testing.T) {
					session := newSession(t, newState(side, cardID))
					result := session.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID})
					if result.Status != StatusIllegal || result.IllegalCode != "target_required" || session.g.instances[sourceID].zone != "hand" {
						t.Fatalf("untargeted spell was committed: %#v", result)
					}
				})
			}
			t.Run("amulet_play_then_engage", func(t *testing.T) {
				state := newState(side, 10162210)
				state.Players[side] = withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: heldID, CardID: 10133320, DeclaredType: "spell"})
				session := newSession(t, state)
				if result := session.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				if session.g.instances[sourceID].zone != "field" || session.g.instances[heldID].cost != 7 {
					t.Fatal("amulet did not remain on field, or incorrectly spellboosted")
				}
				if result := session.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "engage", Source: sourceID}); result.Status != StatusCompleted {
					t.Fatalf("empty-target engage: %#v", result)
				}
				view, _ := session.View(side)
				if view.Own.PP != 7 || view.Own.LeaderLife != 11 {
					t.Fatalf("engage cost or heal incorrect: %#v", view.Own)
				}
				if result := session.SubmitAs(strings.Repeat("c", 32), side, SimulatorCommand{Kind: "engage", Source: sourceID}); result.Status != StatusIllegal {
					t.Fatalf("amulet engaged twice: %#v", result)
				}
			})
			t.Run("self_destroy_without_enemy", func(t *testing.T) {
				session := newSession(t, newState(side, 10162220))
				if result := session.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				if result := session.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "engage", Source: sourceID}); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				view, _ := session.View(side)
				if session.g.instances[sourceID].zone != "graveyard" || view.Own.LeaderLife != 11 || view.Own.PP != 7 {
					t.Fatalf("self destruction skipped remaining heal: %#v", view.Own)
				}
			})
			for _, cardID := range []int{10162210, 10162220} {
				t.Run(fmt.Sprintf("engage_target_%d", cardID), func(t *testing.T) {
					state := newState(side, cardID)
					targetSide := side
					if cardID == 10162220 {
						targetSide = oppositeSide(side)
					}
					state.Players[targetSide] = withInstance(state.Players[targetSide], "field", ir.TestInstance{InstanceID: allyID, CardID: 90001110, DeclaredType: "follower"})
					session := newSession(t, state)
					if result := session.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); result.Status != StatusCompleted {
						t.Fatal(result)
					}
					result := session.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "engage", Source: sourceID})
					if result.Status != StatusSuspended || result.Choice == nil || len(result.Choice.Candidates) != 1 || result.Choice.Candidates[0].InstanceID != allyID {
						t.Fatalf("incorrect engage candidates: %#v", result)
					}
					choice := result.Choice
					result = session.Resume(ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{allyID}})
					if result.Status != StatusCompleted {
						t.Fatal(result)
					}
					target := session.g.instances[allyID]
					if cardID == 10162210 && (target.attack != 2 || target.life != 3) || cardID == 10162220 && target.zone != "graveyard" {
						t.Fatalf("engage did not affect selected follower: %#v", target)
					}
				})
			}
			t.Run("countdown_engage_lastwords", func(t *testing.T) {
				state := newState(side, 10161210)
				actor := state.Players[side]
				actor.Zones["hand"] = nil
				actor = withInstance(actor, "field", ir.TestInstance{InstanceID: sourceID, CardID: 10161210, DeclaredType: "amulet", Overrides: ir.InstanceOverrides{Countdown: intPtr(1)}})
				actor = withInstance(actor, "deck", ir.TestInstance{InstanceID: allyID, CardID: 90001110, DeclaredType: "follower"})
				state.Players[side] = withInstance(actor, "deck", ir.TestInstance{InstanceID: enemyID, CardID: 90001110, DeclaredType: "follower"})
				session := newSession(t, state)
				if result := session.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "engage", Source: sourceID}); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				view, _ := session.View(side)
				if session.g.instances[sourceID].zone != "graveyard" || len(view.Own.Hand) != 2 || view.Own.PP != 9 {
					t.Fatalf("countdown did not destroy amulet and draw: %#v", view.Own)
				}
			})
			t.Run("vessel_destroys_all_followers", func(t *testing.T) {
				state := newState(side, 10163220)
				state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: allyID, CardID: 90001110, DeclaredType: "follower"})
				opponent := oppositeSide(side)
				state.Players[opponent] = withInstance(state.Players[opponent], "field", ir.TestInstance{InstanceID: enemyID, CardID: 90001110, DeclaredType: "follower"})
				state.Players[opponent] = withInstance(state.Players[opponent], "field", ir.TestInstance{InstanceID: heldID, CardID: 10162210, DeclaredType: "amulet"})
				session := newSession(t, state)
				if result := session.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				if result := session.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "engage", Source: sourceID}); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				for _, id := range []string{sourceID, allyID, enemyID} {
					if session.g.instances[id].zone != "graveyard" {
						t.Errorf("instance %s survived vessel", id)
					}
				}
				if session.g.instances[heldID].zone != "field" {
					t.Fatal("vessel destroyed another amulet")
				}
			})
		})
	}
}
