package handler

import (
	"context"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

// RequestContext holds pre-resolved request-level data for a single command
// execution. It lazily resolves binding, snapshot, and profile on first access.
type RequestContext struct {
	Ctx            context.Context
	Cmd            *CommandRequest
	App            *renderapp.App
	Region         renderregion.Value
	RegionStr      string
	Platform       string
	PlatformUserID string

	// Lazy binding resolution
	bindingOnce  sync.Once
	binding      *accountdata.ResolvedBinding
	harukiUserID int
	bindingErr   error

	// Lazy snapshot resolution
	basicSnapshotOnce sync.Once
	basicSnapshot     snapshot.Snapshot
	basicSnapshotErr  error
	fullSnapshotOnce  sync.Once
	fullSnapshot      snapshot.Snapshot
	fullSnapshotErr   error

	// Lazy self target / public profile resolution
	selfTargetOnce    sync.Once
	selfTarget        *ResolvedGameTarget
	publicProfileOnce sync.Once
	publicProfileResp *sekaiapi.GetAnotherProfileResponse

	// Lazy profile resolution
	profileOnce     sync.Once
	detailedProfile *drawing.DetailedProfileCardRequest
	profileCard     *drawing.ProfileCardRequest
}

// NewRequestContext creates a RequestContext from a resolved command.
// Region is already resolved (resolveRegionFromDefaultBinding was called in Execute).
func NewRequestContext(ctx context.Context, r *CommandRequest, app *renderapp.App) *RequestContext {
	regionStr := regionWithDefault(r.Region)
	requestApp := app
	if app != nil {
		requestScoped := *app
		requestScoped.Assets = app.Assets.WithContext(ctx)
		requestScoped.Music = app.Music.WithContext(ctx)
		requestApp = &requestScoped
	}
	return &RequestContext{
		Ctx:            ctx,
		Cmd:            r,
		App:            requestApp,
		RegionStr:      regionStr,
		Region:         renderregion.Normalize(regionStr),
		Platform:       strings.TrimSpace(r.RequesterPlatform),
		PlatformUserID: strings.TrimSpace(r.RequesterUserID),
	}
}

func (rc *RequestContext) requestScopedSelfQuery() userQueryParams {
	if rc == nil {
		return userQueryParams{Mode: "self"}
	}
	params := userQueryParams{
		Mode:           "self",
		Platform:       rc.Platform,
		PlatformUserID: rc.PlatformUserID,
	}
	if rc.Cmd == nil {
		return params
	}

	var decoded userQueryParams
	mergeParams(rc.Cmd.Params, &decoded)
	switch strings.TrimSpace(decoded.Mode) {
	case "", "self":
		if strings.TrimSpace(decoded.Platform) != "" {
			params.Platform = strings.TrimSpace(decoded.Platform)
		}
		if strings.TrimSpace(decoded.PlatformUserID) != "" {
			params.PlatformUserID = strings.TrimSpace(decoded.PlatformUserID)
		}
		if strings.TrimSpace(decoded.Selector) != "" {
			params.Selector = strings.TrimSpace(decoded.Selector)
		}
	}
	return params
}

func (rc *RequestContext) snapshotSelector(needMySekai bool) (snapshot.Selector, snapshot.ResolveOptions) {
	query := rc.requestScopedSelfQuery()
	selector := snapshot.Selector{
		IMPlatform: strings.TrimSpace(query.Platform),
		IMUserID:   strings.TrimSpace(query.PlatformUserID),
		Region:     rc.Region,
	}
	opts := snapshot.ResolveOptions{
		PreferGlobalDefault: !rc.Cmd.RegionExplicit,
		NeedMySekai:         needMySekai,
	}

	if binding, _ := rc.GetBinding(); binding != nil {
		selector.PJSKUserID = strings.TrimSpace(binding.PJSKUserID)
		if normalized := renderregion.Normalize(binding.Server); !normalized.IsZero() {
			selector.Region = normalized
		}
		opts.PreferGlobalDefault = false
	}

	return selector, opts
}

// GetBinding lazily resolves the user's binding using the global-default →
// regional fallback pattern. Returns (binding, harukiUserID). Binding may be nil.
func (rc *RequestContext) GetBinding() (*accountdata.ResolvedBinding, int) {
	rc.bindingOnce.Do(func() {
		tBinding := time.Now()
		defer func() {
			recordCommandStage(rc.Ctx, "binding.resolve", time.Since(tBinding))
		}()
		if rc.App == nil || rc.App.Bindings == nil {
			return
		}
		query := rc.requestScopedSelfQuery()
		if query.Platform == "" || query.PlatformUserID == "" {
			return
		}
		rc.harukiUserID, rc.binding, rc.bindingErr = resolveBindingWithFallback(
			rc.Ctx,
			rc.App.Bindings,
			query.Platform,
			query.PlatformUserID,
			rc.RegionStr,
			rc.Cmd.RegionExplicit,
			bindingResolutionOptions{Selector: query.Selector},
		)
	})
	return rc.binding, rc.harukiUserID
}

// ResolveSnapshot resolves a request-scoped snapshot via the configured
// snapshot provider chain. In production this should be the live
// Toolbox/internal-cloud provider only; dev/test may still enable static
// snapshot fallback through allow_fallback.
// Cached per request for suite-only and full(mysekai) modes separately.
func (rc *RequestContext) ResolveSnapshot(needMySekai bool) snapshot.Snapshot {
	if needMySekai {
		rc.fullSnapshotOnce.Do(func() {
			tSnapshot := time.Now()
			defer func() {
				recordCommandStage(rc.Ctx, "snapshot.resolve_full", time.Since(tSnapshot))
			}()
			selector, opts := rc.snapshotSelector(true)
			rc.fullSnapshot, rc.fullSnapshotErr = resolveSnapshotBySelectorWithError(rc.Ctx, rc.App, selector, opts)
		})
		return rc.fullSnapshot
	}
	rc.basicSnapshotOnce.Do(func() {
		tSnapshot := time.Now()
		defer func() {
			recordCommandStage(rc.Ctx, "snapshot.resolve_basic", time.Since(tSnapshot))
		}()
		selector, opts := rc.snapshotSelector(false)
		rc.basicSnapshot, rc.basicSnapshotErr = resolveSnapshotBySelectorWithError(rc.Ctx, rc.App, selector, opts)
	})
	return rc.basicSnapshot
}

func (rc *RequestContext) SnapshotError(needMySekai bool) error {
	if rc == nil {
		return nil
	}
	_ = rc.ResolveSnapshot(needMySekai)
	if needMySekai {
		return rc.fullSnapshotErr
	}
	return rc.basicSnapshotErr
}

func (rc *RequestContext) GetSelfTarget() *ResolvedGameTarget {
	rc.selfTargetOnce.Do(func() {
		startedAt := time.Now()
		defer func() {
			recordCommandStage(rc.Ctx, "target.resolve", time.Since(startedAt))
		}()
		if rc.App == nil || rc.App.Bindings == nil {
			return
		}
		query := rc.requestScopedSelfQuery()
		if query.Platform == "" || query.PlatformUserID == "" {
			return
		}
		target, err := resolveGameTarget(rc.Ctx, query, rc.RegionStr, rc.Cmd.RegionExplicit, rc.App)
		if err != nil {
			return
		}
		rc.selfTarget = &target
	})
	return rc.selfTarget
}

func (rc *RequestContext) GetPublicProfileResponse() *sekaiapi.GetAnotherProfileResponse {
	rc.publicProfileOnce.Do(func() {
		tProfile := time.Now()
		defer func() {
			recordCommandStage(rc.Ctx, "sekai.profile", time.Since(tProfile))
		}()
		target := rc.GetSelfTarget()
		if target == nil || rc.App == nil || rc.App.SekaiAPI == nil {
			return
		}
		region := resolvedTargetRegion(rc.RegionStr, *target)
		resp, err := fetchCachedSekaiUserProfile(rc.Ctx, rc.App, region, target.PJSKUserID)
		if err != nil {
			return
		}
		rc.publicProfileResp = resp
	})
	return rc.publicProfileResp
}

func (rc *RequestContext) resolveProfiles() {
	rc.profileOnce.Do(func() {
		startedAt := time.Now()
		defer func() {
			recordCommandStage(rc.Ctx, "profile.resolve", time.Since(startedAt))
		}()
		if target := rc.GetSelfTarget(); target != nil {
			if resp := rc.GetPublicProfileResponse(); resp != nil {
				region := resolvedTargetRegion(rc.RegionStr, *target)
				rc.detailedProfile, rc.profileCard = buildPublicMusicProfilesFromResolvedTarget(
					rc.Ctx,
					*target,
					region,
					rc.Platform,
					rc.PlatformUserID,
					resp,
					rc.App,
				)
			}
		}
		if rc.detailedProfile == nil && rc.profileCard == nil {
			rc.detailedProfile, rc.profileCard = buildPublicMusicProfiles(rc)
		}
		// Only fall back to the full suite snapshot when the cheaper public
		// profile path left a gap. When both cards are already built, resolving
		// the snapshot here re-runs binding + factory.Build (a full suite parse)
		// whose result would just be discarded by the nil-guards below.
		if rc.detailedProfile == nil || rc.profileCard == nil {
			if snap := rc.ResolveSnapshot(false); snap != nil {
				if rc.detailedProfile == nil {
					if detail := snap.DetailedProfile(rc.Region); detail != nil {
						rc.detailedProfile = cloneDetailedProfileForCurrentTarget(rc, detail)
					}
				}
				if rc.profileCard == nil {
					if card := snap.ProfileCard(rc.Region); card != nil {
						rc.profileCard = cloneProfileCardForCurrentTarget(rc, card)
					}
				}
			}
		}
	})
}

// GetDetailedProfile lazily resolves the user's detailed profile, preferring
// the public Sekai API profile and only falling back to snapshot data when the
// API profile is unavailable.
func (rc *RequestContext) GetDetailedProfile() *drawing.DetailedProfileCardRequest {
	rc.resolveProfiles()
	return rc.detailedProfile
}

// GetProfileCard returns the compact profile card (resolved along with detailed profile).
func (rc *RequestContext) GetProfileCard() *drawing.ProfileCardRequest {
	rc.resolveProfiles()
	return rc.profileCard
}

// requireVisibleSuiteSnapshot is used by suite-dependent commands that should
// stop falling back to the public SekaiAPI path when the user simply has no
// suite data. If the binding exists but suite was intentionally hidden, the
// helper returns (binding, nil, nil) so callers can keep the existing
// hide-suite behavior unchanged.
func (rc *RequestContext) requireVisibleSuiteSnapshot() (*accountdata.ResolvedBinding, snapshot.Snapshot, error) {
	if rc == nil {
		return nil, nil, onebot11.NewReplayError(ErrMsgSuiteDataNotFound)
	}
	if rc.Platform == "" || rc.PlatformUserID == "" || rc.App == nil || rc.App.Bindings == nil {
		return nil, nil, nil
	}

	binding, _ := rc.GetBinding()
	if binding == nil {
		if rc.bindingErr != nil {
			return nil, nil, rc.bindingErr
		}
		return nil, nil, accountdata.ErrNoBinding
	}
	if !binding.SuiteVisible {
		return binding, nil, nil
	}

	snap := rc.ResolveSnapshot(false)
	if snap == nil {
		if snapshotErr := rc.SnapshotError(false); snapshotErr != nil {
			return binding, nil, normalizeToolboxDataFetchError(snapshotErr, "suite", binding)
		}
		return binding, nil, newSuiteDataNotFoundReplayErrorForBinding(binding)
	}
	return binding, snap, nil
}

// ImageMessage is a convenience method to store image bytes and return an image message.
func (rc *RequestContext) ImageMessage(data []byte) (onebot11.Message, error) {
	startedAt := time.Now()
	url, err := rc.App.ImageCache.StoreAndGetURL(rc.Ctx, data, BotModulePJSK)
	recordCommandStage(rc.Ctx, "image.store", time.Since(startedAt))
	if err != nil {
		return nil, err
	}
	return onebot11.Message{onebot11.Image(url, "")}, nil
}
