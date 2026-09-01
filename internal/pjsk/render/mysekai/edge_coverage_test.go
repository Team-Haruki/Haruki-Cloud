package mysekai

import (
	"context"
	"reflect"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/testutil"
)

func TestTalkNicknameAndProfileDataSourceEdgeBranches(t *testing.T) {
	testTalkNicknameEdgeBranches(t)
	testProfileDataSourceEdgeBranches(t)
}

func testTalkNicknameEdgeBranches(t *testing.T) {
	masterdataDir := writeMysekaiRenderCoverageMasterdata(t)
	writeTestJSON(t, masterdataDir+"/gameCharacters.json", []map[string]any{{
		"id": 1, "firstName": "Hoshino", "givenName": "Ichika", "firstNameEnglish": "Star", "givenNameEnglish": "Singer",
	}})
	writeTestJSON(t, masterdataDir+"/gameCharacterUnits.json", []map[string]any{
		{"id": 101, "gameCharacterId": 1, "unit": "light_sound"},
		{"id": 201, "gameCharacterId": 21, "unit": "piapro"},
		{"id": 202, "gameCharacterId": 21, "unit": "idol"},
		{"id": 220, "gameCharacterId": 22, "unit": "idol"},
	})
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true})
	controller.nicknames = map[string]int{"alias": 9}
	{
		testutil.RequireArgs(t, !(controller.lookupTalkCharacterID("") != 0), "talk nickname lookup mismatch")
		testutil.RequireArgs(t, !(controller.lookupTalkCharacterID("alias") != 9), "talk nickname lookup mismatch")
	}

	for _, query := range []string{"Hoshino", "Ichika", "HoshinoIchika", "Hoshino Ichika", "Star", "Singer", "StarSinger", "Star Singer"} {
		{
			got := controller.lookupTalkCharacterID(query)
			testutil.Require(t, !(got != 1), "lookupTalkCharacterID(%q) = %d", query, got)
		}

	}
	testutil.RequireArgs(t, !(controller.lookupTalkCharacterID("missing") != 0), "missing talk character should not resolve")

	units := controller.masterdata.loadList("gameCharacterUnits.json")
	{
		characterID, unitID, err := controller.resolveTalkCharacter("101")
		{
			testutil.Require(t, !(err != nil), "resolve numeric unit = %d, %d, %v", characterID, unitID, err)
			testutil.Require(t, !(characterID != 1), "resolve numeric unit = %d, %d, %v", characterID, unitID, err)
			testutil.Require(t, !(unitID != 101), "resolve numeric unit = %d, %d, %v", characterID, unitID, err)
		}
	}
	{

		_, _, err := controller.resolveTalkCharacter("404")
		testutil.RequireArgs(t, !(err == nil), "missing numeric character should fail")
	}
	{

		_, _, err := controller.resolveTalkCharacter("missing")
		testutil.RequireArgs(t, !(err == nil), "missing named character should fail")
	}
	{

		_, _, err := controller.resolveTalkCharacterUnit("missing", "", 404, units)
		testutil.RequireArgs(t, !(err == nil), "character without units should fail")
	}
	{

		_, _, err := controller.resolveTalkCharacterUnit("miku", "", 21, units)
		testutil.RequireArgs(t, !(err == nil), "Miku with multiple units should require a unit")
	}
	{

		characterID, unitID, err := controller.resolveTalkCharacterUnit("miku", "idol", 21, units)
		{
			testutil.Require(t, !(err != nil), "Miku idol unit = %d, %d, %v", characterID, unitID, err)
			testutil.Require(t, !(characterID != 21), "Miku idol unit = %d, %d, %v", characterID, unitID, err)
			testutil.Require(t, !(unitID != 202), "Miku idol unit = %d, %d, %v", characterID, unitID, err)
		}
	}
	{

		_, _, err := controller.resolveTalkCharacterUnit("rin", "street", 22, units)
		testutil.RequireArgs(t, !(err == nil), "fixed virtual singer unit mismatch should fail")
	}
	{

		characterID, unitID, err := controller.resolveTalkCharacterUnit("rin", "idol", 22, units)
		{
			testutil.Require(t, !(err != nil), "fixed virtual singer unit = %d, %d, %v", characterID, unitID, err)
			testutil.Require(t, !(characterID != 22), "fixed virtual singer unit = %d, %d, %v", characterID, unitID, err)
			testutil.Require(t, !(unitID != 220), "fixed virtual singer unit = %d, %d, %v", characterID, unitID, err)
		}
	}
	{

		unit, query := extractMysekaiTalkUnit("  ")
		{
			testutil.Require(t, !(unit != ""), "empty talk unit = %q, %q", unit, query)
			testutil.Require(t, !(query != ""), "empty talk unit = %q, %q", unit, query)
		}
	}
	{

		unit, query := extractMysekaiTalkUnit("ln Ichika")
		{
			testutil.Require(t, !(unit != "light_sound"), "aliased talk unit = %q, %q", unit, query)
			testutil.Require(t, !(query != "Ichika"), "aliased talk unit = %q, %q", unit, query)
		}
	}
	{
		testutil.RequireArgs(t, !(normalizeMysekaiTalkUnit(" LN ") != "light_sound"), "talk unit normalization mismatch")
		testutil.RequireArgs(t, !(normalizeMysekaiTalkUnit(" Custom ") != "custom"), "talk unit normalization mismatch")
	}
	{

		id, ok := ResolveNicknameCharacterID(" ")
		{
			testutil.Require(t, !(ok), "blank nickname = %d, %v", id, ok)
			testutil.Require(t, !(id != 0), "blank nickname = %d, %v", id, ok)
		}
	}
	{

		id, ok := ResolveNicknameCharacterID("prefix miku suffix")
		{
			testutil.Require(t, ok, "token nickname = %d, %v", id, ok)
			testutil.Require(t, !(id != 21), "token nickname = %d, %v", id, ok)
		}
	}
	{

		id, ok := ResolveNicknameCharacterID("definitely-missing")
		{
			testutil.Require(t, !(ok), "missing nickname = %d, %v", id, ok)
			testutil.Require(t, !(id != 0), "missing nickname = %d, %v", id, ok)
		}
	}

}

