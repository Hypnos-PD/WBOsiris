package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/runner"
)

func revisionRoom(t *testing.T) (*Server, *match) {
	t.Helper()
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 77776001, CardType: "spell", Cost: 1},
		{ID: 77776002, CardType: "spell", PlayEffects: []ir.Effect{
			ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1}}},
		}},
	}}
	state := ir.State{Turn: ir.Turn{Active: "own", Number: 1}, Phase: "main", FirstPlayer: "own", Players: map[string]ir.PlayerState{}}
	for n, side := range []string{"own", "oppo"} {
		player := ir.PlayerState{Leader: ir.Leader{Life: 20, MaxLife: 20}, PP: 3, MaxPP: 3, Zones: map[string][]ir.TestInstance{}}
		for i := 1; i <= 8; i++ {
			zone := "deck"
			if i <= 3 {
				zone = "hand"
			}
			cardID := 77776001
			if i == 3 {
				cardID = 77776002
			}
			player.Zones[zone] = append(player.Zones[zone], ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n*100+i), CardID: cardID, DeclaredType: "spell"})
		}
		state.Players[side] = player
	}
	session, err := runner.NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	room := &match{session: session, joined: true, started: true, players: map[string]string{"host": "own", "guest": "oppo"}, mulligan: map[string]bool{}, mulliganSelection: map[string][]string{}}
	s := &Server{cards: pack, matches: map[string]*match{"revision": room}}
	recordReplay(room)
	return s, room
}

func revisionPost(t *testing.T, h http.Handler, token, suffix string, input command) response {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/matches/revision"+suffix, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	var out response
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &out) != nil {
		t.Fatalf("invalid response %d: %s", res.Code, res.Body.String())
	}
	return out
}

func TestDelayedEndTurnCannotEndTheNextRound(t *testing.T) {
	s, room := revisionRoom(t)
	h := s.Handler()
	zero := uint64(0)
	for n, token := range []string{"host", "guest"} {
		view, _ := room.session.View(map[string]string{"host": "own", "guest": "oppo"}[token])
		out := revisionPost(t, h, token, "", command{Kind: "end_turn", ActionID: fmt.Sprintf("%032x", n+10), ExpectedRevision: &view.Revision})
		if out.Result.Status != runner.StatusCompleted {
			t.Fatal(out.Result)
		}
	}
	before, _ := room.session.View("own")
	events := room.session.Events()
	frames := len(room.replay)
	if before.Turn.Number != 2 || before.Turn.Active != "own" {
		t.Fatal("fixture did not reach next own turn")
	}
	for _, expected := range []uint64{zero, before.Revision + 1} {
		out := revisionPost(t, h, "host", "", command{Kind: "end_turn", ActionID: strings.Repeat("a", 32), ExpectedRevision: &expected})
		after, _ := room.session.View("own")
		wantJSON, _ := json.Marshal(before)
		gotJSON, _ := json.Marshal(out.State)
		if out.Result.Status != runner.StatusRejected || out.Result.ErrorCode != "stale_state" || !reflect.DeepEqual(after, before) || !bytes.Equal(wantJSON, gotJSON) || len(room.replay) != frames || !reflect.DeepEqual(events, room.session.Events()) {
			t.Fatal("stale command changed state, events or replay", out.Result)
		}
	}
	out := revisionPost(t, h, "host", "", command{Kind: "end_turn", ActionID: strings.Repeat("b", 32), ExpectedRevision: &before.Revision})
	if out.Result.Status != runner.StatusCompleted || out.State.Turn.Active != "oppo" {
		t.Fatal("fresh command was not accepted", out.Result)
	}
}

func TestConcurrentCommandsForSameRevisionCommitOnlyOne(t *testing.T) {
	s, room := revisionRoom(t)
	h := s.Handler()
	zero := uint64(0)
	results := make(chan response, 2)
	var wg sync.WaitGroup
	for n := 1; n <= 2; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			results <- revisionPost(t, h, "host", "", command{Kind: "play", Source: fmt.Sprintf("%032x", n), ActionID: fmt.Sprintf("%032x", n+20), ExpectedRevision: &zero})
		}(n)
	}
	wg.Wait()
	close(results)
	completed, rejected := 0, 0
	for out := range results {
		switch {
		case out.Result.Status == runner.StatusCompleted:
			completed++
		case out.Result.Status == runner.StatusRejected && out.Result.ErrorCode == "stale_state":
			rejected++
		default:
			t.Fatal(out.Result)
		}
	}
	view, _ := room.session.View("own")
	if completed != 1 || rejected != 1 || view.Own.PP != 2 || len(view.Own.Graveyard) != 1 || view.Revision != 1 || len(room.replay) != 2 {
		t.Fatal("concurrent stale command spent resources or created a replay frame")
	}
}

func TestStaleChoiceKeepsPendingRequestAndCorrectSeatVisibility(t *testing.T) {
	s, room := revisionRoom(t)
	h := s.Handler()
	zero := uint64(0)
	paused := revisionPost(t, h, "host", "", command{Kind: "play", Source: fmt.Sprintf("%032x", 3), ActionID: strings.Repeat("a", 32), ExpectedRevision: &zero})
	if paused.Result.Status != runner.StatusSuspended {
		t.Fatal(paused.Result)
	}
	choice := paused.State.PendingChoice
	input := command{ActionID: choice.ActionID, RequestID: choice.RequestID, StateRevision: choice.StateRevision, SelectedOptionID: 1, ExpectedRevision: &zero}
	for _, token := range []string{"host", "guest"} {
		out := revisionPost(t, h, token, "", input)
		if out.Result.ErrorCode != "stale_state" || (out.State.PendingChoice != nil) != (token == "host") || out.State.Viewer != map[string]string{"host": "own", "guest": "oppo"}[token] || out.State.Oppo.Hand != nil {
			t.Fatal("stale response lost the pending request or exposed the other seat", out)
		}
	}
	if len(room.replay) != 2 || room.session.PendingChoice().RequestID != choice.RequestID {
		t.Fatal("stale resume changed the pending action")
	}
	input.ExpectedRevision = &paused.State.Revision
	if out := revisionPost(t, h, "host", "", input); out.Result.Status != runner.StatusCompleted {
		t.Fatal("fresh resume failed", out.Result)
	}
}

func TestMulliganRevisionGuardAcceptsZeroAndRejectsFutureState(t *testing.T) {
	s, room := revisionRoom(t)
	room.started = false
	h := s.Handler()
	future, zero := uint64(1), uint64(0)
	out := revisionPost(t, h, "host", "/mulligan", command{ExpectedRevision: &future})
	if out.Result.ErrorCode != "stale_state" || room.mulligan["own"] || len(room.replay) != 1 {
		t.Fatal("mulligan from wrong state was recorded")
	}
	out = revisionPost(t, h, "host", "/mulligan", command{ExpectedRevision: &zero})
	if !out.MulliganReady || !room.mulligan["own"] || out.State.Revision != 0 {
		t.Fatal("zero revision was treated as missing or invalid")
	}
}

func TestRevisionGuardDoesNotBypassAuthentication(t *testing.T) {
	s, _ := revisionRoom(t)
	req := httptest.NewRequest(http.MethodPost, "/api/matches/revision", strings.NewReader(`{"expectedRevision":99}`))
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized || strings.Contains(res.Body.String(), "state") {
		t.Fatal("revision mismatch leaked a private match")
	}
}
