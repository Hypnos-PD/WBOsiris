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

func TestCompiledFieldSupportCards(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	sourceID, allyID, enemyID, otherID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32)
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			newState := func(cardID int, zone, kind string) ir.State {
				state := testState()
				state.Turn.Active = side
				actor := state.Players[side]
				actor.PP, actor.MaxPP, actor.EP, actor.Leader.Life = 10, 10, 1, 10
				state.Players[side] = withInstance(actor, zone, ir.TestInstance{InstanceID: sourceID, CardID: cardID, DeclaredType: kind})
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
			command := func(t *testing.T, session *Session, id, kind, source string) {
				t.Helper()
				if result := session.SubmitAs(strings.Repeat(id, 32), side, SimulatorCommand{Kind: kind, Source: source}); result.Status != StatusCompleted {
					t.Fatal(result)
				}
			}
			t.Run("ernesta_fanfare_and_evolve_exclude_self", func(t *testing.T) {
				state := newState(10121120, "hand", "follower")
				state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: allyID, CardID: 90021110, DeclaredType: "follower"})
				state.Players[side] = withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: otherID, CardID: 90021110, DeclaredType: "follower"})
				opponent := oppositeSide(side)
				state.Players[opponent] = withInstance(state.Players[opponent], "field", ir.TestInstance{InstanceID: enemyID, CardID: 90021110, DeclaredType: "follower"})
				session := newSession(t, state)
				command(t, session, "a", "play", sourceID)
				if session.g.instances[allyID].attack != 2 || session.g.instances[sourceID].attack != 4 {
					t.Fatal("fanfare did not distinguish self and ally")
				}
				command(t, session, "b", "evolve", sourceID)
				if source := session.g.instances[sourceID]; source.attack != 6 || source.life != 8 {
					t.Fatal("evolve buff included its own source")
				}
				if ally := session.g.instances[allyID]; ally.attack != 3 || ally.life != 3 {
					t.Fatal("evolve did not repeat the allied buff")
				}
				if session.g.instances[otherID].attack != 1 || session.g.instances[enemyID].attack != 1 {
					t.Fatal("field buff escaped its side or zone")
				}
			})
			t.Run("lyrala_only_heals_for_allied_officers", func(t *testing.T) {
				state := newState(10121130, "hand", "follower")
				state.Players[side] = withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: allyID, CardID: 90021110, DeclaredType: "follower"})
				state.Players[side] = withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: otherID, CardID: 90001110, DeclaredType: "follower"})
				opponent := oppositeSide(side)
				state.Players[opponent] = withInstance(state.Players[opponent], "hand", ir.TestInstance{InstanceID: enemyID, CardID: 90021110, DeclaredType: "follower"})
				session := newSession(t, state)
				command(t, session, "a", "play", sourceID)
				if p := session.g.player(side); p.leaderLife != 11 || len(p.field) != 2 || p.field[1].card.ID != 90021120 {
					t.Fatal("fanfare summon missed officer-triggered heal")
				}
				command(t, session, "b", "play", otherID)
				if session.g.player(side).leaderLife != 11 {
					t.Fatal("non-officer triggered heal")
				}
				command(t, session, "c", "play", allyID)
				if session.g.player(side).leaderLife != 12 {
					t.Fatal("officer played from hand missed heal")
				}
				command(t, session, "d", "end_turn", "")
				if result := session.SubmitAs(strings.Repeat("e", 32), opponent, SimulatorCommand{Kind: "play", Source: enemyID}); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				if session.g.player(side).leaderLife != 12 || session.g.player(opponent).leaderLife != 20 {
					t.Fatal("enemy officer triggered heal")
				}
			})
			t.Run("shinobi_summon_is_fresh", func(t *testing.T) {
				state := newState(10122140, "field", "follower")
				session := newSession(t, state)
				source := session.g.instances[sourceID]
				source.attack, source.life = 5, 4
				source.abilities["ward"] = true
				command(t, session, "a", "evolve", sourceID)
				field := session.g.player(side).field
				if len(field) != 2 {
					t.Fatalf("field size=%d", len(field))
				}
				fresh := field[1]
				if fresh.attack != 2 || fresh.life != 1 || fresh.evolved || fresh.abilities["ward"] || !fresh.abilities["stealth"] || !fresh.summoningSick {
					t.Fatalf("summon inherited source modifications: %#v", fresh)
				}
			})
			t.Run("zirconia_buffs_new_summons_after_creation", func(t *testing.T) {
				state := newState(10123130, "field", "follower")
				session := newSession(t, state)
				command(t, session, "a", "evolve", sourceID)
				field := session.g.player(side).field
				if len(field) != 3 || field[0].attack != 6 || field[0].life != 6 {
					t.Fatal("evolution included source in buff or missed summons")
				}
				for _, knight := range field[1:] {
					if knight.card.ID != 90021110 || knight.attack != 2 || knight.life != 2 {
						t.Fatal("new knight missed post-summon buff")
					}
				}
			})
			for _, option := range []int{1, 2} {
				for _, fieldCount := range []int{0, 4, 5} {
					t.Run(fmt.Sprintf("majesty_mode%d_field%d", option, fieldCount), func(t *testing.T) {
						state := newState(10122310, "hand", "spell")
						for n := 0; n < fieldCount; n++ {
							state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+100), CardID: 90001110, DeclaredType: "follower"})
						}
						session := newSession(t, state)
						step := session.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID})
						if step.Status != StatusSuspended || len(step.Choice.Candidates) != 2 {
							t.Fatalf("mode choices unavailable: %#v", step)
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
						choice := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: option}
						for _, run := range []*Session{session, restored} {
							if result := run.Resume(choice); result.Status != StatusCompleted {
								t.Fatal(result)
							}
							actor := run.g.player(side)
							wantCount := fieldCount
							if option == 1 {
								wantCount = min(5, fieldCount+2)
							}
							if len(actor.field) != wantCount || actor.pp != 7 || actor.shadows != 1 || run.g.instances[sourceID].zone != "graveyard" {
								t.Fatal("mode changed field size, cost or spell completion")
							}
							for n, card := range actor.field {
								if n < fieldCount {
									wantAttack, wantLife := 1, 2
									if option == 2 {
										wantAttack, wantLife = 2, 3
									}
									if card.attack != wantAttack || card.life != wantLife {
										t.Fatal("mode buffed existing cards incorrectly")
									}
								} else if card.card.ID != []int{90021120, 90021110}[n-fieldCount] {
									t.Fatal("summon order changed after mode choice")
								}
							}
							if run.g.rng.Consumed() != 0 {
								t.Fatal("deterministic mode consumed randomness")
							}
						}
						if !reflect.DeepEqual(session.g.snapshot(), restored.g.snapshot()) {
							t.Fatal("restoration changed mode outcome")
						}
					})
				}
			}
		})
	}
}