func testProfileDataSourceEdgeBranches(t *testing.T) {
	mergeMySekaiDataSources(nil, map[string]any{}, false)
	profile := &drawing.ProfileCardRequest{DataSources: []drawing.ProfileDataSource{{Name: "Suite"}}}
	mergeMySekaiDataSources(profile, map[string]any{}, true)
	testutil.Require(t, !(profile.DataSources[0].Name != "Mysekai数据"), "source-less replacement = %+v", profile.DataSources)

	profile = &drawing.ProfileCardRequest{DataSources: []drawing.ProfileDataSource{{Name: "Mysekai数据"}}}
	mergeMySekaiDataSources(profile, map[string]any{"source": "toolbox"}, false)
	{
		testutil.Require(t, !(profile.DataSources[0].Source == nil), "existing MySekai source = %+v", profile.DataSources)
		testutil.Require(t, !(*profile.DataSources[0].Source != "toolbox"), "existing MySekai source = %+v", profile.DataSources)
	}

	mode := "full"
	profile = &drawing.ProfileCardRequest{DataSources: []drawing.ProfileDataSource{{Name: "Suite", Mode: &mode}}}
	mergeMySekaiDataSources(profile, map[string]any{"source": "toolbox", "local_source": "cache", "upload_time": int64(1700000000)}, true)
	{
		testutil.Require(t, !(len(profile.DataSources) != 1), "single-source replacement = %+v", profile.DataSources)
		testutil.Require(t, !(profile.DataSources[0].Source == nil), "single-source replacement = %+v", profile.DataSources)
		testutil.Require(t, !(*profile.DataSources[0].Source != "toolbox(cache)"), "single-source replacement = %+v", profile.DataSources)
		testutil.Require(t, !(profile.DataSources[0].Mode == nil), "single-source replacement = %+v", profile.DataSources)
	}

	profile = &drawing.ProfileCardRequest{DataSources: []drawing.ProfileDataSource{{Name: "Suite"}, {Name: "Public"}}}
	mergeMySekaiDataSources(profile, map[string]any{"local_source": "local"}, false)
	{
		testutil.Require(t, !(len(profile.DataSources) != 3), "appended MySekai source = %+v", profile.DataSources)
		testutil.Require(t, !(*profile.DataSources[2].Source != "local"), "appended MySekai source = %+v", profile.DataSources)
	}

	replaceWithMySekaiDataSource(nil, nil)
	profile = &drawing.ProfileCardRequest{}
	replaceWithMySekaiDataSource(profile, map[string]any{})
	{
		testutil.Require(t, !(len(profile.DataSources) != 1), "fallback replacement source = %+v", profile.DataSources)
		testutil.Require(t, !(profile.DataSources[0].Name != "Mysekai数据"), "fallback replacement source = %+v", profile.DataSources)
	}

	stripProfileDataSourceDetails(nil)
	source := "source"
	profile.DataSources = []drawing.ProfileDataSource{{Source: &source, Mode: &mode}}
	stripProfileDataSourceDetails(profile)
	{
		testutil.Require(t, !(profile.DataSources[0].Source != nil), "stripped data source = %+v", profile.DataSources)
		testutil.Require(t, !(profile.DataSources[0].Mode != nil), "stripped data source = %+v", profile.DataSources)
	}

}

