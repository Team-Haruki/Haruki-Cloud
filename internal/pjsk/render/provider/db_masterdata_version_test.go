package provider

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"

	_ "github.com/mattn/go-sqlite3"
)

func openIngestVersionDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:ingest_version_%d?mode=memory&cache=shared", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE master_ingest_version (
		target text NOT NULL, region text NOT NULL, content_hash text, data_version text,
		started_at timestamp, finished_at timestamp, status text NOT NULL, error text,
		PRIMARY KEY (target, region))`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMasterdataIngestHashReadsCommittedVersionOnly(t *testing.T) {
	ctx := context.Background()
	db := openIngestVersionDB(t)
	if _, ok := masterdataIngestHash(ctx, db, "sqlite3", "jp"); ok {
		t.Fatal("no row must read as unknown")
	}
	for _, row := range []struct {
		target, region, hash, status string
		finished                     string
	}{
		{"a", "jp", "sha256:jp-1", "ok", "2026-10-01 00:00:00"},
		{"b", "jp", "sha256:jp-2", "ok", "2026-10-02 00:00:00"},
		{"a", "cn", "sha256:cn-1", "staging", "2026-10-02 00:00:00"},
		{"a", "tw", "sha256:tw-1", "failed", ""},
		{"a", "kr", "", "ok", "2026-10-02 00:00:00"},
	} {
		finished := any(nil)
		if row.finished != "" {
			finished = row.finished
		}
		if _, err := db.Exec(`INSERT INTO master_ingest_version (target, region, content_hash, status, finished_at) VALUES (?, ?, ?, ?, ?)`,
			row.target, row.region, row.hash, row.status, finished); err != nil {
			t.Fatal(err)
		}
	}
	if hash, ok := masterdataIngestHash(ctx, db, "sqlite3", "jp"); !ok || hash != "sha256:jp-2" {
		t.Fatalf("jp = %q, %v; want the latest committed version", hash, ok)
	}
	for _, region := range []string{"cn", "tw", "kr", "en"} {
		if hash, ok := masterdataIngestHash(ctx, db, "sqlite3", region); ok {
			t.Fatalf("%s must read as unknown, got %q", region, hash)
		}
	}
	if _, ok := masterdataIngestHash(ctx, nil, "sqlite3", "jp"); ok {
		t.Fatal("nil pool must read as unknown")
	}
	// sqlite accepts the postgres placeholder and NULLS LAST as well.
	if hash, ok := masterdataIngestHash(ctx, db, "postgres", "jp"); !ok || hash != "sha256:jp-2" {
		t.Fatalf("postgres statement = %q, %v", hash, ok)
	}
	// A database without the table (a failed query) reads as unknown rather
	// than failing the caller.
	bare, err := sql.Open("sqlite3", fmt.Sprintf("file:ingest_version_bare_%d?mode=memory&cache=shared", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	defer bare.Close()
	if _, ok := masterdataIngestHash(ctx, bare, "sqlite3", "jp"); ok {
		t.Fatal("a failed query must read as unknown")
	}
}

func TestDatabaseProviderMasterdataIngestHashRequiresRawPool(t *testing.T) {
	ctx := context.Background()
	if _, ok := (*DatabaseProvider)(nil).MasterdataIngestHash(ctx); ok {
		t.Fatal("nil provider must read as unknown")
	}
	provider := &DatabaseProvider{region: renderregion.JP}
	if _, ok := provider.MasterdataIngestHash(ctx); ok {
		t.Fatal("provider without a mysekai pool must read as unknown")
	}
	provider.mysekai = &dbMySekaiProvider{}
	if _, ok := provider.MasterdataIngestHash(ctx); ok {
		t.Fatal("provider without a raw pool must read as unknown")
	}
	db := openIngestVersionDB(t)
	if _, err := db.Exec(`INSERT INTO master_ingest_version (target, region, content_hash, status, finished_at) VALUES ('a', 'jp', 'sha256:jp-1', 'ok', '2026-10-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	provider.mysekai = &dbMySekaiProvider{db: db, dbType: "sqlite3"}
	if hash, ok := provider.MasterdataIngestHash(ctx); !ok || hash != "sha256:jp-1" {
		t.Fatalf("hash = %q, %v", hash, ok)
	}
}
