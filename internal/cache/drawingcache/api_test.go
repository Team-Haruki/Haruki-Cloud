package drawingcache

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func TestCacheAPIPostGetAndStats(t *testing.T) {
	db := openTestDB(t)
	dao := NewDAO(db)
	storageDir := t.TempDir()
	api := NewAPI(dao, storageDir)
	app := fiber.New()
	api.RegisterRoutes(app)

	key := strings.Repeat("a", 64)
	targetPath := filepath.Join(storageDir, "api", "pjsk", "profile", "public", key+".png")
	writeTestCacheFile(t, targetPath, []byte("profile"))

	postResp := doCacheRequest(t, app, http.MethodPost, "/cache", url.Values{
		"key":       {key},
		"ttl":       {"600"},
		"api_path":  {"/api/pjsk/profile/"},
		"user_id":   {"public"},
		"file_path": {targetPath},
	})
	if postResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /cache status=%d body=%s", postResp.StatusCode, string(postResp.Body))
	}

	var postBody struct {
		Message    string `json:"message"`
		APIPath    string `json:"api_path"`
		UserID     string `json:"user_id"`
		FilePath   string `json:"file_path"`
		TTLSeconds int64  `json:"ttl_seconds"`
		ExpiresAt  string `json:"expires_at"`
	}
	decodeJSON(t, postResp.Body, &postBody)
	if postBody.Message != "ok" || postBody.APIPath != "api/pjsk/profile" || postBody.UserID != "public" {
		t.Fatalf("unexpected POST payload: %+v", postBody)
	}
	if postBody.FilePath != targetPath || postBody.TTLSeconds != 600 || postBody.ExpiresAt == "" {
		t.Fatalf("unexpected POST cache metadata: %+v", postBody)
	}

	getResp := doCacheRequest(t, app, http.MethodGet, "/cache?key="+key+"&api_path=/api/pjsk/profile", nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /cache status=%d body=%s", getResp.StatusCode, string(getResp.Body))
	}
	var getBody struct {
		Key        string `json:"key"`
		FilePath   string `json:"file_path"`
		TTLSeconds int64  `json:"ttl_seconds"`
	}
	decodeJSON(t, getResp.Body, &getBody)
	if getBody.Key != key || getBody.FilePath != targetPath || getBody.TTLSeconds != 600 {
		t.Fatalf("unexpected GET payload: %+v", getBody)
	}

	statsResp := doCacheRequest(t, app, http.MethodGet, "/cache/stats?api_path=/api/pjsk/profile", nil)
	if statsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /cache/stats status=%d body=%s", statsResp.StatusCode, string(statsResp.Body))
	}
	var stats cacheStatsSnapshot
	decodeJSON(t, statsResp.Body, &stats)
	if stats.Totals.Hits != 1 || stats.Totals.Misses != 0 || stats.Totals.Stores != 1 || stats.Totals.HitRate != 1 {
		t.Fatalf("unexpected totals: %+v", stats.Totals)
	}
	if len(stats.Paths) != 1 || stats.Paths[0].APIPath != "api/pjsk/profile" || stats.Paths[0].Stores != 1 {
		t.Fatalf("unexpected path stats: %+v", stats.Paths)
	}
}

func TestCacheAPIExpiredRecordDeletesFileAndRow(t *testing.T) {
	db := openTestDB(t)
	dao := NewDAO(db)
	storageDir := t.TempDir()
	api := NewAPI(dao, storageDir)
	app := fiber.New()
	api.RegisterRoutes(app)

	now := time.Date(2026, time.April, 28, 12, 0, 0, 0, time.UTC)
	api.now = func() time.Time { return now }
	key := strings.Repeat("b", 64)
	targetPath := filepath.Join(storageDir, "api", "pjsk", "music", "list", "public", key+".png")
	writeTestCacheFile(t, targetPath, []byte("music"))

	if err := dao.SaveRecord(&CacheRecord{
		Sha256Key:  key,
		APIPath:    "api/pjsk/music/list",
		UserID:     "public",
		FilePath:   targetPath,
		CreatedAt:  now.Add(-2 * time.Hour),
		LastUsedAt: now.Add(-2 * time.Hour),
		TTLSeconds: 60,
		ExpiresAt:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("SaveRecord: %v", err)
	}

	resp := doCacheRequest(t, app, http.MethodGet, "/cache?key="+key, nil)
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(string(resp.Body), "record expired") {
		t.Fatalf("expected expired miss, status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	if _, err := os.Stat(targetPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected expired file to be removed, err=%v", err)
	}
	if _, err := dao.GetRecord(key); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("expected expired row to be removed, err=%v", err)
	}
}

