package main

import (
	"path/filepath"
	"testing"

	"wbo/internal/project"
)

// 环境协议的核心路径：随机动作也要能把一局打完，且终局奖励与胜负一致。
// 协议层（JSON-lines）由 WBDecima 侧的冒烟测试覆盖，这里测的是同一套会话逻辑。
func TestEnvSessionPlaysRandomEpisode(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, false, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	cards, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	for seed := uint64(1); seed <= 5; seed++ {
		session, err := newEnvSession(cards, envCommand{Cmd: "reset", Seed: seed, Opponent: "random"})
		if err != nil {
			t.Fatal(err)
		}
		event, err := session.advance(cards)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		steps := 0
		for !event.Done {
			if len(event.Legal) == 0 {
				t.Fatalf("seed %d: state without legal actions", seed)
			}
			if err := session.step(cards, int(seed+uint64(steps))%len(event.Legal)); err != nil {
				t.Fatalf("seed %d step %d: %v", seed, steps, err)
			}
			event, err = session.advance(cards)
			if err != nil {
				t.Fatalf("seed %d step %d: %v", seed, steps, err)
			}
			if steps++; steps > 500 {
				t.Fatalf("seed %d: episode did not finish", seed)
			}
		}
		if event.Winner != "own" && event.Winner != "oppo" {
			t.Fatalf("seed %d: winner = %q", seed, event.Winner)
		}
		wantReward := -1.0
		if event.Winner == "own" {
			wantReward = 1
		}
		if event.Reward != wantReward {
			t.Fatalf("seed %d: reward = %v, want %v", seed, event.Reward, wantReward)
		}
	}
}
