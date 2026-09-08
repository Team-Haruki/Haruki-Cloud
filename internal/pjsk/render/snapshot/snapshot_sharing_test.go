package snapshot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/accountdata"
	renderregion "haruki-cloud/internal/pjsk/region"
)

// The signal fires when getOrBuild has registered its flight and starts waiting.
type snapshotWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *snapshotWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func awaitSnapshotSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for snapshot work")
	}
}

type concurrentSnapshotBindings struct{}

func (concurrentSnapshotBindings) ResolveUserBinding(_ context.Context, _, _, server string) (int, *accountdata.ResolvedBinding, error) {
	return 1, &accountdata.ResolvedBinding{PJSKUserID: "123456789", Server: server, SuiteVisible: true, MySekaiVisible: true}, nil
}

func (concurrentSnapshotBindings) List(context.Context, string, string) ([]accountdata.BindingListItem, error) {
	return nil, nil
}

type authorizedSnapshotClient struct {
	calls atomic.Int32
}

func (c *authorizedSnapshotClient) GetSuiteDataConditionalContext(_ context.Context, _ string, _ int64, _, requester string, known int64) ([]byte, bool, error) {
	c.calls.Add(1)
	if requester == "revoked" {
		return nil, false, errSnapshotAccessRevoked
	}
	if known == 1710000000 {
		return nil, true, nil
	}
	return []byte(minimalSuiteJSON), false, nil
}

func (c *authorizedSnapshotClient) GetMySekaiDataConditionalContext(context.Context, string, int64, string, string, int64) ([]byte, bool, error) {
	return []byte(`{"upload_time":1710000000,"updatedResources":{}}`), false, nil
}

var errSnapshotAccessRevoked = errors.New("access revoked")

func TestSnapshotConcurrentBuildKeepsPerRequestAuthorization(t *testing.T) {
	client := &authorizedSnapshotClient{}
	provider := NewToolboxSnapshotProvider(concurrentSnapshotBindings{}, client, nil, nil).
		WithPrivateDataCache(NewPrivateDataCache()).
		WithBuiltSnapshotCache(NewBuiltSnapshotCache())
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	var builds atomic.Int32
	provider.factory = snapshotFactoryFunc(func(ctx context.Context, input BuildInput) (Snapshot, error) {
		builds.Add(1)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return NewDefaultSnapshotFactory(nil, nil).Build(ctx, input)
	})

	const count = 16
	type outcome struct {
		snapshot Snapshot
		err      error
	}
	results := make(chan outcome, count)
	for i := range count {
		base, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx := &snapshotWaitContext{Context: WithRequestCache(base), waiting: make(chan struct{})}
		go func() {
			selector := validToolboxSelector()
			selector.IMUserID = fmt.Sprint(i)
			snapshot, err := provider.Resolve(ctx, selector, ResolveOptions{})
			results <- outcome{snapshot, err}
		}()
		awaitSnapshotSignal(t, ctx.waiting)
	}
	if got := client.calls.Load(); got != count {
		t.Fatalf("authorized reads = %d, want %d", got, count)
	}
	unblock()
	var first Snapshot
	for range count {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if first == nil {
			first = result.snapshot
		} else if first != result.snapshot {
			t.Fatal("same-version waiters received different snapshot instances")
		}
	}
	if builds.Load() != 1 {
		t.Fatalf("builds = %d, want 1", builds.Load())
	}
	owned, err := first.RawBytes()
	if err != nil {
		t.Fatal(err)
	}
	owned[0] = 'x'
	fresh, _ := first.RawBytes()
	if fresh[0] == 'x' {
		t.Fatal("raw byte mutation escaped caller boundary")
	}
	profile := first.DetailedProfile(renderregion.JP)
	profile.Nickname = "changed"
	if first.DetailedProfile(renderregion.JP).Nickname == "changed" {
		t.Fatal("profile mutation escaped caller boundary")
	}
	selector := validToolboxSelector()
	selector.IMUserID = "revoked"
	if snapshot, err := provider.Resolve(WithRequestCache(context.Background()), selector, ResolveOptions{}); snapshot != nil || !errors.Is(err, errSnapshotAccessRevoked) {
		t.Fatalf("revoked warm read = %v, %v", snapshot, err)
	}
	if builds.Load() != 1 || client.calls.Load() != count+1 {
		t.Fatal("revoked read reused authorization or reached the builder")
	}
}

func TestSnapshotBuildCancellationDoesNotCancelOtherWaiters(t *testing.T) {
	cache := NewBuiltSnapshotCache()
	key := builtKey(1, 100)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	expected := buildTestSnapshot(t)
	var builds atomic.Int32
	build := func(ctx context.Context) (Snapshot, error) {
		builds.Add(1)
		close(started)
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > sharedSnapshotBuildTimeout {
			return nil, errors.New("shared build has no bounded deadline")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
		}
		commandtrace.RecordOperation(ctx, "snapshot.build", time.Millisecond)
		return expected, nil
	}
	leaderCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	leaderResult := make(chan error, 1)
	go func() {
		_, _, err := cache.getOrBuild(leaderCtx, key, 100, build)
		leaderResult <- err
	}()
	awaitSnapshotSignal(t, started)
	traceCtx, trace := commandtrace.WithTrace(context.Background())
	waiter := &snapshotWaitContext{Context: traceCtx, waiting: make(chan struct{})}
	waiterResult := make(chan error, 1)
	go func() {
		got, _, err := cache.getOrBuild(waiter, key, 100, build)
		if err == nil && got != expected {
			err = errors.New("unexpected snapshot")
		}
		waiterResult <- err
	}()
	awaitSnapshotSignal(t, waiter.waiting)
	cancel()
	select {
	case err := <-leaderResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled leader = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled leader did not return independently")
	}
	unblock()
	if err := <-waiterResult; err != nil {
		t.Fatal(err)
	}
	if builds.Load() != 1 || cache.Get(key) != expected {
		t.Fatal("cancellation interrupted or duplicated the shared build")
	}
	found := false
	for _, operation := range trace.Snapshot().Operations {
		if operation.Name == "snapshot.build" && operation.Count == 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("waiter did not receive shared build tracing")
	}
}

