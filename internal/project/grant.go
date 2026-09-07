package project

import (
	"fmt"
	"wbo/internal/syntax"
)

func grantParts(s *syntax.Statement) (map[string]string, *syntax.Statement, error) {
	if len(s.Blocks()) != 1 {
		return nil, nil, fmt.Errorf("grant 必须包含一个能力块")
	}
	labels, body, err := modeOptionParts(s.Blocks()[0])
	if err != nil {
		return nil, nil, err
	}
	if len(body) != 1 {
		return nil, nil, fmt.Errorf("grant 必须包含一个触发能力")
	}
	return labels, body[0], nil
}