func TestTimeResourceAndSnapshotHelperEdges(t *testing.T) {
	var nilController *Controller
	var nilResolver *masterdataResolver
	testutil.RequireArgs(t, !(nilResolver.ResolveContext(context.Background(), renderregion.JP) != nil), "nil masterdata resolver should return nil")
	{
		testutil.RequireArgs(t, !(nilController.ensure() == nil), "nil controller ensure methods should fail")
		testutil.RequireArgs(t, !(nilController.ensureMasterdata() == nil), "nil controller ensure methods should fail")
		testutil.RequireArgs(t, !(nilController.ensureSnapshot() == nil), "nil controller ensure methods should fail")
	}
	testutil.RequireArgs(t, !((&Controller{}).ensure() == nil), "controller without snapshot should fail ensure")
	{

		_, _, err := (&Controller{}).prepareSnapshot("jp")
		testutil.RequireArgs(t, !(err == nil), "preparing an unconfigured snapshot should fail")
	}
	{

		got := (&Controller{}).resolveRegion("")
		testutil.Require(t, !(got != renderregion.JP), "zero controller region resolved to %s", got)
	}
	{

		_, err := (&Controller{}).SnapshotExpired("jp")
		testutil.RequireArgs(t, !(err == nil), "SnapshotExpired without data should fail")
	}

	badJSON := (&Controller{defaultRegion: renderregion.JP}).WithMySekaiData([]byte(`{`))
	{
		_, err := badJSON.ResolvePhoto(PhotoQuery{Seq: 1})
		testutil.RequireArgs(t, !(err == nil), "malformed direct MySekai JSON should fail")
	}
	testutil.RequireArgs(t, !((&Controller{}).mysekaiProfileCard(renderregion.JP, nil, nil, false) != nil), "profile card without an override or snapshot should be nil")
	{
		testutil.RequireArgs(t, !(nilController.currentMysekaiPhenomenaGroundColor(renderregion.JP, nil) != nil), "phenomena color without controller masterdata should be nil")
		testutil.RequireArgs(t, !((&Controller{}).currentMysekaiPhenomenaGroundColor(renderregion.JP, nil) != nil), "phenomena color without controller masterdata should be nil")
	}
	{
		testutil.RequireArgs(t, !(currentMysekaiPhenomenaID(renderregion.JP, nil) != 0), "invalid phenomena schedules should not resolve a phenomenon")
		testutil.RequireArgs(t, !(currentMysekaiPhenomenaID(renderregion.JP, map[string]any{"mysekaiPhenomenaSchedules": []any{"invalid"}}) != 0), "invalid phenomena schedules should not resolve a phenomenon")
	}
	{

		gateID, level, skin := extractMysekaiGateInfo(nil)
		{
			testutil.Require(t, !(gateID != 1), "default gate = %d, %d, %d", gateID, level, skin)
			testutil.Require(t, !(level != 1), "default gate = %d, %d, %d", gateID, level, skin)
			testutil.Require(t, !(skin != 0), "default gate = %d, %d, %d", gateID, level, skin)
		}
	}
	{

		gateID, level, skin := extractMysekaiGateInfo(map[string]any{"userMysekaiGateCharacterVisit": map[string]any{}})
		{
			testutil.Require(t, !(gateID != 1), "missing nested gate = %d, %d, %d", gateID, level, skin)
			testutil.Require(t, !(level != 1), "missing nested gate = %d, %d, %d", gateID, level, skin)
			testutil.Require(t, !(skin != 0), "missing nested gate = %d, %d, %d", gateID, level, skin)
		}
	}
	{

		gateID, level, skin := extractMysekaiGateInfo(map[string]any{"userMysekaiGateCharacterVisit": map[string]any{"userMysekaiGate": map[string]any{"mysekaiGateId": -1, "mysekaiGateLevel": 0, "mysekaiGateSkinId": 3}}})
		{
			testutil.Require(t, !(gateID != 1), "normalized gate = %d, %d, %d", gateID, level, skin)
			testutil.Require(t, !(level != 1), "normalized gate = %d, %d, %d", gateID, level, skin)
			testutil.Require(t, !(skin != 3), "normalized gate = %d, %d, %d", gateID, level, skin)
		}
	}

	winter := time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)
	summer := time.Date(2026, time.June, 15, 12, 0, 0, 0, time.UTC)
	{
		testutil.RequireArgs(t, !(mysekaiRegionUTCOffset("unknown", winter) != 8), "region UTC offset mismatch")
		testutil.RequireArgs(t, !(mysekaiRegionUTCOffset("en", winter) != -8), "region UTC offset mismatch")
		testutil.RequireArgs(t, !(mysekaiRegionUTCOffset("en", summer) != -7), "region UTC offset mismatch")
		testutil.RequireArgs(t, !(mysekaiRegionUTCOffset("jp", winter) != 9), "region UTC offset mismatch")
	}
	testutil.RequireArgs(t, !(resolveMysekaiSnapshotTimeMs(nil) != 0), "nil snapshot time should be zero")
	{

		got := resolveMysekaiSnapshotTimeMs(map[string]any{"now": 1, "updatedResources": map[string]any{"now": 3}, "upload_time": 2})
		testutil.Require(t, !(got != 3000), "best snapshot time = %d", got)
	}
	testutil.RequireArgs(t, !(isMysekaiSnapshotExpired(renderregion.JP, nil, time.Now())), "missing snapshot timestamp should not be expired")
	testutil.RequireArgs(t, !(mysekaiBirthdayPhenom(func(path string) string { return path }, "natural", winter, false).RefreshReason != "natural"), "birthday phenom without character suffix mismatch")
	testutil.RequireArgs(t, !(mysekaiBirthdayPhenom(func(path string) string { return path }, "bdstart_1", winter, true).TextFill[0] != 0), "current birthday phenom should use active colors")
	{
		testutil.RequireArgs(t, reflect.DeepEqual(resourceTextColor("mysekai_material_5", nil), []int{200, 50, 0}), "resource colors mismatch")
		testutil.RequireArgs(t, reflect.DeepEqual(resourceTextColor("mysekai_music_record_1", nil), []int{50, 0, 200}), "resource colors mismatch")
		testutil.RequireArgs(t, reflect.DeepEqual(resourceTextColor("plain", nil), []int{100, 100, 100}), "resource colors mismatch")
	}
	{
		testutil.RequireArgs(t, !(resourceRarity("material_174", nil) != 2), "resource rarity mismatch")
		testutil.RequireArgs(t, !(resourceRarity("mysekai_material_67", nil) != 2), "resource rarity mismatch")
		testutil.RequireArgs(t, !(resourceRarity("mysekai_material_32", nil) != 1), "resource rarity mismatch")
		testutil.RequireArgs(t, !(resourceRarity("mysekai_material_9", map[int]string{9: "rarity_3"}) != 2), "resource rarity mismatch")
		testutil.RequireArgs(t, !(resourceRarity("mysekai_material_10", map[int]string{10: "rarity_2"}) != 1), "resource rarity mismatch")
		testutil.RequireArgs(t, !(resourceRarity("other", nil) != 0), "resource rarity mismatch")
	}
	{
		testutil.RequireArgs(t, !(matchesResourceIDRange("other", "material_", 1, 2)), "resource range should reject wrong or invalid IDs")
		testutil.RequireArgs(t, !(matchesResourceIDRange("material_bad", "material_", 1, 2)), "resource range should reject wrong or invalid IDs")
	}
	{
		testutil.RequireArgs(t, !(musicRecordIconPath(func(path string) string { return path }, false) != nil), "music record icon mismatch")
		testutil.RequireArgs(t, !(*musicRecordIconPath(func(path string) string { return path }, true) != "mysekai/music_record.png"), "music record icon mismatch")
	}
	{
		testutil.RequireArgs(t, !(formatMysekaiQuantity(999) != "999"), "resource quantity formatting mismatch")
		testutil.RequireArgs(t, !(formatMysekaiQuantity(1_234) != "1k2"), "resource quantity formatting mismatch")
		testutil.RequireArgs(t, !(formatMysekaiQuantity(10_000) != "10k"), "resource quantity formatting mismatch")
	}
	{
		testutil.RequireArgs(t, !(adjustedResourceSortScore("plain", 3, nil) != 3), "resource sort score mismatch")
		testutil.RequireArgs(t, !(adjustedResourceSortScore("mysekai_material_5", 3, nil) <= adjustedResourceSortScore("plain", 3, nil)), "resource sort score mismatch")
	}

}

