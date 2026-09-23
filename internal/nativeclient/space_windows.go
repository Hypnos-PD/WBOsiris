//go:build windows

package nativeclient

import "golang.org/x/sys/windows"

// freeSpace 返回路径所在卷的可用字节数。
func freeSpace(path string) (int64, error) {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var available uint64
	// 第三个参数是卷总容量，这里不需要。
	if err := windows.GetDiskFreeSpaceEx(pointer, &available, nil, nil); err != nil {
		return 0, err
	}
	return int64(available), nil
}
