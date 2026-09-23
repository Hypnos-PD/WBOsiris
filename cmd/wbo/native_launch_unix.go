//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// setProcessGroup 让 broker 单独成组，退出时能整组收掉（它自己可能起子进程）。
func setProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup 收掉整个进程组。负号表示按组发送信号。
func killProcessGroup(command *exec.Cmd) error {
	return syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
}
