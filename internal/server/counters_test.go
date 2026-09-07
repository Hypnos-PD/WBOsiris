package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/runner"
)

func TestCounterMatchAndReplayVisibility(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")})
	if err != nil {
		t.Fatal(err)
	}
	var state ir.State
	for _, scenario := range s.tests.Scenarios {
		if scenario.Name == "William evolution boosts the hand without changing its own X" {
			state = scenario.InitialState
		}
	}
	if state.Players == nil {
		t.Fatal("missing counter fixture")
	}
	session, err := runner.NewSession(s.cards, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	room := &match{session: session, joined: true, started: true, players: map[string]string{"host": "own", "guest": "oppo"}}
	s.matches["counter"] = room
	recordReplay(room)
	view, _ := session.View("own")
	if result := session.Submit("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", runner.SimulatorCommand{Kind: "evolve", Source: view.Own.Field[0].InstanceID}); result.Status != runner.StatusCompleted {
		t.Fatal(result)
	}
	recordReplay(room)
	for _, token := range []string{"host", "guest"} {
		status, body := perform(t, s.Handler(), http.MethodGet, "/api/matches/counter/replay", token)
		var replay replayResponse
		if status != http.StatusOK || json.Unmarshal(body, &replay) != nil || len(replay.Frames) != 2 {
			t.Fatal("invalid replay", status)
		}
		for n, frame := range replay.Frames {
			if len(frame.State.Oppo.Hand) != 0 {
				t.Fatal("replay exposed opponent hand")
			}
			field := frame.State.Own.Field
			if token == "guest" {
				field = frame.State.Oppo.Field
			}
			if field[0].Counters["x"] != 5 {
				t.Fatal("replay lost public counter")
			}
			if token == "host" && frame.State.Own.Hand[0].Counters["x"] != 2+n*2 {
				t.Fatal("replay changed historical hand counter")
			}
		}
		status, body = perform(t, s.Handler(), http.MethodGet, "/api/matches/counter", token)
		var live response
		if status != http.StatusOK || json.Unmarshal(body, &live) != nil || len(live.State.Oppo.Hand) != 0 {
			t.Fatal("invalid live visibility")
		}
		if token == "host" && live.State.Own.Hand[0].Counters["x"] != 4 {
			t.Fatal("live state lost boosted counter")
		}
	}
}
