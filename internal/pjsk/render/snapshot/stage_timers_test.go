package snapshot

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"
)

func snapshotTimerCounts(t *testing.T, trace *commandtrace.Trace) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, operation := range trace.Snapshot().Operations {
		counts[operation.Name] = operation.Count
	}
	raw, err := json.Marshal(trace.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private_projection", "private_payload_value", "private_requester", "339871638031728641"} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("trace contains private content: %s", raw)
		}
	}
	return counts
}

func TestPrivateDataTimersDistinguish200304AndBypass(t *testing.T) {
	cache := NewPrivateDataCache()
	key := PrivateDataKey{Server: "jp", DataType: "suite", UID: 339871638031728641, Projection: "private_projection"}
	payload := []byte(`{"upload_time":1710000000,"value":"private_payload_value"}`)
	for _, tc := range []struct {
		name      string
		cache     *PrivateDataCache
		unchanged bool
		want      map[string]int
	}{
		{"cold", cache, false, map[string]int{"snapshot.raw_cache_lookup": 1, "snapshot.raw_cache_miss": 1, "snapshot.payload_stamp": 1, "snapshot.payload_copy": 1}},
		{"validated", cache, true, map[string]int{"snapshot.raw_cache_lookup": 1, "snapshot.raw_cache_hit": 1}},
		{"changed", cache, false, map[string]int{"snapshot.raw_cache_lookup": 1, "snapshot.raw_cache_miss": 1, "snapshot.payload_stamp": 1, "snapshot.payload_copy": 1}},
		{"bypass", nil, false, map[string]int{"snapshot.raw_cache_bypass": 1, "snapshot.payload_stamp": 1, "snapshot.payload_copy": 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, trace := commandtrace.WithTrace(t.Context())
			_, hit, err := tc.cache.fetchPayloadContext(ctx, key, func(int64) ([]byte, bool, error) { return payload, tc.unchanged, nil })
			if err != nil || hit != tc.unchanged {
				t.Fatalf("fetch = %t, %v", hit, err)
			}
			if got := snapshotTimerCounts(t, trace); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("operations=%v want=%v", got, tc.want)
			}
		})
	}
	ctx, trace := commandtrace.WithTrace(t.Context())
	if _, _, err := cache.fetchPayloadContext(ctx, key, func(int64) ([]byte, bool, error) { return nil, false, context.Canceled }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if got := snapshotTimerCounts(t, trace); !reflect.DeepEqual(got, map[string]int{"snapshot.raw_cache_lookup": 1}) {
		t.Fatalf("canceled fetch unexpectedly prepared/reused data: %v", got)
	}
}

func TestDirectMySekaiTimersCoverBindingFetchCopyAndFailure(t *testing.T) {
	client := &fakePrivateDataClient{mysekaiJSON: []byte(`{"upload_time":1710000000,"value":"private_payload_value"}`), uploadTime: "1710000000"}
	bindings := newValidToolboxProvider(client).bindings
	provider := NewToolboxMySekaiPayloadProvider(bindings, client).WithPrivateDataCache(NewPrivateDataCache())
	ctx, trace := commandtrace.WithTrace(t.Context())
	if _, err := provider.Resolve(ctx, validToolboxSelector(), false); err != nil {
		t.Fatal(err)
	}
	counts := snapshotTimerCounts(t, trace)
	for name, want := range map[string]int{"snapshot.binding": 1, "snapshot.private_data": 1, "snapshot.payload_stamp": 1, "snapshot.payload_copy": 2, "snapshot.raw_cache_miss": 1} {
		if counts[name] != want {
			t.Fatalf("%s=%d want=%d", name, counts[name], want)
		}
	}
	client.mysekaiErr = context.Canceled
	ctx, trace = commandtrace.WithTrace(t.Context())
	if _, err := provider.Resolve(ctx, validToolboxSelector(), false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	counts = snapshotTimerCounts(t, trace)
	if counts["snapshot.binding"] != 1 || counts["snapshot.private_data"] != 1 || counts["snapshot.payload_copy"] != 0 {
		t.Fatalf("failed fetch timers=%v", counts)
	}
}

func TestRequestSnapshotCacheTimersKeepCanceledWaitSemantics(t *testing.T) {
	base := WithRequestCache(t.Context())
	key := privateDataCacheKey{Server: "jp", DataType: "suite", PlatformUserID: "private_requester"}
	leaderCtx, _ := commandtrace.WithTrace(base)
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = cachedPrivateData(leaderCtx, key, func() (privateDataPayload, error) {
			close(started)
			<-release
			return privateDataPayload{data: []byte(`{}`)}, nil
		})
	}()
	awaitSnapshotSignal(t, started)
	canceled, cancel := context.WithCancel(base)
	cancel()
	followerCtx, trace := commandtrace.WithTrace(canceled)
	result := make(chan error, 1)
	go func() {
		_, err, hit := cachedPrivateData(followerCtx, key, func() (privateDataPayload, error) { return privateDataPayload{}, errors.New("unexpected fetch") })
		if !hit && err == nil {
			err = errors.New("expected request cache hit")
		}
		result <- err
	}()
	close(release)
	if err := <-result; err != nil {
		t.Fatalf("existing synchronous cached result changed: %v", err)
	}
	<-done
	counts := snapshotTimerCounts(t, trace)
	for _, name := range []string{"snapshot.request_cache_hit", "snapshot.cache_wait", "snapshot.cache_wait_canceled"} {
		if counts[name] != 1 {
			t.Fatalf("timers=%v", counts)
		}
	}
}

func TestBuiltSnapshotTimersDoNotMergeAfterCancellation(t *testing.T) {
	cache := NewBuiltSnapshotCache()
	key := builtKey(1, 100)
	expected := buildTestSnapshot(t)
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	base, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx, trace := commandtrace.WithTrace(base)
	result := make(chan error, 1)
	go func() {
		_, _, err := cache.getOrBuild(ctx, key, 100, func(shared context.Context) (Snapshot, error) {
			close(started)
			<-release
			commandtrace.RecordOperation(shared, "snapshot.test_shared_work", time.Millisecond)
			close(finished)
			return expected, nil
		})
		result <- err
	}()
	awaitSnapshotSignal(t, started)
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
	before := trace.Snapshot()
	counts := snapshotTimerCounts(t, trace)
	for _, name := range []string{"snapshot.built_cache_miss", "snapshot.build_wait", "snapshot.build_wait_canceled"} {
		if counts[name] != 1 {
			t.Fatalf("timers=%v", counts)
		}
	}
	close(release)
	awaitSnapshotSignal(t, finished)
	if after := trace.Snapshot(); !reflect.DeepEqual(before, after) {
		t.Fatalf("completed shared work changed canceled caller trace")
	}
}

func TestBuiltSnapshotTimersDistinguishHitAndBypass(t *testing.T) {
	cache := NewBuiltSnapshotCache()
	key := builtKey(1, 100)
	expected := buildTestSnapshot(t)
	cache.Put(key, expected, 100)
	for _, tc := range []struct {
		cache  *BuiltSnapshotCache
		metric string
	}{{cache, "snapshot.built_cache_hit"}, {nil, "snapshot.built_cache_bypass"}} {
		ctx, trace := commandtrace.WithTrace(t.Context())
		_, _, err := tc.cache.getOrBuild(ctx, key, 100, func(context.Context) (Snapshot, error) { return expected, nil })
		if err != nil {
			t.Fatal(err)
		}
		if got := snapshotTimerCounts(t, trace); !reflect.DeepEqual(got, map[string]int{tc.metric: 1}) {
			t.Fatalf("timers=%v", got)
		}
	}
}
