package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// selfplay 是"能对战"的压测入口：两局 greedy 自对弈必须跑完并且退出码为 0。
func TestSelfplayRunsTwoGreedyGames(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = write
	code := runSelfplay([]string{"--source-root", root, "--games", "2", "--policy", "greedy", filepath.Join(root, "cards")})
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = original
	output, err := io.ReadAll(read)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("selfplay exit code %d, output:\n%s", code, output)
	}
	if !strings.Contains(string(output), "自对弈 2 局") || !strings.Contains(string(output), "引擎/策略报错 0") {
		t.Fatalf("unexpected selfplay report:\n%s", output)
	}
}
