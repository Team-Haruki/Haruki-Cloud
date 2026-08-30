package mysekai

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"haruki-cloud/internal/observability/commandtrace"

	_ "github.com/mattn/go-sqlite3"
)

func newTestDBMasterdataStore(t *testing.T) *dbMasterdataStore {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE musics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		game_id INTEGER NOT NULL,
		server_region TEXT NOT NULL,
		title TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create musics table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO musics (game_id, server_region, title) VALUES (1, 'jp', 'Test Song')`); err != nil {
		t.Fatalf("insert music: %v", err)
	}
	return &dbMasterdataStore{
		db:     db,
		region: "jp",
		ctx:    context.Background(),
		cache: &dbMasterdataCache{
			lists:    make(map[string][]map[string]any),
			mapsByID: make(map[string]map[int]map[string]any),
		},
	}
}

func TestMasterdataTableQueryWhitelistMatchesFileMapping(t *testing.T) {
	if len(masterdataTableQueries) != len(fileToTable) {
		t.Fatalf("query whitelist has %d entries for %d file mappings", len(masterdataTableQueries), len(fileToTable))
	}

	mappedTables := make(map[string]struct{}, len(fileToTable))
	for filename, table := range fileToTable {
		mappedTables[table] = struct{}{}
		query, ok := masterdataTableQueries[table]
		if !ok {
			t.Errorf("file mapping %q references table %q without a fixed query", filename, table)
			continue
		}
		want := `SELECT * FROM "` + table + `" WHERE server_region = $1`
		if query != want {
			t.Errorf("fixed query for table %q = %q, want %q", table, query, want)
		}
	}
	for table := range masterdataTableQueries {
		if _, ok := mappedTables[table]; !ok {
			t.Errorf("fixed query for unmapped table %q", table)
		}
	}

	if _, err := queryMasterdataTable(context.Background(), nil, `musics"; DROP TABLE musics; --`, "jp"); err == nil {
		t.Fatal("unlisted table identifier should be rejected before reaching the database")
	}
}

func TestDBMasterdataStoreWithContextTracesColdLoad(t *testing.T) {
	store := newTestDBMasterdataStore(t)
	ctx, trace := commandtrace.WithTrace(context.Background())
	controller := (&Controller{masterdata: store}).WithContext(ctx)
	bound, ok := controller.masterdata.(*dbMasterdataStore)
	if !ok {
		t.Fatalf("bound masterdata type = %T", controller.masterdata)
	}
	if bound.cache != store.cache {
		t.Fatal("request clone did not share the protected masterdata cache")
	}

	items := bound.loadList("musics.json")
	if len(items) != 1 || intNumber(items[0]["id"], 0) != 1 {
		t.Fatalf("unexpected masterdata items: %+v", items)
	}

	operations := make(map[string]int)
	for _, operation := range trace.Snapshot().Operations {
		operations[operation.Name] = operation.Count
	}
	for _, name := range []string{"mysekai.masterdata_query", "mysekai.masterdata_decode"} {
		if operations[name] != 1 {
			t.Fatalf("%s count = %d, operations=%+v", name, operations[name], trace.Snapshot().Operations)
		}
	}
}

func TestDBMasterdataStoreColdLoadHonorsRequestCancellation(t *testing.T) {
	store := newTestDBMasterdataStore(t)
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := store.WithContext(canceledCtx).(*dbMasterdataStore)
	if items := canceled.loadList("musics.json"); items != nil {
		t.Fatalf("canceled cold load returned items: %+v", items)
	}
	if !errors.Is(canceledCtx.Err(), context.Canceled) {
		t.Fatalf("context error = %v", canceledCtx.Err())
	}

	active := store.WithContext(context.Background()).(*dbMasterdataStore)
	if items := active.loadList("musics.json"); len(items) != 1 {
		t.Fatalf("canceled load poisoned shared cache: %+v", items)
	}
}

func TestDBMasterdataStoreContextClonesShareCacheSafely(t *testing.T) {
	store := newTestDBMasterdataStore(t)
	const readers = 12
	var wg sync.WaitGroup
	errs := make(chan error, readers)
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			clone := store.WithContext(context.Background()).(*dbMasterdataStore)
			if items := clone.loadList("musics.json"); len(items) != 1 {
				errs <- errors.New("unexpected masterdata result")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestControllerResetMasterdataCacheReloadsAllDBViews(t *testing.T) {
	store := newTestDBMasterdataStore(t)
	resolver := &masterdataResolver{cache: map[string]masterdataSource{"jp": store}}
	controller := &Controller{masterdata: store, resolver: resolver}

	items := store.loadList("musics.json")
	byID := store.loadMapByID("musics.json")
	if got := stringValue(items[0]["title"]); got != "Test Song" {
		t.Fatalf("initial list title = %q", got)
	}
	if got := stringValue(byID[1]["title"]); got != "Test Song" {
		t.Fatalf("initial map title = %q", got)
	}

	if _, err := store.db.Exec(`UPDATE musics SET title = 'Updated Song' WHERE game_id = 1`); err != nil {
		t.Fatalf("update music: %v", err)
	}
	if got := stringValue(store.loadList("musics.json")[0]["title"]); got != "Test Song" {
		t.Fatalf("cache unexpectedly refreshed before reset: %q", got)
	}

	controller.ResetMasterdataCache()

	items = store.loadList("musics.json")
	byID = store.loadMapByID("musics.json")
	if got := stringValue(items[0]["title"]); got != "Updated Song" {
		t.Fatalf("reloaded list title = %q", got)
	}
	if got := stringValue(byID[1]["title"]); got != "Updated Song" {
		t.Fatalf("reloaded map title = %q", got)
	}
}
