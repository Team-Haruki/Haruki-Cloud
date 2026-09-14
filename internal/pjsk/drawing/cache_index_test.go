package drawing

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/imagecache"
)

type fakeRenderIndex struct {
	mu        sync.Mutex
	entries   map[string]imagecache.RenderIndexEntry
	lookupErr error
	touchErr  error
	deleteErr error
	lookups   int
	touched   [][]string
	deleted   [][]string
}

func (f *fakeRenderIndex) LookupRender(_ context.Context, key string) (imagecache.RenderIndexEntry, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups++
	if f.lookupErr != nil {
		return imagecache.RenderIndexEntry{}, false, f.lookupErr
	}
	entry, ok := f.entries[key]
	return entry, ok, nil
}

func (f *fakeRenderIndex) TouchRender(_ context.Context, keys []string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.touched = append(f.touched, append([]string(nil), keys...))
	return int64(len(keys)), f.touchErr
}

func (f *fakeRenderIndex) DeleteRender(_ context.Context, keys []string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, append([]string(nil), keys...))
	return int64(len(keys)), f.deleteErr
}

func (f *fakeRenderIndex) calls() (int, [][]string, [][]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lookups, f.touched, f.deleted
}

const testIndexCDNPath = "pjsk/api/pjsk/card/list/cafe.png"

func indexEntry(key string, expiresAt time.Time) imagecache.RenderIndexEntry {
	return imagecache.RenderIndexEntry{
		RequestKey: key, ContentHash: strings.Repeat("c", 64), APIPath: "api/pjsk/card/list",
		UserID: "public", GroupName: "pjsk", KeyVersion: 3, TTLSeconds: 3600, ExpiresAt: expiresAt,
		Entry: imagecache.ImageEntry{CDNPath: testIndexCDNPath, StorageBackend: imagecache.BackendGarage,
			MediaType: "image/png", SizeBytes: 5},
	}
}

var testIndexPolicy = renderCachePolicy{APIPath: "api/pjsk/card/list", UserID: "public", TTL: time.Hour}

func newIndexClient(t *testing.T, index RenderIndex) *RenderCacheClient {
	t.Helper()
	objects := storagetest.NewMemory()
	objects.Seed(map[string][]byte{testIndexCDNPath: []byte("stored")})
	cfg := RenderCacheConfig{TTL: time.Hour, Index: index, Artifacts: objects, TouchInterval: time.Minute}
	client := NewRenderCacheClient(cfg)
	if client == nil {
		t.Fatal("index-mode client not constructed")
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func failRender(t *testing.T) func(context.Context) ([]byte, error) {
	return func(context.Context) ([]byte, error) {
		t.Error("index hit rendered again")
		return nil, errors.New("unexpected render")
	}
}

func TestNewRenderCacheClientIndexModeEnableCondition(t *testing.T) {
	var typedNil *imagecache.PGStore
	if NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour, Index: typedNil}) != nil {
		t.Fatal("typed-nil index enabled the cache")
	}
	if NewRenderCacheClient(RenderCacheConfig{Index: &fakeRenderIndex{}}) != nil {
		t.Fatal("zero TTL enabled the cache")
	}
	client := NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour, Index: &fakeRenderIndex{}})
	if client == nil || !client.indexMode() || client.fetcher == nil || client.indexWriter == nil {
		t.Fatalf("index-only client = %+v", client)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour}) != nil {
		t.Fatal("a client without an index was constructed")
	}
	var nilClient *RenderCacheClient
	if nilClient.Close() != nil || nilClient.indexMode() {
		t.Fatal("nil client not inert")
	}
}

func TestIndexLookupHitServesRefAndTouches(t *testing.T) {
	key := strings.Repeat("a", 64)
	index := &fakeRenderIndex{entries: map[string]imagecache.RenderIndexEntry{key: indexEntry(key, time.Now().Add(time.Hour))}}
	client := newIndexClient(t, index)
	for range 3 {
		image, err := client.renderRemoteImageFlight(t.Context(), "/api/pjsk/card/list", key, testIndexPolicy, failRender(t))
		if err != nil {
			t.Fatal(err)
		}
		ref := image.Ref()
		if ref == nil || ref.CDNPath != testIndexCDNPath || ref.NodeName != "" || !ref.IndexWritten || ref.CacheKey != key || ref.ExpiresAt == nil {
			t.Fatalf("ref = %+v", ref)
		}
		if data, err := image.Bytes(t.Context()); err != nil || string(data) != "stored" {
			t.Fatalf("bytes = %q, %v", data, err)
		}
	}
	_ = client.Close()
	_, touched, deleted := index.calls()
	if len(touched) != 1 || len(touched[0]) != 1 || touched[0][0] != key || len(deleted) != 0 {
		t.Fatalf("touched=%v deleted=%v, want one throttled touch", touched, deleted)
	}
}

func TestIndexLookupInfiniteRowHasNoExpiry(t *testing.T) {
	key := strings.Repeat("d", 64)
	index := &fakeRenderIndex{entries: map[string]imagecache.RenderIndexEntry{key: indexEntry(key, time.Time{})}}
	client := newIndexClient(t, index)
	image, hit := client.lookupIndexContext(t.Context(), key)
	if !hit || image.Ref().ExpiresAt != nil {
		t.Fatalf("infinite row = %+v hit=%v", image.Ref(), hit)
	}
}

