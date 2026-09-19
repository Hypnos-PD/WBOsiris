package main

import (
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/project"
	"wbo/internal/runner"
)

// 多选选择的累积规则：可自由选择/撤销（保证任何合法组合都可达），
// 达到下限才能 confirm，达到上限自动提交。
func TestEnvChoiceAccumulationSupportsSelectConfirmAndDeselect(t *testing.T) {
	pending := runner.ChoiceRequest{
		RequestID: "req", ActionID: "act", StateRevision: 7,
		Kind: "target", MinSelections: 1, MaxSelections: 3,
		Candidates: []runner.ChoiceCandidate{
			{Kind: "entity", InstanceID: "x1"},
			{Kind: "entity", InstanceID: "x2"},
			{Kind: "entity", InstanceID: "x3"},
			{Kind: "entity", InstanceID: "x4"},
		},
	}
	session := &envSession{learner: "own"}
	session.resetChoice(pending.RequestID)

	legal := session.choiceLegal(pending)
	if len(legal) != 4 {
		t.Fatalf("初始应有 4 个 select，实际 %d 个：%+v", len(legal), legal)
	}
	for _, action := range legal {
		if action.Kind != "select" {
			t.Fatalf("未做任何选择时不应出现 confirm/deselect：%+v", action)
		}
	}
	if _, complete, err := session.choiceStep(pending, 1); err != nil || complete {
		t.Fatalf("选第 2 个候选后不应提交（err=%v complete=%v）", err, complete)
	}

	legal = session.choiceLegal(pending)
	if countKind(legal, "select") != 3 || countKind(legal, "deselect") != 1 || countKind(legal, "confirm") != 1 {
		t.Fatalf("选过 x2 后应剩 3 select + 1 deselect + 1 confirm，实际 %+v", legal)
	}
	// 撤销刚才的选择，回到初始状态。
	deselect := firstOfKind(legal, "deselect")
	if legal[deselect].Source != "e:x2" {
		t.Fatalf("撤销的应该是刚选的 x2：%+v", legal[deselect])
	}
	if _, complete, err := session.choiceStep(pending, deselect); err != nil || complete {
		t.Fatalf("撤销后不应提交（err=%v complete=%v）", err, complete)
	}
	legal = session.choiceLegal(pending)
	if countKind(legal, "select") != 4 || countKind(legal, "deselect") != 0 || countKind(legal, "confirm") != 0 {
		t.Fatalf("撤销后应回到初始可用集合，实际 %+v", legal)
	}

	// 选两个后 confirm，应带着这两个选择提交。
	if _, complete, err := session.choiceStep(pending, firstOfKind(legal, "select")); err != nil || complete {
		t.Fatalf("第一次选择不应提交（err=%v complete=%v）", err, complete)
	}
	legal = session.choiceLegal(pending)
	if _, complete, err := session.choiceStep(pending, firstOfKind(legal, "select")); err != nil || complete {
		t.Fatalf("第二次选择不应提交（err=%v complete=%v）", err, complete)
	}
	legal = session.choiceLegal(pending)
	response, complete, err := session.choiceStep(pending, firstOfKind(legal, "confirm"))
	if err != nil || !complete {
		t.Fatalf("confirm 应产生完整响应（err=%v complete=%v）", err, complete)
	}
	if len(response.SelectedInstanceIDs) != 2 {
		t.Fatalf("选择的实体不对：%+v", response.SelectedInstanceIDs)
	}
	if response.RequestID != pending.RequestID || response.StateRevision != pending.StateRevision {
		t.Fatalf("响应必须带回 requestId/stateRevision：%+v", response)
	}

	// 达到上限时自动提交，不需要 confirm，也不会走进死路。
	session.resetChoice(pending.RequestID)
	for index := 0; index < 3; index++ {
		legal = session.choiceLegal(pending)
		response, complete, err = session.choiceStep(pending, firstOfKind(legal, "select"))
		if err != nil {
			t.Fatal(err)
		}
		if want := index == 2; complete != want {
			t.Fatalf("第 %d 次选择后 complete=%v，期望 %v", index+1, complete, want)
		}
	}
	if len(response.SelectedInstanceIDs) != 3 {
		t.Fatalf("自动提交应带上 3 个选择：%+v", response.SelectedInstanceIDs)
	}
}

