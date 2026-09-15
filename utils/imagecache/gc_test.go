package imagecache

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/logger"

	"github.com/DATA-DOG/go-sqlmock"
)

var gcTestNow = time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

func newTestGC(t *testing.T, cfg GCConfig) (*GC, sqlmock.Sqlmock, *storagetest.Memory, *bytes.Buffer) {
	t.Helper()
	store, mock := newEqualMockPGStore(t, PGStoreOptions{})
	objects := storagetest.NewMemory()
	var buf bytes.Buffer
	cfg.Enabled = true
	gc := NewGC(store, objects, cfg, logger.NewLogger("GCTest", "DEBUG", &buf))
	gc.now = func() time.Time { return gcTestNow }
	return gc, mock, objects, &buf
}

func gcRetentionCutoff(days int) time.Time {
	return gcTestNow.Add(-time.Duration(days) * 24 * time.Hour)
}

func orphanRow(rows *sqlmock.Rows, hash, cdnPath string) *sqlmock.Rows {
	return rows.AddRow(hash, "pjsk", cdnPath, "", BackendGarage, "image/png", int64(3), nil, gcRetentionCutoff(31))
}

func expectEntryDelete(mock sqlmock.Sqlmock, hash string, cutoff time.Time, affected int64) {
	mock.ExpectExec(deleteOrphanEntrySQL).WithArgs(hash, cutoff).WillReturnResult(sqlmock.NewResult(0, affected))
}

func expectLiveRows(mock sqlmock.Sqlmock, hashes string, pairs ...string) {
	rows := sqlmock.NewRows(liveEntryColumns)
	for i := 0; i+1 < len(pairs); i += 2 {
		rows.AddRow(pairs[i], pairs[i+1])
	}
	mock.ExpectQuery(liveEntryPathsSQL).WithArgs(hashes).WillReturnRows(rows)
}

func methodsOf(calls []storagetest.Op) string {
	parts := make([]string, 0, len(calls))
	for _, call := range calls {
		parts = append(parts, call.Method+":"+string(call.Key))
	}
	return strings.Join(parts, ",")
}

func TestNewGCDisabledOrWithoutIndex(t *testing.T) {
	store, _ := newEqualMockPGStore(t, PGStoreOptions{})
	if gc := NewGC(store, nil, GCConfig{}, nil); gc != nil {
		t.Fatal("NewGC with gc_enabled=false must be nil")
	}
	if gc := NewGC(nil, nil, GCConfig{Enabled: true}, nil); gc != nil {
		t.Fatal("NewGC without an index must be nil")
	}
	gc := NewGC(store, nil, GCConfig{Enabled: true}, nil)
	if gc == nil || gc.objects != storage.Disabled() || gc.log == nil {
		t.Fatalf("NewGC defaults = %+v", gc)
	}
	if gc.cfg.Interval != DefaultGCInterval || gc.cfg.Batch != DefaultGCBatch || gc.cfg.ObjectRetentionDays != DefaultGCObjectRetentionDays {
		t.Fatalf("normalized config = %+v", gc.cfg)
	}
	var nilGC *GC
	nilGC.Start(context.Background())
	if report, err := nilGC.RunOnce(context.Background()); err != nil || report != (GCReport{}) {
		t.Fatalf("nil RunOnce = (%+v, %v)", report, err)
	}
}

