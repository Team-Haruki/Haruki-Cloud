package handler

import (
	"context"
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	"haruki-cloud/internal/pjsk/render/profile"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	accountdata "haruki-cloud/internal/pjsk/userdata"
	"haruki-cloud/utils/drawing"
	sekaiutils "haruki-cloud/utils/sekai"
)

// userQueryParams mirrors sekai.UserQueryParams for bridge-side decoding.
type userQueryParams struct {
	Mode           string `json:"mode"`
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	AtUserID       string `json:"at_user_id"`
	PJSKUserID     string `json:"pjsk_user_id"`
	Selector       string `json:"selector,omitempty"`
}

type resolvedGameTarget struct {
	HarukiUserID int
	PJSKUserID   string
	Visible      bool
	BgSettings   *drawing.ProfileBgSettings
	Binding      *accountdata.ResolvedBinding
}

type mySekaiRenderContext struct {
	Controller *rendermysekai.Controller
	Profile    *drawing.ProfileCardRequest
}

func resolveGameTarget(ctx context.Context, p userQueryParams, region string, regionExplicit bool, app *renderapp.App) (resolvedGameTarget, error) {
	if app == nil || app.Bindings == nil {
		return resolvedGameTarget{}, fmt.Errorf("绑定服务未就绪")
	}
	switch p.Mode {
	case "self":
		var hid int
		var binding *accountdata.ResolvedBinding
		var err error
		if p.Selector != "" {
			hid, binding, err = app.Bindings.ResolveUserBindingBySelector(ctx, p.Platform, p.PlatformUserID, region, p.Selector)
		} else if !regionExplicit {
			// No explicit region prefix — use global default binding directly,
			// so the user's global default account is picked instead of a
			// potentially different server-specific default.
			hid, binding, err = app.Bindings.ResolveUserBinding(ctx, p.Platform, p.PlatformUserID, accountdata.GlobalDefaultBindingScope)
			if err != nil {
				hid, binding, err = app.Bindings.ResolveUserBinding(ctx, p.Platform, p.PlatformUserID, region)
			}
		} else {
			hid, binding, err = app.Bindings.ResolveUserBinding(ctx, p.Platform, p.PlatformUserID, region)
		}
		if err != nil {
			return resolvedGameTarget{}, fmt.Errorf("解析绑定账号失败：%w", err)
		}
		return resolvedGameTarget{
			HarukiUserID: hid,
			PJSKUserID:   binding.PJSKUserID,
			Visible:      binding.Visible,
			BgSettings:   binding.Bg,
			Binding:      binding,
		}, nil
	case "at_user":
		_, binding, err := app.Bindings.ResolveUserBinding(ctx, p.Platform, p.AtUserID, region)
		if err != nil {
			return resolvedGameTarget{}, fmt.Errorf("未找到该用户的绑定账号：%w", err)
		}
		if !binding.Visible {
			return resolvedGameTarget{}, fmt.Errorf("该用户已隐藏个人信息")
		}
		return resolvedGameTarget{
			PJSKUserID: binding.PJSKUserID,
			Visible:    binding.Visible,
			BgSettings: binding.Bg,
			Binding:    binding,
		}, nil
	case "uid":
		return resolvedGameTarget{
			PJSKUserID: p.PJSKUserID,
			Visible:    true,
		}, nil
	default:
		return resolvedGameTarget{}, fmt.Errorf("未知的查询模式：%q", p.Mode)
	}
}

// resolveGameUID resolves a game UID from userQueryParams using the identity and
// binding layers in the renderapp. For "at_user" mode the caller's visibility
// is checked; if the target has hidden their profile an error is returned.
//
// Returns (harukiUserID, pjskUserID, visible, error).
// harukiUserID is 0 and visible is true when the mode is "uid".
func resolveGameUID(ctx context.Context, p userQueryParams, region string, regionExplicit bool, app *renderapp.App) (int, string, bool, error) {
	target, err := resolveGameTarget(ctx, p, region, regionExplicit, app)
	if err != nil {
		return 0, "", false, err
	}
	return target.HarukiUserID, target.PJSKUserID, target.Visible, nil
}

