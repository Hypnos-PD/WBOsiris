package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/runner"
)

func serverRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func perform(t *testing.T, handler http.Handler, method, path, token string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res.Code, res.Body.Bytes()
}

func TestMatchRoomCreateJoinProtocol(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")})
	if err != nil {
		t.Fatal(err)
	}
	handler := s.Handler()
	status, body := perform(t, handler, http.MethodPost, "/api/matches", "")
	if status != http.StatusOK {
		t.Fatalf("create status=%d", status)
	}
	var owner response
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&owner); err != nil {
		t.Fatal(err)
	}
	if owner.MatchID == "" || owner.PlayerToken == "" || owner.JoinCode == "" || owner.Side != "own" || !owner.Waiting {
		t.Fatalf("invalid owner match response: %#v", owner)
	}
	status, body = perform(t, handler, http.MethodGet, "/api/matches", "")
	if status != http.StatusOK {
		t.Fatalf("list status=%d", status)
	}
	// Room presence is time-sensitive and must not be served from a stale cache.
	var rooms []struct {
		ID      string `json:"id"`
		Waiting bool   `json:"waiting"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&rooms); err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 1 || rooms[0].ID != owner.MatchID || !rooms[0].Waiting {
		t.Fatalf("invalid room listing: %#v", rooms)
	}

	status, _ = perform(t, handler, http.MethodPost, "/api/matches/"+owner.MatchID+"/join?code=wrong", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong code status=%d", status)
	}

	status, body = perform(t, handler, http.MethodPost, "/api/matches/"+owner.MatchID+"/join?code="+owner.JoinCode, "")
	if status != http.StatusOK {
		t.Fatalf("join status=%d", status)
	}
	var opponent response
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&opponent); err != nil {
		t.Fatal(err)
	}
	if opponent.MatchID != owner.MatchID || opponent.PlayerToken == "" || opponent.Side != "oppo" || opponent.Waiting {
		t.Fatalf("invalid opponent match response: %#v", opponent)
	}
	status, body = perform(t, handler, http.MethodGet, "/api/matches", "")
	if status != http.StatusOK {
		t.Fatalf("list-after-join status=%d", status)
	}
	rooms = nil
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&rooms); err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 1 || rooms[0].Waiting {
		t.Fatalf("room listing did not update after join: %#v", rooms)
	}
}

func TestMatchCreateValidatesAndUsesSubmittedDeck(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")})
	if err != nil {
		t.Fatal(err)
	}
	deck := make([]int, 40)
	ids := make([]int, 14)
	for i := range ids {
		ids[i] = s.cards.Cards[i].ID
	}
	for i := range deck {
		deck[i] = ids[i%len(ids)]
	}
	body, _ := json.Marshal(createRequest{Deck: deck})
	req := httptest.NewRequest(http.MethodPost, "/api/matches", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("valid deck create status=%d body=%s", res.Code, res.Body.String())
	}
	var created response
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.State.Own.HandCount != 4 || created.State.Own.DeckCount != 36 {
		t.Fatalf("submitted deck was not installed: %#v", created.State.Own)
	}
	for name, bad := range map[string][]int{
		"short":    deck[:39],
		"too_many": append(append([]int(nil), deck...), 10001110),
		"unknown":  func() []int { x := append([]int(nil), deck...); x[0] = 99999999; return x }(),
	} {
		t.Run(name, func(t *testing.T) {
			data, _ := json.Marshal(createRequest{Deck: bad})
			req := httptest.NewRequest(http.MethodPost, "/api/matches", bytes.NewReader(data))
			res := httptest.NewRecorder()
			s.Handler().ServeHTTP(res, req)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("invalid deck status=%d", res.Code)
			}
		})
	}
}

func TestCORSPreflight(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodOptions, "/api/matches", nil)
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("preflight status=%d", res.Code)
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("allow-origin=%q", got)
	}
	if got := res.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") {
		t.Fatalf("allow-headers=%q", got)
	}
}

func TestMatchListIsSorted(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")})
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	perform(t, h, http.MethodPost, "/api/matches", "")
	perform(t, h, http.MethodPost, "/api/matches", "")
	req := httptest.NewRequest(http.MethodGet, "/api/matches", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("list status=%d", res.Code)
	}
	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control=%q", got)
	}
	body := res.Body.Bytes()
	var rooms []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&rooms); err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 2 || rooms[0].ID >= rooms[1].ID {
		t.Fatalf("rooms are not sorted: %#v", rooms)
	}
}

func TestSessionViewsSerializeUnderConcurrentRequests(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(createRequest{Scenario: "最大能量已满时保持上限并抽牌"})
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(body))
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("session create status=%d body=%s", res.Code, res.Body.String())
	}
	var created response
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errors := make(chan int, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			request := httptest.NewRequest(http.MethodGet, "/api/sessions/"+created.SessionID, nil)
			response := httptest.NewRecorder()
			s.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				errors <- response.Code
			}
		}()
	}
	wg.Wait()
	close(errors)
	for status := range errors {
		t.Fatalf("concurrent session view status=%d", status)
	}
}

func TestMatchReplayRequiresTokenAndReturnsEventLog(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")})
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	status, body := perform(t, h, http.MethodPost, "/api/matches", "")
	if status != http.StatusOK {
		t.Fatalf("create status=%d", status)
	}
	var created response
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	status, _ = perform(t, h, http.MethodGet, "/api/matches/"+created.MatchID+"/replay", "wrong")
	if status != http.StatusUnauthorized {
		t.Fatalf("replay without owner token status=%d", status)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/matches/"+created.MatchID+"/replay", nil)
	req.Header.Set("Authorization", "Bearer "+created.PlayerToken)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK || res.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("replay status=%d cache=%q", res.Code, res.Header().Get("Cache-Control"))
	}
	var replay replayResponse
	if err := json.Unmarshal(res.Body.Bytes(), &replay); err != nil {
		t.Fatal(err)
	}
	if replay.MatchID != created.MatchID || replay.State.Own.HandCount != 4 || len(replay.Frames) != 1 || replay.Frames[0].State.Own.HandCount != 4 {
		t.Fatalf("invalid replay response: %#v", replay)
	}
}

func TestReplayFrameEventsAndPrivateViewsAcrossSuspendedAction(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")})
	if err != nil {
		t.Fatal(err)
	}
	var state ir.State
	for _, scenario := range s.tests.Scenarios {
		if scenario.Name == "Sylvia super evolution destroys two selected followers together" {
			state = scenario.InitialState
			break
		}
	}
	if state.Players == nil {
		t.Fatal("missing replay fixture")
	}
	for _, side := range []string{"own", "oppo"} {
		player := state.Players[side]
		id := strings.Repeat("a", 32)
		if side == "oppo" {
			id = strings.Repeat("b", 32)
		}
		player.Zones["hand"] = []ir.TestInstance{{InstanceID: id, CardID: 10001110, DeclaredType: "follower"}}
		state.Players[side] = player
	}
	session, err := runner.NewSession(s.cards, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	room := &match{session: session, joined: true, started: true, players: map[string]string{"host": "own", "guest": "oppo"}}
	s.matches["history"] = room
	recordReplay(room)
	h := s.Handler()
	read := func(token string) replayResponse {
		t.Helper()
		status, body := perform(t, h, http.MethodGet, "/api/matches/history/replay", token)
		if status != http.StatusOK {
			t.Fatalf("replay status %d: %s", status, body)
		}
		var replay replayResponse
		if err := json.Unmarshal(body, &replay); err != nil {
			t.Fatal(err)
		}
		return replay
	}
	post := func(input command) response {
		t.Helper()
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/api/matches/history", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer host")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("command status %d: %s", res.Code, res.Body.String())
		}
		var result response
		if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	initial, _ := json.Marshal(read("host").Frames[0])
	view, _ := session.View("own")
	paused := post(command{Kind: "superevolve", Source: view.Own.Field[0].InstanceID, ActionID: strings.Repeat("c", 32)})
	if paused.Result.Status != runner.StatusSuspended {
		t.Fatalf("not suspended: %#v", paused.Result)
	}
	host, guest := read("host"), read("guest")
	if len(host.Frames) != 2 || len(guest.Frames) != 2 {
		t.Fatal("missing suspended frame")
	}
	if host.Frames[1].State.PendingChoice == nil || guest.Frames[1].State.PendingChoice != nil {
		t.Fatal("incorrect choice visibility")
	}
	suspended, _ := json.Marshal(host.Frames[1])
	choice := paused.State.PendingChoice
	input := command{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision}
	if rejected := post(input); rejected.Result.Status != runner.StatusRejected {
		t.Fatal("expected invalid empty selection")
	}
	if len(read("host").Frames) != 2 {
		t.Fatal("rejected command added a frame")
	}
	for _, candidate := range choice.Candidates[:2] {
		input.SelectedInstanceIDs = append(input.SelectedInstanceIDs, candidate.InstanceID)
	}
	if completed := post(input); completed.Result.Status != runner.StatusCompleted {
		t.Fatalf("not completed: %#v", completed.Result)
	}
	for _, token := range []string{"host", "guest"} {
		replay := read(token)
		if len(replay.Frames) != 3 {
			t.Fatal("missing completion frame")
		}
		previous := 0
		for i, frame := range replay.Frames {
			if frame.EventCount < previous || frame.EventCount > len(replay.Events) {
				t.Fatal("invalid event boundary")
			}
			if i > 0 && frame.Revision <= replay.Frames[i-1].Revision {
				t.Fatal("unordered frames")
			}
			if len(frame.State.Oppo.Hand) != 0 || len(frame.State.Own.Hand) != 1 {
				t.Fatal("private hand visibility changed")
			}
			previous = frame.EventCount
		}
		if previous != len(replay.Events) {
			t.Fatal("events missing from final frame")
		}
		last := replay.Frames[2]
		if last.State.PendingChoice != nil {
			t.Fatal("final frame retains choice")
		}
		destroyed := 0
		for _, event := range replay.Events[replay.Frames[1].EventCount:last.EventCount] {
			if event.Kind == "destroyed" {
				destroyed++
			}
		}
		if destroyed != 2 {
			t.Fatalf("completion contains %d destructions", destroyed)
		}
		if token == "host" {
			firstNow, _ := json.Marshal(replay.Frames[0])
			pausedNow, _ := json.Marshal(replay.Frames[1])
			if !bytes.Equal(initial, firstNow) || !bytes.Equal(suspended, pausedNow) {
				t.Fatal("historical snapshot mutated")
			}
		} else if last.State.Viewer != "oppo" || last.State.Turn.Active != "oppo" {
			t.Fatal("guest perspective was not inverted")
		}
	}
}
