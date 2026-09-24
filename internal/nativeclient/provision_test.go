package nativeclient

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleLog = `The referenced script on this Behaviour (Game Object 'Title') is missing!
Manifest Load Error:Could not find a part of the path "S:\steamapps\common\ShadowverseWB\ShadowverseWB_Data\StreamingAssets\PreinResource\manifests\assetbundle.Chs.manifest".
Manifest Load Error:Could not find file "S:\steamapps\common\ShadowverseWB\ShadowverseWB_Data\Persistent\dat\RJ\RJKAM6CYLA7Q3GV3746GJZVN2SR3XTYD"
Manifest Load Error:Could not find file "S:\steamapps\common\ShadowverseWB\ShadowverseWB_Data\Persistent\dat\RJ\RJKAM6CYLA7Q3GV3746GJZVN2SR3XTYD"
`

func TestLearnManifestHnameFromPlayerLog(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "Player.log")
	if err := os.WriteFile(log, []byte(sampleLog), 0o644); err != nil {
		t.Fatal(err)
	}
	hname, err := LearnManifestHname(log, Install{})
	if err != nil {
		t.Fatalf("应当从日志里学到名字：%v", err)
	}
	if hname != "RJKAM6CYLA7Q3GV3746GJZVN2SR3XTYD" {
		t.Fatalf("学到的是 %s", hname)
	}
}

func TestLearnManifestHnameRejectsAssetNames(t *testing.T) {
	// 资源（26 位）也会出现在同类报错里，但它不是清单；认成清单会把清单写到错的位置。
	dir := t.TempDir()
	log := filepath.Join(dir, "Player.log")
	line := `Manifest Load Error:Could not find file "S:\steamapps\common\ShadowverseWB\ShadowverseWB_Data\Persistent\dat\4Y\4YHDWV2L4LRFYB2YRWKCJW4EZU"` + "\n"
	if err := os.WriteFile(log, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LearnManifestHname(log, Install{}); err == nil {
		t.Fatal("26 位的资源名不该被当成清单名")
	}
}

func TestLearnManifestHnameWithoutLogExplainsItself(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "Player.log")
	if err := os.WriteFile(log, []byte("nothing here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LearnManifestHname(log, Install{}); err == nil {
		t.Fatal("没有记录时应当报错，而不是返回空名字")
	}
}

// TestProvisionLaysOutBothResourceTrees 钉住客户端实际要的两个位置：
// 清单在 PreinResource/manifests 与 Persistent/dat 各一份，资源本体在
// PreinResource/AssetBundle 里。只铺 dat 是实测走不通的那条路。
func TestProvisionLaysOutBothResourceTrees(t *testing.T) {
	client := t.TempDir()
	source := t.TempDir()
	manifest := source + "/manifests/raw/assetbundle.Chs.manifest"
	blob := source + "/blobs/raw/4Y/4YHDWV2L4LRFYB2YRWKCJW4EZU"
	for _, path := range []string{manifest, blob, client + "/ShadowverseWB_Data/Persistent/dat"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(manifest, []byte("manifest-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blob, []byte("bundle-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := Provision(ProvisionOptions{
		ClientDir:     client,
		Source:        ResourceSource{Root: source},
		Lang:          "Chs",
		ManifestHname: "RJKAM6CYLA7Q3GV3746GJZVN2SR3XTYD",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Bundles != 1 {
		t.Fatalf("应当只铺 1 个资源，报告说 %d", report.Bundles)
	}
	for _, want := range []string{
		client + "/ShadowverseWB_Data/StreamingAssets/PreinResource/manifests/assetbundle.Chs.manifest",
		client + "/ShadowverseWB_Data/Persistent/dat/RJ/RJKAM6CYLA7Q3GV3746GJZVN2SR3XTYD",
		client + "/ShadowverseWB_Data/StreamingAssets/PreinResource/AssetBundle/4Y/4YHDWV2L4LRFYB2YRWKCJW4EZU",
	} {
		raw, err := os.ReadFile(want)
		if err != nil {
			t.Fatalf("缺 %s：%v", want, err)
		}
		if len(raw) == 0 {
			t.Fatalf("%s 是空的", want)
		}
	}

	// 再跑一次不应该重复复制（跳过），否则每次启动都要重铺 26 GiB。
	again, err := Provision(ProvisionOptions{
		ClientDir:     client,
		Source:        ResourceSource{Root: source},
		Lang:          "Chs",
		ManifestHname: "RJKAM6CYLA7Q3GV3746GJZVN2SR3XTYD",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if again.Skipped != 1 {
		t.Fatalf("第二次应当跳过已存在的资源，跳过数 %d", again.Skipped)
	}
}
