package runner

import (
	"path/filepath"
	"strings"
	"testing"

	"wbo/internal/engine/ir"
	"wbo/internal/engine/project"
)

// 伊卡洛斯的飞翔给手牌中的创造物附加【谢幕曲】抽 1 张；
// 场景测试只能看到"获得突进"，谢幕曲是否真的发动要在这里走完整流程。
func TestIcarusFlightGrantsADrawingLastWords(t *testing.T) {
	pack := loadCardsForTest(t, "10002/10272310", "90000/90072110", "90000/90073110", "90000/90073120", "90000/90073130", "90000/90074110", "10000/10001110")
	spellID, artifactID, deckID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	state := testState()
	own := state.Players["own"]
	own.PP, own.MaxPP = 6, 6
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: spellID, CardID: 10272310, DeclaredType: "spell"})
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: artifactID, CardID: 90072110, DeclaredType: "follower"})
	own = withInstance(own, "deck", ir.TestInstance{InstanceID: deckID, CardID: 10001110, DeclaredType: "follower"})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: spellID}); result.Status != StatusSuspended {
		t.Fatalf("spell did not ask for a target: %#v", result)
	}
	choice := session.PendingChoice()
	if result := session.Resume(ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{artifactID}}); result.Status != StatusCompleted {
		t.Fatalf("grant did not resolve: %#v", result)
	}
	artifact := session.g.instances[artifactID]
	if !artifact.abilities["rush"] || len(artifact.grants) != 1 {
		t.Fatalf("artifact did not gain rush and a granted ability: %#v", artifact)
	}
	if result := session.Begin(strings.Repeat("b", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: artifactID}); result.Status != StatusCompleted {
		t.Fatalf("playing the artifact failed: %#v", result)
	}
	if result := session.Begin(strings.Repeat("c", 32), ir.SourceAction{Kind: "end_turn", Actor: "own"}); result.Status != StatusCompleted {
		t.Fatalf("ending the turn failed: %#v", result)
	}
	// 结束回合会消耗掉手牌里的牌，因此只比较结算前后的抽牌数量。
	before := handCardCount(session.g.own.hand, 10001110)
	session.actionID = strings.Repeat("d", 32)
	session.g.destroyByEffect([]*instance{session.g.instances[artifactID]})
	if step := session.run(); step.Status != StatusCompleted {
		t.Fatalf("last words did not resolve: %#v", step)
	}
	if after := handCardCount(session.g.own.hand, 10001110); after != before+1 {
		t.Fatalf("granted last words drew %d cards, want 1", after-before)
	}
}

// 恶意的神谕把「自己的回合结束时破坏本卡牌」附加给敌方随从，
// 因此它只在对手自己的回合结束时发动。
func TestMaliciousOracleDestroysTheEnemyFollowerAtItsOwnersTurnEnd(t *testing.T) {
	pack := loadCardsForTest(t, "10002/10261120", "10000/10001110")
	sourceID, enemyID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	state := testState()
	own := state.Players["own"]
	own.PP, own.MaxPP = 2, 2
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10261120, DeclaredType: "follower"})
	state.Players["own"] = own
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: enemyID, CardID: 10001110, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID}); result.Status != StatusSuspended {
		t.Fatalf("fanfare did not ask for a target: %#v", result)
	}
	choice := session.PendingChoice()
	if result := session.Resume(ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{enemyID}}); result.Status != StatusCompleted {
		t.Fatalf("grant did not resolve: %#v", result)
	}
	if session.g.instances[enemyID].zone != "field" || len(session.g.instances[enemyID].grants) != 1 {
		t.Fatalf("enemy follower did not receive the granted ability: %#v", session.g.instances[enemyID])
	}
	if result := session.Begin(strings.Repeat("b", 32), ir.SourceAction{Kind: "end_turn", Actor: "own"}); result.Status != StatusCompleted {
		t.Fatalf("own turn did not end: %#v", result)
	}
	if session.g.instances[enemyID].zone != "field" {
		t.Fatal("granted ability fired on the wrong player's turn end")
	}
	if result := session.Begin(strings.Repeat("c", 32), ir.SourceAction{Kind: "end_turn", Actor: "oppo"}); result.Status != StatusCompleted {
		t.Fatalf("opponent turn did not end: %#v", result)
	}
	if session.g.instances[enemyID].zone != "graveyard" {
		t.Fatalf("enemy follower survived its controller's turn end: %s", session.g.instances[enemyID].zone)
	}
}

