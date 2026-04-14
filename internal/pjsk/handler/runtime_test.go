package handler

import (
	"context"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderprofile "haruki-cloud/internal/pjsk/render/profile"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	renderuserdata "haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
)

type runtimeSnapshotStub struct {
	detail       *drawing.DetailedProfileCardRequest
	card         *drawing.ProfileCardRequest
	musicResults map[string]map[int]string
}

func (s *runtimeSnapshotStub) Require() error { return nil }

func (s *runtimeSnapshotStub) DetailedProfile(renderregion.Value) *drawing.DetailedProfileCardRequest {
	return s.detail
}

func (s *runtimeSnapshotStub) ProfileCard(renderregion.Value) *drawing.ProfileCardRequest {
	return s.card
}

func (s *runtimeSnapshotStub) MusicResults(diff string) map[int]string {
	if s == nil || s.musicResults == nil {
		return nil
	}
	src := s.musicResults[diff]
	if len(src) == 0 {
		return nil
	}
	out := make(map[int]string, len(src))
	for musicID, result := range src {
		out[musicID] = result
	}
	return out
}

func (s *runtimeSnapshotStub) GetMusicResult(musicID int, diff string) string {
	if s == nil || s.musicResults == nil {
		return ""
	}
	return s.musicResults[diff][musicID]
}

func (s *runtimeSnapshotStub) ChallengeLive() *renderuserdata.ChallengeLiveData { return nil }

func (s *runtimeSnapshotStub) RawBytes() ([]byte, error) { return nil, nil }

func (s *runtimeSnapshotStub) RawValue(string) ([]byte, error) { return nil, nil }

func (s *runtimeSnapshotStub) RawFilePath() string { return "" }

func (s *runtimeSnapshotStub) RawData() *renderuserdata.RawUserData { return nil }

func (s *runtimeSnapshotStub) MusicMetaBytes() []byte { return nil }

func (s *runtimeSnapshotStub) MusicMetaPath() string { return "" }

type runtimeSnapshotProviderStub struct {
	snapshot         renderuserdata.Snapshot
	resolveCount     int
	resolveNeedFlags []bool
	selectors        []renderuserdata.Selector
}

func (p *runtimeSnapshotProviderStub) Resolve(_ context.Context, selector renderuserdata.Selector, opts renderuserdata.ResolveOptions) (renderuserdata.Snapshot, error) {
	p.resolveCount++
	p.resolveNeedFlags = append(p.resolveNeedFlags, opts.NeedMySekai)
	p.selectors = append(p.selectors, selector)
	return p.snapshot, nil
}

func TestRequestContextCachesBasicSnapshotAndUsesSnapshotProfiles(t *testing.T) {
	provider := &runtimeSnapshotProviderStub{
		snapshot: &runtimeSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{Nickname: "snapshot-detail"},
			card: &drawing.ProfileCardRequest{
				Profile: &drawing.BasicProfile{Nickname: "snapshot-card"},
			},
		},
	}

	rc := NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Snapshots: provider,
	})

	first := rc.ResolveSnapshot(false)
	second := rc.ResolveSnapshot(false)
	if first == nil || second == nil {
		t.Fatalf("expected cached snapshot")
	}
	if provider.resolveCount != 1 {
		t.Fatalf("expected one basic snapshot resolve, got %d", provider.resolveCount)
	}
	if len(provider.resolveNeedFlags) != 1 || provider.resolveNeedFlags[0] {
		t.Fatalf("unexpected resolve flags: %+v", provider.resolveNeedFlags)
	}

	detail := rc.GetDetailedProfile()
	if detail == nil || detail.Nickname != "snapshot-detail" {
		t.Fatalf("unexpected detailed profile: %+v", detail)
	}
	card := rc.GetProfileCard()
	if card == nil || card.Profile == nil || card.Profile.Nickname != "snapshot-card" {
		t.Fatalf("unexpected profile card: %+v", card)
	}
	if provider.resolveCount != 1 {
		t.Fatalf("profile resolution should reuse cached basic snapshot, got %d resolves", provider.resolveCount)
	}
}

func TestRequestContextCachesMySekaiSnapshotSeparately(t *testing.T) {
	provider := &runtimeSnapshotProviderStub{
		snapshot: &runtimeSnapshotStub{},
	}

	rc := NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Snapshots: provider,
	})

	_ = rc.ResolveSnapshot(true)
	_ = rc.ResolveSnapshot(true)
	if provider.resolveCount != 1 {
		t.Fatalf("expected one mysekai snapshot resolve, got %d", provider.resolveCount)
	}
	if len(provider.resolveNeedFlags) != 1 || !provider.resolveNeedFlags[0] {
		t.Fatalf("unexpected resolve flags: %+v", provider.resolveNeedFlags)
	}
}

func TestRequestContextUsesConfiguredSnapshotProviderFactory(t *testing.T) {
	liveProvider := &runtimeSnapshotProviderStub{
		snapshot: &runtimeSnapshotStub{},
	}
	fallbackProvider := &runtimeSnapshotProviderStub{
		snapshot: &runtimeSnapshotStub{},
	}

	originalFactory := snapshotProviderFactory
	snapshotProviderFactory = func(app *renderapp.App) renderuserdata.SnapshotProvider {
		if app != nil && app.Config.UserSnapshot.Provider == "internal_cloud" {
			return liveProvider
		}
		return originalFactory(app)
	}
	defer func() {
		snapshotProviderFactory = originalFactory
	}()

	rc := NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{Provider: "internal_cloud"},
		},
		Snapshots: fallbackProvider,
	})

	if snapshot := rc.ResolveSnapshot(false); snapshot == nil {
		t.Fatal("expected snapshot from configured provider")
	}
	if liveProvider.resolveCount != 1 {
		t.Fatalf("expected live provider to resolve once, got %d", liveProvider.resolveCount)
	}
	if fallbackProvider.resolveCount != 0 {
		t.Fatalf("expected fallback provider to stay unused, got %d resolves", fallbackProvider.resolveCount)
	}
}

func TestResolveCardBoxDetailedProfileDoesNotFallbackToProfileControllerSnapshot(t *testing.T) {
	profileController := renderprofile.NewController(nil, nil, nil, &runtimeSnapshotStub{
		detail: &drawing.DetailedProfileCardRequest{
			Nickname:  "controller-snapshot",
			UserCards: []any{map[string]any{"card_id": 1001}},
		},
	})

	rc := NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Profiles: profileController,
	})

	if detail := resolveCardBoxDetailedProfile(rc); detail != nil {
		t.Fatalf("expected nil detail without snapshot provider, got %+v", detail)
	}
}
