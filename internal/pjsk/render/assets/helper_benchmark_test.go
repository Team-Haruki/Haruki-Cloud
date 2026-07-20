package assets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
)

func BenchmarkAssetHelperFirstExistingTwentyThousandEntries(b *testing.B) {
	b.StopTimer()
	root := b.TempDir()
	directory := filepath.Join(root, "thumbnail", "chara")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		b.Fatalf("mkdir benchmark directory: %v", err)
	}
	for i := range 20_000 {
		name := filepath.Join(directory, fmt.Sprintf("res%05d.png", i))
		if err := os.WriteFile(name, nil, 0o644); err != nil {
			b.Fatalf("write benchmark asset: %v", err)
		}
	}

	b.Run("exact", func(b *testing.B) {
		fileSystem := &countingAssetFileSystem{delegate: osAssetFileSystem{}}
		helper := NewAssetHelper(root, nil)
		helper.fs = fileSystem
		const candidate = "thumbnail/chara/res19999.png"

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if got := helper.FirstExisting(candidate); got == "" {
				b.Fatal("exact asset did not resolve")
			}
		}
		b.ReportMetric(float64(fileSystem.readDirCalls.Load())/float64(b.N), "readdir/op")
	})

	b.Run("case_fold_cached", func(b *testing.B) {
		const candidate = "THUMBNAIL/CHARA/RES19999.PNG"
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(candidate))); err == nil {
			b.Skip("filesystem is case-insensitive; case-fold fallback cannot be benchmarked")
		}
		fileSystem := &countingAssetFileSystem{delegate: osAssetFileSystem{}}
		helper := NewAssetHelper(root, nil)
		helper.fs = fileSystem
		if got := helper.FirstExisting(candidate); got == "" {
			b.Fatal("case-fold asset did not resolve during cache warmup")
		}
		fileSystem.readDirCalls.Store(0)

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if got := helper.FirstExisting(candidate); got == "" {
				b.Fatal("case-fold asset did not resolve")
			}
		}
		b.ReportMetric(float64(fileSystem.readDirCalls.Load())/float64(b.N), "readdir/op")
	})

	b.Run("missing_cached", func(b *testing.B) {
		fileSystem := &countingAssetFileSystem{delegate: osAssetFileSystem{}}
		helper := NewAssetHelper(root, nil)
		helper.fs = fileSystem
		const candidate = "thumbnail/chara/not_present.png"
		if got := helper.FirstExisting(candidate); got != "" {
			b.Fatalf("missing asset resolved to %q during cache warmup", got)
		}
		fileSystem.readDirCalls.Store(0)

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if got := helper.FirstExisting(candidate); got != "" {
				b.Fatalf("missing asset resolved to %q", got)
			}
		}
		b.ReportMetric(float64(fileSystem.readDirCalls.Load())/float64(b.N), "readdir/op")
	})

	b.Run("cold_unique_exact_722", func(b *testing.B) {
		fileSystem := &countingAssetFileSystem{delegate: osAssetFileSystem{}}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			helper := NewAssetHelper(root, nil)
			helper.fs = fileSystem
			for index := range 722 {
				candidate := fmt.Sprintf("thumbnail/chara/res%05d.png", index)
				if got := helper.FirstExisting(candidate); got == "" {
					b.Fatalf("exact asset %q did not resolve", candidate)
				}
			}
		}
		b.ReportMetric(float64(fileSystem.readDirCalls.Load())/float64(b.N), "readdir/batch")
	})

	b.Run("cold_unique_exact_722_traced", func(b *testing.B) {
		fileSystem := &countingAssetFileSystem{delegate: osAssetFileSystem{}}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			ctx, _ := commandtrace.WithTrace(context.Background())
			helper := NewAssetHelper(root, nil).WithContext(ctx)
			helper.fs = fileSystem
			for index := range 722 {
				candidate := fmt.Sprintf("thumbnail/chara/res%05d.png", index)
				if got := helper.FirstExisting(candidate); got == "" {
					b.Fatalf("exact asset %q did not resolve", candidate)
				}
			}
		}
		b.ReportMetric(float64(fileSystem.readDirCalls.Load())/float64(b.N), "readdir/batch")
	})

	b.Run("cold_unique_missing_722", func(b *testing.B) {
		fileSystem := &countingAssetFileSystem{delegate: osAssetFileSystem{}}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			helper := NewAssetHelper(root, nil)
			helper.fs = fileSystem
			for index := range 722 {
				candidate := fmt.Sprintf("thumbnail/chara/missing%05d.png", index)
				if got := helper.FirstExisting(candidate); got != "" {
					b.Fatalf("missing asset %q resolved to %q", candidate, got)
				}
			}
		}
		b.ReportMetric(float64(fileSystem.readDirCalls.Load())/float64(b.N), "readdir/batch")
	})
}

func BenchmarkAssetResolutionCacheHotKeyUnderChurn(b *testing.B) {
	const capacity = 1_024
	cache := &assetResolutionCache{
		entries:    make(map[string]*assetResolutionEntry, capacity),
		ttl:        time.Hour,
		maxEntries: capacity,
	}
	now := time.Unix(1_000, 0)
	cache.store("hot", "hot.png", now)
	for index := range capacity - 1 {
		cache.store(fmt.Sprintf("warm-%d", index), "warm.png", now)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for index := range b.N {
		if got, ok := cache.lookup("hot", now); !ok || got != "hot.png" {
			b.Fatalf("hot cache entry was evicted: %q, %t", got, ok)
		}
		cache.store(fmt.Sprintf("churn-%d", index), "cold.png", now)
	}
}
