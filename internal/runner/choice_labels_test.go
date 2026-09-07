package runner

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestModeLabelsSurviveContinuationWithoutAliasing(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			state := testState()
			state.Turn.Active = side
			actor := state.Players[side]
			actor.PP, actor.MaxPP, actor.Leader.Life = 3, 3, 10
			source := strings.Repeat("1", 32)
			state.Players[side] = withInstance(actor, "hand", ir.TestInstance{InstanceID: source, CardID: 10132310, DeclaredType: "spell"})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: source})
			if step.Status != StatusSuspended {
				t.Fatal(step)
			}
			request := s.PendingChoice()
			if len(request.Candidates[0].Labels) != 5 || request.Candidates[0].Labels["eng"] != "Gain 4 earth sigils." {
				t.Fatal("mode metadata not propagated")
			}
			opponent, err := s.View(oppositeSide(side))
			if err != nil || opponent.PendingChoice != nil {
				t.Fatal("mode labels leaked to opponent")
			}
			step.Choice.Candidates[0].Labels["eng"] = "forged step"
			request.Candidates[0].Labels["eng"] = "forged view"
			savedView := s.Continuation()
			savedView.Pending.Request.Candidates[0].Labels["eng"] = "forged checkpoint"
			for _, invalid := range []ChoiceResponse{{}, {RequestID: request.RequestID, ActionID: request.ActionID, StateRevision: request.StateRevision, SelectedOptionID: 99}} {
				rejected := s.Resume(invalid)
				if rejected.Status != StatusRejected || rejected.Choice == nil {
					t.Fatal("invalid response did not retain choice", rejected)
				}
				rejected.Choice.Candidates[0].Labels["eng"] = "forged rejection"
				rejected.Choice.Candidates[0].OptionID = 99
			}
			if s.PendingChoice().Candidates[0].Labels["eng"] != "Gain 4 earth sigils." {
				t.Fatal("exported labels alias live state")
			}
			data, err := s.EncodeContinuation()
			if err != nil {
				t.Fatal(err)
			}
			again, err := s.EncodeContinuation()
			if err != nil || !bytes.Equal(data, again) {
				t.Fatal("labels break deterministic encoding")
			}
			for _, change := range []string{"replace", "remove", "extra", "version"} {
				saved, err := DecodeContinuation(data)
				if err != nil {
					t.Fatal(err)
				}
				switch change {
				case "replace":
					saved.Pending.Request.Candidates[0].Labels["eng"] = "forged"
				case "remove":
					delete(saved.Pending.Request.Candidates[0].Labels, "chs")
				case "extra":
					saved.Pending.Request.Candidates[0].Labels["fra"] = "forged"
				case "version":
					saved.Version = "0.13.0"
				}
				if _, err := RestoreSession(pack, saved); err == nil {
					t.Fatal("accepted forged checkpoint", change)
				}
			}
			saved, err := DecodeContinuation(data)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := RestoreSession(pack, saved)
			if err != nil {
				t.Fatal(err)
			}
			saved.Pending.Request.Candidates[0].Labels["eng"] = "mutated after restore"
			if !reflect.DeepEqual(s.PendingChoice(), restored.PendingChoice()) {
				t.Fatal("restored request changed or aliases caller input")
			}
			r := s.PendingChoice()
			response := ChoiceResponse{RequestID: r.RequestID, ActionID: r.ActionID, StateRevision: r.StateRevision, SelectedOptionID: 2}
			for _, run := range []*Session{s, restored} {
				if result := run.Resume(response); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				if run.g.player(side).leaderLife != 14 || run.g.rng.Consumed() != 0 {
					t.Fatal("labels changed mode execution")
				}
			}
			if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
				t.Fatal("restored labeled mode diverged")
			}
		})
	}
}
