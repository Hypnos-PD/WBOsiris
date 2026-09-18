package runner

import (
	"path/filepath"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/project"
)

// 真红与群青·塞达&贝阿朵丽丝（10424110）：入场曲召唤自己的复制体，爆能强化 6 的
// "使其获得【毁灭】"指向那个复制体。场景测试无法只给复制体断言关键词（本体没有【毁灭】），
// 所以这条链路由 Go 测试覆盖：一次打出的入场曲与爆能强化共享同一个打出帧。
func TestEnhanceBlockReadsFanfareOutputs(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards", "10004", "10424110.wbo")}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	sourceID := strings.Repeat("1", 32)
	state := testState()
	actor := state.Players["own"]
	actor.PP, actor.MaxPP = 6, 6
	actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10424110, DeclaredType: "follower"})
	state.Players["own"] = actor
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	result := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID})
	if result.Status != StatusCompleted {
		t.Fatalf("play did not complete: %#v", result)
	}
	copies, banes := 0, 0
	for _, i := range session.g.own.field {
		if i.card.ID != 10424110 {
			continue
		}
		copies++
		if i.abilities["bane"] {
			banes++
		}
	}
	if copies != 2 || banes != 1 {
		t.Fatalf("copies=%d banes=%d; want 2 copies and exactly one with bane", copies, banes)
	}
	if !session.g.instances[sourceID].abilities["storm"] {
		t.Fatal("the enhanced source did not gain storm")
	}
}
