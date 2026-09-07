package syntax

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

type lexer struct {
	path            string
	src             []byte
	off             int
	line, col       int
	diagnostics     []Diagnostic
	pendingComments []string
}

func Lex(path string, src []byte) ([]Token, []Diagnostic) {
	l := &lexer{path: path, src: src, line: 1, col: 1}
	if len(src) >= 3 && string(src[:3]) == "\xef\xbb\xbf" {
		l.off = 3
	}
	if !utf8.Valid(src[l.off:]) {
		p := l.pos()
		return nil, []Diagnostic{{Code: "WBO-E001-SYNTAX", Severity: "错误", Message: "文件不是有效的 UTF-8", Span: Span{File: path, Start: p, End: p}}}
	}
	var tokens []Token
	for {
		l.skipSeparators()
		if l.off >= len(l.src) {
			p := l.pos()
			tokens = append(tokens, Token{Kind: EOF, Span: Span{File: path, Start: p, End: p}, LeadingComments: l.takeComments()})
			break
		}
		start := l.pos()
		r, _ := utf8.DecodeRune(l.src[l.off:])
		switch {
		case isLetter(r) || r == '_':
			begin := l.off
			for l.off < len(l.src) {
				r, _ = utf8.DecodeRune(l.src[l.off:])
				if !isLetter(r) && !isDigit(r) && r != '_' {
					break
				}
				l.advanceRune()
			}
			raw := string(l.src[begin:l.off])
			tokens = append(tokens, l.token(Identifier, raw, raw, start))
		case isDigit(r):
			begin := l.off
			for l.off < len(l.src) {
				r, _ = utf8.DecodeRune(l.src[l.off:])
				if !isDigit(r) {
					break
				}
				l.advanceRune()
			}
			raw := string(l.src[begin:l.off])
			tokens = append(tokens, l.token(Integer, raw, raw, start))
		case r == '"':
			tokens = append(tokens, l.scanString(start))
		default:
			matched := ""
			for _, op := range []string{"<=", ">=", "==", "!=", "{", "}", ";", ".", "/", "+", "-", "<", ">", "=", "[", "]", ",", "(", ")"} {
				if strings.HasPrefix(string(l.src[l.off:]), op) {
					matched = op
					break
				}
			}
			if matched == "" {
				l.advanceRune()
				l.error(start, "无法识别的字符 %q", r)
				continue
			}
			for range matched {
				l.advanceRune()
			}
			tokens = append(tokens, l.token(Symbol, matched, matched, start))
		}
	}
	return tokens, l.diagnostics
}

func (l *lexer) scanString(start Position) Token {
	begin := l.off
	if strings.HasPrefix(string(l.src[l.off:]), `"""`) {
		l.advanceRune()
		l.advanceRune()
		l.advanceRune()
		content := l.off
		for l.off < len(l.src) && !strings.HasPrefix(string(l.src[l.off:]), `"""`) {
			l.advanceRune()
		}
		if l.off >= len(l.src) {
			l.error(start, "三引号字符串未闭合")
			return l.token(TripleString, string(l.src[begin:l.off]), dedentTriple(string(l.src[content:l.off])), start)
		}
		value := dedentTriple(string(l.src[content:l.off]))
		l.advanceRune()
		l.advanceRune()
		l.advanceRune()
		return l.token(TripleString, string(l.src[begin:l.off]), value, start)
	}
	l.advanceRune()
	var b strings.Builder
	valid := true
	closed := false
	for l.off < len(l.src) {
		r, _ := utf8.DecodeRune(l.src[l.off:])
		if r == '"' {
			l.advanceRune()
			closed = true
			break
		}
		if r == '\n' || r == '\r' {
			l.error(start, "普通字符串不能跨行")
			valid = false
			break
		}
		if r != '\\' {
			b.WriteRune(r)
			l.advanceRune()
			continue
		}
		l.advanceRune()
		if l.off >= len(l.src) {
			break
		}
		e, _ := utf8.DecodeRune(l.src[l.off:])
		l.advanceRune()
		switch e {
		case '"', '\\':
			b.WriteRune(e)
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'u':
			if l.off >= len(l.src) || l.src[l.off] != '{' {
				l.error(start, "无效的 Unicode 转义")
				valid = false
				continue
			}
			l.advanceRune()
			hs := l.off
			for l.off < len(l.src) && isHex(rune(l.src[l.off])) {
				l.advanceRune()
			}
			hex := string(l.src[hs:l.off])
			if len(hex) < 1 || len(hex) > 6 || l.off >= len(l.src) || l.src[l.off] != '}' {
				l.error(start, "Unicode 转义必须包含 1 至 6 个十六进制数字")
				valid = false
				continue
			}
			l.advanceRune()
			n, _ := strconv.ParseUint(hex, 16, 32)
			rr := rune(n)
			if !utf8.ValidRune(rr) || rr >= 0xd800 && rr <= 0xdfff {
				l.error(start, "Unicode 转义不是标量值")
				valid = false
			} else {
				b.WriteRune(rr)
			}
		default:
			l.error(start, "不支持的转义 \\%c", e)
			valid = false
		}
	}
	if !closed && valid {
		l.error(start, "字符串未闭合")
	}
	return l.token(String, string(l.src[begin:l.off]), b.String(), start)
}