func TestCacheAPIMissingFileDeletesRowAndRecordsStats(t *testing.T) {
	db := openTestDB(t)
	dao := NewDAO(db)
	storageDir := t.TempDir()
	api := NewAPI(dao, storageDir)
	app := fiber.New()
	api.RegisterRoutes(app)

	now := time.Now().UTC()
	key := strings.Repeat("c", 64)
	targetPath := filepath.Join(storageDir, "api", "pjsk", "card", "detail", "public", key+".png")
	if err := dao.SaveRecord(&CacheRecord{
		Sha256Key:  key,
		APIPath:    "api/pjsk/card/detail",
		UserID:     "public",
		FilePath:   targetPath,
		CreatedAt:  now,
		LastUsedAt: now,
		TTLSeconds: 600,
	}); err != nil {
		t.Fatalf("SaveRecord: %v", err)
	}

	resp := doCacheRequest(t, app, http.MethodGet, "/cache?key="+key, nil)
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(string(resp.Body), "file not found") {
		t.Fatalf("expected missing-file miss, status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	if _, err := dao.GetRecord(key); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("expected missing-file row to be removed, err=%v", err)
	}
	stats := api.stats.snapshot("api/pjsk/card/detail")
	if len(stats.Paths) != 1 || stats.Paths[0].MissingFiles != 1 || stats.Paths[0].Misses != 1 {
		t.Fatalf("unexpected missing-file stats: %+v", stats.Paths)
	}
}

func TestCacheAPIPreservesInfiniteTTL(t *testing.T) {
	db := openTestDB(t)
	dao := NewDAO(db)
	storageDir := t.TempDir()
	api := NewAPI(dao, storageDir)
	app := fiber.New()
	api.RegisterRoutes(app)

	key := strings.Repeat("d", 64)
	targetPath := filepath.Join(storageDir, "api", "pjsk", "card", "list", "public", key+".png")
	writeTestCacheFile(t, targetPath, []byte("cards"))

	resp := doCacheRequest(t, app, http.MethodPost, "/cache", url.Values{
		"key":       {key},
		"ttl":       {"0"},
		"api_path":  {"api/pjsk/card/list"},
		"file_path": {targetPath},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /cache status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	var body struct {
		TTLSeconds int64  `json:"ttl_seconds"`
		ExpiresAt  string `json:"expires_at"`
	}
	decodeJSON(t, resp.Body, &body)
	if body.TTLSeconds != 0 || body.ExpiresAt != "" {
		t.Fatalf("expected infinite ttl response, got %+v", body)
	}

	record, err := dao.GetRecord(key)
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if record.TTLSeconds != 0 || !record.ExpiresAt.Equal(infiniteExpiresAt) {
		t.Fatalf("expected infinite ttl record, got ttl=%d expires=%s", record.TTLSeconds, record.ExpiresAt)
	}
}

func TestCleanupExpiredBatchIgnoresInfiniteTTLRecords(t *testing.T) {
	db := openTestDB(t)
	dao := NewDAO(db)
	storageDir := t.TempDir()
	now := time.Now().UTC().Add(-2 * time.Hour)

	expiredKey := strings.Repeat("e", 64)
	expiredPath := filepath.Join(storageDir, "api", "pjsk", "event", "list", "public", expiredKey+".png")
	writeTestCacheFile(t, expiredPath, []byte("expired"))
	if err := dao.SaveRecord(&CacheRecord{
		Sha256Key:  expiredKey,
		APIPath:    "api/pjsk/event/list",
		UserID:     "public",
		FilePath:   expiredPath,
		CreatedAt:  now.Add(-2 * time.Hour),
		LastUsedAt: now.Add(-2 * time.Hour),
		TTLSeconds: 60,
		ExpiresAt:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("save expired record: %v", err)
	}

	infiniteKey := strings.Repeat("f", 64)
	infinitePath := filepath.Join(storageDir, "api", "pjsk", "card", "detail", "public", infiniteKey+".png")
	writeTestCacheFile(t, infinitePath, []byte("infinite"))
	if err := dao.SaveRecord(&CacheRecord{
		Sha256Key:  infiniteKey,
		APIPath:    "api/pjsk/card/detail",
		UserID:     "public",
		FilePath:   infinitePath,
		CreatedAt:  now,
		LastUsedAt: now,
		TTLSeconds: 0,
	}); err != nil {
		t.Fatalf("save infinite record: %v", err)
	}

	cleaned, err := cleanupExpiredBatch(db, storageDir, 10)
	if err != nil {
		t.Fatalf("cleanupExpiredBatch: %v", err)
	}
	if cleaned != 1 {
		t.Fatalf("expected 1 cleaned record, got %d", cleaned)
	}
	if _, err := dao.GetRecord(infiniteKey); err != nil {
		t.Fatalf("expected infinite record to remain, got error: %v", err)
	}
}

func TestCacheServiceDefaultsDBPathAndRegistersRoutes(t *testing.T) {
	storageDir := t.TempDir()
	service, err := NewService(t.Context(), Config{StorageDir: storageDir, GCInterval: -time.Second})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })

	if service.Config().DBPath != filepath.Join(storageDir, DefaultDBFilename) {
		t.Fatalf("unexpected db path: %s", service.Config().DBPath)
	}

	app := fiber.New()
	service.RegisterRoutes(app)
	resp := doCacheRequest(t, app, http.MethodGet, "/cache/stats", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /cache/stats status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func writeTestCacheFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type cacheTestResponse struct {
	StatusCode int
	Body       []byte
}

func doCacheRequest(t *testing.T, app *fiber.App, method, target string, form url.Values) cacheTestResponse {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "localhost"
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second, FailOnTimeout: true})
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return cacheTestResponse{StatusCode: resp.StatusCode, Body: payload}
}

func decodeJSON(t *testing.T, body []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("decode JSON %s: %v", string(body), err)
	}
}

func TestNormalizeCacheStatsPathKeepsSlashSeparatedAPIPath(t *testing.T) {
	got := normalizeCacheStatsPath(" /api/pjsk/profile/ ")
	if got != "api/pjsk/profile" {
		t.Fatalf("unexpected normalized api path: %s", got)
	}
}

func TestValidateSHA256KeyRejectsBadKeys(t *testing.T) {
	if err := ValidateSHA256Key(strings.Repeat("0", 64)); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	if err := ValidateSHA256Key(fmt.Sprintf("%063s", "0")); err == nil {
		t.Fatalf("expected short key to be rejected")
	}
}
