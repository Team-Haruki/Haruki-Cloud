package mysekai

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// EN's birthdayParties carry no mysekaiSiteHarvestFixtureId, and its plants
// stand on tiles without a delivery dewdrop, so neither the master link nor
// the dewdrop fallback finds the party (2026-10-24..27 Honami party: every EN
// map drew the generic tree). The plant is 8000 + the party ID, and a plant
// no party maps to uses the party running at the snapshot time.
func TestBuildMapRequestBindsENBirthdayPlantsWithoutTheMasterField(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiSiteHarvestFixtures.json"), []map[string]any{
		{"id": 8001, "assetbundleName": "mdl_site_dewdrop_birthday_plant106", "mysekaiSiteHarvestFixtureType": "birthday_plant", "mysekaiSiteHarvestFixtureRarityType": "rarity_2"},
		{"id": 8002, "assetbundleName": "mdl_site_dewdrop_birthday_plant103", "mysekaiSiteHarvestFixtureType": "birthday_plant", "mysekaiSiteHarvestFixtureRarityType": "rarity_2"},
		{"id": 8003, "assetbundleName": "mdl_site_rock_common_stone01", "mysekaiSiteHarvestFixtureType": "mineral", "mysekaiSiteHarvestFixtureRarityType": "rarity_1"},
		{"id": 8099, "assetbundleName": "mdl_site_dewdrop_birthday_plant149", "mysekaiSiteHarvestFixtureType": "birthday_plant", "mysekaiSiteHarvestFixtureRarityType": "rarity_2"},
	})
	// The EN rows of 2026-10-30, minus the fields the map does not read.
	writeTestJSON(t, filepath.Join(masterdataDir, "birthdayParties.json"), []map[string]any{
		{"id": 1, "gameCharacterUnitId": 6, "startAt": int64(1790924400000), "closedAt": int64(1791442799000), "deliveryItemMaterialId": 179, "assetbundleName": "haruka_2025"},
		{"id": 2, "gameCharacterUnitId": 3, "startAt": int64(1792825200000), "closedAt": int64(1793343599000), "deliveryItemMaterialId": 176, "assetbundleName": "honami_2025"},
		{"id": 3, "gameCharacterUnitId": 49, "startAt": int64(1793606400000), "closedAt": int64(1794124799000), "deliveryItemMaterialId": 198, "assetbundleName": "meiko_2025"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacterUnits.json"), []map[string]any{{"id": 3, "gameCharacterId": 3}, {"id": 6, "gameCharacterId": 6}})
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacters.json"), []map[string]any{{"id": 3, "givenNameEnglish": "Honami"}, {"id": 6, "givenNameEnglish": "Haruka"}})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMaterials.json"), []map[string]any{
		{"id": 6, "iconAssetbundleName": "item_stone", "mysekaiMaterialRarityType": "rarity_1"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiItems.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMusicRecords.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "musics.json"), []map[string]any{})

	// 2026-10-30 12:00 CST: inside the Honami party only. No dewdrop lies on
	// either plant's tile.
	mysekaiJSON := `{"updatedResources": {"now": 1793332800000, "userMysekaiHarvestMaps": [{
  "mysekaiSiteId": 5,
  "userMysekaiSiteHarvestFixtures": [
    {"mysekaiSiteHarvestFixtureId": 8002, "positionX": 3, "positionZ": 4},
    {"mysekaiSiteHarvestFixtureId": 8099, "positionX": 9, "positionZ": 9},
    {"mysekaiSiteHarvestFixtureId": 8003, "positionX": 5, "positionZ": 6}
  ],
  "userMysekaiSiteHarvestResourceDrops": [
    {"resourceType": "mysekai_material", "resourceId": 6, "positionX": 5, "positionZ": 6, "quantity": 2}
  ]
}]}}`
	controller := NewController(nil, nil, renderregion.EN, assets.NewAssetHelper(filepath.Join(root, "asset"), nil), MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true}).WithMySekaiData([]byte(mysekaiJSON))

	req, err := controller.BuildMapRequest(MapQuery{Region: "en", MapIDs: []int{5}})
	if err != nil {
		t.Fatalf("BuildMapRequest() error = %v", err)
	}
	site := req.Maps[0]
	honami := "asset/en-assets/ondemand/mysekai/birthday/honami_2025/icon_refresh.png"
	if plant := findMapHarvestPoint(site.HarvestPoints, 8002); plant == nil || !slices.Equal(plant.ImagePath, drawing.AssetKey{honami}) || plant.FallbackImagePath == nil {
		t.Fatalf("party 2 plant (fixture 8002) = %+v", plant)
	}
	if plant := findMapHarvestPoint(site.HarvestPoints, 8099); plant == nil || !slices.Equal(plant.ImagePath, drawing.AssetKey{honami}) {
		t.Fatalf("unbound plant during the Honami party = %+v", plant)
	}
	for _, point := range site.HarvestPoints {
		for _, path := range point.ImagePath {
			if strings.Contains(path, "mdl_site_dewdrop_birthday_plant") {
				t.Fatalf("plant %d still asks for the model-named static icon: %q", *point.ID, point.ImagePath)
			}
		}
	}

	// Outside every party window the unbound plant keeps today's path.
	outside := strings.Replace(mysekaiJSON, "1793332800000", "1795000000000", 1)
	req, err = controller.WithMySekaiData([]byte(outside)).BuildMapRequest(MapQuery{Region: "en", MapIDs: []int{5}})
	if err != nil {
		t.Fatalf("BuildMapRequest(outside) error = %v", err)
	}
	if plant := findMapHarvestPoint(req.Maps[0].HarvestPoints, 8099); plant == nil || plant.ImagePath.First() != "static_images/mysekai/harvest_fixture_icon/rarity_2/mdl_site_dewdrop_birthday_plant149.png" {
		t.Fatalf("unbound plant outside a party = %+v", plant)
	}
	if plant := findMapHarvestPoint(req.Maps[0].HarvestPoints, 8002); plant == nil || plant.ImagePath.First() != honami {
		t.Fatalf("bound plant outside its party = %+v", plant)
	}
}

