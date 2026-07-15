package snapshot

import (
	"context"
	"errors"
	"sync"
	"testing"

	"haruki-cloud/internal/pjsk/accountdata"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func suiteKey() PrivateDataKey {
	return PrivateDataKey{Server: "jp", DataType: "suite", UID: 123456789}
}

func TestPrivateDataCacheColdMissFetchesWithoutProbe(t *testing.T) {
	c := NewPrivateDataCache()
	probeCalls, fetchCalls := 0, 0
	payload := []byte(`{"upload_time":1710000000,"x":1}`)

	data, cross, err := c.Fetch(suiteKey(),
		func() (int64, error) { probeCalls++; return 1710000000, nil },
		func() ([]byte, error) { fetchCalls++; return payload, nil },
	)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if cross {
		t.Fatalf("cold miss must not report a cross-request hit")
	}
	if probeCalls != 0 {
		t.Fatalf("cold miss must not probe upload_time, got %d probes", probeCalls)
	}
	if fetchCalls != 1 {
		t.Fatalf("cold miss must full-fetch exactly once, got %d", fetchCalls)
	}
	if string(data) != string(payload) {
		t.Fatalf("unexpected data %q", data)
	}
}

func TestPrivateDataCacheServesOnMatchingUploadTime(t *testing.T) {
	c := NewPrivateDataCache()
	payload := []byte(`{"upload_time":1710000000}`)
	fetchCalls := 0
	fetch := func() ([]byte, error) { fetchCalls++; return payload, nil }

	if _, _, err := c.Fetch(suiteKey(), func() (int64, error) { return 1710000000, nil }, fetch); err != nil {
		t.Fatalf("cold Fetch() error = %v", err)
	}

	probeCalls := 0
	data, cross, err := c.Fetch(suiteKey(),
		func() (int64, error) { probeCalls++; return 1710000000, nil },
		func() ([]byte, error) { fetchCalls++; return []byte("SHOULD_NOT_FETCH"), nil },
	)
	if err != nil {
		t.Fatalf("warm Fetch() error = %v", err)
	}
	if !cross {
		t.Fatalf("matching upload_time must serve from cache")
	}
	if probeCalls != 1 {
		t.Fatalf("warm hit must probe once, got %d", probeCalls)
	}
	if fetchCalls != 1 {
		t.Fatalf("warm hit must not re-fetch, total fetches = %d", fetchCalls)
	}
	if string(data) != string(payload) {
		t.Fatalf("warm hit returned %q, want cached payload", data)
	}
}

func TestPrivateDataCacheRefetchesOnChangedUploadTime(t *testing.T) {
	c := NewPrivateDataCache()
	first := []byte(`{"upload_time":1710000000,"v":1}`)
	second := []byte(`{"upload_time":1710000500,"v":2}`)

	if _, _, err := c.Fetch(suiteKey(), func() (int64, error) { return 1710000000, nil }, func() ([]byte, error) { return first, nil }); err != nil {
		t.Fatalf("cold Fetch() error = %v", err)
	}

	fetchCalls := 0
	data, cross, err := c.Fetch(suiteKey(),
		func() (int64, error) { return 1710000500, nil }, // changed
		func() ([]byte, error) { fetchCalls++; return second, nil },
	)
	if err != nil {
		t.Fatalf("stale Fetch() error = %v", err)
	}
	if cross {
		t.Fatalf("changed upload_time must not report a cache hit")
	}
	if fetchCalls != 1 {
		t.Fatalf("changed upload_time must re-fetch once, got %d", fetchCalls)
	}
	if string(data) != string(second) {
		t.Fatalf("stale Fetch returned %q, want refreshed payload", data)
	}

	// The refreshed payload is now cached under the new upload_time.
	data, cross, err = c.Fetch(suiteKey(),
		func() (int64, error) { return 1710000500, nil },
		func() ([]byte, error) { t.Fatal("must not fetch after refresh"); return nil, nil },
	)
	if err != nil || !cross || string(data) != string(second) {
		t.Fatalf("post-refresh hit failed: cross=%v err=%v data=%q", cross, err, data)
	}
}

func TestPrivateDataCacheNeverServesOnProbeError(t *testing.T) {
	c := NewPrivateDataCache()
	payload := []byte(`{"upload_time":1710000000}`)
	if _, _, err := c.Fetch(suiteKey(), func() (int64, error) { return 1710000000, nil }, func() ([]byte, error) { return payload, nil }); err != nil {
		t.Fatalf("cold Fetch() error = %v", err)
	}

	boom := errors.New("invalid platform or platform_user_id")
	data, cross, err := c.Fetch(suiteKey(),
		func() (int64, error) { return 0, boom },
		func() ([]byte, error) {
			t.Fatal("must not full-fetch when the authorized probe fails")
			return nil, nil
		},
	)
	if !errors.Is(err, boom) {
		t.Fatalf("probe error must propagate, got %v", err)
	}
	if cross || data != nil {
		t.Fatalf("probe error must not serve cached data (cross=%v, data=%q)", cross, data)
	}
}

func TestPrivateDataCacheSkipsCachingWithoutUploadTime(t *testing.T) {
	c := NewPrivateDataCache()
	payload := []byte(`{"no_upload_time":1}`)
	fetchCalls, probeCalls := 0, 0
	fetch := func() ([]byte, error) { fetchCalls++; return payload, nil }
	probe := func() (int64, error) { probeCalls++; return 1710000000, nil }

	if _, _, err := c.Fetch(suiteKey(), probe, fetch); err != nil {
		t.Fatalf("first Fetch() error = %v", err)
	}
	if _, cross, err := c.Fetch(suiteKey(), probe, fetch); err != nil || cross {
		t.Fatalf("payload without upload_time must not be cached (cross=%v, err=%v)", cross, err)
	}
	if fetchCalls != 2 {
		t.Fatalf("uncacheable payload must be re-fetched, got %d fetches", fetchCalls)
	}
	if probeCalls != 0 {
		t.Fatalf("an uncached account must never be probed, got %d probes", probeCalls)
	}
}

func TestPrivateDataCacheNilReceiverBypasses(t *testing.T) {
	var c *PrivateDataCache
	payload := []byte(`{"upload_time":1710000000}`)
	data, cross, err := c.Fetch(suiteKey(),
		func() (int64, error) { t.Fatal("nil cache must not probe"); return 0, nil },
		func() ([]byte, error) { return payload, nil },
	)
	if err != nil || cross || string(data) != string(payload) {
		t.Fatalf("nil receiver must bypass to full fetch (cross=%v, err=%v, data=%q)", cross, err, data)
	}
}

func TestPrivateDataCacheServedDataIsIsolatedFromCache(t *testing.T) {
	c := NewPrivateDataCache()
	payload := []byte(`{"upload_time":1710000000}`)
	if _, _, err := c.Fetch(suiteKey(), func() (int64, error) { return 1710000000, nil }, func() ([]byte, error) { return payload, nil }); err != nil {
		t.Fatalf("cold Fetch() error = %v", err)
	}
	first, _, err := c.Fetch(suiteKey(), func() (int64, error) { return 1710000000, nil }, func() ([]byte, error) { return nil, nil })
	if err != nil {
		t.Fatalf("warm Fetch() error = %v", err)
	}
	for i := range first {
		first[i] = 'X'
	}
	second, _, err := c.Fetch(suiteKey(), func() (int64, error) { return 1710000000, nil }, func() ([]byte, error) { return nil, nil })
	if err != nil {
		t.Fatalf("second warm Fetch() error = %v", err)
	}
	if string(second) != string(payload) {
		t.Fatalf("mutating a served copy corrupted the cache: %q", second)
	}
}

func TestPrivateDataCacheEvictsLeastRecentlyUsed(t *testing.T) {
	c := NewPrivateDataCache()
	c.maxEntries = 2
	c.maxBytes = 0 // disable the byte bound for this test

	store := func(uid int64) {
		key := PrivateDataKey{Server: "jp", DataType: "suite", UID: uid}
		if _, _, err := c.Fetch(key, func() (int64, error) { return 1, nil }, func() ([]byte, error) { return []byte(`{"upload_time":1}`), nil }); err != nil {
			t.Fatalf("store uid %d error = %v", uid, err)
		}
	}
	store(1)
	store(2)
	store(3) // evicts uid 1 (least recently used)

	probeCalls := 0
	_, cross, err := c.Fetch(PrivateDataKey{Server: "jp", DataType: "suite", UID: 1},
		func() (int64, error) { probeCalls++; return 1, nil },
		func() ([]byte, error) { return []byte(`{"upload_time":1}`), nil },
	)
	if err != nil {
		t.Fatalf("Fetch(evicted) error = %v", err)
	}
	if cross || probeCalls != 0 {
		t.Fatalf("evicted entry must be a cold miss (cross=%v, probes=%d)", cross, probeCalls)
	}
}

func TestPrivateDataCacheConcurrentFetchIsRaceFree(t *testing.T) {
	c := NewPrivateDataCache()
	payload := []byte(`{"upload_time":1710000000}`)
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, _, err := c.Fetch(suiteKey(),
				func() (int64, error) { return 1710000000, nil },
				func() ([]byte, error) { return payload, nil },
			)
			if err != nil || string(data) != string(payload) {
				t.Errorf("concurrent Fetch mismatch: err=%v data=%q", err, data)
			}
		}()
	}
	wg.Wait()
}

