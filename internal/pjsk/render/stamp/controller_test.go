package stamp

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type testStampSource struct {
	region renderregion.Value
	stamps []masterdata.Stamp
}

func newTestStampSource(region renderregion.Value) *testStampSource {
	return &testStampSource{region: region}
}

func (s *testStampSource) DefaultRegion() renderregion.Value { return s.region }

func (s *testStampSource) GetStamps() ([]masterdata.Stamp, error) {
	return append([]masterdata.Stamp(nil), s.stamps...), nil
}

func TestControllerBuildStampListRequestUsesRequestedRegionSource(t *testing.T) {
	cn := newTestStampSource(renderregion.CN)
	cn.stamps = []masterdata.Stamp{{ID: 1, AssetBundleName: "cn_stamp"}}

	jp := newTestStampSource(renderregion.JP)
	jp.stamps = []masterdata.Stamp{{ID: 2, AssetBundleName: "jp_stamp"}}

	controller := NewController(cn, nil, assets.NewAssetHelper("", nil))
	controller.RegisterSource(jp)

	req, err := controller.BuildStampListRequest(ListQuery{Region: renderregion.JP})
	if err != nil {
		t.Fatalf("BuildStampListRequest failed: %v", err)
	}
	if len(req.Stamps) != 1 || req.Stamps[0].ID != 2 {
		t.Fatalf("unexpected stamps: %+v", req.Stamps)
	}
}

func TestControllerBuildStampListRequestFiltersAndUsesPrompt(t *testing.T) {
	dir := t.TempDir()
	mustWriteStampAsset(t, dir, "stamp_a")
	mustWriteStampAsset(t, dir, "stamp_b_rip")

	source := newTestStampSource(renderregion.JP)
	source.stamps = []masterdata.Stamp{
		{ID: 2, AssetBundleName: "stamp_b"},
		{ID: 1, AssetBundleName: "stamp_a"},
		{ID: 3, AssetBundleName: "missing"},
	}

	controller := NewController(source, nil, assets.NewAssetHelper(dir, nil))
	req, err := controller.BuildStampListRequest(ListQuery{
		Region:        renderregion.JP,
		PromptMessage: "自定义提示",
		IDs:           []int{2, 1},
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("BuildStampListRequest failed: %v", err)
	}
	if req.PromptMessage == nil || *req.PromptMessage != "自定义提示" {
		t.Fatalf("unexpected prompt: %#v", req.PromptMessage)
	}
	if len(req.Stamps) != 1 {
		t.Fatalf("expected 1 stamp, got %d", len(req.Stamps))
	}
	if req.Stamps[0].ID != 1 {
		t.Fatalf("expected sorted stamp id 1, got %d", req.Stamps[0].ID)
	}
}

func mustWriteStampAsset(t *testing.T, root, bundle string) {
	t.Helper()
	var rel string
	switch bundle {
	case "stamp_a":
		rel = filepath.Join("stamp", bundle, bundle+".png")
	case "stamp_b_rip":
		rel = filepath.Join("stamp", bundle, "stamp_b.png")
	default:
		t.Fatalf("unknown bundle: %s", bundle)
	}
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte("png"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestControllerBuildStampListRequestFailsWhenNoMatch(t *testing.T) {
	source := newTestStampSource(renderregion.JP)
	source.stamps = []masterdata.Stamp{{ID: 1, AssetBundleName: "missing"}}

	controller := NewController(source, nil, assets.NewAssetHelper("/tmp/non-existent-stamp-root", nil))
	_, err := controller.BuildStampListRequest(ListQuery{Region: renderregion.JP})
	if err == nil {
		t.Fatal("expected error when no assets matched")
	}
}

func (s *testStampSource) String() string {
	return fmt.Sprintf("testStampSource(%s)", s.region)
}