func TestIndexLookupExpiredRowRendersAndDeletes(t *testing.T) {
	key := strings.Repeat("e", 64)
	index := &fakeRenderIndex{entries: map[string]imagecache.RenderIndexEntry{key: indexEntry(key, time.Now().Add(-time.Second))}}
	client := newIndexClient(t, index)
	var renders atomic.Int32
	render := func(context.Context) ([]byte, error) { renders.Add(1); return []byte("fresh"), nil }
	data, err := client.renderRemoteFlight(t.Context(), "/api/pjsk/card/list", key, testIndexPolicy, render)
	if err != nil || string(data) != "fresh" || renders.Load() != 1 {
		t.Fatalf("data=%q err=%v renders=%d", data, err, renders.Load())
	}
	_ = client.Close()
	_, touched, deleted := index.calls()
	if len(deleted) != 1 || deleted[0][0] != key || len(touched) != 0 {
		t.Fatalf("deleted=%v touched=%v", deleted, touched)
	}
}

func TestIndexLookupErrorAndUnusableRowAreMisses(t *testing.T) {
	key := strings.Repeat("f", 64)
	index := &fakeRenderIndex{lookupErr: errors.New("pg down")}
	client := newIndexClient(t, index)
	for range 2 {
		if _, hit := client.lookupIndexContext(t.Context(), key); hit {
			t.Fatal("lookup error served a hit")
		}
	}
	if client.indexErrLog.Load() == 0 {
		t.Fatal("lookup error was not logged")
	}
	bad := indexEntry(key, time.Time{})
	bad.Entry.CDNPath = "../escape.png"
	index = &fakeRenderIndex{entries: map[string]imagecache.RenderIndexEntry{key: bad}}
	client = newIndexClient(t, index)
	if _, hit := client.lookupIndexContext(t.Context(), key); hit {
		t.Fatal("unusable cdn_path served a hit")
	}
}

func TestIndexMissRendersOnceAndReusesPendingBytes(t *testing.T) {
	key := strings.Repeat("2", 64)
	client := newIndexClient(t, &fakeRenderIndex{})
	var renders atomic.Int32
	render := func(context.Context) ([]byte, error) { renders.Add(1); return []byte("bytes"), nil }
	for range 2 {
		if data, err := client.renderRemoteFlight(t.Context(), "/api/pjsk/card/list", key, testIndexPolicy, render); err != nil || string(data) != "bytes" {
			t.Fatalf("data=%q err=%v", data, err)
		}
	}
	if renders.Load() != 1 {
		t.Fatalf("pending bytes not reused: renders=%d", renders.Load())
	}
}

// artifactRender simulates postPrepared answering the directive with a ref.
func artifactRender(t *testing.T, ref *ArtifactRef, renders *atomic.Int32) func(context.Context) ([]byte, error) {
	return func(ctx context.Context) ([]byte, error) {
		renders.Add(1)
		d, ok := directiveFrom(ctx)
		if !ok || !d.Store {
			t.Error("cached render carried no storing directive")
			return nil, errors.New("no directive")
		}
		d.outcome.Ref = ref
		return nil, nil
	}
}

func artifactCtx(t *testing.T) context.Context {
	return context.WithValue(t.Context(), artifactModeCtxKey{}, newArtifactSettings(ArtifactConfig{Endpoints: []string{"*"}}))
}

func TestIndexModeRefPendingUntilIndexed(t *testing.T) {
	for _, written := range []bool{true, false} {
		key := strings.Repeat("3", 63)
		if written {
			key += "a"
		} else {
			key += "b"
		}
		client := newIndexClient(t, &fakeRenderIndex{})
		ref := testRef(t, "cn09")
		ref.IndexWritten = written
		var renders atomic.Int32
		image, err := client.renderRemoteImageFlight(artifactCtx(t), "/api/pjsk/card/list", key, testIndexPolicy, artifactRender(t, ref, &renders))
		if err != nil || image.Ref() != ref {
			t.Fatalf("image=%+v err=%v", image, err)
		}
		_, pendingRef, pending := client.pending.lookupEntry(key)
		if written && pending {
			t.Fatal("index_written ref kept a pending entry")
		}
		if !written && (!pending || pendingRef != ref) {
			t.Fatalf("unindexed ref not pending: %v %v", pendingRef, pending)
		}
		if _, hit := client.pending.get(key); hit {
			t.Fatal("pending ref leaked through the bytes getter")
		}
	}
}

func TestImageArtifactAndEscapedCDNPath(t *testing.T) {
	ref := &ArtifactRef{CDNPath: "pjsk/api/a b/ü.png"}
	image := ImageArtifact(ref)
	if image.Ref() != ref {
		t.Fatalf("image = %+v", image)
	}
	if _, err := image.Bytes(t.Context()); !errors.Is(err, ErrArtifactBytesUnavailable) {
		t.Fatalf("bytes err = %v", err)
	}
	if got := ref.EscapedCDNPath(); got != "pjsk/api/a%20b/%C3%BC.png" {
		t.Fatalf("escaped = %q", got)
	}
	var nilRef *ArtifactRef
	if nilRef.EscapedCDNPath() != "" {
		t.Fatal("nil ref escaped to a path")
	}
}

func TestIndexModeRenderErrorPropagates(t *testing.T) {
	client := newIndexClient(t, &fakeRenderIndex{})
	want := errors.New("drawing down")
	_, err := client.renderRemoteImageFlight(t.Context(), "/api/pjsk/card/list", strings.Repeat("6", 64), testIndexPolicy,
		func(context.Context) ([]byte, error) { return nil, want })
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}
