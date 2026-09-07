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

func TestMixedTargetWireProtocolForBothSeats(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			other := oppositeMatchSide(side)
			source, follower := strings.Repeat("1", 32), strings.Repeat("2", 32)
			pack := &ir.CardPack{Cards: []ir.Card{
				{ID: 77776001, CardType: "spell", Cost: 7, PlayEffects: []ir.Effect{
					ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("a", 32)}, Kind: "require", Policy: "required", Binding: "target", Source: ir.CharacterSetRef{Kind: "characters", Side: "oppo"}},
					ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "damage", Target: ir.BindingRef{Kind: "binding", Name: "target"}, Amount: 5},
				}},
				{ID: 77776002, CardType: "follower", Stats: &ir.Stats{Attack: 2, Life: 2}},
			}}
			state := ir.State{Turn: ir.Turn{Active: side, Number: 7}, Phase: "main", Players: map[string]ir.PlayerState{
				side:  {Leader: ir.Leader{Life: 20, MaxLife: 20}, PP: 7, MaxPP: 7, Zones: map[string][]ir.TestInstance{"hand": {{InstanceID: source, CardID: 77776001, DeclaredType: "spell"}}}},
				other: {Leader: ir.Leader{Life: 20, MaxLife: 20}, Zones: map[string][]ir.TestInstance{"field": {{InstanceID: follower, CardID: 77776002, DeclaredType: "follower"}}}},
			}}
			session, err := runner.NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			room := &match{session: session, joined: true, started: true, players: map[string]string{"actor": side, "observer": other}}
			server := &Server{cards: pack, matches: map[string]*match{"mixed": room}}
			recordReplay(room)
			handler := server.Handler()
			post := func(cmd command) response {
				t.Helper()
				body, _ := json.Marshal(cmd)
				req := httptest.NewRequest(http.MethodPost, "/api/matches/mixed", bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer actor")
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)
				var out response
				if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &out) != nil {
					t.Fatal(res.Code, res.Body.String())
				}
				return out
			}
			out := post(command{Kind: "play", ActionID: strings.Repeat("c", 32), Source: source})
			pending := out.State.PendingChoice
			if out.Result.Status != runner.StatusSuspended || pending == nil || len(pending.Candidates) != 2 || pending.Candidates[1].LeaderSide != other {
				t.Fatal(out)
			}
			status, data := perform(t, handler, http.MethodGet, "/api/matches/mixed", "observer")
			var observer response
			if status != http.StatusOK || json.Unmarshal(data, &observer) != nil || observer.State.PendingChoice != nil {
				t.Fatal("choice leaked", string(data))
			}
			out = post(command{ActionID: pending.ActionID, RequestID: pending.RequestID, StateRevision: pending.StateRevision, SelectedLeaderSides: []string{other}})
			if out.Result.Status != runner.StatusCompleted || out.State.Oppo.LeaderLife != 15 || out.State.Own.PP != 0 || len(out.State.Oppo.Field) != 1 {
				t.Fatal("leader response failed", out)
			}
			status, data = perform(t, handler, http.MethodGet, "/api/matches/mixed/replay", "actor")
			if status != http.StatusOK || !bytes.Contains(data, []byte(`"kind":"leader"`)) || !bytes.Contains(data, []byte(`"leaderSide":"`+other+`"`)) {
				t.Fatal("replay lost candidate", string(data))
			}
		})
	}
}
