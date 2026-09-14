package drawing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPendingRenderReusedAndIsolated(t *testing.T) {
	client := newIndexClient(t, &fakeRenderIndex{})
	var renders atomic.Int32
	render := func(context.Context) ([]byte, error) { renders.Add(1); return []byte("original image"), nil }
	policy := renderCachePolicy{APIPath: "api/pjsk/profile", UserID: "public", TTL: time.Hour}
	key := strings.Repeat("a", 64)
	first, err := client.renderRemoteFlight(t.Context(), "/api/pjsk/profile", key, policy, render)
	if err != nil {
		t.Fatal(err)
	}
	first[0] = 'X'
	for range 10 {
		got, err := client.renderRemoteFlight(t.Context(), "/api/pjsk/profile", key, policy, render)
		if err != nil || string(got) != "original image" {
			t.Fatalf("result=%q err=%v", got, err)
		}
	}
	if renders.Load() != 1 {
		t.Fatalf("renders=%d want 1", renders.Load())
	}
}

func TestPendingRenderLimitsExpiryAndGeneration(t *testing.T) {
	cache := newLocalRenderCacheWithLimits(time.Second, 2, 12)
	cache.set("a", []byte("aaaaaa"), time.Second, false)
	old := cache.peekGeneration("a")
	cache.set("a", []byte("newaaa"), time.Second, false)
	cache.deleteGeneration("a", old)
	if got, _ := cache.get("a"); string(got) != "newaaa" {
		t.Fatalf("old store removed new generation: %q", got)
	}
	cache.set("b", []byte("bbbbbb"), time.Second, false)
	cache.set("c", []byte("cccccc"), time.Second, false)
	if cache.totalBytes > 12 || len(cache.entries) > 2 {
		t.Fatal("pending memory limit exceeded")
	}
	cache.mu.Lock()
	cache.entries["c"].expiresAt = time.Now().Add(-time.Second)
	cache.mu.Unlock()
	if _, hit := cache.get("c"); hit {
		t.Fatal("expired pending result returned")
	}
	cache.set("oversize", make([]byte, 13), time.Second, false)
	if _, hit := cache.get("oversize"); hit {
		t.Fatal("oversized result retained")
	}
}

func TestPendingRenderRetriesAfterExpiry(t *testing.T) {
	client := newIndexClient(t, &fakeRenderIndex{})
	var renders atomic.Int32
	render := func(context.Context) ([]byte, error) { renders.Add(1); return []byte("image"), nil }
	key := strings.Repeat("b", 64)
	policy := renderCachePolicy{APIPath: "api/pjsk/card/list", UserID: "public", TTL: time.Hour}
	for range 2 {
		if _, err := client.renderRemoteFlight(t.Context(), "/api/pjsk/card/list", key, policy, render); err != nil {
			t.Fatal(err)
		}
	}
	if renders.Load() != 1 {
		t.Fatalf("pending image not reused: %d", renders.Load())
	}
	client.pending.mu.Lock()
	client.pending.entries[key].expiresAt = time.Now().Add(-time.Second)
	client.pending.mu.Unlock()
	if _, err := client.renderRemoteFlight(t.Context(), "/api/pjsk/card/list", key, policy, render); err != nil {
		t.Fatal(err)
	}
	if renders.Load() != 2 {
		t.Fatalf("expired pending image prevented retry: %d", renders.Load())
	}
}

