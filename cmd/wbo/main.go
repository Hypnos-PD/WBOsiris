package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"wbo/internal/project"
	"wbo/internal/ruleset"
	"wbo/internal/runner"
	"wbo/internal/syntax"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var code int
	switch os.Args[1] {
	case "check":
		code = runCheck(os.Args[2:])
	case "format":
		code = runFormat(os.Args[2:])
	case "compile":
		code = runCompile(os.Args[2:])
	case "test":
		code = runTest(os.Args[2:])
	default:
		usage()
		code = 2
	}
	os.Exit(code)
}
func usage() { fmt.Fprintln(os.Stderr, "用法: wbo <check|format|compile|test> [选项] PATH...") }

func runCheck(args []string) int {
	args = interspersed(args, map[string]bool{"--strict-references": false, "--source-root": true})
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	strict := fs.Bool("strict-references", false, "将未解析引用视为错误")
	root := fs.String("source-root", "", "稳定源路径根目录（默认当前工作目录）")
	fs.SetOutput(os.Stderr)
	if fs.Parse(args) != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "check 需要至少一个路径")
		return 2
	}
	l := load(fs.Args(), *strict, *root)
	printDiagnostics(l.Diagnostics)
	errors, warnings := counts(l.Diagnostics)
	sc := 0
	for _, t := range l.Tests {
		sc += len(t.Scenarios)
	}
	fmt.Fprintf(os.Stderr, "汇总: %d 张卡牌，%d 个场景，%d 个错误，%d 个警告\n", len(l.Cards), sc, errors, warnings)
	if errors > 0 {
		return 1
	}
	return 0
}

func runFormat(args []string) int {
	args = interspersed(args, map[string]bool{"--write": false})
	fs := flag.NewFlagSet("format", flag.ContinueOnError)
	write := fs.Bool("write", false, "原子改写文件")
	fs.SetOutput(os.Stderr)
	if fs.Parse(args) != nil {
		return 2
	}
	if fs.NArg() == 0 || !*write && fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "format 无 --write 时只允许一个文件")
		return 2
	}
	for _, path := range fs.Args() {
		linfo, err := os.Lstat(path)
		if err == nil && linfo.Mode()&os.ModeSymlink != 0 {
			fmt.Fprintf(os.Stderr, "%s: format 拒绝符号链接\n", path)
			return 1
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			fmt.Fprintf(os.Stderr, "%s: 必须是可读文件\n", path)
			return 1
		}
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		f, ds := syntax.Parse(path, src)
		if hasDiagnosticErrors(ds) {
			printDiagnostics(ds)
			return 1
		}
		ds = project.ValidateFile(f)
		if hasDiagnosticErrors(ds) {
			printDiagnostics(ds)
			return 1
		}
		formatted := syntax.Format(f)
		f2, ds2 := syntax.Parse(path, formatted)
		if hasDiagnosticErrors(ds2) {
			printDiagnostics(ds2)
			return 1
		}
		if string(syntax.Format(f2)) != string(formatted) {
			fmt.Fprintln(os.Stderr, "格式器内部幂等检查失败")
			return 1
		}
		if !*write {
			if _, err := os.Stdout.Write(formatted); err != nil {
				fmt.Fprintln(os.Stderr, "写入 stdout 失败:", err)
				return 1
			}
			continue
		}
		if err := atomicWrite(path, formatted, info.Mode()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	return 0
}

func runCompile(args []string) int {
	args = interspersed(args, map[string]bool{"--allow-unresolved": false, "--output": true, "--source-root": true})
	fs := flag.NewFlagSet("compile", flag.ContinueOnError)
	allow := fs.Bool("allow-unresolved", false, "允许未解析引用")
	output := fs.String("output", "", "输出文件")
	root := fs.String("source-root", "", "稳定源路径根目录（默认当前工作目录）")
	fs.SetOutput(os.Stderr)
	if fs.Parse(args) != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "compile 需要至少一个路径")
		return 2
	}
	if *output != "" && strings.ToLower(filepath.Ext(*output)) != ".wbos" {
		fmt.Fprintln(os.Stderr, "compile 输出文件必须使用 .wbos 后缀")
		return 2
	}
	l := load(fs.Args(), !*allow, *root)
	printDiagnostics(l.Diagnostics)
	b, err := project.Compile(l, *allow)
	if err != nil {
		fmt.Fprintln(os.Stderr, "编译失败:", err)
		return 1
	}
	if *output == "" {
		if _, err := os.Stdout.Write(b); err != nil {
			fmt.Fprintln(os.Stderr, "写入 stdout 失败:", err)
			return 1
		}
		return 0
	}
	if err := atomicWrite(*output, b, 0644); err != nil {
		fmt.Fprintln(os.Stderr, "写入输出失败:", err)
		return 1
	}
	return 0
}

