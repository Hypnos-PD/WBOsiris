//go:build windows

package main

import (
	"fmt"
	"os/exec"
)

// native launch 本身只在 Linux/Proton 上成立（依赖 bubblewrap 的挂载命名空间），
// 这几个函数只是让整个包在 Windows 上也能编译。

func setProcessGroup(*exec.Cmd) {}

func killProcessGroup(*exec.Cmd) error {
	return fmt.Errorf("Windows 上还没有实现进程组回收")
}
