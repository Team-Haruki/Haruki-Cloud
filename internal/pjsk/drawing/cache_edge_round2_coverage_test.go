package drawing

import (
	"container/list"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalRenderCacheZeroValueAndManualEdges(t *testing.T) {
	testNilAndZeroLocalRenderCache(t)
	testLocalRenderCacheRemovalEdges(t)
}

func testNilAndZeroLocalRenderCache(t *testing.T) {
	t.Helper()
	var nilCache *localRenderCache
	if data, ok := nilCache.get("missing"); ok || data != nil {
		t.Fatalf("nil cache get = %q, %v", data, ok)
	}
	nilCache.set("ignored", []byte("x"), 0, false)

	cache := &localRenderCache{maxEntries: 2, maxBytes: 8, ttl: time.Millisecond}
	if data, ok := cache.get("missing"); ok || data != nil {
		t.Fatalf("empty cache get = %q, %v", data, ok)
	}
	cache.set("value", []byte("abc"), 0, false)
	cache.entries["value"].element = nil
	if data, ok := cache.get("value"); !ok || string(data) != "abc" {
		t.Fatalf("cache get without LRU element = %q, %v", data, ok)
	}
	cache.entries["value"].expiresAt = time.Now().Add(-time.Second)
	if _, ok := cache.get("value"); ok {
		t.Fatal("expired entry remained in cache")
	}
	cache.set("oversized", make([]byte, 9), 0, false)
	if len(cache.entries) != 0 {
		t.Fatalf("oversized entry stored: %+v", cache.entries)
	}
}

func testLocalRenderCacheRemovalEdges(t *testing.T) {
	t.Helper()
	cache := &localRenderCache{
		entries:    map[string]*localRenderEntry{"nil": nil},
		lru:        list.New(),
		maxEntries: 0,
		maxBytes:   1,
	}
	cache.sweepExpiredLocked(time.Now())
	cache.entries["orphan"] = &localRenderEntry{size: 2}
	cache.totalBytes = 2
	cache.evictLocked()
	other := &localRenderEntry{}
	cache.removeEntryLocked("orphan", other)
	if cache.entries["orphan"] == nil {
		t.Fatal("mismatched entry was removed")
	}
	cache.entries["orphan"].size = 4
	cache.totalBytes = 1
	cache.removeEntryLocked("orphan", cache.entries["orphan"])
	if cache.totalBytes != 0 {
		t.Fatalf("negative byte count was not clamped: %d", cache.totalBytes)
	}
}

func TestRemoteRenderCacheFileSafetyEdges(t *testing.T) {
	root := t.TempDir()
	client := &RenderCacheClient{storageDir: root}
	if _, err := client.readCacheFile(root); err == nil {
		t.Fatal("directory was accepted as a cache file")
	}
	targetDirectory := filepath.Join(root, "target")
	if err := os.Mkdir(targetDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := client.prepareCacheTarget(targetDirectory); err == nil {
		t.Fatal("directory was accepted as a cache target")
	}
	if err := ensureRenderCacheDirectory(root, filepath.Dir(root)); err == nil {
		t.Fatal("escaping directory was accepted")
	}
	fileComponent := filepath.Join(root, "component")
	if err := os.WriteFile(fileComponent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureRenderCacheDirectoryComponent(fileComponent); err == nil {
		t.Fatal("file component was accepted as a directory")
	}
	if _, err := resolveContainedCacheFile(root, filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing cache file resolved")
	}
	if _, _, err := absoluteContainedCachePath(root, filepath.Join(root, "safe")); err != nil {
		t.Fatalf("contained path rejected: %v", err)
	}
	if err := writeRenderCacheFileAtomic(filepath.Join(root, "missing", "file"), []byte("x")); err == nil {
		t.Fatal("atomic write with missing parent succeeded")
	}
	if err := writeRenderCacheFileAtomic(targetDirectory, []byte("x")); err == nil {
		t.Fatal("atomic write replaced a directory")
	}
	webp := []byte("RIFF\x10\x00\x00\x00WEBPVP8 ")
	if got := renderCacheFileExtFromData(webp); got != ".webp" {
		t.Fatalf("WebP extension = %q", got)
	}
	if got := renderCacheFileExtFromData(make([]byte, 513)); got != ".png" {
		t.Fatalf("long unknown data extension = %q", got)
	}
}
