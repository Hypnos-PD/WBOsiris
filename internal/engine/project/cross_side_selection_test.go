package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// `field.followers` 是双方战场合并后的集合：跨方选择"战场上的1个随从"用它表达，
// `other` 仍然只排除来源实例自身。
func TestCrossSideSelectionKeepsBothSides(t *testing.T) {
	body := compiledFanfare(t, `fanfare {
		choose target from field.followers other;
		buff target +2/-2;
		choose ally from own.field.followers;
		buff ally +1/+1;
	}`)
	if len(body) != 4 {
		t.Fatalf("effects = %d", len(body))
	}
	choose := body[0].(ir.SelectionEffect)
	exclude, ok := choose.Source.(ir.ExcludeRef)
	if !ok {
		t.Fatalf("other did not compile into an exclusion: %#v", choose.Source)
	}
	zone, ok := exclude.Source.(ir.ZoneRef)
	if !ok || zone.Side != "" || zone.Zone != "field" || zone.Member != "follower" {
		t.Fatalf("cross-side set lost its shape: %#v", exclude.Source)
	}
	if _, ok := exclude.Value.(ir.SelfRef); !ok {
		t.Fatalf("other must exclude self: %#v", exclude.Value)
	}
	own := body[2].(ir.SelectionEffect)
	ownZone := own.Source.(ir.ZoneRef)
	if ownZone.Side != "own" || ownZone.Member != "follower" {
		t.Fatalf("own-side set changed: %#v", own.Source)
	}
}
