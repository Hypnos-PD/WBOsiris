package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"wbo/internal/ai"
	"wbo/internal/project"
	"wbo/internal/ruleset"
	"wbo/internal/runner"
)

// runSelfplay 批量自对弈：不看胜负，看"能不能一直跑完"。
// 崩溃、非法动作、超时、回合上限、执行预算超限都会被报出来。
func runSelfplay(args []string) int {
	args = interspersed(args, map[string]bool{"--source-root": true, "--games": true, "--seed": true, "--policy": true, "--max-turns": true, "--format": true, "--random-deck": false})
	fs := flag.NewFlagSet("selfplay", flag.ContinueOnError)
	root := fs.String("source-root", "", "稳定源路径根目录（默认当前工作目录）")
	games := fs.Int("games", 1, "对局数量")
	seed := fs.Uint64("seed", 1, "起始种子（每局 +1）")
	policyName := fs.String("policy", "greedy", "双方策略：greedy 或 random")
	maxTurns := fs.Int("max-turns", 120, "单局回合上限，超过记为异常")
	formatID := fs.String("format", runner.FormatRotation, "赛制：rotation（指定模式）或 unlimited（无限制模式）")
	randomDeck := fs.Bool("random-deck", false, "每局按赛制随机生成双方卡组（覆盖整个卡池）")
	quiet := fs.Bool("quiet", false, "只输出汇总")
	fs.SetOutput(os.Stderr)
	if fs.Parse(args) != nil {
		return 2
	}
	if *games <= 0 {
		fmt.Fprintln(os.Stderr, "selfplay 需要 --games 大于 0")
		return 2
	}
	paths := fs.Args()
	if len(paths) == 0 {
		paths = []string{"cards"}
	}
	loaded := load(paths, true, *root)
	printDiagnostics(loaded.Diagnostics)
	if loaded.HasErrors() {
		return 1
	}
	cards, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		fmt.Fprintln(os.Stderr, "编译卡池失败:", err)
		return 1
	}
	policy := func(value uint64) ai.Policy {
		if *policyName == "random" {
			return ai.NewRandom(value)
		}
		return &ai.Greedy{}
	}
	if *policyName != "greedy" && *policyName != "random" {
		fmt.Fprintln(os.Stderr, "未知策略（只支持 greedy 或 random）:", *policyName)
		return 2
	}
	deck := runner.PracticeDeck()
	format, err := runner.FormatByID(cards, *formatID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := runner.ValidateDeckForFormat(cards, deck, format); err != nil {
		fmt.Fprintln(os.Stderr, "默认练习卡组在该赛制下不合法:", err)
		return 1
	}
	limits := ai.Limits{MaxTurns: *maxTurns}
	wins := map[string]int{}
	faults := map[string]int{}
	seen := map[int]bool{}
	failures := 0
	totalTurns, totalActions := 0, 0
	for n := 0; n < *games; n++ {
		matchSeed := *seed + uint64(n)
		first := "own"
		if matchSeed%2 == 1 {
			first = "oppo"
		}
		session, err := runner.NewMatchSessionWithFirstPlayer(cards, deck, deck, matchSeed, first)
		if *randomDeck {
			ownDeck, deckErr := ai.RandomDeck(cards, format, ruleset.NewRNG(matchSeed*2+1))
			if deckErr != nil {
				fmt.Fprintln(os.Stderr, "生成随机卡组失败:", deckErr)
				return 1
			}
			oppoDeck, deckErr := ai.RandomDeck(cards, format, ruleset.NewRNG(matchSeed*2+2))
			if deckErr != nil {
				fmt.Fprintln(os.Stderr, "生成随机卡组失败:", deckErr)
				return 1
			}
			session, err = runner.NewMatchSessionWithFirstPlayer(cards, ownDeck, oppoDeck, matchSeed, first)
			for _, id := range append(append([]int(nil), ownDeck...), oppoDeck...) {
				seen[id] = true
			}
		} else {
			for _, id := range deck {
				seen[id] = true
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "初始化对局失败:", err)
			return 1
		}
		result, err := ai.PlayMatch(session, policy(matchSeed*2+1), policy(matchSeed*2+2), limits)
		if err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "第 %d 局（种子 %d）失败: %v\n", n+1, matchSeed, err)
			continue
		}
		if result.Fault != "" {
			faults[result.Fault]++
			if !*quiet {
				fmt.Fprintf(os.Stderr, "第 %d 局（种子 %d）异常: %s\n", n+1, matchSeed, result.Fault)
			}
		} else {
			wins[result.Winner]++
		}
		totalTurns += result.Turns
		totalActions += result.Actions + result.Choices
	}
	deckLabel := "练习卡组"
	if *randomDeck {
		deckLabel = "随机卡组"
	}
	fmt.Fprintf(os.Stdout, "自对弈 %d 局（策略 %s · 赛制 %s · %s）\n", *games, *policyName, format.ID, deckLabel)
	fmt.Fprintf(os.Stdout, "先手胜 %d · 后手胜 %d · 异常 %d · 引擎/策略报错 %d\n", wins["own"], wins["oppo"], sumFaults(faults), failures)
	if len(faults) > 0 {
		keys := make([]string, 0, len(faults))
		for key := range faults {
			keys = append(keys, key)
		}
		sortStrings(keys)
		for _, key := range keys {
			fmt.Fprintf(os.Stdout, "  - %s × %d\n", key, faults[key])
		}
	}
	if *games > 0 {
		fmt.Fprintf(os.Stdout, "平均回合 %.1f · 平均动作 %.1f\n", float64(totalTurns)/float64(*games), float64(totalActions)/float64(*games))
	}
	available := 0
	for n := range cards.Cards {
		if runner.MatchCardUnavailableReason(&cards.Cards[n]) == "" && format.AllowsPack(cards.Cards[n].Meta.Pack) {
			available++
		}
	}
	fmt.Fprintf(os.Stdout, "卡池覆盖 %d/%d 张（%.0f%%）\n", len(seen), available, 100*float64(len(seen))/float64(max(available, 1)))
	if failures > 0 || len(faults) > 0 {
		return 1
	}
	return 0
}

func sumFaults(faults map[string]int) int {
	total := 0
	for _, count := range faults {
		total += count
	}
	return total
}

func sortStrings(values []string) {
	for n := 1; n < len(values); n++ {
		for k := n; k > 0 && strings.Compare(values[k], values[k-1]) < 0; k-- {
			values[k], values[k-1] = values[k-1], values[k]
		}
	}
}
