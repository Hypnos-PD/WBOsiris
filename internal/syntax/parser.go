package syntax

type parser struct {
	path        string
	tokens      []Token
	i           int
	diagnostics []Diagnostic
}

func Parse(path string, src []byte) (*File, []Diagnostic) {
	tokens, ds := Lex(path, src)
	p := &parser{path: path, tokens: tokens, diagnostics: ds}
	f := &File{Path: path, Source: src}
	f.Statements = p.statements(false)
	f.TrailingComments = append([]string(nil), p.peek().LeadingComments...)
	return f, p.diagnostics
}

func (p *parser) statements(inBlock bool) []*Statement {
	var out []*Statement
	for p.peek().Kind != EOF {
		if p.peek().Value == "}" {
			if inBlock {
				return out
			}
			p.error(p.peek().Span, "多余的右花括号")
			p.i++
			continue
		}
		s := p.statement()
		if s != nil {
			out = append(out, s)
		}
	}
	if inBlock {
		p.error(p.peek().Span, "块未闭合，缺少 }")
	}
	return out
}

func (p *parser) statement() *Statement {
	start := p.peek().Span.Start
	s := &Statement{}
	for p.peek().Kind != EOF && p.peek().Value != "}" {
		t := p.peek()
		p.i++
		switch t.Value {
		case ";":
			s.Terminated = true
			s.TerminatorComments = append([]string(nil), t.LeadingComments...)
			s.Span = Span{File: p.path, Start: start, End: t.Span.End}
			return s
		case "{":
			body := p.statements(true)
			closing := p.peek()
			end := closing.Span.End
			if p.peek().Value == "}" {
				p.i++
			}
			s.Elements = append(s.Elements, Element{Block: body, IsBlock: true, Span: Span{File: p.path, Start: t.Span.Start, End: end}, LeadingComments: append([]string(nil), t.LeadingComments...), TrailingComments: append([]string(nil), closing.LeadingComments...)})
			if p.peek().Value != "else" {
				s.Span = Span{File: p.path, Start: start, End: end}
				return s
			}
		default:
			tt := t
			s.Elements = append(s.Elements, Element{Token: &tt, Span: t.Span})
		}
	}
	end := p.peek().Span.Start
	if len(s.Elements) == 0 {
		return nil
	}
	p.error(Span{File: p.path, Start: start, End: end}, "语句缺少分号或块")
	s.Span = Span{File: p.path, Start: start, End: end}
	return s
}

func (p *parser) peek() Token {
	if p.i >= len(p.tokens) {
		return Token{Kind: EOF}
	}
	return p.tokens[p.i]
}
func (p *parser) error(sp Span, msg string) {
	p.diagnostics = append(p.diagnostics, Diagnostic{Code: "WBO-E001-SYNTAX", Severity: "错误", Message: msg, Span: sp})
}
