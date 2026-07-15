package snapshot

import (
	"context"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/accountdata"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func buildTestSnapshot(t *testing.T) Snapshot {
	t.Helper()
	snap, err := NewDefaultSnapshotFactory(nil, nil).Build(context.Background(), BuildInput{
		Region:    renderregion.JP,
		SuiteJSON: []byte(minimalSuiteJSON),
	})
	if err != nil {
		t.Fatalf("build test snapshot: %v", err)
	}
	return snap
}

func builtKey(uid int64, suiteUT int64) builtSnapshotKey {
	return builtSnapshotKey{Region: "jp", UID: uid, SuiteUploadTime: suiteUT}
}

func TestBuiltSnapshotCacheGetPut(t *testing.T) {
	c := NewBuiltSnapshotCache()
	snap := buildTestSnapshot(t)
	key := builtKey(1, 100)

	if got := c.Get(key); got != nil {
		t.Fatalf("empty cache must miss, got %v", got)
	}
	c.Put(key, snap)
	if got := c.Get(key); got != snap {
		t.Fatalf("Get must return the stored snapshot instance")
	}
	if got := c.Get(builtKey(2, 100)); got != nil {
		t.Fatalf("different key must miss, got %v", got)
	}
}

func TestBuiltSnapshotCacheNilReceiverAndNilSnapshot(t *testing.T) {
	var c *BuiltSnapshotCache
	if got := c.Get(builtKey(1, 100)); got != nil {
		t.Fatalf("nil receiver must miss")
	}
	c.Put(builtKey(1, 100), buildTestSnapshot(t)) // must not panic

	c2 := NewBuiltSnapshotCache()
	c2.Put(builtKey(1, 100), nil) // nil snapshot ignored
	if got := c2.Get(builtKey(1, 100)); got != nil {
		t.Fatalf("nil snapshot must not be stored")
	}
}

func TestBuiltSnapshotCacheEvictsLeastRecentlyUsed(t *testing.T) {
	c := NewBuiltSnapshotCache()
	c.maxEntries = 2
	c.Put(builtKey(1, 100), buildTestSnapshot(t))
	c.Put(builtKey(2, 100), buildTestSnapshot(t))
	c.Put(builtKey(3, 100), buildTestSnapshot(t)) // evicts uid 1

	if got := c.Get(builtKey(1, 100)); got != nil {
		t.Fatalf("least-recently-used entry should have been evicted")
	}
	if got := c.Get(builtKey(2, 100)); got == nil {
		t.Fatalf("uid 2 should still be cached")
	}
	if got := c.Get(builtKey(3, 100)); got == nil {
		t.Fatalf("uid 3 should still be cached")
	}
}

func TestBuiltSnapshotCacheTTLExpiry(t *testing.T) {
	c := NewBuiltSnapshotCache()
	c.ttl = 5 * time.Millisecond
	key := builtKey(1, 100)
	c.Put(key, buildTestSnapshot(t))
	if got := c.Get(key); got == nil {
		t.Fatalf("entry should be present before TTL")
	}
	time.Sleep(20 * time.Millisecond)
	if got := c.Get(key); got != nil {
		t.Fatalf("entry should be expired after TTL")
	}
}

func newMemoProvider(client *fakePrivateDataClient) *ToolboxSnapshotProvider {
	return NewToolboxSnapshotProvider(
		&fakeBindingLookup{
			bindings: map[string]*accountdata.ResolvedBinding{
				"jp": {PJSKUserID: "123456789", Server: "jp", SuiteVisible: true},
			},
		},
		client,
		nil,
		nil,
	).WithPrivateDataCache(NewPrivateDataCache()).WithBuiltSnapshotCache(NewBuiltSnapshotCache())
}

func TestToolboxSnapshotProviderMemoizesBuiltSnapshot(t *testing.T) {
	client := &fakePrivateDataClient{suiteJSON: []byte(minimalSuiteJSON), uploadTime: "1710000000"}
	provider := newMemoProvider(client)
	selector := Selector{IMPlatform: "qq", IMUserID: "10001", Region: renderregion.JP}

	snap1, err := provider.Resolve(WithRequestCache(context.Background()), selector, ResolveOptions{})
	if err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}
	snap2, err := provider.Resolve(WithRequestCache(context.Background()), selector, ResolveOptions{})
	if err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}
	if snap1 != snap2 {
		t.Fatalf("unchanged account must reuse the memoized built snapshot instance")
	}
	if got := len(client.suiteCalls); got != 1 {
		t.Fatalf("suite fetches = %d, want 1 (raw payload also served from cache)", got)
	}
}

func TestToolboxSnapshotProviderRebuildsOnChangedUploadTime(t *testing.T) {
	client := &fakePrivateDataClient{suiteJSON: []byte(minimalSuiteJSON), uploadTime: "1710000000"}
	provider := newMemoProvider(client)
	selector := Selector{IMPlatform: "qq", IMUserID: "10001", Region: renderregion.JP}

	snap1, err := provider.Resolve(WithRequestCache(context.Background()), selector, ResolveOptions{})
	if err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}

	// The account uploaded new data: both the probe value and the payload's
	// embedded upload_time change, so the memo key differs and forces a rebuild.
	client.suiteJSON = []byte(strings.ReplaceAll(minimalSuiteJSON, "1710000000", "1710000500"))
	client.uploadTime = "1710000500"

	snap2, err := provider.Resolve(WithRequestCache(context.Background()), selector, ResolveOptions{})
	if err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}
	if snap1 == snap2 {
		t.Fatalf("changed upload_time must rebuild the snapshot, not reuse the memo")
	}
	if got := len(client.suiteCalls); got != 2 {
		t.Fatalf("changed data must trigger a fresh suite fetch, got %d fetches", got)
	}
}
