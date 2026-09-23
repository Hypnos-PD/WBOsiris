package nativeclient

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// 单个文件的核对结果。
const (
	StatusMatch      = "match"
	StatusMismatch   = "mismatch"
	StatusMissing    = "missing"
	StatusUnreadable = "unreadable"
)

// FileVerdict 是一个文件与 profile 的比对结果。
type FileVerdict struct {
	Path           string
	Status         string
	Critical       bool
	ExpectedSHA256 string
	ActualSHA256   string
	ExpectedBytes  int64
	ActualBytes    int64
	Reason         string
}

// Probe 是一次完整核对。
//
// Supported 与 Identical 是两件事：契约数据全部来自 GameAssembly.dll，所以它一致
// 就说明这份客户端能用；其余文件不一致只说明客户端被改动过或更新到一半。安装器
// 用 Supported 放行，用 Identical 决定要不要提示用户。
type Probe struct {
	Root      string
	ProfileID string
	Files     []FileVerdict
}

// Check 逐个核对 profile 登记的文件。
func Check(root string, profile *Profile) (*Probe, error) {
	if profile == nil {
		return nil, fmt.Errorf("nativeclient: 缺少 profile")
	}
	probe := &Probe{Root: root, ProfileID: profile.ID}
	for _, want := range profile.RequiredFiles {
		probe.Files = append(probe.Files, checkFile(root, want))
	}
	return probe, nil
}

func checkFile(root string, want ProfileFile) FileVerdict {
	verdict := FileVerdict{
		Path:           want.Path,
		Critical:       want.Path == CriticalPath,
		ExpectedSHA256: want.SHA256,
		ExpectedBytes:  want.Bytes,
	}
	full := filepath.Join(root, filepath.FromSlash(want.Path))
	info, err := os.Stat(full)
	if err != nil {
		verdict.Status = StatusMissing
		verdict.Reason = err.Error()
		return verdict
	}
	if !info.Mode().IsRegular() {
		verdict.Status = StatusUnreadable
		verdict.Reason = "不是普通文件"
		return verdict
	}
	// 先比长度：不一致就没必要花时间读整个文件（其中一个是 195 MB）。
	if want.Bytes != 0 && info.Size() != want.Bytes {
		verdict.Status = StatusMismatch
		verdict.ActualBytes = info.Size()
		verdict.Reason = fmt.Sprintf("长度 %d，期望 %d", info.Size(), want.Bytes)
		return verdict
	}
	verdict.ActualBytes = info.Size()
	sum, err := hashFile(full)
	if err != nil {
		verdict.Status = StatusUnreadable
		verdict.Reason = err.Error()
		return verdict
	}
	verdict.ActualSHA256 = sum
	if sum != want.SHA256 {
		verdict.Status = StatusMismatch
		verdict.Reason = "内容哈希不一致"
		return verdict
	}
	verdict.Status = StatusMatch
	return verdict
}

func hashFile(name string) (string, error) {
	handle, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer handle.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, handle); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// Supported 表示决定契约有效性的那个文件一致，这份客户端可以继续使用。
func (p *Probe) Supported() bool {
	for _, file := range p.Files {
		if file.Critical && file.Status != StatusMatch {
			return false
		}
	}
	return true
}

// Identical 表示 profile 登记的文件全部逐字节一致。
func (p *Probe) Identical() bool {
	for _, file := range p.Files {
		if file.Status != StatusMatch {
			return false
		}
	}
	return true
}

// Divergent 返回所有与 profile 不一致的文件。
func (p *Probe) Divergent() []FileVerdict {
	var divergent []FileVerdict
	for _, file := range p.Files {
		if file.Status != StatusMatch {
			divergent = append(divergent, file)
		}
	}
	return divergent
}

// Critical 返回关键文件的核对结果。
func (p *Probe) Critical() (FileVerdict, bool) {
	for _, file := range p.Files {
		if file.Critical {
			return file, true
		}
	}
	return FileVerdict{}, false
}
