package mysekai

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
)

type sonarMasterdataSource struct {
	lists map[string][]map[string]any
	maps  map[string]map[int]map[string]any
}

func (*sonarMasterdataSource) Configured() bool { return true }
func (s *sonarMasterdataSource) loadList(filename string) []map[string]any {
	return s.lists[filename]
}
func (s *sonarMasterdataSource) loadMapByID(filename string) map[int]map[string]any {
	return s.maps[filename]
}
func (*sonarMasterdataSource) loadObject(string, any) bool { return false }
func (s *sonarMasterdataSource) WithContext(context.Context) masterdataSource {
	return s
}

func TestMusicRecordRefactorBranches(t *testing.T) {
	merged := map[string]any{"userMysekaiMusicRecords": []any{
		"invalid",
		map[string]any{"mysekaiMusicRecordId": 7, "obtainedAt": 123},
	}}
	if got := obtainedMysekaiMusicRecords(merged)[7]; got != 123 {
		t.Fatalf("obtained timestamp = %d", got)
	}
	windows := limitedMusicWindows([]map[string]any{{"musicId": 0}, {"musicId": 8, "startAt": 1, "endAt": 20}})
	if len(windows[8]) != 1 {
		t.Fatalf("limited windows = %#v", windows)
	}
	tags := musicTagsByID([]map[string]any{
		{"musicId": 0, "musicTag": "idol"},
		{"musicId": 8, "musicTag": "all"},
		{"musicId": 8, "musicTag": "street"},
		{"musicId": 8, "musicTag": "idol"},
	})
	if tags[8] != "street" {
		t.Fatalf("music tag = %q", tags[8])
	}

	musics := map[int]map[string]any{
		8:  {"publishedAt": int64(1), "assetbundleName": "music_8"},
		9:  {"publishedAt": int64(1), "assetbundleName": "music_9"},
		10: {"publishedAt": int64(99), "assetbundleName": "future"},
	}
	records := []map[string]any{
		{"mysekaiMusicTrackType": "voice", "id": 1, "externalId": 8},
		{"mysekaiMusicTrackType": "music", "id": 0, "externalId": 8},
		{"mysekaiMusicTrackType": "music", "id": 2, "externalId": 241},
		{"mysekaiMusicTrackType": "music", "id": 3, "externalId": 10},
		{"mysekaiMusicTrackType": "music", "id": 7, "externalId": 8},
		{"mysekaiMusicTrackType": "music", "id": 9, "externalId": 9},
	}
	byCategory, obtained := collectMysekaiMusicRecordIDs(records, musics, tags, windows, map[int]int64{7: 123}, 10)
	if !slices.Equal(byCategory["street"], []int{8}) || !slices.Equal(byCategory["vocaloid"], []int{9}) || obtained[8] != 123 {
		t.Fatalf("collected records = %#v, %#v", byCategory, obtained)
	}
	if _, _, ok := availableMysekaiMusicRecord(map[string]any{"mysekaiMusicTrackType": "music", "id": 1, "externalId": 8}, musics, map[int][]map[string]any{8: {{"startAt": 20, "endAt": 30}}}, 10); ok {
		t.Fatal("unavailable limited music was accepted")
	}

	controller := &Controller{}
	category, total, obtainedCount := controller.buildMusicRecordCategory(renderregion.JP, true, "street", "icon", []int{9, 8, 11}, map[int]int64{8: 2}, map[int]map[string]any{8: musics[8], 9: musics[9], 11: {}})
	if category == nil || total != 3 || obtainedCount != 1 || len(category.Musicrecords) != 2 || category.Musicrecords[0].ID == nil {
		t.Fatalf("category = %#v, total=%d, obtained=%d", category, total, obtainedCount)
	}
	if category, _, _ := controller.buildMusicRecordCategory(renderregion.JP, false, "other", "", nil, nil, nil); category != nil {
		t.Fatalf("empty category = %#v", category)
	}
	ids := []int{3, 2, 1}
	sortMusicRecordIDs(ids, map[int]int64{3: 20, 2: 10})
	if !slices.Equal(ids, []int{2, 3, 1}) {
		t.Fatalf("sorted ids = %#v", ids)
	}
}

