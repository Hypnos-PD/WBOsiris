package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// illustrationEntry 是主界面插图（home illustration）在协议里的形态。
// 字段与 WBArts 的 home_illust_index.json + config.json 对齐：客户端拿这些
// 参数就能自己算出相机窗口、背景摆位与要播的动画。
type illustrationEntry struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Type           string            `json:"type,omitempty"`
	Source         string            `json:"source"` // bundled | wbarts
	Skel           string            `json:"skel,omitempty"`
	Atlas          string            `json:"atlas,omitempty"`
	Background     string            `json:"background,omitempty"`
	Thumbnail      string            `json:"thumbnail,omitempty"`
	IdleAnimation  string            `json:"idleAnimation"`
	TapAnimations  []string          `json:"tapAnimations,omitempty"`
	BlendTimes     []float64         `json:"blendTimes,omitempty"`
	DefaultMix     float64           `json:"defaultMix"`
	SkeletonScale  float64           `json:"skeletonScale"`
	PrefabScale    float64           `json:"prefabScale"`
	AspectLayouts  map[string]any    `json:"aspectLayouts,omitempty"`
	Names          map[string]string `json:"names,omitempty"`
	CharacterVoice []string          `json:"-"`
}

type illustrationCache struct {
	once  sync.Once
	items []illustrationEntry
}

// illustrationsHandler 返回可选的主界面插图列表：内置的 hi_1001 永远在，
// 另外扫描 --illustration-root 指向的 WBArts 数据目录（默认 ../WBArts/data）。
func (s *Server) illustrationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.illustrations.once.Do(func() { s.illustrations.items = s.scanIllustrations() })
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, struct {
		Items []illustrationEntry `json:"items"`
		Root  string              `json:"root,omitempty"`
	}{s.illustrations.items, s.illustrationRoot})
}

// scanIllustrations 先放内置条目，再按 id 顺序补上 WBArts 目录里发现的插图。
func (s *Server) scanIllustrations() []illustrationEntry {
	items := []illustrationEntry{{
		ID: "hi_1001", Name: "拉卜赛因", Type: "home", Source: "bundled",
		Skel: "/assets/home/hi_1001.skel", Atlas: "/assets/home/hi_1001.atlas",
		Background: "/assets/home/hi_1001-bg.webp",
		// 内置版的参数与 WBArts 的 hi_1001 一致，直接内联，省一次请求。
		IdleAnimation: "idle",
		TapAnimations: []string{"tap_01", "tap_02", "tap_03", "tap_04"},
		BlendTimes:    []float64{0.25, 0.25, 0.25, 0.25},
		DefaultMix:    0.2, SkeletonScale: 0.01, PrefabScale: 0.434,
		AspectLayouts: map[string]any{
			"4:3":  map[string]any{"x": 0.13, "y": 0, "scale_x": 1.18, "scale_y": 1.18},
			"16:9": map[string]any{"x": 0, "y": 0, "scale_x": 1.43, "scale_y": 1.43},
			"21:9": map[string]any{"x": 0, "y": 0, "scale_x": 1.2, "scale_y": 1.2},
		},
	}}
	root := s.illustrationRoot
	if root == "" {
		return items
	}
	index := readIllustrationIndex(root)
	entries, err := os.ReadDir(filepath.Join(root, "home-illustration"))
	if err != nil {
		return items
	}
	found := make([]illustrationEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		dir := filepath.Join(root, "home-illustration", id)
		config, err := readIllustrationConfig(filepath.Join(dir, "config.json"))
		if err != nil {
			continue
		}
		spine := index[id]
		if spine.skel == nil || spine.atlas == nil || spine.background == nil {
			continue
		}
		name := spine.name
		if name == "" {
			name = config.CharacterNames["chs"]
		}
		found = append(found, illustrationEntry{
			ID: id, Name: name, Type: spine.kind, Source: "wbarts",
			Skel: "/illustration-assets/" + relativeAsset(*spine.skel), Atlas: "/illustration-assets/" + relativeAsset(*spine.atlas),
			Background:    "/illustration-assets/" + relativeAsset(*spine.background),
			Thumbnail:     thumbnailURL(spine.thumbnail),
			IdleAnimation: stringDefault(config.IdleAnimation, "idle"),
			TapAnimations: config.TapAnimations, BlendTimes: config.BlendTimes,
			DefaultMix:    defaultValue(config.DefaultMix, 0.2),
			SkeletonScale: defaultValue(config.SkeletonScale, 0.01),
			PrefabScale:   defaultValue(valueOr(config.PrefabScale, config.PrefabTransforms.PrefabScale, spine.prefabScale), 1),
			AspectLayouts: spine.aspectLayouts,
			Names:         spine.names,
		})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].ID < found[j].ID })
	seen := map[string]bool{"hi_1001": true}
	for _, item := range found {
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		items = append(items, item)
	}
	return items
}

