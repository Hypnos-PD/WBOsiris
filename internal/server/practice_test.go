package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"wbo/internal/engine/ai"
	"wbo/internal/engine/runner"
)

func postMatch(t *testing.T, handler http.Handler, path, token string, payload any) (int, response) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	var out response
	if res.Code == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return res.Code, out
}

func getMatch(t *testing.T, handler http.Handler, path, token string) (int, response) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	var out response
	if res.Code == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
	}
	return res.Code, out
}

// 练习模式：创建即开局，人类换牌后 AI 自己会走完它的回合；
// 人类一直按合法动作推进，最终必须能走到分出胜负，且中途没有 AI 故障。
func TestPracticeMatchAgainstBotCompletes(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")})
	if err != nil {
		t.Fatal(err)
	}
	handler := s.Handler()
	status, created := postMatch(t, handler, "/api/matches", "", createRequest{Mode: "bot", BotPolicy: "greedy"})
	if status != http.StatusOK {
		t.Fatalf("create practice match status=%d", status)
	}
	if !created.Bot || created.Waiting || created.Side != "own" || created.PlayerToken == "" {
		t.Fatalf("practice match did not start immediately: %#v", created)
	}
	if created.JoinCode != "" {
		t.Fatal("practice match exposed an invitation code")
	}
	if created.MatchPhase != "mulligan" {
		t.Fatalf("practice match phase = %q, want mulligan", created.MatchPhase)
	}
	status, afterMulligan := postMatch(t, handler, "/api/matches/"+created.MatchID+"/mulligan", created.PlayerToken, command{})
	if status != http.StatusOK || afterMulligan.MatchPhase != "main" {
		t.Fatalf("mulligan status=%d phase=%q", status, afterMulligan.MatchPhase)
	}
	token := created.PlayerToken
	me := ai.NewDriver("own", &ai.Greedy{}, ai.Limits{})
	state := afterMulligan
	for step := 0; step < 400; step++ {
		if state.BotError != "" {
			t.Fatalf("bot faulted: %s", state.BotError)
		}
		if state.State.GameOver {
			break
		}
		if state.State.PendingChoice != nil {
			answer := (&ai.Greedy{}).Answer(state.State, *state.State.PendingChoice)
			status, state = postMatch(t, handler, "/api/matches/"+created.MatchID, token, command{
				RequestID: answer.RequestID, ActionID: answer.ActionID, StateRevision: answer.StateRevision,
				SelectedInstanceIDs: answer.SelectedInstanceIDs, SelectedLeaderSides: answer.SelectedLeaderSides,
				SelectedOptionID: answer.SelectedOptionID,
			})
			if status != http.StatusOK {
				t.Fatalf("choice status=%d", status)
			}
			continue
		}
		actions := state.LegalActions
		if len(actions) == 0 {
			// 轮到 AI 或对局已结束：重新读一次状态。
			status, state = getMatch(t, handler, "/api/matches/"+created.MatchID, token)
			if status != http.StatusOK {
				t.Fatalf("poll status=%d", status)
			}
			continue
		}
		action, ok := (&ai.Greedy{}).Choose(state.State, actions)
		if !ok {
			t.Fatal("greedy declined to act")
		}
		wire := command{Kind: action.Kind, Source: action.Source, Defender: action.Defender}
		if action.Kind == "attack_leader" || action.Kind == "attack_entity" {
			wire.Kind = "attack"
		}
		if action.Kind == "attack_leader" {
			wire.Defender = ""
		}
		status, state = postMatch(t, handler, "/api/matches/"+created.MatchID, token, wire)
		if status != http.StatusOK {
			t.Fatalf("action %s status=%d", action.Kind, status)
		}
		if state.Result != nil && state.Result.Status == runner.StatusFault {
			t.Fatalf("engine faulted: %#v", state.Result)
		}
	}
	if !state.State.GameOver {
		t.Fatalf("practice match did not finish: %#v", state.State.Turn)
	}
	if state.State.Winner != "own" && state.State.Winner != "oppo" {
		t.Fatalf("practice match produced no winner: %#v", state.State)
	}
	_ = me
}
