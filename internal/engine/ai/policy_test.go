package ai

import (
	"path/filepath"
	"testing"

	"wbo/internal/engine/project"
	"wbo/internal/engine/runner"
)

func greedyView() runner.StateView {
	return runner.StateView{
		Viewer: "oppo",
		Turn:   runner.TurnView{Active: "own", Number: 6},
		Phase:  "main",
		Own: runner.PlayerView{
			PP: 5, MaxPP: 5, LeaderLife: 20,
			Field: []runner.EntityView{
				{InstanceID: "attacker", CardID: 10001110, Cost: 2, Attack: 4, Life: 5, AttackLimit: 1},
				{InstanceID: "small", CardID: 10001110, Cost: 2, Attack: 1, Life: 1, AttackLimit: 1},
			},
			Hand: []runner.EntityView{{InstanceID: "inhand", CardID: 10001110, Cost: 3, Attack: 3, Life: 3}},
		},
		Oppo: runner.PlayerView{
			LeaderLife: 3,
			Field:      []runner.EntityView{{InstanceID: "blocker", CardID: 10001110, Attack: 2, Life: 2, AttackLimit: 1}},
		},
	}
}

// 能斩杀就斩杀：主战者只剩 3 点而手里有 4/5 的攻击动作。
func TestGreedyPrefersLethalAttack(t *testing.T) {
	view := greedyView()
	actions := []runner.LegalAction{
		{Kind: "play", Actor: "oppo", Source: "inhand"},
		{Kind: "attack_entity", Actor: "oppo", Source: "attacker", Defender: "blocker"},
		{Kind: "attack_leader", Actor: "oppo", Source: "attacker", Defender: "own"},
		{Kind: "end_turn", Actor: "oppo"},
	}
	chosen, ok := (&Greedy{}).Choose(view, actions)
	if !ok || chosen.Kind != "attack_leader" || chosen.Source != "attacker" {
		t.Fatalf("greedy chose %#v, want the lethal attack_leader", chosen)
	}
	command := CommandFor(chosen)
	if command.Kind != "attack" || command.Defender != "" {
		t.Fatalf("leader attack must submit as attack without defender: %#v", command)
	}
}

// 不斩杀时优先做有利交换，而不是无脑打脸。
func TestGreedyPrefersFavourableTrade(t *testing.T) {
	view := greedyView()
	view.Oppo.LeaderLife = 20
	actions := []runner.LegalAction{
		{Kind: "attack_leader", Actor: "oppo", Source: "attacker", Defender: "own"},
		{Kind: "attack_entity", Actor: "oppo", Source: "attacker", Defender: "blocker"},
		{Kind: "end_turn", Actor: "oppo"},
	}
	chosen, ok := (&Greedy{}).Choose(view, actions)
	if !ok || chosen.Kind != "attack_entity" || chosen.Defender != "blocker" {
		t.Fatalf("greedy chose %#v, want the favourable trade", chosen)
	}
	if command := CommandFor(chosen); command.Kind != "attack" || command.Defender != "blocker" {
		t.Fatalf("entity attack must carry the defender: %#v", command)
	}
}

// 花费能量点：同样的合法动作里，优先打出更贵的牌。
func TestGreedySpendsPlayPointsOnBiggerCards(t *testing.T) {
	view := greedyView()
	view.Oppo.LeaderLife = 20
	view.Own.Field = nil
	view.Own.Hand = []runner.EntityView{
		{InstanceID: "cheap", CardID: 10001110, Cost: 1},
		{InstanceID: "pricey", CardID: 10001110, Cost: 5},
	}
	actions := []runner.LegalAction{
		{Kind: "play", Actor: "oppo", Source: "cheap"},
		{Kind: "play", Actor: "oppo", Source: "pricey"},
		{Kind: "end_turn", Actor: "oppo"},
	}
	chosen, ok := (&Greedy{}).Choose(view, actions)
	if !ok || chosen.Source != "pricey" {
		t.Fatalf("greedy chose %#v, want the pricier card", chosen)
	}
}

