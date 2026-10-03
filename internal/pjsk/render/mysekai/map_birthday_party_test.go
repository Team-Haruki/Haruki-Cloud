package mysekai

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
)

// Second-year parties no longer fit the 174-199 delivery range: Haruka 2026
// drops material 297 from fixture 8027. The map has to follow
// birthdayParties.json or the plant falls back to the tree icon.
func TestBuildMapRequestUsesBirthdayPartyMasterdata(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiSiteHarvestFixtures.json"), []map[string]any{
		{"id": 2001, "assetbundleName": "mdl_site_rock_common_stone01", "mysekaiSiteHarvestFixtureType": "mineral", "mysekaiSiteHarvestFixtureRarityType": "rarity_1"},
		{"id": 2002, "assetbundleName": "mdl_site_rock_common_stone02", "mysekaiSiteHarvestFixtureType": "mineral", "mysekaiSiteHarvestFixtureRarityType": "rarity_1"},
		{"id": 8027, "assetbundleName": "mdl_site_dewdrop_birthday_plant106", "mysekaiSiteHarvestFixtureType": "birthday_plant", "mysekaiSiteHarvestFixtureRarityType": "rarity_2"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "birthdayParties.json"), []map[string]any{
		{"id": 1, "gameCharacterUnitId": 6, "deliveryItemMaterialId": 179, "assetbundleName": "haruka_2025", "mysekaiSiteHarvestFixtureId": 8001},
		{"id": 27, "gameCharacterUnitId": 6, "deliveryItemMaterialId": 297, "assetbundleName": "haruka_2026", "mysekaiSiteHarvestFixtureId": 8027},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacterUnits.json"), []map[string]any{{"id": 6, "gameCharacterId": 6}})
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacters.json"), []map[string]any{{"id": 6, "givenNameEnglish": "Haruka"}})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMaterials.json"), []map[string]any{
		{"id": 6, "iconAssetbundleName": "item_stone", "mysekaiMaterialRarityType": "rarity_1"},
		{"id": 17, "iconAssetbundleName": "item_battery", "mysekaiMaterialRarityType": "rarity_1"},
		{"id": 297, "iconAssetbundleName": "item_dewdrop", "mysekaiMaterialRarityType": "rarity_2"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiItems.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMusicRecords.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "musics.json"), []map[string]any{})

	mysekaiJSON := `{"updatedResources": {"userMysekaiHarvestMaps": [{
  "mysekaiSiteId": 5,
  "userMysekaiSiteHarvestFixtures": [
    {"mysekaiSiteHarvestFixtureId": 8027, "positionX": 3, "positionZ": 4},
    {"mysekaiSiteHarvestFixtureId": 2001, "positionX": 5, "positionZ": 6},
    {"mysekaiSiteHarvestFixtureId": 2002, "positionX": 7, "positionZ": 8}
  ],
  "userMysekaiSiteHarvestResourceDrops": [
    {"resourceType": "mysekai_material", "resourceId": 297, "positionX": 3, "positionZ": 4, "quantity": 20},
    {"resourceType": "mysekai_material", "resourceId": 17, "positionX": 5, "positionZ": 6, "quantity": 1},
    {"resourceType": "mysekai_material", "resourceId": 6, "positionX": 7, "positionZ": 8, "quantity": 2}
  ]
}]}}`
	controller := NewController(nil, nil, renderregion.JP, assets.NewAssetHelper(filepath.Join(root, "asset"), nil), MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true}).WithMySekaiData([]byte(mysekaiJSON))

	req, err := controller.BuildMapRequest(MapQuery{Region: "jp", MapIDs: []int{5}, HighlightMaterialIDs: []int{17}})
	if err != nil {
		t.Fatalf("BuildMapRequest() error = %v", err)
	}
	site := req.Maps[0]

	plant := findMapHarvestPoint(site.HarvestPoints, 8027)
	if plant == nil || plant.ImagePath.First() != "asset/jp-assets/ondemand/mysekai/birthday/haruka_2026/icon_refresh.png" {
		t.Fatalf("birthday plant image = %+v", plant)
	}
	if len(plant.ImagePath) < 2 || plant.OutlineColor != nil {
		t.Fatalf("birthday plant candidates = %q outline = %v", plant.ImagePath, plant.OutlineColor)
	}

	dewdrop := findMapResourceDrop(site.ResourceDrops, "mysekai_material", 297)
	if dewdrop == nil || dewdrop.Rarity != 2 || dewdrop.Hide {
		t.Fatalf("2026 delivery drop = %+v", dewdrop)
	}
	battery := findMapResourceDrop(site.ResourceDrops, "mysekai_material", 17)
	if battery == nil || battery.Rarity != 1 || battery.OutlineColor != nil || battery.LightSize != nil {
		t.Fatalf("highlighted battery icon must stay plain: %+v", battery)
	}

	rock := findMapHarvestPoint(site.HarvestPoints, 2001)
	if rock == nil || !slices.Equal(rock.OutlineColor, mysekaiMapRarePointOutlineColor) || rock.OutlineWidth == nil || *rock.OutlineWidth != mysekaiMapRarePointOutlineWidth {
		t.Fatalf("rare harvest point outline = %+v", rock)
	}
	if plain := findMapHarvestPoint(site.HarvestPoints, 2002); plain == nil || plain.OutlineColor != nil {
		t.Fatalf("common harvest point outline = %+v", plain)
	}

	// Without the highlight the battery's point is not outlined.
	req, err = controller.BuildMapRequest(MapQuery{Region: "jp", MapIDs: []int{5}})
	if err != nil {
		t.Fatalf("BuildMapRequest() error = %v", err)
	}
	if rock := findMapHarvestPoint(req.Maps[0].HarvestPoints, 2001); rock == nil || rock.OutlineColor != nil {
		t.Fatalf("unhighlighted rock outline = %+v", rock)
	}
}

func TestOutlineRareHarvestPointsSkipsHarvestedAndHiddenDrops(t *testing.T) {
	points := []drawing.MysekaiMsrMapHarvestPoint{{PositionX: 1, PositionZ: 1}, {PositionX: 2, PositionZ: 2}, {PositionX: 3, PositionZ: 3}}
	drops := []drawing.MysekaiMsrMapResourceDrop{
		{Type: "mysekai_material", ID: 12, Rarity: 2, Status: "after_drop", PositionX: 1, PositionZ: 1},
		{Type: "mysekai_material", ID: 12, Rarity: 2, Status: "before_drop", Hide: true, PositionX: 2, PositionZ: 2},
		{Type: "mysekai_material", ID: 179, Rarity: 2, Status: "before_drop", PositionX: 3, PositionZ: 3},
	}
	outlineRareHarvestPoints(points, drops, mysekaiMapAssets{})
	for _, point := range points {
		if point.OutlineColor != nil {
			t.Fatalf("unexpected outline on %+v", point)
		}
	}
}

func TestBirthdayDeliveryCharacter(t *testing.T) {
	deliveries := map[int]int{297: 6}
	if birthdayDeliveryCharacter(297, deliveries) != 6 || birthdayDeliveryCharacter(179, nil) != 6 || birthdayDeliveryCharacter(300, deliveries) != 0 {
		t.Fatal("birthday delivery character mapping is incorrect")
	}
	if !mysekaiIsBirthdayDrop("material", 297, deliveries) || mysekaiIsBirthdayDrop("mysekai_item", 297, deliveries) {
		t.Fatal("birthday drop detection is incorrect")
	}
	if mysekaiBirthdayPartyIconCandidates("jp", "  ") != nil {
		t.Fatal("blank party asset must not produce candidates")
	}
}
