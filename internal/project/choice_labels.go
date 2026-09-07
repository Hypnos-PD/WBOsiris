package project

import (
	"fmt"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func modeOptionParts(body []*syntax.Statement) (map[string]string, []*syntax.Statement, error) {
	var labels map[string]string
	for n, statement := range body {
		if statement.Word(0) != "label" {
			return labels, body[n:], nil
		}
		tokens := statement.Tokens()
		if len(tokens) != 3 || tokens[1].Kind != syntax.Identifier || tokens[2].Kind != syntax.String && tokens[2].Kind != syntax.TripleString || !statement.Terminated || len(statement.Blocks()) != 0 {
			return nil, nil, fmt.Errorf("label 语言 字符串; 必须位于 option 效果之前")
		}
		if labels == nil {
			labels = map[string]string{}
		}
		if _, exists := labels[tokens[1].Value]; exists {
			return nil, nil, fmt.Errorf("同一 option 的 label 语言不能重复")
		}
		labels[tokens[1].Value] = tokens[2].Value
		if !ir.ValidChoiceLabels(labels) {
			return nil, nil, fmt.Errorf("label 语言必须为 chs/eng/jpn/kor/cht，文本不能为空")
		}
	}
	return labels, nil, nil
}
