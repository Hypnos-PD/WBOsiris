package project

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"wbo/internal/syntax"
)

type Locale struct{ Name, Text string }

type Card struct {
	Path, Version, ID, Type string
	Cost                    int
	Stats                   *[2]int
	Traits                  []string
	Effect                  []*syntax.Statement
	Meta                    struct {
		Pack          int
		Class, Rarity string
	}
	Locales map[string]Locale
	File    *syntax.File
	Decl    *syntax.Statement
}

type TestFile struct {
	Path, Version, UseCards string
	Scenarios               []*syntax.Statement
	File                    *syntax.File
}

type Loaded struct {
	Root         string
	Cards        []*Card
	Dependencies []*Card
	Tests        []*TestFile
	Diagnostics  []syntax.Diagnostic
	Unresolved   map[string]syntax.Span
}

func tokenValues(s *syntax.Statement) []string {
	t := s.Tokens()
	out := make([]string, len(t))
	for i := range t {
		out[i] = t[i].Value
	}
	return out
}

func version(s *syntax.Statement, head string) (string, bool) {
	v := tokenValues(s)
	if len(v) != 6 || v[0] != head || v[2] != "." || v[4] != "." || !s.Terminated {
		return "", false
	}
	for _, i := range []int{1, 3, 5} {
		if _, err := strconv.ParseUint(v[i], 10, 32); err != nil {
			return "", false
		}
	}
	return v[1] + "." + v[3] + "." + v[5], true
}

func integer(t syntax.Token) (int, bool) {
	if t.Kind != syntax.Integer || len(t.Value) > 1 && t.Value[0] == '0' {
		return 0, false
	}
	n, err := strconv.ParseUint(t.Value, 10, 31)
	return int(n), err == nil
}

func diag(ds *[]syntax.Diagnostic, code, severity, msg string, sp syntax.Span) {
	*ds = append(*ds, syntax.Diagnostic{Code: code, Severity: severity, Message: msg, Span: sp})
}

func sourceID(path string) string {
	h := sha256.Sum256([]byte(strings.ReplaceAll(path, "\\", "/")))
	return hex.EncodeToString(h[:16])
}
func contentHash(src []byte) string {
	h := sha256.Sum256(src)
	return "sha256:" + hex.EncodeToString(h[:])
}
func nodeID(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:16])
}

func parseText(s *syntax.Statement, key string) (string, bool) {
	t := s.Tokens()
	if len(t) != 2 || t[0].Value != key || (t[1].Kind != syntax.String && t[1].Kind != syntax.TripleString) || !s.Terminated {
		return "", false
	}
	return t[1].Value, true
}

func words(v []string) string { return strings.Join(v, " ") }
func shapeError(ds *[]syntax.Diagnostic, s *syntax.Statement, want string) {
	diag(ds, "WBO-E001-SYNTAX", "错误", fmt.Sprintf("%s 语句形状无效，应为 %s", s.Word(0), want), s.Span)
}
