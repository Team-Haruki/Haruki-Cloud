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

func TestPrivateDataCacheColdMissFetchesWithoutKnownUploadTime(t *testing.T) {
	c := NewPrivateDataCache()
	payload := []byte(`{"upload_time":1710000000,"x":1}`)
	var knownTimes []int64

	data, cross, err := c.Fetch(suiteKey(), func(known int64) ([]byte, bool, error) {
		knownTimes = append(knownTimes, known)
		return payload, false, nil
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if cross {
		t.Fatalf("cold miss must not report a cross-request hit")
	}
	if len(knownTimes) != 1 || knownTimes[0] != 0 {
		t.Fatalf("cold miss must fetch once with known=0, got %v", knownTimes)
	}
	if string(data) != string(payload) {
		t.Fatalf("unexpected data %q", data)
	}
}

func TestPrivateDataCacheServesOnNotModified(t *testing.T) {
	c := NewPrivateDataCache()
	payload := []byte(`{"upload_time":1710000000}`)

	if _, _, err := c.Fetch(suiteKey(), func(int64) ([]byte, bool, error) { return payload, false, nil }); err != nil {
		t.Fatalf("cold Fetch() error = %v", err)
	}

	var knownTimes []int64
	data, cross, err := c.Fetch(suiteKey(), func(known int64) ([]byte, bool, error) {
		knownTimes = append(knownTimes, known)
		return nil, true, nil
	})
	if err != nil {
		t.Fatalf("warm Fetch() error = %v", err)
	}
	if !cross {
		t.Fatalf("not-modified must serve from cache")
	}
	if len(knownTimes) != 1 || knownTimes[0] != 1710000000 {
		t.Fatalf("warm fetch must carry the cached upload_time, got %v", knownTimes)
	}
	if string(data) != string(payload) {
		t.Fatalf("warm hit returned %q, want cached payload", data)
	}
}

func TestPrivateDataCacheRefetchesOnChangedUploadTime(t *testing.T) {
	c := NewPrivateDataCache()
	first := []byte(`{"upload_time":1710000000,"v":1}`)
	second := []byte(`{"upload_time":1710000500,"v":2}`)

	if _, _, err := c.Fetch(suiteKey(), func(int64) ([]byte, bool, error) { return first, false, nil }); err != nil {
		t.Fatalf("cold Fetch() error = %v", err)
	}

	fetchCalls := 0
	data, cross, err := c.Fetch(suiteKey(), func(known int64) ([]byte, bool, error) {
		fetchCalls++
		if known != 1710000000 {
			t.Fatalf("stale fetch must carry the previous upload_time, got %d", known)
		}
		return second, false, nil
	})
	if err != nil {
		t.Fatalf("stale Fetch() error = %v", err)
	}
	if cross {
		t.Fatalf("changed upload_time must not report a cache hit")
	}
	if fetchCalls != 1 {
		t.Fatalf("changed upload_time must fetch once, got %d", fetchCalls)
	}
	if string(data) != string(second) {
		t.Fatalf("stale Fetch returned %q, want refreshed payload", data)
	}

	// The refreshed payload is now cached under the new upload_time.
	data, cross, err = c.Fetch(suiteKey(), func(known int64) ([]byte, bool, error) {
		if known != 1710000500 {
			t.Fatalf("post-refresh fetch must carry the new upload_time, got %d", known)
		}
		return nil, true, nil
	})
	if err != nil || !cross || string(data) != string(second) {
		t.Fatalf("post-refresh hit failed: cross=%v err=%v data=%q", cross, err, data)
	}
}

func TestPrivateDataCacheNeverServesOnFetchError(t *testing.T) {
	c := NewPrivateDataCache()
	payload := []byte(`{"upload_time":1710000000}`)
	if _, _, err := c.Fetch(suiteKey(), func(int64) ([]byte, bool, error) { return payload, false, nil }); err != nil {
		t.Fatalf("cold Fetch() error = %v", err)
	}

	boom := errors.New("invalid platform or platform_user_id")
	data, cross, err := c.Fetch(suiteKey(), func(int64) ([]byte, bool, error) { return nil, false, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("fetch error must propagate, got %v", err)
	}
	if cross || data != nil {
		t.Fatalf("fetch error must not serve cached data (cross=%v, data=%q)", cross, data)
	}
}

func TestPrivateDataCacheRejectsNotModifiedWithoutEntry(t *testing.T) {
	c := NewPrivateDataCache()
	_, cross, err := c.Fetch(suiteKey(), func(known int64) ([]byte, bool, error) {
		if known != 0 {
			t.Fatalf("cold fetch must carry known=0, got %d", known)
		}
		return nil, true, nil
	})
	if err == nil {
		t.Fatal("not-modified without a cached payload must be an error")
	}
	if cross {
		t.Fatal("protocol violation must not report a cache hit")
	}
}

func TestPrivateDataCacheSkipsCachingWithoutUploadTime(t *testing.T) {
	c := NewPrivateDataCache()
	payload := []byte(`{"no_upload_time":1}`)
	var knownTimes []int64
	fetch := func(known int64) ([]byte, bool, error) {
		knownTimes = append(knownTimes, known)
		return payload, false, nil
	}

	if _, _, err := c.Fetch(suiteKey(), fetch); err != nil {
		t.Fatalf("first Fetch() error = %v", err)
	}
	if _, cross, err := c.Fetch(suiteKey(), fetch); err != nil || cross {
		t.Fatalf("payload without upload_time must not be cached (cross=%v, err=%v)", cross, err)
	}
	if len(knownTimes) != 2 || knownTimes[0] != 0 || knownTimes[1] != 0 {
		t.Fatalf("an uncacheable payload must keep fetching with known=0, got %v", knownTimes)
	}
}

func TestPrivateDataCacheNilReceiverBypasses(t *testing.T) {
	var c *PrivateDataCache
	payload := []byte(`{"upload_time":1710000000}`)
	data, cross, err := c.Fetch(suiteKey(), func(known int64) ([]byte, bool, error) {
		if known != 0 {
			t.Fatalf("nil cache must fetch with known=0, got %d", known)
		}
		return payload, false, nil
	})
	if err != nil || cross || string(data) != string(payload) {
		t.Fatalf("nil receiver must bypass to a plain fetch (cross=%v, err=%v, data=%q)", cross, err, data)
	}
}

func TestPrivateDataCacheServedDataIsIsolatedFromCache(t *testing.T) {
	c := NewPrivateDataCache()
	payload := []byte(`{"upload_time":1710000000}`)
	if _, _, err := c.Fetch(suiteKey(), func(int64) ([]byte, bool, error) { return payload, false, nil }); err != nil {
		t.Fatalf("cold Fetch() error = %v", err)
	}
	notModified := func(int64) ([]byte, bool, error) { return nil, true, nil }
	first, _, err := c.Fetch(suiteKey(), notModified)
	if err != nil {
		t.Fatalf("warm Fetch() error = %v", err)
	}
	for i := range first {
		first[i] = 'X'
	}
	second, _, err := c.Fetch(suiteKey(), notModified)
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
		if _, _, err := c.Fetch(key, func(int64) ([]byte, bool, error) { return []byte(`{"upload_time":1}`), false, nil }); err != nil {
			t.Fatalf("store uid %d error = %v", uid, err)
		}
	}
	store(1)
	store(2)
	store(3) // evicts uid 1 (least recently used)

	_, cross, err := c.Fetch(PrivateDataKey{Server: "jp", DataType: "suite", UID: 1},
		func(known int64) ([]byte, bool, error) {
			if known != 0 {
				t.Fatalf("evicted entry must be a cold miss, got known=%d", known)
			}
			return []byte(`{"upload_time":1}`), false, nil
		},
	)
	if err != nil {
		t.Fatalf("Fetch(evicted) error = %v", err)
	}
	if cross {
		t.Fatalf("evicted entry must not report a cache hit")
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
			data, _, err := c.Fetch(suiteKey(), func(known int64) ([]byte, bool, error) {
				if known == 1710000000 {
					return nil, true, nil
				}
				return payload, false, nil
			})
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

	// First request is cold: one full suite fetch carrying no known upload_time.
	if _, err := provider.Resolve(WithRequestCache(context.Background()), selector, ResolveOptions{}); err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}
	if got := len(client.suiteCalls); got != 1 {
		t.Fatalf("cold request: suite fetches = %d, want 1", got)
	}
	if got := client.suiteKnownTimes; len(got) != 1 || got[0] != 0 {
		t.Fatalf("cold request must not carry a known upload_time, got %v", got)
	}

	// Second, independent request validates the cached upload_time upstream and
	// is answered not-modified: no new payload transfer.
	if _, err := provider.Resolve(WithRequestCache(context.Background()), selector, ResolveOptions{}); err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}
	if got := len(client.suiteCalls); got != 1 {
		t.Fatalf("warm request: suite fetches = %d, want 1 (served from cross-request cache)", got)
	}
	if client.suiteNotModified != 1 {
		t.Fatalf("warm request: not-modified answers = %d, want 1", client.suiteNotModified)
	}
	if got := client.suiteKnownTimes; len(got) != 2 || got[1] != 1710000000 {
		t.Fatalf("warm request must carry the cached upload_time, got %v", got)
	}
}
