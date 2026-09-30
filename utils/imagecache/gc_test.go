package imagecache

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/logger"
)

var gcTestNow = time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

func newTestGC(t *testing.T, cfg GCConfig) (*GC, sqlmock.Sqlmock, *storagetest.Memory, *bytes.Buffer) {
	t.Helper()
	store, mock := newEqualMockPGStore(t, PGStoreOptions{})
	store.lifecycle.Store(true)
	store.widened.Store(true)
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
func orphanRow(rows *sqlmock.Rows, hash, path string) *sqlmock.Rows {
	return rows.AddRow(hash, "pjsk", path, "", BackendGarage, "image/png", int64(3), nil, gcRetentionCutoff(31))
}
func expectLiveRows(mock sqlmock.Sqlmock, hashes string, pairs ...string) {
	rows := sqlmock.NewRows(liveEntryColumns)
	for i := 0; i+1 < len(pairs); i += 2 {
		rows.AddRow(pairs[i], pairs[i+1])
	}
	mock.ExpectQuery(liveEntryPathsSQL).WithArgs(hashes).WillReturnRows(rows)
}
func expectContentLock(mock sqlmock.Sqlmock, hash string) {
	mock.ExpectBegin()
	mock.ExpectExec(ContentLockSQL).WithArgs(hash).WillReturnResult(sqlmock.NewResult(0, 0))
}
func expectRetire(mock sqlmock.Sqlmock, hash, key string, count int64) {
	expectContentLock(mock, hash)
	mock.ExpectQuery(retireObjectSQL).WithArgs(hash, gcRetentionCutoff(30), key).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
	mock.ExpectCommit()
}
func expectPending(mock sqlmock.Sqlmock, hash, key string, live ...string) {
	expectContentLock(mock, hash)
	mock.ExpectQuery(pendingObjectSQL).WithArgs(hash, key).WillReturnRows(sqlmock.NewRows([]string{"one"}).AddRow(1))
	expectLiveRows(mock, `{"`+hash+`"}`, live...)
}

func TestNewGCDisabledOrWithoutIndex(t *testing.T) {
	store, _ := newEqualMockPGStore(t, PGStoreOptions{})
	if NewGC(store, nil, GCConfig{}, nil) != nil || NewGC(nil, nil, GCConfig{Enabled: true}, nil) != nil {
		t.Fatal("disabled GC constructed")
	}
	gc := NewGC(store, nil, GCConfig{Enabled: true}, nil)
	if gc == nil || gc.cfg.ObjectDeleteEnabled || gc.cfg.Batch != DefaultGCBatch || gc.objects != storage.Disabled() {
		t.Fatal("unsafe defaults")
	}
	var nilGC *GC
	nilGC.Start(context.Background())
	if _, err := nilGC.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := (GCConfig{Batch: 99999}).normalized().Batch; got != 1000 {
		t.Fatal(got)
	}
}

func TestGCDryRunWritesNothing(t *testing.T) {
	gc, mock, objects, _ := newTestGC(t, GCConfig{DryRun: true, ObjectDeleteEnabled: true, Batch: 5})
	mock.ExpectQuery(expiredRenderKeysSQL).WithArgs(gcTestNow, 5).WillReturnRows(sqlmock.NewRows([]string{"request_key"}).AddRow("old"))
	mock.ExpectQuery(orphanGarageEntriesSQL).WithArgs(gcRetentionCutoff(30), 5).WillReturnRows(orphanRow(sqlmock.NewRows(orphanColumns), "h", "old.png"))
	report, err := gc.RunOnce(context.Background())
	if err != nil || report.ExpiredRenderRows != 1 || report.OrphanEntries != 1 || len(objects.Calls()) != 0 {
		t.Fatalf("report=%+v err=%v calls=%v", report, err, objects.Calls())
	}
}

func TestGCProtocolGateKeepsObjectsAndRows(t *testing.T) {
	gc, mock, objects, _ := newTestGC(t, GCConfig{})
	mock.ExpectQuery(expiredRenderKeysSQL).WillReturnRows(sqlmock.NewRows([]string{"request_key"}).AddRow("old"))
	mock.ExpectExec(deleteExpiredRenderSQL).WithArgs(`{"old"}`, gcTestNow).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(orphanGarageEntriesSQL).WillReturnRows(orphanRow(sqlmock.NewRows(orphanColumns), "h", "old.png"))
	report, err := gc.RunOnce(context.Background())
	if err != nil || report.DeletedRenderRows != 1 || report.DeletedEntries != 0 || len(objects.Calls()) != 0 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestGCRetirementMustCommitBeforeObjectDelete(t *testing.T) {
	for _, failCommit := range []bool{false, true} {
		t.Run(time.Duration(map[bool]int{false: 0, true: 1}[failCommit]).String(), func(t *testing.T) {
			gc, mock, objects, _ := newTestGC(t, GCConfig{ObjectDeleteEnabled: true})
			objects.Seed(map[string][]byte{"old.png": []byte("old")})
			expectContentLock(mock, "h")
			mock.ExpectQuery(retireObjectSQL).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			commit := mock.ExpectCommit()
			if failCommit {
				commit.WillReturnError(errors.New("connection lost"))
			} else {
				expectPending(mock, "h", "old.png")
				mock.ExpectExec(finishObjectDeleteSQL).WithArgs("h", "old.png").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			report := GCReport{}
			err := gc.collectEntry(context.Background(), ImageEntry{Hash: "h", CDNPath: "old.png"}, gcRetentionCutoff(30), &report)
			if failCommit {
				if err == nil || len(objects.Calls()) != 0 {
					t.Fatalf("delete before retirement commit: %v %v", err, objects.Calls())
				}
			} else if err != nil || report.DeletedObjects != 1 {
				t.Fatalf("report=%+v err=%v", report, err)
			}
		})
	}
}

func TestGCFailedDeleteSurvivesCollectorRestart(t *testing.T) {
	gc, mock, objects, _ := newTestGC(t, GCConfig{ObjectDeleteEnabled: true, Batch: 2})
	objects.Seed(map[string][]byte{"old.png": []byte("old")})
	expectRetire(mock, "h", "old.png", 1)
	expectPending(mock, "h", "old.png")
	mock.ExpectExec(retryObjectDeleteSQL).WithArgs("h", "old.png").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	objects.FailDelete = func(storage.Key) error { return errors.New("quorum unavailable") }
	report := GCReport{}
	if err := gc.collectEntry(context.Background(), ImageEntry{Hash: "h", CDNPath: "old.png"}, gcRetentionCutoff(30), &report); err != nil || report.ObjectLeaks != 1 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	// A new instance has no in-memory queue; its bounded SELECT recovers the task.
	restarted := NewGC(gc.index, objects, gc.cfg, gc.log)
	restarted.now = gc.now
	mock.ExpectQuery(pendingObjectsSQL).WithArgs(gcTestNow, 2).WillReturnRows(sqlmock.NewRows([]string{"content_hash", "cdn_path"}).AddRow("h", "old.png"))
	expectPending(mock, "h", "old.png")
	mock.ExpectExec(finishObjectDeleteSQL).WithArgs("h", "old.png").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	objects.FailDelete = nil
	if err := restarted.retryPending(context.Background(), &report); err != nil || report.RetriedObjectDeletes != 1 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestGCReReferenceCancelsOldDelete(t *testing.T) {
	gc, mock, objects, _ := newTestGC(t, GCConfig{ObjectDeleteEnabled: true})
	expectRetire(mock, "h", "old.png", 1)
	expectPending(mock, "h", "old.png", "h", "old.png")
	mock.ExpectExec(finishObjectDeleteSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	report := GCReport{}
	if err := gc.collectEntry(context.Background(), ImageEntry{Hash: "h", CDNPath: "old.png"}, gcRetentionCutoff(30), &report); err != nil || report.SkippedLiveObjectDeletes != 1 || len(objects.Calls()) != 0 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestGCAmbiguousDeleteCannotRemoveReplacementGeneration(t *testing.T) {
	gc, mock, objects, _ := newTestGC(t, GCConfig{ObjectDeleteEnabled: true})
	objects.Seed(map[string][]byte{"old.png": []byte("old")})
	expectRetire(mock, "h", "old.png", 1)
	expectPending(mock, "h", "old.png")
	mock.ExpectExec(finishObjectDeleteSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// Model a writer whose new upload finishes after an old Delete was sent.
	objects.FailDelete = func(storage.Key) error {
		objects.Seed(map[string][]byte{"new-generation.png": []byte("new")})
		return nil
	}
	report := GCReport{}
	if err := gc.collectEntry(context.Background(), ImageEntry{Hash: "h", CDNPath: "old.png"}, gcRetentionCutoff(30), &report); err != nil {
		t.Fatal(err)
	}
	data, err := objects.Get(context.Background(), "new-generation.png")
	if err != nil || string(data) != "new" {
		t.Fatalf("late old delete harmed replacement: %q %v", data, err)
	}
}

func TestGCDeleteThenCommitFailureLeavesRetrySafe(t *testing.T) {
	gc, mock, objects, _ := newTestGC(t, GCConfig{ObjectDeleteEnabled: true})
	objects.Seed(map[string][]byte{"old.png": []byte("old")})
	expectRetire(mock, "h", "old.png", 1)
	expectPending(mock, "h", "old.png")
	mock.ExpectExec(finishObjectDeleteSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(errors.New("lost acknowledgment"))
	report := GCReport{}
	if err := gc.collectEntry(context.Background(), ImageEntry{Hash: "h", CDNPath: "old.png"}, gcRetentionCutoff(30), &report); err == nil {
		t.Fatal("commit failure hidden")
	}
	if _, err := objects.Stat(context.Background(), "old.png"); !errors.Is(err, storage.ErrNotExist) {
		t.Fatal(err)
	}
	// Retirement was a separate committed transaction, so only the queue can
	// roll back; it cannot restore an image row referring to this missing key.
}
