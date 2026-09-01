package mysekai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/testutil"
)

func TestMysekaiRunFixtureCodeCacheReplacesPreviousSignatureForPath(t *testing.T) {
	root := t.TempDir()
	fullPath := filepath.Join(root, "jp", "mysekairun", "jp.html")
	{
		err := os.MkdirAll(filepath.Dir(fullPath), 0o755)
		testutil.Require(t, !(err != nil), "mkdir fixture code dir: %v", err)
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
		{
			err := os.WriteFile(fullPath, []byte(content), 0o644)
			testutil.Require(t, !(err != nil), "write fixture codes: %v", err)
		}
		{

			err := os.Chtimes(fullPath, modTime, modTime)
			testutil.Require(t, !(err != nil), "set fixture code modtime: %v", err)
		}

	}

	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{
		LocalDir:      root,
		AllowFallback: true,
	})
	baseTime := time.Now().Add(-time.Hour).Truncate(time.Second)
	writeFixtureCodes("A1", baseTime)
	{
		got := controller.loadMySekaiRunFixtureFriendcodes(renderregion.JP, "Fixture A")
		{
			testutil.Require(t, !(len(got) != 1), "initial fixture codes = %+v", got)
			testutil.Require(t, !(got[0] != "A1"), "initial fixture codes = %+v", got)
		}
	}

	firstRaw, ok := mysekaiRunFixtureCodeCache.Load(fullPath)
	testutil.RequireArgs(t, ok, "fixture code index was not cached by path")

	first, ok := firstRaw.(mysekaiRunFixtureCodeCacheEntry)
	{
		testutil.Require(t, ok, "unexpected initial cache entry: %#v", firstRaw)
		testutil.Require(t, !(first.signature == ""), "unexpected initial cache entry: %#v", firstRaw)
	}

	// Keep the file size unchanged and move only mtime so this specifically
	// verifies that a new signature replaces the old index at the same key.
	writeFixtureCodes("B2", baseTime.Add(time.Second))
	{
		got := controller.loadMySekaiRunFixtureFriendcodes(renderregion.JP, "Fixture A")
		{
			testutil.Require(t, !(len(got) != 1), "updated fixture codes = %+v", got)
			testutil.Require(t, !(got[0] != "B2"), "updated fixture codes = %+v", got)
		}
	}

	secondRaw, ok := mysekaiRunFixtureCodeCache.Load(fullPath)
	testutil.RequireArgs(t, ok, "updated fixture code index was not cached")

	second, ok := secondRaw.(mysekaiRunFixtureCodeCacheEntry)
	{
		testutil.Require(t, ok, "cache signature was not replaced: before=%#v after=%#v", firstRaw, secondRaw)
		testutil.Require(t, !(second.signature == first.signature), "cache signature was not replaced: before=%#v after=%#v", firstRaw, secondRaw)
	}

	matchingKeys := 0
	mysekaiRunFixtureCodeCache.Range(func(key, _ any) bool {
		path, _ := key.(string)
		if path == fullPath || strings.HasPrefix(path, fullPath+"|") {
			matchingKeys++
			testutil.Check(t, !(path != fullPath), "cache retained signature in key: %q", path)

		}
		return true
	})
	testutil.Require(t, !(matchingKeys != 1), "cache entries for path = %d, want 1", matchingKeys)

}
