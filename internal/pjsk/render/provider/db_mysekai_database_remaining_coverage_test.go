//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package provider

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestDBMySekaiProviderDatabaseAndFallbackPaths(t *testing.T) {
	ctx := context.Background()
	base := openProviderBehaviorDB(t, "remaining_mysekai")
	dsn := filepath.Join(t.TempDir(), "mysekai.sqlite")
	bootstrap, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open bootstrap SQLite database: %v", err)
	}
	t.Cleanup(func() { _ = bootstrap.Close() })
	if _, err := bootstrap.ExecContext(ctx, `CREATE TABLE musics (
		id INTEGER PRIMARY KEY,
		game_id INTEGER,
		title TEXT,
		json_blob BLOB,
		raw_blob BLOB,
		number_value REAL,
		optional_value TEXT,
		server_region TEXT
	)`); err != nil {
		t.Fatalf("create raw music table: %v", err)
	}
	for _, args := range [][]any{
		{1, 100, "JP music", []byte(`{"nested":true}`), []byte(`not-json`), 1.5, nil, "jp"},
		{2, 0, "zero ID", []byte(`[1,2]`), []byte(`plain`), 2.5, "present", "jp"},
		{3, 200, "TW music", []byte(`null`), []byte(`other`), 3.5, nil, "tw"},
	} {
		if _, err := bootstrap.ExecContext(ctx, `INSERT INTO musics
			(id, game_id, title, json_blob, raw_blob, number_value, optional_value, server_region)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, args...); err != nil {
			t.Fatalf("insert raw music row: %v", err)
		}
	}

	var nilProvider *dbMySekaiProvider
	if nilProvider.Configured() || nilProvider.LoadList("musics.json") != nil || nilProvider.LoadMapByID("musics.json") != nil || nilProvider.LoadObject("x.json", &map[string]any{}) || nilProvider.Close() != nil {
		t.Fatal("nil MySekai provider methods should be defensive")
	}
	withoutDSN := newDBMySekaiProvider(base.client, renderregion.JP, databaseProviderConfig{})
	if withoutDSN.dbType != "postgres" || withoutDSN.Configured() {
		t.Fatalf("provider without DSN = %+v", withoutDSN)
	}
	unknownDriver := newDBMySekaiProvider(base.client, renderregion.JP, databaseProviderConfig{sekaiDBType: "definitely-missing", sekaiDSN: "ignored"})
	if unknownDriver.db != nil || unknownDriver.Configured() {
		t.Fatal("unknown SQL driver should leave MySekai DB unconfigured")
	}
	missingFile := newDBMySekaiProvider(base.client, renderregion.JP, databaseProviderConfig{sekaiDBType: "sqlite3", sekaiDSN: filepath.Join(t.TempDir(), "missing", "db.sqlite") + "?mode=rw"})
	if missingFile.db != nil {
		t.Fatal("failed SQLite ping should leave MySekai DB unset")
	}

	p := newDBMySekaiProvider(base.client, renderregion.JP, databaseProviderConfig{sekaiDBType: " sqlite3 ", sekaiDSN: " " + dsn + " "})
	if !p.Configured() || p.db == nil || p.dbType != "sqlite3" {
		t.Fatalf("configured MySekai provider = %+v", p)
	}
	t.Cleanup(func() { _ = p.Close() })

	items := p.LoadList("musics.json")
	if len(items) != 2 || items[0]["id"] != int64(100) || items[0]["title"] != "JP music" || items[0]["optionalValue"] != nil {
		t.Fatalf("LoadList(musics.json) = %#v", items)
	}
	if _, ok := items[0]["jsonBlob"].(map[string]any); !ok || items[0]["rawBlob"] != "not-json" || items[0]["numberValue"] != float64(1.5) {
		t.Fatalf("normalized DB values = %#v", items[0])
	}
	items[0]["title"] = "cached mutation"
	if cached := p.LoadList("musics.json"); cached[0]["title"] != "cached mutation" {
		t.Fatalf("cached DB list = %#v", cached)
	}
	byID := p.LoadMapByID("musics.json")
	if len(byID) != 1 || byID[100]["title"] != "cached mutation" {
		t.Fatalf("LoadMapByID(musics.json) = %#v", byID)
	}
	byID[100]["title"] = "map mutation"
	if cached := p.LoadMapByID("musics.json"); cached[100]["title"] != "map mutation" {
		t.Fatalf("cached DB map = %#v", cached)
	}
	if got := p.LoadList("not-mapped.json"); got != nil {
		t.Fatalf("unknown DB mapping without fallback = %#v", got)
	}
	if got := p.LoadMapByID("not-mapped.json"); got != nil {
		t.Fatalf("unknown DB map without fallback = %#v", got)
	}

	root := t.TempDir()
	writeTestFile(t, root, "cards.json", `[{"id":7,"prefix":"fallback card"}]`)
	writeTestFile(t, root, "not-mapped.json", `[{"id":8,"name":"fallback unknown"}]`)
	writeTestFile(t, root, "object.json", `{"enabled":true,"count":2}`)
	p.local = &localMySekaiProvider{store: newLocalStore(root)}
	if !p.Configured() {
		t.Fatal("DB + local MySekai provider should be configured")
	}
	for i := 0; i < 2; i++ {
		fallback := p.LoadList("cards.json")
		if len(fallback) != 1 || fallback[0]["prefix"] != "fallback card" {
			t.Fatalf("DB-error local fallback attempt %d = %#v", i, fallback)
		}
	}
	if _, unavailable := p.unavailable["cards.json"]; !unavailable {
		t.Fatal("missing DB table should be negative-cached")
	}
	if fallback := p.LoadMapByID("cards.json"); len(fallback) != 1 || fallback[7]["prefix"] != "fallback card" {
		t.Fatalf("fallback map = %#v", fallback)
	}
	if fallback := p.LoadList("not-mapped.json"); len(fallback) != 1 || fallback[0]["name"] != "fallback unknown" {
		t.Fatalf("unknown mapping fallback = %#v", fallback)
	}
	var object struct {
		Enabled bool `json:"enabled"`
		Count   int  `json:"count"`
	}
	if !p.LoadObject("object.json", &object) || !object.Enabled || object.Count != 2 || p.LoadObject("missing.json", &object) {
		t.Fatalf("local object fallback = %+v", object)
	}

	p.dbType = "postgres"
	postgresPlaceholderItems, err := p.queryTable("musics")
	if err != nil || len(postgresPlaceholderItems) != 2 {
		t.Fatalf("Postgres-style placeholder query on SQLite = %#v, %v", postgresPlaceholderItems, err)
	}
	p.dbType = "sqlite3"
	if _, err := p.queryTable("missing_table"); err == nil {
		t.Fatal("querying a missing table should fail")
	}
	if err := p.Close(); err != nil {
		t.Fatalf("first MySekai SQL provider close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second MySekai SQL provider close: %v", err)
	}
}

func TestDBMySekaiColumnAndDatabaseProviderHelpers(t *testing.T) {
	if mysekaiColumnKey("id") != "" || mysekaiColumnKey("game_id") != "id" || mysekaiColumnKey("server_region") != "" || mysekaiColumnKey("fixture_main__genre") != "fixtureMainGenre" {
		t.Fatal("MySekai DB column conversion mismatch")
	}
	if snakeToCamel("") != "" || snakeToCamel("alreadyCamel") != "alreadyCamel" || snakeToCamel("a__b_c") != "aBC" {
		t.Fatal("snakeToCamel conversion mismatch")
	}
	parsed := normalizeDBMasterdataValue([]byte(`{"value":1}`), nil, 0)
	if _, ok := parsed.(map[string]any); !ok {
		t.Fatalf("normalized JSON blob = %#v", parsed)
	}
	if normalizeDBMasterdataValue([]byte(`bad-json`), nil, 0) != "bad-json" || normalizeDBMasterdataValue(int64(9), nil, 0) != int64(9) {
		t.Fatal("DB masterdata scalar normalization mismatch")
	}

	option := WithSekaiDatabase(" sqlite3 ", " data.sqlite ")
	option(nil)
	var cfg databaseProviderConfig
	option(&cfg)
	if cfg.sekaiDBType != "sqlite3" || cfg.sekaiDSN != "data.sqlite" {
		t.Fatalf("WithSekaiDatabase config = %+v", cfg)
	}
	if NewDatabaseProvider(nil, renderregion.JP) != nil {
		t.Fatal("nil DB client should produce nil provider")
	}
	p := openProviderBehaviorDB(t, "remaining_database")
	if p.Region() != renderregion.JP {
		t.Fatalf("database provider region = %s", p.Region())
	}
	providers := []any{p.Cards(), p.Characters(), p.Skills(), p.Events(), p.Musics(), p.Gachas(), p.Costumes(), p.Honors(), p.Stamps(), p.VLives(), p.Education(), p.PlayerFrames(), p.MySekai()}
	for i, child := range providers {
		if child == nil || reflect.ValueOf(child).IsNil() {
			t.Fatalf("database child provider %d is nil", i)
		}
	}
	if (*DatabaseProvider)(nil).Close() != nil || (&DatabaseProvider{}).Close() != nil || p.Close() != nil {
		t.Fatal("closing an unconfigured database provider should succeed")
	}
	(*DatabaseProvider)(nil).SetLocalMasterdataDir("ignored", false)
	p.SetLocalMasterdataDir(t.TempDir(), true)
	if p.events.local == nil || p.events.store == nil || p.musics.local == nil || p.mysekai.local == nil || p.honors.store == nil || p.education.store == nil {
		t.Fatal("local masterdata providers were not configured")
	}
	p.SetLocalMasterdataDir(" ", false)
	if p.events.local != nil || p.events.store != nil || p.musics.local != nil || p.mysekai.local != nil || p.honors.store != nil || p.education.store != nil {
		t.Fatal("blank local masterdata root should clear fallback providers")
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
		if got != test.want || ok != test.ok {
			t.Fatalf("parseMasterdataRegion(%q) = %s, %v", test.value, got, ok)
		}
	}
	if got, ok := regionForLocalMasterdataRepo(" HARUKI-SEKAI-SC-MASTER "); !ok || got != renderregion.CN {
		t.Fatalf("regionForLocalMasterdataRepo() = %s, %v", got, ok)
	}
	if got, ok := regionForLocalMasterdataRepo("missing"); ok || got != renderregion.Unknown {
		t.Fatalf("missing repo region = %s, %v", got, ok)
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
		if got != test.want || ok != test.ok {
			t.Fatalf("inferLocalMasterdataDirRegion(%q) = %s, %v", test.path, got, ok)
		}
	}
	if dirs := localMasterdataCandidateDirs(filepath.Join(t.TempDir(), "haruki-sekai-sc-master", "master"), renderregion.JP); len(dirs) == 0 {
		t.Fatal("candidate masterdata directories should not be empty")
	}
}

func TestProviderAdapterBaseRemainingBranches(t *testing.T) {
	p := openProviderBehaviorDB(t, "remaining_adapter")
	base := NewProviderAdapterBase(p)
	if base.DefaultRegion() != renderregion.JP || base.Context() == nil {
		t.Fatal("provider adapter defaults mismatch")
	}
	var nilBase *PjskProviderAdapterBase
	if nilBase.Context() == nil {
		t.Fatal("nil provider adapter should supply a context")
	}
	base.Ctx = nil
	if base.Context() == nil {
		t.Fatal("adapter with nil context should supply a context")
	}
	cloned := base.CloneWithContext(context.Background())
	if cloned.P != p || cloned.Ctx == nil {
		t.Fatalf("cloned provider adapter = %+v", cloned)
	}
	cloned = base.CloneWithContext(nil)
	if cloned.P != p || cloned.Ctx == nil {
		t.Fatalf("nil-context provider adapter clone = %+v", cloned)
	}
}
