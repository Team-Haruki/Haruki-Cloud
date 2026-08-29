//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package mysekai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

type closeTrackingMasterdataSource struct {
	closed bool
	reset  bool
}

func (*closeTrackingMasterdataSource) Configured() bool                          { return true }
func (*closeTrackingMasterdataSource) loadList(string) []map[string]any          { return nil }
func (*closeTrackingMasterdataSource) loadMapByID(string) map[int]map[string]any { return nil }
func (*closeTrackingMasterdataSource) loadObject(string, any) bool               { return false }
func (s *closeTrackingMasterdataSource) resetCache()                             { s.reset = true }
func (s *closeTrackingMasterdataSource) Close()                                  { s.closed = true }

func writeMysekaiRenderCoverageMasterdata(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]any{
		"mysekaiFixtures.json": []map[string]any{{
			"id": 1, "name": "Chair", "assetbundleName": "chair", "mysekaiFixtureType": "furniture",
			"mysekaiFixtureMainGenreId": 1, "mysekaiFixtureSubGenreId": 11,
			"gridSize": map[string]any{"width": 1, "depth": 1, "height": 1},
		}},
		"mysekaiFixtureMainGenres.json": []map[string]any{{"id": 1, "name": "Main", "assetbundleName": "main"}},
		"mysekaiFixtureSubGenres.json":  []map[string]any{{"id": 11, "name": "Sub", "assetbundleName": "sub"}},
		"mysekaiBlueprints.json": []map[string]any{{
			"id": 10, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 1, "isEnableSketch": false,
		}},
		"mysekaiBlueprintMysekaiMaterialCosts.json":   []map[string]any{{"mysekaiBlueprintId": 10, "mysekaiMaterialId": 1, "quantity": 2}},
		"mysekaiFixtureOnlyDisassembleMaterials.json": []map[string]any{{"mysekaiFixtureId": 1, "mysekaiMaterialId": 1, "quantity": 1}},
		"mysekaiFixtureTags.json":                     []map[string]any{},
		"mysekaiMaterials.json": []map[string]any{{
			"id": 1, "iconAssetbundleName": "material_one", "mysekaiMaterialRarityType": "rarity_4",
		}},
		"mysekaiItems.json": []map[string]any{{"id": 2, "iconAssetbundleName": "item_two"}},
		"mysekaiGateMaterialGroups.json": []map[string]any{{
			"id": 1, "groupId": 1001, "mysekaiMaterialId": 1, "quantity": 2,
		}},
		"mysekaiGates.json":           []map[string]any{{"id": 1, "assetbundleName": "gate_one"}},
		"mysekaiGateSkins.json":       []map[string]any{},
		"mysekaiGateUnitSkins.json":   []map[string]any{},
		"mysekaiGateCommonSkins.json": []map[string]any{},
		"mysekaiPhenomenas.json":      []map[string]any{},
		"mysekaiMusicRecords.json": []map[string]any{{
			"id": 4, "externalId": 1, "mysekaiMusicTrackType": "music",
		}},
		"musics.json":            []map[string]any{{"id": 1, "assetbundleName": "jacket_one", "publishedAt": 1}},
		"musicTags.json":         []map[string]any{{"musicId": 1, "musicTag": "idol"}},
		"limitedTimeMusics.json": []map[string]any{},
		"gameCharacters.json": []map[string]any{{
			"id": 1, "firstName": "Hoshino", "givenName": "Ichika", "givenNameEnglish": "Ichika",
		}},
		"gameCharacterUnits.json":                         []map[string]any{{"id": 101, "gameCharacterId": 1, "unit": "light_sound"}},
		"mysekaiGateCharacterLotteries.json":              []map[string]any{},
		"mysekaiGameCharacterUnitGroups.json":             []map[string]any{},
		"characterArchiveMysekaiCharacterTalkGroups.json": []map[string]any{},
		"mysekaiCharacterTalkConditions.json":             []map[string]any{},
		"mysekaiCharacterTalkConditionGroups.json":        []map[string]any{},
		"mysekaiCharacterTalks.json":                      []map[string]any{},
		"mysekaiSiteHarvestFixtures.json":                 []map[string]any{},
		"mysekaiPhenomenaBackgroundColors.json":           []map[string]any{},
	}
	for name, data := range files {
		writeTestJSON(t, filepath.Join(root, name), data)
	}
	return root
}