// resolveLiveSnapshot resolves the requester's Toolbox binding and fetches a
// live snapshot. If needMySekai is true it also fetches mysekai data and merges
// it into the snapshot. Returns nil if the user has no usable binding or if any
// API call fails (callers fall back to the controller-level static snapshot).
func resolveLiveSnapshot(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App, needMySekai bool) userdata.Snapshot {
	platform := strings.TrimSpace(r.RequesterPlatform)
	platformUserID := strings.TrimSpace(r.RequesterUserID)
	return resolveSnapshotBySelector(ctx, app, userdata.Selector{
		IMPlatform: platform,
		IMUserID:   platformUserID,
		Region:     renderregion.Normalize(regionWithDefault(r.Region)),
	}, userdata.ResolveOptions{
		PreferGlobalDefault: !r.RegionExplicit,
		NeedMySekai:         needMySekai,
	})
}

func resolveSnapshotBySelector(ctx context.Context, app *renderapp.App, selector userdata.Selector, opts userdata.ResolveOptions) userdata.Snapshot {
	if app == nil || app.Snapshots == nil {
		return nil
	}
	snapshot, err := app.Snapshots.Resolve(ctx, selector, opts)
	if err != nil {
		return nil
	}
	return snapshot
}

func resolveTargetSnapshot(
	ctx context.Context,
	app *renderapp.App,
	regionStr string,
	platform string,
	platformUserID string,
	pjskUserID string,
	needMySekai bool,
) userdata.Snapshot {
	return resolveSnapshotBySelector(ctx, app, userdata.Selector{
		IMPlatform: strings.TrimSpace(platform),
		IMUserID:   strings.TrimSpace(platformUserID),
		Region:     renderregion.Normalize(regionStr),
		PJSKUserID: strings.TrimSpace(pjskUserID),
	}, userdata.ResolveOptions{
		PreferGlobalDefault: false,
		NeedMySekai:         needMySekai,
	})
}

func resolveMySekaiPayloadBySelector(ctx context.Context, app *renderapp.App, selector userdata.Selector, preferGlobalDefault bool) []byte {
	if app == nil || app.MySekaiPayloads == nil {
		return nil
	}
	payload, err := app.MySekaiPayloads.Resolve(ctx, selector, preferGlobalDefault)
	if err != nil || len(payload) == 0 {
		return nil
	}
	return payload
}

func resolveTargetMySekaiPayload(
	ctx context.Context,
	app *renderapp.App,
	regionStr string,
	platform string,
	platformUserID string,
	pjskUserID string,
) []byte {
	return resolveMySekaiPayloadBySelector(ctx, app, userdata.Selector{
		IMPlatform: strings.TrimSpace(platform),
		IMUserID:   strings.TrimSpace(platformUserID),
		Region:     renderregion.Normalize(regionStr),
		PJSKUserID: strings.TrimSpace(pjskUserID),
	}, false)
}

func resolveTargetMySekaiController(
	ctx context.Context,
	app *renderapp.App,
	regionStr string,
	platform string,
	platformUserID string,
	pjskUserID string,
) *rendermysekai.Controller {
	if app == nil || app.MySekai == nil {
		return nil
	}
	if snapshot := resolveTargetSnapshot(ctx, app, regionStr, platform, platformUserID, pjskUserID, true); snapshot != nil {
		return app.MySekai.WithSnapshot(snapshot)
	}
	if data := resolveTargetMySekaiPayload(ctx, app, regionStr, platform, platformUserID, pjskUserID); len(data) > 0 {
		return app.MySekai.WithMySekaiData(data)
	}
	return app.MySekai
}

func resolveMySekaiRenderContext(
	ctx context.Context,
	app *renderapp.App,
	params userQueryParams,
	regionStr string,
	regionExplicit bool,
) (mySekaiRenderContext, error) {
	if app == nil || app.MySekai == nil {
		return mySekaiRenderContext{}, fmt.Errorf("mysekai service unavailable: mysekai controller is not configured")
	}

	result := mySekaiRenderContext{Controller: app.MySekai}
	if app.Bindings == nil || strings.TrimSpace(params.Platform) == "" || strings.TrimSpace(params.PlatformUserID) == "" {
		return result, nil
	}

	target, err := resolveGameTarget(ctx, params, regionStr, regionExplicit, app)
	if err != nil {
		return mySekaiRenderContext{}, err
	}

	platform, platformUserID := platformCredentials(params)
	result.Profile = buildPublicProfileCardForTarget(ctx, target, regionStr, platform, platformUserID, app)
	if controller := resolveTargetMySekaiController(ctx, app, regionStr, platform, platformUserID, target.PJSKUserID); controller != nil {
		result.Controller = controller
	}
	return result, nil
}

