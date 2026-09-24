package runner

import (
	"strings"
	"testing"

	"wbo/internal/engine/ir"
)

// 「【模式】选择 N 个能力发动」：多选请求保存两个选项，恢复后按选项编号顺序结算。
func TestMultiSelectModeSurvivesContinuation(t *testing.T) {
	modeID := strings.Repeat("1", 32)
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 70000001, CardType: "spell", Cost: 2, PlayEffects: []ir.Effect{
		ir.ModeEffect{NodeBase: ir.NodeBase{ID: modeID}, Kind: "mode", Count: 2, Options: []ir.ModeOption{
			{ID: 1, Body: []ir.Effect{ir.AdjustEffect{Kind: "adjust_resource", Owner: "own", Resource: "pp", Delta: 1}}},
			{ID: 2, Body: []ir.Effect{ir.AdjustEffect{Kind: "adjust_resource", Owner: "own", Resource: "combo", Delta: 5}}},
			{ID: 3, Body: []ir.Effect{ir.AdjustEffect{Kind: "adjust_resource", Owner: "own", Resource: "shadows", Delta: 1}}},
		}},
	}}}}
	sourceID := strings.Repeat("2", 32)
	state := testState()
	own := state.Players["own"]
	own.PP, own.MaxPP = 2, 2
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 70000001, DeclaredType: "spell"})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID})
	if step.Status != StatusSuspended || step.Choice == nil || step.Choice.Kind != "mode" || step.Choice.MinSelections != 2 || step.Choice.MaxSelections != 2 {
		t.Fatalf("multi-select mode request missing: %#v", step)
	}
	data, err := session.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	continuation, err := DecodeContinuation(data)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(pack, continuation)
	if err != nil {
		t.Fatal(err)
	}
	result := restored.Resume(ChoiceResponse{
		RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision,
		SelectedOptionIDs: []int{3, 1},
	})
	if result.Status != StatusCompleted {
		t.Fatalf("multi-select resume failed: %#v", result)
	}
	// 打出法术本身进墓场加 1 点墓场资源；选项 3 再加 1；选项 1 回复 1 点能量点。
	if restored.g.own.shadows != 2 || restored.g.own.pp != 1 || restored.g.own.combo != 1 {
		t.Fatalf("selected options did not resolve: shadows=%d pp=%d combo=%d", restored.g.own.shadows, restored.g.own.pp, restored.g.own.combo)
	}
}

func TestMultiSelectModeRejectsDuplicates(t *testing.T) {
	modeID := strings.Repeat("1", 32)
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 70000001, CardType: "spell", Cost: 1, PlayEffects: []ir.Effect{
		ir.ModeEffect{NodeBase: ir.NodeBase{ID: modeID}, Kind: "mode", Count: 2, Options: []ir.ModeOption{
			{ID: 1, Body: []ir.Effect{ir.AdjustEffect{Kind: "adjust_resource", Owner: "own", Resource: "pp", Delta: 1}}},
			{ID: 2, Body: []ir.Effect{ir.AdjustEffect{Kind: "adjust_resource", Owner: "own", Resource: "shadows", Delta: 1}}},
		}},
	}}}}
	sourceID := strings.Repeat("2", 32)
	state := testState()
	own := state.Players["own"]
	own.PP, own.MaxPP = 1, 1
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 70000001, DeclaredType: "spell"})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID})
	if step.Status != StatusSuspended {
		t.Fatalf("expected a mode request: %#v", step)
	}
	choice := session.PendingChoice()
	result := session.Resume(ChoiceResponse{
		RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision,
		SelectedOptionIDs: []int{1, 1},
	})
	if result.Status != StatusRejected || result.ErrorCode != "invalid_option" {
		t.Fatalf("duplicate options were accepted: %#v", result)
	}
}
