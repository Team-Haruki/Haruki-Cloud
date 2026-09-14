package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/core/urlhost"
	"haruki-cloud/internal/pjsk/meta"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/imagecache"

	"github.com/DATA-DOG/go-sqlmock"
)

func imageHostsFor(t *testing.T, base string) *urlhost.Set {
	t.Helper()
	hosts, err := urlhost.New(map[string]string{"cn09": base}, urlhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return hosts
}

func TestNewAppImageCacheOnSlot(t *testing.T) {
	ctx := context.Background()
	memory := storagetest.NewMemory()
	cfg := Config{Stores: storage.Set{ImageCache: memory}.Normalized(), ImageHosts: imageHostsFor(t, "https://ic-cn09.example")}
	client := newAppImageCache(ctx, cfg, nil)
	if client == nil {
		t.Fatal("slot-backed image cache not built")
	}
	url, err := client.StoreAndGetURL(ctx, []byte("slot image"), "pjsk")
	if err != nil || !strings.HasPrefix(url, "https://ic-cn09.example/pjsk/") {
		t.Fatalf("StoreAndGetURL() = %q, %v", url, err)
	}
	if calls := memory.Calls(); len(calls) != 2 || calls[1].Method != "Put" {
		t.Fatalf("slot calls = %+v", calls)
	}
}

func TestNewAppImageCacheLegacyDirFallback(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg := Config{Stores: storage.Set{}.Normalized(), ImageCacheDir: dir, ImageHosts: urlhost.Single("https://ic.example/")}
	client := newAppImageCache(ctx, cfg, nil)
	if client == nil {
		t.Fatal("legacy dir image cache not built")
	}
	url, err := client.StoreAndGetURL(ctx, []byte("legacy image"), "pjsk")
	if err != nil {
		t.Fatal(err)
	}
	name := url[strings.LastIndex(url, "/")+1:]
	if url != "https://ic.example/pjsk/"+name {
		t.Fatalf("url = %q", url)
	}
	if _, err := os.Stat(filepath.Join(dir, "pjsk", name)); err != nil {
		t.Fatalf("legacy file not written: %v", err)
	}
	if got, ok := client.URLForFile(ctx, filepath.Join(dir, "pjsk", name)); !ok || got != url {
		t.Fatalf("URLForFile() = %q, %v", got, ok)
	}
}

func TestNewAppImageCacheUnconfiguredAndPartial(t *testing.T) {
	ctx := context.Background()
	if client := newAppImageCache(ctx, Config{Stores: storage.Set{}.Normalized(), ImageHosts: urlhost.Single("")}, nil); client != nil {
		t.Fatal("nothing configured built a client")
	}
	if client := newAppImageCache(ctx, Config{Stores: storage.Set{}.Normalized(), ImageHosts: imageHostsFor(t, "https://ic.example")}, nil); client != nil {
		t.Fatal("hosts without objects built a client")
	}
	if client := newAppImageCache(ctx, Config{Stores: storage.Set{ImageCache: storagetest.NewMemory()}.Normalized(), ImageHosts: urlhost.Single("")}, nil); client != nil {
		t.Fatal("objects without hosts built a client")
	}
	if store, root := legacyImageCacheStore("  "); store != nil || root != "" {
		t.Fatalf("blank legacy dir = %v, %q", store, root)
	}
}

func TestOpenAppImageStoreErrors(t *testing.T) {
	ctx := context.Background()
	if store, err := openAppImageStore(ctx, " ", 0); store != nil || err != nil {
		t.Fatalf("empty DSN = %v, %v", store, err)
	}
	if store, err := openAppImageStore(ctx, "://invalid", 4); store != nil || err == nil || !strings.Contains(err.Error(), "image cache index") {
		t.Fatalf("invalid DSN = %v, %v", store, err)
	}

	schemaSteps := func(mock sqlmock.Sqlmock) {
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS image_cache_entries").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mockOpener := func(t *testing.T, expect func(sqlmock.Sqlmock)) (imageStoreOpener, sqlmock.Sqlmock) {
		db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(false))
		if err != nil {
			t.Fatal(err)
		}
		expect(mock)
		return func(dsn string, opts imagecache.PGStoreOptions) (*imagecache.PGStore, error) {
			if dsn != "postgres://index" || opts.MaxOpen != 4 {
				t.Fatalf("opener got %q %+v", dsn, opts)
			}
			return imagecache.NewPGStoreFromDB(db, opts), nil
		}, mock
	}

	open, mock := mockOpener(t, func(mock sqlmock.Sqlmock) {
		schemaSteps(mock)
		mock.ExpectQuery("information_schema.columns").WillReturnRows(sqlmock.NewRows([]string{"?column?"}))
	})
	store, err := openAppImageStoreWith(ctx, "postgres://index", 4, open)
	if err != nil || store == nil || store.Widened() {
		t.Fatalf("healthy index = %v, %v", store, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	open, mock = mockOpener(t, func(mock sqlmock.Sqlmock) {
		schemaSteps(mock)
		mock.ExpectQuery("information_schema.columns").WillReturnError(errors.New("permission denied"))
		mock.ExpectClose()
	})
	store, err = openAppImageStoreWith(ctx, "postgres://index", 4, open)
	if store != nil || err == nil || !strings.Contains(err.Error(), "image cache index schema") {
		t.Fatalf("schema failure = %v, %v", store, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInitErrorRecordedByNew(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runtime := New(nil, nil, Config{InitContext: ctx, MetaLoader: meta.NewLoader(nil), ImageCachePGURL: "://invalid"})
	t.Cleanup(func() { _ = runtime.Close() })
	if err := runtime.InitError(); err == nil {
		t.Fatal("unreachable image cache index was swallowed")
	}
	if runtime.Drawing != nil || runtime.ImageCache != nil {
		t.Fatal("unexpected clients on an empty config")
	}
	var zero *App
	if zero.InitError() != nil || (&App{}).InitError() != nil {
		t.Fatal("zero App reports an init error")
	}
}
