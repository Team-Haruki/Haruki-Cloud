package snapshot

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	renderregion "haruki-cloud/internal/pjsk/region"
)

const revalidationMySekaiJSON = `{"upload_time":1710000000,"updatedResources":{}}`

func newRevalidationProvider(client *fakePrivateDataClient) *ToolboxSnapshotProvider {
	return newValidToolboxProvider(client).WithPrivateDataCache(NewPrivateDataCache()).WithBuiltSnapshotCache(NewBuiltSnapshotCache())
}

// warmBuiltOnly resolves once so both caches are filled, then drops the raw
// payload cache the way its smaller byte budget does under load.
func warmBuiltOnly(t *testing.T, provider *ToolboxSnapshotProvider, opts ResolveOptions) Snapshot {
	t.Helper()
	snap, err := provider.Resolve(WithRequestCache(context.Background()), validToolboxSelector(), opts)
	if err != nil {
		t.Fatalf("warm Resolve() error = %v", err)
	}
	provider.WithPrivateDataCache(NewPrivateDataCache())
	return snap
}

func operationCounts(trace *commandtrace.Trace) map[string]int {
	counts := map[string]int{}
	for _, op := range trace.Snapshot().Operations {
		counts[op.Name] += op.Count
	}
	return counts
}

func TestToolboxProviderRevalidatesBuiltSnapshotAfterRawEviction(t *testing.T) {
	for _, needMySekai := range []bool{false, true} {
		client := &fakePrivateDataClient{suiteJSON: []byte(minimalSuiteJSON), mysekaiJSON: []byte(revalidationMySekaiJSON), uploadTime: "1710000000"}
		provider := newRevalidationProvider(client)
		opts := ResolveOptions{NeedMySekai: needMySekai}
		warm := warmBuiltOnly(t, provider, opts)

		ctx, trace := commandtrace.WithTrace(WithRequestCache(context.Background()))
		snap, err := provider.Resolve(ctx, validToolboxSelector(), opts)
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		if snap != warm {
			t.Fatal("expected the revalidated built snapshot instance")
		}
		if got := client.suiteKnownTimes; !slices.Equal(got, []int64{0, 1710000000}) {
			t.Fatalf("suite known times = %v, want the built snapshot's upload_time on the miss", got)
		}
		if len(client.suiteCalls) != 1 || client.suiteNotModified != 1 {
			t.Fatalf("expected one full suite transfer, got %d full and %d not-modified", len(client.suiteCalls), client.suiteNotModified)
		}
		if needMySekai && (len(client.mysekaiCalls) != 1 || client.mysekaiNotModified != 1) {
			t.Fatalf("expected one full mysekai transfer, got %d full and %d not-modified", len(client.mysekaiCalls), client.mysekaiNotModified)
		}
		if counts := operationCounts(trace); counts["snapshot.built_cache_revalidated"] != 1 || counts["snapshot.raw_cache_version_only"] == 0 {
			t.Fatalf("operations = %v", counts)
		}
	}
}