// illustrationAssetHandler 只读地服务插图素材（skel/atlas/图集/背景/缩略图）。
func (s *Server) illustrationAssetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.illustrationRoot == "" {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/illustration-assets/")
	clean := filepath.Clean("/" + name)
	if clean == "/" || strings.Contains(clean, "..") {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(s.illustrationRoot, clean)
	rel, err := filepath.Rel(s.illustrationRoot, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, full)
}

type illustrationIndexEntry struct {
	name          string
	kind          string
	prefabScale   float64
	skel          *string
	atlas         *string
	background    *string
	thumbnail     *string
	aspectLayouts map[string]any
	names         map[string]string
}

// readIllustrationIndex 读 home_illust_index.json，取出每张插图的文件与摆放参数。
func readIllustrationIndex(root string) map[string]illustrationIndexEntry {
	data, err := os.ReadFile(filepath.Join(root, "home_illust_index.json"))
	if err != nil {
		return nil
	}
	var raw []struct {
		ID            string             `json:"id"`
		Type          string             `json:"type"`
		Names         map[string]string  `json:"names"`
		Spine         map[string]*string `json:"spine"`
		Backgrounds   []string           `json:"backgrounds"`
		Thumbnail     string             `json:"thumbnail"`
		AspectLayouts map[string]any     `json:"aspectLayouts"`
		PrefabScale   float64            `json:"prefabScale"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	out := map[string]illustrationIndexEntry{}
	for _, item := range raw {
		entry := illustrationIndexEntry{name: item.Names["chs"], kind: item.Type, aspectLayouts: item.AspectLayouts, names: item.Names, prefabScale: item.PrefabScale}
		if item.Spine != nil {
			entry.skel, entry.atlas = item.Spine["skel"], item.Spine["atlas"]
		}
		if len(item.Backgrounds) > 0 {
			background := item.Backgrounds[0]
			entry.background = &background
		}
		if item.Thumbnail != "" {
			thumbnail := item.Thumbnail
			entry.thumbnail = &thumbnail
		}
		out[item.ID] = entry
	}
	return out
}

type illustrationConfig struct {
	CharacterNames map[string]string `json:"character_names"`
	IdleAnimation  string            `json:"idle_animation"`
	TapAnimations  []string          `json:"tap_animations"`
	BlendTimes     []float64         `json:"blend_times"`
	DefaultMix     float64           `json:"default_mix"`
	SkeletonScale  float64           `json:"skeleton_scale"`
	PrefabScale    float64           `json:"prefab_scale"`
	// prefab_scale 在 config.json 里位于 prefab_transforms 下，索引里则可能是 prefabScale。
	PrefabTransforms struct {
		PrefabScale float64 `json:"prefab_scale"`
	} `json:"prefab_transforms"`
}

func readIllustrationConfig(path string) (illustrationConfig, error) {
	var config illustrationConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return config, err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, err
	}
	return config, nil
}

// relativeAsset 把索引里的 data/... 路径转成插图素材路由下的相对路径。
func relativeAsset(path string) string {
	return strings.TrimPrefix(strings.TrimPrefix(path, "data/"), "/")
}

func thumbnailURL(path *string) string {
	if path == nil {
		return ""
	}
	return "/illustration-assets/" + relativeAsset(*path)
}

// valueOr 依次取第一个非零值（索引里有就用索引，否则读 config）。
func valueOr(values ...float64) float64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func defaultValue(value, fallback float64) float64 {
	if value == 0 {
		return fallback
	}
	return value
}

func stringDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
