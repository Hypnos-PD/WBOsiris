package runner

import "wbo/internal/ir"

func copyRepeatBindings(parent frame) frame {
	bindings := frame{}
	for name, items := range parent {
		bindings[name] = append([]ir.EventTarget(nil), items...)
	}
	return bindings
}

func (s *Session) insideRepeat() bool {
	for _, f := range s.stack {
		if f.repeatRemaining > 0 {
			return true
		}
	}
	return false
}