func TestToolboxProviderRevalidationFetchesChangedData(t *testing.T) {
	client := &fakePrivateDataClient{suiteJSON: []byte(minimalSuiteJSON), uploadTime: "1710000000"}
	provider := newRevalidationProvider(client)
	warm := warmBuiltOnly(t, provider, ResolveOptions{})

	client.suiteJSON = []byte(strings.ReplaceAll(minimalSuiteJSON, "1710000000", "1710000500"))
	client.uploadTime = "1710000500"
	snap, err := provider.Resolve(WithRequestCache(context.Background()), validToolboxSelector(), ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if snap == warm || len(client.suiteCalls) != 2 {
		t.Fatalf("changed data must be transferred and rebuilt (full fetches %d)", len(client.suiteCalls))
	}
	if key, ok := provider.builtCache.latestKey(builtSnapshotIdentity{Region: "jp", UID: 123456789}); !ok || key.SuiteUploadTime != 1710000500 {
		t.Fatalf("latest key = %+v, %t", key, ok)
	}
}

func TestToolboxProviderRevalidationRefetchesWhenBuiltEntryIsGone(t *testing.T) {
	client := &fakePrivateDataClient{suiteJSON: []byte(minimalSuiteJSON), mysekaiJSON: []byte(revalidationMySekaiJSON), uploadTime: "1710000000"}
	provider := newRevalidationProvider(client)
	opts := ResolveOptions{NeedMySekai: true}
	warm := warmBuiltOnly(t, provider, opts)

	// Evict the built entry between the candidate lookup and its use.
	provider.client = evictingClient{fakePrivateDataClient: client, evict: func() {
		provider.builtCache.mu.Lock()
		for el := provider.builtCache.ll.Front(); el != nil; el = provider.builtCache.ll.Front() {
			provider.builtCache.removeElementLocked(el)
		}
		provider.builtCache.mu.Unlock()
	}}
	snap, err := provider.Resolve(WithRequestCache(context.Background()), validToolboxSelector(), opts)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if snap == nil || snap == warm {
		t.Fatal("expected a fresh build after the built entry was evicted")
	}
	if len(client.suiteCalls) != 2 || len(client.mysekaiCalls) != 2 {
		t.Fatalf("expected full refetches, got suite %d mysekai %d", len(client.suiteCalls), len(client.mysekaiCalls))
	}
}

type evictingClient struct {
	*fakePrivateDataClient
	evict func()
}

func (c evictingClient) GetSuiteDataConditionalContext(ctx context.Context, server string, uid int64, platform, platformUserID string, known int64) ([]byte, bool, error) {
	data, notModified, err := c.fakePrivateDataClient.GetSuiteDataConditionalContext(ctx, server, uid, platform, platformUserID, known)
	c.evict()
	return data, notModified, err
}

func TestToolboxProviderRevalidationKeepsUpstreamErrors(t *testing.T) {
	client := &fakePrivateDataClient{suiteJSON: []byte(minimalSuiteJSON), uploadTime: "1710000000"}
	provider := newRevalidationProvider(client)
	warmBuiltOnly(t, provider, ResolveOptions{})

	denied := errors.New("authorization revoked")
	client.suiteErr = denied
	if snap, err := provider.Resolve(WithRequestCache(context.Background()), validToolboxSelector(), ResolveOptions{}); !errors.Is(err, denied) || snap != nil {
		t.Fatalf("Resolve() = %v, %v; the built snapshot must never be served without the caller's validation", snap, err)
	}
}

func TestToolboxProviderVersionOnlyRequestEntryIsNotServedForMusicMeta(t *testing.T) {
	client := &fakePrivateDataClient{suiteJSON: []byte(minimalSuiteJSON), uploadTime: "1710000000"}
	provider := newRevalidationProvider(client)
	warmBuiltOnly(t, provider, ResolveOptions{})

	ctx := WithRequestCache(context.Background())
	if _, err := provider.Resolve(ctx, validToolboxSelector(), ResolveOptions{}); err != nil {
		t.Fatalf("revalidated Resolve() error = %v", err)
	}
	// The same command now needs music meta, which the built cache never
	// memoizes: the versionOnly request entry must be replaced by a body.
	snap, err := provider.Resolve(ctx, validToolboxSelector(), ResolveOptions{NeedMusicMeta: true})
	if err != nil || snap == nil {
		t.Fatalf("music meta Resolve() = %v, %v", snap, err)
	}
	if len(client.suiteCalls) != 2 {
		t.Fatalf("expected the body to be fetched for the music meta resolve, got %d full fetches", len(client.suiteCalls))
	}
	if _, err := provider.Resolve(ctx, validToolboxSelector(), ResolveOptions{NeedMusicMeta: true}); err != nil || len(client.suiteCalls) != 2 {
		t.Fatalf("refreshed request entry was not reused: %v, %d full fetches", err, len(client.suiteCalls))
	}
}

func TestBuiltSnapshotLatestKeyFollowsPutsAndRemovals(t *testing.T) {
	c := NewBuiltSnapshotCacheWithLimits(2, 0, time.Hour)
	id := builtSnapshotIdentity{Region: "jp", UID: 1}
	if _, ok := c.latestKey(id); ok {
		t.Fatal("empty cache must not report a key")
	}
	c.Put(builtKey(1, 100), buildTestSnapshot(t), 10)
	c.Put(builtKey(1, 200), buildTestSnapshot(t), 10)
	if key, ok := c.latestKey(id); !ok || key.SuiteUploadTime != 200 {
		t.Fatalf("latest = %+v, %t; want the newest put", key, ok)
	}
	// Evicting the older entry keeps the newest; evicting the newest drops it.
	c.Put(builtKey(2, 100), buildTestSnapshot(t), 10)
	if key, ok := c.latestKey(id); !ok || key.SuiteUploadTime != 200 {
		t.Fatalf("latest after unrelated eviction = %+v, %t", key, ok)
	}
	c.Put(builtKey(3, 100), buildTestSnapshot(t), 10)
	if _, ok := c.latestKey(id); ok {
		t.Fatal("latest key must be dropped with its evicted entry")
	}
	if len(c.latest) != 2 {
		t.Fatalf("latest index holds %d identities, want 2", len(c.latest))
	}

	expiring := NewBuiltSnapshotCacheWithLimits(0, 0, time.Millisecond)
	expiring.Put(builtKey(1, 100), buildTestSnapshot(t), 10)
	time.Sleep(5 * time.Millisecond)
	if _, ok := expiring.latestKey(id); ok {
		t.Fatal("expired entries must not be revalidated")
	}
	var nilCache *BuiltSnapshotCache
	if _, ok := nilCache.latestKey(id); ok {
		t.Fatal("nil cache must not report a key")
	}
}

func TestPrivateDataCacheVersionOnlyRequiresFallback(t *testing.T) {
	cache := NewPrivateDataCache()
	payload, hit, err := cache.fetchPayloadWithFallback(context.Background(), suiteKey(), 42, func(known int64) ([]byte, bool, error) {
		if known != 42 {
			t.Fatalf("known = %d, want the fallback", known)
		}
		return nil, true, nil
	})
	if err != nil || hit || !payload.versionOnly || payload.uploadTime != 42 || payload.data != nil {
		t.Fatalf("version-only payload = %+v, %t, %v", payload, hit, err)
	}
	if entry := cache.load(suiteKey()); entry != nil {
		t.Fatal("a version-only payload must not be stored")
	}
	if _, _, err := cache.fetchPayloadWithFallback(context.Background(), suiteKey(), 0, func(int64) ([]byte, bool, error) { return nil, true, nil }); err == nil {
		t.Fatal("not-modified without any known version must still fail")
	}
	// A raw entry takes precedence over the fallback version.
	if _, _, err := cache.fetchPayloadWithFallback(context.Background(), suiteKey(), 0, func(int64) ([]byte, bool, error) {
		return []byte(`{"upload_time":7}`), false, nil
	}); err != nil {
		t.Fatal(err)
	}
	payload, hit, err = cache.fetchPayloadWithFallback(context.Background(), suiteKey(), 42, func(known int64) ([]byte, bool, error) {
		if known != 7 {
			t.Fatalf("known = %d, want the raw entry's version", known)
		}
		return nil, true, nil
	})
	if err != nil || !hit || payload.versionOnly || string(payload.data) != `{"upload_time":7}` {
		t.Fatalf("raw hit = %+v, %t, %v", payload, hit, err)
	}
}

func TestToolboxProviderRevalidationRefetchFailures(t *testing.T) {
	failed := errors.New("toolbox unavailable")
	for _, tc := range []struct {
		name    string
		mutate  func(*fakePrivateDataClient)
		wantErr string
	}{
		{"suite error", func(c *fakePrivateDataClient) { c.suiteErr = failed }, failed.Error()},
		{"empty suite", func(c *fakePrivateDataClient) { c.suiteJSON = nil }, "suite snapshot is empty"},
		{"mysekai error", func(c *fakePrivateDataClient) { c.mysekaiErr = failed }, failed.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakePrivateDataClient{suiteJSON: []byte(minimalSuiteJSON), mysekaiJSON: []byte(revalidationMySekaiJSON), uploadTime: "1710000000"}
			provider := newRevalidationProvider(client)
			opts := ResolveOptions{NeedMySekai: true}
			warmBuiltOnly(t, provider, opts)
			provider.WithBuiltSnapshotCache(NewBuiltSnapshotCache())
			suiteRequest := privateDataRequest{server: "jp", dataType: "suite", uid: 123456789, platform: "qq", imUserID: "10001"}
			tc.mutate(client)
			_, _, err := provider.refetchVersionOnly(WithRequestCache(context.Background()), suiteRequest, func(known int64) ([]byte, bool, error) {
				return client.GetSuiteDataConditionalContext(context.Background(), "jp", 123456789, "qq", "10001", known)
			}, toolboxPrivateDataResult{privateDataPayload: privateDataPayload{uploadTime: 1710000000, versionOnly: true}}, privateDataPayload{uploadTime: 1710000000, versionOnly: true})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("refetchVersionOnly() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
	if got := snapshotRegionForBinding(renderregion.EN, ""); got != renderregion.EN {
		t.Fatalf("snapshotRegionForBinding without binding server = %v", got)
	}
}
