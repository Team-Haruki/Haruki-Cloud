package handler

import (
	"context"
	"strings"
	"sync"

	"haruki-cloud/internal/pjsk/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
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

// GetBinding lazily resolves the user's binding using the global-default →
// regional fallback pattern. Returns (binding, harukiUserID). Binding may be nil.
func (rc *RequestContext) GetBinding() (*accountdata.ResolvedBinding, int) {
	rc.bindingOnce.Do(func() {
		if rc.Platform == "" || rc.PlatformUserID == "" || rc.App.Bindings == nil {
			return
		}
		if !rc.Cmd.RegionExplicit {
			rc.harukiUserID, rc.binding, rc.bindingErr = rc.App.Bindings.ResolveUserBinding(
				rc.Ctx, rc.Platform, rc.PlatformUserID, accountdata.GlobalDefaultBindingScope)
			if rc.bindingErr != nil || rc.binding == nil {
				rc.harukiUserID, rc.binding, rc.bindingErr = rc.App.Bindings.ResolveUserBinding(
					rc.Ctx, rc.Platform, rc.PlatformUserID, rc.RegionStr)
			}
		} else {
			rc.harukiUserID, rc.binding, rc.bindingErr = rc.App.Bindings.ResolveUserBinding(
				rc.Ctx, rc.Platform, rc.PlatformUserID, rc.RegionStr)
		}
	})
	return rc.binding, rc.harukiUserID
}

// ResolveSnapshot resolves a request-scoped snapshot via the configured
// snapshot provider chain. In production this should be the live
// Toolbox/internal-cloud provider only; dev/test may still enable static
// snapshot fallback through allow_fallback.
// Cached per request for suite-only and full(mysekai) modes separately.
func (rc *RequestContext) ResolveSnapshot(needMySekai bool) userdata.Snapshot {
	if needMySekai {
		rc.fullSnapshotOnce.Do(func() {
			rc.fullSnapshot = resolveLiveSnapshot(rc, true)
		})
		return rc.fullSnapshot
	}
	rc.basicSnapshotOnce.Do(func() {
		rc.basicSnapshot = resolveLiveSnapshot(rc, false)
	})
	return rc.basicSnapshot
}

func (rc *RequestContext) GetSelfTarget() *resolvedGameTarget {
	rc.selfTargetOnce.Do(func() {
		if rc.Platform == "" || rc.PlatformUserID == "" || rc.App == nil || rc.App.Bindings == nil {
			return
		}
		target, err := resolveGameTarget(rc.Ctx, userQueryParams{
			Mode:           "self",
			Platform:       rc.Platform,
			PlatformUserID: rc.PlatformUserID,
		}, rc.RegionStr, rc.Cmd.RegionExplicit, rc.App)
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
		if target == nil {
			return
		}
		region := resolvedTargetRegion(rc.RegionStr, *target)
		resp, err := sekaiapi.GetSekaiAPIClient().GetUserProfile(region, target.PJSKUserID)
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
		return nil, nil, onebot11.NewReplayError(ErrMsgSuiteDataNotFound)
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
