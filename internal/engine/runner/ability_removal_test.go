package runner

import (
	"path/filepath"
	"strings"
	"testing"

	"wbo/internal/engine/ir"
	"wbo/internal/engine/project"
)

// 腐臭的僵尸：谢幕曲召唤一个已经失去谢幕曲的后继，所以第二次破坏不再产生新个体。
// 场景测试只能观察"死后场上还剩 1 个僵尸"，链是否真的停止由这里验证。
func TestZombieLastwordsRemovalStopsTheChain(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards", "90000", "90051140.wbo")}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	zombieID := strings.Repeat("1", 32)
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: zombieID, CardID: 90051140, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	session.actionID = strings.Repeat("a", 32)
	session.g.destroyByEffect([]*instance{session.g.instances[zombieID]})
	if step := session.run(); step.Status != StatusCompleted {
		t.Fatalf("last words did not resolve: %#v", step)
	}
	if zombies := fieldCardCount(session.g.own.field, 90051140); zombies != 1 || session.g.own.shadows != 1 {
		t.Fatalf("after first death zombies=%d shadows=%d", zombies, session.g.own.shadows)
	}
	successor := session.g.own.field[0]
	if len(successor.suppressed) != 1 || !successor.suppressed["lastwords"] {
		t.Fatalf("successor kept its last words: %#v", successor.suppressed)
	}
	session.actionID = strings.Repeat("b", 32)
	session.g.destroyByEffect([]*instance{successor})
	if step := session.run(); step.Status != StatusCompleted {
		t.Fatalf("second death failed: %#v", step)
	}
	if zombies := fieldCardCount(session.g.own.field, 90051140); zombies != 0 || session.g.own.shadows != 2 {
		t.Fatalf("chain continued: zombies=%d shadows=%d", zombies, session.g.own.shadows)
	}
}

// remove all abilities from …：关键字、附加能力与卡面触发能力一起失效。
func TestRemoveAllAbilitiesDropsKeywordsAndTriggers(t *testing.T) {
	const source, summoned = 70000001, 70000002
	abilityID := strings.Repeat("1", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: source, CardType: "follower", Stats: &ir.Stats{Attack: 2, Life: 2}, Intrinsic: []string{"ward"}, Abilities: []ir.Ability{{
			ID: abilityID, Trigger: ir.SimpleTrigger{Kind: "lastwords"},
			Body: []ir.Effect{ir.CardEffect{Kind: "summon", Owner: "own", Count: 1, CardID: summoned, Output: "summoned"}},
		}}},
		{ID: summoned, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
	}}
	targetID := strings.Repeat("2", 32)
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: targetID, CardID: source, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	target := session.g.instances[targetID]
	if !target.abilities["ward"] {
		t.Fatal("ward was not loaded from the card definition")
	}
	session.g.removeAbility(target, "all")
	if target.abilities["ward"] || len(target.grants) != 0 || !target.suppressAll {
		t.Fatalf("all abilities survived: %#v", target)
	}
	session.actionID = strings.Repeat("c", 32)
	session.g.destroyByEffect([]*instance{target})
	if step := session.run(); step.Status != StatusCompleted {
		t.Fatalf("death after removal failed: %#v", step)
	}
	if fieldCardCount(session.g.own.field, summoned) != 0 {
		t.Fatal("last words survived remove all abilities")
	}
}

func fieldCardCount(field []*instance, cardID int) int {
	count := 0
	for _, i := range field {
		if i.card.ID == cardID {
			count++
		}
	}
	return count
}
