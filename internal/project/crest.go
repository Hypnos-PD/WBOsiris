package project

import "wbo/internal/syntax"

func validateCrest(parent *Card, decl *syntax.Statement, ds *[]syntax.Diagnostic) *Card {
	c := &Card{ID: parent.ID, Type: "crest", Path: parent.Path, File: parent.File, Decl: decl, Locales: map[string]Locale{}}
	localeIndex := 0
	countdownSeen, abilitySeen := false, false
	for _, s := range decl.Blocks()[0] {
		if s.Word(0) == "locale" {
			t := s.Tokens()
			if len(t) != 2 || len(s.Blocks()) != 1 || s.Terminated || localeIndex >= len(locales) || s.Word(1) != locales[localeIndex] {
				shapeError(ds, s, "locale chs|eng|jpn|kor|cht { name ...; text ...; }")
				continue
			}
			parseLocale(c, s.Word(1), s.Blocks()[0], ds)
			localeIndex++
			continue
		}
		if localeIndex > 0 || !set("counter", "countdown", "lastwords", "when")[s.Word(0)] {
			shapeError(ds, s, "纹章仅允许计数器、吟唱、谢幕曲和事件监听，之后声明本地化")
			continue
		}
		if s.Word(0) == "when" && (s.Word(1) == "self" || filterContains(s.Tokens(), "while")) {
			shapeError(ds, s, "纹章监听来源固定为主战者区域")
		}
		if s.Word(0) == "countdown" {
			if countdownSeen || len(s.Tokens()) != 2 || intAt(s, 1) < 1 || intAt(s, 1) > 65535 {
				shapeError(ds, s, "纹章只能声明一次 countdown，数值为 1..65535")
			}
			countdownSeen = true
		}
		abilitySeen = abilitySeen || s.Word(0) == "when" || s.Word(0) == "lastwords"
		c.Effect = append(c.Effect, s)
	}
	if localeIndex != len(locales) {
		shapeError(ds, decl, "纹章必须声明五种本地化")
	}
	if !abilitySeen {
		shapeError(ds, decl, "纹章必须声明至少一个能力")
	}
	validateEffectBlock(c.Effect, ds, map[string]bool{"self": true}, "")
	strictValidateCard(c, ds)
	return c
}
