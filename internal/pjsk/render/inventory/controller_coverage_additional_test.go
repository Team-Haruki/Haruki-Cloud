package inventory

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

func TestInventoryRenderAndControllerLifecycle(t *testing.T) {
	profile := &drawing.DetailedProfileCardRequest{ID: "1", Region: "JP", Nickname: "Tester", LeaderImagePath: "leader.png"}
	snap := &inventorySnapshotStub{raw: &snapshot.RawUserData{UserGamedata: snapshot.RawUserGamedata{Coin: 10}}, profile: profile}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("inventory-image"))
	}))
	defer server.Close()
	controller := NewController(drawing.NewHarukiDrawingClient(server.URL), assets.NewAssetHelper("", nil), snap, renderregion.JP, MasterdataOptions{})
	got, err := controller.RenderList(Query{Region: renderregion.JP})
	if err != nil || !bytes.Equal(got, []byte("inventory-image")) {
		t.Fatalf("RenderList() = %q, %v", got, err)
	}
	if clone := controller.WithContext(context.Background()); clone == nil || clone == controller {
		t.Fatal("WithContext did not clone the controller")
	}
	if (*Controller)(nil).WithContext(context.Background()) != nil {
		t.Fatal("nil controller WithContext returned a controller")
	}
	if _, err := (*Controller)(nil).BuildListRequestFromSnapshot(Query{}); err == nil {
		t.Fatal("nil controller built an inventory request")
	}
	if _, err := NewController(nil, nil, nil, renderregion.JP, MasterdataOptions{}).BuildListRequestFromSnapshot(Query{}); err == nil {
		t.Fatal("missing snapshot built an inventory request")
	}
	if _, err := NewController(nil, nil, nil, renderregion.JP, MasterdataOptions{}).RenderList(Query{}); err == nil {
		t.Fatal("missing drawing client rendered inventory")
	}
	badController := NewController(drawing.NewHarukiDrawingClient(server.URL), nil, &inventorySnapshotStub{}, renderregion.JP, MasterdataOptions{})
	if _, err := badController.RenderList(Query{}); err == nil {
		t.Fatal("invalid snapshot rendered inventory")
	}
	(*Controller)(nil).ResetMasterdataCache()
	(&Controller{}).ResetMasterdataCache()
}

func TestInventoryCategoryAndAssetHelperBranches(t *testing.T) {
	for name, tc := range map[string]struct {
		typ  string
		name string
		want string
	}{
		"currency": {typ: "coin", want: "currency"},
		"boost":    {typ: "boost_item", want: "boost"},
		"costume":  {typ: "costume_material", want: "costume"},
		"music":    {typ: "vocal_card", want: "music"},
		"ticket":   {name: "招募券", want: "tickets"},
		"event":    {name: "活动交换所", want: "event"},
		"training": {typ: "master_lesson", want: "training"},
		"basic":    {typ: "gem", want: "basic"},
		"other":    {typ: "mystery", want: "other"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := inventoryCategoryForMaterial(tc.typ, tc.name); got != tc.want {
				t.Fatalf("category = %q, want %q", got, tc.want)
			}
		})
	}

	controller := NewController(nil, assets.NewAssetHelper("", nil), nil, renderregion.JP, MasterdataOptions{})
	for _, tc := range []struct {
		resource string
		id       int
		wantPath bool
	}{
		{resource: "coin", wantPath: true},
		{resource: "virtual_coin", wantPath: true},
		{resource: "jewel", wantPath: true},
		{resource: "material", id: 1, wantPath: true},
		{resource: "boost_item", id: 1, wantPath: true},
		{resource: "practice_ticket", id: 1, wantPath: true},
		{resource: "skill_practice_ticket", id: 1, wantPath: true},
		{resource: "material", id: 0},
		{resource: "unknown", id: 1},
	} {
		got := controller.inventoryIconPath(renderregion.JP, tc.resource, tc.id)
		if (got != "") != tc.wantPath {
			t.Errorf("inventoryIconPath(%q, %d) = %q", tc.resource, tc.id, got)
		}
	}
	for _, resource := range []string{"event_item", "gacha_ticket", "gacha_ceil_item", "mysekai_material"} {
		if got := controller.inventoryIconByAssetName(renderregion.JP, resource, "asset_name"); got == "" {
			t.Errorf("inventoryIconByAssetName(%q) is empty", resource)
		}
	}
	if controller.inventoryIconByAssetName(renderregion.JP, "event_item", " ") != "" ||
		controller.inventoryIconByAssetName(renderregion.JP, "unknown", "asset") != "" {
		t.Fatal("invalid asset name or resource returned a path")
	}
}

func TestInventoryRegionAndSectionHelperBranches(t *testing.T) {
	for region, want := range map[renderregion.Value]string{
		renderregion.JP: "haruki-sekai-master",
		renderregion.CN: "haruki-sekai-sc-master",
		renderregion.TW: "haruki-sekai-tc-master",
		renderregion.KR: "haruki-sekai-kr-master",
		renderregion.EN: "haruki-sekai-en-master",
	} {
		if got := masterdataRepoName(region); got != want {
			t.Errorf("masterdataRepoName(%s) = %q", region, got)
		}
	}
	if masterdataRepoName(renderregion.Value("xx")) != "" {
		t.Fatal("unknown region returned a repository")
	}
	sections := buildInventorySections([]drawing.InventoryItem{
		{ID: 2, Name: "B", Category: "", Quantity: 1, Seq: 2},
		{ID: 1, Name: "A", Category: "other", Quantity: 1, Seq: 1},
		{ID: 3, Category: "basic", Quantity: -1},
	})
	if len(sections) != 1 || sections[0].Key != "other" || len(sections[0].Items) != 2 || sections[0].Items[0].ID != 1 {
		t.Fatalf("inventory sections = %#v", sections)
	}
	if fallbackSeq(8, 9) != 8 || fallbackSeq(0, 9) != 9 {
		t.Fatal("fallback sequence failed")
	}
}