// resolveRegionFromDefaultBinding resolves the region for a command where the
// user did not provide an explicit region prefix or -r flag. It looks up the
// user's global default binding (server = "default") and returns the server of
// the bound account (e.g. "jp", "tw", "kr", "en"). Falls back to "jp" if the
// user has no global default binding or if any lookup fails.
func resolveRegionFromDefaultBinding(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) string {
	if r.RegionExplicit {
		return r.Region
	}
	platform := strings.TrimSpace(r.RequesterPlatform)
	platformUserID := strings.TrimSpace(r.RequesterUserID)
	if platform == "" || platformUserID == "" || app.Bindings == nil {
		return r.Region
	}
	_, binding, err := app.Bindings.ResolveUserBinding(ctx, platform, platformUserID, accountdata.GlobalDefaultBindingScope)
	if err != nil || binding == nil || strings.TrimSpace(binding.Server) == "" {
		return r.Region
	}
	normalized := renderregion.Normalize(binding.Server)
	if normalized.IsZero() {
		return r.Region
	}
	return normalized.String()
}

func resolveCardBoxDetailedProfile(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) *drawing.DetailedProfileCardRequest {
	if r == nil || app == nil {
		return nil
	}
	region := renderregion.Normalize(r.Region)
	if snapshot := resolveLiveSnapshot(ctx, r, app, false); snapshot != nil {
		if detail := snapshot.DetailedProfile(region); detail != nil && len(detail.UserCards) > 0 {
			return detail
		}
	}
	if app.Profiles != nil {
		if detail := app.Profiles.SnapshotDetailedProfile(region); detail != nil && len(detail.UserCards) > 0 {
			return detail
		}
	}
	return nil
}

func buildPublicMusicProfiles(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) (*drawing.DetailedProfileCardRequest, *drawing.ProfileCardRequest) {
	if r == nil || app == nil || app.Profiles == nil || app.Bindings == nil {
		return nil, nil
	}
	if strings.TrimSpace(r.RequesterPlatform) == "" || strings.TrimSpace(r.RequesterUserID) == "" {
		return nil, nil
	}

	region := regionWithDefault(r.Region)

	queryParams := userQueryParams{
		Mode:           "self",
		Platform:       strings.TrimSpace(r.RequesterPlatform),
		PlatformUserID: strings.TrimSpace(r.RequesterUserID),
	}
	target, err := resolveGameTarget(ctx, queryParams, region, r.RegionExplicit, app)
	if err != nil {
		return nil, nil
	}

	resp, err := sekaiutils.GetSekaiAPIClient().GetUserProfile(region, target.PJSKUserID)
	if err != nil {
		return nil, nil
	}

	return buildPublicMusicProfilesFromResolvedTarget(ctx, target, region, queryParams.Platform, queryParams.PlatformUserID, resp, app)
}

func buildPublicMusicProfilesFromResolvedTarget(
	ctx context.Context,
	target resolvedGameTarget,
	region string,
	platform string,
	platformUserID string,
	resp *sekaiutils.GetAnotherProfileResponse,
	app *renderapp.App,
) (*drawing.DetailedProfileCardRequest, *drawing.ProfileCardRequest) {
	if app == nil || app.Profiles == nil || resp == nil {
		return nil, nil
	}

	var profileSnapshot userdata.Snapshot
	if hasUsableSuiteData(target.Binding) {
		profileSnapshot = resolveTargetSnapshot(ctx, app, region, platform, platformUserID, target.PJSKUserID, false)
	}

	q := profile.Query{
		Region:     region,
		Visible:    target.Visible,
		BgSettings: target.BgSettings,
	}
	detail, err := app.Profiles.BuildDetailedProfileCardFromAPIWithSnapshot(q, resp, profileSnapshot)
	if err != nil {
		return nil, nil
	}
	card, err := app.Profiles.BuildProfileCardFromAPIWithSnapshot(q, resp, profileSnapshot)
	if err != nil {
		return detail, nil
	}
	return detail, card
}

