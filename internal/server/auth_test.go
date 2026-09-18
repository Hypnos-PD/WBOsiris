package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// 本地模式（没有校验地址）不要求登录：建房照常可用。
func TestLobbyWithoutAuthAllowsCreating(t *testing.T) {
	root := serverRoot(t)
	s, err := NewWithOptions(root, []string{filepath.Join(root, "cards")}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	handler := s.Handler()
	status, body := perform(t, handler, http.MethodGet, "/api/health", "")
	if status != http.StatusOK {
		t.Fatalf("health status=%d", status)
	}
	var health struct {
		OK                 bool `json:"ok"`
		LobbyRequiresLogin bool `json:"lobbyRequiresLogin"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatal(err)
	}
	if !health.OK || health.LobbyRequiresLogin {
		t.Fatalf("health = %#v", health)
	}
	status, _ = postMatch(t, handler, "/api/matches", "", createRequest{})
	if status != http.StatusOK {
		t.Fatalf("create without auth status=%d", status)
	}
}

// 托管模式：没有 token 一律 401，token 通过账号服务校验后才放行。
func TestLobbyAuthVerifiesTokenAgainstAccountService(t *testing.T) {
	// 假装成 WBArts 的 /api/auth/me：只认 "good-token"。
	accounts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"username":"tester","role":"user"}`))
	}))
	defer accounts.Close()

	root := serverRoot(t)
	s, err := NewWithOptions(root, []string{filepath.Join(root, "cards")}, "", accounts.URL)
	if err != nil {
		t.Fatal(err)
	}
	handler := s.Handler()
	status, body := perform(t, handler, http.MethodGet, "/api/health", "")
	if status != http.StatusOK {
		t.Fatalf("health status=%d", status)
	}
	var health struct {
		LobbyRequiresLogin bool `json:"lobbyRequiresLogin"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatal(err)
	}
	if !health.LobbyRequiresLogin {
		t.Fatal("health should report that the lobby requires login")
	}
	// 没带 token：401，且不能建房。
	if status, _ := postMatch(t, handler, "/api/matches", "", createRequest{}); status != http.StatusUnauthorized {
		t.Fatalf("create without token status=%d", status)
	}
	// token 无效：401。
	request := httptest.NewRequest(http.MethodPost, "/api/matches", bytes.NewReader([]byte("{}")))
	request.Header.Set("Authorization", "Bearer bad-token")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, request)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("bad token status=%d", res.Code)
	}
	// token 有效：建房成功。
	request = httptest.NewRequest(http.MethodPost, "/api/matches", bytes.NewReader([]byte("{}")))
	request.Header.Set("Authorization", "Bearer good-token")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, request)
	if res.Code != http.StatusOK {
		t.Fatalf("good token status=%d body=%s", res.Code, res.Body.String())
	}
}
