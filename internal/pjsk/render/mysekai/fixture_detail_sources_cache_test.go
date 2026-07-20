package mysekai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestMysekaiRunFixtureCodeCacheReplacesPreviousSignatureForPath(t *testing.T) {
	root := t.TempDir()
	fullPath := filepath.Join(root, "jp", "mysekairun", "jp.html")
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir fixture code dir: %v", err)
	}
	cleanup := func() {
		mysekaiRunFixtureCodeCache.Range(func(key, _ any) bool {
			path, _ := key.(string)
			if path == fullPath || strings.HasPrefix(path, fullPath+"|") {
				mysekaiRunFixtureCodeCache.Delete(key)
			}
			return true
		})
	}
	cleanup()
	t.Cleanup(cleanup)

	writeFixtureCodes := func(code string, modTime time.Time) {
		t.Helper()
		content := "<table><tr><td>1</td><td>Fixture A</td><td>" + code + "</td></tr></table>"
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture codes: %v", err)
		}
		if err := os.Chtimes(fullPath, modTime, modTime); err != nil {
			t.Fatalf("set fixture code modtime: %v", err)
		}
	}

	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{
		LocalDir:      root,
		AllowFallback: true,
	})
	baseTime := time.Now().Add(-time.Hour).Truncate(time.Second)
	writeFixtureCodes("A1", baseTime)
	if got := controller.loadMySekaiRunFixtureFriendcodes(renderregion.JP, "Fixture A"); len(got) != 1 || got[0] != "A1" {
		t.Fatalf("initial fixture codes = %+v", got)
	}
	firstRaw, ok := mysekaiRunFixtureCodeCache.Load(fullPath)
	if !ok {
		t.Fatal("fixture code index was not cached by path")
	}
	first, ok := firstRaw.(mysekaiRunFixtureCodeCacheEntry)
	if !ok || first.signature == "" {
		t.Fatalf("unexpected initial cache entry: %#v", firstRaw)
	}

	// Keep the file size unchanged and move only mtime so this specifically
	// verifies that a new signature replaces the old index at the same key.
	writeFixtureCodes("B2", baseTime.Add(time.Second))
	if got := controller.loadMySekaiRunFixtureFriendcodes(renderregion.JP, "Fixture A"); len(got) != 1 || got[0] != "B2" {
		t.Fatalf("updated fixture codes = %+v", got)
	}
	secondRaw, ok := mysekaiRunFixtureCodeCache.Load(fullPath)
	if !ok {
		t.Fatal("updated fixture code index was not cached")
	}
	second, ok := secondRaw.(mysekaiRunFixtureCodeCacheEntry)
	if !ok || second.signature == first.signature {
		t.Fatalf("cache signature was not replaced: before=%#v after=%#v", firstRaw, secondRaw)
	}

	matchingKeys := 0
	mysekaiRunFixtureCodeCache.Range(func(key, _ any) bool {
		path, _ := key.(string)
		if path == fullPath || strings.HasPrefix(path, fullPath+"|") {
			matchingKeys++
			if path != fullPath {
				t.Errorf("cache retained signature in key: %q", path)
			}
		}
		return true
	})
	if matchingKeys != 1 {
		t.Fatalf("cache entries for path = %d, want 1", matchingKeys)
	}
}
