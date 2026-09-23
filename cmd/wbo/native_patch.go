//go:build !windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// 从进程外部改写"对端公钥"常量。
//
// 为什么要从外面做：客户端谈判用的那把公钥是写死的，对应的私钥在官方服务器上，
// 我们不可能有。把它换成我们自己的公钥，客户端就会把请求加密成只有我们能解开的
// 形式。这份常量来自内嵌在 GameAssembly.dll 里的加密元数据，磁盘上没有明文，只有
// 运行时内存里才有。
//
// 为什么不在代理里做：代理是 Unity 按需加载的，第一个 P/Invoke 之前才进进程，而
// 扫一遍地址空间要几秒——那批包已经用原公钥加好密了。从外面盯就没这个问题：进程
// 一起来就能改。

type memoryRegion struct {
	start uint64
	end   uint64
	write bool
}

// patchGamePeerKey 一直盯着游戏进程，找到常量就改写，成功即返回。
func patchGamePeerKey(processName string, pattern, peerKey []byte, logf func(string, ...any), stop <-chan struct{}) {
	deadline := time.Now().Add(90 * time.Second)
	var lastPID int
	for time.Now().Before(deadline) {
		select {
		case <-stop:
			return
		default:
		}
		pid := findGameProcess(processName, lastPID)
		if pid == 0 {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if pid != lastPID {
			lastPID = pid
			logf("公钥改写：盯上游戏进程 %d", pid)
		}
		patched, err := patchProcessMemory(pid, pattern, peerKey)
		if err != nil {
			// 进程刚起来时映射还在变，失败是常态，继续试。
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if patched > 0 {
			logf("公钥改写：已在进程 %d 改写 %d 处", pid, patched)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	logf("公钥改写：90 秒内没有找到可改写的常量（客户端可能还没走到那一步）")
}

// findGameProcess 在 /proc 里找命令行以该名字结尾的进程。
//
// 只认"最后一段"，因为启动链上很多进程（启动器、bwrap、reaper）的命令行里都会
// 出现这个名字，真正持有那些内存的是 Wine 起的那一个。
func findGameProcess(name string, previous int) int {
	if previous != 0 && commandLineMatches(previous, name) {
		return previous
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		if commandLineMatches(pid, name) {
			return pid
		}
	}
	return 0
}

func commandLineMatches(pid int, name string) bool {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil || len(raw) == 0 {
		return false
	}
	parts := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
	last := parts[len(parts)-1]
	return strings.EqualFold(filepath.Base(strings.ReplaceAll(last, `\`, "/")), name)
}

// patchProcessMemory 扫描该进程所有可写、已提交的内存区域并改写。
func patchProcessMemory(pid int, pattern, peerKey []byte) (int, error) {
	maps, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "maps"))
	if err != nil {
		return 0, err
	}
	memory, err := os.OpenFile(filepath.Join("/proc", strconv.Itoa(pid), "mem"), os.O_RDWR, 0)
	if err != nil {
		return 0, err
	}
	defer memory.Close()
	patched := 0
	for _, region := range parseWritableRegions(string(maps)) {
		size := region.end - region.start
		if size == 0 || size > 512<<20 {
			continue
		}
		buffer := make([]byte, size)
		if _, err := memory.ReadAt(buffer, int64(region.start)); err != nil {
			continue
		}
		for offset := 0; offset+len(pattern) <= len(buffer); {
			found := bytes.Index(buffer[offset:], pattern)
			if found < 0 {
				break
			}
			at := offset + found
			if _, err := memory.WriteAt(peerKey, int64(region.start)+int64(at)); err != nil {
				return patched, err
			}
			patched++
			offset = at + len(pattern)
		}
	}
	if patched == 0 {
		return 0, errors.New("常量还没出现在这段内存里")
	}
	return patched, nil
}

func parseWritableRegions(maps string) []memoryRegion {
	var regions []memoryRegion
	for _, line := range strings.Split(maps, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "rw") {
			continue
		}
		bounds := strings.SplitN(fields[0], "-", 2)
		if len(bounds) != 2 {
			continue
		}
		start, err := strconv.ParseUint(bounds[0], 16, 64)
		if err != nil {
			continue
		}
		end, err := strconv.ParseUint(bounds[1], 16, 64)
		if err != nil {
			continue
		}
		regions = append(regions, memoryRegion{start: start, end: end, write: true})
	}
	return regions
}

func describePatch(processName string, pattern, peerKey []byte) string {
	return fmt.Sprintf("%s 的 %d 字节常量 → %d 字节公钥", processName, len(pattern), len(peerKey))
}
