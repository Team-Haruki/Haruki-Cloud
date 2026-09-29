package assets

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func requireAssetStage(t *testing.T, trace *commandtrace.Trace, name string, count int) {
	t.Helper()
	stat := operationStatsByName(trace.Snapshot())[name]
	if stat.Count != count || (count > 0 && stat.Total <= 0) {
		t.Fatalf("%s=%+v, want count %d with elapsed time", name, stat, count)
	}
}

func TestAssetReaderStoreStagesCountActualIO(t *testing.T) {
	store := storagetest.NewMemory()
	store.Seed(map[string][]byte{"present.png": []byte("image")})
	reader := NewAssetReader(nil, store)
	ctx, trace := commandtrace.WithTrace(t.Context())
	data, _, err := reader.ReadFirst(ctx, "missing.png", "present.png")
	if err != nil || string(data) != "image" {
		t.Fatalf("read=%q err=%v", data, err)
	}
	requireAssetStage(t, trace, "asset.store_get", 2)
	if _, found := reader.Stat(ctx, "present.png"); !found {
		t.Fatal("read should populate existence memo")
	}
	requireAssetStage(t, trace, "asset.store_stat", 0)
	if _, found := reader.Stat(ctx, "missing.png"); found {
		t.Fatal("missing asset exists")
	}
	requireAssetStage(t, trace, "asset.store_stat", 1)
	if _, found := reader.Stat(ctx, "missing.png"); found {
		t.Fatal("memoized missing asset exists")
	}
	requireAssetStage(t, trace, "asset.store_stat", 1)
}

func TestAssetReaderStoreStagesIncludeErrorsAndCancellation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "backend error", true: "canceled"}[canceled], func(t *testing.T) {
			store := storagetest.NewMemory()
			boom := errors.New("store unavailable")
			store.FailGet = func(storage.Key) error { return boom }
			store.FailStat = func(storage.Key) error { return boom }
			reader := NewAssetReader(nil, store)
			ctx, trace := commandtrace.WithTrace(t.Context())
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			wantErr := boom
			if canceled {
				cancel()
				wantErr = context.Canceled
			}
			if _, _, err := reader.ReadFirst(ctx, "asset.png"); !errors.Is(err, wantErr) {
				t.Fatalf("read error=%v", err)
			}
			requireAssetStage(t, trace, "asset.store_get", 1)
			if _, found := reader.Stat(ctx, "asset.png"); found {
				t.Fatal("failed stat reported found")
			}
			requireAssetStage(t, trace, "asset.store_stat", 1)
		})
	}
}

func TestAssetReaderLocalStagesIncludeErrors(t *testing.T) {
	root := writeAssetTree(t, map[string]string{"nested/present.png": "image"})
	helper := NewAssetHelper(root, nil)
	// Warm resolution so the new direct Stat timer is distinguishable from
	// the helper's existing path-probe timers.
	for _, path := range []string{"nested/present.png", "nested"} {
		if helper.FirstExisting(path) == "" {
			t.Fatal("fixture not resolved")
		}
	}
	reader := NewAssetReader(helper, nil)
	ctx, trace := commandtrace.WithTrace(t.Context())
	if data, _, err := reader.ReadFirst(ctx, "nested/present.png"); err != nil || string(data) != "image" {
		t.Fatalf("read=%q err=%v", data, err)
	}
	requireAssetStage(t, trace, "asset.read_file", 1)
	if _, _, err := reader.ReadFirst(ctx, "nested"); err == nil {
		t.Fatal("reading a directory should fail")
	}
	requireAssetStage(t, trace, "asset.read_file", 2)
	if _, found := reader.Stat(ctx, "nested/present.png"); !found {
		t.Fatal("fixture stat failed")
	}
	requireAssetStage(t, trace, "asset.stat", 1)
	if err := os.Remove(filepath.Join(root, "nested/present.png")); err != nil {
		t.Fatal(err)
	}
	if _, found := reader.Stat(ctx, "nested/present.png"); found {
		t.Fatal("removed file exists")
	}
	requireAssetStage(t, trace, "asset.stat", 2)
}

func TestStoreDirectoryWaitStageSkipsWarmDirectory(t *testing.T) {
	for _, fails := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "error"}[fails], func(t *testing.T) {
			store := storagetest.NewMemory()
			if fails {
				store.FailListDir = func(storage.Key) error { return errors.New("listing unavailable") }
			}
			helper := storeOnlyHelper(t, store, StoreProbeConfig{})
			ctx, trace := commandtrace.WithTrace(t.Context())
			if _, err := helper.store.dirIndex(ctx, "", time.Time{}); err != nil {
				t.Fatal(err)
			}
			requireAssetStage(t, trace, "asset.store_directory_wait", 1)
			requireAssetStage(t, trace, "asset.store_list", 1)
			if _, err := helper.store.dirIndex(ctx, "", time.Time{}); err != nil {
				t.Fatal(err)
			}
			requireAssetStage(t, trace, "asset.store_directory_wait", 1)
			requireAssetStage(t, trace, "asset.store_list", 1)
		})
	}
}

func TestStoreDirectoryWaitDoesNotLeakIntoCanceledCaller(t *testing.T) {
	store := storagetest.NewMemory()
	store.Seed(map[string][]byte{"present.png": []byte("image")})
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	store.FailListDir = func(storage.Key) error { once.Do(func() { close(entered) }); <-release; return nil }
	helper := storeOnlyHelper(t, store, StoreProbeConfig{})
	ctx, trace := commandtrace.WithTrace(t.Context())
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, _, err := helper.store.resolve(ctx, "present.png"); done <- err }()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("resolve error=%v", err)
	}
	requireAssetStage(t, trace, "asset.store_probe_wait", 1)
	requireAssetStage(t, trace, "asset.store_directory_wait", 0)
	close(release)
	// Either join the still-running key flight or see its completed cache entry.
	if _, found, err := helper.store.resolve(t.Context(), "present.png"); err != nil || !found {
		t.Fatalf("completed shared lookup: found=%v err=%v", found, err)
	}
	requireAssetStage(t, trace, "asset.store_directory_wait", 0)
	requireAssetStage(t, trace, "asset.store_list", 0)
}
