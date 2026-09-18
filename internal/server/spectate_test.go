package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 观战凭据只读：能看状态与公开事件，看不到双方手牌，也不能提交动作。
func TestSpectatorSeesPublicStateOnly(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")})
	if err != nil {
		t.Fatal(err)
	}
	handler := s.Handler()
	status, created := postMatch(t, handler, "/api/matches", "", createRequest{Mode: "bot"})
	if status != http.StatusOK {
		t.Fatalf("create status=%d", status)
	}
	if status, _ := postMatch(t, handler, "/api/matches/"+created.MatchID+"/mulligan", created.PlayerToken, command{}); status != http.StatusOK {
		t.Fatal("mulligan failed")
	}
	status, spectator := postMatch(t, handler, "/api/matches/"+created.MatchID+"/spectate", "", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("spectate status=%d", status)
	}
	if spectator.Side != spectatorSide || spectator.PlayerToken == "" {
		t.Fatalf("spectator credentials = %#v", spectator)
	}
	if spectator.State.Viewer != spectatorSide {
		t.Fatalf("spectator view viewer=%q", spectator.State.Viewer)
	}
	if len(spectator.State.Own.Hand) != 0 || len(spectator.State.Oppo.Hand) != 0 {
		t.Fatal("spectator view leaked a hand")
	}
	// 观战不能提交动作。
	status, _ = postMatch(t, handler, "/api/matches/"+created.MatchID, spectator.PlayerToken, command{Kind: "end_turn"})
	if status != http.StatusForbidden {
		t.Fatalf("spectator action status=%d, want 403", status)
	}
	// 观战凭据刷新状态也只能用只读接口。
	status, _ = getMatch(t, handler, "/api/matches/"+created.MatchID, spectator.PlayerToken)
	if status != http.StatusOK {
		t.Fatalf("spectator GET status=%d", status)
	}
}

// 推送流：玩家凭据连上后，第一个事件就是当前状态，且带 revision。
func TestMatchStreamPushesState(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")})
	if err != nil {
		t.Fatal(err)
	}
	handler := s.Handler()
	status, created := postMatch(t, handler, "/api/matches", "", createRequest{Mode: "bot"})
	if status != http.StatusOK {
		t.Fatalf("create status=%d", status)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/matches/"+created.MatchID+"/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+created.PlayerToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream status=%d type=%q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(response.Body)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var payload struct {
			MatchID  string `json:"matchId"`
			Side     string `json:"side"`
			Revision uint64 `json:"revision"`
			State    struct {
				Viewer string `json:"viewer"`
			} `json:"state"`
		}
		if err := json.Unmarshal(bytes.TrimPrefix([]byte(line), []byte("data: ")), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.MatchID != created.MatchID || payload.Side != "own" || payload.State.Viewer != "own" {
			t.Fatalf("unexpected stream payload: %#v", payload)
		}
		return
	}
	t.Fatal("stream produced no state event")
}