// 炎龙之剑：启动 1 破坏自身、强化并附加「谢幕曲：召唤1张炎龙之剑」。
// 场景测试只能看到强化，谢幕曲是否真的召唤出护符由这里验证。
func TestFlameDragonSwordGrantSummonsItself(t *testing.T) {
	pack := loadCardsForTest(t, "10002/10242210", "10000/10001110")
	swordID, unitID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	state := testState()
	own := state.Players["own"]
	own.PP, own.MaxPP = 1, 1
	own = withInstance(own, "field", ir.TestInstance{InstanceID: swordID, CardID: 10242210, DeclaredType: "amulet"})
	own = withInstance(own, "field", ir.TestInstance{InstanceID: unitID, CardID: 10001110, DeclaredType: "follower"})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "engage", Actor: "own", Source: swordID}); result.Status != StatusSuspended {
		t.Fatalf("engage did not ask for a target: %#v", result)
	}
	choice := session.PendingChoice()
	if result := session.Resume(ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{unitID}}); result.Status != StatusCompleted {
		t.Fatalf("engage did not resolve: %#v", result)
	}
	unit := session.g.instances[unitID]
	if session.g.instances[swordID].zone != "graveyard" || unit.attack != 3 || unit.life != 3 || len(unit.grants) != 1 {
		t.Fatalf("engage did not buff and grant: sword=%s unit=%d/%d grants=%d", session.g.instances[swordID].zone, unit.attack, unit.life, len(unit.grants))
	}
	session.actionID = strings.Repeat("b", 32)
	session.g.destroyByEffect([]*instance{unit})
	if step := session.run(); step.Status != StatusCompleted {
		t.Fatalf("granted last words did not resolve: %#v", step)
	}
	if count := fieldCardCount(session.g.own.field, 10242210); count != 1 {
		t.Fatalf("flame dragon sword summoned %d copies, want 1", count)
	}
}

// 缠绕密林·丽梅格：超进化时给敌方随从附加"回合结束时对自己的主战者造成 1 点、
// 对自己造成 2 点伤害"。这个触发只在它自己控制者的回合结束时发动，场景测试
// 走不到对手的回合结束，所以由这里验证。
func TestUntamedWildGrantPunishesItsOwnController(t *testing.T) {
	pack := loadCardsForTest(t, "10002/10214120", "10000/10001110")
	sourceID, enemyID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	state := testState()
	state.Turn.Active, state.Turn.Number = "own", 7
	own := state.Players["own"]
	own.SEP = 1
	own = withInstance(own, "field", ir.TestInstance{InstanceID: sourceID, CardID: 10214120, DeclaredType: "follower"})
	state.Players["own"] = own
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: enemyID, CardID: 10001110, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "superevolve", Actor: "own", Source: sourceID}); result.Status != StatusSuspended {
		t.Fatalf("super-evolve did not ask for targets: %#v", result)
	}
	choice := session.PendingChoice()
	if result := session.Resume(ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{enemyID}}); result.Status != StatusCompleted {
		t.Fatalf("grant did not resolve: %#v", result)
	}
	if len(session.g.instances[enemyID].grants) != 1 {
		t.Fatalf("enemy follower did not receive the granted ability: %#v", session.g.instances[enemyID].grants)
	}
	if result := session.Begin(strings.Repeat("b", 32), ir.SourceAction{Kind: "end_turn", Actor: "own"}); result.Status != StatusCompleted {
		t.Fatalf("own turn did not end: %#v", result)
	}
	if session.g.oppo.leaderLife != 20 || session.g.instances[enemyID].life != 2 {
		t.Fatal("granted ability fired before its controller's turn end")
	}
	if result := session.Begin(strings.Repeat("c", 32), ir.SourceAction{Kind: "end_turn", Actor: "oppo"}); result.Status != StatusCompleted {
		t.Fatalf("opponent turn did not end: %#v", result)
	}
	if life := session.g.oppo.leaderLife; life != 19 {
		t.Fatalf("opponent leader life = %d, want 19", life)
	}
	if stats := session.g.instances[enemyID]; stats.life != 0 || stats.zone != "graveyard" {
		t.Fatalf("granted self damage did not destroy the follower: %d/%s", stats.life, stats.zone)
	}
}

// 「不会被能力破坏」只挡能力造成的破坏：必杀这类战斗规则造成的破坏仍然生效。
func TestAbilityDestructionGuardBlocksEffectDestructionOnly(t *testing.T) {
	const guarded, plain = 70000001, 70000002
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: guarded, CardType: "follower", Stats: &ir.Stats{Attack: 2, Life: 2}, Intrinsic: []string{"ability_destruction_guard"}},
		{ID: plain, CardType: "follower", Stats: &ir.Stats{Attack: 2, Life: 2}},
	}}
	guardedID := strings.Repeat("1", 32)
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: guardedID, CardID: guarded, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	target := session.g.instances[guardedID]
	session.actionID = strings.Repeat("a", 32)
	session.g.destroyByEffect([]*instance{target})
	if step := session.run(); step.Status != StatusCompleted {
		t.Fatalf("effect destruction failed: %#v", step)
	}
	if target.zone != "field" {
		t.Fatalf("ability destruction guard did not protect the follower: %s", target.zone)
	}
	session.actionID = strings.Repeat("b", 32)
	session.g.destroyByCombat([]*instance{target})
	if step := session.run(); step.Status != StatusCompleted {
		t.Fatalf("combat destruction failed: %#v", step)
	}
	if target.zone != "graveyard" {
		t.Fatalf("bane style destruction was blocked: %s", target.zone)
	}
}

func loadCardsForTest(t *testing.T, relative ...string) *ir.CardPack {
	t.Helper()
	root := filepath.Join("..", "..", "..")
	paths := make([]string, 0, len(relative))
	for _, path := range relative {
		paths = append(paths, filepath.Join(root, "cards", filepath.FromSlash(path)+".wbo"))
	}
	loaded := project.LoadWithRoot(paths, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

func handCardCount(hand []*instance, cardID int) int {
	count := 0
	for _, i := range hand {
		if i.card.ID == cardID {
			count++
		}
	}
	return count
}
