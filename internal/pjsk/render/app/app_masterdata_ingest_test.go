package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"
)

// fakeIngestVersion stands in for the database provider's ingest version
// row: unknown until a hash is set.
type fakeIngestVersion struct {
	mu    sync.Mutex
	hash  string
	known bool
	reads int
}

func (v *fakeIngestVersion) set(hash string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.hash, v.known = hash, hash != ""
}

func (v *fakeIngestVersion) MasterdataIngestHash(context.Context) (string, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.reads++
	return v.hash, v.known
}

func loadedIngestHash(state *registryMasterdataRefreshState, region renderregion.Value) (string, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	hash, ok := state.loaded[region]
	return hash, ok
}

func newIngestRegistryTestState(t *testing.T, registry *fakeMasterdataRegistry, delays []time.Duration, regions ...renderregion.Value) (*registryMasterdataRefreshState, map[renderregion.Value]*atomicMasterdataResetter, map[renderregion.Value]*fakeIngestVersion) {
	t.Helper()
	state, resetters, _, _ := newRegistryTestState(t, registry, delays, regions...)
	versions := make(map[renderregion.Value]*fakeIngestVersion, len(regions))
	state.versioners = make(map[renderregion.Value]masterdataIngestVersioner, len(regions))
	for _, region := range regions {
		version := &fakeIngestVersion{}
		versions[region] = version
		state.versioners[region] = version
	}
	return state, resetters, versions
}

func TestRegistryMasterdataRefreshSkipsResetsWhenIngestHashUnchanged(t *testing.T) {
	registry := &fakeMasterdataRegistry{hashes: map[string]string{"jp": "sha256:jp-1"}}
	state, resetters, versions := newIngestRegistryTestState(t, registry, []time.Duration{20 * time.Millisecond, 40 * time.Millisecond}, renderregion.JP)
	jp, version := resetters[renderregion.JP], versions[renderregion.JP]
	ctx := context.Background()

	// Startup: the committed hash is recorded as what the caches hold.
	version.set("sha256:jp-1")
	state.poll(ctx)
	if loaded, _ := loadedIngestHash(state, renderregion.JP); jp.resets() != 0 || loaded != "sha256:jp-1" {
		t.Fatalf("first observation must only record: resets=%d loaded=%q", jp.resets(), loaded)
	}

	// The registry moves before the ingest lands: nothing to reload yet.
	registry.set("jp", "sha256:jp-2")
	state.poll(ctx)
	if jp.resets() != 0 {
		t.Fatalf("registry change with an unchanged ingest hash must not reset, got %d", jp.resets())
	}
	time.Sleep(70 * time.Millisecond)
	if jp.resets() != 0 {
		t.Fatalf("settle follow-ups with an unchanged ingest hash must not reset, got %d", jp.resets())
	}

	// The ingest lands: the next poll reloads once, and the follow-ups of
	// a later registry change find the caches already at that hash.
	version.set("sha256:jp-2")
	state.poll(ctx)
	if loaded, _ := loadedIngestHash(state, renderregion.JP); jp.resets() != 1 || loaded != "sha256:jp-2" {
		t.Fatalf("ingest change must reset once: resets=%d loaded=%q", jp.resets(), loaded)
	}
	state.poll(ctx)
	if jp.resets() != 1 {
		t.Fatalf("unchanged ingest hash must not reset again, got %d", jp.resets())
	}
}

func TestRegistryMasterdataRefreshResetsOnceWhenIngestLandedFirst(t *testing.T) {
	registry := &fakeMasterdataRegistry{hashes: map[string]string{"jp": "sha256:jp-1"}}
	state, resetters, versions := newIngestRegistryTestState(t, registry, []time.Duration{20 * time.Millisecond, 40 * time.Millisecond}, renderregion.JP)
	jp, version := resetters[renderregion.JP], versions[renderregion.JP]
	ctx := context.Background()

	version.set("sha256:jp-1")
	state.poll(ctx)
	// Ingest and registry both moved between two polls: one reload serves
	// the immediate reset and both settle follow-ups.
	version.set("sha256:jp-2")
	registry.set("jp", "sha256:jp-2")
	state.poll(ctx)
	if jp.resets() != 1 {
		t.Fatalf("one reset expected, got %d", jp.resets())
	}
	time.Sleep(70 * time.Millisecond)
	if jp.resets() != 1 {
		t.Fatalf("settle follow-ups must skip an unchanged ingest hash, got %d resets", jp.resets())
	}
}

func TestRegistryMasterdataRefreshKeepsTimeDrivenResetsWhenHashUnknown(t *testing.T) {
	registry := &fakeMasterdataRegistry{hashes: map[string]string{"jp": "sha256:jp-1"}}
	state, resetters, versions := newIngestRegistryTestState(t, registry, []time.Duration{20 * time.Millisecond, 40 * time.Millisecond}, renderregion.JP)
	jp, version := resetters[renderregion.JP], versions[renderregion.JP]
	ctx := context.Background()

	state.poll(ctx)
	registry.set("jp", "sha256:jp-2")
	state.poll(ctx)
	if jp.resets() != 1 {
		t.Fatalf("unknown hash must reset right away, got %d", jp.resets())
	}
	waitForResets(t, jp, 3)
	if loaded, ok := loadedIngestHash(state, renderregion.JP); !ok || loaded != "" {
		t.Fatalf("a reset at an unknown hash must be recorded as unknown: %q, %v", loaded, ok)
	}
	// Once the ingest reports a hash, the unknown reset is superseded by
	// one more reload, after which the hash is tracked.
	version.set("sha256:jp-2")
	state.poll(ctx)
	if loaded, _ := loadedIngestHash(state, renderregion.JP); jp.resets() != 4 || loaded != "sha256:jp-2" {
		t.Fatalf("first known hash after an unknown reset must reload: resets=%d loaded=%q", jp.resets(), loaded)
	}
	state.poll(ctx)
	if jp.resets() != 4 {
		t.Fatalf("tracked hash must not reset again, got %d", jp.resets())
	}
}

