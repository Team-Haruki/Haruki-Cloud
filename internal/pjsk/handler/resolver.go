package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
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
			hid, binding, err = app.Bindings.ResolveUserBindingBySelector(ctx, p.Platform, p.PlatformUserID, p.Selector)
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
func resolveLiveSnapshot(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App, needMySekai bool) *userdata.Service {
	platform := strings.TrimSpace(r.RequesterPlatform)
	platformUserID := strings.TrimSpace(r.RequesterUserID)
	if platform == "" || platformUserID == "" || app.Bindings == nil {
		return nil
	}

	regionStr := strings.TrimSpace(r.Region)
	if regionStr == "" {
		regionStr = "jp"
	}

	// Try global default binding first when no explicit region prefix,
	// then fall back to region-specific binding.
	var binding *accountdata.ResolvedBinding
	var resolveErr error
	if !r.RegionExplicit {
		_, binding, resolveErr = app.Bindings.ResolveUserBinding(ctx, platform, platformUserID, accountdata.GlobalDefaultBindingScope)
		if resolveErr != nil || binding == nil || !hasUsableSuiteData(binding) {
			_, binding, resolveErr = app.Bindings.ResolveUserBinding(ctx, platform, platformUserID, regionStr)
		}
	} else {
		_, binding, resolveErr = app.Bindings.ResolveUserBinding(ctx, platform, platformUserID, regionStr)
	}
	if resolveErr != nil || binding == nil || !hasUsableSuiteData(binding) {
		return nil
	}
	uid, convErr := strconv.ParseInt(binding.PJSKUserID, 10, 64)
	if convErr != nil {
		return nil
	}

	tc := sekaiutils.GetToolboxClient()
	suiteJSON, suiteErr := tc.GetSuiteData(regionStr, uid, platform, platformUserID)
	if suiteErr != nil || len(suiteJSON) == 0 {
		return nil
	}

	var mysekaiJSON []byte
	if needMySekai && hasUsableMySekaiData(binding) {
		mysekaiJSON, _ = tc.GetMySekaiData(regionStr, uid, platform, platformUserID)
	}

	region := renderregion.Normalize(regionStr)
	snapshot, err := userdata.NewFromBytes(app.Sekai, app.Assets, region, suiteJSON, mysekaiJSON, nil)
	if err != nil {
		return nil
	}
	return snapshot
}

// resolveMySekaiOnly fetches only the mysekai data from Toolbox, without
// requiring suite data. This is the lightweight fallback when the full
// merged snapshot is unavailable (e.g. the user has mysekai data uploaded
// but GetSuiteData fails). Returns nil on any error.
func resolveMySekaiOnly(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) []byte {
	platform := strings.TrimSpace(r.RequesterPlatform)
	platformUserID := strings.TrimSpace(r.RequesterUserID)
	if platform == "" || platformUserID == "" || app.Bindings == nil {
		return nil
	}

	regionStr := strings.TrimSpace(r.Region)
	if regionStr == "" {
		regionStr = "jp"
	}

	var binding *accountdata.ResolvedBinding
	var resolveErr error
	if !r.RegionExplicit {
		_, binding, resolveErr = app.Bindings.ResolveUserBinding(ctx, platform, platformUserID, accountdata.GlobalDefaultBindingScope)
		if resolveErr != nil || binding == nil || !hasUsableMySekaiData(binding) {
			_, binding, resolveErr = app.Bindings.ResolveUserBinding(ctx, platform, platformUserID, regionStr)
		}
	} else {
		_, binding, resolveErr = app.Bindings.ResolveUserBinding(ctx, platform, platformUserID, regionStr)
	}
	if resolveErr != nil || binding == nil || !hasUsableMySekaiData(binding) {
		return nil
	}
	uid, convErr := strconv.ParseInt(binding.PJSKUserID, 10, 64)
	if convErr != nil {
		return nil
	}

	tc := sekaiutils.GetToolboxClient()
	data, err := tc.GetMySekaiData(regionStr, uid, platform, platformUserID)
	if err != nil || len(data) == 0 {
		return nil
	}
	return data
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

	region := strings.TrimSpace(r.Region)
	if region == "" {
		region = string(renderregion.JP)
	}

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

	var framesJSON []byte
	if hasUsableSuiteData(target.Binding) {
		if uid, convErr := strconv.ParseInt(target.PJSKUserID, 10, 64); convErr == nil {
			framesJSON, _ = sekaiutils.GetToolboxClient().GetPrivateDataValue(
				region, sekaiutils.ToolboxDataTypeSuite, uid, queryParams.Platform, queryParams.PlatformUserID, "userPlayerFrames")
		}
	}

	q := profile.Query{
		Region:     region,
		Visible:    target.Visible,
		BgSettings: target.BgSettings,
	}
	detail, err := app.Profiles.BuildDetailedProfileCardFromAPI(q, resp, framesJSON)
	if err != nil {
		return nil, nil
	}
	card, err := app.Profiles.BuildProfileCardFromAPI(q, resp, framesJSON)
	if err != nil {
		return detail, nil
	}
	return detail, card
}

// buildPublicProfileCardForTarget builds a ProfileCardRequest for a resolved
// game target. Used by mysekai commands where the target is already resolved
// through userQueryParams (supporting u[i] selectors and region binding).
func buildPublicProfileCardForTarget(target resolvedGameTarget, region, platform, platformUserID string, app *renderapp.App) *drawing.ProfileCardRequest {
	if app == nil || app.Profiles == nil {
		return nil
	}

	resp, err := sekaiutils.GetSekaiAPIClient().GetUserProfile(region, target.PJSKUserID)
	if err != nil {
		return nil
	}

	var framesJSON []byte
	if hasUsableSuiteData(target.Binding) {
		if uid, convErr := strconv.ParseInt(target.PJSKUserID, 10, 64); convErr == nil {
			framesJSON, _ = sekaiutils.GetToolboxClient().GetPrivateDataValue(
				region, sekaiutils.ToolboxDataTypeSuite, uid, platform, platformUserID, "userPlayerFrames")
		}
	}

	q := profile.Query{
		Region:     region,
		Visible:    target.Visible,
		BgSettings: target.BgSettings,
	}
	card, err := app.Profiles.BuildProfileCardFromAPI(q, resp, framesJSON)
	if err != nil {
		return nil
	}
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