func dedentTriple(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if strings.HasPrefix(s, "\n") {
		s = s[1:]
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	prefix := ""
	set := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		p := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if !set {
			prefix, set = p, true
		} else {
			for !strings.HasPrefix(p, prefix) {
				prefix = prefix[:len(prefix)-1]
			}
		}
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
		} else if strings.HasPrefix(line, prefix) {
			lines[i] = line[len(prefix):]
		}
	}
	return strings.Join(lines, "\n")
}

func (l *lexer) skipSeparators() {
	for l.off < len(l.src) {
		r, _ := utf8.DecodeRune(l.src[l.off:])
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			l.advanceRune()
			continue
		}
		if strings.HasPrefix(string(l.src[l.off:]), "<<") || strings.HasPrefix(string(l.src[l.off:]), "//") {
			begin := l.off
			for l.off < len(l.src) {
				r, _ = utf8.DecodeRune(l.src[l.off:])
				if r == '\r' || r == '\n' {
					break
				}
				l.advanceRune()
			}
			l.pendingComments = append(l.pendingComments, strings.TrimSpace(string(l.src[begin:l.off])))
			continue
		}
		if strings.HasPrefix(string(l.src[l.off:]), "/*") {
			start := l.pos()
			l.advanceRune()
			l.advanceRune()
			depth := 1
			for l.off < len(l.src) && depth > 0 {
				if strings.HasPrefix(string(l.src[l.off:]), "/*") {
					l.advanceRune()
					l.advanceRune()
					depth++
				} else if strings.HasPrefix(string(l.src[l.off:]), "*/") {
					l.advanceRune()
					l.advanceRune()
					depth--
				} else {
					l.advanceRune()
				}
			}
			if depth > 0 {
				l.error(start, "块注释未闭合")
			}
			continue
		}
		break
	}
}

func (l *lexer) pos() Position { return Position{Byte: l.off, Line: l.line, Column: l.col} }
func (l *lexer) advanceRune() {
	r, n := utf8.DecodeRune(l.src[l.off:])
	l.off += n
	if r == '\r' {
		if l.off < len(l.src) && l.src[l.off] == '\n' {
			l.off++
		}
		l.line++
		l.col = 1
	} else if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
}
func (l *lexer) token(k Kind, raw, value string, start Position) Token {
	return Token{Kind: k, Raw: raw, Value: value, Span: Span{File: l.path, Start: start, End: l.pos()}, LeadingComments: l.takeComments()}
}

func (l *lexer) takeComments() []string {
	comments := l.pendingComments
	l.pendingComments = nil
	return comments
}
func (l *lexer) error(start Position, f string, a ...any) {
	l.diagnostics = append(l.diagnostics, Diagnostic{Code: "WBO-E001-SYNTAX", Severity: "错误", Message: fmt.Sprintf(f, a...), Span: Span{File: l.path, Start: start, End: l.pos()}})
}
func isLetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }
func isDigit(r rune) bool  { return r >= '0' && r <= '9' }
func isHex(r rune) bool    { return isDigit(r) || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' }
