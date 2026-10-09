package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/config"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/accountdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	renderprofile "haruki-cloud/internal/pjsk/render/profile"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/usererror"
)

const warmTestOverlapWait = 2 * time.Second

// warmOverlapProbe lets a snapshot resolve and a profile fetch each record
// whether the other one was in flight at the same time. A serial caller makes
// both sides time out instead of deadlocking.
type warmOverlapProbe struct {
	snapshotStarted chan struct{}
	profileStarted  chan struct{}
	snapshotOnce    sync.Once
	profileOnce     sync.Once
	snapshotSawPeer atomic.Bool
	profileSawPeer  atomic.Bool
}

func newWarmOverlapProbe() *warmOverlapProbe {
	return &warmOverlapProbe{snapshotStarted: make(chan struct{}), profileStarted: make(chan struct{})}
}

func (p *warmOverlapProbe) enterSnapshot() {
	p.snapshotOnce.Do(func() { close(p.snapshotStarted) })
	select {
	case <-p.profileStarted:
		p.snapshotSawPeer.Store(true)
	case <-time.After(warmTestOverlapWait):
	}
}

func (p *warmOverlapProbe) enterProfile() {
	p.profileOnce.Do(func() { close(p.profileStarted) })
	select {
	case <-p.snapshotStarted:
		p.profileSawPeer.Store(true)
	case <-time.After(warmTestOverlapWait):
	}
}

type warmSnapshotProviderStub struct {
	probe    *warmOverlapProbe
	snapshot rendersnapshot.Snapshot
	err      error
	calls    atomic.Int32
}

func (p *warmSnapshotProviderStub) Resolve(context.Context, rendersnapshot.Selector, rendersnapshot.ResolveOptions) (rendersnapshot.Snapshot, error) {
	p.calls.Add(1)
	if p.probe != nil {
		p.probe.enterSnapshot()
	}
	if p.err != nil {
		return nil, p.err
	}
	return p.snapshot, nil
}

type warmProfileServer struct {
	server *httptest.Server
	probe  *warmOverlapProbe
	calls  atomic.Int32
	fail   bool
}

func newWarmProfileServer(t *testing.T, uid string, probe *warmOverlapProbe, fail bool) *warmProfileServer {
	t.Helper()
	s := &warmProfileServer{probe: probe, fail: fail}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/jp/"+uid+"/profile" {
			http.NotFound(w, r)
			return
		}
		s.calls.Add(1)
		if s.probe != nil {
			s.probe.enterProfile()
		}
		if s.fail {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{UserID: 12345678901234, Name: "Warm User"},
		})
	}))
	t.Cleanup(s.server.Close)

	oldBaseURL := config.Cfg.SekaiAPI.BaseURL
	oldToken := config.Cfg.SekaiAPI.Token
	config.Cfg.SekaiAPI.BaseURL = s.server.URL
	config.Cfg.SekaiAPI.Token = "test-token"
	t.Cleanup(func() {
		config.Cfg.SekaiAPI.BaseURL = oldBaseURL
		config.Cfg.SekaiAPI.Token = oldToken
	})
	return s
}

// disableSekaiProfileCache makes every profile lookup reach the test server,
// so call counts are not hidden by entries left by other tests.
func disableSekaiProfileCache(t *testing.T) {
	t.Helper()
	old := config.Cfg.Backend.APICacheTTL
	config.Cfg.Backend.APICacheTTL = 0
	t.Cleanup(func() { config.Cfg.Backend.APICacheTTL = old })
}

func useWarmSnapshotProvider(t *testing.T, provider rendersnapshot.HarukiSnapshotProvider) {
	t.Helper()
	original := snapshotProviderFactory
	snapshotProviderFactory = func(*renderapp.App) rendersnapshot.HarukiSnapshotProvider { return provider }
	t.Cleanup(func() { snapshotProviderFactory = original })
}

