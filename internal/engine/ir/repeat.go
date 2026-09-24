package ir

import "encoding/json"

func (e RepeatEffect) MarshalJSON() ([]byte, error) {
	times, err := numericValue(e.Times, e.TimesExpr, false)
	if err != nil {
		return nil, err
	}
	object := effectObject(e.NodeBase, e.Kind)
	object["times"], object["body"] = times, e.Body
	return json.Marshal(object)
}