func countKind(legal []runner.LegalAction, kind string) int {
	count := 0
	for _, action := range legal {
		if action.Kind == kind {
			count++
		}
	}
	return count
}

func firstOfKind(legal []runner.LegalAction, kind string) int {
	for index, action := range legal {
		if action.Kind == kind {
			return index
		}
	}
	return -1
}

// envStats 是一局的统计结果。
type envStats struct {
	winner       string
	turn         int
	actions      int
	choiceEvents int
	selects      int
	confirms     int
	multiSelect  int
}

// playEpisode 用确定性的"计数器取模"策略跑完一局，并统计选择类动作。
// 学习者的选择在 v2 里是自回归子动作（select/confirm），这里同时验证它们可用。
func playEpisode(t *testing.T, cards *ir.CardPack, seed uint64) envStats {
	t.Helper()
	session, err := newEnvSession(cards, envCommand{Cmd: "reset", Seed: seed, Opponent: "random"})
	if err != nil {
		t.Fatalf("seed %d: %v", seed, err)
	}
	event, err := session.advance(cards)
	if err != nil {
		t.Fatalf("seed %d: %v", seed, err)
	}
	stats := envStats{}
	for !event.Done {
		if len(event.Legal) == 0 {
			t.Fatalf("seed %d: state without legal actions", seed)
		}
		if event.Choice != nil {
			stats.choiceEvents++
			if event.Choice.MaxSelections > 1 {
				stats.multiSelect++
			}
		}
		index := int((seed + uint64(stats.actions)) % uint64(len(event.Legal)))
		switch event.Legal[index].Kind {
		case "select":
			stats.selects++
		case "confirm":
			stats.confirms++
		}
		if err := session.step(cards, index); err != nil {
			t.Fatalf("seed %d step %d: %v", seed, stats.actions, err)
		}
		event, err = session.advance(cards)
		if err != nil {
			t.Fatalf("seed %d step %d: %v", seed, stats.actions, err)
		}
		if stats.actions++; stats.actions > 800 {
			t.Fatalf("seed %d: episode did not finish", seed)
		}
	}
	stats.winner, stats.turn = event.Winner, event.Turn
	return stats
}

// v2 的核心契约：学习者的选择通过 select/confirm 子动作完成；
// 同一 seed 与同一策略下结果、动作数、选择次数都必须一致（确定性）。
func TestEnvSessionChoiceSubActionsAreDeterministic(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, false, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	cards, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	totalChoices, totalSelects, totalMulti := 0, 0, 0
	for seed := uint64(1); seed <= 12; seed++ {
		first := playEpisode(t, cards, seed)
		second := playEpisode(t, cards, seed)
		if first != second {
			t.Fatalf("seed %d: 同一策略下两次运行不一致: %+v vs %+v", seed, first, second)
		}
		if first.winner != "own" && first.winner != "oppo" {
			t.Fatalf("seed %d: winner = %q", seed, first.winner)
		}
		totalChoices += first.choiceEvents
		totalSelects += first.selects
		totalMulti += first.multiSelect
	}
	if totalChoices == 0 {
		t.Fatal("练习卡组 + 随机对手下没有出现任何学习者选择，协议 v2 的选择路径未被覆盖")
	}
	if totalSelects == 0 {
		t.Fatal("选择事件出现了但没有 select 子动作")
	}
	t.Logf("12 局共 %d 次选择事件、%d 次 select、%d 次多选", totalChoices, totalSelects, totalMulti)
}

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
