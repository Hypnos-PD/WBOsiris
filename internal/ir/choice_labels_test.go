package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestModeLabelsRoundTripAndValidation(t *testing.T) {
	for _, labels := range []map[string]string{nil, {"chs": "First", "eng": "First option"}, {"jpn": "First"}, {"chs": ""}, {"chs": " \n"}, {"fra": "First"}} {
		want := ModeEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "mode", Options: []ModeOption{
			{ID: 7, Body: []Effect{}, Origin: testOrigin(), Labels: labels},
			{ID: 2, Body: []Effect{}, Origin: testOrigin()},
		}}
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeEffect(data, map[string]bool{})
		if !ValidChoiceLabels(labels) {
			if err == nil {
				t.Fatal("accepted invalid labels", labels)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got.(ModeEffect).Options[0].Labels, labels) {
			t.Fatal("labels lost in decode", err)
		}
	}
}