// 额外能量点只在"能解锁一张本来打不出的牌"时才用。
func TestGreedyUsesExtraPPOnlyWhenItUnlocksAPlay(t *testing.T) {
	view := greedyView()
	view.Own.PP = 4
	view.Own.Hand = []runner.EntityView{{InstanceID: "big", CardID: 10001110, Cost: 5, Attack: 5, Life: 5}}
	actions := []runner.LegalAction{
		{Kind: "use_extra_pp", Actor: "oppo"},
		{Kind: "end_turn", Actor: "oppo"},
	}
	if chosen, ok := (&Greedy{}).Choose(view, actions); !ok || chosen.Kind != "use_extra_pp" {
		t.Fatalf("greedy chose %#v, want use_extra_pp", chosen)
	}
	view.Own.Hand = nil
	if chosen, ok := (&Greedy{}).Choose(view, actions); !ok || chosen.Kind != "end_turn" {
		t.Fatalf("greedy chose %#v, want end_turn when nothing is unlocked", chosen)
	}
}

// 选择请求：模式按编号从小到大；目标优先对手随从，其次对手主战者。
func TestGreedyAnswersChoices(t *testing.T) {
	view := greedyView()
	mode := runner.ChoiceRequest{
		RequestID: "r1", ActionID: "a1", Kind: "mode", MinSelections: 2, MaxSelections: 2,
		Candidates: []runner.ChoiceCandidate{{Kind: "option", OptionID: 3}, {Kind: "option", OptionID: 1}, {Kind: "option", OptionID: 2}},
	}
	response := (&Greedy{}).Answer(view, mode)
	if len(response.SelectedOptionIDs) != 2 || response.SelectedOptionIDs[0] != 1 || response.SelectedOptionIDs[1] != 2 {
		t.Fatalf("mode answer = %#v, want options 1 and 2", response.SelectedOptionIDs)
	}
	if response.RequestID != "r1" || response.ActionID != "a1" {
		t.Fatalf("answer must echo the request identity: %#v", response)
	}
	target := runner.ChoiceRequest{
		RequestID: "r2", ActionID: "a2", Kind: "target", MinSelections: 1, MaxSelections: 1,
		Candidates: []runner.ChoiceCandidate{
			{Kind: "entity", InstanceID: "attacker"},
			{Kind: "leader", LeaderSide: "oppo"},
			{Kind: "entity", InstanceID: "blocker"},
		},
	}
	// 对手主战者的引擎座位是 own（观察者是 oppo），所以这里应当挑 blocker 而不是自己的随从。
	response = (&Greedy{}).Answer(view, target)
	if len(response.SelectedInstanceIDs) != 1 || response.SelectedInstanceIDs[0] != "blocker" {
		t.Fatalf("target answer = %#v, want the enemy follower", response.SelectedInstanceIDs)
	}
}

// Random 只从合法动作里挑，并且同种子可复现。
func TestRandomIsDeterministicAndLegal(t *testing.T) {
	view := greedyView()
	actions := []runner.LegalAction{{Kind: "play", Source: "inhand"}, {Kind: "attack_leader", Source: "attacker"}, {Kind: "end_turn"}}
	run := func() []string {
		policy := NewRandom(7)
		sequence := make([]string, 0, 8)
		for n := 0; n < 8; n++ {
			chosen, ok := policy.Choose(view, actions)
			if !ok {
				t.Fatal("random declined to act")
			}
			sequence = append(sequence, chosen.Kind+":"+chosen.Source)
		}
		return sequence
	}
	first, second := run(), run()
	for n := range first {
		if first[n] != second[n] {
			t.Fatalf("random is not reproducible at step %d: %s vs %s", n, first[n], second[n])
		}
	}
}

// 端到端：随机策略也能把一局打完（这是"能对战"的最低标准，也是压测入口）。
func TestSelfPlayFinishesWithTheRealCardPool(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	deck := runner.PracticeDeck()
	session, err := runner.NewMatchSessionWithFirstPlayer(pack, deck, deck, 99, "own")
	if err != nil {
		t.Fatal(err)
	}
	result, err := PlayMatch(session, NewRandom(99), NewRandom(199), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fault != "" {
		t.Fatalf("self play faulted: %s", result.Fault)
	}
	if result.Winner != "own" && result.Winner != "oppo" {
		t.Fatalf("self play produced no winner: %#v", result)
	}
	if result.Actions == 0 || result.Turns == 0 {
		t.Fatalf("self play did nothing: %#v", result)
	}
}