func TestMapRefactorHelperBranches(t *testing.T) {
	merged := map[string]any{"userMysekaiHarvestMaps": []any{
		"invalid",
		map[string]any{"mysekaiSiteId": 0},
		map[string]any{"mysekaiSiteId": 5},
	}}
	if len(indexMysekaiHarvestMaps(merged)) != 1 {
		t.Fatalf("indexed maps = %#v", indexMysekaiHarvestMaps(merged))
	}
	drops := []any{
		"invalid",
		map[string]any{"resourceId": 100, "positionX": 1, "positionZ": 2},
		map[string]any{"resourceId": 174, "positionX": 1, "positionZ": 2},
		map[string]any{"resourceId": 175, "positionX": 1, "positionZ": 2},
	}
	characters := birthdayCharactersByHarvestPosition(drops)
	if characters[mysekaiHarvestPosKey(1, 2)] != 1 {
		t.Fatalf("birthday characters = %#v", characters)
	}
	if positiveIntPointer(0) != nil || *positiveIntPointer(4) != 4 {
		t.Fatal("positiveIntPointer returned the wrong pointer")
	}
	if got := mysekaiHarvestPointStatus(map[string]any{}); got != "spawned" {
		t.Fatalf("default status = %q", got)
	}
	if got := mysekaiHarvestPointStatus(map[string]any{"mysekaiSiteHarvestFixtureStatus": "harvested"}); got != "harvested" {
		t.Fatalf("legacy status = %q", got)
	}
	if !isToneGustHarvestFixture("other", "prefix_tone_gust_suffix") || isToneGustHarvestFixture("tree", "tree") {
		t.Fatal("tone gust detection is incorrect")
	}
	controller := &Controller{}
	path, fallback, size, offsetX, offsetZ := controller.mysekaiHarvestPointImage(renderregion.JP, "tree", "rarity_1", "tree", 0, 0, nil, nil)
	if path == "" || fallback != nil || size != nil || offsetX != 0 || offsetZ != -48 {
		t.Fatalf("regular harvest image = %q, %#v, %#v, %v, %v", path, fallback, size, offsetX, offsetZ)
	}
	assets := mysekaiMapAssets{harvestFixtures: map[int]map[string]any{}}
	points := controller.buildMysekaiMapHarvestPoints(renderregion.JP, []any{"invalid", map[string]any{"mysekaiSiteHarvestFixtureId": 99}}, drops, assets)
	if len(points) != 0 {
		t.Fatalf("invalid harvest points = %#v", points)
	}
}

func TestResourceRefactorGateSkinFilenames(t *testing.T) {
	if gateSkinMasterdataFilename("unit") != "mysekaiGateUnitSkins.json" || gateSkinMasterdataFilename("common") != "mysekaiGateCommonSkins.json" || gateSkinMasterdataFilename("unknown") != "" {
		t.Fatal("gate skin masterdata routing is incorrect")
	}
}

func TestDoorUpgradeFullRequestCoverage(t *testing.T) {
	showFull := true
	source := &sonarMasterdataSource{
		lists: map[string][]map[string]any{
			"mysekaiGateMaterialGroups.json": {
				{"groupId": 0},
				{"groupId": 1041, "mysekaiMaterialId": 1, "quantity": 99},
				{"groupId": 1001, "mysekaiMaterialId": 1, "quantity": 2},
				{"groupId": 1002, "mysekaiMaterialId": 1, "quantity": 3},
				{"groupId": 2001, "mysekaiMaterialId": 2, "quantity": 4},
			},
			"mysekaiMaterials.json": {
				{"id": 1, "iconAssetbundleName": "material_1"},
				{"id": 2, "iconAssetbundleName": "material_2"},
			},
		},
		maps: map[string]map[int]map[string]any{
			"mysekaiGates.json": {
				1: {"assetbundleName": "gate_1"},
				2: {"assetbundleName": "gate_2"},
			},
		},
	}
	controller := &Controller{masterdata: source, defaultRegion: renderregion.JP}
	request, err := controller.BuildDoorUpgradeRequest(DoorUpgradeQuery{Region: "jp", ShowFull: &showFull})
	if err != nil {
		t.Fatalf("BuildDoorUpgradeRequest() error = %v", err)
	}
	if request.Profile != nil || len(request.GateMaterials) != 2 {
		t.Fatalf("full request = %#v", request)
	}
	if request.GateMaterials[0].Level != nil || len(request.GateMaterials[0].LevelMaterials) != 2 {
		t.Fatalf("first gate = %#v", request.GateMaterials[0])
	}
}

func TestStaticPathRelativeToRootBranches(t *testing.T) {
	if staticPathRelativeToRoot(" ", "/tmp/value") != "" || staticPathRelativeToRoot("https://example.com/assets", "/tmp/value") != "" {
		t.Fatal("non-local roots were accepted")
	}
	root := filepath.Join(t.TempDir(), "root")
	if got := staticPathRelativeToRoot(root, filepath.Join(root, "static_images", "icon.png")); got != "static_images/icon.png" {
		t.Fatalf("root-relative static path = %q", got)
	}
	staticRoot := filepath.Join(t.TempDir(), "static_images")
	if got := staticPathRelativeToRoot(staticRoot, filepath.Join(staticRoot, "icon.png")); got != "static_images/icon.png" {
		t.Fatalf("static-root path = %q", got)
	}
	if got := staticPathRelativeToRoot(filepath.Join(t.TempDir(), "other"), "/unrelated/icon.png"); got != "" {
		t.Fatalf("unrelated path = %q", got)
	}
}
