package assets

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
)

func TestAssetHelperFirstExistingFallsBackToLegacyRoots(t *testing.T) {
	tmpDir := t.TempDir()
	primary := filepath.Join(tmpDir, "primary")
	legacy := filepath.Join(tmpDir, "legacy")

	if err := os.MkdirAll(primary, 0o755); err != nil {
		t.Fatalf("mkdir primary: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(legacy, "icons"), 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	target := filepath.Join(legacy, "icons", "card.png")
	if err := os.WriteFile(target, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	helper := NewAssetHelper(primary, []string{legacy})
	got := helper.FirstExisting("icons/card.png")
	if got != filepath.ToSlash(target) {
		t.Fatalf("expected %q, got %q", filepath.ToSlash(target), got)
	}
}

func TestResolveAssetPathFallsBackToPrimaryPath(t *testing.T) {
	helper := NewAssetHelper("/srv/assets", []string{"/srv/assets-legacy"})
	got := ResolveAssetPath(helper, "", "cards/1.webp")
	if want := "/srv/assets/cards/1.webp"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveAssetPathSupportsURLRoots(t *testing.T) {
	helper := NewAssetHelper("https://sekai-assets.haruki.seiunx.com/jp-assets", nil)
	got := ResolveAssetPath(helper, "", "music/jacket/jacket_s_001/jacket_s_001.png")
	want := "https://sekai-assets.haruki.seiunx.com/jp-assets/music/jacket/jacket_s_001/jacket_s_001.png"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestAssetHelperFirstExistingSupportsAbsolutePaths(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "card", "frame_rarity_4.png")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	helper := NewAssetHelper(filepath.Join(tmpDir, "primary"), nil)
	got := helper.FirstExisting(target)
	if got != filepath.ToSlash(target) {
		t.Fatalf("expected %q, got %q", filepath.ToSlash(target), got)
	}
}

func TestAssetHelperFirstExistingSupportsAssetPrefixedPathsAgainstAssetDataRoot(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "jp-assets", "startapp", "honor", "honor_top_000020", "rank_main.png")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	helper := NewAssetHelper(tmpDir, nil)
	got := helper.FirstExisting("asset/jp-assets/startapp/honor/honor_top_000020/rank_main.png")
	if got != filepath.ToSlash(target) {
		t.Fatalf("expected %q, got %q", filepath.ToSlash(target), got)
	}
}

func TestMakeRelativeSupportsURLRoots(t *testing.T) {
	base := "https://sekai-assets.haruki.seiunx.com/jp-assets"
	target := "https://sekai-assets.haruki.seiunx.com/jp-assets/music/jacket/jacket_s_001/jacket_s_001.png"
	got := MakeRelative(base, target)
	want := "music/jacket/jacket_s_001/jacket_s_001.png"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveRegionAssetPathFallsBackToOnDemand(t *testing.T) {
	tmpDir := t.TempDir()
	helper := NewAssetHelper(tmpDir, nil)

	rel := filepath.Join("asset", "jp-assets", "ondemand", "gacha", "ab_gacha_1", "logo", "logo.png")
	full := filepath.Join(tmpDir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got := ResolveRegionAssetPath(helper, "jp", filepath.Join("gacha", "ab_gacha_1", "logo", "logo.png"))
	if want := filepath.ToSlash(rel); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveRegionAssetPathHandlesCaseMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	helper := NewAssetHelper(tmpDir, nil)

	full := filepath.Join(
		tmpDir,
		"asset",
		"jp-assets",
		"startapp",
		"music",
		"jacket",
		"Jacket_S_001",
		"Jacket_S_001.png",
	)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got := ResolveRegionAssetPath(helper, "jp", filepath.Join("music", "jacket", "jacket_s_001", "jacket_s_001.png"))
	if want := filepath.ToSlash(filepath.Join("asset", "jp-assets", "startapp", "music", "jacket", "jacket_s_001", "jacket_s_001.png")); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveRegionAssetPathPrefersStartAppOverOnDemand(t *testing.T) {
	tmpDir := t.TempDir()
	helper := NewAssetHelper(tmpDir, nil)
	rel := filepath.Join("honor", "honor_6881", "degree_sub.png")

	startApp := filepath.Join(tmpDir, "asset", "jp-assets", "startapp", rel)
	if err := os.MkdirAll(filepath.Dir(startApp), 0o755); err != nil {
		t.Fatalf("mkdir startapp: %v", err)
	}
	if err := os.WriteFile(startApp, []byte("startapp"), 0o644); err != nil {
		t.Fatalf("write startapp file: %v", err)
	}

	onDemand := filepath.Join(tmpDir, "asset", "jp-assets", "ondemand", rel)
	if err := os.MkdirAll(filepath.Dir(onDemand), 0o755); err != nil {
		t.Fatalf("mkdir ondemand: %v", err)
	}
	if err := os.WriteFile(onDemand, []byte("ondemand"), 0o644); err != nil {
		t.Fatalf("write ondemand file: %v", err)
	}

	got := ResolveRegionAssetPath(helper, "jp", rel)
	if want := filepath.ToSlash(filepath.Join("asset", "jp-assets", "startapp", rel)); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveRegionAssetPathPrefersOnDemandForGacha(t *testing.T) {
	tmpDir := t.TempDir()
	helper := NewAssetHelper(tmpDir, nil)
	rel := filepath.Join("gacha", "ab_gacha_1", "logo", "logo.png")

	startApp := filepath.Join(tmpDir, "asset", "jp-assets", "startapp", rel)
	if err := os.MkdirAll(filepath.Dir(startApp), 0o755); err != nil {
		t.Fatalf("mkdir startapp: %v", err)
	}
	if err := os.WriteFile(startApp, []byte("startapp"), 0o644); err != nil {
		t.Fatalf("write startapp file: %v", err)
	}

	onDemand := filepath.Join(tmpDir, "asset", "jp-assets", "ondemand", rel)
	if err := os.MkdirAll(filepath.Dir(onDemand), 0o755); err != nil {
		t.Fatalf("mkdir ondemand: %v", err)
	}
	if err := os.WriteFile(onDemand, []byte("ondemand"), 0o644); err != nil {
		t.Fatalf("write ondemand file: %v", err)
	}

	got := ResolveRegionAssetPath(helper, "jp", rel)
	if want := filepath.ToSlash(filepath.Join("asset", "jp-assets", "ondemand", rel)); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveRegionAssetPathPrefersOnDemandForVirtualLive(t *testing.T) {
	tmpDir := t.TempDir()
	helper := NewAssetHelper(tmpDir, nil)
	rel := filepath.Join("virtual_live", "select", "banner", "vlentrance_00371", "vlentrance_00371.png")

	startApp := filepath.Join(tmpDir, "asset", "jp-assets", "startapp", rel)
	if err := os.MkdirAll(filepath.Dir(startApp), 0o755); err != nil {
		t.Fatalf("mkdir startapp: %v", err)
	}
	if err := os.WriteFile(startApp, []byte("startapp"), 0o644); err != nil {
		t.Fatalf("write startapp file: %v", err)
	}

	onDemand := filepath.Join(tmpDir, "asset", "jp-assets", "ondemand", rel)
	if err := os.MkdirAll(filepath.Dir(onDemand), 0o755); err != nil {
		t.Fatalf("mkdir ondemand: %v", err)
	}
	if err := os.WriteFile(onDemand, []byte("ondemand"), 0o644); err != nil {
		t.Fatalf("write ondemand file: %v", err)
	}

	got := ResolveRegionAssetPath(helper, "jp", rel)
	if want := filepath.ToSlash(filepath.Join("asset", "jp-assets", "ondemand", rel)); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveEventBannerPathFallsBackToEventStoryBanner(t *testing.T) {
	tmpDir := t.TempDir()
	helper := NewAssetHelper(tmpDir, nil)
	rel := filepath.Join("event_story", "event_angelclover_2021", "screen_image", "banner_event_story.png")

	onDemand := filepath.Join(tmpDir, "asset", "jp-assets", "ondemand", rel)
	if err := os.MkdirAll(filepath.Dir(onDemand), 0o755); err != nil {
		t.Fatalf("mkdir ondemand: %v", err)
	}
	if err := os.WriteFile(onDemand, []byte("ondemand"), 0o644); err != nil {
		t.Fatalf("write ondemand file: %v", err)
	}

	got := ResolveEventBannerPath(helper, "jp", "event_angelclover_2021")
	if want := filepath.ToSlash(filepath.Join("asset", "jp-assets", "ondemand", rel)); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveRegionAssetPathFallsBackToRelativePathWhenHelperMisses(t *testing.T) {
	helper := NewAssetHelper("/srv/haruki-assets", nil)
	rel := filepath.Join("thumbnail", "chara", "res001_no001_normal.png")

	got := ResolveRegionAssetPath(helper, "jp", rel)
	want := filepath.ToSlash(filepath.Join("asset", "jp-assets", "startapp", rel))
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveRegionAssetPathPrefersStartAppForBondsHonor(t *testing.T) {
	tmpDir := t.TempDir()
	helper := NewAssetHelper(tmpDir, nil)
	rel := filepath.Join("bonds_honor", "character", "chr_sd_11_01.png")

	startApp := filepath.Join(tmpDir, "asset", "jp-assets", "startapp", rel)
	if err := os.MkdirAll(filepath.Dir(startApp), 0o755); err != nil {
		t.Fatalf("mkdir startapp: %v", err)
	}
	if err := os.WriteFile(startApp, []byte("startapp"), 0o644); err != nil {
		t.Fatalf("write startapp file: %v", err)
	}

	onDemand := filepath.Join(tmpDir, "asset", "jp-assets", "ondemand", rel)
	if err := os.MkdirAll(filepath.Dir(onDemand), 0o755); err != nil {
		t.Fatalf("mkdir ondemand: %v", err)
	}
	if err := os.WriteFile(onDemand, []byte("ondemand"), 0o644); err != nil {
		t.Fatalf("write ondemand file: %v", err)
	}

	got := ResolveRegionAssetPath(helper, "jp", rel)
	if want := filepath.ToSlash(filepath.Join("asset", "jp-assets", "startapp", rel)); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

// These two pin the original stat-first performance invariant: an exact path must
// not enter the case-insensitive walk, while a miscased path must still resolve.
// Multi-root exact-before-casefold precedence is covered separately below.
//
// The walk is what made a full card box cost ~25s: it os.ReadDir's every parent component, one of
// which holds ~21k thumbnails, and it ran even when the exact path was sitting right there.

func TestAssetHelperFirstExistingResolvesExactPathWithoutTheCaseInsensitiveWalk(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "thumbnail", "chara"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(root, "thumbnail", "chara", "res001_no001_normal.png")
	if err := os.WriteFile(target, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	fileSystem := &countingAssetFileSystem{delegate: osAssetFileSystem{}}
	ctx, trace := commandtrace.WithTrace(context.Background())
	helper := NewAssetHelper(root, nil).WithContext(ctx)
	helper.fs = fileSystem
	got := helper.FirstExisting("thumbnail/chara/res001_no001_normal.png")
	if got != filepath.ToSlash(target) {
		t.Fatalf("expected %q, got %q", filepath.ToSlash(target), got)
	}
	if got := fileSystem.readDirCalls.Load(); got != 0 {
		t.Fatalf("exact path performed %d ReadDir calls, want 0", got)
	}
	operations := operationStatsByName(trace.Snapshot())
	if got := operations["asset.stat"].Count; got != 1 {
		t.Fatalf("asset.stat count = %d, want 1", got)
	}
	if _, ok := operations["asset.case_walk"]; ok {
		t.Fatalf("exact path unexpectedly recorded asset.case_walk: %+v", operations)
	}
	if _, ok := operations["asset.readdir"]; ok {
		t.Fatalf("exact path unexpectedly recorded asset.readdir: %+v", operations)
	}
}

func TestAssetHelperFirstExistingStillResolvesAMiscasedPath(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "icons"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(root, "icons", "card.png")
	if err := os.WriteFile(target, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	// On a case-insensitive filesystem (the default on macOS) the stat fast path answers this
	// itself, so the fallback it is meant to exercise never runs and the assertion is vacuous.
	// Production is ext4, where the stat genuinely misses; skip rather than pretend to cover it.
	if _, err := os.Stat(filepath.Join(root, "ICONS", "CARD.PNG")); err == nil {
		t.Skip("filesystem is case-insensitive; the case-insensitive walk cannot be exercised here")
	}

	helper := NewAssetHelper(root, nil)
	got := helper.FirstExisting("ICONS/CARD.PNG")
	if got != filepath.ToSlash(target) {
		t.Fatalf("miscased path must still fall back to the on-disk name: expected %q, got %q",
			filepath.ToSlash(target), got)
	}
}

func TestAssetHelperFirstExistingPrefersLegacyExactOverPrimaryCaseFold(t *testing.T) {
	fileSystem := newMemoryAssetFileSystem(map[string]*fstest.MapFile{
		"primary":                directoryFile(time.Unix(10, 0)),
		"primary/icons":          directoryFile(time.Unix(11, 0)),
		"primary/icons/Card.PNG": regularFile(),
		"legacy":                 directoryFile(time.Unix(20, 0)),
		"legacy/icons":           directoryFile(time.Unix(21, 0)),
		"legacy/icons/card.png":  regularFile(),
	})
	helper := NewAssetHelper("primary", []string{"legacy"})
	helper.fs = fileSystem

	got := helper.FirstExisting("icons/card.png")
	if want := "legacy/icons/card.png"; got != want {
		t.Fatalf("legacy exact path must beat primary case-fold match: got %q, want %q", got, want)
	}
	if got := fileSystem.readDirCalls.Load(); got != 0 {
		t.Fatalf("exact match in second root performed %d ReadDir calls, want 0", got)
	}
}

func TestAssetHelperFirstExistingCachesCaseInsensitiveMiss(t *testing.T) {
	fileSystem := newMemoryAssetFileSystem(map[string]*fstest.MapFile{
		"root":             directoryFile(time.Unix(10, 0)),
		"root/icons":       directoryFile(time.Unix(11, 0)),
		"root/icons/a.png": regularFile(),
	})
	helper := NewAssetHelper("root", nil)
	helper.fs = fileSystem

	for range 2 {
		if got := helper.FirstExisting("icons/MISSING.PNG"); got != "" {
			t.Fatalf("missing path resolved to %q", got)
		}
	}
	if got := fileSystem.readDirCalls.Load(); got != 1 {
		t.Fatalf("repeated miss performed %d ReadDir calls, want 1", got)
	}
}

func TestAssetHelperDirectoryCacheInvalidatesWhenModTimeChanges(t *testing.T) {
	initialModTime := time.Unix(100, 0)
	fileSystem := newMemoryAssetFileSystem(map[string]*fstest.MapFile{
		"root":                directoryFile(time.Unix(90, 0)),
		"root/icons":          directoryFile(initialModTime),
		"root/icons/Card.PNG": regularFile(),
	})
	helper := NewAssetHelper("root", nil)
	helper.fs = fileSystem

	if got, want := helper.FirstExisting("icons/CARD.png"), "root/icons/Card.PNG"; got != want {
		t.Fatalf("first resolution = %q, want %q", got, want)
	}
	fileSystem.replaceFile(
		"root/icons/Card.PNG",
		"root/icons/cArD.png",
		"root/icons",
		initialModTime.Add(time.Second),
	)
	helper.ClearResolutionCache()
	if got, want := helper.FirstExisting("icons/CARD.png"), "root/icons/cArD.png"; got != want {
		t.Fatalf("resolution after directory change = %q, want %q", got, want)
	}
	if got := fileSystem.readDirCalls.Load(); got != 2 {
		t.Fatalf("cache invalidation performed %d ReadDir calls, want 2", got)
	}
}

func TestAssetHelperWithContextSharesDirectoryCacheAndRecordsAggregates(t *testing.T) {
	fileSystem := newMemoryAssetFileSystem(map[string]*fstest.MapFile{
		"root":                directoryFile(time.Unix(10, 0)),
		"root/icons":          directoryFile(time.Unix(11, 0)),
		"root/icons/Card.PNG": regularFile(),
	})
	base := NewAssetHelper("root", nil)
	base.fs = fileSystem

	firstCtx, firstTrace := commandtrace.WithTrace(context.Background())
	if got := base.WithContext(firstCtx).FirstExisting("icons/CARD.png"); got != "root/icons/Card.PNG" {
		t.Fatalf("first resolution = %q", got)
	}
	firstOperations := operationStatsByName(firstTrace.Snapshot())
	for _, name := range []string{"asset.stat", "asset.case_walk", "asset.readdir"} {
		if firstOperations[name].Count == 0 {
			t.Fatalf("first trace did not record %s: %+v", name, firstOperations)
		}
	}

	secondCtx, secondTrace := commandtrace.WithTrace(context.Background())
	if got := base.WithContext(secondCtx).FirstExisting("icons/CARD.png"); got != "root/icons/Card.PNG" {
		t.Fatalf("second resolution = %q", got)
	}
	secondOperations := operationStatsByName(secondTrace.Snapshot())
	if secondOperations["asset.resolve_cache_hit"].Count != 1 {
		t.Fatalf("second trace is missing the resolution cache hit: %+v", secondOperations)
	}
	for _, unexpected := range []string{"asset.stat", "asset.case_walk", "asset.readdir"} {
		if _, ok := secondOperations[unexpected]; ok {
			t.Fatalf("resolution cache hit unexpectedly recorded %s: %+v", unexpected, secondOperations)
		}
	}
}

func TestAssetHelperResolutionCacheStoresMissAndCanBeCleared(t *testing.T) {
	fileSystem := newMemoryAssetFileSystem(map[string]*fstest.MapFile{
		"root":       directoryFile(time.Unix(10, 0)),
		"root/icons": directoryFile(time.Unix(11, 0)),
	})
	helper := NewAssetHelper("root", nil)
	helper.fs = fileSystem

	if got := helper.FirstExisting("icons/new.png"); got != "" {
		t.Fatalf("initial miss = %q", got)
	}
	fileSystem.mu.Lock()
	fileSystem.files["root/icons/new.png"] = regularFile()
	fileSystem.files["root/icons"].ModTime = time.Unix(12, 0)
	fileSystem.mu.Unlock()
	if got := helper.FirstExisting("icons/new.png"); got != "" {
		t.Fatalf("cached miss unexpectedly refreshed before invalidation: %q", got)
	}

	helper.ClearResolutionCache()
	if got := helper.FirstExisting("icons/new.png"); got != "root/icons/new.png" {
		t.Fatalf("resolution after clear = %q", got)
	}
}

func TestAssetHelperDirectoryCacheIsConcurrentSafe(t *testing.T) {
	fileSystem := newMemoryAssetFileSystem(map[string]*fstest.MapFile{
		"root":                directoryFile(time.Unix(10, 0)),
		"root/icons":          directoryFile(time.Unix(11, 0)),
		"root/icons/Card.PNG": regularFile(),
	})
	fileSystem.readDelay = 10 * time.Millisecond
	helper := NewAssetHelper("root", nil)
	helper.fs = fileSystem

	const workers = 64
	start := make(chan struct{})
	errors := make(chan string, workers)
	contexts := make([]context.Context, workers)
	traces := make([]*commandtrace.Trace, workers)
	for i := range workers {
		contexts[i], traces[i] = commandtrace.WithTrace(context.Background())
	}
	var wg sync.WaitGroup
	for index := range workers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			if got := helper.WithContext(contexts[index]).FirstExisting("icons/CARD.png"); got != "root/icons/Card.PNG" {
				errors <- got
			}
		}(index)
	}
	close(start)
	wg.Wait()
	close(errors)
	for got := range errors {
		t.Errorf("concurrent resolution = %q, want %q", got, "root/icons/Card.PNG")
	}
	if got := fileSystem.readDirCalls.Load(); got != 1 {
		t.Fatalf("concurrent cache fill performed %d ReadDir calls, want 1", got)
	}
	for index, trace := range traces {
		operations := operationStatsByName(trace.Snapshot())
		for _, name := range []string{"asset.stat", "asset.case_walk", "asset.readdir"} {
			if operations[name].Count == 0 {
				t.Fatalf("trace[%d] did not record shared %s: %+v", index, name, operations)
			}
		}
	}
}

func TestAssetHelperDirectorySingleflightMergesIndependentTraceIntoEveryWaiter(t *testing.T) {
	readStarted := make(chan struct{})
	releaseRead := make(chan struct{})
	fileSystem := newMemoryAssetFileSystem(map[string]*fstest.MapFile{
		"root":                directoryFile(time.Unix(10, 0)),
		"root/icons":          directoryFile(time.Unix(11, 0)),
		"root/icons/Card.PNG": regularFile(),
	})
	fileSystem.readStarted = readStarted
	fileSystem.releaseRead = releaseRead
	base := NewAssetHelper("root", nil)
	base.fs = fileSystem

	type directoryResult struct {
		index *assetDirectoryIndex
		ok    bool
	}
	firstCtx, firstTrace := commandtrace.WithTrace(context.Background())
	secondCtx, secondTrace := commandtrace.WithTrace(context.Background())
	firstResult := make(chan directoryResult, 1)
	secondResult := make(chan directoryResult, 1)
	go func() {
		index, ok := base.WithContext(firstCtx).directoryIndex("root/icons")
		firstResult <- directoryResult{index: index, ok: ok}
	}()
	<-readStarted
	go func() {
		index, ok := base.WithContext(secondCtx).directoryIndex("root/icons")
		secondResult <- directoryResult{index: index, ok: ok}
	}()

	deadline := time.Now().Add(time.Second)
	for operationStatsByName(secondTrace.Snapshot())["asset.stat"].Count == 0 {
		if time.Now().After(deadline) {
			t.Fatal("second caller did not reach the directory load")
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(10 * time.Millisecond)
	close(releaseRead)

	for index, result := range []directoryResult{<-firstResult, <-secondResult} {
		if !result.ok || result.index == nil {
			t.Fatalf("result[%d] did not resolve the directory", index)
		}
	}
	if got := fileSystem.readDirCalls.Load(); got != 1 {
		t.Fatalf("shared directory fill performed %d ReadDir calls, want 1", got)
	}
	for index, trace := range []*commandtrace.Trace{firstTrace, secondTrace} {
		operations := operationStatsByName(trace.Snapshot())
		for _, name := range []string{"asset.directory_wait", "asset.directory_shared", "asset.readdir"} {
			if operations[name].Count == 0 {
				t.Fatalf("trace[%d] did not receive shared %s: %+v", index, name, operations)
			}
		}
	}
}

func TestAssetResolutionCacheEvictsLeastRecentlyUsedAndKeepsHotEntry(t *testing.T) {
	cache := &assetResolutionCache{
		entries:    make(map[string]*assetResolutionEntry),
		ttl:        time.Hour,
		maxEntries: 3,
	}
	now := time.Unix(1_000, 0)
	cache.store("hot", "hot.png", now)
	cache.store("cold-1", "cold-1.png", now)
	cache.store("cold-2", "cold-2.png", now)

	if got, ok := cache.lookup("hot", now); !ok || got != "hot.png" {
		t.Fatalf("hot lookup = %q, %t", got, ok)
	}
	cache.store("new-1", "new-1.png", now)
	if _, ok := cache.lookup("cold-1", now); ok {
		t.Fatal("least-recently-used entry was not evicted")
	}
	if got, ok := cache.lookup("hot", now); !ok || got != "hot.png" {
		t.Fatalf("hot entry was lost after first eviction: %q, %t", got, ok)
	}

	cache.store("new-2", "new-2.png", now)
	if got, ok := cache.lookup("hot", now); !ok || got != "hot.png" {
		t.Fatalf("hot entry was lost under churn: %q, %t", got, ok)
	}
	cache.mu.Lock()
	entryCount := len(cache.entries)
	cache.mu.Unlock()
	if entryCount != 3 {
		t.Fatalf("cache contains %d entries, want 3", entryCount)
	}
}

func TestAssetResolutionCacheRemovesExpiredEntry(t *testing.T) {
	cache := &assetResolutionCache{
		entries:    make(map[string]*assetResolutionEntry),
		ttl:        time.Second,
		maxEntries: 2,
	}
	now := time.Unix(1_000, 0)
	cache.store("expired", "old.png", now)
	if _, ok := cache.lookup("expired", now.Add(time.Second)); ok {
		t.Fatal("expired resolution remained visible")
	}
	cache.mu.Lock()
	entryCount := len(cache.entries)
	recentCount := cache.recent.Len()
	cache.mu.Unlock()
	if entryCount != 0 || recentCount != 0 {
		t.Fatalf("expired resolution was not removed: entries=%d recent=%d", entryCount, recentCount)
	}
}

func TestAssetResolutionCacheClearRejectsInFlightOldGeneration(t *testing.T) {
	cache := &assetResolutionCache{
		entries:    make(map[string]*assetResolutionEntry),
		ttl:        time.Hour,
		maxEntries: 2,
	}
	now := time.Unix(1_000, 0)
	oldGeneration := cache.currentGeneration()
	cache.clear()
	if cache.storeForGeneration("stale", "stale.png", now, oldGeneration) {
		t.Fatal("store from the generation before ClearResolutionCache succeeded")
	}
	if _, ok := cache.lookup("stale", now); ok {
		t.Fatal("old in-flight result repopulated the cleared cache")
	}
	if !cache.storeForGeneration("fresh", "fresh.png", now, cache.currentGeneration()) {
		t.Fatal("store from the current generation was rejected")
	}
	if got, ok := cache.lookup("fresh", now); !ok || got != "fresh.png" {
		t.Fatalf("fresh result = %q, %t", got, ok)
	}
}

func TestAssetDirectoryCacheEvictsLeastRecentlyUsedWithinBothBounds(t *testing.T) {
	modTime := time.Unix(1_000, 0)
	cache := &assetDirectoryCache{
		entries:    make(map[string]*assetDirectoryCacheEntry),
		maxEntries: 2,
		maxNames:   4,
	}
	cache.store("a", testAssetDirectoryIndex(modTime, "a.png"))
	cache.store("b", testAssetDirectoryIndex(modTime, "b.png"))
	if _, ok := cache.lookup("a", modTime); !ok {
		t.Fatal("failed to touch hot directory entry")
	}
	cache.store("c", testAssetDirectoryIndex(modTime, "c.png"))

	if _, ok := cache.lookup("b", modTime); ok {
		t.Fatal("least-recently-used directory was not evicted")
	}
	for _, parent := range []string{"a", "c"} {
		if _, ok := cache.lookup(parent, modTime); !ok {
			t.Fatalf("directory %q was unexpectedly evicted", parent)
		}
	}

	cache.mu.Lock()
	cache.maxNames = 3
	cache.mu.Unlock()
	cache.store("weighted", testAssetDirectoryIndex(modTime, "1", "2", "3"))
	cache.mu.Lock()
	entryCount := len(cache.entries)
	indexed := cache.indexed
	cache.mu.Unlock()
	if entryCount != 1 || indexed != 3 {
		t.Fatalf("weighted eviction left entries=%d indexed=%d, want 1 and 3", entryCount, indexed)
	}

	cache.store("oversized", testAssetDirectoryIndex(modTime, "1", "2", "3", "4", "5"))
	if _, ok := cache.lookup("oversized", modTime); ok {
		t.Fatal("directory larger than the entire name budget was cached")
	}
	if _, ok := cache.lookup("weighted", modTime); !ok {
		t.Fatal("oversized insertion displaced the existing hot directory")
	}
}

func TestAssetDirectoryCacheDropsStaleIndexBeforeReload(t *testing.T) {
	cache := &assetDirectoryCache{
		entries:    make(map[string]*assetDirectoryCacheEntry),
		maxEntries: 2,
		maxNames:   4,
	}
	oldModTime := time.Unix(1_000, 0)
	cache.store("icons", testAssetDirectoryIndex(oldModTime, "old.png"))
	if _, ok := cache.lookup("icons", oldModTime.Add(time.Second)); ok {
		t.Fatal("stale directory index remained visible")
	}
	cache.mu.Lock()
	entryCount := len(cache.entries)
	indexed := cache.indexed
	recentCount := cache.recent.Len()
	cache.mu.Unlock()
	if entryCount != 0 || indexed != 0 || recentCount != 0 {
		t.Fatalf("stale index was not fully removed: entries=%d indexed=%d recent=%d", entryCount, indexed, recentCount)
	}
}

func testAssetDirectoryIndex(modTime time.Time, names ...string) *assetDirectoryIndex {
	index := &assetDirectoryIndex{
		modTime: modTime,
		exact:   make(map[string]string, len(names)),
		folded:  make(map[string]string, len(names)),
	}
	for _, name := range names {
		index.exact[name] = name
		index.folded[foldAssetName(name)] = name
	}
	return index
}

type countingAssetFileSystem struct {
	delegate     assetFileSystem
	readDirCalls atomic.Int64
}

func (f *countingAssetFileSystem) Stat(name string) (fs.FileInfo, error) {
	return f.delegate.Stat(name)
}

func (f *countingAssetFileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	f.readDirCalls.Add(1)
	return f.delegate.ReadDir(name)
}

type memoryAssetFileSystem struct {
	mu           sync.RWMutex
	files        fstest.MapFS
	readDirCalls atomic.Int64
	readDelay    time.Duration
	readStarted  chan struct{}
	releaseRead  <-chan struct{}
	readOnce     sync.Once
}

func newMemoryAssetFileSystem(files map[string]*fstest.MapFile) *memoryAssetFileSystem {
	return &memoryAssetFileSystem{files: fstest.MapFS(files)}
}

func (f *memoryAssetFileSystem) Stat(name string) (fs.FileInfo, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.files.Stat(memoryAssetPath(name))
}

func (f *memoryAssetFileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	f.readDirCalls.Add(1)
	if f.readStarted != nil {
		f.readOnce.Do(func() { close(f.readStarted) })
	}
	if f.releaseRead != nil {
		<-f.releaseRead
	}
	if f.readDelay > 0 {
		time.Sleep(f.readDelay)
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.files.ReadDir(memoryAssetPath(name))
}

func (f *memoryAssetFileSystem) replaceFile(oldName, newName, parent string, modTime time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, memoryAssetPath(oldName))
	f.files[memoryAssetPath(newName)] = regularFile()
	f.files[memoryAssetPath(parent)].ModTime = modTime
}

func memoryAssetPath(name string) string {
	clean := filepath.ToSlash(filepath.Clean(name))
	return strings.TrimPrefix(clean, "./")
}

func directoryFile(modTime time.Time) *fstest.MapFile {
	return &fstest.MapFile{Mode: fs.ModeDir | 0o755, ModTime: modTime}
}

func regularFile() *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("ok"), Mode: 0o644}
}

func operationStatsByName(snapshot commandtrace.Snapshot) map[string]commandtrace.Stats {
	result := make(map[string]commandtrace.Stats, len(snapshot.Operations))
	for _, operation := range snapshot.Operations {
		result[operation.Name] = operation
	}
	return result
}
