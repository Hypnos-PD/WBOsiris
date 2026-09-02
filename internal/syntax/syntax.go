package syntax

import "fmt"

type Position struct {
	Byte   int `json:"byte"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

type Span struct {
	File  string   `json:"file,omitempty"`
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Span     Span   `json:"span"`
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s:%d:%d: %s %s: %s", d.Span.File, d.Span.Start.Line, d.Span.Start.Column, d.Severity, d.Code, d.Message)
}

type Kind int

const (
	EOF Kind = iota
	Identifier
	Integer
	String
	TripleString
	Symbol
)

type Token struct {
	Kind            Kind
	Raw             string
	Value           string
	Span            Span
	LeadingComments []string
}

type Element struct {
	Token            *Token
	Block            []*Statement
	IsBlock          bool
	Span             Span
	LeadingComments  []string
	TrailingComments []string
}

type Statement struct {
	Elements           []Element
	Terminated         bool
	TerminatorComments []string
	Span               Span
}

func (s *Statement) Tokens() []Token {
	var out []Token
	for _, e := range s.Elements {
		if e.Token != nil {
			out = append(out, *e.Token)
		}
	}
	return out
}

func (s *Statement) Word(i int) string {
	t := s.Tokens()
	if i < 0 || i >= len(t) {
		return ""
	}
	return t[i].Value
}

func (s *Statement) Blocks() [][]*Statement {
	var out [][]*Statement
	for _, e := range s.Elements {
		if e.IsBlock {
			out = append(out, e.Block)
		}
	}
	return out
}

type File struct {
	Path             string
	Source           []byte
	Statements       []*Statement
	TrailingComments []string
}
