package runner

import (
	"strings"
	"testing"

	"wbo/internal/engine/ir"
)

// 【结晶】：以较低费用当作护符打出，卡面换成衍生护符；吟唱归零后按其谢幕曲召唤本体。
func TestCrystallizeCountsDownAndSummonsTheFollower(t *testing.T) {
	pack := repeatCardPack(t)
	state := crestState("own")
	card := strings.Repeat("1", 32)
	// 结晶只在**付不起本体费用**时才可用（10661110 本体 5 费 / 结晶 2 费），
	// 所以这里把 PP 调到 4：如果调到 5 及以上，crystallize 不再合法。
	own := state.Players["own"]
	own.PP, own.MaxPP = 4, 10
	state.Players["own"] = own
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: card, CardID: 10661110, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if r := s.SubmitAs(strings.Repeat("a", 32), "own", SimulatorCommand{Kind: "crystallize", Source: card}); r.Status != StatusCompleted {
		t.Fatal(r)
	}
	i := s.g.instances[card]
	if i.zone != "field" || i.card.CardType != "amulet" || i.countdown != 3 {
		t.Fatalf("crystallize did not produce an amulet with countdown 3: %+v", i)
	}
	// 结界护符在控制者回合开始递减：结束三个回合后吟唱归零并结算谢幕曲。
	for n := 0; n < 6; n++ {
		crestEndTurn(t, s)
	}
	if s.g.instances[card].zone != "graveyard" {
		t.Fatalf("the crystallized amulet should have expired into the graveyard, got %s", s.g.instances[card].zone)
	}
	summoned := 0
	for _, field := range s.g.player("own").field {
		if field.card.ID == 10661110 {
			summoned++
		}
	}
	if summoned != 1 {
		t.Fatalf("expected one summoned follower after the countdown expired, got %d", summoned)
	}
}

// 结晶与激奏同款：**只有支付不起本体费用时**才能结晶。这条直接决定训练侧看到的
// 合法动作列表——付得起本体时只应出现 `play`，绝不能把两个选项同时摆给模型。
func TestCrystallizeIsIllegalWhenTheBaseCostIsAffordable(t *testing.T) {
	pack := repeatCardPack(t)
	card := strings.Repeat("1", 32)
	base := 5 // cards/10006/10661110.wbo：本体 5 费 / 结晶 2 费
	for _, affordable := range []int{base, base + 3} {
		state := crestState("own")
		own := state.Players["own"]
		own.PP, own.MaxPP = affordable, 10
		state.Players["own"] = withInstance(own, "hand", ir.TestInstance{InstanceID: card, CardID: 10661110, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		if r := s.SubmitAs(strings.Repeat("a", 32), "own", SimulatorCommand{Kind: "crystallize", Source: card}); r.Status == StatusCompleted {
			t.Fatalf("pp=%d 时结晶不该合法（本体 %d 费它付得起），却结算成功了", affordable, base)
		}
		// 训练侧看到的就是这个列表：付得起本体 → 只有 play，没有 crystallize。
		kinds := map[string]bool{}
		for _, action := range s.LegalActionsFor("own") {
			if action.Source == card {
				kinds[action.Kind] = true
			}
		}
		if kinds["crystallize"] {
			t.Fatalf("pp=%d 的合法动作列表里不该出现 crystallize：%v", affordable, kinds)
		}
		if !kinds["play"] {
			t.Fatalf("pp=%d 时本体打出应该仍合法：%v", affordable, kinds)
		}
	}
}

// 反面：PP 低于本体费用、但够付结晶费用时，结晶必须出现且能结算。
func TestCrystallizeIsLegalBelowTheBaseCost(t *testing.T) {
	pack := repeatCardPack(t)
	card := strings.Repeat("1", 32)
	state := crestState("own")
	own := state.Players["own"]
	own.PP, own.MaxPP = 4, 10 // 本体 5 费付不起，结晶 2 费付得起
	state.Players["own"] = withInstance(own, "hand", ir.TestInstance{InstanceID: card, CardID: 10661110, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, action := range s.LegalActionsFor("own") {
		if action.Source == card && action.Kind == "crystallize" {
			found = true
		}
		if action.Source == card && action.Kind == "play" {
			t.Fatalf("PP 不够付本体费用，列表里不该出现 play：%+v", action)
		}
	}
	if !found {
		t.Fatal("PP 只够结晶时，crystallize 应该出现在合法动作里")
	}
	if r := s.SubmitAs(strings.Repeat("a", 32), "own", SimulatorCommand{Kind: "crystallize", Source: card}); r.Status != StatusCompleted {
		t.Fatal(r)
	}
}
