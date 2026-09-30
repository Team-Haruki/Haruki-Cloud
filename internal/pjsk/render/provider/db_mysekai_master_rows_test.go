package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/testutil"

	_ "github.com/mattn/go-sqlite3"
)

// openMasterRowsProvider opens an ent client and a DatabaseProvider whose raw
// row store shares the same in-memory SQLite database.
func openMasterRowsProvider(t *testing.T, name string) *DatabaseProvider {
	t.Helper()
	dsn := fmt.Sprintf("file:master_rows_%s_%d?mode=memory&cache=shared&_fk=1", name, time.Now().UnixNano())
	client := sekaienttest.Open(t, "sqlite3", dsn)
	p := NewDatabaseProvider(client, renderregion.JP, WithSekaiDatabase("sqlite3", dsn))
	testutil.Require(t, p.mysekai != nil && p.mysekai.db != nil, "raw row store did not open on %s", dsn)
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestMasterRowsServeCustomProfileTablesFromDatabaseFirst(t *testing.T) {
	ctx := context.Background()
	p := openMasterRowsProvider(t, "custom_profile")
	client := p.client

	_, err := client.Customprofiletextcolor.Create().SetGameID(10).SetSeq(1).SetColorCode("#ffffff").SetServerRegion("jp").Save(ctx)
	testutil.Require(t, err == nil, "create text color: %v", err)
	_, err = client.Customprofiletextcolor.Create().SetGameID(11).SetSeq(2).SetColorCode("#000000").SetServerRegion("tw").Save(ctx)
	testutil.Require(t, err == nil, "create tw text color: %v", err)
	_, err = client.Omikuji.Create().SetGameID(183).SetOmikujiGroupID(1).SetUnit("idol").SetFortuneType("grate_fortune").
		SetSummary("line1\nline2").SetTitle1("願望").SetDescription1("必ず叶う").SetOmikujiCoverFilePath("omikuji_cover").SetServerRegion("jp").Save(ctx)
	testutil.Require(t, err == nil, "create omikuji: %v", err)

	root := t.TempDir()
	writeTestFile(t, root, "customProfileTextColors.json", `[{"id":10,"seq":1,"colorCode":"#local"}]`)
	p.mysekai.local = &localMySekaiProvider{store: newLocalStore(root)}

	rows, ok := p.mysekai.LoadMasterRows(ctx, "customProfileTextColors.json")
	testutil.Require(t, ok, "text colors were not served")
	testutil.Require(t, len(rows) == 1 && rows[10]["colorCode"] == "#ffffff", "text colors = %#v, want the JP database row", rows)
	_, hasIdentity := rows[10]["game_id"]
	testutil.Require(t, !hasIdentity && rows[10]["id"] == int64(10), "row keys must use the game JSON names: %#v", rows[10])

	omikujis, ok := p.mysekai.LoadMasterRows(ctx, "omikujis.json")
	testutil.Require(t, ok && len(omikujis) == 1, "omikujis = %#v", omikujis)
	row := omikujis[183]
	testutil.Require(t, row["summary"] == "line1\nline2" && row["title1"] == "願望" && row["omikujiCoverFilePath"] == "omikuji_cover", "omikuji row = %#v", row)
	_, hasTitle2 := row["title2"]
	testutil.Require(t, !hasTitle2, "NULL columns must be absent like missing JSON keys: %#v", row)
}

func TestMasterRowsFallBackToLocalWhenTableIsEmptyOrMissing(t *testing.T) {
	ctx := context.Background()
	p := openMasterRowsProvider(t, "fallback")
	root := t.TempDir()
	writeTestFile(t, root, "customProfileTextFonts.json", `[{"id":3,"name":"local","fontName":"FOT-Local"}]`)
	writeTestFile(t, root, "mysekaiGateSkins.json", `[{"id":5,"mysekaiGateSkinType":"unit","mysekaiGateSkinTypeId":2}]`)

	// Existing but empty table (created by the schema, not yet ingested).
	rows, ok := p.mysekai.LoadMasterRows(ctx, "customProfileTextFonts.json")
	testutil.Require(t, ok && len(rows) == 0, "empty table without local = %#v ok=%v", rows, ok)

	p.mysekai.local = &localMySekaiProvider{store: newLocalStore(root)}
	p.mysekai.resetLocalMasterdataCache()
	rows, ok = p.mysekai.LoadMasterRows(ctx, "customProfileTextFonts.json")
	testutil.Require(t, ok && len(rows) == 1 && rows[3]["fontName"] == "FOT-Local", "empty table with local = %#v ok=%v", rows, ok)
	list := p.mysekai.LoadListContext(ctx, "mysekaiGateSkins.json")
	testutil.Require(t, len(list) == 1, "gate skins from local = %#v", list)

	// A table that exists but is empty is still "served"; a file that maps
	// to no table and has no local copy is not.
	_, ok = p.mysekai.LoadMasterRows(ctx, "customProfileShapeResources.json")
	testutil.Require(t, ok, "existing empty shape table reported unserved")
	_, ok = p.mysekai.LoadMasterRows(ctx, "notAMasterTable.json")
	testutil.Require(t, !ok, "unmapped file without a local copy reported ok")

	_, err := p.client.Customprofiletextfont.Create().SetGameID(3).SetName("db").SetFontName("FOT-DB").SetServerRegion("jp").Save(ctx)
	testutil.Require(t, err == nil, "create font: %v", err)
	rows, _ = p.mysekai.LoadMasterRows(ctx, "customProfileTextFonts.json")
	testutil.Require(t, rows[3]["fontName"] == "FOT-Local", "cached fallback rows were dropped: %#v", rows)
	p.mysekai.resetLocalMasterdataCache()
	rows, _ = p.mysekai.LoadMasterRows(ctx, "customProfileTextFonts.json")
	testutil.Require(t, rows[3]["fontName"] == "FOT-DB", "database rows must win after reset: %#v", rows)
}

func TestMasterRowsWhitelistCoversForwardedTables(t *testing.T) {
	for _, filename := range []string{
		"customProfileCharacterIconResources.json", "customProfileCollectionResources.json", "customProfileEtcResources.json",
		"customProfileGeneralBackgroundResources.json", "customProfileMaterialResources.json", "customProfileMemberStandingPictureResources.json",
		"customProfilePlayerInfoResources.json", "customProfileShapeResources.json", "customProfileStoryBackgroundResources.json",
		"customProfileTextColors.json", "customProfileTextFonts.json", "customProfileUserInterfaceIconResources.json",
		"omikujis.json", "unitStoryEpisodeGroups.json", "eventStories.json",
		"mysekaiGateSkins.json", "mysekaiGateUnitSkins.json", "mysekaiGateCommonSkins.json", "customMusicScoreTags.json",
	} {
		table, ok := mysekaiFileToTable[filename]
		testutil.Require(t, ok, "%s is not mapped to a table", filename)
		_, pg := mysekaiPostgresTableQueries[table]
		_, qm := mysekaiQuestionMarkTableQueries[table]
		testutil.Require(t, pg && qm, "%s (%s) is missing a fixed query: postgres=%v question=%v", filename, table, pg, qm)
	}
}

func TestMasterRowsTolerateMissingNewTable(t *testing.T) {
	ctx := context.Background()
	p := openMasterRowsProvider(t, "missing_table")

	// honorwords is a JP 7.0.0 table this database does not have yet: it is
	// reported unserved (so callers treat the region as lacking the data)
	// and a local copy still answers.
	rows, ok := p.mysekai.LoadMasterRows(ctx, "honorWords.json")
	testutil.Require(t, !ok && len(rows) == 0, "missing table = %#v ok=%v", rows, ok)

	root := t.TempDir()
	writeTestFile(t, root, "honorWords.json", `[{"id":10101,"honorGroupId":1,"assetbundleName":"honor_word_01_01"}]`)
	p.mysekai.local = &localMySekaiProvider{store: newLocalStore(root)}
	rows, ok = p.mysekai.LoadMasterRows(ctx, "honorWords.json")
	testutil.Require(t, ok && len(rows) == 1 && rows[10101]["assetbundleName"] == "honor_word_01_01", "missing table with local = %#v ok=%v", rows, ok)
}

func TestPlayerShopRawTablesWithoutEntSchema(t *testing.T) {
	p := openMasterRowsProvider(t, "player_shop")
	for _, statement := range []string{
		`CREATE TABLE mysekaitools (game_id INTEGER, server_region TEXT, name TEXT, assetbundle_name TEXT)`,
		`INSERT INTO mysekaitools VALUES (10,'jp','Chainsaw','ax0005'),(10,'tw','Other','other')`,
		`CREATE TABLE mysekaiblueprintshops (server_region TEXT, mysekai_blueprint_shop_item_lottery_type TEXT, consume_jewel_quantity INTEGER, purchase_limit INTEGER)`,
		`INSERT INTO mysekaiblueprintshops VALUES ('jp','daily',100,5),('jp','weekly',100,3),('tw','daily',200,5)`,
		`CREATE TABLE mysekaimaterialpossessions (game_id INTEGER, server_region TEXT, level INTEGER, possession_limit INTEGER)`,
		`INSERT INTO mysekaimaterialpossessions VALUES (1,'jp',1,10000)`,
	} {
		if _, err := p.mysekai.db.ExecContext(context.Background(), statement); err != nil {
			t.Fatal(err)
		}
	}
	tools := p.mysekai.LoadListContext(context.Background(), "mysekaiTools.json")
	if len(tools) != 1 || tools[0]["assetbundleName"] != "ax0005" {
		t.Fatal(tools)
	}
	rules := p.mysekai.LoadListContext(context.Background(), "mysekaiBlueprintShops.json")
	if len(rules) != 2 || rules[0]["consumeJewelQuantity"] != int64(100) {
		t.Fatal(rules)
	}
	capacity := p.mysekai.LoadListContext(context.Background(), "mysekaiMaterialPossessions.json")
	if len(capacity) != 1 || capacity[0]["possessionLimit"] != int64(10000) {
		t.Fatal(capacity)
	}
}

func TestPlayerFramePartsUseRegionGroupAndMasterdataRefresh(t *testing.T) {
	p := openMasterRowsProvider(t, "player_frame_parts")
	for _, statement := range []string{
		`CREATE TABLE playerframeparts (game_id INTEGER, player_frame_group_id INTEGER, game_character_id INTEGER, server_region TEXT)`,
		`INSERT INTO playerframeparts VALUES (301,2,1,'jp'),(407,2,2,'jp'),(999,2,1,'en'),(888,3,1,'jp')`,
	} {
		if _, err := p.mysekai.db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	parts, err := p.PlayerFrames().GetPartsByGroupID(t.Context(), 2)
	if err != nil || len(parts) != 2 || parts[1] != 301 || parts[2] != 407 {
		t.Fatalf("parts: %+v, %v", parts, err)
	}
	if _, err := p.mysekai.db.ExecContext(t.Context(), `UPDATE playerframeparts SET game_id=302 WHERE game_id=301`); err != nil {
		t.Fatal(err)
	}
	p.ResetMasterdataCache()
	parts, err = p.PlayerFrames().GetPartsByGroupID(t.Context(), 2)
	if err != nil || parts[1] != 302 {
		t.Fatalf("stale frame parts after refresh: %+v, %v", parts, err)
	}
}