func TestMysekaiRenderWrappersExerciseSuccessfulDrawingPaths(t *testing.T) {
	masterdataDir := writeMysekaiRenderCoverageMasterdata(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("rendered"))
	}))
	defer server.Close()

	raw := []byte(`{
		"upload_time":1700000000000,
		"source":"toolbox",
		"updatedResources":{
			"userMysekaiGamedata":{"mysekaiRank":5},
			"userMysekaiBlueprints":[{"mysekaiBlueprintId":10}],
			"userMysekaiMaterials":[{"mysekaiMaterialId":1,"quantity":5}],
			"userMysekaiGates":[{"mysekaiGateId":1,"mysekaiGateLevel":0}],
			"userMysekaiMusicRecords":[{"mysekaiMusicRecordId":4,"obtainedAt":1000}],
			"userMysekaiHarvestMaps":[{"mysekaiSiteId":5,"userMysekaiSiteHarvestFixtures":[],"userMysekaiSiteHarvestResourceDrops":[]}]
		}
	}`)
	controller := NewController(
		drawing.NewHarukiDrawingClient(server.URL), nil, renderregion.JP, nil,
		MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true},
	).WithMySekaiData(raw)
	profile := &drawing.ProfileCardRequest{Profile: &drawing.BasicProfile{Nickname: "Tester"}}
	falseValue := false
	trueValue := true

	assertRendered := func(name string, image []byte, err error) {
		t.Helper()
		if err != nil || string(image) != "rendered" {
			t.Fatalf("%s = %q, %v", name, image, err)
		}
	}
	assertCall := func(name string, call func() ([]byte, error)) {
		t.Helper()
		image, err := call()
		assertRendered(name, image, err)
	}
	assertCall("RenderFixtureList", func() ([]byte, error) {
		return controller.RenderFixtureList(FixtureListQuery{
			ShowProfile: &falseValue, ShowProgress: &falseValue, ShowObtained: &falseValue,
		})
	})
	assertCall("RenderFixtureDetail", func() ([]byte, error) {
		return controller.RenderFixtureDetail(FixtureDetailQuery{Query: "1"})
	})
	assertCall("RenderDoorUpgrade", func() ([]byte, error) {
		return controller.RenderDoorUpgrade(DoorUpgradeQuery{ShowFull: &trueValue})
	})
	assertCall("RenderMusicRecord", func() ([]byte, error) {
		return controller.RenderMusicRecord(MusicRecordQuery{ShowID: &trueValue, Profile: profile})
	})
	assertCall("RenderResource", func() ([]byte, error) {
		return controller.RenderResource(ResourceQuery{Profile: profile})
	})
	assertCall("RenderTalkList", func() ([]byte, error) {
		return controller.RenderTalkList(TalkListQuery{Query: "Ichika", ShowAllTalks: &trueValue, Profile: profile})
	})
	assertCall("RenderMap", func() ([]byte, error) {
		return controller.RenderMap(MapQuery{MapIDs: []int{5}, ShowHarvested: &trueValue})
	})

	payload, err := controller.BuildMapRequest(MapQuery{MapIDs: []int{5}})
	if err != nil {
		t.Fatalf("BuildMapRequest = %v", err)
	}
	assertCall("RenderMapRequest", func() ([]byte, error) { return controller.RenderMapRequest(payload) })
	if _, err := controller.RenderMapRequest(nil); err == nil {
		t.Fatal("nil map payload should fail")
	}
	if _, err := (*Controller)(nil).RenderMapRequest(payload); err == nil {
		t.Fatal("nil map controller should fail")
	}
	if remaining, err := controller.HasRemainingHarvestResources(MapQuery{MapIDs: []int{5}}); err != nil || remaining {
		t.Fatalf("HasRemainingHarvestResources = %v, %v", remaining, err)
	}
	if MapRequestHasRemainingHarvestResources(nil) || MapRequestHasRemainingHarvestResources(&drawing.MysekaiMsrMapRequest{Maps: []drawing.MysekaiMsrMapData{{ResourceDrops: []drawing.MysekaiMsrMapResourceDrop{{Hide: true}}}}}) {
		t.Fatal("hidden or nil map resources should not remain")
	}
	if !MapRequestHasRemainingHarvestResources(&drawing.MysekaiMsrMapRequest{Maps: []drawing.MysekaiMsrMapData{{ResourceDrops: []drawing.MysekaiMsrMapResourceDrop{{Hide: false}}}}}) {
		t.Fatal("visible map resource should remain")
	}

	for name, call := range map[string]func() error{
		"resource": func() error { _, err := (*Controller)(nil).RenderResource(ResourceQuery{}); return err },
		"music":    func() error { _, err := (*Controller)(nil).RenderMusicRecord(MusicRecordQuery{}); return err },
		"talk":     func() error { _, err := (*Controller)(nil).RenderTalkList(TalkListQuery{}); return err },
		"door":     func() error { _, err := (*Controller)(nil).RenderDoorUpgrade(DoorUpgradeQuery{}); return err },
		"fixtures": func() error { _, err := (*Controller)(nil).RenderFixtureList(FixtureListQuery{}); return err },
		"detail":   func() error { _, err := (*Controller)(nil).RenderFixtureDetail(FixtureDetailQuery{}); return err },
		"map":      func() error { _, err := (*Controller)(nil).RenderMap(MapQuery{}); return err },
	} {
		if err := call(); err == nil {
			t.Fatalf("nil %s renderer should fail", name)
		}
	}
}

