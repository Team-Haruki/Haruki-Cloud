package mysekai

import (
	"reflect"
	"testing"
)

func TestBuildScenePreviewLayoutFlattensMySekaiPayload(t *testing.T) {
	merged := map[string]any{
		"updatedResources": map[string]any{
			"userMysekaiGamedata": map[string]any{"mysekaiRank": 12},
			"userMysekaiSiteHousingLayouts": []any{
				map[string]any{"mysekaiSiteId": 2},
			},
			"userMysekaiGates": []any{
				map[string]any{"mysekaiGateId": 3, "mysekaiGateLevel": 9},
			},
		},
	}

	got, err := buildScenePreviewLayout(merged)
	if err != nil {
		t.Fatalf("buildScenePreviewLayout() error = %v", err)
	}
	if rank := intNumber(got["mysekaiRank"], 0); rank != 12 {
		t.Fatalf("mysekaiRank = %d", rank)
	}
	if layouts := nestedList(got, "userMysekaiSiteHousingLayouts"); len(layouts) != 1 {
		t.Fatalf("layout count = %d", len(layouts))
	}
	gate, ok := got["userMysekaiGate"].(map[string]any)
	if !ok || intNumber(gate["mysekaiGateId"], 0) != 3 {
		t.Fatalf("userMysekaiGate = %+v", got["userMysekaiGate"])
	}
}

func TestNormalizeScenePreviewSiteIDs(t *testing.T) {
	got, err := normalizeScenePreviewSiteIDs(nil)
	if err != nil {
		t.Fatalf("normalize default: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1, 2, 3, 4}) {
		t.Fatalf("default sites = %+v", got)
	}

	got, err = normalizeScenePreviewSiteIDs([]int{1, 2, 1, 3})
	if err != nil {
		t.Fatalf("normalize explicit: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("explicit sites = %+v", got)
	}
}

func TestFilterScenePreviewAvailableSites(t *testing.T) {
	layout := map[string]any{
		"userMysekaiSiteHousingLayouts": []any{
			map[string]any{"mysekaiSiteId": 1},
			map[string]any{"mysekaiSiteId": 2},
			map[string]any{"mysekaiSiteId": 3},
		},
	}

	got := filterScenePreviewAvailableSites(layout, []int{1, 2, 3, 4})
	if !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("available sites = %+v", got)
	}
}

func TestScenePreviewCardMasterdataIsDBMapped(t *testing.T) {
	if got := fileToTable["cards.json"]; got != "cards" {
		t.Fatalf("cards.json table mapping = %q", got)
	}
	for file, table := range map[string]string{
		"mysekaiCustomFixtures.json": "mysekaicustomfixtures",
		"mysekaiRankReleases.json":   "mysekairankreleases",
		"mysekaiSiteLevels.json":     "mysekaisitelevels",
		"mysekaiSiteLayouts.json":    "mysekaisitelayouts",
	} {
		if got := fileToTable[file]; got != table {
			t.Fatalf("%s table mapping = %q", file, got)
		}
	}
}
