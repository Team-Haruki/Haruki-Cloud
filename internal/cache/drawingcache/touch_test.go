package drawingcache

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func touchTestAPI(t *testing.T, ttl int64, base time.Time) (*API, *fiber.App, string) {
	t.Helper()
	root := t.TempDir()
	db, err := InitDB(filepath.Join(root, "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	dao := NewDAO(db)
	key := strings.Repeat("a", 64)
	file := filepath.Join(root, "image.png")
	if err := os.WriteFile(file, []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := dao.SaveRecord(&CacheRecord{Sha256Key: key, APIPath: "api/pjsk/profile", UserID: "public", FilePath: file, CreatedAt: base, LastUsedAt: base, TTLSeconds: ttl, ExpiresAt: cacheExpiresAt(base, ttl)}); err != nil {
		t.Fatal(err)
	}
	api := NewAPI(dao, root)
	app := fiber.New()
	api.RegisterRoutes(app)
	if _, err := db.Exec(`CREATE TABLE touch_writes (n INTEGER); CREATE TRIGGER count_touch AFTER UPDATE ON image_cache_index BEGIN INSERT INTO touch_writes VALUES (1); END`); err != nil {
		t.Fatal(err)
	}
	return api, app, key
}

func hitTouchAPI(t *testing.T, app *fiber.App, key string) {
	t.Helper()
	response, err := app.Test(httptest.NewRequest("GET", "/cache?key="+key, nil))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("cache status=%d", response.StatusCode)
	}
}

func TestCacheHitsCoalesceAndPreserveSlidingTTL(t *testing.T) {
	for _, ttl := range []int64{0, 1, 60} {
		t.Run(fmt.Sprint(ttl), func(t *testing.T) {
			base := time.Now().UTC()
			api, app, key := touchTestAPI(t, ttl, base)
			now := base.Add(100 * time.Millisecond)
			api.now = func() time.Time { return now }
			for range 1000 {
				hitTouchAPI(t, app, key)
			}
			var n int
			if err := api.dao.db.QueryRow(`SELECT COUNT(*) FROM touch_writes`).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 0 {
				t.Fatalf("1000 hits caused %d writes before flush", n)
			}
			if err := api.flushTouches(); err != nil {
				t.Fatal(err)
			}
			if err := api.dao.db.QueryRow(`SELECT COUNT(*) FROM touch_writes`).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 1 {
				t.Fatalf("coalesced writes=%d want 1", n)
			}
			record, err := api.dao.GetRecord(key)
			if err != nil {
				t.Fatal(err)
			}
			if !record.LastUsedAt.Equal(now) || !record.ExpiresAt.Equal(cacheExpiresAt(now, ttl)) {
				t.Fatalf("wrong lifetime: %+v", record)
			}
		})
	}
}

func TestGCFlushesPendingHitsBeforeDeleting(t *testing.T) {
	base := time.Now().UTC().Add(-4100 * time.Millisecond)
	api, app, key := touchTestAPI(t, 4, base)
	api.now = func() time.Time { return base.Add(900 * time.Millisecond) }
	hitTouchAPI(t, app, key)
	if len(api.pendingTouches) != 1 {
		t.Fatal("hit should await coalesced flush")
	}
	if n, err := api.cleanupExpired(100); err != nil || n != 0 {
		t.Fatalf("GC deleted renewed record: n=%d err=%v", n, err)
	}
	record, err := api.dao.GetRecord(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(record.FilePath); err != nil {
		t.Fatal(err)
	}
}

func TestFailedTouchFlushRetainsBoundedPendingState(t *testing.T) {
	base := time.Now().UTC()
	api, app, key := touchTestAPI(t, 60, base)
	api.now = func() time.Time { return base.Add(100 * time.Millisecond) }
	hitTouchAPI(t, app, key)
	if _, err := api.dao.db.Exec(`CREATE TRIGGER fail_touch BEFORE UPDATE ON image_cache_index BEGIN SELECT RAISE(FAIL,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if err := api.flushTouches(); err == nil {
		t.Fatal("expected write failure")
	}
	if len(api.pendingTouches) != 1 {
		t.Fatal("failed flush lost latest hit")
	}
	if _, err := api.cleanupExpired(100); err == nil {
		t.Fatal("GC must stop if renewal flush fails")
	}
}

func TestServiceCloseFlushesPendingTouches(t *testing.T) {
	root := t.TempDir()
	service, err := NewService(t.Context(), Config{StorageDir: root, GCInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close() })
	base := time.Now().UTC()
	key := strings.Repeat("c", 64)
	file := filepath.Join(root, "image.png")
	if err := os.WriteFile(file, []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := service.api.dao.SaveRecord(&CacheRecord{Sha256Key: key, APIPath: "api/pjsk/profile", UserID: "public", FilePath: file, CreatedAt: base, LastUsedAt: base, TTLSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	now := base.Add(100 * time.Millisecond)
	service.api.now = func() time.Time { return now }
	app := fiber.New()
	service.RegisterRoutes(app)
	hitTouchAPI(t, app, key)
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := InitDB(service.cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	record, err := NewDAO(db).GetRecord(key)
	if err != nil || !record.LastUsedAt.Equal(now) {
		t.Fatalf("shutdown lost touch: record=%+v err=%v", record, err)
	}
}

func TestCacheHitsDuringGC(t *testing.T) {
	api, app, key := touchTestAPI(t, 60, time.Now().UTC())
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for range 100 {
				response, err := app.Test(httptest.NewRequest("GET", "/cache?key="+key, nil))
				if err != nil {
					t.Error(err)
					return
				}
				response.Body.Close()
				if response.StatusCode != 200 {
					t.Errorf("concurrent cache hit status=%d", response.StatusCode)
					return
				}
			}
		})
	}
	workers.Go(func() {
		for range 30 {
			if n, err := api.cleanupExpired(100); err != nil || n != 0 {
				t.Errorf("GC n=%d err=%v", n, err)
				return
			}
		}
	})
	workers.Wait()
	if err := api.flushTouches(); err != nil {
		t.Fatal(err)
	}
	record, err := api.dao.GetRecord(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(record.FilePath); err != nil {
		t.Fatal(err)
	}
}

func TestLazyExpiryPreservesSharedFileWithPendingRenewal(t *testing.T) {
	base := time.Now().UTC().Add(-4100 * time.Millisecond)
	api, app, expiredKey := touchTestAPI(t, 4, base)
	renewed, err := api.dao.GetRecord(expiredKey)
	if err != nil {
		t.Fatal(err)
	}
	renewed.Sha256Key = strings.Repeat("d", 64)
	if err := api.dao.SaveRecord(renewed); err != nil {
		t.Fatal(err)
	}
	api.now = func() time.Time { return base.Add(900 * time.Millisecond) }
	hitTouchAPI(t, app, renewed.Sha256Key)
	api.now = time.Now
	response, err := app.Test(httptest.NewRequest("GET", "/cache?key="+expiredKey, nil))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatalf("expired row status=%d", response.StatusCode)
	}
	if _, err := os.Stat(renewed.FilePath); err != nil {
		t.Fatalf("lazy expiry removed renewed shared image: %v", err)
	}
	hitTouchAPI(t, app, renewed.Sha256Key)
}
