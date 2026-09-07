package ir

// GrantEffect gives each selected follower an independent triggered ability.
type GrantEffect struct {
	NodeBase
	Kind    string            `json:"kind"`
	Target  Ref               `json:"target"`
	Ability Ability           `json:"ability"`
	Labels  map[string]string `json:"labels,omitempty"`
}

func (e GrantEffect) effectKind() string   { return e.Kind }
func (e GrantEffect) effectBase() NodeBase { return e.NodeBase }

func ValidGrantedTrigger(t Trigger) bool {
	if t, ok := t.(SimpleTrigger); ok {
		return t.Kind == "lastwords"
	}
	if t, ok := t.(EventTrigger); ok {
		return (t.Event == "turn_started" || t.Event == "turn_ended") && t.SourceZone == "" && !t.SelfOnly && t.SubjectType == "" && t.Predicate == nil
	}
	return false
}

func validGrantedBody(body []Effect) bool {
	for _, effect := range body {
		switch e := effect.(type) {
		case SelectionEffect:
			if e.Kind == "require" {
				return false
			}
		case IfEffect:
			if !validGrantedBody(e.Then) || !validGrantedBody(e.Else) {
				return false
			}
		case RepeatEffect:
			if !validGrantedBody(e.Body) {
				return false
			}
		case ModeEffect:
			for _, option := range e.Options {
				if !validGrantedBody(option.Body) {
					return false
				}
			}
		case PayResourceEffect:
			if !validGrantedBody(e.OnPaid) {
				return false
			}
		}
	}
	return true
}
