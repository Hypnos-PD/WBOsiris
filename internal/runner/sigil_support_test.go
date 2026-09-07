package runner

import (
	"fmt"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestCompiledSigilSupportCards(t *testing.T) {
	pack := repeatCardPack(t)
	sourceID := strings.Repeat("1", 32)
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			newSession := func(t *testing.T, cardID int, zone, kind string) *Session {
				t.Helper()
				state := testState()
				state.Turn.Active, state.Turn.Number = side, 9
				actor := state.Players[side]
				actor.PP, actor.MaxPP, actor.EP, actor.SEP = 10, 10, 1, 1
				state.Players[side] = withInstance(actor, zone, ir.TestInstance{InstanceID: sourceID, CardID: cardID, DeclaredType: kind})
				for seat, owner := range []string{"own", "oppo"} {
					for n := 0; n < 6; n++ {
						state.Players[owner] = withInstance(state.Players[owner], "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", 100+n+100*seat), CardID: 90021110, DeclaredType: "follower"})
					}
				}
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				return s
			}
			command := func(t *testing.T, s *Session, id, kind string) StepResult {
				t.Helper()
				return s.SubmitAs(strings.Repeat(id, 32), side, SimulatorCommand{Kind: kind, Source: sourceID})
			}
			for _, cardID := range []int{10131130, 10141130} {
				for _, hidden := range []bool{false, true} {
					t.Run(fmt.Sprintf("target%d/hidden%t", cardID, hidden), func(t *testing.T) {
						s := newSession(t, cardID, "hand", "follower")
						source := s.g.instances[sourceID]
						enemy := s.g.summonFor(source, "oppo", 1, 90021110, false)[0]
						enemy.life = 8
						enemy.abilities["aura"] = hidden
						result := command(t, s, "a", "play")
						if cardID == 10131130 {
							if result.Status != StatusCompleted {
								t.Fatal(result)
							}
							result = command(t, s, "b", "evolve")
						}
						if hidden {
							if result.Status != StatusCompleted || enemy.zone != "field" || enemy.life != 8 {
								t.Fatal("no legal target blocked action or affected protected enemy", result)
							}
							return
						}
						if result.Status != StatusSuspended || len(result.Choice.Candidates) != 1 {
							t.Fatal(result)
						}
						if step := s.Resume(ChoiceResponse{RequestID: result.Choice.RequestID, ActionID: result.Choice.ActionID, StateRevision: result.Choice.StateRevision, SelectedInstanceIDs: []string{enemy.id}}); step.Status != StatusCompleted {
							t.Fatal(step)
						}
						if cardID == 10131130 && (enemy.life != 3 || enemy.zone != "field") || cardID == 10141130 && enemy.zone != "graveyard" {
							t.Fatal("targeted ability did not resolve")
						}
					})
				}
			}
			t.Run("twilight_super_evolve_draws_three", func(t *testing.T) {
				s := newSession(t, 10143140, "field", "follower")
				if result := command(t, s, "a", "superevolve"); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				if p := s.g.player(side); len(p.hand) != 3 || p.sep != 0 || s.g.instances[sourceID].attack != 12 {
					t.Fatal("super evolution missed draw or stats")
				}
			})
			t.Run("life_reduction_ignores_damage_protections", func(t *testing.T) {
				s := newSession(t, 10143140, "hand", "follower")
				source := s.g.instances[sourceID]
				target := s.g.summonFor(source, "own", 1, 90021110, false)[0]
				target.life, target.superEvolved, target.damageReduction = 8, true, 10
				target.abilities["barrier"] = true
				s.g.execTargetEffect(ir.TargetEffect{Kind: "buff_stats", Target: ir.SelfRef{Kind: "self"}, LifeDelta: -9}, target, nil)
				if target.zone != "graveyard" {
					t.Fatal("damage or destruction protection blocked life reduction")
				}
			})
			t.Run("pact_engage_once_then_natural_expiry", func(t *testing.T) {
				s := newSession(t, 10163210, "hand", "amulet")
				if result := command(t, s, "a", "play"); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				if result := command(t, s, "b", "engage"); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				if result := command(t, s, "c", "engage"); result.Status != StatusIllegal {
					t.Fatal("second engage allowed", result)
				}
				if s.g.instances[sourceID].countdown != 1 || s.g.player(side).pp != 7 {
					t.Fatal("engage did not pay exactly once")
				}
				if result := s.SubmitAs(strings.Repeat("d", 32), side, SimulatorCommand{Kind: "end_turn"}); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				if result := s.SubmitAs(strings.Repeat("e", 32), oppositeSide(side), SimulatorCommand{Kind: "end_turn"}); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				field := s.g.player(side).field
				if len(field) != 1 || field[0].card.ID != 90061120 || field[0].attack != 4 || field[0].life != 4 || !field[0].abilities["rush"] || s.g.instances[sourceID].zone != "graveyard" {
					t.Fatal("natural countdown missed fresh token")
				}
			})
			t.Run("shadowcrypt_fanfare_and_ghost_expiry", func(t *testing.T) {
				s := newSession(t, 10152210, "hand", "amulet")
				if result := command(t, s, "a", "play"); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				if s.g.player(side).shadows != 2 {
					t.Fatal("fanfare missed shadows")
				}
				if result := command(t, s, "b", "engage"); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				p := s.g.player(side)
				if len(p.field) != 2 || p.shadows != 3 || p.pp != 7 {
					t.Fatal("engage missed destruction or summons")
				}
				if result := s.SubmitAs(strings.Repeat("c", 32), side, SimulatorCommand{Kind: "end_turn"}); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				if len(p.field) != 0 || len(p.banished) != 2 || p.shadows != 3 {
					t.Fatal("ghost expiry added shadows or failed to banish")
				}
			})
		})
	}
}
