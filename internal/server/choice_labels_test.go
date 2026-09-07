package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/runner"
)

func TestTurnBoundaryChoiceLabelsStayWithTheirController(t *testing.T) {
	for _, endingSide := range []string{"own", "oppo"} {
		t.Run(endingSide, func(t *testing.T) {
			chooser := oppositeMatchSide(endingSide)
			pack := &ir.CardPack{Cards: []ir.Card{
				{ID: 77775001, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{{
					ID: strings.Repeat("a", 32), Trigger: ir.EventTrigger{Kind: "event", Event: "turn_started", Side: "own"}, Body: []ir.Effect{
						ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "mode", Options: []ir.ModeOption{
							{ID: 7, Labels: map[string]string{"chs": "private mode label", "eng": "private translated label"}},
							{ID: 2, Labels: map[string]string{"chs": "second choice"}},
						}},
					},
				}}},
				{ID: 77775002, CardType: "spell"},
			}}
			state := ir.State{Turn: ir.Turn{Active: endingSide, Number: 1}, Phase: "main", Players: map[string]ir.PlayerState{}}
			for seat, side := range []string{"own", "oppo"} {
				player := ir.PlayerState{Leader: ir.Leader{Life: 20, MaxLife: 20}, PP: 1, MaxPP: 1, Zones: map[string][]ir.TestInstance{
					"deck": {{InstanceID: fmt.Sprintf("%032x", 10+seat), CardID: 77775002, DeclaredType: "spell"}},
				}}
				if side == chooser {
					player.Zones["field"] = []ir.TestInstance{{InstanceID: strings.Repeat("1", 32), CardID: 77775001, DeclaredType: "follower"}}
				}
				state.Players[side] = player
			}
			session, err := runner.NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			room := &match{session: session, joined: true, started: true, players: map[string]string{"host": endingSide, "guest": chooser}}
			s := &Server{cards: pack, matches: map[string]*match{"labels": room}}
			recordReplay(room)
			h := s.Handler()
			body, _ := json.Marshal(command{Kind: "end_turn", ActionID: strings.Repeat("c", 32)})
			req := httptest.NewRequest(http.MethodPost, "/api/matches/labels", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer host")
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			var out response
			if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &out) != nil || out.Result == nil || out.Result.Status != runner.StatusSuspended {
				t.Fatal("turn did not suspend", res.Code, res.Body.String())
			}
			if out.Result.Choice != nil || out.State.PendingChoice != nil || strings.Contains(res.Body.String(), "private mode label") {
				t.Fatal("turn response disclosed opponent choice", res.Body.String())
			}
			for _, token := range []string{"host", "guest"} {
				for _, suffix := range []string{"", "/replay"} {
					status, data := perform(t, h, http.MethodGet, "/api/matches/labels"+suffix, token)
					if status != http.StatusOK || bytes.Contains(data, []byte("private mode label")) != (token == "guest") {
						t.Fatalf("wrong visibility token=%s suffix=%s status=%d: %s", token, suffix, status, data)
					}
				}
			}
			pending := session.PendingChoice()
			body, _ = json.Marshal(command{ActionID: pending.ActionID, RequestID: pending.RequestID, StateRevision: pending.StateRevision, SelectedOptionID: 7})
			req = httptest.NewRequest(http.MethodPost, "/api/matches/labels", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer guest")
			res = httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &out) != nil || out.Result.Status != runner.StatusCompleted {
				t.Fatal("chooser could not finish turn", res.Code, res.Body.String())
			}
		})
	}
}
