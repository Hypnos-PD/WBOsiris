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
		// 回合开始/结束监听保持原有的"无筛选、无自身限制"形状；
		// 另外允许把事件监听（如"自己的随从进化时""通过爆能强化使用卡牌时"）附加给信仰等永久实体。
		if t.Event == "turn_started" || t.Event == "turn_ended" {
			return t.OncePerTurn == "" && t.DuringTurn == "" && t.SourceZone == "" && !t.SelfOnly && t.SubjectType == "" && t.Predicate == nil && t.Condition == nil
		}
		return !t.SelfOnly && !t.ExcludeSelf
	}
	return false
}

// ValidSuppressedAbility 列出可以被"失去能力"操作移除的触发能力种类。
// 目前只有谢幕曲需要（腐臭的僵尸等衍生体），"all" 由编译器单独映射。
func ValidSuppressedAbility(kind string) bool {
	return kind == "lastwords"
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
