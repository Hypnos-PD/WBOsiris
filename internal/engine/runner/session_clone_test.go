package runner

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/engine/ir"
	"wbo/internal/engine/project"
)

// TestSessionCloneIsDeepAndIndependent 守住搜索的地基：
//
// 克隆必须"两边各自独立"——在克隆上继续推进不能影响原会话，反之亦然。
// 只要有一个容器（实例 map、双方区域、栈、触发队列、RNG）是共享的，
// 搜索就会悄悄污染真实对局，而这种错在搜索结果的统计里很难看出来。
func TestSessionCloneIsDeepAndIndependent(t *testing.T) {
	pack := repeatCardPack(t)
	state := crestState("own")
	own := state.Players["own"]
	own.PP, own.MaxPP = 6, 10
	state.Players["own"] = withInstance(own, "hand", ir.TestInstance{InstanceID: strings.Repeat("1", 32), CardID: 10661110, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 7)
	if err != nil {
		t.Fatal(err)
	}
	clone, err := session.Clone()
	if err != nil {
		t.Fatal(err)
	}
	before, err := session.View("own")
	if err != nil {
		t.Fatal(err)
	}
	cloned, err := clone.View("own")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, cloned) {
		t.Fatalf("克隆出来的局面与原局面不一致：%+v vs %+v", before, cloned)
	}
	if !reflect.DeepEqual(session.LegalActionsFor("own"), clone.LegalActionsFor("own")) {
		t.Fatal("克隆出来的合法动作与原局面不一致")
	}
	// 在克隆上结束回合：原会话必须**一点都没变**（PP/回合/手牌都一样）。
	if result := clone.SubmitAs(strings.Repeat("c", 32), "own", SimulatorCommand{Kind: "end_turn"}); result.Status != StatusCompleted {
		t.Fatalf("克隆上结束回合失败：%s %s", result.Status, result.ErrorCode)
	}
	after, err := session.View("own")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("原会话被克隆的推进污染了：%+v → %+v", before, after)
	}
	advanced, err := clone.View("own")
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(after, advanced) {
		t.Fatal("克隆推进之后局面没变，说明克隆是假的")
	}
}

// BenchmarkSessionClone 量一次克隆的成本：搜索每层都要克隆，
// 它决定了 MCTS 的深度与候选数上限（原 JSON 续局路线是 5ms）。
func BenchmarkSessionClone(b *testing.B) {
	// repeatCardPack 只接受 *testing.T；基准里用同一套加载路径自己装一份。
	root := filepath.Join("..", "..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, true, root)
	if loaded.HasErrors() {
		b.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		b.Fatal(err)
	}
	state := crestState("own")
	session, err := NewSession(pack, state, 11)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, err := session.Clone(); err != nil {
			b.Fatal(err)
		}
	}
}