func TestLoadMysekaiBirthdayPartiesDerivesOnlyMissingPlantFixtures(t *testing.T) {
	root := t.TempDir()
	writeTestJSON(t, filepath.Join(root, "birthdayParties.json"), []map[string]any{
		// JP/TW/CN/KR shape: the named fixture is used as is.
		{"id": 1, "assetbundleName": "haruka_2025", "mysekaiSiteHarvestFixtureId": 8001},
		{"id": 27, "assetbundleName": "haruka_2026", "mysekaiSiteHarvestFixtureId": 8027},
		// EN shape: 8000 + id, when that fixture is a birthday plant.
		{"id": 2, "assetbundleName": "honami_2025"},
		{"id": 3, "assetbundleName": "meiko_2025", "mysekaiSiteHarvestFixtureId": 0},
		{"id": 4, "assetbundleName": "akito_2025"},
		// A named fixture wins over a derived one.
		{"id": 5, "assetbundleName": "shizuku_2025"},
		{"id": 40, "assetbundleName": "named_8005", "mysekaiSiteHarvestFixtureId": 8005},
		{"id": 6, "assetbundleName": "  "},
	})
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: root, AllowFallback: true}).withRegion("jp")
	harvestFixtures := map[int]map[string]any{
		8002: {"mysekaiSiteHarvestFixtureType": "birthday_plant"},
		8003: {"mysekaiSiteHarvestFixtureType": "birthday_plant"},
		8004: {"mysekaiSiteHarvestFixtureType": "mineral"},
		8005: {"mysekaiSiteHarvestFixtureType": "birthday_plant"},
		8006: {"mysekaiSiteHarvestFixtureType": "birthday_plant"},
	}
	got := controller.loadMysekaiBirthdayParties(harvestFixtures).plantAssets
	want := map[int]string{8001: "haruka_2025", 8027: "haruka_2026", 8002: "honami_2025", 8003: "meiko_2025", 8005: "named_8005"}
	if !maps.Equal(got, want) {
		t.Fatalf("plant assets = %v, want %v", got, want)
	}
}

func TestActiveMysekaiBirthdayPartyNeedsOneMatch(t *testing.T) {
	windows := []mysekaiBirthdayPartyWindow{
		{startAt: 100, closedAt: 200, assetbundleName: "honami_2025"},
		{startAt: 300, closedAt: 400, assetbundleName: "rin_2025"},
		{startAt: 300, closedAt: 400, assetbundleName: "len_2025"},
		{startAt: 0, closedAt: 999, assetbundleName: "unscheduled"},
	}
	cases := map[int64]string{99: "", 100: "honami_2025", 200: "honami_2025", 201: "", 350: ""}
	for at, want := range cases {
		if got := activeMysekaiBirthdayParty(windows, at); got != want {
			t.Fatalf("active party at %d = %q, want %q", at, got, want)
		}
	}
}
