package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"wbo/internal/runner"
)

func setupServer(t *testing.T) *Server {
	t.Helper()
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards")})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func setupPost(t *testing.T, h http.Handler, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func setupResponse(t *testing.T, res *httptest.ResponseRecorder) response {
	t.Helper()
	var out response
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &out) != nil {
		t.Fatalf("unexpected response %d: %s", res.Code, res.Body.String())
	}
	return out
}

func TestMatchSetupEntropyUsesIndependentFirstPlayerBit(t *testing.T) {
	for _, bit := range []byte{0, 1, 254, 255} {
		data := []byte{8, 7, 6, 5, 4, 3, 2, 1, bit}
		seed, first, err := readMatchSetup(bytes.NewReader(data))
		if err != nil || seed != 0x0102030405060708 || (first == "own") != (bit&1 == 0) {
			t.Fatal(seed, first, err)
		}
	}
	if _, _, err := readMatchSetup(bytes.NewReader(make([]byte, 8))); err == nil {
		t.Fatal("incomplete entropy silently selected a default opening")
	}
}

func TestWaitingRoomHasNoOpeningAndEntropyFailureLeavesItJoinable(t *testing.T) {
	s := setupServer(t)
	calls := 0
	s.matchSetup = func() (uint64, string, error) { calls++; return 0, "", errors.New("unavailable") }
	h := s.Handler()
	host := setupResponse(t, setupPost(t, h, "/api/matches", "", createRequest{}))
	room := s.matches[host.MatchID]
	path := "/api/matches/" + host.MatchID
	if calls != 0 || room.session != nil || len(room.replay) != 0 || host.MatchPhase != "waiting" || host.State.FirstPlayer != "" || len(host.State.Own.Hand) != 0 || host.State.Own.DeckCount != 40 || len(host.Events) != 0 {
		t.Fatal("waiting room revealed a generated opening")
	}
	join := path + "/join?code=" + host.JoinCode
	if res := setupPost(t, h, join, "", map[string]any{"deck": []int{}}); res.Code != http.StatusBadRequest || calls != 0 {
		t.Fatal("invalid deck consumed opening randomness")
	}
	if res := setupPost(t, h, join, "", createRequest{}); res.Code != http.StatusInternalServerError || calls != 1 || room.session != nil || room.joined || len(room.players) != 1 {
		t.Fatal("entropy failure partially joined a room")
	}
	s.matchSetup = func() (uint64, string, error) { calls++; return 123, "oppo", nil }
	guest := setupResponse(t, setupPost(t, h, join, "", createRequest{}))
	if !room.joined || calls != 2 || guest.State.FirstPlayer != "own" || guest.State.Own.HandCount != 4 || guest.State.Oppo.Hand != nil {
		t.Fatal("valid retry did not start the selected opening")
	}
	_, data := perform(t, h, http.MethodGet, path+"/replay", guest.PlayerToken)
	var replay replayResponse
	if json.Unmarshal(data, &replay) != nil || replay.State.Phase != "mulligan" || replay.Frames[0].State.Phase != "mulligan" {
		t.Fatal("replay reported a main phase before mulligan confirmation")
	}
	if res := setupPost(t, h, join, "", createRequest{}); res.Code != http.StatusConflict || calls != 2 {
		t.Fatal("duplicate join rerolled the opening")
	}
}

func TestMatchMulliganArrivalOrderDoesNotChangeEitherOpening(t *testing.T) {
	s := setupServer(t)
	h := s.Handler()
	for _, first := range []string{"own", "oppo"} {
		s.matchSetup = func() (uint64, string, error) { return 12345, first, nil }
		var expected runner.StateView
		for pass, order := range [][]string{{"own", "oppo"}, {"oppo", "own"}} {
			host := setupResponse(t, setupPost(t, h, "/api/matches", "", createRequest{}))
			path := "/api/matches/" + host.MatchID
			guest := setupResponse(t, setupPost(t, h, path+"/join?code="+host.JoinCode, "", createRequest{}))
			room := s.matches[host.MatchID]
			tokens := map[string]string{"own": host.PlayerToken, "oppo": guest.PlayerToken}
			selections := map[string][]string{}
			for _, side := range []string{"own", "oppo"} {
				view, _ := room.session.View(side)
				selections[side] = []string{view.Own.Hand[0].InstanceID}
			}
			before, _ := room.session.View("own")
			for n, side := range order {
				out := setupResponse(t, setupPost(t, h, path+"/mulligan", tokens[side], command{SelectedInstanceIDs: selections[side]}))
				if n == 0 {
					after, _ := room.session.View("own")
					if room.started || !reflect.DeepEqual(before, after) || !out.MulliganReady {
						t.Fatal("first confirmation changed either hand before both were ready")
					}
				}
			}
			view, _ := room.session.View("own")
			if !room.started || view.FirstPlayer != first || view.Turn.Active != first || view.Turn.Number != 1 {
				t.Fatal("match started with the wrong seat")
			}
			if pass == 0 {
				expected = view
			} else if !reflect.DeepEqual(view, expected) {
				t.Fatal("HTTP arrival order changed deterministic mulligans")
			}
			for _, side := range []string{"own", "oppo"} {
				status, data := perform(t, h, http.MethodGet, path, tokens[side])
				var read response
				if status != http.StatusOK || json.Unmarshal(data, &read) != nil || read.State.Oppo.Hand != nil || bytes.Contains(data, []byte(`"seed"`)) || bytes.Contains(data, []byte(`"rng"`)) {
					t.Fatal("state exposed hidden opening data")
				}
				want := 4
				if side == first {
					want = 5
				}
				if len(read.State.Own.Hand) != want || read.State.Own.ExtraPPAvailable != (side != first) {
					t.Fatal("opening resources did not follow the assigned order")
				}
				_, data = perform(t, h, http.MethodGet, path+"/replay", tokens[side])
				var replay replayResponse
				if json.Unmarshal(data, &replay) != nil || len(replay.Frames) != 2 || replay.Frames[0].State.Phase != "mulligan" || replay.Frames[1].State.Phase != "main" || replay.State.FirstPlayer != read.State.FirstPlayer {
					t.Fatal("replay lost opening phase or first-player perspective")
				}
			}
			for n, side := range []string{first, oppositeMatchSide(first)} {
				out := setupResponse(t, setupPost(t, h, path, tokens[side], command{Kind: "end_turn", ActionID: fmt.Sprintf("%032x", n+1)}))
				if out.Result.Status != runner.StatusCompleted || out.State.Turn.Number != n+1 {
					t.Fatal("network round advanced after the wrong seat", out.Result, out.State.Turn)
				}
			}
		}
	}
}

func TestConcurrentJoinsGenerateOnlyOneOpening(t *testing.T) {
	s := setupServer(t)
	calls := 0
	s.matchSetup = func() (uint64, string, error) { calls++; return 9, "oppo", nil }
	h := s.Handler()
	host := setupResponse(t, setupPost(t, h, "/api/matches", "", createRequest{}))
	path := "/api/matches/" + host.MatchID + "/join?code=" + host.JoinCode
	statuses := make(chan int, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- setupPost(t, h, path, "", createRequest{}).Code
		}()
	}
	wg.Wait()
	close(statuses)
	joined := 0
	for status := range statuses {
		if status == http.StatusOK {
			joined++
		} else if status != http.StatusConflict {
			t.Fatal(status)
		}
	}
	if joined != 1 || calls != 1 || len(s.matches[host.MatchID].replay) != 1 {
		t.Fatal("concurrent joins generated multiple openings")
	}
}
