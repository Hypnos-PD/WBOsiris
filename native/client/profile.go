// Package nativeclient 负责原版《影之诗：超凡世界》客户端的发现、校验与导入。
//
// 这一层不实现任何游戏规则，只回答两个问题：这台机器上的客户端是不是我们支持的
// 那个构建，以及把它导入到工作目录需要复制哪些文件。
//
// 支持身份以 GameAssembly.dll 的 SHA-256 为准：线协议契约（路由、DTO 字段布局、
// 内存偏移）都是从那一份二进制里提取的。gameVersion 只是给人看的标签，任何判定
// 都不依赖它。
package nativeclient

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

// Steam 上的应用编号与安装目录名；发现逻辑与校验报告都用这两个常量。
const (
	SteamAppID  = "2584990"
	InstallDir  = "ShadowverseWB"
	ModuleName  = "GameAssembly.dll"
	ProcessName = "ShadowverseWB.exe"
)

//go:embed profiles/*.json
var profileData embed.FS

// ProfileFile 是必须与目标构建逐字节一致的一个文件。
//
// Path 相对客户端根目录，统一用正斜杠；哈希是小写十六进制。
type ProfileFile struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// Profile 描述一个受支持的客户端构建。
type Profile struct {
	Schema                    int           `json:"schema"`
	ID                        string        `json:"id"`
	GameVersion               string        `json:"gameVersion"`
	UnityVersion              string        `json:"unityVersion"`
	SteamAppID                string        `json:"steamAppId"`
	ProcessName               string        `json:"processName"`
	ModuleName                string        `json:"moduleName"`
	GameAssemblySHA256        string        `json:"gameAssemblySHA256"`
	PatchedGameAssemblySHA256 string        `json:"patchedGameAssemblySHA256"`
	RequiredFiles             []ProfileFile `json:"requiredFiles"`
	Provenance                string        `json:"provenance"`
}

// CriticalPath 是决定"这个构建是不是我们支持的那个"的文件。
//
// 契约数据全部提取自它，所以它的哈希不一致就意味着整份契约都不适用；其余文件
// 不一致只说明这份客户端被改动过或更新过一部分，报告出来由调用方决定怎么办。
const CriticalPath = ModuleName

// LoadProfile 读取一份 profile。
func LoadProfile(r io.Reader) (*Profile, error) {
	raw, err := io.ReadAll(io.LimitReader(r, 8<<20))
	if err != nil {
		return nil, err
	}
	var p Profile
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return nil, fmt.Errorf("nativeclient: 解析 profile 失败: %w", err)
	}
	if err := p.normalize(); err != nil {
		return nil, err
	}
	return &p, nil
}

// LoadProfileFile 从磁盘读取一份 profile。
func LoadProfileFile(name string) (*Profile, error) {
	raw, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return LoadProfile(bytes.NewReader(raw))
}

// ProfileByID 取内置的 profile；传空字符串表示"当前唯一受支持的那一份"。
func ProfileByID(id string) (*Profile, error) {
	entries, err := profileData.ReadDir("profiles")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("nativeclient: 没有内置 profile")
	}
	for _, name := range names {
		raw, err := profileData.ReadFile(path.Join("profiles", name))
		if err != nil {
			return nil, err
		}
		p, err := LoadProfile(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("nativeclient: 内置 profile %s 无效: %w", name, err)
		}
		if id == "" || p.ID == id {
			return p, nil
		}
	}
	return nil, fmt.Errorf("nativeclient: 没有 id 为 %q 的 profile", id)
}

// normalize 统一大小写与路径分隔符，并拒绝自相矛盾的数据。
func (p *Profile) normalize() error {
	if p.Schema != 1 {
		return fmt.Errorf("nativeclient: 不支持 profile schema %d", p.Schema)
	}
	if p.ID == "" {
		return fmt.Errorf("nativeclient: profile 缺少 id")
	}
	p.GameAssemblySHA256 = normalizeHash(p.GameAssemblySHA256)
	p.PatchedGameAssemblySHA256 = normalizeHash(p.PatchedGameAssemblySHA256)
	for n := range p.RequiredFiles {
		file := &p.RequiredFiles[n]
		file.SHA256 = normalizeHash(file.SHA256)
		file.Path = path.Clean(strings.ReplaceAll(file.Path, `\`, "/"))
		if file.Path == "." || strings.HasPrefix(file.Path, "../") || path.IsAbs(file.Path) {
			return fmt.Errorf("nativeclient: profile 中的路径必须相对客户端根目录: %q", file.Path)
		}
		if len(file.SHA256) != 64 {
			return fmt.Errorf("nativeclient: %s 的 sha256 不是 64 位十六进制", file.Path)
		}
	}
	if len(p.GameAssemblySHA256) != 64 {
		return fmt.Errorf("nativeclient: profile 缺少 gameAssemblySHA256")
	}
	if p.ProcessName == "" || p.ModuleName == "" {
		return fmt.Errorf("nativeclient: profile 缺少 processName/moduleName")
	}
	seen := map[string]bool{}
	for _, file := range p.RequiredFiles {
		if seen[file.Path] {
			return fmt.Errorf("nativeclient: profile 重复登记了 %s", file.Path)
		}
		seen[file.Path] = true
	}
	if !seen[CriticalPath] {
		return fmt.Errorf("nativeclient: profile 没有登记关键文件 %s", CriticalPath)
	}
	return nil
}

func normalizeHash(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