func newWarmTestRequestContext(t *testing.T, uid string, suiteVisible bool) *RequestContext {
	t.Helper()
	ctx := context.Background()
	service := newHandlerTestBindingServiceWithValidator(t, handlerMultiRegionBindingValidator{
		profiles: map[string]map[string]string{"jp": {uid: "Warm User"}},
	})
	if _, err := service.Bind(ctx, "qq", "warm-"+uid, uid); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if !suiteVisible {
		if _, err := service.SetBindingSuiteVisible(ctx, "qq", "warm-"+uid, "jp", false); err != nil {
			t.Fatalf("hide suite: %v", err)
		}
	}
	return NewRequestContext(ctx, &CommandRequest{
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "warm-" + uid,
	}, &renderapp.App{
		Config:   renderapp.Config{UserSnapshot: renderapp.UserSnapshotConfig{Provider: "toolbox"}},
		Bindings: service,
		SekaiAPI: sekaiapi.NewSekaiAPIClient(&config.Cfg.SekaiAPI),
	})
}

func TestWarmSuiteAndPublicProfileOverlapsBothFetches(t *testing.T) {
	disableSekaiProfileCache(t)
	const uid = "20000000000001"
	probe := newWarmOverlapProbe()
	server := newWarmProfileServer(t, uid, probe, false)
	provider := &warmSnapshotProviderStub{probe: probe, snapshot: &runtimeSnapshotStub{}}
	useWarmSnapshotProvider(t, provider)
	rc := newWarmTestRequestContext(t, uid, true)

	rc.warmSuiteAndPublicProfile(false)

	if !probe.snapshotSawPeer.Load() || !probe.profileSawPeer.Load() {
		t.Fatalf("expected snapshot and profile fetches to overlap, snapshot saw profile=%t profile saw snapshot=%t",
			probe.snapshotSawPeer.Load(), probe.profileSawPeer.Load())
	}
	binding, snap, err := rc.requireVisibleSuiteSnapshot()
	if err != nil || binding == nil || snap == nil {
		t.Fatalf("requireVisibleSuiteSnapshot() = %v, %v, %v", binding, snap, err)
	}
	if resp := rc.GetPublicProfileResponse(); resp == nil || resp.User.Name != "Warm User" {
		t.Fatalf("unexpected public profile: %+v", resp)
	}
	if provider.calls.Load() != 1 || server.calls.Load() != 1 {
		t.Fatalf("expected memoized single fetches, got snapshot=%d profile=%d", provider.calls.Load(), server.calls.Load())
	}
}

func TestWarmSuiteAndPublicProfileKeepsSnapshotErrorAndProfile(t *testing.T) {
	disableSekaiProfileCache(t)
	const uid = "20000000000002"
	server := newWarmProfileServer(t, uid, nil, false)
	provider := &warmSnapshotProviderStub{err: sekaiapi.ErrGameDataNotFound}
	useWarmSnapshotProvider(t, provider)

	warmed := newWarmTestRequestContext(t, uid, true)
	warmed.warmSuiteAndPublicProfile(false)
	_, _, warmedErr := warmed.requireVisibleSuiteSnapshot()

	// The snapshot failure must surface exactly as the serial path reports it,
	// and must not have cancelled the profile fetch that ran next to it.
	serial := newWarmTestRequestContext(t, uid, true)
	_, _, serialErr := serial.requireVisibleSuiteSnapshot()
	if warmedErr == nil || serialErr == nil || warmedErr.Error() != serialErr.Error() {
		t.Fatalf("snapshot error changed: warmed=%v serial=%v", warmedErr, serialErr)
	}
	testutil.RequireUserError(t, warmedErr, "", "")
	if resp := warmed.GetPublicProfileResponse(); resp == nil {
		t.Fatal("expected profile fetch to complete despite snapshot failure")
	}
	if server.calls.Load() != 1 {
		t.Fatalf("expected one profile fetch from the warm-up, got %d", server.calls.Load())
	}
}

func TestWarmSuiteAndPublicProfileKeepsSnapshotWhenProfileFails(t *testing.T) {
	disableSekaiProfileCache(t)
	const uid = "20000000000003"
	newWarmProfileServer(t, uid, nil, true)
	provider := &warmSnapshotProviderStub{snapshot: &runtimeSnapshotStub{}}
	useWarmSnapshotProvider(t, provider)
	rc := newWarmTestRequestContext(t, uid, true)

	rc.warmSuiteAndPublicProfile(false)

	if _, snap, err := rc.requireVisibleSuiteSnapshot(); err != nil || snap == nil {
		t.Fatalf("expected snapshot despite profile failure, got %v, %v", snap, err)
	}
	if resp := rc.GetPublicProfileResponse(); resp != nil {
		t.Fatalf("expected no public profile, got %+v", resp)
	}
}

