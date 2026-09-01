package project

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"wbo/internal/syntax"
)

func Discover(paths []string) ([]string, []syntax.Diagnostic) {
	seen := map[string]bool{}
	var files []string
	var ds []syntax.Diagnostic
	for _, input := range paths {
		info, err := os.Stat(input)
		if err != nil {
			diag(&ds, "WBO-E001-SYNTAX", "错误", "无法访问路径: "+err.Error(), syntax.Span{File: input, Start: syntax.Position{Line: 1, Column: 1}, End: syntax.Position{Line: 1, Column: 1}})
			continue
		}
		if !info.IsDir() {
			ext := strings.ToLower(filepath.Ext(input))
			if ext == ".wbo" || ext == ".wbotest" {
				p, _ := filepath.Abs(input)
				if !seen[p] {
					seen[p] = true
					files = append(files, p)
				}
			}
			continue
		}
		filepath.WalkDir(input, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				diag(&ds, "WBO-E001-SYNTAX", "错误", "遍历路径失败: "+err.Error(), syntax.Span{File: path, Start: syntax.Position{Line: 1, Column: 1}, End: syntax.Position{Line: 1, Column: 1}})
				return nil
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".wbo" && ext != ".wbotest" {
				return nil
			}
			p, _ := filepath.Abs(path)
			if !seen[p] {
				seen[p] = true
				files = append(files, p)
			}
			return nil
		})
	}
	sort.Strings(files)
	return files, ds
}

func Load(paths []string, strictReferences bool) *Loaded {
	root, err := filepath.Abs(".")
	if err != nil {
		root = "."
	}
	return LoadWithRoot(paths, strictReferences, root)
}

func LoadWithRoot(paths []string, strictReferences bool, root string) *Loaded {
	files, ds := Discover(paths)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	absRoot = filepath.Clean(absRoot)
	l := &Loaded{Root: absRoot, Diagnostics: ds, Unresolved: map[string]syntax.Span{}}
	for _, path := range files {
		if !withinRoot(absRoot, path) {
			diag(&l.Diagnostics, "WBO-E016-PACK-DEPENDENCY", "错误", "源文件不在 source root 内", syntax.Span{File: path, Start: syntax.Position{Line: 1, Column: 1}, End: syntax.Position{Line: 1, Column: 1}})
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			diag(&l.Diagnostics, "WBO-E001-SYNTAX", "错误", "读取文件失败: "+err.Error(), syntax.Span{File: path, Start: syntax.Position{Line: 1, Column: 1}, End: syntax.Position{Line: 1, Column: 1}})
			continue
		}
		f, pds := syntax.Parse(path, src)
		l.Diagnostics = append(l.Diagnostics, pds...)
		if hasErrors(pds) {
			continue
		}
		if strings.HasSuffix(path, ".wbotest") {
			if t := validateTest(f, &l.Diagnostics); t != nil {
				l.Tests = append(l.Tests, t)
			}
		} else {
			if c := validateCard(f, &l.Diagnostics); c != nil {
				l.Cards = append(l.Cards, c)
			}
		}
	}
	loadTestDependencies(l)
	validateCollection(l, strictReferences)
	return l
}

func withinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func loadTestDependencies(l *Loaded) {
	existing := map[string]bool{}
	for _, c := range l.Cards {
		existing[c.Path] = true
	}
	for _, tf := range l.Tests {
		root := filepath.Clean(filepath.Join(filepath.Dir(tf.Path), filepath.FromSlash(tf.UseCards)))
		files, ds := Discover([]string{root})
		l.Diagnostics = append(l.Diagnostics, ds...)
		for _, path := range files {
			if existing[path] || !strings.HasSuffix(path, ".wbo") {
				continue
			}
			existing[path] = true
			src, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			f, pds := syntax.Parse(path, src)
			l.Diagnostics = append(l.Diagnostics, pds...)
			if hasErrors(pds) {
				continue
			}
			if c := validateCard(f, &l.Diagnostics); c != nil {
				l.Dependencies = append(l.Dependencies, c)
			}
		}
	}
}

func hasErrors(ds []syntax.Diagnostic) bool {
	for _, d := range ds {
		if d.Severity == "错误" {
			return true
		}
	}
	return false
}

func ValidateFile(f *syntax.File) []syntax.Diagnostic {
	var ds []syntax.Diagnostic
	if strings.HasSuffix(strings.ToLower(f.Path), ".wbotest") {
		validateTest(f, &ds)
	} else {
		validateCard(f, &ds)
	}
	return ds
}
func (l *Loaded) HasErrors() bool { return hasErrors(l.Diagnostics) }
