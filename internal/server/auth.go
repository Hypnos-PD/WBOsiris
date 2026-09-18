package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// lobbyAuth 是"进入大厅需要登录"的校验方式。
//
// 我们不自己再建一套账号：WBArts 已经有 JWT 账号体系（/api/auth/login|me），
// 客户端与服务同源时本来就在同一个 localStorage 里共享 jwt。
// 这里把客户端的 Bearer token 交给 WBArts 的 /api/auth/me 验证，
// 既不用共享密钥，也不会把账号逻辑复制一份。
type lobbyAuth struct {
	required  bool
	verifyURL string
	client    *http.Client
}

type lobbyUser struct {
	ID       any    `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Banned   bool   `json:"banned"`
}

// newLobbyAuth 按 verifyURL 构造：空字符串表示本地/离线模式，不要求登录。
func newLobbyAuth(verifyURL string) lobbyAuth {
	verifyURL = strings.TrimSpace(verifyURL)
	return lobbyAuth{
		required:  verifyURL != "",
		verifyURL: verifyURL,
		client:    &http.Client{Timeout: 8 * time.Second},
	}
}

// verify 校验请求里的 Bearer token；required 为 false 时直接放行。
func (a lobbyAuth) verify(r *http.Request) (lobbyUser, error) {
	if !a.required {
		return lobbyUser{}, nil
	}
	token := bearerToken(r)
	if token == "" {
		return lobbyUser{}, fmt.Errorf("login required")
	}
	return a.verifyToken(r.Context(), token)
}

func (a lobbyAuth) verifyToken(ctx context.Context, token string) (lobbyUser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.verifyURL, nil)
	if err != nil {
		return lobbyUser{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := a.client.Do(request)
	if err != nil {
		return lobbyUser{}, fmt.Errorf("无法连接账号服务: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return lobbyUser{}, fmt.Errorf("login required")
	}
	if response.StatusCode != http.StatusOK {
		return lobbyUser{}, fmt.Errorf("账号服务返回 %d", response.StatusCode)
	}
	var user lobbyUser
	if err := json.NewDecoder(response.Body).Decode(&user); err != nil {
		return lobbyUser{}, fmt.Errorf("账号服务响应无效: %w", err)
	}
	if user.Banned {
		return lobbyUser{}, fmt.Errorf("账号已被封禁")
	}
	return user, nil
}

// requireLobbyLogin 在大厅相关动作前校验登录；失败时已经写好响应，返回 false。
func (s *Server) requireLobbyLogin(w http.ResponseWriter, r *http.Request) bool {
	if _, err := s.lobbyAuth.verify(r); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return false
	}
	return true
}
