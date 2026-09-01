package assets

import (
	"container/list"
	"testing"
	"time"
)

func TestAssetResolutionCacheEdgeBranches(t *testing.T) {
	var nilCache *assetResolutionCache
	if _, ok := nilCache.lookup("key", time.Now()); ok || nilCache.storeForGeneration("key", "path", time.Now(), 0) || nilCache.currentGeneration() != 0 {
		t.Fatal("nil resolution cache returned a value")
	}
	nilCache.store("key", "path", time.Now())

	now := time.Now()
	cache := &assetResolutionCache{ttl: time.Minute, maxEntries: 1}
	cache.store("one", "first", now)
	cache.store("one", "updated", now)
	if path, ok := cache.lookup("one", now); !ok || path != "updated" {
		t.Fatalf("updated resolution = %q, %v", path, ok)
	}
	generation := cache.currentGeneration()
	if cache.storeForGeneration("stale", "path", now, generation+1) {
		t.Fatal("stale generation was stored")
	}
	if !cache.storeForGeneration("two", "second", now, generation) {
		t.Fatal("current generation was rejected")
	}
	if _, ok := cache.lookup("one", now); ok {
		t.Fatal("least recently used resolution was not evicted")
	}
	cache.entries["two"].expiresAt = now.Add(-time.Second)
	if _, ok := cache.lookup("two", now); ok {
		t.Fatal("expired resolution remained cached")
	}
	cache.clear()
	if cache.currentGeneration() != generation+1 {
		t.Fatal("cache generation was not advanced")
	}
	if (&assetResolutionCache{}).entryLimit() != assetResolutionMaxEntries {
		t.Fatal("default resolution entry limit mismatch")
	}
}

func TestAssetDirectoryCacheEdgeBranches(t *testing.T) {
	var nilCache *assetDirectoryCache
	if _, ok := nilCache.lookup("root", time.Time{}); ok {
		t.Fatal("nil directory cache returned a value")
	}
	nilCache.store("root", nil)

	now := time.Now()
	cache := &assetDirectoryCache{maxEntries: 1, maxNames: 2}
	first := testAssetDirectoryIndex(now, "a")
	cache.store("root", first)
	if got, ok := cache.lookup("root", now); !ok || got != first {
		t.Fatalf("directory cache lookup = %+v, %v", got, ok)
	}
	cache.store("root", testAssetDirectoryIndex(now, "b"))
	cache.store("oversized", testAssetDirectoryIndex(now, "1", "2", "3"))
	if _, ok := cache.lookup("root", now.Add(time.Second)); ok {
		t.Fatal("stale directory index remained cached")
	}
	cache.removeLocked("missing", nil)
	cache.indexed = -1
	cache.removeLocked("missing", &assetDirectoryCacheEntry{index: testAssetDirectoryIndex(now, "x")})
	if cache.indexed != 0 {
		t.Fatalf("negative indexed count was not clamped: %d", cache.indexed)
	}
	if (&assetDirectoryCache{}).entryLimit() != assetDirectoryMaxEntries || (&assetDirectoryCache{}).nameLimit() != assetDirectoryMaxNames {
		t.Fatal("default directory cache limits mismatch")
	}
}

func TestAssetHelperZeroValueCacheFallbacks(t *testing.T) {
	helper := &AssetHelper{}
	if helper.fileSystem() == nil || helper.cache() == nil || helper.resolutions() == nil {
		t.Fatal("zero-value helper did not provide fallbacks")
	}
	var nilHelper *AssetHelper
	if nilHelper.resolutions() == nil {
		t.Fatal("nil helper did not provide a resolution cache")
	}
	cache := &assetResolutionCache{entries: map[string]*assetResolutionEntry{}, recent: list.List{}, ttl: 0}
	cache.store("key", "path", time.Now())
	if _, ok := cache.lookup("key", time.Now()); ok || cache.storeForGeneration("key", "path", time.Now(), 0) {
		t.Fatal("disabled resolution cache accepted an entry")
	}
}
