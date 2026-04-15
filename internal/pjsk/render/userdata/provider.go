package userdata

import (
	"context"
	"errors"

	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/drawing"
)

var (
	ErrSnapshotUnavailable = errors.New("userdata: snapshot is unavailable")
	ErrProviderUnavailable = errors.New("userdata: snapshot provider is unavailable")
	ErrSnapshotNotFound    = errors.New("userdata: snapshot not found")
)

// Snapshot is the stable read-only contract consumed by render controllers.
// It deliberately keeps the existing mature view-building methods, so new
// providers can plug in without forcing controller-specific rewrites.
type Snapshot interface {
	Require() error
	DetailedProfile(region renderregion.Value) *drawing.DetailedProfileCardRequest
	ProfileCard(region renderregion.Value) *drawing.ProfileCardRequest
	MusicResults(diff string) map[int]string
	GetMusicResult(musicID int, diff string) string
	ChallengeLive() *ChallengeLiveData
	RawBytes() ([]byte, error)
	RawValue(key string) ([]byte, error)
	RawFilePath() string
	RawData() *RawUserData
	MusicMetaBytes() []byte
	MusicMetaPath() string
}

// Selector identifies the caller whose snapshot should be resolved.
// PJSKUserID is optional and, when set, narrows resolution to a specific bound
// game account owned by the caller.
type Selector struct {
	IMPlatform string
	IMUserID   string
	Region     renderregion.Value
	PJSKUserID string
}

// ResolveOptions controls how the provider interprets the selector at runtime.
type ResolveOptions struct {
	PreferGlobalDefault bool
	NeedMySekai         bool
}

// SnapshotProvider resolves request-scoped snapshots.
type SnapshotProvider interface {
	Resolve(ctx context.Context, selector Selector, opts ResolveOptions) (Snapshot, error)
}

// StaticSnapshotProvider always returns the same prebuilt snapshot. It is
// useful for local-file fixtures, development fallback and tests.
type StaticSnapshotProvider struct {
	snapshot Snapshot
}

func NewStaticSnapshotProvider(snapshot Snapshot) *StaticSnapshotProvider {
	return &StaticSnapshotProvider{snapshot: snapshot}
}

func (p *StaticSnapshotProvider) Resolve(_ context.Context, _ Selector, _ ResolveOptions) (Snapshot, error) {
	if p == nil || p.snapshot == nil {
		return nil, ErrSnapshotUnavailable
	}
	if err := p.snapshot.Require(); err != nil {
		return nil, err
	}
	return p.snapshot, nil
}

var _ Snapshot = (*Service)(nil)