func TestHousingCompetitionGlobalPruningAndDefaultLimits(t *testing.T) {
	var nilCache *housingCompetitionStatsCache
	nilCache.pruneLocked(time.Time{})
	cache := &housingCompetitionStatsCache{
		buckets: map[housingCompetitionStatsCacheKey]*housingCompetitionStatsBucket{
			{Region: "jp", HousingID: 1}: nil,
			{Region: "jp", HousingID: 2}: {
				entries:   map[string]HousingCompetitionEntry{"a": {CacheKey: "a"}},
				sampledAt: time.Now().UTC(),
			},
		},
	}
	cache.pruneLocked(time.Time{})
	testutil.Require(t, !(len(cache.buckets) != 1), "default pruning = %+v", cache.buckets)

	now := time.Now().UTC()
	global := &housingCompetitionStatsCache{
		entryTTL:         time.Hour,
		maxEntries:       2,
		maxBucketEntries: 10,
		maxBuckets:       10,
		buckets: map[housingCompetitionStatsCacheKey]*housingCompetitionStatsBucket{
			{Region: "jp", HousingID: 1}: {
				refreshedAt: now.Add(-2 * time.Minute),
				entries: map[string]HousingCompetitionEntry{
					"a": {CacheKey: "a", ReviewCount: 1, LastSeenAt: 1},
					"b": {CacheKey: "b", ReviewCount: 2, LastSeenAt: 2},
				},
			},
			{Region: "tw", HousingID: 2}: {
				refreshedAt: now.Add(-time.Minute),
				entries: map[string]HousingCompetitionEntry{
					"c": {CacheKey: "c", ReviewCount: 3, LastSeenAt: 3},
					"d": {CacheKey: "d", ReviewCount: 4, LastSeenAt: 4},
				},
			},
		},
	}
	global.pruneLocked(now)
	total := 0
	for _, bucket := range global.buckets {
		total += len(bucket.entries)
	}
	testutil.Require(t, !(total != 2), "globally pruned entries = %+v", global.buckets)

}
