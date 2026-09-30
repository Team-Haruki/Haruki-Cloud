package inventory

import (
	"slices"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/storage/storagetest"
)

func TestInventoryFiltersBeforeIconsAndForwardsRemoteCandidates(t *testing.T) {
	store := storagetest.NewMemory()
	helper := assets.NewAssetHelper("", nil).WithStore(store, assets.StoreProbeConfig{}, nil)
	defer helper.Close()
	c := NewController(nil, helper, nil, renderregion.KR, MasterdataOptions{})
	raw := &snapshot.RawUserData{
		UserGamedata:   snapshot.RawUserGamedata{Coin: 10},
		UserMaterials:  []snapshot.RawUserMaterial{{MaterialID: 1, Quantity: 100}},
		UserBoostItems: []snapshot.RawUserBoostItem{{BoostItemID: 2, Quantity: 3}},
	}
	unfiltered := c.inventoryItems(renderregion.KR, raw, emptyRegionMasterdata())
	for _, item := range unfiltered {
		if item.IconPath != nil {
			t.Fatal("unfiltered inventory resolved an icon")
		}
	}
	profile := &drawing.DetailedProfileCardRequest{}
	request, err := c.BuildListRequestFromSnapshot(Query{Region: renderregion.KR, Filter: FilterBoost, Profile: profile, Snapshot: &inventorySnapshotStub{raw: raw, profile: profile}})
	if err != nil {
		t.Fatal(err)
	}
	if request.TotalItems != 1 {
		t.Fatalf("items = %d", request.TotalItems)
	}
	item := request.Sections[0].Items[0]
	want := []string{
		"asset/kr-assets/startapp/thumbnail/boost_item/boost_item2.png",
		"asset/kr-assets/ondemand/thumbnail/boost_item/boost_item2.png",
		"asset/jp-assets/startapp/thumbnail/boost_item/boost_item2.png",
		"asset/jp-assets/ondemand/thumbnail/boost_item/boost_item2.png",
	}
	if !slices.Equal([]string(item.IconPath), want) {
		t.Fatalf("candidate order = %v, want %v", item.IconPath, want)
	}

	if len(store.Calls()) != 0 {
		t.Fatalf("Cloud probed Drawing-only candidates: %v", store.Calls())
	}
}
