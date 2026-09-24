package runner

import (
	"path/filepath"
	"strings"
	"testing"

	"wbo/internal/engine/ir"
	"wbo/internal/engine/project"
)

// 官方 QA（h3225qzf5）：『古旧天剑·伊德梅塔』的【进化时】发生两次信仰值-5，
// 信仰就获得两份「通过爆能强化使用卡牌时，使自己的战场上的所有随从+1/+1」，
// 因此一次爆能强化的打出会发动两次强化。
// 场景测试的 DSL 每个 scenario 只允许一个主动作，所以这里用引擎入口驱动两次进化。
func TestFaithGrantStacksAcrossRepeatedEvolutions(t *testing.T) {
	const (
		blades = 10624120 // 古旧天剑·伊德梅塔
		probe  = 10621110 // 勇烈的士兵（【爆能强化_3】本随从+1/+1）
	)
	root := filepath.Join("..", "..", "..")
	loaded := project.LoadWithRoot([]string{
		filepath.Join(root, "cards", "10006", "10624120.wbo"),
		filepath.Join(root, "cards", "10006", "10621110.wbo"),
		filepath.Join(root, "cards", "90000", "90024320.wbo"),
	}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	state := testState()
	actor := state.Players["own"]
	actor.PP, actor.MaxPP = 3, 10
	actor.EP = 2
	faithID, firstID, secondID, probeID := strings.Repeat("f", 32), strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	actor = withInstance(actor, "crests", ir.TestInstance{InstanceID: faithID, CardID: blades, DeclaredType: "faith"})
	actor = withInstance(actor, "field", ir.TestInstance{InstanceID: firstID, CardID: blades, DeclaredType: "follower"})
	actor = withInstance(actor, "field", ir.TestInstance{InstanceID: secondID, CardID: blades, DeclaredType: "follower"})
	actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: probeID, CardID: probe, DeclaredType: "follower"})
	state.Players["own"] = actor
	session, err := NewSession(pack, state, 71008)
	if err != nil {
		t.Fatal(err)
	}
	faith := session.g.instances[faithID]
	faith.counters["value"] = 10
	if step := session.SubmitAs(strings.Repeat("a", 32), "own", SimulatorCommand{Kind: "evolve", Source: firstID}); step.Status != StatusCompleted {
		t.Fatalf("first evolution failed: %#v", step)
	}
	// 一回合只能进化一次；这里跨回合地再进化一次（测试只关心"信仰被授予两次"）。
	session.g.own.evolvedThisTurn = false
	if step := session.SubmitAs(strings.Repeat("c", 32), "own", SimulatorCommand{Kind: "evolve", Source: secondID}); step.Status != StatusCompleted {
		t.Fatalf("second evolution failed: %#v", step)
	}
	if faith.counters["value"] != 0 {
		t.Fatalf("faith value after two -5 payments = %d, want 0", faith.counters["value"])
	}
	if len(faith.grants) != 2 {
		t.Fatalf("faith grants = %d, want 2", len(faith.grants))
	}
	if result := session.SubmitAs(strings.Repeat("b", 32), "own", SimulatorCommand{Kind: "play", Source: probeID}); result.Status != StatusCompleted {
		t.Fatal(result)
	}
	played := session.g.instances[probeID]
	// 2/2 + 【爆能强化_3】的 +1/+1（自己）+ 两份信仰强化的 +1/+1。
	if played.attack != 5 || played.life != 5 {
		t.Fatalf("stats = %d/%d, want 5/5", played.attack, played.life)
	}
}