func TestPendingRefReusedUntilIndexed(t *testing.T) {
	key := strings.Repeat("4", 64)
	index := &fakeRenderIndex{}
	client := newIndexClient(t, index)
	ref := testRef(t, "cn09")
	ref.IndexWritten = false
	var renders atomic.Int32
	for range 3 {
		image, err := client.renderRemoteImageFlight(artifactCtx(t), "/api/pjsk/card/list", key, testIndexPolicy, artifactRender(t, ref, &renders))
		if err != nil || image.Ref() != ref {
			t.Fatalf("image=%+v err=%v", image, err)
		}
	}
	if renders.Load() != 1 {
		t.Fatalf("pending ref not reused: renders=%d", renders.Load())
	}
	if lookups, _, _ := index.calls(); lookups != 1 {
		t.Fatalf("pending ref still consulted the index: %d lookups", lookups)
	}
	client.pending.mu.Lock()
	entry := client.pending.entries[key]
	ttl := time.Until(entry.expiresAt)
	size := entry.size
	client.pending.mu.Unlock()
	if ttl <= 30*time.Second || ttl > pendingRenderCacheTTLIndex {
		t.Fatalf("pending ref ttl = %v", ttl)
	}
	if size != int64(len(ref.CDNPath)+pendingRefBaseBytes) {
		t.Fatalf("pending ref accounted at %d bytes", size)
	}
}

func TestPendingBytesKeptOnDegradedInIndexMode(t *testing.T) {
	key := strings.Repeat("5", 64)
	client := newIndexClient(t, &fakeRenderIndex{})
	var renders atomic.Int32
	render := func(ctx context.Context) ([]byte, error) {
		renders.Add(1)
		if d, ok := directiveFrom(ctx); ok {
			d.outcome.Degraded = true
		}
		return []byte("degraded"), nil
	}
	policy := renderCachePolicy{APIPath: "api/pjsk/card/list", UserID: "public", TTL: 10 * time.Second}
	for range 2 {
		data, err := client.renderRemoteFlight(artifactCtx(t), "/api/pjsk/card/list", key, policy, render)
		if err != nil || string(data) != "degraded" {
			t.Fatalf("data=%q err=%v", data, err)
		}
	}
	if renders.Load() != 1 {
		t.Fatalf("degraded bytes not pending: renders=%d", renders.Load())
	}
}

func TestPendingRefAccountingLimits(t *testing.T) {
	cache := newLocalRenderCacheWithLimits(time.Minute, 4, int64(pendingRefBaseBytes+4))
	ref := &ArtifactRef{CDNPath: "a.png"}
	if gen := cache.setRef("too-big", ref, time.Minute); gen != 0 {
		t.Fatalf("oversized ref retained with generation %d", gen)
	}
	if gen := cache.setRef("nil", nil, time.Minute); gen != 0 {
		t.Fatal("nil ref retained")
	}
	var nilCache *localRenderCache
	if nilCache.setRef("x", ref, time.Minute) != 0 {
		t.Fatal("nil cache retained a ref")
	}
	if _, _, ok := nilCache.lookupEntry("x"); ok {
		t.Fatal("nil cache returned an entry")
	}
	small := &ArtifactRef{CDNPath: "a"}
	gen := cache.setRef("fits", small, time.Minute)
	if gen == 0 {
		t.Fatal("small ref not retained")
	}
	if _, got, ok := cache.lookupEntry("fits"); !ok || got != small {
		t.Fatal("small ref not returned")
	}
	evictor := newLocalRenderCacheWithLimits(time.Minute, 1, 1<<20)
	evictor.set("other", []byte("x"), time.Minute, false)
	if gen := evictor.setRef("new", small, time.Minute); gen == 0 {
		t.Fatal("ref evicted itself")
	}
	if _, hit := evictor.get("other"); hit {
		t.Fatal("LRU did not evict the older entry")
	}
}

