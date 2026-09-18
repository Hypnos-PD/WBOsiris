package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"wbo/internal/runner"
)

func TestDeckCodeEndpointRoundTrip(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")})
	if err != nil {
		t.Fatal(err)
	}
	deck := runner.PracticeDeck()
	post := func(payload map[string]any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/deckcode", bytes.NewReader(data))
		res := httptest.NewRecorder()
		s.Handler().ServeHTTP(res, req)
		return res
	}
	res := post(map[string]any{"cards": deck, "format": runner.FormatRotation})
	if res.Code != http.StatusOK {
		t.Fatalf("encode status=%d body=%s", res.Code, res.Body.String())
	}
	var encoded deckCodeResponse
	if err := json.NewDecoder(res.Body).Decode(&encoded); err != nil {
		t.Fatal(err)
	}
	// 默认练习卡组是"中立 + 精灵"，推断出的职业是精灵。
	if encoded.Code == "" || !encoded.Legal || encoded.Class != "forestcraft" {
		t.Fatalf("encode response = %#v", encoded)
	}
	res = post(map[string]any{"code": encoded.Code})
	if res.Code != http.StatusOK {
		t.Fatalf("decode status=%d body=%s", res.Code, res.Body.String())
	}
	var decoded deckCodeResponse
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Legal || len(decoded.Cards) != 40 || decoded.Format != runner.FormatRotation {
		t.Fatalf("decode response = %#v", decoded)
	}
	// 非法码给出 400；不合法的牌组仍然能编码，但会标记 legal=false 并说明原因。
	if res := post(map[string]any{"code": "1.9.c9hM"}); res.Code != http.StatusBadRequest {
		t.Fatalf("malformed code status=%d", res.Code)
	}
	res = post(map[string]any{"cards": deck[:10], "format": runner.FormatRotation})
	if res.Code != http.StatusOK {
		t.Fatalf("draft encode status=%d", res.Code)
	}
	var draft deckCodeResponse
	if err := json.NewDecoder(res.Body).Decode(&draft); err != nil {
		t.Fatal(err)
	}
	if draft.Legal || draft.Problem == "" {
		t.Fatalf("draft should be flagged as incomplete: %#v", draft)
	}
}
