// 选择（target/mode/fusion_material）在多选场景下的端到端覆盖与确定性验证。
package main

import (
	"path/filepath"
	"reflect"
	"testing"

	"wbo/internal/engine/ai"
	"wbo/internal/engine/project"
	"wbo/internal/engine/ruleset"
	"wbo/internal/engine/runner"
)

// 随机卡组才能碰到多选（mode / fusion_material / 多目标）这类选择请求：
// 练习卡组 12 局里只有单选，所以这里用随机卡组做端到端覆盖，
// 并验证"同一 seed + 同一策略 => 同一结果"的确定性。
func TestEnvMultiSelectChoicesArePlayableAndDeterministic(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, false, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	cards, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	format, err := runner.FormatByID(cards, "rotation")
	if err != nil {
		t.Fatal(err)
	}
	type episodeResult struct {
		winner       string
		turn         int
		choices      int
		multiSelects int
		kinds        map[string]int
	}
	play := func(seed uint64) (episodeResult, bool) {
		rng := ruleset.NewRNG(seed * 7919)
		learner, err := ai.RandomDeck(cards, format, rng)
		if err != nil {
			t.Fatal(err)
		}
		opponent, err := ai.RandomDeck(cards, format, rng)
		if err != nil {
			t.Fatal(err)
		}
		session, err := newEnvSession(cards, envCommand{Cmd: "reset", Seed: seed, Deck: learner, OppoDeck: opponent, Opponent: "greedy"})
		if err != nil {
			return episodeResult{}, false // 随机卡组偶尔不合法，跳过
		}
		event, err := session.advance(cards)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		result := episodeResult{kinds: map[string]int{}}
		steps := 0
		for !event.Done && steps < 2000 {
			if len(event.Legal) == 0 {
				t.Fatalf("seed %d: 空合法动作", seed)
			}
			if event.Choice != nil {
				result.choices++
				if event.Choice.MaxSelections > 1 || event.Choice.MinSelections > 1 {
					result.multiSelects++
					result.kinds[event.Choice.Kind]++
				}
			}
			index := int((seed + uint64(steps)) % uint64(len(event.Legal)))
			if err := session.step(cards, index); err != nil {
				t.Fatalf("seed %d step %d: %v", seed, steps, err)
			}
			event, err = session.advance(cards)
			if err != nil {
				t.Fatalf("seed %d step %d: %v", seed, steps, err)
			}
			steps++
		}
		if !event.Done {
			t.Fatalf("seed %d: 未在步数上限内结束", seed)
		}
		result.winner, result.turn = event.Winner, event.Turn
		return result, true
	}

	multi, choices, games := 0, 0, 0
	kinds := map[string]int{}
	for seed := uint64(1); seed <= 40; seed++ {
		result, ok := play(seed)
		if !ok {
			continue
		}
		games++
		choices += result.choices
		multi += result.multiSelects
		for kind, count := range result.kinds {
			kinds[kind] += count
		}
		if result.winner != "own" && result.winner != "oppo" {
			t.Fatalf("seed %d: winner = %q", seed, result.winner)
		}
	}
	if games < 30 || choices < 30 {
		t.Fatalf("随机卡组覆盖不足：games=%d choices=%d", games, choices)
	}
	if multi < 5 {
		t.Fatalf("多选覆盖不足：multi=%d（kinds=%v）", multi, kinds)
	}
	if len(kinds) < 2 {
		t.Fatalf("多选种类覆盖不足：%v", kinds)
	}
	// 确定性：同一 seed 两次运行的胜负、回合数、选择次数一致。
	first, okFirst := play(35)
	second, okSecond := play(35)
	if okFirst != okSecond || first.winner != second.winner || first.turn != second.turn ||
		first.choices != second.choices || first.multiSelects != second.multiSelects ||
		!reflect.DeepEqual(first.kinds, second.kinds) {
		t.Fatalf("seed 35 两次运行不一致：%+v vs %+v", first, second)
	}
	t.Logf("随机卡组 %d 局：选择事件 %d，多选 %d，种类 %v", games, choices, multi, kinds)
}
