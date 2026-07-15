package mysekai

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
)

func TestMysekaiBirthdayRefreshIconMissIsNotCached(t *testing.T) {
	root := t.TempDir()
	helper := assets.NewAssetHelper(root, nil)
	controller := NewController(nil, nil, renderregion.JP, helper, MasterdataOptions{})
	now := time.Date(2026, time.July, 15, 0, 0, 0, 0, time.UTC)
	item := map[string]any{"givenNameEnglish": "Haruka"}
	cacheKey := strings.Join(helper.Roots(), "\x00") + "|jp|haruka|" + strconv.Itoa(now.Year())

	if got := controller.resolveMysekaiBirthdayRefreshIconPath(renderregion.JP, item, now); got != "" {
		t.Fatalf("initial missing icon path = %q, want empty", got)
	}
	if _, ok := mysekaiBirthdayRefreshIconCache.Load(cacheKey); ok {
		t.Fatal("missing icon must not be cached")
	}

	dir := filepath.Join(root, "jp-assets", assets.RegionAssetOnDemand, "mysekai", "birthday", "haruka_2026")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir birthday icon dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "icon_refresh.png"), []byte("png"), 0o644); err != nil {
		t.Fatalf("write birthday icon: %v", err)
	}

	want := "asset/jp-assets/ondemand/mysekai/birthday/haruka_2026/icon_refresh.png"
	if got := controller.resolveMysekaiBirthdayRefreshIconPath(renderregion.JP, item, now); got != want {
		t.Fatalf("icon path after asset appears = %q, want %q", got, want)
	}
}

func TestMysekaiBirthdayRefreshIconExpiredEntryIsRefreshed(t *testing.T) {
	root := t.TempDir()
	helper := assets.NewAssetHelper(root, nil)
	controller := NewController(nil, nil, renderregion.JP, helper, MasterdataOptions{})
	now := time.Date(2026, time.July, 15, 0, 0, 0, 0, time.UTC)
	item := map[string]any{"givenNameEnglish": "Haruka"}

	writeIcon := func(year int) {
		t.Helper()
		dir := filepath.Join(root, "jp-assets", assets.RegionAssetOnDemand, "mysekai", "birthday", fmt.Sprintf("haruka_%d", year))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir birthday icon dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "icon_refresh.png"), []byte("png"), 0o644); err != nil {
			t.Fatalf("write birthday icon: %v", err)
		}
	}

	writeIcon(2025)
	wantOld := "asset/jp-assets/ondemand/mysekai/birthday/haruka_2025/icon_refresh.png"
	if got := controller.resolveMysekaiBirthdayRefreshIconPath(renderregion.JP, item, now); got != wantOld {
		t.Fatalf("initial fallback icon path = %q, want %q", got, wantOld)
	}

	writeIcon(2026)
	cacheKey := strings.Join(helper.Roots(), "\x00") + "|jp|haruka|" + strconv.Itoa(now.Year())
	mysekaiBirthdayRefreshIconCache.Store(cacheKey, mysekaiBirthdayRefreshIconCacheEntry{
		path:      wantOld,
		expiresAt: time.Now().Add(-time.Second),
	})

	wantCurrent := "asset/jp-assets/ondemand/mysekai/birthday/haruka_2026/icon_refresh.png"
	if got := controller.resolveMysekaiBirthdayRefreshIconPath(renderregion.JP, item, now); got != wantCurrent {
		t.Fatalf("icon path after cache expiry = %q, want %q", got, wantCurrent)
	}

	cached, ok := mysekaiBirthdayRefreshIconCache.Load(cacheKey)
	if !ok {
		t.Fatal("refreshed icon path was not cached")
	}
	entry, ok := cached.(mysekaiBirthdayRefreshIconCacheEntry)
	if !ok || entry.path != wantCurrent {
		t.Fatalf("unexpected refreshed cache entry: %#v", cached)
	}
	remaining := time.Until(entry.expiresAt)
	if remaining <= 0 || remaining > mysekaiBirthdayRefreshIconCacheTTL {
		t.Fatalf("unexpected cache TTL: %s", remaining)
	}
}
