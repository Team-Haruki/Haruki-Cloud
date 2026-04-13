package handler

import (
	"context"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	renderuserdata "haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
)

type runtimeSnapshotStub struct {
	detail *drawing.DetailedProfileCardRequest
	card   *drawing.ProfileCardRequest
}

func (s *runtimeSnapshotStub) Require() error { return nil }

func (s *runtimeSnapshotStub) DetailedProfile(renderregion.Value) *drawing.DetailedProfileCardRequest {
	return s.detail
}

func (s *runtimeSnapshotStub) ProfileCard(renderregion.Value) *drawing.ProfileCardRequest {
	return s.card
}

func (s *runtimeSnapshotStub) MusicResults(string) map[int]string { return nil }

func (s *runtimeSnapshotStub) GetMusicResult(int, string) string { return "" }

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