func runTest(args []string) int {
	args = interspersed(args, map[string]bool{"--source-root": true, "--ruleset": true})
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	root := fs.String("source-root", "", "稳定源路径根目录（默认当前工作目录）")
	rulesetID := fs.String("ruleset", ruleset.DefaultID, "对局规则集")
	fs.SetOutput(os.Stderr)
	if fs.Parse(args) != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "test 需要至少一个路径")
		return 2
	}
	l := load(fs.Args(), false, *root)
	printDiagnostics(l.Diagnostics)
	if l.HasErrors() {
		return 1
	}
	cardPack, testPack, err := project.BuildRuntimePacks(l)
	if err != nil {
		fmt.Fprintln(os.Stderr, "编译测试失败:", err)
		return 1
	}
	results := runner.RunWithRuleset(cardPack, testPack, *rulesetID)
	failed := 0
	for _, result := range results {
		if result.Passed() {
			fmt.Fprintf(os.Stdout, "PASS %s\n", result.Scenario)
			continue
		}
		failed++
		fmt.Fprintf(os.Stdout, "FAIL %s\n", result.Scenario)
		for _, failure := range result.Failures {
			fmt.Fprintf(os.Stdout, "  %s\n", failure)
		}
	}
	fmt.Fprintf(os.Stdout, "汇总: %d 个场景，%d 个通过，%d 个失败\n", len(results), len(results)-failed, failed)
	if failed > 0 {
		return 1
	}
	return 0
}

func load(paths []string, strict bool, root string) *project.Loaded {
	if root == "" {
		cwd, err := os.Getwd()
		if err == nil {
			root = cwd
		}
	}
	return project.LoadWithRoot(paths, strict, root)
}

func interspersed(args []string, known map[string]bool) []string {
	var options, paths []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name := a
		if j := strings.IndexByte(a, '='); j >= 0 {
			name = a[:j]
		}
		needsValue, ok := known[name]
		if strings.HasPrefix(a, "--") {
			if ok && needsValue && !strings.Contains(a, "=") && (i+1 >= len(args) || strings.HasPrefix(args[i+1], "--")) {
				options = append(options, "--wbo-missing-flag-value")
				continue
			}
			options = append(options, a)
			if ok && needsValue && !strings.Contains(a, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				options = append(options, args[i])
			}
			continue
		}
		paths = append(paths, a)
	}
	return append(options, paths...)
}

func printDiagnostics(ds []syntax.Diagnostic) {
	sort.SliceStable(ds, func(i, j int) bool {
		if ds[i].Span.File != ds[j].Span.File {
			return ds[i].Span.File < ds[j].Span.File
		}
		if ds[i].Span.Start.Byte != ds[j].Span.Start.Byte {
			return ds[i].Span.Start.Byte < ds[j].Span.Start.Byte
		}
		return ds[i].Code < ds[j].Code
	})
	for _, d := range ds {
		fmt.Fprintln(os.Stderr, d.String())
	}
}
func counts(ds []syntax.Diagnostic) (int, int) {
	e, w := 0, 0
	for _, d := range ds {
		if d.Severity == "错误" {
			e++
		} else {
			w++
		}
	}
	return e, w
}
func hasDiagnosticErrors(ds []syntax.Diagnostic) bool { e, _ := counts(ds); return e > 0 }
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".wbo-format-*")
	if err != nil {
		return err
	}
	name := f.Name()
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	if err = f.Chmod(mode.Perm()); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}
