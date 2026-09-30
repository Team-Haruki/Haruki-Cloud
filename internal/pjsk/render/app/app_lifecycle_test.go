package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"haruki-cloud/internal/pjsk/meta"
	"haruki-cloud/utils/imagecache"
)

func TestAppCloseCancelsInternallyCreatedMetaRefresh(t *testing.T) {
	regionCount := len(meta.Regions())
	started := make(chan struct{}, regionCount)
	canceled := make(chan struct{}, regionCount)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) <= int64(regionCount) {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		started <- struct{}{}
		<-r.Context().Done()
		canceled <- struct{}{}
	}))
	t.Cleanup(server.Close)
	lifecycle, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	runtime := New(nil, nil, Config{
		InitContext:              lifecycle,
		MusicMetaSource:          meta.SourceRegistry,
		MusicMetaBaseURL:         server.URL,
		MusicMetaRefreshInterval: 20 * time.Millisecond,
	})
	t.Cleanup(func() { _ = runtime.Close() })
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for range regionCount {
		select {
		case <-started:
		case <-deadline.C:
			t.Fatal("metadata background refresh did not start")
		}
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	for range regionCount {
		select {
		case <-canceled:
		case <-deadline.C:
			t.Fatal("App.Close did not cancel metadata HTTP requests")
		}
	}
	if lifecycle.Err() != nil {
		t.Fatal("App.Close canceled the caller-owned context")
	}
}

func TestAppCloseReleasesIndexWithoutObjectClient(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectClose()
	runtime := &App{ImageIndex: imagecache.NewPGStoreFromDB(db, imagecache.PGStoreOptions{})}
	if err = runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if err = runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
