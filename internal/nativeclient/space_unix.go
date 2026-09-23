//go:build !windows

package nativeclient

import "golang.org/x/sys/unix"

// freeSpace 返回路径所在文件系统的可用字节数。
func freeSpace(path string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	// Bavail 是非特权用户可用的块数；Bsize 是块大小。
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