func TestRenderIndexWriterThrottleBatchAndClose(t *testing.T) {
	index := &fakeRenderIndex{touchErr: errors.New("touch"), deleteErr: errors.New("delete")}
	writer := newRenderIndexWriter(index, 0)
	if writer.touchInterval != defaultRenderIndexTouchInterval {
		t.Fatalf("default interval = %v", writer.touchInterval)
	}
	now := time.Unix(1_700_000_000, 0)
	writer.now = func() time.Time { return now }
	writer.flushEvery = time.Hour
	writer.touch("k")
	writer.touch("k")
	writer.flush()
	now = now.Add(defaultRenderIndexTouchInterval)
	writer.touch("k")
	writer.expire("gone")
	writer.flush()
	_, touched, deleted := index.calls()
	if len(touched) != 2 || len(deleted) != 1 || deleted[0][0] != "gone" {
		t.Fatalf("touched=%v deleted=%v", touched, deleted)
	}
	if writer.lastErrLog.Load() == 0 {
		t.Fatal("flush errors not logged")
	}
	// The throttle map is pruned once entries age past the interval.
	now = now.Add(2 * defaultRenderIndexTouchInterval)
	writer.flush()
	if len(writer.lastTouch) != 0 {
		t.Fatalf("lastTouch not pruned: %d", len(writer.lastTouch))
	}
	for i := range 4*renderIndexBatchCap + 1 {
		writer.lastTouch[strings.Repeat("x", 8)+string(rune('a'+i%26))+time.Duration(i).String()] = now
	}
	writer.flush()
	if len(writer.lastTouch) != 0 {
		t.Fatalf("oversized throttle map kept %d keys", len(writer.lastTouch))
	}
	writer.close()
	writer.close()
	writer.touch("after")
	writer.expire("after")
	if len(writer.touches.order) != 0 || len(writer.expires.order) != 0 {
		t.Fatal("closed writer buffered keys")
	}
	var nilWriter *renderIndexWriter
	nilWriter.close()
}

func TestRenderIndexWriterLoopFlushes(t *testing.T) {
	index := &fakeRenderIndex{}
	writer := newRenderIndexWriter(index, time.Minute)
	writer.flushEvery = 5 * time.Millisecond
	writer.touch("loop")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, touched, _ := index.calls(); len(touched) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ticker never flushed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	writer.close()
}

func TestKeyBatchDropsOldest(t *testing.T) {
	var batch keyBatch
	for i := range renderIndexBatchCap + 2 {
		batch.add(time.Duration(i).String())
	}
	batch.add(time.Duration(renderIndexBatchCap + 1).String())
	keys := batch.drain()
	if len(keys) != renderIndexBatchCap || keys[0] != time.Duration(2).String() {
		t.Fatalf("len=%d first=%q", len(keys), keys[0])
	}
	if len(batch.drain()) != 0 {
		t.Fatal("drain kept keys")
	}
}

func TestHarukiDrawingClientClose(t *testing.T) {
	var nilClient *HarukiDrawingClient
	if nilClient.Close() != nil {
		t.Fatal("nil client close failed")
	}
	client := &HarukiDrawingClient{}
	if client.Close() != nil {
		t.Fatal("client without cache close failed")
	}
	client.SetRenderCache(NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour, Index: &fakeRenderIndex{}}))
	if client.Close() != nil {
		t.Fatal("index cache close failed")
	}
}

func TestThrottleLogWindow(t *testing.T) {
	var last atomic.Int64
	now := time.Unix(1_700_000_000, 0)
	if !throttleLog(&last, now) || throttleLog(&last, now.Add(time.Second)) || !throttleLog(&last, now.Add(renderIndexErrorLogInterval)) {
		t.Fatal("throttle window wrong")
	}
}

// The protected renderCachePolicy / renderCacheKeyMaterial block (T1 range
// hash d8af4a12...) moved down when RenderCacheConfig grew; pin its content by
// anchor instead of by line number.
func TestProtectedRenderCacheTypesUnchanged(t *testing.T) {
	raw, err := os.ReadFile("cache_types.go")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitAfter(string(raw), "\n")
	start := -1
	for i, line := range lines {
		if line == "type renderCachePolicy struct {\n" {
			start = i
			break
		}
	}
	if start < 0 || start+16 > len(lines) {
		t.Fatal("renderCachePolicy block not found")
	}
	digest := sha256.Sum256([]byte(strings.Join(lines[start:start+16], "")))
	if got := hex.EncodeToString(digest[:]); got != "d8af4a12f54c182465ea637fb9189c27237fdc19cf2a3f8f33be0710aad7a68e" {
		t.Fatalf("protected block hash = %s", got)
	}
}