func TestRegistryMasterdataRefreshWarmsResetRegions(t *testing.T) {
	registry := &fakeMasterdataRegistry{hashes: map[string]string{"jp": "sha256:jp-1", "cn": "sha256:cn-1"}}
	state, resetters, _, _ := newRegistryTestState(t, registry, nil, renderregion.JP, renderregion.CN)
	var mu sync.Mutex
	var warmed []renderregion.Value
	state.warm = func(region renderregion.Value) {
		mu.Lock()
		defer mu.Unlock()
		warmed = append(warmed, region)
	}
	ctx := context.Background()
	state.poll(ctx)
	registry.set("jp", "sha256:jp-2")
	state.poll(ctx)
	mu.Lock()
	defer mu.Unlock()
	if resetters[renderregion.JP].resets() != 1 || len(warmed) != 1 || warmed[0] != renderregion.JP {
		t.Fatalf("reset region must be warmed: warmed=%v", warmed)
	}
}

// fakeWarmProvider is a region provider whose only behaviour is the
// warm-up; the embedded nil interface satisfies MasterDataProvider.
type fakeWarmProvider struct {
	provider.MasterDataProvider
	mu      sync.Mutex
	started chan struct{}
	release chan struct{}
	calls   int
	err     error
}

func (p *fakeWarmProvider) WarmMasterdata(ctx context.Context) error {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	if p.started != nil {
		p.started <- struct{}{}
	}
	if p.release != nil {
		select {
		case <-p.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return p.err
}

func (p *fakeWarmProvider) warmCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestWarmMasterdataIndexesBoundsConcurrencyAndCoversEveryRegion(t *testing.T) {
	started := make(chan struct{}, 8)
	release := make(chan struct{})
	providers := map[renderregion.Value]provider.MasterDataProvider{}
	fakes := map[renderregion.Value]*fakeWarmProvider{}
	for _, region := range []renderregion.Value{renderregion.JP, renderregion.CN, renderregion.TW, renderregion.KR, renderregion.EN} {
		fake := &fakeWarmProvider{started: started, release: release}
		fakes[region] = fake
		providers[region] = fake
	}
	masterDir := t.TempDir()
	providers[renderregion.Unknown] = provider.NewLocalProvider(masterDir, renderregion.JP)
	app := &App{Providers: providers}
	done := app.WarmMasterdataIndexes(context.Background())

	for range masterdataWarmConcurrency {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("warm-up did not start")
		}
	}
	select {
	case <-started:
		t.Fatalf("more than %d regions warmed at once", masterdataWarmConcurrency)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("warm-up did not finish")
	}
	for region, fake := range fakes {
		if fake.warmCalls() != 1 {
			t.Fatalf("%s warmed %d times, want 1", region, fake.warmCalls())
		}
	}
}

func TestWarmMasterdataIndexesToleratesFailuresAndNilApp(t *testing.T) {
	select {
	case <-(*App)(nil).WarmMasterdataIndexes(context.Background()):
	default:
		t.Fatal("nil app must report completion at once")
	}
	select {
	case <-(&App{}).WarmMasterdataIndexes(context.TODO()):
	default:
		t.Fatal("app without providers must report completion at once")
	}
	failing := &fakeWarmProvider{err: errors.New("database unavailable")}
	app := &App{Providers: map[renderregion.Value]provider.MasterDataProvider{renderregion.JP: failing}}
	select {
	case <-app.WarmMasterdataIndexes(context.Background()):
	case <-time.After(5 * time.Second):
		t.Fatal("failed warm-up did not finish")
	}
	if failing.warmCalls() != 1 {
		t.Fatalf("warm calls = %d", failing.warmCalls())
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	app.masterdataWarm = newMasterdataWarmup()
	if app.warmLimiter() != app.masterdataWarm {
		t.Fatal("app must reuse its own limiter")
	}
	if app.masterdataWarm.run(ctx, renderregion.JP, failing) {
		t.Fatal("a cancelled context must not run the warm-up")
	}
	if failing.warmCalls() != 1 {
		t.Fatalf("cancelled warm-up still ran: calls=%d", failing.warmCalls())
	}
	if (&masterdataWarmup{}).run(context.Background(), renderregion.JP, failing) || newMasterdataWarmup().run(context.Background(), renderregion.JP, nil) || (*masterdataWarmup)(nil).run(context.Background(), renderregion.JP, failing) {
		t.Fatal("nil warmer, slots or limiter must be a no-op")
	}
	if failing.warmCalls() != 1 {
		t.Fatalf("no-op limiters still ran the warm-up: calls=%d", failing.warmCalls())
	}
}

func TestWarmRegionAsyncReloadsOneRegion(t *testing.T) {
	fake := &fakeWarmProvider{started: make(chan struct{}, 1)}
	app := &App{Providers: map[renderregion.Value]provider.MasterDataProvider{renderregion.JP: fake}}
	app.warmRegionAsync(context.TODO(), renderregion.CN)
	app.warmRegionAsync(context.TODO(), renderregion.JP)
	(*App)(nil).warmRegionAsync(context.Background(), renderregion.JP)
	select {
	case <-fake.started:
	case <-time.After(5 * time.Second):
		t.Fatal("region warm-up did not start")
	}
}
