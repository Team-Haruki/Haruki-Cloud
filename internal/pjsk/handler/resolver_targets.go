package handler

import (
	"context"
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"

	"haruki-cloud/internal/pjsk/accountdata"
)

func resolveGameTarget(ctx context.Context, p userQueryParams, region string, regionExplicit bool, app *renderapp.App) (resolvedGameTarget, error) {
	if app == nil || app.Bindings == nil {
		return resolvedGameTarget{}, accountdata.ErrBindingServiceUnavailable
	}
	switch p.Mode {
	case "self":
		var hid int
		var binding *accountdata.ResolvedBinding
		var err error
		if p.Selector != "" {
			hid, binding, err = app.Bindings.ResolveUserBindingBySelector(ctx, p.Platform, p.PlatformUserID, selectorBindingServer(region, regionExplicit), p.Selector)
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
			return resolvedGameTarget{}, normalizeBindingLookupError(err, "解析绑定账号失败")
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
			return resolvedGameTarget{}, normalizeBindingLookupError(err, "未找到该用户的绑定账号")
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

func selectorBindingServer(region string, regionExplicit bool) string {
	if !regionExplicit {
		return ""
	}
	return strings.TrimSpace(region)
}

func resolvedTargetRegion(region string, target resolvedGameTarget) string {
	if target.Binding != nil {
		if normalized := renderregion.Normalize(target.Binding.Server); !normalized.IsZero() {
			return normalized.String()
		}
	}
	return regionWithDefault(region)
}
