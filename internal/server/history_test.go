package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/runner"
)

func TestDestroyedHistoryAndReplayDoNotLeakLaterHandTransformation(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			other := oppositeMatchSide(side)
			source, victim := strings.Repeat("1", 32), strings.Repeat("2", 32)
			pack := &ir.CardPack{Cards: []ir.Card{
				{ID: 77778001, CardType: "spell", PlayEffects: []ir.Effect{
					ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("a", 32)}, Kind: "destroy", Output: "destroyed", Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}},
					ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "return", Target: ir.BindingRef{Kind: "binding", Name: "destroyed"}, Destination: "hand"},
					ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("c", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}},
					ir.CardEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "transform", Target: ir.BindingRef{Kind: "binding", Name: "destroyed"}, CardID: 77778003, PreserveInstanceID: true, PreserveMaterials: true},
					ir.CardEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "reanimate", Owner: "own", MaxCost: 4, TieBreak: "random", Output: "summoned"},
				}},
				{ID: 77778002, CardType: "follower", Cost: 4, Stats: &ir.Stats{Attack: 3, Life: 3}},
				{ID: 77778003, CardType: "follower", Cost: 5, Stats: &ir.Stats{Attack: 5, Life: 5}},
			}}
			state := ir.State{Turn: ir.Turn{Active: side, Number: 7}, Phase: "main", Players: map[string]ir.PlayerState{
				side: {Leader: ir.Leader{Life: 20, MaxLife: 20}, PP: 7, MaxPP: 7, Zones: map[string][]ir.TestInstance{
					"hand":  {{InstanceID: source, CardID: 77778001, DeclaredType: "spell"}},
					"field": {{InstanceID: victim, CardID: 77778002, DeclaredType: "follower"}},
				}},
				other: {Leader: ir.Leader{Life: 20, MaxLife: 20}},
			}}
			session, err := runner.NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			room := &match{session: session, joined: true, started: true, players: map[string]string{"actor": side, "observer": other}}
			server := &Server{cards: pack, matches: map[string]*match{"history": room}}
			recordReplay(room)
			handler := server.Handler()
			post := func(cmd command) response {
				t.Helper()
				body, _ := json.Marshal(cmd)
				req := httptest.NewRequest(http.MethodPost, "/api/matches/history", bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer actor")
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)
				var out response
				if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &out) != nil {
					t.Fatal(res.Code, res.Body.String())
				}
				return out
			}
			out := post(command{Kind: "play", ActionID: strings.Repeat("f", 32), Source: source})
			if out.Result.Status != runner.StatusSuspended || len(out.State.Own.Hand) != 1 || out.State.Own.Hand[0].CardID != 77778002 {
				t.Fatal("destroy result did not retain its live binding", out)
			}
			pending := out.State.PendingChoice
			out = post(command{ActionID: pending.ActionID, RequestID: pending.RequestID, StateRevision: pending.StateRevision, SelectedOptionID: 1})
			if out.Result.Status != runner.StatusCompleted || out.State.Own.Hand[0].CardID != 77778003 || len(out.State.Own.Field) != 1 || out.State.Own.Field[0].CardID != 77778002 {
				t.Fatal("transformation or reanimation failed", out)
			}
			for _, token := range []string{"actor", "observer"} {
				status, body := perform(t, handler, http.MethodGet, "/api/matches/history/replay", token)
				var replay replayResponse
				if status != http.StatusOK || json.Unmarshal(body, &replay) != nil || len(replay.Frames) != 3 {
					t.Fatal("invalid history replay", status, string(body))
				}
				if token == "observer" && bytes.Contains(body, []byte("77778003")) {
					t.Fatal("history or replay leaked a hidden transformed identity")
				}
				for _, frame := range replay.Frames[1:] {
					p := frame.State.Own
					if token == "observer" {
						p = frame.State.Oppo
					}
					if len(p.Destroyed) != 1 || p.Destroyed[0].CardID != 77778002 {
						t.Fatal("replay changed the destroyed identity")
					}
				}
				for _, event := range replay.Events {
					if event.Kind == "destroyed" && (event.Subject == nil || event.Subject.CardID != 77778002) {
						t.Fatal("death event did not retain its historical identity")
					}
				}
			}
		})
	}
}
