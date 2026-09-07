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
	for _, comment := range f.TrailingComments {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(comment)
	}
	return []byte(b.String())
}

func formatStatement(b *strings.Builder, s *Statement, indent int) {
	prefix := strings.Repeat(" ", indent)
	line := ""
	lineNeedsPrefix := true
	afterBlock := false
	flushLine := func(newline bool) {
		if line == "" {
			return
		}
		if lineNeedsPrefix {
			b.WriteString(prefix)
		}
		b.WriteString(line)
		line = ""
		lineNeedsPrefix = true
		if newline {
			b.WriteByte('\n')
		}
	}
	writeComments := func(comments []string) {
		if len(comments) == 0 {
			return
		}
		flushLine(true)
		if afterBlock {
			b.WriteByte('\n')
			afterBlock = false
		}
		for _, comment := range comments {
			b.WriteString(prefix)
			b.WriteString(comment)
			b.WriteByte('\n')
		}
	}
	for _, e := range s.Elements {
		if e.Token != nil {
			writeComments(e.Token.LeadingComments)
			text := formattedToken(*e.Token)
			if afterBlock {
				b.WriteByte(' ')
				afterBlock = false
				lineNeedsPrefix = false
			}
			line = addToken(line, text, e.Token.Value, indent)
			continue
		}
		writeComments(e.LeadingComments)
		if line != "" {
			flushLine(false)
			b.WriteString(" {")
		} else {
			b.WriteString(prefix)
			b.WriteByte('{')
		}
		b.WriteByte('\n')
		wroteContent := false
		for _, child := range e.Block {
			if wroteContent {
				b.WriteByte('\n')
			}
			formatStatement(b, child, indent+4)
			wroteContent = true
		}
		for _, comment := range e.TrailingComments {
			if wroteContent {
				b.WriteByte('\n')
			}
			b.WriteString(strings.Repeat(" ", indent+4))
			b.WriteString(comment)
			wroteContent = true
		}
		if wroteContent {
			b.WriteByte('\n')
		}
		b.WriteString(strings.Repeat(" ", indent))
		b.WriteByte('}')
		afterBlock = true
	}
	writeComments(s.TerminatorComments)
	flushLine(false)
	if s.Terminated {
		if !afterBlock && line == "" && len(s.TerminatorComments) > 0 {
			b.WriteString(prefix)
		}
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
	noBefore := value == "." || value == "/" || value == "," || value == "]" || value == "(" || value == ")"
	noAfterPrev := last == "." || last == "/" || last == "[" || last == "(" || last == "+" || last == "-"
	if noBefore || noAfterPrev || (value == "+" || value == "-") {
		return line + text
	}
	return line + " " + text
}
