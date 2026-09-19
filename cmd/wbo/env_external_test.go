package main

import (
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/project"
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
