package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/runner"
)

func TestFusionMaterialsAndEventsRemainPrivateInMatchAndReplay(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")})
	if err != nil {
		t.Fatal(err)
	}
	var state ir.State
	for _, scenario := range s.tests.Scenarios {
		if scenario.Name == "A fused core gains the resulting follower stats and intrinsic ability" {
			state = scenario.InitialState
		}
	}
	if state.Players == nil {
		t.Fatal("missing fusion fixture")
	}
	actor := state.Players["own"]
	actor.PP, actor.MaxPP = 3, 3
	state.Players["own"] = actor
	session, err := runner.NewSession(s.cards, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	room := &match{session: session, joined: true, started: true, players: map[string]string{"host": "own", "guest": "oppo"}}
	s.matches["fusion"] = room
	recordReplay(room)
	h := s.Handler()
	post := func(input command) response {
		t.Helper()
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/api/matches/fusion", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer host")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		var out response
		if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &out) != nil {
			t.Fatal("invalid command response", res.Code, res.Body.String())
		}
		return out
	}
	view, _ := session.View("own")
	sourceID, materialID := view.Own.Hand[0].InstanceID, view.Own.Hand[1].InstanceID
	step := post(command{Kind: "fusion", Source: sourceID, ActionID: strings.Repeat("1", 32)})
	if step.Result.Status != runner.StatusSuspended {
		t.Fatal(step.Result)
	}
	step = post(command{SelectedInstanceIDs: []string{materialID}})
	if step.Result.Status != runner.StatusCompleted {
		t.Fatal(step.Result)
	}
	fusion := step.State.Own.Hand[0].Fusion
	if fusion == nil || !fusion.Enabled || fusion.UsedThisTurn || fusion.TotalCost != 1 || fusion.DistinctKinds != 1 || len(fusion.Materials) != 1 || fusion.Materials[0].CardID != 90071220 {
		t.Fatal("owner missing material state", fusion)
	}
	for _, played := range []bool{false, true} {
		if played {
			step = post(command{Kind: "play", Source: sourceID, ActionID: strings.Repeat("2", 32)})
			if step.Result.Status != runner.StatusCompleted || step.State.Own.Field[0].Fusion == nil {
				t.Fatal("owner lost material state after playing", step.Result)
			}
		}
		for _, token := range []string{"host", "guest"} {
			status, body := perform(t, h, http.MethodGet, "/api/matches/fusion/replay", token)
			var replay replayResponse
			if status != http.StatusOK || json.Unmarshal(body, &replay) != nil {
				t.Fatal("invalid replay", status)
			}
			if len(replay.Events) < 2 || replay.Events[0].Kind != "card_fused" || replay.Events[1].Kind != "card_transformed" {
				t.Fatal("missing fusion or transform event")
			}
			for n, event := range replay.Events[:2] {
				if event.Sequence != uint64(n+1) {
					t.Fatal("redaction changed event indexing")
				}
				if token == "guest" && (event.Subject != nil || event.Target != nil || event.InstanceID != "" || event.CardID != 0) {
					t.Fatal("guest replay exposed hidden hand identity")
				}
				if token == "host" && event.Subject == nil {
					t.Fatal("owner replay lost event identity")
				}
			}
			for _, frame := range replay.Frames {
				if frame.EventCount > len(replay.Events) {
					t.Fatal("redaction invalidated replay frame boundaries")
				}
				if token == "guest" {
					if len(frame.State.Oppo.Hand) != 0 {
						t.Fatal("guest replay exposed hand")
					}
					for _, card := range frame.State.Oppo.Field {
						if card.Fusion != nil {
							t.Fatal("public card revealed private material history")
						}
					}
				}
			}
			if token == "guest" && bytes.Contains(body, []byte("90071220")) {
				t.Fatal("consumed material identity leaked through replay payload")
			}
		}
		_, body := perform(t, h, http.MethodGet, "/api/matches/fusion", "guest")
		var guest response
		if json.Unmarshal(body, &guest) != nil || guest.Events[0].Subject != nil || guest.Events[1].Target != nil || bytes.Contains(body, []byte("90071220")) {
			t.Fatal("live response exposed private fusion information")
		}
	}
}