func TestWarmSuiteAndPublicProfileSkipsWhenNothingWouldBeFetched(t *testing.T) {
	disableSekaiProfileCache(t)
	const uid = "20000000000004"
	server := newWarmProfileServer(t, uid, nil, false)
	provider := &warmSnapshotProviderStub{snapshot: &runtimeSnapshotStub{}}
	useWarmSnapshotProvider(t, provider)

	hidden := newWarmTestRequestContext(t, uid, false)
	hidden.warmSuiteAndPublicProfile(false)
	if binding, snap, err := hidden.requireVisibleSuiteSnapshot(); err != nil || binding == nil || snap != nil {
		t.Fatalf("hidden suite = %v, %v, %v", binding, snap, err)
	}

	unbound := NewRequestContext(context.Background(), &CommandRequest{
		Region: "jp", RequesterPlatform: "qq", RequesterUserID: "warm-unbound",
	}, hidden.App)
	unbound.warmSuiteAndPublicProfile(false)
	if _, _, err := unbound.requireVisibleSuiteSnapshot(); !errors.Is(err, accountdata.ErrNoBinding) {
		t.Fatalf("unbound error = %v", err)
	}

	var nilRC *RequestContext
	nilRC.warmSuiteAndPublicProfile(false)
	(&RequestContext{}).warmSuiteAndPublicProfile(false)

	if provider.calls.Load() != 0 || server.calls.Load() != 0 {
		t.Fatalf("expected no upstream calls, got snapshot=%d profile=%d", provider.calls.Load(), server.calls.Load())
	}
}

func TestFetchProfileAndTargetSnapshotOverlapsAndKeepsProfileErrorFatal(t *testing.T) {
	disableSekaiProfileCache(t)
	const uid = "20000000000005"
	target := ResolvedGameTarget{
		PJSKUserID: uid,
		Binding:    &accountdata.ResolvedBinding{Server: "jp", PJSKUserID: uid, SuiteVisible: true},
	}
	selfQuery := userQueryParams{Mode: "self", Platform: "qq", PlatformUserID: "warm-" + uid}

	t.Run("overlap", func(t *testing.T) {
		probe := newWarmOverlapProbe()
		newWarmProfileServer(t, uid, probe, false)
		useWarmSnapshotProvider(t, &warmSnapshotProviderStub{probe: probe, snapshot: &runtimeSnapshotStub{}})
		rc := newWarmTestRequestContext(t, uid, true)
		resp, snap, err := fetchProfileAndTargetSnapshot(rc, selfQuery, target, "jp")
		if err != nil || resp == nil || snap == nil {
			t.Fatalf("fetchProfileAndTargetSnapshot() = %v, %v, %v", resp, snap, err)
		}
		if !probe.snapshotSawPeer.Load() || !probe.profileSawPeer.Load() {
			t.Fatal("expected profile and snapshot fetches to overlap")
		}
	})

	t.Run("snapshot error is optional", func(t *testing.T) {
		newWarmProfileServer(t, uid, nil, false)
		useWarmSnapshotProvider(t, &warmSnapshotProviderStub{err: errors.New("toolbox down")})
		rc := newWarmTestRequestContext(t, uid, true)
		resp, snap, err := fetchProfileAndTargetSnapshot(rc, selfQuery, target, "jp")
		if err != nil || resp == nil || snap != nil {
			t.Fatalf("fetchProfileAndTargetSnapshot() = %v, %v, %v", resp, snap, err)
		}
	})

	t.Run("profile error is fatal", func(t *testing.T) {
		newWarmProfileServer(t, uid, nil, true)
		useWarmSnapshotProvider(t, &warmSnapshotProviderStub{snapshot: &runtimeSnapshotStub{}})
		rc := newWarmTestRequestContext(t, uid, true)
		resp, snap, err := fetchProfileAndTargetSnapshot(rc, selfQuery, target, "jp")
		if !errors.Is(err, sekaiapi.ErrUserNotFound) || resp != nil || snap != nil {
			t.Fatalf("fetchProfileAndTargetSnapshot() = %v, %v, %v", resp, snap, err)
		}
	})

	t.Run("no snapshot outside self mode", func(t *testing.T) {
		newWarmProfileServer(t, uid, nil, false)
		provider := &warmSnapshotProviderStub{snapshot: &runtimeSnapshotStub{}}
		useWarmSnapshotProvider(t, provider)
		rc := newWarmTestRequestContext(t, uid, true)
		resp, snap, err := fetchProfileAndTargetSnapshot(rc, userQueryParams{Mode: "uid", PJSKUserID: uid}, target, "jp")
		if err != nil || resp == nil || snap != nil || provider.calls.Load() != 0 {
			t.Fatalf("fetchProfileAndTargetSnapshot() = %v, %v, %v (snapshot calls %d)", resp, snap, err, provider.calls.Load())
		}
	})
}

