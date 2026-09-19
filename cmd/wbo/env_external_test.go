package main

import (
	"path/filepath"
	"testing"

	"wbo/internal/ai"
	"wbo/internal/ir"
	"wbo/internal/project"
	"wbo/internal/ruleset"
	"wbo/internal/runner"
)

type externalRun struct {
	winner string
	turns  int
	steps  int
	own    int
	oppo   int
}

// playExternal 双方都由"环境外"的策略驱动（自对弈/联赛的走法）。
func playExternal(t *testing.T, cards *ir.CardPack, seed uint64) externalRun {
	t.Helper()
	session, err := newEnvSession(cards, envCommand{Cmd: "reset", Seed: seed, Opponent: "external"})
	if err != nil {
		t.Fatal(err)
	}
	event, err := session.advance(cards)
	if err != nil {
		t.Fatal(err)
	}
	run := externalRun{}
	for !event.Done {
		if event.Side != "own" && event.Side != "oppo" {
			t.Fatalf("状态事件的 side 非法：%q", event.Side)
		}
		if event.Side == "own" {
			run.own++
		} else {
			run.oppo++
		}
		if len(event.Legal) == 0 {
			t.Fatalf("第 %d 步没有合法动作", run.steps)
		}
		index := int((seed + uint64(run.steps)) % uint64(len(event.Legal)))
		if err := session.step(cards, index); err != nil {
			t.Fatalf("第 %d 步（%s）：%v", run.steps, event.Side, err)
		}
		event, err = session.advance(cards)
		if err != nil {
			t.Fatalf("第 %d 步之后：%v", run.steps, err)
		}
		if run.steps++; run.steps > 800 {
			t.Fatal("对局没有在步数上限内结束")
		}
	}
	run.winner, run.turns = event.Winner, event.Turn
	return run
}

func loadEnvCards(t *testing.T) *ir.CardPack {
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

// external 模式：两边都给训练侧决定，且同一 seed 与同一策略下结果确定。
func TestEnvExternalDriveBothSides(t *testing.T) {
	cards := loadEnvCards(t)
	totalOwn, totalOppo := 0, 0
	for seed := uint64(1); seed <= 4; seed++ {
		first := playExternal(t, cards, seed)
		second := playExternal(t, cards, seed)
		if first != second {
			t.Fatalf("seed %d：同一策略两次运行不一致 %+v vs %+v", seed, first, second)
		}
		if first.winner != "own" && first.winner != "oppo" {
			t.Fatalf("seed %d：没有产出胜负 %+v", seed, first)
		}
		totalOwn += first.own
		totalOppo += first.oppo
	}
	if totalOwn == 0 || totalOppo == 0 {
		t.Fatalf("external 模式下双方都应有决定：own=%d oppo=%d", totalOwn, totalOppo)
	}
}

// deck 命令给出的是合法卡组：长度 40、同名字段合法、能直接用于 reset。
func TestEnvDeckCommandReturnsLegalDeck(t *testing.T) {
	cards := loadEnvCards(t)
	format, err := runner.FormatByID(cards, "rotation")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for seed := uint64(1); seed <= 5; seed++ {
		deck, err := ai.RandomDeck(cards, format, ruleset.NewRNG(seed))
		if err != nil {
			t.Fatal(err)
		}
		if len(deck) != 40 {
			t.Fatalf("seed %d：卡组 %d 张", seed, len(deck))
		}
		if err := runner.ValidateDeckForFormat(cards, deck, format); err != nil {
			t.Fatalf("seed %d：卡组不合法 %v", seed, err)
		}
		// 同一 seed 必须给出同一副卡组（可复现）。
		again, err := ai.RandomDeck(cards, format, ruleset.NewRNG(seed))
		if err != nil {
			t.Fatal(err)
		}
		for index := range deck {
			if deck[index] != again[index] {
				t.Fatalf("seed %d：同一 seed 两次生成的卡组不同", seed)
			}
		}
		for _, card := range deck {
			seen[card] = true
		}
	}
	if len(seen) < 10 {
		t.Fatalf("随机卡组覆盖太窄：只用到 %d 张不同卡", len(seen))
	}
}

// lookahead 不能改变真实对局：revision 与后续走向都必须保持不变。
func TestEnvLookaheadDoesNotTouchTheRealSession(t *testing.T) {
	cards := loadEnvCards(t)
	session, err := newEnvSession(cards, envCommand{Cmd: "reset", Seed: 5, Opponent: "greedy"})
	if err != nil {
		t.Fatal(err)
	}
	event, err := session.advance(cards)
	if err != nil {
		t.Fatal(err)
	}
	if event.Done {
		t.Fatal("第 5 号种子第一手就结束了？")
	}
	before, err := session.session.View(session.learner)
	if err != nil {
		t.Fatal(err)
	}
	// 对每个候选都做一次假想推进，真实会话必须纹丝不动。
	for index := range event.Legal {
		preview, err := session.lookahead(cards, index)
		if err != nil {
			t.Fatalf("候选 %d：%v", index, err)
		}
		if !preview.Lookahead {
			t.Fatalf("候选 %d：返回的事件没有 lookahead 标记", index)
		}
		if preview.Type != "state" && preview.Type != "done" {
			t.Fatalf("候选 %d：事件类型 %q", index, preview.Type)
		}
	}
	after, err := session.session.View(session.learner)
	if err != nil {
		t.Fatal(err)
	}
	if before.Revision != after.Revision || before.Turn.Number != after.Turn.Number {
		t.Fatalf("假想推进污染了真实对局：revision %d→%d、turn %d→%d",
			before.Revision, after.Revision, before.Turn.Number, after.Turn.Number)
	}
	// 假想推进是确定的：同一候选两次结果一致。
	first, err := session.lookahead(cards, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.lookahead(cards, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.Turn != second.Turn || len(first.Legal) != len(second.Legal) {
		t.Fatalf("同一候选两次假想推进结果不一致：%+v vs %+v", first, second)
	}
}