func TestControllerResourceExtractionAndLifecycleBranches(t *testing.T) {
	masterdataDir := writeMysekaiRenderCoverageMasterdata(t)
	groups := make([]map[string]any, 0, 10)
	units := make([]map[string]any, 0, 10)
	visits := []any{"invalid", map[string]any{"mysekaiGameCharacterUnitGroupId": 404}}
	for index := 1; index <= 8; index++ {
		groupID := index
		unitID := 100 + index
		groups = append(groups, map[string]any{"id": groupID, "gameCharacterUnitId1": unitID})
		units = append(units, map[string]any{"id": unitID, "gameCharacterId": index})
		visits = append(visits, map[string]any{"mysekaiGameCharacterUnitGroupId": groupID, "isReservation": index == 1})
	}
	groups = append(groups,
		map[string]any{"id": 50, "gameCharacterUnitId1": 150, "gameCharacterUnitId2": 151},
		map[string]any{"id": 51, "gameCharacterUnitId1": 0},
	)
	visits = append([]any{map[string]any{"mysekaiGameCharacterUnitGroupId": 50}, map[string]any{"mysekaiGameCharacterUnitGroupId": 51}}, visits...)
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiGameCharacterUnitGroups.json"), groups)
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacterUnits.json"), units)
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true})

	if got := controller.extractVisitCharacters(renderregion.JP, map[string]any{}); len(got) != 0 {
		t.Fatalf("visits without a visit object = %+v", got)
	}
	if got := controller.extractVisitCharacters(renderregion.JP, map[string]any{"userMysekaiGateCharacterVisit": map[string]any{}}); len(got) != 0 {
		t.Fatalf("visits without characters = %+v", got)
	}
	gotVisits := controller.extractVisitCharacters(renderregion.JP, map[string]any{"userMysekaiGateCharacterVisit": map[string]any{"userMysekaiGateCharacters": visits}})
	if len(gotVisits) != 6 || !gotVisits[0].IsReservation || gotVisits[0].ReservationIconPath == nil || gotVisits[0].MemoriaImagePath == nil {
		t.Fatalf("visit extraction = %+v", gotVisits)
	}
	if controller.gameCharacterIDByUnitID(101) != 1 || controller.gameCharacterIDByUnitID(999) != 0 {
		t.Fatal("unit to character lookup mismatch")
	}

	fixtureIDs := userMysekaiFixtureIDs([]any{
		"invalid",
		map[string]any{"fixtureID": 1},
		map[string]any{"mysekai_fixture_id": 2},
		map[string]any{"mysekaiFixture": map[string]any{"id": 3}},
		map[string]any{},
	})
	if len(fixtureIDs) != 3 {
		t.Fatalf("user fixture IDs = %v", fixtureIDs)
	}
	blueprints := map[int]map[string]any{
		10: {"mysekaiCraftType": "mysekai_fixture", "craftTargetId": 1},
		11: {"mysekaiCraftType": "item", "craftTargetId": 2},
		12: {"mysekaiCraftType": "mysekai_fixture", "craftTargetId": 0},
	}
	fromBlueprints := userMysekaiBlueprintFixtureIDs(map[string]any{"userMysekaiBlueprints": []any{
		"invalid", map[string]any{"mysekaiBlueprintId": 404}, map[string]any{"mysekaiBlueprintId": 11}, map[string]any{"mysekaiBlueprintId": 10},
	}}, blueprints)
	if len(fromBlueprints) != 1 {
		t.Fatalf("blueprint fixture IDs = %v", fromBlueprints)
	}
	if got := controller.craftableMysekaiFixtureIDs(blueprints); len(got) != 1 {
		t.Fatalf("craftable fixture IDs = %v", got)
	}

	merged := map[string]any{
		"userMysekaiMusicRecords": []any{"invalid", map[string]any{"mysekaiMusicRecordId": 4}},
		"userMysekaiHarvestMaps": []any{
			"invalid",
			map[string]any{"mysekaiSiteId": 99},
			map[string]any{"mysekaiSiteId": 5, "userMysekaiSiteHarvestResourceDrops": []any{
				"invalid",
				map[string]any{"status": "after_drop", "type": "item", "id": 2},
				map[string]any{"status": "before_drop", "type": "item", "id": 2, "quantity": 0},
				map[string]any{"mysekaiSiteHarvestResourceDropStatus": "before_drop", "resourceType": "mysekai_material", "resourceId": 1, "quantity": 2},
				map[string]any{"status": "before_drop", "type": "fixture", "id": 1},
				map[string]any{"status": "before_drop", "type": "music_record", "id": 4},
				map[string]any{"status": "before_drop", "type": "unknown", "id": 9},
				map[string]any{"status": "before_drop"},
			}},
		},
	}
	sites := controller.extractSiteResourceNumbers(renderregion.JP, merged)
	if len(sites) != 1 || len(sites[0].ResourceNumbers) != 4 {
		t.Fatalf("site resources = %+v", sites)
	}
	if controller.hasMysekaiMusicRecord(merged, 4) != true || controller.hasMysekaiMusicRecord(merged, 5) {
		t.Fatal("music record ownership mismatch")
	}
	if path, _ := controller.resourceImagePath(renderregion.JP, "invalid", nil, nil, nil, nil, nil); path != "" {
		t.Fatalf("invalid resource image path = %q", path)
	}
	if got := (*Controller)(nil).loadIconNameMap("x", "y"); len(got) != 0 {
		t.Fatalf("nil icon map = %v", got)
	}
	if got := (&Controller{}).loadFieldMap("x", "y"); len(got) != 0 {
		t.Fatalf("nil field map = %v", got)
	}
	if mysekaiBirthdayCharacterImageName(nil) != "" || mysekaiBirthdayCharacterImageName(map[string]any{"givenNameEnglish": " Ichika "}) != "ichika" {
		t.Fatal("birthday image name mismatch")
	}
	storeMysekaiBirthdayRefreshIcon("", "")

	var nilController *Controller
	if nilController.WithSnapshot(nil) != nil || nilController.WithContext(context.Background()) != nil {
		t.Fatal("nil controller clones should remain nil")
	}
	cloned := controller.WithSnapshot(nil)
	if cloned == controller || cloned.snapshot != nil {
		t.Fatal("WithSnapshot should shallow-clone the controller")
	}
	if controller.WithContext(nil) == controller {
		t.Fatal("WithContext should shallow-clone the controller")
	}
	if controller.WithMySekaiData(nil) != nil {
		t.Fatal("empty direct MySekai data should be rejected")
	}

	source := &closeTrackingMasterdataSource{}
	resolver := &masterdataResolver{cache: map[string]masterdataSource{"jp": source}}
	lifecycle := &Controller{resolver: resolver}
	lifecycle.ResetMasterdataCache()
	if !source.reset {
		t.Fatal("controller cache reset did not reach source")
	}
	lifecycle.Close()
	if !source.closed || len(resolver.cache) != 0 {
		t.Fatalf("controller close source=%+v cache=%v", source, resolver.cache)
	}
	nilController.Close()
	nilController.ResetMasterdataCache()
	(*masterdataResolver)(nil).Close()
	(*masterdataResolver)(nil).ResetMasterdataCache()
	(&Controller{}).Close()
	(&Controller{}).ResetMasterdataCache()

	if got := (*fixtureCategoryNotFoundError)(nil).Error(); got != "mysekai fixture category not found" {
		t.Fatalf("nil category error = %q", got)
	}
	if got := (&fixtureCategoryNotFoundError{query: " missing "}).Error(); !strings.HasSuffix(got, "missing") {
		t.Fatalf("category error = %q", got)
	}
}