// buildPublicProfileCardForTarget builds a ProfileCardRequest for a resolved
// game target. Used by mysekai commands where the target is already resolved
// through userQueryParams (supporting u[i] selectors and region binding).
func buildPublicProfileCardForTarget(ctx context.Context, target resolvedGameTarget, region, platform, platformUserID string, app *renderapp.App) *drawing.ProfileCardRequest {
	if app == nil || app.Profiles == nil {
		return nil
	}

	resp, err := sekaiutils.GetSekaiAPIClient().GetUserProfile(region, target.PJSKUserID)
	if err != nil {
		return nil
	}
	_, card := buildPublicMusicProfilesFromResolvedTarget(ctx, target, region, platform, platformUserID, resp, app)
	return card
}

func hasUsableSuiteData(binding *accountdata.ResolvedBinding) bool {
	return binding != nil && binding.SuiteVisible
}

// MySekai availability is governed by a dedicated binding flag rather than
// reusing SuiteVisible. This matches the split semantics we want in Cloud and
// keeps suite/mysekai private-data visibility independently configurable.
func hasUsableMySekaiData(binding *accountdata.ResolvedBinding) bool {
	return binding != nil && binding.MySekaiVisible
}

// bindingResolutionOptions specifies how to resolve a user binding using the
// global-default → regional fallback pattern. Each call site can customize the
// data requirement (suite vs mysekai) by setting the appropriate validator.
type bindingResolutionOptions struct {
	// RequireSuite, if true, considers a binding valid only when hasUsableSuiteData returns true.
	RequireSuite bool
	// RequireMySekai, if true, considers a binding valid only when hasUsableMySekaiData returns true.
	RequireMySekai bool
}

// resolveBindingWithFallback resolves a user binding using the standard pattern:
// 1. If regionExplicit is false, try global default binding first
// 2. Fall back to region-specific binding
// 3. Validate the binding against the options (suite/mysekai requirements)
//
// Returns (harukiUserID, binding, nil) on success. The binding is non-nil on success.
// Returns (0, nil, nil) when the binding service is unavailable or no valid binding found.
func resolveBindingWithFallback(
	ctx context.Context,
	bindings *accountdata.BindingService,
	platform, platformUserID, regionStr string,
	regionExplicit bool,
	opts bindingResolutionOptions,
) (int, *accountdata.ResolvedBinding, error) {
	if bindings == nil || platform == "" || platformUserID == "" {
		return 0, nil, nil
	}

	var hid int
	var binding *accountdata.ResolvedBinding
	var err error

	isValid := func(b *accountdata.ResolvedBinding) bool {
		if b == nil {
			return false
		}
		if opts.RequireSuite && !hasUsableSuiteData(b) {
			return false
		}
		if opts.RequireMySekai && !hasUsableMySekaiData(b) {
			return false
		}
		return true
	}

	if !regionExplicit {
		hid, binding, err = bindings.ResolveUserBinding(ctx, platform, platformUserID, accountdata.GlobalDefaultBindingScope)
		if err != nil || !isValid(binding) {
			hid, binding, err = bindings.ResolveUserBinding(ctx, platform, platformUserID, regionStr)
		}
	} else {
		hid, binding, err = bindings.ResolveUserBinding(ctx, platform, platformUserID, regionStr)
	}

	if err != nil || !isValid(binding) {
		return 0, nil, nil
	}
	return hid, binding, nil
}

// platformCredentials returns the (platform, platformUserID) pair for toolbox
// key queries. Returns empty strings for "uid" mode (no credentials available).
func platformCredentials(p userQueryParams) (string, string) {
	switch p.Mode {
	case "self":
		return p.Platform, p.PlatformUserID
	case "at_user":
		return p.Platform, p.AtUserID
	default:
		return "", ""
	}
}
