package search

import (
	"path/filepath"
	"testing"
	"time"

	"wbo/internal/ai"
	"wbo/internal/ir"
	"wbo/internal/project"
	"wbo/internal/runner"
)

func loadCards(t testing.TB) *ir.CardPack {
	t.Helper()
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, false, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	cards, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	return cards
}

// startSession 开一局并推进到学习者需要行动的位置（先让双方摸牌、开始对局）。
func startSession(t testing.TB, cards *ir.CardPack, seed uint64) *runner.Session {
	t.Helper()
	session, err := runner.NewMatchSession(cards, runner.PracticeDeck(), runner.PracticeDeck(), seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := ai.KeepAllMulligan(session); err != nil {
		t.Fatal(err)
	}
	return session
}

func TestCloneIsIndependentOfOriginal(t *testing.T) {
	cards := loadCards(t)
	session := startSession(t, cards, 7)
	clone, err := Clone(cards, session)
	if err != nil {
		t.Fatal(err)
	}
	before, err := session.View("own")
	if err != nil {
		t.Fatal(err)
	}
	if len(clone.LegalActionsFor("own")) == 0 {
		t.Fatal("克隆出来的会话没有合法动作")
	}
	// 在克隆里走一步（或回答一次选择），原会话必须完全不受影响。
	driver := ai.NewDriver("own", ai.NewRandom(1), ai.Limits{})
	if _, err := driver.Step(clone); err != nil {
		t.Fatal(err)
	}
	after, err := session.View("own")
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || after.Turn.Number != before.Turn.Number {
		t.Fatalf("原会话被克隆的推进影响了：revision %d→%d、turn %d→%d",
			before.Revision, after.Revision, before.Turn.Number, after.Turn.Number)
	}
}

func TestRandomPlayoutFinishesAndIsDeterministic(t *testing.T) {
	cards := loadCards(t)
	session := startSession(t, cards, 11)
	first, err := RandomPlayout(cards, session, 5)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RandomPlayout(cards, session, 5)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("同一 seed 的推演结果不一致：%+v vs %+v", first, second)
	}
	if first.Winner != "own" && first.Winner != "oppo" {
		t.Fatalf("推演没有产出胜负：%+v", first)
	}
	if first.Reward != 1 && first.Reward != -1 {
		t.Fatalf("奖励与胜负不一致：%+v", first)
	}
	if first.Turns <= 0 || first.Actions <= 0 {
		t.Fatalf("推演统计异常：%+v", first)
	}
}

// 这两个基准回答"搜索有没有预算可用"：克隆成本决定搜索分支数，推演成本决定评估次数。
func BenchmarkClone(b *testing.B) {
	cards := loadCards(b)
	session := startSession(b, cards, 3)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Clone(cards, session); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRandomPlayout(b *testing.B) {
	cards := loadCards(b)
	session := startSession(b, cards, 3)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := RandomPlayout(cards, session, uint64(i)+1); err != nil {
			b.Fatal(err)
		}
	}
}

// 诊断：把克隆的成本拆开看——续局 JSON 有多大、编码/解码/恢复各占多少。
// 这组数字直接决定"搜索能不能住在 Go 里"。
func TestCloneCostDiagnostics(t *testing.T) {
	cards := loadCards(t)
	session := startSession(t, cards, 3)

	start := time.Now()
	data, err := session.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	encodeCost := time.Since(start)

	start = time.Now()
	continuation, err := runner.DecodeContinuation(data)
	if err != nil {
		t.Fatal(err)
	}
	decodeCost := time.Since(start)

	start = time.Now()
	if _, err := runner.RestoreSession(cards, continuation); err != nil {
		t.Fatal(err)
	}
	restoreCost := time.Since(start)

	t.Logf("续局 JSON %d 字节；编码 %v，解码 %v，恢复 %v", len(data), encodeCost, decodeCost, restoreCost)
}

func BenchmarkEncodeContinuationOnly(b *testing.B) {
	cards := loadCards(b)
	session := startSession(b, cards, 3)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := session.EncodeContinuation(); err != nil {
			b.Fatal(err)
		}
	}
}

// 恢复是克隆成本的另一半（解码 + 重建会话）。单独测量才能判断该优化哪一边。
func BenchmarkRestoreOnly(b *testing.B) {
	cards := loadCards(b)
	session := startSession(b, cards, 3)
	data, err := session.EncodeContinuation()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		continuation, err := runner.DecodeContinuation(data)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := runner.RestoreSession(cards, continuation); err != nil {
			b.Fatal(err)
		}
	}
}
