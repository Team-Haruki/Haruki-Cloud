package assetindex

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func benchmarkManifest(b *testing.B, count int) *storagetest.Memory {
	b.Helper()
	store := storagetest.NewMemory()
	domains := []string{"card", "background", "event", "home", "honor", "music", "mysekai", "thumbnail", "virtual_live", "stamp"}
	manifest := Manifest{Version: Version, Region: "jp", Complete: true}
	objects := make([][]Object, len(domains))
	modified := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	for i := range count {
		domain := i % len(domains)
		key := storage.Key(fmt.Sprintf("jp-assets/startapp/%s/bundle_%06d/image_%02d.png", domains[domain], i/40, i/10%4))
		objects[domain] = append(objects[domain], Object{Key: key, Size: 4096, ETag: "0123456789abcdef0123456789abcdef", Modified: modified})
	}
	for i, domain := range domains {
		prefix := "jp-assets/startapp/" + domain + "/"
		data, err := json.Marshal(Shard{Version: Version, Region: "jp", Prefix: prefix, Objects: objects[i]})
		if err != nil {
			b.Fatal(err)
		}
		hash := digest(data)
		key := storage.Key(Root + "jp/shards/" + hash + ".json")
		store.Seed(map[string][]byte{string(key): data})
		manifest.Shards = append(manifest.Shards, ShardRef{Prefix: prefix, Blob: Blob{Key: key, SHA256: hash}})
	}
	manifest.Revision = manifestRevision(manifest.Shards)
	data, err := json.Marshal(manifest)
	if err != nil {
		b.Fatal(err)
	}
	store.Seed(map[string][]byte{string(PointerKey("jp")): data})
	return store
}

// Retained heap excludes immutable shard JSON already owned by the test store.
// The timed loop includes pointer and shard decoding, validation, and indexing,
// but its in-memory transport intentionally excludes production network delay.
func BenchmarkManagerRefresh100K(b *testing.B) {
	store := benchmarkManifest(b, 100_000)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	manager := New(store, Config{Enabled: true}, nil)
	if err := manager.Refresh(context.Background()); err != nil {
		b.Fatal(err)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	retained := float64(int64(after.HeapAlloc)-int64(before.HeapAlloc)) / (1024 * 1024)
	runtime.KeepAlive(manager)
	manager.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		manager = New(store, Config{Enabled: true}, nil)
		if err := manager.Refresh(context.Background()); err != nil {
			b.Fatal(err)
		}
		manager.Close()
	}
	b.ReportMetric(retained, "retained-MiB")
}
