//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package costume

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderassets "haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestCostumeControllerConstructionGapBranches(t *testing.T) {
	controller := NewController(nil, nil, nil)
	if controller == nil || controller.assets == nil {
		t.Fatal("controller did not create a default asset helper")
	}

	var nilController *Controller
	nilController.RegisterSource(nil)
	nilController.Set3DPreviewConfig(Preview3DConfig{})
	if nilController.default3DPreviewStaticOutputDir("ignored") != "" {
		t.Fatal("nil controller resolved a preview output directory")
	}
	if nilController.WithContext(context.Background()) != nil {
		t.Fatal("nil controller produced a contextual clone")
	}

	for _, primary := range []string{"", ".", "https://assets.example.test"} {
		candidate := &Controller{assets: renderassets.NewAssetHelper(primary, nil)}
		if got := candidate.default3DPreviewStaticOutputDir("preview"); got != "" {
			t.Fatalf("primary %q produced output directory %q", primary, got)
		}
	}
	root := t.TempDir()
	local := &Controller{assets: renderassets.NewAssetHelper(root, nil)}
	if got, want := local.default3DPreviewStaticOutputDir(""), filepath.Join(root, defaultPreview3DStaticRelativeDir); got != want {
		t.Fatalf("default output directory = %q, want %q", got, want)
	}
	if got, want := local.default3DPreviewStaticOutputDir("custom/path"), filepath.Join(root, "custom", "path"); got != want {
		t.Fatalf("custom output directory = %q, want %q", got, want)
	}

	nonContextual := NewController(denseListTestSource{}, nil, nil)
	clone := nonContextual.WithContext(context.Background())
	if clone == nil || len(clone.sources.OrderedSources()) == 0 {
		t.Fatal("non-contextual source was not retained in clone")
	}
}

func TestCostumeListBuildGapBranches(t *testing.T) {
	controller := &Controller{}
	hair := &costumeListBuild{controller: controller, query: ListQuery{PartType: "hair", Character3DID: 1}}
	if err := hair.prepareItems(); err == nil {
		t.Fatal("hair preparation succeeded without preview service")
	}
	head := &costumeListBuild{controller: controller, query: ListQuery{PartType: "head"}}
	if err := head.prepareItems(); err == nil {
		t.Fatal("accessory preparation succeeded without preview service")
	}

	base, heads, hairs := splitCostumeListItems([]*masterdata.Costume3d{
		nil,
		{ID: 1, PartType: "body"},
		{ID: 2, PartType: "head"},
		{ID: 3, PartType: "hair"},
	})
	if len(base) != 1 || len(heads) != 1 || len(hairs) != 1 {
		t.Fatalf("split items = %d/%d/%d", len(base), len(heads), len(hairs))
	}

	filterBuild := &costumeListBuild{query: ListQuery{AccessoryIDs: []int{1}}}
	if err := filterBuild.applyAccessoryIDFilter(); err == nil {
		t.Fatal("accessory filter succeeded outside accessory mode")
	}
	filterBuild.accessoryListMode = true
	filterBuild.accessoryItems = []costumeAccessoryListItem{{accessoryID: 1}, {accessoryID: 2}}
	if err := filterBuild.applyAccessoryIDFilter(); err != nil || len(filterBuild.accessoryItems) != 1 {
		t.Fatalf("accessory filter result = %+v, %v", filterBuild.accessoryItems, err)
	}

	pageBuild := &costumeListBuild{
		query: ListQuery{Page: 10, PageSize: 1},
		items: []*masterdata.Costume3d{{ID: 1}},
	}
	if page := pageBuild.paginate(); page.number != page.totalPages {
		t.Fatalf("out-of-range page = %+v", page)
	}
	if got := boundedCostumePageSize(MaxPageSize + 1); got != MaxPageSize {
		t.Fatalf("bounded page size = %d", got)
	}
}