func TestGCDryRunWritesNothing(t *testing.T) {
	gc, mock, objects, buf := newTestGC(t, GCConfig{DryRun: true, Batch: 5, ObjectRetentionDays: 30})
	objects.Seed(map[string][]byte{"pjsk/api/a/h1.png": []byte("png")})
	mock.ExpectQuery(expiredRenderKeysSQL).WithArgs(gcTestNow, 5).
		WillReturnRows(sqlmock.NewRows([]string{"request_key"}).AddRow("k1").AddRow("k2"))
	mock.ExpectQuery(orphanGarageEntriesSQL).WithArgs(gcRetentionCutoff(30), 5).
		WillReturnRows(orphanRow(sqlmock.NewRows(orphanColumns), "h1", "pjsk/api/a/h1.png"))

	report, err := gc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	want := GCReport{ExpiredRenderRows: 2, OrphanEntries: 1, DryRun: true}
	if report != want {
		t.Fatalf("report = %+v, want %+v", report, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if calls := objects.Calls(); len(calls) != 0 {
		t.Fatalf("dry run touched objects: %s", methodsOf(calls))
	}
	logs := buf.String()
	for _, want := range []string{"dry run: expired render rows", "dry run: orphan garage entries", "k1", "pjsk/api/a/h1.png", "image cache gc cycle"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs missing %q:\n%s", want, logs)
		}
	}
	if strings.Count(logs, `msg="image cache gc cycle"`) != 1 {
		t.Fatalf("want exactly one summary line:\n%s", logs)
	}
}

func TestGCRunOrderRenderRowsThenEntryThenObject(t *testing.T) {
	gc, mock, objects, _ := newTestGC(t, GCConfig{Batch: 50, ObjectRetentionDays: 7})
	objects.Seed(map[string][]byte{"pjsk/api/a/h1.png": []byte("1"), "pjsk/h2.png": []byte("2")})
	mock.ExpectQuery(expiredRenderKeysSQL).WithArgs(gcTestNow, 50).
		WillReturnRows(sqlmock.NewRows([]string{"request_key"}).AddRow("k1"))
	mock.ExpectExec(deleteRenderSQL).WithArgs(`{"k1"}`).WillReturnResult(sqlmock.NewResult(0, 1))
	rows := orphanRow(sqlmock.NewRows(orphanColumns), "h1", "pjsk/api/a/h1.png")
	mock.ExpectQuery(orphanGarageEntriesSQL).WithArgs(gcRetentionCutoff(7), 50).
		WillReturnRows(orphanRow(rows, "h2", "pjsk/h2.png"))
	expectEntryDelete(mock, "h1", gcRetentionCutoff(7), 1)
	expectLiveRows(mock, `{"h1"}`)
	expectEntryDelete(mock, "h2", gcRetentionCutoff(7), 1)
	expectLiveRows(mock, `{"h2"}`)

	var deletesSeen []string
	objects.FailDelete = func(key storage.Key) error {
		deletesSeen = append(deletesSeen, string(key))
		// Each object delete must follow its own row delete.
		switch key {
		case "pjsk/api/a/h1.png":
			if err := mock.ExpectationsWereMet(); err == nil {
				t.Error("h1 object deleted after every row delete; want it before h2's row delete")
			}
		case "pjsk/h2.png":
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("h2 object deleted before its row: %v", err)
			}
		}
		return nil
	}

	report, err := gc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	want := GCReport{ExpiredRenderRows: 1, DeletedRenderRows: 1, OrphanEntries: 2, DeletedEntries: 2, DeletedObjects: 2}
	if report != want {
		t.Fatalf("report = %+v, want %+v", report, want)
	}
	if strings.Join(deletesSeen, ",") != "pjsk/api/a/h1.png,pjsk/h2.png" {
		t.Fatalf("object deletes = %v (recorded cdn_path verbatim)", deletesSeen)
	}
	if _, err := objects.Stat(context.Background(), "pjsk/api/a/h1.png"); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("object h1 still present: %v", err)
	}
}

