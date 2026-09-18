package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIllustrationsAlwaysIncludeBundledDefault(t *testing.T) {
	// 没有 WBArts 数据目录时也必须给出内置的那张，否则大厅没有主界面可用。
	root := serverRoot(t)
	s, err := NewWithIllustrations(root, []string{filepath.Join(root, "cards")}, "")
	if err != nil {
		t.Fatal(err)
	}
	status, body := perform(t, s.Handler(), http.MethodGet, "/api/illustrations", "")
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	var payload struct {
		Items []illustrationEntry `json:"items"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("items = %d, want the bundled one only", len(payload.Items))
	}
	bundled := payload.Items[0]
	if bundled.ID != "hi_1001" || bundled.Source != "bundled" || bundled.PrefabScale == 0 || len(bundled.AspectLayouts) == 0 {
		t.Fatalf("bundled entry = %#v", bundled)
	}
	if bundled.Skel == "" || bundled.Atlas == "" || bundled.Background == "" {
		t.Fatalf("bundled entry misses asset urls: %#v", bundled)
	}
}

func TestIllustrationsScanWBArtsDataDirectory(t *testing.T) {
	root := serverRoot(t)
	data := filepath.Join(root, "..", "WBArts", "data")
	if _, err := os.Stat(filepath.Join(data, "home_illust_index.json")); err != nil {
		t.Skip("WBArts data directory is not available")
	}
	s, err := NewWithIllustrations(root, []string{filepath.Join(root, "cards")}, data)
	if err != nil {
		t.Fatal(err)
	}
	items := s.scanIllustrations()
	if len(items) < 2 {
		t.Fatalf("expected the bundled illustration plus WBArts entries, got %d", len(items))
	}
	if items[0].Source != "bundled" {
		t.Fatalf("bundled entry must come first: %#v", items[0])
	}
	found := false
	for _, item := range items[1:] {
		if item.Source != "wbarts" || !strings.HasPrefix(item.Skel, "/illustration-assets/") {
			t.Fatalf("wbarts entry looks wrong: %#v", item)
		}
		if item.PrefabScale == 0 || item.SkeletonScale == 0 || item.IdleAnimation == "" {
			t.Fatalf("wbarts entry misses composition parameters: %#v", item)
		}
		if item.ID == "hi_1002" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected hi_1002 in the scanned list")
	}
	// 内置的那张不会因为目录里也有同名目录而重复。
	seen := map[string]bool{}
	for _, item := range items {
		if seen[item.ID] {
			t.Fatalf("duplicate illustration %s", item.ID)
		}
		seen[item.ID] = true
	}
}

func TestIllustrationAssetsAreServedReadOnly(t *testing.T) {
	root := serverRoot(t)
	data := filepath.Join(root, "..", "WBArts", "data")
	if _, err := os.Stat(filepath.Join(data, "home_illust_index.json")); err != nil {
		t.Skip("WBArts data directory is not available")
	}
	s, err := NewWithIllustrations(root, []string{filepath.Join(root, "cards")}, data)
	if err != nil {
		t.Fatal(err)
	}
	handler := s.Handler()
	status, body := perform(t, handler, http.MethodGet, "/illustration-assets/home-illustration/hi_1002/spine_hi_1002.skel", "")
	if status != http.StatusOK || len(body) == 0 {
		t.Fatalf("asset status=%d size=%d", status, len(body))
	}
	// 目录穿越不能被读出内容（net/http 会把 `..` 归一化后重定向，最终也拿不到文件）。
	request := httptest.NewRequest(http.MethodGet, "/illustration-assets/../home_illust_index.json", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, request)
	if res.Code == http.StatusOK && strings.Contains(res.Body.String(), "hi_1002") {
		t.Fatal("path traversal served the illustration index")
	}
	// 目录本身与空路径直接 404。
	for _, path := range []string{"/illustration-assets/home-illustration/hi_1002", "/illustration-assets/"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, request)
		if res.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d, want 404", path, res.Code)
		}
	}
	if status, _ := perform(t, handler, http.MethodPost, "/api/illustrations", ""); status != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/illustrations status=%d", status)
	}
}
