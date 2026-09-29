package provider

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/observability/commandtrace"
	renderregion "haruki-cloud/internal/pjsk/region"

	_ "github.com/mattn/go-sqlite3"
)

func TestDBMySekaiQueryTrace(t *testing.T) {
	for _, tc := range []struct {
		name       string
		table      string
		setup      string
		cancel     bool
		wantError  bool
		wantRows   int
		wantDecode bool
	}{
		{name: "rows and JSON conversion", table: "mysekaiitems", wantRows: 2, wantDecode: true},
		{name: "empty table", table: "mysekaiitems", setup: "DELETE FROM trace_rows", wantDecode: true},
		{name: "query error", table: "mysekaitools", wantError: true},
		{name: "canceled query", table: "mysekaiitems", cancel: true, wantError: true},
		{name: "iteration error after decoded row", table: "mysekaiitems", setup: "UPDATE trace_rows SET amount = -9223372036854775808 WHERE id = 2", wantError: true, wantRows: 1, wantDecode: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := newMySekaiTraceProvider(t)
			if tc.setup != "" {
				if _, err := provider.db.ExecContext(t.Context(), tc.setup); err != nil {
					t.Fatal(err)
				}
			}
			ctx, trace := commandtrace.WithNewTrace(t.Context())
			if tc.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			rows, err := provider.queryTable(ctx, tc.table)
			if (err != nil) != tc.wantError || len(rows) != tc.wantRows {
				t.Fatalf("rows=%d err=%v, want rows=%d error=%v", len(rows), err, tc.wantRows, tc.wantError)
			}
			if tc.cancel && !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled query error=%v", err)
			}
			if !tc.wantError && rows == nil {
				t.Fatal("successful query must retain a nonnil result")
			}
			if len(rows) > 0 {
				if rows[0]["id"] != int64(98712345678911) || rows[0]["displayName"] != "fixture-secret-title" {
					t.Fatalf("row conversion changed: %#v", rows[0])
				}
				payload, ok := rows[0]["payload"].(map[string]any)
				if !ok || payload["secret"] != "fixture-secret-token" {
					t.Fatalf("JSON conversion changed: %#v", rows[0]["payload"])
				}
				if _, ok := rows[0]["serverRegion"]; ok {
					t.Fatal("region column must remain omitted")
				}
			}

			snapshot := trace.Snapshot()
			operations := make(map[string]commandtrace.Stats)
			for _, operation := range snapshot.Operations {
				operations[operation.Name] = operation
			}
			query := operations["mysekai.provider_query"]
			if query.Count != 1 || query.Total <= 0 || query.Max != query.Total {
				t.Fatalf("query stats=%+v", query)
			}
			wantOperations := 1
			if tc.wantDecode {
				wantOperations++
				decode := operations["mysekai.provider_decode"]
				if decode.Count != 1 || decode.Max != decode.Total || decode.Total > query.Total {
					t.Fatalf("decode stats=%+v query=%+v", decode, query)
				}
				if tc.wantRows > 0 && decode.Total <= 0 {
					t.Fatalf("completed rows have no decode duration: %+v", decode)
				}
				if tc.wantRows == 0 && decode.Total != 0 {
					t.Fatalf("empty table has decode work: %+v", decode)
				}
			}
			if len(operations) != wantOperations || len(snapshot.Phases) != 0 {
				t.Fatalf("unexpected trace dimensions: %+v", snapshot)
			}

			var output strings.Builder
			logger := slog.New(slog.NewJSONHandler(&output, nil))
			logger.LogAttrs(t.Context(), slog.LevelInfo, "trace", slog.Any("operations", snapshot.OperationValue()))
			for _, private := range []string{"SELECT", "mysekaiitems", "mysekaitools", "trace_rows", "98712345678911", "fixture-secret-title", "fixture-secret-token"} {
				if strings.Contains(output.String(), private) {
					t.Fatalf("trace contains private query details %q: %s", private, output.String())
				}
			}
		})
	}
}

func newMySekaiTraceProvider(t *testing.T) *dbMySekaiProvider {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "trace.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, statement := range []string{
		`CREATE TABLE trace_rows (id INTEGER PRIMARY KEY, game_id INTEGER, display_name TEXT, payload BLOB, amount INTEGER, server_region TEXT)`,
		`CREATE VIEW mysekaiitems AS SELECT id, game_id, display_name, payload, abs(amount) AS amount, server_region FROM trace_rows`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	for i, region := range []string{"jp", "jp", "tw"} {
		if _, err := db.ExecContext(t.Context(), `INSERT INTO trace_rows (id, game_id, display_name, payload, amount, server_region) VALUES (?, ?, ?, ?, ?, ?)`, i+1, int64(98712345678911)+int64(i), "fixture-secret-title", []byte(`{"secret":"fixture-secret-token"}`), i+1, region); err != nil {
			t.Fatal(err)
		}
	}
	return &dbMySekaiProvider{db: db, dbType: "sqlite3", region: renderregion.JP}
}
