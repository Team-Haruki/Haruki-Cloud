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

func TestControllerBuildStampListRequestPaginatesAtFiveByFive(t *testing.T) {
	dir := t.TempDir()
	source := newTestStampSource(renderregion.JP)
	for i := 1; i <= 26; i++ {
		bundle := fmt.Sprintf("stamp_%02d", i)
		mustWriteNamedStampAsset(t, dir, bundle)
		source.stamps = append(source.stamps, masterdata.Stamp{ID: i, AssetBundleName: bundle})
	}

	controller := NewController(source, nil, assets.NewAssetHelper(dir, nil))
	req, err := controller.BuildStampListRequest(ListQuery{Region: renderregion.JP})
	if err != nil {
		t.Fatalf("BuildStampListRequest failed: %v", err)
	}
	if len(req.Stamps) != 25 {
		t.Fatalf("expected first page size 25, got %d", len(req.Stamps))
	}
	if req.PageMessage == nil || *req.PageMessage != "第 1 / 2 页" {
		t.Fatalf("unexpected first page message: %#v", req.PageMessage)
	}
}

func TestControllerBuildStampListRequestsSupportsSpecificPageAndAll(t *testing.T) {
	dir := t.TempDir()
	source := newTestStampSource(renderregion.JP)
	for i := 1; i <= 30; i++ {
		bundle := fmt.Sprintf("stamp_page_%02d", i)
		mustWriteNamedStampAsset(t, dir, bundle)
		source.stamps = append(source.stamps, masterdata.Stamp{ID: i, AssetBundleName: bundle})
	}

	controller := NewController(source, nil, assets.NewAssetHelper(dir, nil))

	secondPage, err := controller.BuildStampListRequest(ListQuery{Region: renderregion.JP, Page: 2})
	if err != nil {
		t.Fatalf("BuildStampListRequest(page=2) failed: %v", err)
	}
	if len(secondPage.Stamps) != 5 || secondPage.Stamps[0].ID != 26 {
		t.Fatalf("unexpected second page content: %+v", secondPage.Stamps)
	}

	allPages, err := controller.BuildStampListRequests(ListQuery{Region: renderregion.JP, All: true})
	if err != nil {
		t.Fatalf("BuildStampListRequests(all) failed: %v", err)
	}
	if len(allPages) != 2 {
		t.Fatalf("expected 2 pages in all mode, got %d", len(allPages))
	}
}

func mustWriteStampAsset(t *testing.T, root, bundle string) {
	t.Helper()
	var rel string
	switch bundle {
	case "stamp_a":
		rel = filepath.Join("asset", "jp-assets", "startapp", "stamp", bundle, bundle+".png")
	case "stamp_b_rip":
		rel = filepath.Join("asset", "jp-assets", "startapp", "stamp", bundle, "stamp_b.png")
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

func mustWriteNamedStampAsset(t *testing.T, root, bundle string) {
	t.Helper()
	full := filepath.Join(root, "asset", "jp-assets", "startapp", "stamp", bundle, bundle+".png")
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
