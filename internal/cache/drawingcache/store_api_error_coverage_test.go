package drawingcache

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func newDrawingCacheTestDAO(t *testing.T) *DAO {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatalf("init cache database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewDAO(db)
}

func validDrawingCacheRecord(key string) *CacheRecord {
	createdAt := time.Unix(1_700_000_000, 0).UTC()
	return &CacheRecord{
		Sha256Key:  key,
		APIPath:    "pjsk/profile",
		UserID:     "10001",
		FilePath:   "/tmp/cache.png",
		CreatedAt:  createdAt,
		TTLSeconds: 60,
	}
}

func TestInitDBRejectsInvalidPaths(t *testing.T) {
	if _, err := InitDB(" "); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("blank database path error = %v", err)
	}
	parentFile := filepath.Join(t.TempDir(), "parent")
	if err := os.WriteFile(parentFile, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InitDB(filepath.Join(parentFile, "cache.db")); err == nil || !strings.Contains(err.Error(), "create db dir") {
		t.Fatalf("parent file database error = %v", err)
	}
	if _, err := InitDB(t.TempDir()); err == nil || !strings.Contains(err.Error(), "ping sqlite") {
		t.Fatalf("directory database error = %v", err)
	}
}

func TestDrawingCacheDAORejectsInvalidInputs(t *testing.T) {
	var dao *DAO
	key := strings.Repeat("a", 64)
	if _, err := dao.GetRecord(key); err == nil {
		t.Fatal("nil DAO get succeeded")
	}
	if err := dao.SaveRecord(validDrawingCacheRecord(key)); err == nil {
		t.Fatal("nil DAO save succeeded")
	}
	if err := dao.TouchRecordOnHit(key, time.Now(), 60); err == nil {
		t.Fatal("nil DAO touch succeeded")
	}
	if err := dao.DeleteRecord(key); err == nil {
		t.Fatal("nil DAO delete succeeded")
	}

	dao = newDrawingCacheTestDAO(t)
	if _, err := dao.GetRecord("bad"); err == nil {
		t.Fatal("invalid key get succeeded")
	}
	if err := dao.SaveRecord(nil); err == nil {
		t.Fatal("nil record save succeeded")
	}
	if err := dao.TouchRecordOnHit("bad", time.Now(), 60); err == nil {
		t.Fatal("invalid key touch succeeded")
	}
	if err := dao.DeleteRecord("bad"); err == nil {
		t.Fatal("invalid key delete succeeded")
	}
}

func TestDrawingCacheSaveValidation(t *testing.T) {
	dao := newDrawingCacheTestDAO(t)
	key := strings.Repeat("b", 64)
	for _, test := range []struct {
		name   string
		mutate func(*CacheRecord)
	}{
		{name: "invalid key", mutate: func(record *CacheRecord) { record.Sha256Key = "bad" }},
		{name: "empty file", mutate: func(record *CacheRecord) { record.FilePath = " " }},
		{name: "empty api", mutate: func(record *CacheRecord) { record.APIPath = " " }},
		{name: "empty user", mutate: func(record *CacheRecord) { record.UserID = " " }},
		{name: "zero created", mutate: func(record *CacheRecord) { record.CreatedAt = time.Time{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			record := validDrawingCacheRecord(key)
			test.mutate(record)
			if err := dao.SaveRecord(record); err == nil {
				t.Fatal("invalid record save succeeded")
			}
		})
	}
}

func TestDrawingCacheSaveNormalizesLifetimes(t *testing.T) {
	dao := newDrawingCacheTestDAO(t)
	finite := validDrawingCacheRecord(strings.Repeat("c", 64))
	if err := dao.SaveRecord(finite); err != nil {
		t.Fatalf("save finite record: %v", err)
	}
	if !finite.LastUsedAt.Equal(finite.CreatedAt) || finite.ExpiresAt.IsZero() {
		t.Fatalf("finite lifetime = %#v", finite)
	}

	infinite := validDrawingCacheRecord(strings.Repeat("d", 64))
	infinite.TTLSeconds = 0
	if err := dao.SaveRecord(infinite); err != nil {
		t.Fatalf("save infinite record: %v", err)
	}
	loaded, err := dao.GetRecord(infinite.Sha256Key)
	if err != nil || loaded.TTLSeconds != 0 || !loaded.ExpiresAt.Equal(infiniteExpiresAt) {
		t.Fatalf("infinite record = %#v, %v", loaded, err)
	}
	if formatCacheRecordExpiresAt(nil) != "" || formatCacheRecordExpiresAt(loaded) != "" {
		t.Fatal("infinite expiry should be omitted")
	}
}

func TestDrawingCacheGetRejectsCorruptTimestamps(t *testing.T) {
	dao := newDrawingCacheTestDAO(t)
	for index, test := range []struct {
		created string
		last    string
		expires string
		want    string
	}{
		{created: "bad", last: "", expires: time.Now().Format(sqliteTimeLayout), want: "created_at"},
		{created: time.Now().Format(sqliteTimeLayout), last: "bad", expires: time.Now().Format(sqliteTimeLayout), want: "last_used_at"},
		{created: time.Now().Format(sqliteTimeLayout), last: "", expires: "bad", want: "expires_at"},
	} {
		key := fmt.Sprintf("%064x", index+1)
		_, err := dao.db.Exec(`INSERT INTO image_cache_index
			(sha256_key, api_path, user_id, file_path, created_at, last_used_at, ttl_seconds, expires_at)
			VALUES (?, 'api', 'user', '/tmp/file', ?, ?, 60, ?)`, key, test.created, test.last, test.expires)
		if err != nil {
			t.Fatalf("insert corrupt record: %v", err)
		}
		if _, err := dao.GetRecord(key); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("corrupt %s error = %v", test.want, err)
		}
	}
}

func TestDrawingCacheDAOReportsClosedDatabase(t *testing.T) {
	dao := newDrawingCacheTestDAO(t)
	key := strings.Repeat("e", 64)
	if err := dao.db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.GetRecord(key); err == nil || !strings.Contains(err.Error(), "query cache record") {
		t.Fatalf("closed get error = %v", err)
	}
	if err := dao.SaveRecord(validDrawingCacheRecord(key)); err == nil || !strings.Contains(err.Error(), "upsert cache record") {
		t.Fatalf("closed save error = %v", err)
	}
	if err := dao.TouchRecordOnHit(key, time.Now(), 60); err == nil || !strings.Contains(err.Error(), "touch cache record") {
		t.Fatalf("closed touch error = %v", err)
	}
	if err := dao.DeleteRecord(key); err == nil || !strings.Contains(err.Error(), "delete cache record") {
		t.Fatalf("closed delete error = %v", err)
	}
}

func TestDrawingCacheTouchValidationAndMissingRecord(t *testing.T) {
	dao := newDrawingCacheTestDAO(t)
	key := strings.Repeat("f", 64)
	if err := dao.TouchRecordOnHit(key, time.Time{}, 60); err == nil || !strings.Contains(err.Error(), "last_used_at") {
		t.Fatalf("zero touch time error = %v", err)
	}
	if err := dao.TouchRecordOnHit(key, time.Now(), 60); err != ErrRecordNotFound {
		t.Fatalf("missing touch error = %v", err)
	}
	if err := dao.DeleteRecord(key); err != nil {
		t.Fatalf("delete missing record: %v", err)
	}
}

func newDrawingCacheTestApp(t *testing.T) (*fiber.App, *DAO, string) {
	t.Helper()
	dao := newDrawingCacheTestDAO(t)
	storageDir := t.TempDir()
	api := NewAPI(dao, storageDir)
	app := fiber.New()
	api.RegisterRoutes(app)
	return app, dao, storageDir
}

func TestDrawingCacheAPIRejectsInvalidGetRequests(t *testing.T) {
	app, _, _ := newDrawingCacheTestApp(t)
	assertDrawingCacheStatus(t, app, http.MethodGet, "/cache?key=bad", nil, fiber.StatusBadRequest)
	missingKey := strings.Repeat("a", 64)
	assertDrawingCacheStatus(t, app, http.MethodGet, "/cache?key="+missingKey, nil, fiber.StatusNotFound)
}

func TestDrawingCacheAPIRejectsInvalidPostRequests(t *testing.T) {
	app, _, storageDir := newDrawingCacheTestApp(t)
	key := strings.Repeat("a", 64)
	outside := filepath.Join(filepath.Dir(storageDir), "outside.png")
	missing := filepath.Join(storageDir, "missing.png")
	for _, form := range []url.Values{
		{"key": {"bad"}, "ttl": {"0"}, "api_path": {"api"}},
		{"key": {key}, "ttl": {"bad"}, "api_path": {"api"}},
		{"key": {key}, "ttl": {"-1"}, "api_path": {"api"}},
		{"key": {key}, "ttl": {"0"}, "api_path": {" "}},
		{"key": {key}, "ttl": {"0"}, "api_path": {"api"}, "file_path": {outside}},
		{"key": {key}, "ttl": {"0"}, "api_path": {"api"}, "file_path": {missing}},
	} {
		assertDrawingCacheStatus(t, app, http.MethodPost, "/cache", form, fiber.StatusBadRequest)
	}
}

func TestDrawingCacheAPIReportsDatabaseFailures(t *testing.T) {
	app, dao, storageDir := newDrawingCacheTestApp(t)
	key := strings.Repeat("b", 64)
	target := filepath.Join(storageDir, "target.png")
	if err := os.WriteFile(target, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dao.db.Close(); err != nil {
		t.Fatal(err)
	}
	assertDrawingCacheStatus(t, app, http.MethodGet, "/cache?key="+key, nil, fiber.StatusInternalServerError)
	form := url.Values{"key": {key}, "ttl": {"0"}, "api_path": {"api"}, "file_path": {target}}
	assertDrawingCacheStatus(t, app, http.MethodPost, "/cache", form, fiber.StatusInternalServerError)
}

func assertDrawingCacheStatus(t *testing.T, app *fiber.App, method string, target string, form url.Values, want int) {
	t.Helper()
	var request *http.Request
	var err error
	if form == nil {
		request, err = http.NewRequestWithContext(context.Background(), method, target, nil)
	} else {
		request, err = http.NewRequestWithContext(context.Background(), method, target, strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if err != nil {
		t.Fatal(err)
	}
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("%s %s status = %d, want %d", method, target, response.StatusCode, want)
	}
}
