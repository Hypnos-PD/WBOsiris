package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestCompiledEarthSigilModes(t *testing.T) {
	pack := repeatCardPack(t)
	sourceID := strings.Repeat("1", 32)
	for _, side := range []string{"own", "oppo"} {
		for _, option := range []int{1, 2, 3} {
			for _, sigils := range []int{0, 2, 3, 5} {
				for _, fieldCount := range []int{1, 5} {
					t.Run(fmt.Sprintf("%s/mode%d/sigils%d/field%d", side, option, sigils, fieldCount), func(t *testing.T) {
						state := testState()
						state.Turn.Active = side
						actor := state.Players[side]
						actor.PP, actor.MaxPP, actor.Leader.Life = 10, 10, 18
						actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10132310, DeclaredType: "spell"})
						for n := 0; n < fieldCount; n++ {
							actor = withInstance(actor, "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", 100+n), CardID: 90021110, DeclaredType: "follower"})
						}
						if sigils > 0 {
							actor.Zones["field"][fieldCount-1] = ir.TestInstance{InstanceID: fmt.Sprintf("%032x", 100+fieldCount-1), CardID: 90031210, DeclaredType: "amulet", Overrides: ir.InstanceOverrides{Earthsigil: &sigils}}
						}
						state.Players[side] = actor
						other := oppositeSide(side)
						state.Players[other] = withInstance(state.Players[other], "field", ir.TestInstance{InstanceID: strings.Repeat("2", 32), CardID: 90021110, DeclaredType: "follower"})
						s, err := NewSession(pack, state, 1)
						if err != nil {
							t.Fatal(err)
						}
						step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID})
						if step.Status != StatusSuspended || len(step.Choice.Candidates) != 3 {
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
						choice := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: option}
						for _, run := range []*Session{s, restored} {
							if result := run.Resume(choice); result.Status != StatusCompleted {
								t.Fatal(result)
							}
							p := run.g.player(side)
							wantSigils, wantCount, wantShadows, wantLife := sigils, fieldCount, 1, 18
							if option == 1 && (sigils > 0 || fieldCount < 5) {
								wantSigils += 4
								if sigils == 0 {
									wantCount++
								}
							}
							if option == 2 {
								wantLife = 20
							}
							if option == 3 && sigils >= 3 {
								wantSigils -= 3
								if sigils == 3 {
									wantCount--
									wantShadows++
								}
							}
							if len(p.field) != wantCount || p.shadows != wantShadows || p.pp != 7 || p.leaderLife != wantLife {
								t.Fatalf("player=%#v", p)
							}
							total := 0
							for _, card := range p.field {
								total += card.earthsigil
							}
							if total != wantSigils {
								t.Fatalf("sigils=%d want=%d", total, wantSigils)
							}
							if dead := run.g.instances[strings.Repeat("2", 32)].zone == "graveyard"; dead != (option == 3 && sigils >= 3) {
								t.Fatal("earth rite payment and damage disagree")
							}
							if run.g.rng.Consumed() != 0 {
								t.Fatal("deterministic mode used randomness")
							}
						}
						if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
							t.Fatal("mode restore diverged")
						}
					})
				}
			}
		}
	}
}

func TestEarthSigilProtectionMergingAndConsumption(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			state := testState()
			state.Turn.Active = side
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			g := s.g
			anchor := g.summonFor(nil, side, 1, 90021110, false)[0]
			relic := g.summonFor(anchor, "own", 1, 10001210, false)[0]
			sigil := g.summonFor(anchor, "own", 1, 90031210, false)[0]
			sigil.earthsigil = 3
			enemy := g.summonFor(anchor, "oppo", 1, 90021110, false)[0]
			selection := ir.SelectionEffect{Kind: "choose", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field"}}
			if contains(g.selectionCandidates(selection, enemy, nil), sigil) {
				t.Fatal("enemy could select earth sigil")
			}
			selection.Source = ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field"}
			if !contains(g.selectionCandidates(selection, anchor, nil), sigil) {
				t.Fatal("owner could not select earth sigil")
			}
			selection.Kind = "random_choose"
			selection.Source = ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field"}
			if !contains(g.selectionCandidates(selection, enemy, nil), sigil) {
				t.Fatal("random query incorrectly excluded sigil")
			}
			g.destroyByEffect([]*instance{sigil})
			if sigil.zone != "field" {
				t.Fatal("ability destroyed earth sigil")
			}
			fresh := g.summonFor(anchor, "own", 1, 90031210, false)[0]
			if sigil.zone != "banished" || fresh.earthsigil != 4 || g.player(side).shadows != 0 {
				t.Fatal("summoned sigil did not merge without destruction")
			}
			if !g.consumeEarthSigil(anchor, 0) || fresh.earthsigil != 4 || anchor.zone != "field" || relic.zone != "field" {
				t.Fatal("zero payment changed field")
			}
			if g.consumeEarthSigil(anchor, 5) || fresh.earthsigil != 4 {
				t.Fatal("insufficient payment changed sigils")
			}
			if !g.consumeEarthSigil(anchor, 4) || fresh.zone != "graveyard" || anchor.zone != "field" || relic.zone != "field" || g.player(side).shadows != 1 {
				t.Fatal("exhaustion lost ordinary cards or did not destroy sigil")
			}
			g.execAdjust(ir.AdjustEffect{Kind: "adjust_earthsigil", Delta: 0}, anchor, nil)
			if len(g.player(side).field) != 2 {
				t.Fatal("zero gain created a sigil")
			}
			for _, event := range g.events {
				if event.Kind == "follower_summoned" && (event.CardID == 90031210 || event.CardID == 10001210) {
					t.Fatal("amulet emitted follower summon")
				}
			}
		})
	}
}
