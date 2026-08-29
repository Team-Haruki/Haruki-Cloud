package mysekai

import (
	"context"
	"reflect"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestTalkNicknameAndProfileDataSourceEdgeBranches(t *testing.T) {
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

	if controller.lookupTalkCharacterID("") != 0 || controller.lookupTalkCharacterID("alias") != 9 {
		t.Fatal("talk nickname lookup mismatch")
	}
	for _, query := range []string{"Hoshino", "Ichika", "HoshinoIchika", "Hoshino Ichika", "Star", "Singer", "StarSinger", "Star Singer"} {
		if got := controller.lookupTalkCharacterID(query); got != 1 {
			t.Fatalf("lookupTalkCharacterID(%q) = %d", query, got)
		}
	}
	if controller.lookupTalkCharacterID("missing") != 0 {
		t.Fatal("missing talk character should not resolve")
	}

	units := controller.masterdata.loadList("gameCharacterUnits.json")
	if characterID, unitID, err := controller.resolveTalkCharacter("101"); err != nil || characterID != 1 || unitID != 101 {
		t.Fatalf("resolve numeric unit = %d, %d, %v", characterID, unitID, err)
	}
	if _, _, err := controller.resolveTalkCharacter("404"); err == nil {
		t.Fatal("missing numeric character should fail")
	}
	if _, _, err := controller.resolveTalkCharacter("missing"); err == nil {
		t.Fatal("missing named character should fail")
	}
	if _, _, err := controller.resolveTalkCharacterUnit("missing", "", 404, units); err == nil {
		t.Fatal("character without units should fail")
	}
	if _, _, err := controller.resolveTalkCharacterUnit("miku", "", 21, units); err == nil {
		t.Fatal("Miku with multiple units should require a unit")
	}
	if characterID, unitID, err := controller.resolveTalkCharacterUnit("miku", "idol", 21, units); err != nil || characterID != 21 || unitID != 202 {
		t.Fatalf("Miku idol unit = %d, %d, %v", characterID, unitID, err)
	}
	if _, _, err := controller.resolveTalkCharacterUnit("rin", "street", 22, units); err == nil {
		t.Fatal("fixed virtual singer unit mismatch should fail")
	}
	if characterID, unitID, err := controller.resolveTalkCharacterUnit("rin", "idol", 22, units); err != nil || characterID != 22 || unitID != 220 {
		t.Fatalf("fixed virtual singer unit = %d, %d, %v", characterID, unitID, err)
	}
	if unit, query := extractMysekaiTalkUnit("  "); unit != "" || query != "" {
		t.Fatalf("empty talk unit = %q, %q", unit, query)
	}
	if unit, query := extractMysekaiTalkUnit("ln Ichika"); unit != "light_sound" || query != "Ichika" {
		t.Fatalf("aliased talk unit = %q, %q", unit, query)
	}
	if normalizeMysekaiTalkUnit(" LN ") != "light_sound" || normalizeMysekaiTalkUnit(" Custom ") != "custom" {
		t.Fatal("talk unit normalization mismatch")
	}

	if id, ok := ResolveNicknameCharacterID(" "); ok || id != 0 {
		t.Fatalf("blank nickname = %d, %v", id, ok)
	}
	if id, ok := ResolveNicknameCharacterID("prefix miku suffix"); !ok || id != 21 {
		t.Fatalf("token nickname = %d, %v", id, ok)
	}
	if id, ok := ResolveNicknameCharacterID("definitely-missing"); ok || id != 0 {
		t.Fatalf("missing nickname = %d, %v", id, ok)
	}

	mergeMySekaiDataSources(nil, map[string]any{}, false)
	profile := &drawing.ProfileCardRequest{DataSources: []drawing.ProfileDataSource{{Name: "Suite"}}}
	mergeMySekaiDataSources(profile, map[string]any{}, true)
	if profile.DataSources[0].Name != "Mysekai数据" {
		t.Fatalf("source-less replacement = %+v", profile.DataSources)
	}
	profile = &drawing.ProfileCardRequest{DataSources: []drawing.ProfileDataSource{{Name: "Mysekai数据"}}}
	mergeMySekaiDataSources(profile, map[string]any{"source": "toolbox"}, false)
	if profile.DataSources[0].Source == nil || *profile.DataSources[0].Source != "toolbox" {
		t.Fatalf("existing MySekai source = %+v", profile.DataSources)
	}
	mode := "full"
	profile = &drawing.ProfileCardRequest{DataSources: []drawing.ProfileDataSource{{Name: "Suite", Mode: &mode}}}
	mergeMySekaiDataSources(profile, map[string]any{"source": "toolbox", "local_source": "cache", "upload_time": int64(1700000000)}, true)
	if len(profile.DataSources) != 1 || profile.DataSources[0].Source == nil || *profile.DataSources[0].Source != "toolbox(cache)" || profile.DataSources[0].Mode == nil {
		t.Fatalf("single-source replacement = %+v", profile.DataSources)
	}
	profile = &drawing.ProfileCardRequest{DataSources: []drawing.ProfileDataSource{{Name: "Suite"}, {Name: "Public"}}}
	mergeMySekaiDataSources(profile, map[string]any{"local_source": "local"}, false)
	if len(profile.DataSources) != 3 || *profile.DataSources[2].Source != "local" {
		t.Fatalf("appended MySekai source = %+v", profile.DataSources)
	}
	replaceWithMySekaiDataSource(nil, nil)
	profile = &drawing.ProfileCardRequest{}
	replaceWithMySekaiDataSource(profile, map[string]any{})
	if len(profile.DataSources) != 1 || profile.DataSources[0].Name != "Mysekai数据" {
		t.Fatalf("fallback replacement source = %+v", profile.DataSources)
	}
	stripProfileDataSourceDetails(nil)
	source := "source"
	profile.DataSources = []drawing.ProfileDataSource{{Source: &source, Mode: &mode}}
	stripProfileDataSourceDetails(profile)
	if profile.DataSources[0].Source != nil || profile.DataSources[0].Mode != nil {
		t.Fatalf("stripped data source = %+v", profile.DataSources)
	}
}

func TestTimeResourceAndSnapshotHelperEdges(t *testing.T) {
	var nilController *Controller
	var nilResolver *masterdataResolver
	if nilResolver.ResolveContext(context.Background(), renderregion.JP) != nil {
		t.Fatal("nil masterdata resolver should return nil")
	}
	if nilController.ensure() == nil || nilController.ensureMasterdata() == nil || nilController.ensureSnapshot() == nil {
		t.Fatal("nil controller ensure methods should fail")
	}
	if (&Controller{}).ensure() == nil {
		t.Fatal("controller without snapshot should fail ensure")
	}
	if _, _, err := (&Controller{}).prepareSnapshot("jp"); err == nil {
		t.Fatal("preparing an unconfigured snapshot should fail")
	}
	if got := (&Controller{}).resolveRegion(""); got != renderregion.JP {
		t.Fatalf("zero controller region resolved to %s", got)
	}
	if _, err := (&Controller{}).SnapshotExpired("jp"); err == nil {
		t.Fatal("SnapshotExpired without data should fail")
	}
	badJSON := (&Controller{defaultRegion: renderregion.JP}).WithMySekaiData([]byte(`{`))
	if _, err := badJSON.ResolvePhoto(PhotoQuery{Seq: 1}); err == nil {
		t.Fatal("malformed direct MySekai JSON should fail")
	}
	if (&Controller{}).mysekaiProfileCard(renderregion.JP, nil, nil, false) != nil {
		t.Fatal("profile card without an override or snapshot should be nil")
	}
	if nilController.currentMysekaiPhenomenaGroundColor(renderregion.JP, nil) != nil || (&Controller{}).currentMysekaiPhenomenaGroundColor(renderregion.JP, nil) != nil {
		t.Fatal("phenomena color without controller masterdata should be nil")
	}
	if currentMysekaiPhenomenaID(renderregion.JP, nil) != 0 || currentMysekaiPhenomenaID(renderregion.JP, map[string]any{"mysekaiPhenomenaSchedules": []any{"invalid"}}) != 0 {
		t.Fatal("invalid phenomena schedules should not resolve a phenomenon")
	}

	if gateID, level, skin := extractMysekaiGateInfo(nil); gateID != 1 || level != 1 || skin != 0 {
		t.Fatalf("default gate = %d, %d, %d", gateID, level, skin)
	}
	if gateID, level, skin := extractMysekaiGateInfo(map[string]any{"userMysekaiGateCharacterVisit": map[string]any{}}); gateID != 1 || level != 1 || skin != 0 {
		t.Fatalf("missing nested gate = %d, %d, %d", gateID, level, skin)
	}
	if gateID, level, skin := extractMysekaiGateInfo(map[string]any{"userMysekaiGateCharacterVisit": map[string]any{"userMysekaiGate": map[string]any{"mysekaiGateId": -1, "mysekaiGateLevel": 0, "mysekaiGateSkinId": 3}}}); gateID != 1 || level != 1 || skin != 3 {
		t.Fatalf("normalized gate = %d, %d, %d", gateID, level, skin)
	}

	winter := time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)
	summer := time.Date(2026, time.June, 15, 12, 0, 0, 0, time.UTC)
	if mysekaiRegionUTCOffset("unknown", winter) != 8 || mysekaiRegionUTCOffset("en", winter) != -8 || mysekaiRegionUTCOffset("en", summer) != -7 || mysekaiRegionUTCOffset("jp", winter) != 9 {
		t.Fatal("region UTC offset mismatch")
	}
	if resolveMysekaiSnapshotTimeMs(nil) != 0 {
		t.Fatal("nil snapshot time should be zero")
	}
	if got := resolveMysekaiSnapshotTimeMs(map[string]any{"now": 1, "updatedResources": map[string]any{"now": 3}, "upload_time": 2}); got != 3000 {
		t.Fatalf("best snapshot time = %d", got)
	}
	if isMysekaiSnapshotExpired(renderregion.JP, nil, time.Now()) {
		t.Fatal("missing snapshot timestamp should not be expired")
	}
	if mysekaiBirthdayPhenom(func(path string) string { return path }, "natural", winter, false).RefreshReason != "natural" {
		t.Fatal("birthday phenom without character suffix mismatch")
	}
	if mysekaiBirthdayPhenom(func(path string) string { return path }, "bdstart_1", winter, true).TextFill[0] != 0 {
		t.Fatal("current birthday phenom should use active colors")
	}

	if !reflect.DeepEqual(resourceTextColor("mysekai_material_5", nil), []int{200, 50, 0}) || !reflect.DeepEqual(resourceTextColor("mysekai_music_record_1", nil), []int{50, 0, 200}) || !reflect.DeepEqual(resourceTextColor("plain", nil), []int{100, 100, 100}) {
		t.Fatal("resource colors mismatch")
	}
	if resourceRarity("material_174", nil) != 2 || resourceRarity("mysekai_material_67", nil) != 2 || resourceRarity("mysekai_material_32", nil) != 1 || resourceRarity("mysekai_material_9", map[int]string{9: "rarity_3"}) != 2 || resourceRarity("mysekai_material_10", map[int]string{10: "rarity_2"}) != 1 || resourceRarity("other", nil) != 0 {
		t.Fatal("resource rarity mismatch")
	}
	if matchesResourceIDRange("other", "material_", 1, 2) || matchesResourceIDRange("material_bad", "material_", 1, 2) {
		t.Fatal("resource range should reject wrong or invalid IDs")
	}
	if musicRecordIconPath(func(path string) string { return path }, false) != nil || *musicRecordIconPath(func(path string) string { return path }, true) != "mysekai/music_record.png" {
		t.Fatal("music record icon mismatch")
	}
	if formatMysekaiQuantity(999) != "999" || formatMysekaiQuantity(1_234) != "1k2" || formatMysekaiQuantity(10_000) != "10k" {
		t.Fatal("resource quantity formatting mismatch")
	}
	if adjustedResourceSortScore("plain", 3, nil) != 3 || adjustedResourceSortScore("mysekai_material_5", 3, nil) <= adjustedResourceSortScore("plain", 3, nil) {
		t.Fatal("resource sort score mismatch")
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
	if len(cache.buckets) != 1 {
		t.Fatalf("default pruning = %+v", cache.buckets)
	}

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
	if total != 2 {
		t.Fatalf("globally pruned entries = %+v", global.buckets)
	}
}
