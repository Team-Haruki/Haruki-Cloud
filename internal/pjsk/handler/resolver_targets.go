package handler

import (
	"context"
	"fmt"
	"strings"

	"haruki-cloud/internal/i18n"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/usererror"

	"haruki-cloud/internal/pjsk/accountdata"
)

// resolveGameTarget resolves the account a query is about. For another
// user's account (@群友), exposure is what the command shows of it; an owner
// who hid that exposure gets the request refused (hiddenTargetError).
func resolveGameTarget(ctx context.Context, p userQueryParams, region string, regionExplicit bool, app *renderapp.App, exposure accountdata.Exposure) (ResolvedGameTarget, error) {
	if app == nil || app.Bindings == nil {
		return ResolvedGameTarget{}, accountdata.ErrBindingServiceUnavailable
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
			return ResolvedGameTarget{}, normalizeBindingLookupError(err, i18n.Message{})
		}
		return ResolvedGameTarget{
			HarukiUserID: hid,
			PJSKUserID:   binding.PJSKUserID,
			UIDVisible:   binding.Visibility.UID,
			BgSettings:   binding.Bg,
			Binding:      binding,
		}, nil
	case "at_user":
		_, binding, err := app.Bindings.ResolveUserBinding(ctx, p.Platform, p.AtUserID, region)
		if err != nil {
			return ResolvedGameTarget{}, normalizeBindingLookupError(err, i18n.M("binding.target_not_bound"))
		}
		if !binding.Visibility.Allows(exposure) {
			return ResolvedGameTarget{}, hiddenTargetError(exposure)
		}
		return ResolvedGameTarget{
			PJSKUserID: binding.PJSKUserID,
			UIDVisible: binding.Visibility.UID,
			BgSettings: binding.Bg,
			Binding:    binding,
		}, nil
	case "uid":
		return ResolvedGameTarget{
			PJSKUserID: p.PJSKUserID,
			UIDVisible: true,
		}, nil
	default:
		return ResolvedGameTarget{}, fmt.Errorf("unknown query mode %q", p.Mode)
	}
}

// hiddenTargetError is the reply when another user's account hides what a
// command would show of it.
func hiddenTargetError(exposure accountdata.Exposure) error {
	switch exposure {
	case accountdata.ExposureSK:
		return usererror.Forbidden(i18n.M("binding.target_hidden_sk"))
	case accountdata.ExposureArrest:
		return usererror.Forbidden(i18n.M("binding.target_hidden_arrest"))
	default:
		return usererror.Forbidden(i18n.M("binding.target_hidden"))
	}
}

// resolveRegionFromDefaultBinding resolves the region for a command where the
// user did not provide an explicit region prefix or -r flag. It looks up the
// user's global default binding (server = "default") and returns the server of
// the bound account (e.g. "jp", "tw", "kr", "en"). Falls back to "jp" if the
// user has no global default binding or if any lookup fails.
func resolveRegionFromDefaultBinding(ctx context.Context, r *CommandRequest, app *renderapp.App) string {
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

func resolvedTargetRegion(region string, target ResolvedGameTarget) string {
	if target.Binding != nil {
		if normalized := renderregion.Normalize(target.Binding.Server); !normalized.IsZero() {
			return normalized.String()
		}
	}
	return regionWithDefault(region)
}
