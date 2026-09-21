package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"wbo/internal/project"
)

// cardFeaturesPayload 是离线卡面特征快照：训练、评测与客户端都读它，**不需要起引擎进程**。
//
// 保真度与运行时一致：它用的是同一套 `.wbo` 解析器与 IR，而不是另写一个解析器。
type cardFeaturesPayload struct {
	Schema      string        `json:"schema"`
	GeneratedAt int64         `json:"generatedAt"`
	SourceRoot  string        `json:"sourceRoot"`
	Count       int           `json:"count"`
	PoolHash    string        `json:"poolHash"`
	Cards       []envCardInfo `json:"cards"`
}

// runCardFeatures 导出卡面特征：`wbo card-features --out data/card_features.json cards`
func runCardFeatures(args []string) int {
	args = interspersed(args, map[string]bool{"--out": true, "--source-root": true, "--pretty": false})
	fs := flag.NewFlagSet("card-features", flag.ContinueOnError)
	out := fs.String("out", "", "输出文件（默认 stdout）")
	root := fs.String("source-root", "", "稳定源路径根目录（默认当前工作目录）")
	pretty := fs.Bool("pretty", false, "缩进输出（默认紧凑）")
	fs.SetOutput(os.Stderr)
	if fs.Parse(args) != nil {
		return 2
	}
	paths := fs.Args()
	if len(paths) == 0 {
		paths = []string{"cards"}
	}
	loaded := load(paths, true, *root)
	printDiagnostics(loaded.Diagnostics)
	if loaded.HasErrors() {
		return 1
	}
	cards, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		fmt.Fprintln(os.Stderr, "编译卡池失败:", err)
		return 1
	}
	resolvedRoot := *root
	if resolvedRoot == "" {
		resolvedRoot, _ = os.Getwd()
	}
	payload := cardFeaturesPayload{
		Schema:      "wbo-card-features/1",
		GeneratedAt: time.Now().Unix(),
		SourceRoot:  resolvedRoot,
		Count:       len(cards.Cards),
		PoolHash:    cardPoolHash(cards),
		Cards:       cardPool(cards),
	}
	var blob []byte
	if *pretty {
		blob, err = json.MarshalIndent(payload, "", " ")
	} else {
		blob, err = json.Marshal(payload)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "序列化失败:", err)
		return 1
	}
	if *out == "" {
		if _, err := os.Stdout.Write(append(blob, '\n')); err != nil {
			fmt.Fprintln(os.Stderr, "写入 stdout 失败:", err)
			return 1
		}
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil && filepath.Dir(*out) != "." {
		fmt.Fprintln(os.Stderr, "创建输出目录失败:", err)
		return 1
	}
	if err := atomicWrite(*out, append(blob, '\n'), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "写入输出失败:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "写入 %s（%d 张卡，pool %s）\n", *out, payload.Count, payload.PoolHash[:12])
	return 0
}
