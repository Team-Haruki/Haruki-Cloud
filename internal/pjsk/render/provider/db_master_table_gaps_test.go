package provider

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/testutil"
)

func TestDBEducationProviderReadsCharacterMissionsFromDatabaseFirst(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "characterMissionV2s.json", `[
		{"id":901,"characterId":5,"characterMissionType":"local_only","parameterGroupId":101,"isAchievementMission":false}
	]`)

	provider := openProviderBehaviorDB(t, "education_missions_db")
	provider.education.store = newLocalStore(root)
	client := provider.client

	for _, item := range []struct {
		gameID, characterID, parameterGroupID int64
		missionType                           string
		region                                renderregion.Value
	}{
		{gameID: 501, characterID: 5, parameterGroupID: 101, missionType: "leader", region: renderregion.JP},
		{gameID: 502, characterID: 5, parameterGroupID: 102, missionType: "play_live", region: renderregion.JP},
		{gameID: 503, characterID: 5, parameterGroupID: 103, missionType: "other_region", region: renderregion.TW},
	} {
		_, err := client.Charactermissionv2.Create().
			SetGameID(item.gameID).
			SetCharacterID(item.characterID).
			SetCharacterMissionType(item.missionType).
			SetParameterGroupID(item.parameterGroupID).
			SetIsAchievementMission(true).
			SetServerRegion(item.region.String()).
			Save(ctx)
		testutil.Require(t, err == nil, "create character mission %d: %v", item.gameID, err)
	}
	_, err := client.Charactermissionv2Parametergroup.Create().
		SetGameID(101).SetSeq(1).SetRequirement(20).SetExp(30).SetQuantity(2).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx)
	testutil.Require(t, err == nil, "create parameter group: %v", err)

	missions := provider.education.GetCharacterMissions(ctx, 5)
	testutil.Require(t, len(missions) == 2, "character missions = %+v, want the two JP database rows", missions)
	testutil.Require(t, missions[0].ID == 501 && missions[0].CharacterMissionType == "leader", "character missions = %+v", missions)
	testutil.Require(t, missions[1].ID == 502 && missions[1].ParameterGroupID == 102, "character missions = %+v", missions)
	for _, mission := range missions {
		testutil.Require(t, mission.CharacterMissionType != "local_only", "local mission leaked over database rows: %+v", missions)
	}
}

func TestDBEducationProviderCharacterMissionsFallBackWhenTableEmpty(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "characterMissionV2s.json", `[
		{"id":901,"characterId":5,"characterMissionType":"local_only","parameterGroupId":101,"isAchievementMission":false}
	]`)

	provider := openProviderBehaviorDB(t, "education_missions_empty")
	provider.education.store = newLocalStore(root)

	missions := provider.education.GetCharacterMissions(ctx, 5)
	testutil.Require(t, len(missions) == 1 && missions[0].ID == 901, "empty table should fall back to local missions, got %+v", missions)
}

