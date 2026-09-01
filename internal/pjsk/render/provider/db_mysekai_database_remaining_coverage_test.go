//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package provider

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/testutil"
)

func TestDBMySekaiProviderDatabaseAndFallbackPaths(t *testing.T) {
	ctx := context.Background()
	base := openProviderBehaviorDB(t, "remaining_mysekai")
	dsn := filepath.Join(t.TempDir(), "mysekai.sqlite")
	bootstrap, err := sql.Open("sqlite3", dsn)
	testutil.Require(t, !(err != nil), "open bootstrap SQLite database: %v", err)

	t.Cleanup(func() { _ = bootstrap.Close() })
	{
		_, err := bootstrap.ExecContext(ctx, `CREATE TABLE musics (
		id INTEGER PRIMARY KEY,
		game_id INTEGER,
		title TEXT,
		json_blob BLOB,
		raw_blob BLOB,
		number_value REAL,
		optional_value TEXT,
		server_region TEXT
	)`)
		testutil.Require(t, !(err != nil), "create raw music table: %v", err)
	}

	for _, args := range [][]any{
		{1, 100, "JP music", []byte(`{"nested":true}`), []byte(`not-json`), 1.5, nil, "jp"},
		{2, 0, "zero ID", []byte(`[1,2]`), []byte(`plain`), 2.5, "present", "jp"},
		{3, 200, "TW music", []byte(`null`), []byte(`other`), 3.5, nil, "tw"},
	} {
		{
			_, err := bootstrap.ExecContext(ctx, `INSERT INTO musics
			(id, game_id, title, json_blob, raw_blob, number_value, optional_value, server_region)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, args...)
			testutil.Require(t, !(err != nil), "insert raw music row: %v", err)
		}

	}

	var nilProvider *dbMySekaiProvider
	{
		testutil.RequireArgs(t, !(nilProvider.Configured()), "nil MySekai provider methods should be defensive")
		testutil.RequireArgs(t, !(nilProvider.LoadList("musics.json") != nil), "nil MySekai provider methods should be defensive")
		testutil.RequireArgs(t, !(nilProvider.LoadMapByID("musics.json") != nil), "nil MySekai provider methods should be defensive")
		testutil.RequireArgs(t, !(nilProvider.LoadObject("x.json", &map[string]any{})), "nil MySekai provider methods should be defensive")
		testutil.RequireArgs(t, !(nilProvider.Close() != nil), "nil MySekai provider methods should be defensive")
	}

	withoutDSN := newDBMySekaiProvider(base.client, renderregion.JP, databaseProviderConfig{})
	{
		testutil.Require(t, !(withoutDSN.dbType != "postgres"), "provider without DSN = %+v", withoutDSN)
		testutil.Require(t, !(withoutDSN.Configured()), "provider without DSN = %+v", withoutDSN)
	}

	unknownDriver := newDBMySekaiProvider(base.client, renderregion.JP, databaseProviderConfig{sekaiDBType: "definitely-missing", sekaiDSN: "ignored"})
	{
		testutil.RequireArgs(t, !(unknownDriver.db != nil), "unknown SQL driver should leave MySekai DB unconfigured")
		testutil.RequireArgs(t, !(unknownDriver.Configured()), "unknown SQL driver should leave MySekai DB unconfigured")
	}

	missingFile := newDBMySekaiProvider(base.client, renderregion.JP, databaseProviderConfig{sekaiDBType: "sqlite3", sekaiDSN: filepath.Join(t.TempDir(), "missing", "db.sqlite") + "?mode=rw"})
	testutil.RequireArgs(t, !(missingFile.db != nil), "failed SQLite ping should leave MySekai DB unset")

	p := newDBMySekaiProvider(base.client, renderregion.JP, databaseProviderConfig{sekaiDBType: " sqlite3 ", sekaiDSN: " " + dsn + " "})
	{
		testutil.Require(t, p.Configured(), "configured MySekai provider = %+v", p)
		testutil.Require(t, !(p.db == nil), "configured MySekai provider = %+v", p)
		testutil.Require(t, !(p.dbType != "sqlite3"), "configured MySekai provider = %+v", p)
	}

	t.Cleanup(func() { _ = p.Close() })

	items := p.LoadList("musics.json")
	{
		testutil.Require(t, !(len(items) != 2), "LoadList(musics.json) = %#v", items)
		testutil.Require(t, !(items[0]["id"] != int64(100)), "LoadList(musics.json) = %#v", items)
		testutil.Require(t, !(items[0]["title"] != "JP music"), "LoadList(musics.json) = %#v", items)
		testutil.Require(t, !(items[0]["optionalValue"] != nil), "LoadList(musics.json) = %#v", items)
	}
	{

		_, ok := items[0]["jsonBlob"].(map[string]any)
		{
			testutil.Require(t, ok, "normalized DB values = %#v", items[0])
			testutil.Require(t, !(items[0]["rawBlob"] != "not-json"), "normalized DB values = %#v", items[0])
			testutil.Require(t, !(items[0]["numberValue"] != float64(1.5)), "normalized DB values = %#v", items[0])
		}
	}

	items[0]["title"] = "cached mutation"
	{
		cached := p.LoadList("musics.json")
		testutil.Require(t, !(cached[0]["title"] != "cached mutation"), "cached DB list = %#v", cached)
	}

	byID := p.LoadMapByID("musics.json")
	{
		testutil.Require(t, !(len(byID) != 1), "LoadMapByID(musics.json) = %#v", byID)
		testutil.Require(t, !(byID[100]["title"] != "cached mutation"), "LoadMapByID(musics.json) = %#v", byID)
	}

	byID[100]["title"] = "map mutation"
	{
		cached := p.LoadMapByID("musics.json")
		testutil.Require(t, !(cached[100]["title"] != "map mutation"), "cached DB map = %#v", cached)
	}
	{

		got := p.LoadList("not-mapped.json")
		testutil.Require(t, !(got != nil), "unknown DB mapping without fallback = %#v", got)
	}
	{

		got := p.LoadMapByID("not-mapped.json")
		testutil.Require(t, !(got != nil), "unknown DB map without fallback = %#v", got)
	}

	root := t.TempDir()
	writeTestFile(t, root, "cards.json", `[{"id":7,"prefix":"fallback card"}]`)
	writeTestFile(t, root, "not-mapped.json", `[{"id":8,"name":"fallback unknown"}]`)
	writeTestFile(t, root, "object.json", `{"enabled":true,"count":2}`)
	p.local = &localMySekaiProvider{store: newLocalStore(root)}
	testutil.RequireArgs(t, p.Configured(), "DB + local MySekai provider should be configured")

	for i := 0; i < 2; i++ {
		fallback := p.LoadList("cards.json")
		{
			testutil.Require(t, !(len(fallback) != 1), "DB-error local fallback attempt %d = %#v", i, fallback)
			testutil.Require(t, !(fallback[0]["prefix"] != "fallback card"), "DB-error local fallback attempt %d = %#v", i, fallback)
		}

	}
	{
		_, unavailable := p.unavailable["cards.json"]
		testutil.RequireArgs(t, unavailable, "missing DB table should be negative-cached")
	}
	{

		fallback := p.LoadMapByID("cards.json")
		{
			testutil.Require(t, !(len(fallback) != 1), "fallback map = %#v", fallback)
			testutil.Require(t, !(fallback[7]["prefix"] != "fallback card"), "fallback map = %#v", fallback)
		}
	}
	{

		fallback := p.LoadList("not-mapped.json")
		{
			testutil.Require(t, !(len(fallback) != 1), "unknown mapping fallback = %#v", fallback)
			testutil.Require(t, !(fallback[0]["name"] != "fallback unknown"), "unknown mapping fallback = %#v", fallback)
		}
	}

	var object struct {
		Enabled bool `json:"enabled"`
		Count   int  `json:"count"`
	}
	{
		testutil.Require(t, p.LoadObject("object.json", &object), "local object fallback = %+v", object)
		testutil.Require(t, object.Enabled, "local object fallback = %+v", object)
		testutil.Require(t, !(object.Count != 2), "local object fallback = %+v", object)
		testutil.Require(t, !(p.LoadObject("missing.json", &object)), "local object fallback = %+v", object)
	}

	p.dbType = "postgres"
	postgresPlaceholderItems, err := p.queryTable("musics")
	{
		testutil.Require(t, !(err != nil), "Postgres-style placeholder query on SQLite = %#v, %v", postgresPlaceholderItems, err)
		testutil.Require(t, !(len(postgresPlaceholderItems) != 2), "Postgres-style placeholder query on SQLite = %#v, %v", postgresPlaceholderItems, err)
	}

	p.dbType = "sqlite3"
	{
		_, err := p.queryTable("missing_table")
		testutil.RequireArgs(t, !(err == nil), "querying a missing table should fail")
	}
	{

		err := p.Close()
		testutil.Require(t, !(err != nil), "first MySekai SQL provider close: %v", err)
	}
	{

		err := p.Close()
		testutil.Require(t, !(err != nil), "second MySekai SQL provider close: %v", err)
	}

}

func TestDBMySekaiColumnAndDatabaseProviderHelpers(t *testing.T) {
	{
		testutil.RequireArgs(t, !(mysekaiColumnKey("id") != ""), "MySekai DB column conversion mismatch")
		testutil.RequireArgs(t, !(mysekaiColumnKey("game_id") != "id"), "MySekai DB column conversion mismatch")
		testutil.RequireArgs(t, !(mysekaiColumnKey("server_region") != ""), "MySekai DB column conversion mismatch")
		testutil.RequireArgs(t, !(mysekaiColumnKey("fixture_main__genre") != "fixtureMainGenre"), "MySekai DB column conversion mismatch")
	}
	{
		testutil.RequireArgs(t, !(snakeToCamel("") != ""), "snakeToCamel conversion mismatch")
		testutil.RequireArgs(t, !(snakeToCamel("alreadyCamel") != "alreadyCamel"), "snakeToCamel conversion mismatch")
		testutil.RequireArgs(t, !(snakeToCamel("a__b_c") != "aBC"), "snakeToCamel conversion mismatch")
	}

	parsed := normalizeDBMasterdataValue([]byte(`{"value":1}`), nil, 0)
	{
		_, ok := parsed.(map[string]any)
		testutil.Require(t, ok, "normalized JSON blob = %#v", parsed)
	}
	{
		testutil.RequireArgs(t, !(normalizeDBMasterdataValue([]byte(`bad-json`), nil, 0) != "bad-json"), "DB masterdata scalar normalization mismatch")
		testutil.RequireArgs(t, !(normalizeDBMasterdataValue(int64(9), nil, 0) != int64(9)), "DB masterdata scalar normalization mismatch")
	}

	option := WithSekaiDatabase(" sqlite3 ", " data.sqlite ")
	option(nil)
	var cfg databaseProviderConfig
	option(&cfg)
	{
		testutil.Require(t, !(cfg.sekaiDBType != "sqlite3"), "WithSekaiDatabase config = %+v", cfg)
		testutil.Require(t, !(cfg.sekaiDSN != "data.sqlite"), "WithSekaiDatabase config = %+v", cfg)
	}
	testutil.RequireArgs(t, !(NewDatabaseProvider(nil, renderregion.JP) != nil), "nil DB client should produce nil provider")

	p := openProviderBehaviorDB(t, "remaining_database")
	testutil.Require(t, !(p.Region() != renderregion.JP), "database provider region = %s", p.Region())

	providers := []any{p.Cards(), p.Characters(), p.Skills(), p.Events(), p.Musics(), p.Gachas(), p.Costumes(), p.Honors(), p.Stamps(), p.VLives(), p.Education(), p.PlayerFrames(), p.MySekai()}
	for i, child := range providers {
		{
			testutil.Require(t, !(child == nil), "database child provider %d is nil", i)
			testutil.Require(t, !(reflect.ValueOf(child).IsNil()), "database child provider %d is nil", i)
		}

	}
	{
		testutil.RequireArgs(t, !((*DatabaseProvider)(nil).Close() != nil), "closing an unconfigured database provider should succeed")
		testutil.RequireArgs(t, !((&DatabaseProvider{}).Close() != nil), "closing an unconfigured database provider should succeed")
		testutil.RequireArgs(t, !(p.Close() != nil), "closing an unconfigured database provider should succeed")
	}

	(*DatabaseProvider)(nil).SetLocalMasterdataDir("ignored", false)
	p.SetLocalMasterdataDir(t.TempDir(), true)
	{
		testutil.RequireArgs(t, !(p.events.local == nil), "local masterdata providers were not configured")
		testutil.RequireArgs(t, !(p.events.store == nil), "local masterdata providers were not configured")
		testutil.RequireArgs(t, !(p.musics.local == nil), "local masterdata providers were not configured")
		testutil.RequireArgs(t, !(p.mysekai.local == nil), "local masterdata providers were not configured")
		testutil.RequireArgs(t, !(p.honors.store == nil), "local masterdata providers were not configured")
		testutil.RequireArgs(t, !(p.education.store == nil), "local masterdata providers were not configured")
	}

	p.SetLocalMasterdataDir(" ", false)
	{
		testutil.RequireArgs(t, !(p.events.local != nil), "blank local masterdata root should clear fallback providers")
		testutil.RequireArgs(t, !(p.events.store != nil), "blank local masterdata root should clear fallback providers")
		testutil.RequireArgs(t, !(p.musics.local != nil), "blank local masterdata root should clear fallback providers")
		testutil.RequireArgs(t, !(p.mysekai.local != nil), "blank local masterdata root should clear fallback providers")
		testutil.RequireArgs(t, !(p.honors.store != nil), "blank local masterdata root should clear fallback providers")
		testutil.RequireArgs(t, !(p.education.store != nil), "blank local masterdata root should clear fallback providers")
	}

	for _, test := range []struct {
		value string
		want  renderregion.Value
		ok    bool
	}{
		{value: "jp", want: renderregion.JP, ok: true},
		{value: "cn", want: renderregion.CN, ok: true},
		{value: "tw", want: renderregion.TW, ok: true},
		{value: "kr", want: renderregion.KR, ok: true},
		{value: "en", want: renderregion.EN, ok: true},
		{value: "unknown", want: renderregion.Unknown, ok: false},
	} {
		got, ok := parseMasterdataRegion(test.value)
		{
			testutil.Require(t, !(got != test.want), "parseMasterdataRegion(%q) = %s, %v", test.value, got, ok)
			testutil.Require(t, !(ok != test.ok), "parseMasterdataRegion(%q) = %s, %v", test.value, got, ok)
		}

	}
	{
		got, ok := regionForLocalMasterdataRepo(" HARUKI-SEKAI-SC-MASTER ")
		{
			testutil.Require(t, ok, "regionForLocalMasterdataRepo() = %s, %v", got, ok)
			testutil.Require(t, !(got != renderregion.CN), "regionForLocalMasterdataRepo() = %s, %v", got, ok)
		}
	}
	{

		got, ok := regionForLocalMasterdataRepo("missing")
		{
			testutil.Require(t, !(ok), "missing repo region = %s, %v", got, ok)
			testutil.Require(t, !(got != renderregion.Unknown), "missing repo region = %s, %v", got, ok)
		}
	}

	for _, test := range []struct {
		path string
		want renderregion.Value
		ok   bool
	}{
		{path: filepath.Join("root", "tw"), want: renderregion.TW, ok: true},
		{path: filepath.Join("root", "haruki-sekai-kr-master"), want: renderregion.KR, ok: true},
		{path: filepath.Join("root", "haruki-sekai-en-master", "master"), want: renderregion.EN, ok: true},
		{path: filepath.Join("root", "plain"), want: renderregion.Unknown, ok: false},
	} {
		got, ok := inferLocalMasterdataDirRegion(test.path)
		{
			testutil.Require(t, !(got != test.want), "inferLocalMasterdataDirRegion(%q) = %s, %v", test.path, got, ok)
			testutil.Require(t, !(ok != test.ok), "inferLocalMasterdataDirRegion(%q) = %s, %v", test.path, got, ok)
		}

	}
	{
		dirs := localMasterdataCandidateDirs(filepath.Join(t.TempDir(), "haruki-sekai-sc-master", "master"), renderregion.JP)
		testutil.RequireArgs(t, !(len(dirs) == 0), "candidate masterdata directories should not be empty")
	}

}

func TestDBMySekaiQueryWhitelistsMatchFileMapping(t *testing.T) {
	{
		testutil.Require(t, !(len(mysekaiPostgresTableQueries) != len(mysekaiFileToTable)), "query whitelist sizes: postgres=%d question=%d mappings=%d",
			len(mysekaiPostgresTableQueries),
			len(mysekaiQuestionMarkTableQueries),
			len(mysekaiFileToTable))
		testutil.Require(t, !(len(mysekaiQuestionMarkTableQueries) != len(mysekaiFileToTable)), "query whitelist sizes: postgres=%d question=%d mappings=%d",
			len(mysekaiPostgresTableQueries),
			len(mysekaiQuestionMarkTableQueries),
			len(mysekaiFileToTable))
	}

	mappedTables := make(map[string]struct{}, len(mysekaiFileToTable))
	for filename, table := range mysekaiFileToTable {
		mappedTables[table] = struct{}{}
		postgresQuery, postgresOK := mysekaiPostgresTableQueries[table]
		questionQuery, questionOK := mysekaiQuestionMarkTableQueries[table]
		if !postgresOK || !questionOK {
			t.Errorf("file mapping %q references table %q without both fixed queries", filename, table)
			continue
		}
		wantPostgres := `SELECT * FROM "` + table + `" WHERE server_region = $1`
		wantQuestion := `SELECT * FROM ` + table + ` WHERE server_region = ?`
		testutil.Check(t, !(postgresQuery != wantPostgres || questionQuery != wantQuestion), "fixed queries for table %q = (%q, %q), want (%q, %q)", table, postgresQuery, questionQuery, wantPostgres, wantQuestion)

	}
	for table := range mysekaiPostgresTableQueries {
		{
			_, ok := mappedTables[table]
			testutil.Check(t, ok, "Postgres query for unmapped table %q", table)
		}

	}
	for table := range mysekaiQuestionMarkTableQueries {
		{
			_, ok := mappedTables[table]
			testutil.Check(t, ok, "question-mark query for unmapped table %q", table)
		}

	}

	injectedTable := `musics"; DROP TABLE musics; --`
	{
		_, err := queryMySekaiTable(nil, "postgres", injectedTable, "jp")
		testutil.RequireArgs(t, !(err == nil), "Postgres query accepted an unlisted table identifier")
	}
	{

		_, err := queryMySekaiTable(nil, "sqlite3", injectedTable, "jp")
		testutil.RequireArgs(t, !(err == nil), "question-mark query accepted an unlisted table identifier")
	}

}

func TestProviderAdapterBaseRemainingBranches(t *testing.T) {
	p := openProviderBehaviorDB(t, "remaining_adapter")
	base := NewProviderAdapterBase(p)
	{
		testutil.RequireArgs(t, !(base.DefaultRegion() != renderregion.JP), "provider adapter defaults mismatch")
		testutil.RequireArgs(t, !(base.Context() == nil), "provider adapter defaults mismatch")
	}

	var nilBase *PjskProviderAdapterBase
	testutil.RequireArgs(t, !(nilBase.Context() == nil), "nil provider adapter should supply a context")

	base.Ctx = nil
	testutil.RequireArgs(t, !(base.Context() == nil), "adapter with nil context should supply a context")

	cloned := base.CloneWithContext(context.Background())
	{
		testutil.Require(t, !(cloned.P != p), "cloned provider adapter = %+v", cloned)
		testutil.Require(t, !(cloned.Ctx == nil), "cloned provider adapter = %+v", cloned)
	}

	cloned = base.CloneWithContext(nil)
	{
		testutil.Require(t, !(cloned.P != p), "nil-context provider adapter clone = %+v", cloned)
		testutil.Require(t, !(cloned.Ctx == nil), "nil-context provider adapter clone = %+v", cloned)
	}

}
