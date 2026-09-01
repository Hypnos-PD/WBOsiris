package syntax

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func Format(f *File) []byte {
	var b strings.Builder
	for i, s := range f.Statements {
		if i > 0 {
			b.WriteByte('\n')
		}
		formatStatement(&b, s, 0)
	}
	return []byte(b.String())
}

func formatStatement(b *strings.Builder, s *Statement, indent int) {
	b.WriteString(strings.Repeat(" ", indent))
	line := ""
	for i, e := range s.Elements {
		if e.Token != nil {
			text := formattedToken(*e.Token)
			line = addToken(line, text, e.Token.Value, indent)
			continue
		}
		b.WriteString(line)
		b.WriteString(" {")
		if len(e.Block) > 0 {
			b.WriteByte('\n')
			for j, child := range e.Block {
				formatStatement(b, child, indent+4)
				if j+1 < len(e.Block) {
					b.WriteByte('\n')
				}
			}
			b.WriteByte('\n')
			b.WriteString(strings.Repeat(" ", indent))
		} else {
			b.WriteByte('\n')
			b.WriteString(strings.Repeat(" ", indent))
		}
		b.WriteByte('}')
		line = ""
		if i+1 < len(s.Elements) {
			b.WriteByte(' ')
		}
	}
	if line != "" {
		b.WriteString(line)
	}
	if s.Terminated {
		b.WriteByte(';')
	}
}

func formattedToken(t Token) string {
	switch t.Kind {
	case String:
		return quoteString(t.Value)
	case TripleString:
		lines := strings.Split(t.Value, "\n")
		return `"""` + "\n" + strings.Join(lines, "\n") + "\n" + `"""`
	default:
		return t.Value
	}
}

func quoteString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f || !utf8.ValidRune(r) {
				fmt.Fprintf(&b, `\u{%x}`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func addToken(line, text, value string, indent int) string {
	if line == "" {
		return text
	}
	if strings.Contains(text, "\n") {
		parts := strings.Split(text, "\n")
		for i := 1; i < len(parts)-1; i++ {
			parts[i] = strings.Repeat(" ", indent+4) + parts[i]
		}
		parts[len(parts)-1] = strings.Repeat(" ", indent) + parts[len(parts)-1]
		return line + " " + strings.Join(parts, "\n")
	}
	last := line[len(line)-1:]
	noBefore := value == "." || value == "/" || value == "," || value == "]"
	noAfterPrev := last == "." || last == "/" || last == "[" || last == "+" || last == "-"
	if noBefore || noAfterPrev || (value == "+" || value == "-") {
		return line + text
	}
	return line + " " + text
}
