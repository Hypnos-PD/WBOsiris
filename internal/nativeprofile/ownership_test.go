package nativeprofile

import "testing"

// 收藏是按契约算出来的，不是写死的列表：这条测试钉住"算出来的规模"，顺便证明
// 契约与异画参考指的是同一份 CardMaster。
func TestOwnedCollection(t *testing.T) {
	p, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	ids := p.OwnedBaseCardIDs()
	t.Logf("收藏 %d 张，base %d 个，最小 %d 最大 %d", len(p.Owned()), len(ids), ids[0], ids[len(ids)-1])
	if len(ids) != 810 {
		t.Errorf("base 数应为 810，实际 %d", len(ids))
	}
	if len(p.Owned()) != 1642 {
		t.Errorf("收藏卡数应为 1642，实际 %d", len(p.Owned()))
	}
}