func TestSnapshotBuildRetryAndKeyIsolation(t *testing.T) {
	cache := NewBuiltSnapshotCache()
	key := builtKey(1, 100)
	boom := errors.New("build failed")
	if _, _, err := cache.getOrBuild(context.Background(), key, 1, func(context.Context) (Snapshot, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("build error = %v", err)
	}
	if cache.Get(key) != nil {
		t.Fatal("failed build was cached")
	}
	keys := []builtSnapshotKey{
		key,
		builtKey(2, 100),
		builtKey(1, 101),
		{Region: "en", UID: 1, SuiteUploadTime: 100},
		{Region: "jp", UID: 1, SuiteUploadTime: 100, NeedMySekai: true, MySekaiUploadTime: 200},
		{Region: "jp", UID: 1, SuiteUploadTime: 100, NeedMySekai: true, MySekaiUploadTime: 201},
	}
	for _, key := range keys {
		expected := buildTestSnapshot(t)
		got, hit, err := cache.getOrBuild(context.Background(), key, 1, func(context.Context) (Snapshot, error) { return expected, nil })
		if err != nil || hit || got != expected {
			t.Fatalf("key %+v reused a different build: hit=%v err=%v", key, hit, err)
		}
		if got, hit, err = cache.getOrBuild(context.Background(), key, 1, func(context.Context) (Snapshot, error) {
			t.Error("warm hit rebuilt")
			return nil, nil
		}); err != nil || !hit || got != expected {
			t.Fatalf("warm hit = %v, %v", hit, err)
		}
	}
}

func TestPrivatePayloadSharingKeepsOwnedBoundariesAndVersions(t *testing.T) {
	cache := NewPrivateDataCache()
	source := []byte(`{"upload_time":100,"data":"original"}`)
	expected := bytes.Clone(source)
	first, _, err := cache.fetchPayload(suiteKey(), func(int64) ([]byte, bool, error) { return source, false, nil })
	if err != nil {
		t.Fatal(err)
	}
	source[0] = 'x'
	if !bytes.Equal(first.data, expected) || first.uploadTime != 100 {
		t.Fatal("ingestion did not isolate upstream bytes and retain the version")
	}
	warm, hit, err := cache.fetchPayload(suiteKey(), func(known int64) ([]byte, bool, error) {
		if known != 100 {
			t.Fatalf("known version = %d", known)
		}
		return nil, true, nil
	})
	if err != nil || !hit || &warm.data[0] != &first.data[0] {
		t.Fatal("warm payload was not shared")
	}
	owned := warm.cloneBytes()
	owned[0] = 'x'
	if !bytes.Equal(first.data, expected) {
		t.Fatal("owned bytes mutated the shared payload")
	}
	_, _, err = cache.fetchPayload(suiteKey(), func(int64) ([]byte, bool, error) {
		return []byte(`{"upload_time":101}`), false, nil
	})
	if err != nil || !bytes.Equal(first.data, expected) || first.uploadTime != 100 {
		t.Fatal("replacement mutated a previously captured payload")
	}

	ctx := WithRequestCache(context.Background())
	key := privateDataCacheKey{Server: "jp", DataType: "suite", UserID: 1, Platform: "qq", PlatformUserID: "one"}
	calls := 0
	fetch := func() (privateDataPayload, error) { calls++; return warm, nil }
	a, _, _ := cachedPrivateData(ctx, key, fetch)
	b, _, hit := cachedPrivateData(ctx, key, fetch)
	if calls != 1 || !hit || &a.data[0] != &b.data[0] || &a.data[0] != &warm.data[0] {
		t.Fatal("request cache copied or refetched immutable data")
	}
	key.PlatformUserID = "two"
	if _, _, hit := cachedPrivateData(ctx, key, fetch); hit || calls != 2 {
		t.Fatal("request cache shared authorization between requesters")
	}
}

func TestBuiltSnapshotFlightAcceptsNilContext(t *testing.T) {
	for _, cache := range []*BuiltSnapshotCache{nil, NewBuiltSnapshotCache()} {
		expected := buildTestSnapshot(t)
		//lint:ignore SA1012 Exercise the legacy nil-context compatibility boundary.
		got, _, err := cache.getOrBuild(nil, builtKey(1, 100), 1, func(ctx context.Context) (Snapshot, error) {
			if ctx == nil {
				t.Error("build received nil context")
			}
			return expected, nil
		})
		if err != nil || got != expected {
			t.Fatalf("snapshot=%v err=%v", got, err)
		}
	}
}
