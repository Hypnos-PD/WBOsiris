package nativeclient

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// 导入只做一件事：把 Steam 装的那份客户端复制成我们自己的副本。
//
// Steam 的安装目录永远只读，补丁只写副本。这样既不影响游戏完整性校验，也让
// 卸载等于删目录。复制的是整棵目录树，而不是 profile 里登记的 37 个文件，因为
// 客户端启动还需要大量没有逐个登记的资源。

// ImportPlan 是一次导入的清单，先算出来再动手。
type ImportPlan struct {
	Source      string
	Destination string
	Files       int
	Bytes       int64
}

// ImportProgress 在复制每个文件前回调一次。
type ImportProgress func(doneBytes, totalBytes int64, path string)

// PlanImport 统计复制量并检查路径关系。
func PlanImport(source, destination string) (*ImportPlan, error) {
	source, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return nil, err
	}
	if err := checkDisjoint(source, destination); err != nil {
		return nil, err
	}
	if !isDir(source) {
		return nil, fmt.Errorf("nativeclient: 源目录不存在: %s", source)
	}
	plan := &ImportPlan{Source: source, Destination: destination}
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			// 跟随符号链接会读到源目录之外的东西，直接拒绝。
			return fmt.Errorf("nativeclient: 源目录含符号链接，拒绝导入: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		plan.Files++
		plan.Bytes += info.Size()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return plan, nil
}

// RunImport 执行复制。重复运行是安全的：已存在的文件会被覆盖，中断之后可以重来。
func RunImport(plan *ImportPlan, progress ImportProgress) error {
	if plan == nil {
		return fmt.Errorf("nativeclient: 缺少导入清单")
	}
	if err := checkDisjoint(plan.Source, plan.Destination); err != nil {
		return err
	}
	// 客户端实际有二十多 GiB，先问清楚目的地装不装得下，别让用户复制到一半失败。
	if available, err := FreeSpace(plan.Destination); err == nil && available < plan.Bytes {
		return fmt.Errorf("nativeclient: 目标文件系统只剩 %.1f GiB，导入需要 %.1f GiB",
			float64(available)/(1<<30), float64(plan.Bytes)/(1<<30))
	}
	var done int64
	return filepath.WalkDir(plan.Source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(plan.Source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(plan.Destination, 0o755)
		}
		target := filepath.Join(plan.Destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if progress != nil {
			progress(done, plan.Bytes, relative)
		}
		written, err := copyFile(path, target)
		if err != nil {
			return err
		}
		done += written
		return nil
	})
}

// FreeSpace 返回路径所在文件系统的可用字节数。
//
// 目标目录通常还不存在，所以从它往上找到第一个已存在的祖先再问文件系统。
func FreeSpace(path string) (int64, error) {
	probe, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	for {
		if info, err := os.Stat(probe); err == nil && info.IsDir() {
			return freeSpace(probe)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return 0, fmt.Errorf("nativeclient: 找不到 %s 的可写祖先目录", path)
		}
		probe = parent
	}
}

// VerifyImport 在目的地重新核对一遍 profile。
//
// 复制之后要重新验证，而不是相信复制过程本身：这是手上这份副本与契约对得上的
// 唯一证据，也是替换 GameAssembly.dll 之前的前提。
func VerifyImport(destination string, profile *Profile) (*Probe, error) {
	return Check(destination, profile)
}

func copyFile(source, target string) (int64, error) {
	in, err := os.Open(source)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	// 先写临时文件再原子替换，中断不会留下半个 GameAssembly.dll。
	temporary := target + ".wbo-import"
	out, err := os.OpenFile(temporary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm()|0o200)
	if err != nil {
		return 0, err
	}
	written, err := io.Copy(out, in)
	if err != nil {
		out.Close()
		os.Remove(temporary)
		return 0, err
	}
	if err := out.Close(); err != nil {
		os.Remove(temporary)
		return 0, err
	}
	if err := os.Rename(temporary, target); err != nil {
		os.Remove(temporary)
		return 0, err
	}
	return written, nil
}

func checkDisjoint(source, destination string) error {
	source = filepath.Clean(source)
	destination = filepath.Clean(destination)
	if source == destination {
		return fmt.Errorf("nativeclient: 源目录与目标目录相同: %s", source)
	}
	separator := string(filepath.Separator)
	if strings.HasPrefix(destination+separator, source+separator) {
		return fmt.Errorf("nativeclient: 目标目录在源目录内部: %s", destination)
	}
	if strings.HasPrefix(source+separator, destination+separator) {
		return fmt.Errorf("nativeclient: 源目录在目标目录内部: %s", source)
	}
	return nil
}
