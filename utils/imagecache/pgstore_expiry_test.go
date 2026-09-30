package imagecache

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDeleteExpiredRenderUsesObservationCutoff(t *testing.T) {
	store, mock := newEqualMockPGStore(t, PGStoreOptions{})
	cutoff := time.Now()
	if !strings.Contains(deleteExpiredRenderSQL, "expires_at IS NOT NULL AND expires_at <= $2") {
		t.Fatal("expiry deletion must preserve renewed/infinite rows")
	}
	mock.ExpectExec(deleteExpiredRenderSQL).WithArgs(`{"old"}`, cutoff).WillReturnResult(sqlmock.NewResult(0, 0))
	if n, err := store.DeleteExpiredRender(context.Background(), []string{"old"}, cutoff); n != 0 || err != nil {
		t.Fatalf("n=%v err=%v", n, err)
	}
	failure := errors.New("offline")
	mock.ExpectExec(deleteExpiredRenderSQL).WillReturnError(failure)
	if _, err := store.DeleteExpiredRender(context.Background(), []string{"old"}, cutoff); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, err := store.DeleteExpiredRender(context.Background(), nil, cutoff); err != nil {
		t.Fatal(err)
	}
	var nilStore *PGStore
	if _, err := nilStore.DeleteExpiredRender(context.Background(), []string{"old"}, cutoff); err != nil {
		t.Fatal(err)
	}
}