func TestToolboxSnapshotProviderSharesPrivateDataAcrossRequests(t *testing.T) {
	client := &fakePrivateDataClient{
		suiteJSON:  []byte(minimalSuiteJSON), // carries "upload_time": 1710000000
		uploadTime: "1710000000",
	}
	provider := NewToolboxSnapshotProvider(
		&fakeBindingLookup{
			bindings: map[string]*accountdata.ResolvedBinding{
				"jp": {PJSKUserID: "123456789", Server: "jp", SuiteVisible: true},
			},
		},
		client,
		nil,
		nil,
	).WithPrivateDataCache(NewPrivateDataCache())

	selector := Selector{IMPlatform: "qq", IMUserID: "10001", Region: renderregion.JP}

	// First request is cold: one full suite fetch, no upload_time probe.
	if _, err := provider.Resolve(WithRequestCache(context.Background()), selector, ResolveOptions{}); err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}
	if got := len(client.suiteCalls); got != 1 {
		t.Fatalf("cold request: suite fetches = %d, want 1", got)
	}
	if client.suiteUploadTimeCalls != 0 {
		t.Fatalf("cold request must not probe upload_time, got %d", client.suiteUploadTimeCalls)
	}

	// Second, independent request: upload_time probe hits the shared cache, no new full fetch.
	if _, err := provider.Resolve(WithRequestCache(context.Background()), selector, ResolveOptions{}); err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}
	if got := len(client.suiteCalls); got != 1 {
		t.Fatalf("warm request: suite fetches = %d, want 1 (served from cross-request cache)", got)
	}
	if client.suiteUploadTimeCalls != 1 {
		t.Fatalf("warm request: upload_time probes = %d, want 1", client.suiteUploadTimeCalls)
	}
}