type warmMySekaiPayloadProvider struct {
	probe *warmOverlapProbe
	err   error
}

func (p warmMySekaiPayloadProvider) Resolve(context.Context, rendersnapshot.Selector, bool) ([]byte, error) {
	if p.probe != nil {
		p.probe.enterSnapshot()
	}
	if p.err != nil {
		return nil, p.err
	}
	return []byte(`{"updatedResources":{}}`), nil
}

func TestResolveMySekaiRenderContextPrefetchesProfile(t *testing.T) {
	disableSekaiProfileCache(t)
	const uid = "20000000000006"
	params := userQueryParams{Mode: "self", Platform: "qq", PlatformUserID: "warm-" + uid}
	opts := mySekaiRenderContextOptions{NeedProfile: true, PreferMySekaiPayload: true, MySekaiPayloadOnly: true}
	newApp := func(t *testing.T, payloads rendersnapshot.MySekaiPayloadProvider) *renderapp.App {
		app := newWarmTestRequestContext(t, uid, true).App
		app.MySekai = rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true})
		app.Profiles = renderprofile.NewController(runtimeProfileDataSourceStub{region: renderregion.JP}, nil, nil, nil)
		app.MySekaiPayloads = payloads
		return app
	}

	t.Run("overlap", func(t *testing.T) {
		probe := newWarmOverlapProbe()
		newWarmProfileServer(t, uid, probe, false)
		app := newApp(t, warmMySekaiPayloadProvider{probe: probe})
		result, err := resolveMySekaiRenderContextWithOptions(context.Background(), app, params, "jp", false, opts)
		if err != nil {
			t.Fatalf("resolveMySekaiRenderContextWithOptions() error = %v", err)
		}
		if result.Profile == nil || result.Profile.Profile == nil || result.Profile.Profile.ID != uid {
			t.Fatalf("unexpected profile: %+v", result.Profile)
		}
		if !probe.snapshotSawPeer.Load() || !probe.profileSawPeer.Load() {
			t.Fatal("expected mysekai payload and profile fetches to overlap")
		}
	})

	t.Run("payload error wins over profile error", func(t *testing.T) {
		newWarmProfileServer(t, uid, nil, true)
		app := newApp(t, warmMySekaiPayloadProvider{err: sekaiapi.ErrGameDataNotFound})
		_, err := resolveMySekaiRenderContextWithOptions(context.Background(), app, params, "jp", false, opts)
		testutil.RequireUserError(t, err, usererror.CodeSetup, "binding.data.not_found_account")
	})

	t.Run("profile error after payload", func(t *testing.T) {
		newWarmProfileServer(t, uid, nil, true)
		app := newApp(t, warmMySekaiPayloadProvider{})
		_, err := resolveMySekaiRenderContextWithOptions(context.Background(), app, params, "jp", false, opts)
		testutil.RequireUserError(t, err, usererror.CodeNotFound, "upstream.game_data.player_not_found")
	})
}

func TestSekaiProfilePrefetchWaitIsRepeatable(t *testing.T) {
	prefetch := prefetchSekaiUserProfile(context.Background(), nil, "jp", "1")
	for range 2 {
		if resp, err := prefetch.wait(); resp != nil || !errors.Is(err, sekaiapi.ErrClientNotConfigured) {
			t.Fatalf("wait() = %v, %v", resp, err)
		}
	}
	card, err := buildPublicProfileCardForTargetWithPrefetch(context.Background(), ResolvedGameTarget{}, "jp",
		&renderapp.App{Profiles: renderprofile.NewController(runtimeProfileDataSourceStub{region: renderregion.JP}, nil, nil, nil)},
		nil, prefetch)
	if card != nil || !errors.Is(err, sekaiapi.ErrClientNotConfigured) || !strings.HasPrefix(err.Error(), "sekaiapi profile fetch failed") {
		t.Fatalf("buildPublicProfileCardForTargetWithPrefetch() = %v, %v", card, err)
	}
}