func TestCostumeDetailGapBranches(t *testing.T) {
	controller := &Controller{ctx: context.Background()}
	basic := &drawing.CostumeBasic{}
	body := &masterdata.Costume3d{ID: 1, PartType: "body", CharacterID: 2}
	if err := controller.applyCostumeDetailBodyRole(renderregion.JP, body, Query{Character3DID: 3}, basic); err != nil {
		t.Fatalf("body role without preview = %v", err)
	}
	if err := controller.applyCostumeDetailAccessoryRole(renderregion.JP, &masterdata.Costume3d{ID: 2, PartType: "head"}, Query{Character3DID: 3}, basic); err == nil {
		t.Fatal("accessory role succeeded without preview")
	}
	if err := controller.applyCostumeDetailHairRole(renderregion.JP, &masterdata.Costume3d{ID: 3, PartType: "hair"}, Query{Character3DID: 3}, basic); err == nil {
		t.Fatal("hair role succeeded without preview")
	}
	if err := selectCostumeDetailAccessory([]int{10}, 2, Query{Character3DID: 3, AccessoryID: 11}, basic); err == nil {
		t.Fatal("invalid explicit accessory was accepted")
	}
	if err := selectCostumeDetailAccessory([]int{10, 11}, 2, Query{Character3DID: 3}, basic); err == nil {
		t.Fatal("ambiguous accessory was accepted")
	}
	if err := selectCostumeDetailAccessory([]int{10}, 2, Query{Character3DID: 3}, basic); err != nil || basic.AccessoryID != 10 {
		t.Fatalf("single accessory selection = %+v, %v", basic, err)
	}

	if path, err := controller.resolve3DPreviewPath(nil, renderregion.JP, body, Query{}); err != nil || path != "" {
		t.Fatalf("preview path without service = %q, %v", path, err)
	}
	if err := controller.ensure3DPreviewCapture(nil, renderregion.JP, body, Query{}); err != nil {
		t.Fatalf("preview capture without service = %v", err)
	}
	if _, err := controller.resolveNormalizedCostume(renderregion.JP, nil, Query{Character3DID: 99, OutfitID: 1}); err == nil {
		t.Fatal("invalid 3D character ID was accepted")
	}
	if _, err := controller.resolveNormalizedHair(renderregion.JP, nil, Query{HairID: 1}); err == nil {
		t.Fatal("normalized hair resolved without preview")
	}
	if _, err := controller.resolveNormalizedAccessory(renderregion.JP, nil, Query{AccessoryID: 1}, 1); err == nil {
		t.Fatal("normalized accessory resolved without preview")
	}
}

func TestCostumeLookupGapBranches(t *testing.T) {
	wantErr := errors.New("filter failed")
	source := &controllerCoverageSource{filterErr: wantErr}
	controller := &Controller{ctx: context.Background()}
	if _, err := controller.resolveNormalizedOutfit(renderregion.JP, source, Query{OutfitID: 1, Character3DID: 1}, 1, 1); !errors.Is(err, wantErr) {
		t.Fatalf("outfit filter error = %v", err)
	}

	source.filterErr = nil
	source.filtered = []*masterdata.Costume3d{{ID: 1, PartType: "body", CharacterID: 1, ColorID: 1}}
	if _, err := controller.resolveNormalizedOutfit(renderregion.JP, source, Query{OutfitID: 999, Character3DID: 1}, 1, 1); err == nil {
		t.Fatal("missing normalized outfit resolved")
	}

	setCostumeDetailPreviewPath("invalid", "preview.png")
	setCostumeDetailPreviewPath(map[string]any{}, "preview.png")
	root := map[string]any{"costume": map[string]any{}}
	setCostumeDetailPreviewPath(root, "preview.png")
	if root["costume"].(map[string]any)["preview_image_path"] != "preview.png" {
		t.Fatalf("preview path was not set: %#v", root)
	}

	lookup := &singleCostumeLookup{controller: controller, named: false}
	if err := lookup.loadLogicalIDs(); err != nil || lookup.logicalIDs == nil {
		t.Fatalf("unnamed logical IDs = %#v, %v", lookup.logicalIDs, err)
	}
	lookup.named = true
	lookup.partType = "head"
	if err := lookup.loadLogicalIDs(); err == nil {
		t.Fatal("named head lookup succeeded without preview")
	}
	lookup.partType = "body"
	if err := lookup.loadLogicalIDs(); err != nil {
		t.Fatalf("named body lookup without preview = %v", err)
	}
	lookup.items = []*masterdata.Costume3d{{ID: 2}, {ID: 1}}
	lookup.logicalIDs = map[int][]int{1: {7}, 2: {7}}
	if item, err := lookup.resolveNamed(); err != nil || item.ID != 1 {
		t.Fatalf("named duplicate logical ID result = %+v, %v", item, err)
	}
}
