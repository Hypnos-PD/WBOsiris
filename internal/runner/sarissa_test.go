package runner

import (
	"reflect"
	"strings"
	"testing"
	"wbo/internal/ir"
)

func TestSarissaChecksDestroyedFollowerKeywordsAndOwner(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		for _, mode := range []string{"native", "temporary", "removed", "enemy", "nonward", "banish", "return", "transform"} {
			t.Run(owner+"/"+mode, func(t *testing.T) {
				state := testState()
				source, victim := strings.Repeat("a", 32), strings.Repeat("b", 32)
				state.Players[owner] = withInstance(state.Players[owner], "field", ir.TestInstance{InstanceID: source, CardID: 10162110, DeclaredType: "follower"})
				victimOwner, card := owner, 10001130
				if mode == "enemy" {
					victimOwner = oppositeSide(owner)
				}
				if mode == "temporary" || mode == "nonward" {
					card = 10001110
				}
				state.Players[victimOwner] = withInstance(state.Players[victimOwner], "field", ir.TestInstance{InstanceID: victim, CardID: card, DeclaredType: "follower"})
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				v := s.g.instances[victim]
				if mode == "temporary" {
					v.addKeyword("ward", owner)
				}
				if mode == "removed" {
					v.removeKeyword("ward")
				}
				switch mode {
				case "banish":
					s.g.move(v, "banished")
				case "return":
					s.g.returnCard(v, "hand")
				case "transform":
					s.g.execCardEffect(ir.CardEffect{Kind: "transform", Target: ir.SelfRef{Kind: "self"}, CardID: 10001110}, v, nil)
				default:
					s.g.destroyByEffect([]*instance{v})
				}
				if r := s.run(); r.Status != StatusCompleted {
					t.Fatal(r)
				}
				want := 2
				if mode == "native" || mode == "temporary" {
					want++
				}
				if i := s.g.instances[source]; i.attack != want || i.life != want {
					t.Fatal("wrong Sarissa stats", mode, i.attack, i.life)
				}
			})
		}
	}
}

func TestDestroyedKeywordDecisionAndBindingSurviveRestore(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID != 10162110 {
			continue
		}
		a := &pack.Cards[n].Abilities[0]
		a.Body = append([]ir.Effect{
			ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "return", Target: ir.BindingRef{Kind: "binding", Name: "destroyed"}, Destination: "hand"},
			ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}},
		}, a.Body...)
	}
	for _, owner := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active = owner
		p := state.Players[owner]
		p.PP = 1
		p.MaxPP = 1
		source, victim, enemy, spell := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32)
		p = withInstance(p, "field", ir.TestInstance{InstanceID: source, CardID: 10162110, DeclaredType: "follower"})
		p = withInstance(p, "field", ir.TestInstance{InstanceID: victim, CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Keywords: []string{"ward"}}})
		p = withInstance(p, "hand", ir.TestInstance{InstanceID: spell, CardID: 10151310, DeclaredType: "spell"})
		state.Players[owner] = p
		other := oppositeSide(owner)
		state.Players[other] = withInstance(state.Players[other], "field", ir.TestInstance{InstanceID: enemy, CardID: 10001110, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 3)
		if err != nil {
			t.Fatal(err)
		}
		s.g.instances[victim].attack = 6
		s.g.instances[victim].cost = 0
		step := s.SubmitAs(strings.Repeat("a", 32), owner, SimulatorCommand{Kind: "play", Source: spell})
		for _, selected := range []string{victim, enemy} {
			if step.Status != StatusSuspended {
				t.Fatal(step)
			}
			step = s.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{selected}})
		}
		if v := s.g.instances[victim]; step.Status != StatusSuspended || v.zone != "hand" || v.abilities["ward"] || v.cost != 2 || v.attack != 2 {
			t.Fatal("binding did not return the destroyed card", step)
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
		for _, current := range []*Session{s, restored} {
			r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1})
			if r.Status != StatusCompleted || current.g.instances[source].attack != 3 {
				t.Fatal("destruction eligibility changed on return", r)
			}
			history := current.g.fromRef(ir.FilterRef{Kind: "filter", Source: ir.ZoneRef{Kind: "zone", Side: owner, Zone: "destroyed", Member: "follower"}, Predicate: ir.FieldPredicate{Kind: "has_keyword", Keyword: "ward"}}, nil, nil)
			if len(history) != 1 || history[0].id != victim {
				t.Fatal("lost destruction keyword snapshot", history)
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("destruction continuation diverged")
		}
	}
}
