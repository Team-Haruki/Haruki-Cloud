package imagecache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func TestReconcileRejectsDisabledStoreWithoutDeletingRows(t *testing.T) {
	store, _ := newEqualMockPGStore(t, PGStoreOptions{})
	store.lifecycle.Store(true)
	if missing, err := store.ReconcileObject(context.Background(), storage.Disabled(), "h", true); err == nil || missing {
		t.Fatalf("disabled store: %v %v", missing, err)
	}
}
func TestReconcileDoesNotInterpretUnconfiguredAsMissing(t *testing.T) {
	store, mock := newEqualMockPGStore(t, PGStoreOptions{})
	store.lifecycle.Store(true)
	store.widened.Store(true)
	expectContentLock(mock, "h")
	mock.ExpectQuery(lookupWidenedSQL).WithArgs("h").WillReturnRows(sqlmock.NewRows(widenedLookupColumns).AddRow("a.png", "", 1, BackendGarage, "image/png", nil, nil, nil))
	mock.ExpectRollback()
	objects := storagetest.NewMemory()
	objects.FailStat = func(storage.Key) error { return storage.ErrNotConfigured }
	if missing, err := store.ReconcileObject(context.Background(), objects, "h", true); !errors.Is(err, storage.ErrNotConfigured) || missing {
		t.Fatalf("unconfigured as missing: %v %v", missing, err)
	}
}
func TestFreshWriterHintIsBounded(t *testing.T) {
	now := time.Now()
	for _, tt := range []struct {
		age  time.Duration
		node string
		want string
	}{
		{time.Second, "cn09", "cn09"}, {119 * time.Second, "cn09", "cn09"}, {120 * time.Second, "cn09", ""}, {-31 * time.Second, "cn09", ""}, {time.Second, "", ""},
	} {
		entry := ImageEntry{WriterNode: tt.node, WrittenAt: now.Add(-tt.age)}
		if got := entry.FreshWriterNode(now); got != tt.want {
			t.Fatalf("age=%v got=%q want=%q", tt.age, got, tt.want)
		}
	}
}