func TestSnapshotStatusAndPhotoEdgeBranches(t *testing.T) {
	controller := (&Controller{defaultRegion: renderregion.JP}).WithMySekaiData([]byte(`{
		"upload_time":1700000000,
		"updatedResources":{"userMysekaiPhotos":[]}
	}`))
	status, err := controller.SnapshotStatus("", time.Time{})
	if err != nil || status.LastUpdatedAt.UnixMilli() != 1700000000000 {
		t.Fatalf("SnapshotStatus = %+v, %v", status, err)
	}
	if _, err := controller.SnapshotExpired("jp"); err != nil {
		t.Fatalf("SnapshotExpired = %v", err)
	}
	if _, err := (&Controller{}).SnapshotStatus("jp", time.Now()); err == nil {
		t.Fatal("snapshot status without data should fail")
	}
	if _, err := controller.ResolvePhoto(PhotoQuery{Seq: 1}); err == nil {
		t.Fatal("empty photos should fail")
	}

	badItem := (&Controller{defaultRegion: renderregion.JP}).WithMySekaiData([]byte(`{"updatedResources":{"userMysekaiPhotos":[1]}}`))
	if _, err := badItem.ResolvePhoto(PhotoQuery{Seq: 1}); err == nil {
		t.Fatal("non-object photo should fail")
	}
	if _, err := badItem.ResolvePhoto(PhotoQuery{Seq: -2}); err == nil {
		t.Fatal("negative photo index beyond the list should fail")
	}

	if got := normalizeMySekaiTimestampMs(0); got != 0 {
		t.Fatalf("zero timestamp = %d", got)
	}
	if got := normalizeMySekaiTimestampMs(1_700_000_000); got != 1_700_000_000_000 {
		t.Fatalf("seconds timestamp = %d", got)
	}
	if got := normalizeMySekaiTimestampMs(1_700_000_000_000); got != 1_700_000_000_000 {
		t.Fatalf("millisecond timestamp = %d", got)
	}

	root := t.TempDir()
	staticRoot := filepath.Join(root, "static_images")
	if err := os.MkdirAll(staticRoot, 0o755); err != nil {
		t.Fatalf("mkdir static root: %v", err)
	}
	if got := (&Controller{}).staticPath(" "); got != "" {
		t.Fatalf("blank static path = %q", got)
	}
	if got := (&Controller{}).staticPath("icon.png"); got != "static_images/icon.png" {
		t.Fatalf("fallback static path = %q", got)
	}

	if !reflect.DeepEqual(parseMysekaiColorCode("#010203"), []int{1, 2, 3, 255}) || parseMysekaiColorCode("#bad") != nil || parseMysekaiColorCode("#gg0000") != nil {
		t.Fatal("color code parsing mismatch")
	}
}
