package handler

import (
	"context"
	"strings"
	"sync"

	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/userdata"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

// RequestContext holds pre-resolved request-level data for a single command
// execution. It lazily resolves binding, snapshot, and profile on first access.
type RequestContext struct {
	Ctx            context.Context
	Cmd            *parser.ResolvedCommand
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
	basicSnapshot     userdata.Snapshot
	fullSnapshotOnce  sync.Once
	fullSnapshot      userdata.Snapshot

	// Lazy self target / public profile resolution
	selfTargetOnce    sync.Once
	selfTarget        *resolvedGameTarget
	publicProfileOnce sync.Once
	publicProfileResp *sekaiapi.GetAnotherProfileResponse

	// Lazy profile resolution
	profileOnce     sync.Once
	detailedProfile *drawing.DetailedProfileCardRequest
	profileCard     *drawing.ProfileCardRequest
}

// NewRequestContext creates a RequestContext from a resolved command.
// Region is already resolved (resolveRegionFromDefaultBinding was called in Execute).
func NewRequestContext(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) *RequestContext {
	regionStr := regionWithDefault(r.Region)
	return &RequestContext{
		Ctx:            ctx,
		Cmd:            r,
		App:            app,
		RegionStr:      regionStr,
		Region:         renderregion.Normalize(regionStr),
		Platform:       strings.TrimSpace(r.RequesterPlatform),
		PlatformUserID: strings.TrimSpace(r.RequesterUserID),
	}
}

func (rc *RequestContext) requestScopedSelfQuery() userQueryParams {
	params := userQueryParams{
		Mode:           "self",
		Platform:       rc.Platform,
		PlatformUserID: rc.PlatformUserID,
	}
	if rc == nil || rc.Cmd == nil {
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

func (rc *RequestContext) snapshotSelector(needMySekai bool) (userdata.Selector, userdata.ResolveOptions) {
	query := rc.requestScopedSelfQuery()
	selector := userdata.Selector{
		IMPlatform: strings.TrimSpace(query.Platform),
		IMUserID:   strings.TrimSpace(query.PlatformUserID),
		Region:     rc.Region,
	}
	opts := userdata.ResolveOptions{
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

// requireBinding resolves the current self-account binding when requester
// context is available and returns accountdata.ErrNoBinding if nothing matches.
func (rc *RequestContext) requireBinding() (*accountdata.ResolvedBinding, error) {
	if rc == nil {
		return nil, accountdata.ErrNoBinding
	}
	if rc.Platform == "" || rc.PlatformUserID == "" || rc.App == nil || rc.App.Bindings == nil {
		return nil, nil
	}

	binding, _ := rc.GetBinding()
	if binding == nil {
		if rc.bindingErr != nil {
			return nil, rc.bindingErr
		}
		return nil, accountdata.ErrNoBinding
	}
	return binding, nil
}

// ResolveSnapshot resolves a request-scoped snapshot via the configured
// snapshot provider chain. In production this should be the live
// Toolbox/internal-cloud provider only; dev/test may still enable static
// snapshot fallback through allow_fallback.
// Cached per request for suite-only and full(mysekai) modes separately.
func (rc *RequestContext) ResolveSnapshot(needMySekai bool) userdata.Snapshot {
	if needMySekai {
		rc.fullSnapshotOnce.Do(func() {
			selector, opts := rc.snapshotSelector(true)
			rc.fullSnapshot = resolveSnapshotBySelector(rc.Ctx, rc.App, selector, opts)
		})
		return rc.fullSnapshot
	}
	rc.basicSnapshotOnce.Do(func() {
		selector, opts := rc.snapshotSelector(false)
		rc.basicSnapshot = resolveSnapshotBySelector(rc.Ctx, rc.App, selector, opts)
	})
	return rc.basicSnapshot
}

func (rc *RequestContext) GetSelfTarget() *resolvedGameTarget {
	rc.selfTargetOnce.Do(func() {
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
		copy := target
		rc.selfTarget = &copy
	})
	return rc.selfTarget
}

func (rc *RequestContext) GetPublicProfileResponse() *sekaiapi.GetAnotherProfileResponse {
	rc.publicProfileOnce.Do(func() {
		target := rc.GetSelfTarget()
		if target == nil || rc.App == nil || rc.App.SekaiAPI == nil {
			return
		}
		region := resolvedTargetRegion(rc.RegionStr, *target)
		resp, err := rc.App.SekaiAPI.GetUserProfile(region, target.PJSKUserID)
		if err != nil {
			return
		}
		rc.publicProfileResp = resp
	})
	return rc.publicProfileResp
}

func (rc *RequestContext) resolveProfiles() {
	rc.profileOnce.Do(func() {
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
		if snapshot := rc.ResolveSnapshot(false); snapshot != nil {
			if detail := snapshot.DetailedProfile(rc.Region); detail != nil {
				rc.detailedProfile = detail
			}
			if card := snapshot.ProfileCard(rc.Region); card != nil {
				rc.profileCard = card
			}
		}
	})
}

// GetDetailedProfile lazily resolves the user's detailed profile, preferring
// live snapshot data over Sekai API data.
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
func (rc *RequestContext) requireVisibleSuiteSnapshot() (*accountdata.ResolvedBinding, userdata.Snapshot, error) {
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

	snapshot := rc.ResolveSnapshot(false)
	if snapshot == nil {
		return binding, nil, onebot11.NewReplayError(ErrMsgSuiteDataNotFound)
	}
	return binding, snapshot, nil
}

// ImageMessage is a convenience method to store image bytes and return an image message.
func (rc *RequestContext) ImageMessage(data []byte) (onebot11.Message, error) {
	url, err := rc.App.ImageCache.StoreAndGetURL(rc.Ctx, data, BotModulePJSK)
	if err != nil {
		return nil, err
	}
	return onebot11.Message{onebot11.Image(url, "")}, nil
}