func TestGCObjectLeakIsRetriedNextCycle(t *testing.T) {
	gc, mock, objects, buf := newTestGC(t, GCConfig{})
	objects.Seed(map[string][]byte{"pjsk/api/a/h1.png": []byte("1")})
	mock.ExpectQuery(expiredRenderKeysSQL).WillReturnRows(sqlmock.NewRows([]string{"request_key"}))
	mock.ExpectQuery(orphanGarageEntriesSQL).WithArgs(gcRetentionCutoff(DefaultGCObjectRetentionDays), DefaultGCBatch).
		WillReturnRows(orphanRow(sqlmock.NewRows(orphanColumns), "h1", "pjsk/api/a/h1.png"))
	expectEntryDelete(mock, "h1", gcRetentionCutoff(DefaultGCObjectRetentionDays), 1)
	expectLiveRows(mock, `{"h1"}`)
	objects.FailDelete = func(storage.Key) error { return errors.New("garage quorum missing") }

	report, err := gc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("first RunOnce() error = %v", err)
	}
	if report.ObjectLeaks != 1 || report.PendingObjectDeletes != 1 || report.DeletedObjects != 0 || report.DeletedEntries != 1 {
		t.Fatalf("first report = %+v", report)
	}
	if !strings.Contains(buf.String(), "object delete failed") {
		t.Fatalf("leak not logged:\n%s", buf.String())
	}

	objects.FailDelete = nil
	expectLiveRows(mock, `{"h1"}`)
	mock.ExpectQuery(expiredRenderKeysSQL).WillReturnRows(sqlmock.NewRows([]string{"request_key"}))
	mock.ExpectQuery(orphanGarageEntriesSQL).WillReturnRows(sqlmock.NewRows(orphanColumns))
	report, err = gc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("second RunOnce() error = %v", err)
	}
	if report.RetriedObjectDeletes != 1 || report.PendingObjectDeletes != 0 || report.ObjectLeaks != 0 {
		t.Fatalf("second report = %+v", report)
	}
	if _, err := objects.Stat(context.Background(), "pjsk/api/a/h1.png"); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("leaked object not retried: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGCPendingRetryKeepsFailuresAndIsBounded(t *testing.T) {
	gc, mock, objects, buf := newTestGC(t, GCConfig{})
	objects.FailDelete = func(key storage.Key) error {
		if key == "keep" {
			return errors.New("still down")
		}
		return nil
	}
	gc.pending = []pendingObjectDelete{{Hash: "h", Key: "keep"}, {Hash: "h", Key: "gone"}}
	expectLiveRows(mock, `{"h"}`)
	var report GCReport
	if err := gc.retryPending(context.Background(), &report); err != nil {
		t.Fatal(err)
	}
	if report.RetriedObjectDeletes != 1 || len(gc.pending) != 1 || gc.pending[0].Key != "keep" {
		t.Fatalf("retry = %+v pending %v", report, gc.pending)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	gc.pending = make([]pendingObjectDelete, maxGCPendingObjectDeletes)
	gc.pending[0] = pendingObjectDelete{Key: "oldest"}
	gc.pending[1] = pendingObjectDelete{Key: "second"}
	gc.addPending(context.Background(), pendingObjectDelete{Key: "newest"})
	if len(gc.pending) != maxGCPendingObjectDeletes || gc.pending[0].Key != "second" || gc.pending[len(gc.pending)-1].Key != "newest" {
		t.Fatalf("bounded pending: len=%d first=%q last=%q", len(gc.pending), gc.pending[0], gc.pending[len(gc.pending)-1])
	}
	if !strings.Contains(buf.String(), "dropping the oldest") {
		t.Fatalf("drop not logged:\n%s", buf.String())
	}
}

func TestGCEntryDeleteEdgeCases(t *testing.T) {
	gc, mock, objects, buf := newTestGC(t, GCConfig{})
	rows := orphanRow(sqlmock.NewRows(orphanColumns), "gone", "pjsk/api/a/gone.png")
	rows = orphanRow(rows, "bad", "../escape.png")
	rows = orphanRow(rows, "referenced", "pjsk/api/a/referenced.png")
	mock.ExpectQuery(expiredRenderKeysSQL).WillReturnRows(sqlmock.NewRows([]string{"request_key"}))
	mock.ExpectQuery(orphanGarageEntriesSQL).WillReturnRows(rows)
	// Another collector already deleted the row, or it was re-referenced since
	// the SELECT (the guarded delete matches nothing): no object delete.
	cutoff := gcRetentionCutoff(DefaultGCObjectRetentionDays)
	expectEntryDelete(mock, "gone", cutoff, 0)
	expectEntryDelete(mock, "bad", cutoff, 1)
	// The FK (no cascade) refuses a still-referenced row: the object stays.
	fkErr := errors.New("violates foreign key constraint")
	mock.ExpectExec(deleteOrphanEntrySQL).WithArgs("referenced", cutoff).WillReturnError(fkErr)

	report, err := gc.RunOnce(context.Background())
	if !errors.Is(err, fkErr) {
		t.Fatalf("RunOnce() error = %v, want the FK error", err)
	}
	if report.DeletedEntries != 1 || report.ObjectLeaks != 1 || report.DeletedObjects != 0 || report.PendingObjectDeletes != 0 {
		t.Fatalf("report = %+v", report)
	}
	if calls := objects.Calls(); len(calls) != 0 {
		t.Fatalf("unexpected object calls: %s", methodsOf(calls))
	}
	if !strings.Contains(buf.String(), "invalid cdn_path") || !strings.Contains(buf.String(), "image cache gc cycle failed") {
		t.Fatalf("logs:\n%s", buf.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGCPhaseErrorsDoNotStopTheOtherPhase(t *testing.T) {
	gc, mock, _, _ := newTestGC(t, GCConfig{})
	selectErr := errors.New("select failed")
	mock.ExpectQuery(expiredRenderKeysSQL).WillReturnError(selectErr)
	orphanErr := errors.New("orphan select failed")
	mock.ExpectQuery(orphanGarageEntriesSQL).WillReturnError(orphanErr)
	if _, err := gc.RunOnce(context.Background()); !errors.Is(err, selectErr) || !errors.Is(err, orphanErr) {
		t.Fatalf("RunOnce() error = %v", err)
	}

	deleteErr := errors.New("delete render failed")
	mock.ExpectQuery(expiredRenderKeysSQL).WillReturnRows(sqlmock.NewRows([]string{"request_key"}).AddRow("k"))
	mock.ExpectExec(deleteRenderSQL).WillReturnError(deleteErr)
	mock.ExpectQuery(orphanGarageEntriesSQL).WillReturnRows(sqlmock.NewRows(orphanColumns))
	if _, err := gc.RunOnce(context.Background()); !errors.Is(err, deleteErr) {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// The phase predicates live in SQL; pin the parts GC relies on.
func TestGCQueriesExcludeLegacyInfiniteAndReferencedRows(t *testing.T) {
	if !strings.Contains(expiredRenderKeysSQL, "expires_at IS NOT NULL AND expires_at < $1") {
		t.Fatalf("phase 1 must never expire infinite rows: %s", expiredRenderKeysSQL)
	}
	for _, want := range []string{
		"e.storage_backend = 'garage'",
		"e.last_referenced_at < $1",
		"NOT EXISTS (SELECT 1 FROM render_cache_index r WHERE r.content_hash = e.hash)",
	} {
		if !strings.Contains(orphanGarageEntriesSQL, want) {
			t.Fatalf("phase 2 predicate missing %q", want)
		}
	}
	for _, want := range []string{
		"e.storage_backend = 'garage'",
		"e.last_referenced_at < $2",
		"NOT EXISTS (SELECT 1 FROM render_cache_index r WHERE r.content_hash = e.hash)",
	} {
		if !strings.Contains(deleteOrphanEntrySQL, want) {
			t.Fatalf("guarded delete missing %q", want)
		}
	}
	where := orphanGarageEntriesSQL[strings.Index(orphanGarageEntriesSQL, "WHERE"):]
	if strings.Contains(where, "expires_at") {
		t.Fatalf("phase 2 must not consult entries.expires_at: %s", where)
	}
}

func TestGCLoopRunsOnTickAndStopsWithContext(t *testing.T) {
	gc, mock, _, _ := newTestGC(t, GCConfig{})
	mock.ExpectQuery(expiredRenderKeysSQL).WillReturnRows(sqlmock.NewRows([]string{"request_key"}))
	mock.ExpectQuery(orphanGarageEntriesSQL).WillReturnRows(sqlmock.NewRows(orphanColumns))

	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		gc.loop(ctx, ticks)
		close(done)
	}()
	ticks <- gcTestNow
	ticks <- gcTestNow.Add(time.Second) // blocks until the first cycle finished
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not stop with the context")
	}

	closed := make(chan time.Time)
	close(closed)
	gc.loop(context.Background(), closed)

	// Start with a cancelled context returns and spawns a goroutine that exits.
	stopped, stop := context.WithCancel(context.Background())
	stop()
	gc.Start(stopped)
}

// Object keys are content-addressed: while a delete is pending, Drawing or
// Cloud can store the same bytes again under the same key and insert a live
// row. The retry must then drop the delete instead of breaking that row.
func TestGCPendingDeleteSkipsReinsertedRow(t *testing.T) {
	gc, mock, objects, buf := newTestGC(t, GCConfig{})
	objects.Seed(map[string][]byte{"pjsk/api/a/h1.png": []byte("1"), "pjsk/api/b/h2.png": []byte("2")})
	cutoff := gcRetentionCutoff(DefaultGCObjectRetentionDays)
	mock.ExpectQuery(expiredRenderKeysSQL).WillReturnRows(sqlmock.NewRows([]string{"request_key"}))
	rows := orphanRow(sqlmock.NewRows(orphanColumns), "h1", "pjsk/api/a/h1.png")
	mock.ExpectQuery(orphanGarageEntriesSQL).WillReturnRows(orphanRow(rows, "h2", "pjsk/api/b/h2.png"))
	expectEntryDelete(mock, "h1", cutoff, 1)
	expectLiveRows(mock, `{"h1"}`)
	expectEntryDelete(mock, "h2", cutoff, 1)
	expectLiveRows(mock, `{"h2"}`)
	objects.FailDelete = func(storage.Key) error { return errors.New("garage down") }
	if report, err := gc.RunOnce(context.Background()); err != nil || report.PendingObjectDeletes != 2 {
		t.Fatalf("first cycle = (%+v, %v)", report, err)
	}

	// h1 was re-stored at the same cdn_path; h2's hash is live only under a
	// different path (a Cloud row), so its old object is still garbage.
	objects.FailDelete = nil
	expectLiveRows(mock, `{"h1","h2"}`, "h1", "pjsk/api/a/h1.png", "h2", "pjsk/h2.png")
	mock.ExpectQuery(expiredRenderKeysSQL).WillReturnRows(sqlmock.NewRows([]string{"request_key"}))
	mock.ExpectQuery(orphanGarageEntriesSQL).WillReturnRows(sqlmock.NewRows(orphanColumns))
	report, err := gc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("second cycle error = %v", err)
	}
	if report.SkippedLiveObjectDeletes != 1 || report.RetriedObjectDeletes != 1 || report.PendingObjectDeletes != 0 {
		t.Fatalf("second report = %+v", report)
	}
	if got, err := objects.Get(context.Background(), "pjsk/api/a/h1.png"); err != nil || string(got) != "1" {
		t.Fatalf("live object deleted: %q, %v", got, err)
	}
	if _, err := objects.Stat(context.Background(), "pjsk/api/b/h2.png"); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("orphan object kept: %v", err)
	}
	if !strings.Contains(buf.String(), "skipped_live_object_deletes=1") {
		t.Fatalf("summary missing skip count:\n%s", buf.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGCLiveRowChecks(t *testing.T) {
	gc, mock, objects, _ := newTestGC(t, GCConfig{})
	objects.Seed(map[string][]byte{"pjsk/api/a/h1.png": []byte("1"), "pjsk/api/a/h2.png": []byte("2")})
	cutoff := gcRetentionCutoff(DefaultGCObjectRetentionDays)
	checkErr := errors.New("live check failed")
	mock.ExpectQuery(expiredRenderKeysSQL).WillReturnRows(sqlmock.NewRows([]string{"request_key"}))
	rows := orphanRow(sqlmock.NewRows(orphanColumns), "h1", "pjsk/api/a/h1.png")
	mock.ExpectQuery(orphanGarageEntriesSQL).WillReturnRows(orphanRow(rows, "h2", "pjsk/api/a/h2.png"))
	// h1: re-inserted between the row delete and the object delete.
	expectEntryDelete(mock, "h1", cutoff, 1)
	expectLiveRows(mock, `{"h1"}`, "h1", "pjsk/api/a/h1.png")
	// h2: the check fails, so the delete is owed instead of executed.
	expectEntryDelete(mock, "h2", cutoff, 1)
	mock.ExpectQuery(liveEntryPathsSQL).WithArgs(`{"h2"}`).WillReturnError(checkErr)
	report, err := gc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if report.SkippedLiveObjectDeletes != 1 || report.ObjectLeaks != 1 || report.PendingObjectDeletes != 1 || report.DeletedObjects != 0 {
		t.Fatalf("report = %+v", report)
	}
	for _, call := range objects.Calls() {
		if call.Method == "Delete" {
			t.Fatalf("object deleted despite live row / failed check: %s", methodsOf(objects.Calls()))
		}
	}

	// A failing check on retry keeps every pending delete and deletes nothing.
	mock.ExpectQuery(liveEntryPathsSQL).WithArgs(`{"h2"}`).WillReturnError(checkErr)
	mock.ExpectQuery(expiredRenderKeysSQL).WillReturnRows(sqlmock.NewRows([]string{"request_key"}))
	mock.ExpectQuery(orphanGarageEntriesSQL).WillReturnRows(sqlmock.NewRows(orphanColumns))
	report, err = gc.RunOnce(context.Background())
	if !errors.Is(err, checkErr) || report.PendingObjectDeletes != 1 || report.RetriedObjectDeletes != 0 {
		t.Fatalf("retry cycle = (%+v, %v)", report, err)
	}
	if _, err := objects.Stat(context.Background(), "pjsk/api/a/h2.png"); err != nil {
		t.Fatalf("object deleted without a live-row check: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