func TestDBEducationProviderFillsResourceBoxDetailsFromDatabaseInInsertOrder(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "resourceBoxDetails.json", `[
		{"resourceBoxId":300,"resourceBoxPurpose":"challenge","resourceType":"local_only","resourceQuantity":1},
		{"resourceBoxId":301,"resourceBoxPurpose":"challenge","resourceType":"local_fill","resourceId":9,"resourceQuantity":7}
	]`)

	dsn := fmt.Sprintf("file:provider_box_details_cn_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	client := sekaienttest.Open(t, "sqlite3", dsn)
	provider := NewDatabaseProvider(client, renderregion.CN)
	provider.education.store = newLocalStore(root)
	rawDB, err := sql.Open("sqlite3", dsn)
	testutil.Require(t, err == nil, "open raw sqlite handle: %v", err)
	defer rawDB.Close()

	for _, boxID := range []int64{300, 301} {
		_, err := client.Resourceboxe.Create().
			SetGameID(boxID).
			SetResourceBoxPurpose("challenge").
			SetResourceBoxType("expand").
			SetServerRegion(renderregion.CN.String()).
			Save(ctx)
		testutil.Require(t, err == nil, "create cn resource box %d: %v", boxID, err)
	}

	// Rows are inserted out of id order: the reader must still return them
	// by id, which is the ingest (file) order.
	for _, row := range []struct {
		id           int
		resourceType string
		resourceID   int64
		quantity     int64
		level        int64
		region       renderregion.Value
	}{
		{id: 3, resourceType: "jewel", resourceID: 0, quantity: 100, region: renderregion.CN},
		{id: 1, resourceType: "material", resourceID: 15, quantity: 2, region: renderregion.CN},
		{id: 2, resourceType: "skill_practice_ticket", resourceID: 4, quantity: 1, level: 2, region: renderregion.CN},
		{id: 4, resourceType: "other_region", resourceID: 1, quantity: 1, region: renderregion.TW},
	} {
		// The create builder has no SetID (identity column), so the explicit
		// id goes through SQL on the same shared in-memory database.
		_, err := rawDB.ExecContext(ctx,
			`INSERT INTO resourceboxdetails (id, resource_box_id, resource_quantity, resource_id, resource_box_purpose, resource_level, resource_type, server_region) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			row.id, 300, row.quantity, row.resourceID, "challenge", row.level, row.resourceType, row.region.String(),
		)
		testutil.Require(t, err == nil, "create resource box detail %d: %v", row.id, err)
	}

	box := provider.education.GetResourceBoxByPurpose(ctx, "challenge", 300)
	testutil.Require(t, box != nil, "cn resource box 300 missing")
	testutil.Require(t, len(box.Details) == 3, "cn resource box details = %+v, want three database rows", box.Details)
	testutil.Require(t, box.Details[0].ResourceType == "material" && box.Details[0].ResourceID == 15 && box.Details[0].ResourceQuantity == 2, "details out of id order: %+v", box.Details)
	testutil.Require(t, box.Details[1].ResourceType == "skill_practice_ticket" && box.Details[1].ResourceLevel == 2, "details out of id order: %+v", box.Details)
	testutil.Require(t, box.Details[2].ResourceType == "jewel" && box.Details[2].ResourceQuantity == 100, "details out of id order: %+v", box.Details)

	// A box with no database rows still takes the local file as secondary
	// fill while the fallback store is configured.
	localFilled := provider.education.GetResourceBoxByPurpose(ctx, "challenge", 301)
	testutil.Require(t, localFilled != nil && len(localFilled.Details) == 1, "local secondary fill = %+v", localFilled)
	testutil.Require(t, localFilled.Details[0].ResourceType == "local_fill" && localFilled.Details[0].ResourceID == 9, "local secondary fill = %+v", localFilled)
}

func TestDBEducationProviderKeepsInlineResourceBoxDetails(t *testing.T) {
	ctx := context.Background()
	client := openProviderBehaviorDB(t, "education_box_details_jp").client
	provider := NewDatabaseProvider(client, renderregion.JP)

	_, err := client.Resourceboxe.Create().
		SetGameID(400).
		SetResourceBoxPurpose("shop").
		SetResourceBoxType("expand").
		SetDetails([]byte(`[{"resourceType":"coin","resourceId":1,"resourceQuantity":5}]`)).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx)
	testutil.Require(t, err == nil, "create jp resource box: %v", err)
	_, err = client.Resourceboxdetail.Create().
		SetResourceBoxID(400).
		SetResourceBoxPurpose("shop").
		SetResourceType("must_not_override").
		SetResourceQuantity(1).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx)
	testutil.Require(t, err == nil, "create jp resource box detail: %v", err)

	box := provider.education.GetResourceBoxByPurpose(ctx, "shop", 400)
	testutil.Require(t, box != nil && len(box.Details) == 1, "jp resource box = %+v", box)
	testutil.Require(t, box.Details[0].ResourceType == "coin", "inline details were overridden: %+v", box.Details)
}

func TestDBHonorProviderReadsBondsHonorWordsFromDatabaseFirst(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "bondsHonorWords.json", `[
		{"ID":30,"Seq":1,"BondsGroupID":20,"AssetBundleName":"word_30_local","Name":"Local","Description":"local"},
		{"ID":31,"Seq":2,"BondsGroupID":20,"AssetBundleName":"word_31_local","Name":"Local only","Description":"local"}
	]`)

	provider := openProviderBehaviorDB(t, "honors_bonds_words_db")
	provider.honors.store = newLocalStore(root)
	client := provider.client

	for _, row := range []struct {
		gameID int64
		name   string
		region renderregion.Value
	}{
		{gameID: 30, name: "Together", region: renderregion.JP},
		{gameID: 32, name: "Other region", region: renderregion.CN},
	} {
		_, err := client.Bondshonorword.Create().
			SetGameID(row.gameID).
			SetSeq(1).
			SetBondsGroupID(20).
			SetAssetbundleName("word_30").
			SetName(row.name).
			SetDescription("db").
			SetServerRegion(row.region.String()).
			Save(ctx)
		testutil.Require(t, err == nil, "create bonds honor word %d: %v", row.gameID, err)
	}

	word, err := provider.honors.GetBondsHonorWordByID(ctx, 30)
	testutil.Require(t, err == nil && word != nil, "bonds honor word = %+v, %v", word, err)
	testutil.Require(t, word.Name == "Together" && word.AssetBundleName == "word_30" && word.BondsGroupID == 20, "bonds honor word should come from the database: %+v", word)

	_, err = provider.honors.GetBondsHonorWordByID(ctx, 31)
	testutil.Require(t, err != nil, "local-only word must not be merged once the database has rows")
	_, err = provider.honors.GetBondsHonorWordByID(ctx, 32)
	testutil.Require(t, err != nil, "other-region word must not resolve")

	empty := openProviderBehaviorDB(t, "honors_bonds_words_empty")
	empty.honors.store = newLocalStore(root)
	fallback, err := empty.honors.GetBondsHonorWordByID(ctx, 31)
	testutil.Require(t, err == nil && fallback != nil && fallback.Name == "Local only", "empty table should fall back to local words: %+v, %v", fallback, err)

	unconfigured := openProviderBehaviorDB(t, "honors_bonds_words_unconfigured")
	_, err = unconfigured.honors.GetBondsHonorWordByID(ctx, 30)
	testutil.Require(t, err != nil, "empty table without a store must report not configured")
}

func TestDBEventProviderReadsWorldBloomRewardRangesFromDatabaseFirst(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "worldBloomChapterRankingRewardRanges.json", `[
		{"id":1,"eventId":99,"gameCharacterId":5,"fromRank":1,"toRank":10,"resourceBoxId":500},
		{"id":2,"eventId":77,"gameCharacterId":7,"fromRank":11,"toRank":20,"resourceBoxId":501}
	]`)

	provider := openProviderBehaviorDB(t, "events_wb_ranges_db")
	provider.events.store = newLocalStore(root)
	client := provider.client

	for _, row := range []struct {
		gameID, eventID, characterID, fromRank, toRank, boxID int64
		region                                                renderregion.Value
	}{
		{gameID: 11, eventID: 99, characterID: 5, fromRank: 11, toRank: 50, boxID: 601, region: renderregion.JP},
		{gameID: 10, eventID: 99, characterID: 5, fromRank: 1, toRank: 10, boxID: 600, region: renderregion.JP},
		{gameID: 12, eventID: 99, characterID: 5, fromRank: 1, toRank: 1, boxID: 602, region: renderregion.TW},
	} {
		_, err := client.Worldbloomchapterrankingrewardrange.Create().
			SetGameID(row.gameID).
			SetEventID(row.eventID).
			SetGameCharacterID(row.characterID).
			SetFromRank(row.fromRank).
			SetToRank(row.toRank).
			SetIsToRankBorder(false).
			SetResourceBoxID(row.boxID).
			SetServerRegion(row.region.String()).
			Save(ctx)
		testutil.Require(t, err == nil, "create world bloom reward range %d: %v", row.gameID, err)
	}

	ranges, err := provider.events.GetWorldBloomChapterRankingRewardRanges(ctx, 99, 5)
	testutil.Require(t, err == nil && len(ranges) == 2, "database world bloom ranges = %+v, %v", ranges, err)
	testutil.Require(t, ranges[0].ToRank == 10 && ranges[0].ResourceBoxID == 600, "database ranges should sort by to_rank: %+v", ranges)
	testutil.Require(t, ranges[1].ToRank == 50 && ranges[1].ResourceBoxID == 601 && ranges[1].ID == 11, "database ranges should sort by to_rank: %+v", ranges)

	ranges[0].ResourceBoxID = -1
	cached, err := provider.events.GetWorldBloomChapterRankingRewardRanges(ctx, 99, 5)
	testutil.Require(t, err == nil && cached[0].ResourceBoxID == 600, "returned ranges alias the provider cache: %+v, %v", cached, err)

	// A chapter the database does not know still falls back to the local file.
	local, err := provider.events.GetWorldBloomChapterRankingRewardRanges(ctx, 77, 7)
	testutil.Require(t, err == nil && len(local) == 1 && local[0].ResourceBoxID == 501, "local world bloom ranges = %+v, %v", local, err)

	provider.ResetMasterdataCache()
	provider.events.wbRangeMu.RLock()
	loaded := provider.events.wbRangesLoaded
	provider.events.wbRangeMu.RUnlock()
	testutil.Require(t, !loaded, "reset must drop the world bloom range cache")
	again, err := provider.events.GetWorldBloomChapterRankingRewardRanges(ctx, 99, 5)
	testutil.Require(t, err == nil && len(again) == 2, "reload after reset = %+v, %v", again, err)

	empty := openProviderBehaviorDB(t, "events_wb_ranges_empty")
	empty.events.store = newLocalStore(root)
	fallback, err := empty.events.GetWorldBloomChapterRankingRewardRanges(ctx, 99, 5)
	testutil.Require(t, err == nil && len(fallback) == 1 && fallback[0].ResourceBoxID == 500, "empty table should fall back to local ranges: %+v, %v", fallback, err)
}

// gapTableFixture opens a shared in-memory database whose tables can be
// dropped and recreated to simulate a transient query failure.
type gapTableFixture struct {
	dsn      string
	provider *DatabaseProvider
	raw      *sql.DB
}

func newGapTableFixture(t *testing.T, name string, region renderregion.Value) *gapTableFixture {
	t.Helper()
	dsn := fmt.Sprintf("file:provider_gap_%s_%d?mode=memory&cache=shared&_fk=1", name, time.Now().UnixNano())
	client := sekaienttest.Open(t, "sqlite3", dsn)
	raw, err := sql.Open("sqlite3", dsn)
	testutil.Require(t, err == nil, "open raw sqlite handle: %v", err)
	t.Cleanup(func() { raw.Close() })
	return &gapTableFixture{dsn: dsn, provider: NewDatabaseProvider(client, region), raw: raw}
}

func (f *gapTableFixture) dropTable(t *testing.T, table string) {
	t.Helper()
	_, err := f.raw.Exec("DROP TABLE " + table)
	testutil.Require(t, err == nil, "drop %s: %v", table, err)
}

func (f *gapTableFixture) recreateTables(t *testing.T) {
	t.Helper()
	err := f.provider.client.Schema.Create(context.Background())
	testutil.Require(t, err == nil, "recreate schema: %v", err)
}

func TestDBEducationProviderMissionQueryErrorIsNotCached(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "characterMissionV2s.json", `[
		{"id":901,"characterId":5,"characterMissionType":"local_only","parameterGroupId":101,"isAchievementMission":false}
	]`)
	fixture := newGapTableFixture(t, "missions_error", renderregion.JP)
	provider := fixture.provider
	provider.education.store = newLocalStore(root)

	fixture.dropTable(t, "charactermissionv2s")
	testutil.Require(t, provider.education.GetCharacterMissions(ctx, 5) == nil, "query error must not serve local missions as loaded")
	provider.education.missionMu.RLock()
	loaded := provider.education.leaderMissionsLoaded
	provider.education.missionMu.RUnlock()
	testutil.Require(t, !loaded, "query error must not mark missions loaded")

	fixture.recreateTables(t)
	_, err := provider.client.Charactermissionv2.Create().
		SetGameID(501).SetCharacterID(5).SetCharacterMissionType("leader").SetParameterGroupID(101).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx)
	testutil.Require(t, err == nil, "create mission: %v", err)
	missions := provider.education.GetCharacterMissions(ctx, 5)
	testutil.Require(t, len(missions) == 1 && missions[0].ID == 501, "next call must retry and read the database: %+v", missions)

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	provider.ResetMasterdataCache()
	missions = provider.education.GetCharacterMissions(cancelled, 5)
	testutil.Require(t, len(missions) == 1, "cache fill must run detached from a cancelled request context: %+v", missions)
}

func TestDBEducationProviderResourceBoxDetailQueryErrorIsNotCached(t *testing.T) {
	ctx := context.Background()
	fixture := newGapTableFixture(t, "box_details_error", renderregion.CN)
	provider := fixture.provider
	client := provider.client

	_, err := client.Resourceboxe.Create().
		SetGameID(300).SetResourceBoxPurpose("challenge").SetResourceBoxType("expand").
		SetServerRegion(renderregion.CN.String()).
		Save(ctx)
	testutil.Require(t, err == nil, "create cn resource box: %v", err)

	fixture.dropTable(t, "resourceboxdetails")
	testutil.Require(t, provider.education.GetResourceBoxByPurpose(ctx, "challenge", 300) == nil, "detail query error must not serve boxes without contents")
	provider.education.boxMu.RLock()
	loaded := provider.education.boxesLoaded
	provider.education.boxMu.RUnlock()
	testutil.Require(t, !loaded, "detail query error must not mark boxes loaded")

	fixture.recreateTables(t)
	_, err = client.Resourceboxdetail.Create().
		SetResourceBoxID(300).SetResourceBoxPurpose("challenge").SetResourceType("jewel").SetResourceQuantity(100).
		SetServerRegion(renderregion.CN.String()).
		Save(ctx)
	testutil.Require(t, err == nil, "create detail: %v", err)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	box := provider.education.GetResourceBoxByPurpose(cancelled, "challenge", 300)
	testutil.Require(t, box != nil && len(box.Details) == 1 && box.Details[0].ResourceType == "jewel", "next call must retry detached from the request context: %+v", box)
}

func TestDBHonorProviderBondsWordQueryErrorIsNotCached(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "bondsHonorWords.json", `[
		{"ID":30,"Seq":1,"BondsGroupID":20,"AssetBundleName":"word_30_local","Name":"Local","Description":"local"}
	]`)
	fixture := newGapTableFixture(t, "bonds_words_error", renderregion.JP)
	provider := fixture.provider
	provider.honors.store = newLocalStore(root)

	fixture.dropTable(t, "bondshonorwords")
	_, err := provider.honors.GetBondsHonorWordByID(ctx, 30)
	testutil.Require(t, err != nil, "query error must not serve the local file as loaded")
	provider.honors.bondsWordMu.RLock()
	loaded := provider.honors.bondsWordLoaded
	provider.honors.bondsWordMu.RUnlock()
	testutil.Require(t, !loaded, "query error must not mark bonds words loaded")

	fixture.recreateTables(t)
	_, err = provider.client.Bondshonorword.Create().
		SetGameID(30).SetName("Together").SetServerRegion(renderregion.JP.String()).
		Save(ctx)
	testutil.Require(t, err == nil, "create bonds word: %v", err)
	word, err := provider.honors.GetBondsHonorWordByID(ctx, 30)
	testutil.Require(t, err == nil && word != nil && word.Name == "Together", "next call must retry and read the database: %+v, %v", word, err)
}

func TestDBEventProviderWorldBloomRangesCacheEmptyAndRetryErrors(t *testing.T) {
	ctx := context.Background()
	fixture := newGapTableFixture(t, "wb_ranges_error", renderregion.JP)
	provider := fixture.provider

	fixture.dropTable(t, "worldbloomchapterrankingrewardranges")
	ranges, err := provider.events.GetWorldBloomChapterRankingRewardRanges(ctx, 99, 5)
	testutil.Require(t, err == nil && ranges == nil, "query error yields no ranges: %+v, %v", ranges, err)
	provider.events.wbRangeMu.RLock()
	loaded := provider.events.wbRangesLoaded
	provider.events.wbRangeMu.RUnlock()
	testutil.Require(t, !loaded, "query error must not mark ranges loaded")

	fixture.recreateTables(t)
	ranges, err = provider.events.GetWorldBloomChapterRankingRewardRanges(ctx, 99, 5)
	testutil.Require(t, err == nil && ranges == nil, "empty table yields no ranges: %+v, %v", ranges, err)
	provider.events.wbRangeMu.RLock()
	loaded = provider.events.wbRangesLoaded
	provider.events.wbRangeMu.RUnlock()
	testutil.Require(t, loaded, "an empty table must be cached until the next reset")

	_, err = provider.client.Worldbloomchapterrankingrewardrange.Create().
		SetGameID(10).SetEventID(99).SetGameCharacterID(5).SetFromRank(1).SetToRank(10).SetResourceBoxID(600).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx)
	testutil.Require(t, err == nil, "create range: %v", err)
	ranges, err = provider.events.GetWorldBloomChapterRankingRewardRanges(ctx, 99, 5)
	testutil.Require(t, err == nil && ranges == nil, "cached empty result must hold until reset: %+v, %v", ranges, err)
	provider.ResetMasterdataCache()
	ranges, err = provider.events.GetWorldBloomChapterRankingRewardRanges(ctx, 99, 5)
	testutil.Require(t, err == nil && len(ranges) == 1 && ranges[0].ResourceBoxID == 600, "reset must pick up the ingested rows: %+v, %v", ranges, err)
}
