package runner

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/project"
)

func TestCompiledTraitsAndBatIdentity(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[int]string{
		10031210: "earthsigil",
		10121130: "luminous", 10122110: "luminous", 10122120: "luminous", 10122130: "luminous",
		10123110: "levin", 10123310: "levin", 10124110: "levin",
		10132110: "mysteria", 10132130: "mysteria", 10133310: "mysteria", 10134120: "mysteria",
		10144110: "anathema", 10164110: "anathema",
		90031140: "shikigami", 90051110: "departed",
	}
	for _, card := range pack.Cards {
		if want := expected[card.ID]; want != "" {
			if !slices.Equal(card.Traits, []string{want}) {
				t.Errorf("card %d traits=%v, want %s", card.ID, card.Traits, want)
			}
			delete(expected, card.ID)
		}
	}
	if len(expected) != 0 {
		t.Fatalf("missing trait-bearing cards: %v", expected)
	}
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			state := testState()
			state.Turn.Active = side
			sourceID, batID, otherID, enemyID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32)
			actor := state.Players[side]
			actor.PP, actor.MaxPP = 10, 10
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10151110, DeclaredType: "follower"})
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: batID, CardID: 90051120, DeclaredType: "follower"})
			state.Players[side] = withInstance(actor, "hand", ir.TestInstance{InstanceID: otherID, CardID: 90001110, DeclaredType: "follower"})
			opponent := oppositeSide(side)
			enemy := state.Players[opponent]
			enemy.PP, enemy.MaxPP = 10, 10
			state.Players[opponent] = withInstance(enemy, "hand", ir.TestInstance{InstanceID: enemyID, CardID: 90051120, DeclaredType: "follower"})
			session, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			if step := session.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); step.Status != StatusCompleted {
				t.Fatal(step)
			}
			view, _ := session.View(side)
			if len(view.Own.Field) != 2 || view.Own.LeaderLife != 19 || view.Own.Field[1].CardID != 90051120 || !slices.Contains(view.Own.Field[1].Keywords, "storm") || slices.Contains(view.Own.Field[0].Keywords, "storm") {
				t.Fatalf("fanfare bat did not receive identity-triggered storm: %#v", view.Own)
			}
			if step := session.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "play", Source: otherID}); step.Status != StatusCompleted {
				t.Fatal(step)
			}
			view, _ = session.View(side)
			if view.Own.LeaderLife != 19 || session.g.instances[otherID].abilities["storm"] {
				t.Fatal("non-bat incorrectly triggered the ability")
			}
			if step := session.SubmitAs(strings.Repeat("c", 32), side, SimulatorCommand{Kind: "play", Source: batID}); step.Status != StatusCompleted {
				t.Fatal(step)
			}
			view, _ = session.View(side)
			if view.Own.LeaderLife != 18 || !session.g.instances[batID].abilities["storm"] {
				t.Fatal("bat played from hand missed the identity trigger")
			}
			if step := session.SubmitAs(strings.Repeat("d", 32), side, SimulatorCommand{Kind: "end_turn"}); step.Status != StatusCompleted {
				t.Fatal(step)
			}
			if step := session.SubmitAs(strings.Repeat("e", 32), opponent, SimulatorCommand{Kind: "play", Source: enemyID}); step.Status != StatusCompleted {
				t.Fatal(step)
			}
			view, _ = session.View(side)
			if view.Own.LeaderLife != 18 || view.Oppo.LeaderLife != 20 || session.g.instances[enemyID].abilities["storm"] {
				t.Fatal("enemy bat triggered an allied ability")
			}
		})
	}
}
