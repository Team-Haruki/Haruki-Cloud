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
	"haruki-cloud/internal/testutil"
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
		{
			testutil.Require(t, !(err != nil), "%s = %q, %v", name, image, err)
			testutil.Require(t, !(string(image) != "rendered"), "%s = %q, %v", name, image, err)
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
	testutil.Require(t, !(err != nil), "BuildMapRequest = %v", err)

	assertCall("RenderMapRequest", func() ([]byte, error) { return controller.RenderMapRequest(payload) })
	{
		_, err := controller.RenderMapRequest(nil)
		testutil.RequireArgs(t, !(err == nil), "nil map payload should fail")
	}
	{

		_, err := (*Controller)(nil).RenderMapRequest(payload)
		testutil.RequireArgs(t, !(err == nil), "nil map controller should fail")
	}
	{

		remaining, err := controller.HasRemainingHarvestResources(MapQuery{MapIDs: []int{5}})
		{
			testutil.Require(t, !(err != nil), "HasRemainingHarvestResources = %v, %v", remaining, err)
			testutil.Require(t, !(remaining), "HasRemainingHarvestResources = %v, %v", remaining, err)
		}
	}
	{
		testutil.RequireArgs(t, !(MapRequestHasRemainingHarvestResources(nil)), "hidden or nil map resources should not remain")
		testutil.RequireArgs(t, !(MapRequestHasRemainingHarvestResources(&drawing.MysekaiMsrMapRequest{Maps: []drawing.MysekaiMsrMapData{{ResourceDrops: []drawing.MysekaiMsrMapResourceDrop{{Hide: true}}}}})), "hidden or nil map resources should not remain")
	}
	testutil.RequireArgs(t, MapRequestHasRemainingHarvestResources(&drawing.MysekaiMsrMapRequest{Maps: []drawing.MysekaiMsrMapData{{ResourceDrops: []drawing.MysekaiMsrMapResourceDrop{{Hide: false}}}}}), "visible map resource should remain")

	for name, call := range map[string]func() error{
		"resource": func() error { _, err := (*Controller)(nil).RenderResource(ResourceQuery{}); return err },
		"music":    func() error { _, err := (*Controller)(nil).RenderMusicRecord(MusicRecordQuery{}); return err },
		"talk":     func() error { _, err := (*Controller)(nil).RenderTalkList(TalkListQuery{}); return err },
		"door":     func() error { _, err := (*Controller)(nil).RenderDoorUpgrade(DoorUpgradeQuery{}); return err },
		"fixtures": func() error { _, err := (*Controller)(nil).RenderFixtureList(FixtureListQuery{}); return err },
		"detail":   func() error { _, err := (*Controller)(nil).RenderFixtureDetail(FixtureDetailQuery{}); return err },
		"map":      func() error { _, err := (*Controller)(nil).RenderMap(MapQuery{}); return err },
	} {
		{
			err := call()
			testutil.Require(t, !(err == nil), "nil %s renderer should fail", name)
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
	{

		got := controller.extractVisitCharacters(renderregion.JP, map[string]any{})
		testutil.Require(t, !(len(got) != 0), "visits without a visit object = %+v", got)
	}
	{

		got := controller.extractVisitCharacters(renderregion.JP, map[string]any{"userMysekaiGateCharacterVisit": map[string]any{}})
		testutil.Require(t, !(len(got) != 0), "visits without characters = %+v", got)
	}

	gotVisits := controller.extractVisitCharacters(renderregion.JP, map[string]any{"userMysekaiGateCharacterVisit": map[string]any{"userMysekaiGateCharacters": visits}})
	{
		testutil.Require(t, !(len(gotVisits) != 6), "visit extraction = %+v", gotVisits)
		testutil.Require(t, gotVisits[0].IsReservation, "visit extraction = %+v", gotVisits)
		testutil.Require(t, !(gotVisits[0].ReservationIconPath == nil), "visit extraction = %+v", gotVisits)
		testutil.Require(t, !(gotVisits[0].MemoriaImagePath == nil), "visit extraction = %+v", gotVisits)
	}
	{
		testutil.RequireArgs(t, !(controller.gameCharacterIDByUnitID(101) != 1), "unit to character lookup mismatch")
		testutil.RequireArgs(t, !(controller.gameCharacterIDByUnitID(999) != 0), "unit to character lookup mismatch")
	}

	fixtureIDs := userMysekaiFixtureIDs([]any{
		"invalid",
		map[string]any{"fixtureID": 1},
		map[string]any{"mysekai_fixture_id": 2},
		map[string]any{"mysekaiFixture": map[string]any{"id": 3}},
		map[string]any{},
	})
	testutil.Require(t, !(len(fixtureIDs) != 3), "user fixture IDs = %v", fixtureIDs)

	blueprints := map[int]map[string]any{
		10: {"mysekaiCraftType": "mysekai_fixture", "craftTargetId": 1},
		11: {"mysekaiCraftType": "item", "craftTargetId": 2},
		12: {"mysekaiCraftType": "mysekai_fixture", "craftTargetId": 0},
	}
	fromBlueprints := userMysekaiBlueprintFixtureIDs(map[string]any{"userMysekaiBlueprints": []any{
		"invalid", map[string]any{"mysekaiBlueprintId": 404}, map[string]any{"mysekaiBlueprintId": 11}, map[string]any{"mysekaiBlueprintId": 10},
	}}, blueprints)
	testutil.Require(t, !(len(fromBlueprints) != 1), "blueprint fixture IDs = %v", fromBlueprints)
	{

		got := controller.craftableMysekaiFixtureIDs(blueprints)
		testutil.Require(t, !(len(got) != 1), "craftable fixture IDs = %v", got)
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
	{
		testutil.Require(t, !(len(sites) != 1), "site resources = %+v", sites)
		testutil.Require(t, !(len(sites[0].ResourceNumbers) != 4), "site resources = %+v", sites)
	}
	{
		testutil.RequireArgs(t, !(controller.hasMysekaiMusicRecord(merged, 4) != true), "music record ownership mismatch")
		testutil.RequireArgs(t, !(controller.hasMysekaiMusicRecord(merged, 5)), "music record ownership mismatch")
	}
	{

		path, _ := controller.resourceImagePath(renderregion.JP, "invalid", nil, nil, nil, nil, nil)
		testutil.Require(t, !(path != ""), "invalid resource image path = %q", path)
	}
	{

		got := (*Controller)(nil).loadIconNameMap("x", "y")
		testutil.Require(t, !(len(got) != 0), "nil icon map = %v", got)
	}
	{

		got := (&Controller{}).loadFieldMap("x", "y")
		testutil.Require(t, !(len(got) != 0), "nil field map = %v", got)
	}
	{
		testutil.RequireArgs(t, !(mysekaiBirthdayCharacterImageName(nil) != ""), "birthday image name mismatch")
		testutil.RequireArgs(t, !(mysekaiBirthdayCharacterImageName(map[string]any{"givenNameEnglish": " Ichika "}) != "ichika"), "birthday image name mismatch")
	}

	storeMysekaiBirthdayRefreshIcon("", "")

	var nilController *Controller
	{
		testutil.RequireArgs(t, !(nilController.WithSnapshot(nil) != nil), "nil controller clones should remain nil")
		testutil.RequireArgs(t, !(nilController.WithContext(context.Background()) != nil), "nil controller clones should remain nil")
	}

	cloned := controller.WithSnapshot(nil)
	{
		testutil.RequireArgs(t, !(cloned == controller), "WithSnapshot should shallow-clone the controller")
		testutil.RequireArgs(t, !(cloned.snapshot != nil), "WithSnapshot should shallow-clone the controller")
	}
	testutil.RequireArgs(t, !(controller.WithContext(nil) == controller), "WithContext should shallow-clone the controller")
	testutil.RequireArgs(t, !(controller.WithMySekaiData(nil) != nil), "empty direct MySekai data should be rejected")

	source := &closeTrackingMasterdataSource{}
	resolver := &masterdataResolver{cache: map[string]masterdataSource{"jp": source}}
	lifecycle := &Controller{resolver: resolver}
	lifecycle.ResetMasterdataCache()
	testutil.RequireArgs(t, source.reset, "controller cache reset did not reach source")

	lifecycle.Close()
	{
		testutil.Require(t, source.closed, "controller close source=%+v cache=%v", source, resolver.cache)
		testutil.Require(t, !(len(resolver.cache) != 0), "controller close source=%+v cache=%v", source, resolver.cache)
	}

	nilController.Close()
	nilController.ResetMasterdataCache()
	(*masterdataResolver)(nil).Close()
	(*masterdataResolver)(nil).ResetMasterdataCache()
	(&Controller{}).Close()
	(&Controller{}).ResetMasterdataCache()
	{

		got := (*fixtureCategoryNotFoundError)(nil).Error()
		testutil.Require(t, !(got != "mysekai fixture category not found"), "nil category error = %q", got)
	}
	{

		got := (&fixtureCategoryNotFoundError{query: " missing "}).Error()
		testutil.Require(t, strings.HasSuffix(got, "missing"), "category error = %q", got)
	}

}

func TestSnapshotStatusAndPhotoEdgeBranches(t *testing.T) {
	controller := (&Controller{defaultRegion: renderregion.JP}).WithMySekaiData([]byte(`{
		"upload_time":1700000000,
		"updatedResources":{"userMysekaiPhotos":[]}
	}`))
	status, err := controller.SnapshotStatus("", time.Time{})
	{
		testutil.Require(t, !(err != nil), "SnapshotStatus = %+v, %v", status, err)
		testutil.Require(t, !(status.LastUpdatedAt.UnixMilli() != 1700000000000), "SnapshotStatus = %+v, %v", status, err)
	}
	{

		_, err := controller.SnapshotExpired("jp")
		testutil.Require(t, !(err != nil), "SnapshotExpired = %v", err)
	}
	{

		_, err := (&Controller{}).SnapshotStatus("jp", time.Now())
		testutil.RequireArgs(t, !(err == nil), "snapshot status without data should fail")
	}
	{

		_, err := controller.ResolvePhoto(PhotoQuery{Seq: 1})
		testutil.RequireArgs(t, !(err == nil), "empty photos should fail")
	}

	badItem := (&Controller{defaultRegion: renderregion.JP}).WithMySekaiData([]byte(`{"updatedResources":{"userMysekaiPhotos":[1]}}`))
	{
		_, err := badItem.ResolvePhoto(PhotoQuery{Seq: 1})
		testutil.RequireArgs(t, !(err == nil), "non-object photo should fail")
	}
	{

		_, err := badItem.ResolvePhoto(PhotoQuery{Seq: -2})
		testutil.RequireArgs(t, !(err == nil), "negative photo index beyond the list should fail")
	}
	{

		got := normalizeMySekaiTimestampMs(0)
		testutil.Require(t, !(got != 0), "zero timestamp = %d", got)
	}
	{

		got := normalizeMySekaiTimestampMs(1_700_000_000)
		testutil.Require(t, !(got != 1_700_000_000_000), "seconds timestamp = %d", got)
	}
	{

		got := normalizeMySekaiTimestampMs(1_700_000_000_000)
		testutil.Require(t, !(got != 1_700_000_000_000), "millisecond timestamp = %d", got)
	}

	root := t.TempDir()
	staticRoot := filepath.Join(root, "static_images")
	{
		err := os.MkdirAll(staticRoot, 0o755)
		testutil.Require(t, !(err != nil), "mkdir static root: %v", err)
	}
	{

		got := (&Controller{}).staticPath(" ")
		testutil.Require(t, !(got != ""), "blank static path = %q", got)
	}
	{

		got := (&Controller{}).staticPath("icon.png")
		testutil.Require(t, !(got != "static_images/icon.png"), "fallback static path = %q", got)
	}
	{
		testutil.RequireArgs(t, reflect.DeepEqual(parseMysekaiColorCode("#010203"), []int{1, 2, 3, 255}), "color code parsing mismatch")
		testutil.RequireArgs(t, !(parseMysekaiColorCode("#bad") != nil), "color code parsing mismatch")
		testutil.RequireArgs(t, !(parseMysekaiColorCode("#gg0000") != nil), "color code parsing mismatch")
	}

}
