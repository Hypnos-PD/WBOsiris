package main

import (
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/project"
	"wbo/internal/runner"
)

// 多选选择的累积规则：只允许加选（子步骤数天然有界，不会来回循环），
// 达到下限才能 confirm，达到上限自动提交。
func TestEnvChoiceAccumulationSupportsSelectAndConfirm(t *testing.T) {
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
	if countKind(legal, "select") != 3 || countKind(legal, "confirm") != 1 {
		t.Fatalf("选过 x2 后应剩 3 select + 1 confirm，实际 %+v", legal)
	}
	// 已选过的候选不能再选（否则可能无限循环）。
	for _, action := range legal {
		if action.Source == "e:x2" {
			t.Fatalf("已选候选不应再次出现：%+v", action)
		}
	}

	// 再选一个（共两个）后 confirm，应带着这两个选择提交。
	if _, complete, err := session.choiceStep(pending, firstOfKind(legal, "select")); err != nil || complete {
		t.Fatalf("选到两个时不应提交（err=%v complete=%v）", err, complete)
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

// formats 命令给出当前赛制与卡包窗口（构筑/训练侧按赛制筛卡时的唯一真值来源）。
func TestEnvFormatsReportsPackWindows(t *testing.T) {
	cards := loadEnvCards(t)
	format, err := runner.FormatByID(cards, "rotation")
	if err != nil {
		t.Fatal(err)
	}
	if len(format.Packs) == 0 {
		t.Fatal("指定模式应当有卡包窗口")
	}
	unlimited, err := runner.FormatByID(cards, "unlimited")
	if err != nil {
		t.Fatal(err)
	}
	if len(unlimited.Packs) < len(format.Packs) {
		t.Fatalf("无限制模式的卡包不应少于指定模式：%d < %d", len(unlimited.Packs), len(format.Packs))
	}
}

// oracle 是训练专用特权信息：默认关闭（客户端/回放路径拿不到），
// 显式传 oracle=true 时才附带对手手牌内容与双方牌库顺序。
func TestEnvOracleIsOptInAndCarriesHiddenInfo(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, false, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	cards, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}

	// 默认（不传 oracle）：事件里绝不能出现特权字段
	plain, err := newEnvSession(cards, envCommand{Cmd: "reset", Seed: 7, Opponent: "external"})
	if err != nil {
		t.Fatal(err)
	}
	event, err := plain.advance(cards)
	if err != nil {
		t.Fatal(err)
	}
	plain.attachOracle(&event)
	if event.Oracle != nil {
		t.Fatalf("未开启 oracle 时不应附带特权信息：%+v", event.Oracle)
	}

	// 开启 oracle：应能看到对手手牌内容与双方牌库前 8 张
	session, err := newEnvSession(cards, envCommand{Cmd: "reset", Seed: 7, Opponent: "external", Oracle: true})
	if err != nil {
		t.Fatal(err)
	}
	event, err = session.advance(cards)
	if err != nil {
		t.Fatal(err)
	}
	event.Seed = session.seed
	session.attachOracle(&event)
	if event.Oracle == nil {
		t.Fatal("开启 oracle 后应附带特权信息")
	}
	if len(event.Oracle.OppoHand) != event.View.Oppo.HandCount {
		t.Fatalf("对手手牌张数不一致：oracle %d vs view %d",
			len(event.Oracle.OppoHand), event.View.Oppo.HandCount)
	}
	// 特权信息要"完整"：双方牌库给的是**全序**（不是前几张的截断），
	// 长度必须与公开视角的牌库张数对得上（手牌已经抽走了，所以两者相加而不是相等）。
	if len(event.Oracle.OppoDeck) == 0 || len(event.Oracle.OwnDeck) == 0 {
		t.Fatal("双方牌库全序不应为空")
	}
	if len(event.Oracle.OppoDeck) != event.View.Oppo.DeckCount {
		t.Fatalf("对手牌库全序长度 %d 与公开张数 %d 不一致",
			len(event.Oracle.OppoDeck), event.View.Oppo.DeckCount)
	}
	if len(event.Oracle.OwnDeck) != event.View.Own.DeckCount {
		t.Fatalf("自己牌库全序长度 %d 与公开张数 %d 不一致",
			len(event.Oracle.OwnDeck), event.View.Own.DeckCount)
	}
	if event.View.Oppo.Hand != nil {
		t.Fatal("特权信息不应该泄漏进玩家视角的手牌字段")
	}

	// 顺序必须是**真顺序**：对手抽一张之后，新的全序应该正好是旧全序去掉头部。
	before := append([]int(nil), event.Oracle.OppoDeck...)
	if err := session.step(cards, 0); err != nil {
		t.Fatal(err)
	}
	next, err := session.advance(cards)
	if err != nil {
		t.Fatal(err)
	}
	session.attachOracle(&next)
	after := next.Oracle.OppoDeck
	if len(after) == len(before)-1 {
		for index, id := range after {
			if id != before[index+1] {
				t.Fatalf("牌库全序不是真顺序：抽牌后第 %d 张 %d ≠ 抽牌前第 %d 张 %d",
					index, id, index+1, before[index+1])
			}
		}
	}
}

// history 是每个 state 事件附带的"最近可见事件"窗口：训练侧靠它区分
// 「先出 A 再出 B」与「先出 B 再出 A」，而这在聚合量观测里是完全一样的。
func TestEnvStateCarriesVisibleHistoryWindow(t *testing.T) {
	cards := loadEnvCards(t)
	session, err := newEnvSession(cards, envCommand{Cmd: "reset", Seed: 3, Opponent: "external"})
	if err != nil {
		t.Fatal(err)
	}
	event, err := session.advance(cards)
	if err != nil {
		t.Fatal(err)
	}
	session.attachHistory(&event)
	if len(event.History) != 0 {
		t.Fatalf("开局第一步还没有事件：%d 条", len(event.History))
	}

	// 走几步：历史必须单调变长（直到上限），且顺序与 Sequence 一致。
	previous := 0
	for step := 0; step < 6 && !event.Done; step++ {
		if len(event.Legal) == 0 {
			t.Fatal("state 事件必须有合法动作")
		}
		if err := session.step(cards, step%len(event.Legal)); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		event, err = session.advance(cards)
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		session.attachHistory(&event)
		if len(event.History) < previous {
			t.Fatalf("历史窗口不应变短：%d → %d", previous, len(event.History))
		}
		if len(event.History) > envHistoryLimit {
			t.Fatalf("历史窗口超过上限 %d：%d", envHistoryLimit, len(event.History))
		}
		previous = len(event.History)
	}
	if previous == 0 {
		t.Fatal("走了几步之后历史窗口不应为空")
	}
	for index, item := range event.History {
		if index == 0 {
			continue
		}
		if item.Sequence < event.History[index-1].Sequence {
			t.Fatalf("历史窗口必须按 Sequence 升序：%d 在 %d 之后",
				event.History[index-1].Sequence, item.Sequence)
		}
	}

	// 特权事件不许进窗口：开启 oracle 也不行（窗口走的是 EventsFor 的脱敏路径）。
	for _, item := range event.History {
		if item.PrivateTo != "" && item.PrivateTo != event.Side {
			if item.InstanceID != "" || item.CardID != 0 {
				t.Fatalf("对手私有事件的细节不该出现在窗口里：%+v", item)
			}
		}
	}
}
