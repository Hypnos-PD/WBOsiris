package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/runner"
)

func TestCrestReplayPreservesPublicLeaderAreaAndHiddenHands(t *testing.T) {
	for _, owner := range []string{"own", "oppo"} {
		other := oppositeMatchSide(owner)
		pack := &ir.CardPack{Cards: []ir.Card{
			{ID: 77778001, CardType: "follower", Crest: &ir.CrestDefinition{Countdown: 2, Locales: map[string]ir.Locale{"chs": {Name: "Crest", Text: "Rules"}}}},
			{ID: 77778002, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
		}}
		crest, hidden := strings.Repeat("a", 32), strings.Repeat("b", 32)
		state := ir.State{Turn: ir.Turn{Active: other, Number: 7}, Phase: "main", Players: map[string]ir.PlayerState{
			owner: {Leader: ir.Leader{Life: 20, MaxLife: 20}, Zones: map[string][]ir.TestInstance{
				"crests": {{InstanceID: crest, CardID: 77778001, DeclaredType: "crest"}},
				"deck":   {{InstanceID: hidden, CardID: 77778002, DeclaredType: "follower"}},
			}}, other: {Leader: ir.Leader{Life: 20, MaxLife: 20}},
		}}
		s, err := runner.NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		room := &match{session: s, joined: true, started: true, players: map[string]string{"owner": owner, "other": other}}
		server := &Server{cards: pack, matches: map[string]*match{"crests": room}}
		recordReplay(room)
		if r := s.SubmitAs(strings.Repeat("c", 32), other, runner.SimulatorCommand{Kind: "end_turn"}); r.Status != runner.StatusCompleted {
			t.Fatal(r)
		}
		recordReplay(room)
		for _, token := range []string{"owner", "other"} {
			status, body := perform(t, server.Handler(), http.MethodGet, "/api/matches/crests/replay", token)
			var replay replayResponse
			if status != http.StatusOK || json.Unmarshal(body, &replay) != nil || len(replay.Frames) != 2 {
				t.Fatal(status, string(body))
			}
			for index, frame := range replay.Frames {
				p := frame.State.Own
				if token == "other" {
					p = frame.State.Oppo
					if len(p.Hand) != 0 {
						t.Fatal("crest replay leaked hand")
					}
				}
				if len(p.Crests) != 1 || p.Crests[0].InstanceID != crest || p.Crests[0].Countdown != 2-index || p.Crests[0].CrestLocales["chs"].Text != "Rules" {
					t.Fatal("crest replay changed historical state")
				}
			}
			if token == "other" && strings.Contains(string(body), hidden) {
				t.Fatal("private draw leaked through crest replay")
			}
		}
	}
}
