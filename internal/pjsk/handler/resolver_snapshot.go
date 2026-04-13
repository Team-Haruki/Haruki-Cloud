package handler

import (
	"context"
	"strings"

	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"

	accountdata "haruki-cloud/internal/pjsk/userdata"
	sekaiutils "haruki-cloud/utils/sekai"
)

// resolveLiveSnapshot resolves the requester's Toolbox binding and fetches a
// live snapshot. If needMySekai is true it also fetches mysekai data and merges
// it into the snapshot. Returns nil if the user has no usable binding or if any
// API call fails (callers fall back to the controller-level static snapshot).
func resolveLiveSnapshot(rc *RequestContext, needMySekai bool) userdata.Snapshot {
	return resolveSnapshotBySelector(rc.Ctx, rc.App, userdata.Selector{
		IMPlatform: rc.Platform,
		IMUserID:   rc.PlatformUserID,
		Region:     rc.Region,
	}, userdata.ResolveOptions{
		PreferGlobalDefault: !rc.Cmd.RegionExplicit,
		NeedMySekai:         needMySekai,
	})
}

func resolveSnapshotBySelector(ctx context.Context, app *renderapp.App, selector userdata.Selector, opts userdata.ResolveOptions) userdata.Snapshot {
	provider := snapshotProviderFactory(app)
	if provider == nil {
		return nil
	}
	snapshot, err := provider.Resolve(ctx, selector, opts)
	if err != nil {
		return nil
	}
	return snapshot
}

var snapshotProviderFactory = defaultSnapshotProviderFactory

func defaultSnapshotProviderFactory(app *renderapp.App) userdata.SnapshotProvider {
	if app == nil {
		return nil
	}

	providers := make([]userdata.SnapshotProvider, 0, 2)
	if primary := liveSnapshotProvider(app); primary != nil {
		providers = append(providers, primary)
	}
	if app.Snapshots != nil {
		providers = append(providers, app.Snapshots)
	}
	switch len(providers) {
	case 0:
		return nil
	case 1:
		return providers[0]
	default:
		return userdata.NewFallbackSnapshotProvider(app.Config.UserSnapshot.AllowFallback, providers...)
	}
}

func liveSnapshotProvider(app *renderapp.App) userdata.SnapshotProvider {
	if app == nil || app.Bindings == nil {
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(app.Config.UserSnapshot.Provider)) {
	case "toolbox", "internal_cloud":
	default:
		return nil
	}

	provider := userdata.NewToolboxSnapshotProvider(app.Bindings, sekaiutils.GetToolboxClient(), app.Sekai, app.Assets)
	if app.MetaLoader != nil {
		provider = provider.WithMusicMetaSource(app.MetaLoader)
	}
	return provider
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
	// Selector, if non-empty, uses ResolveUserBindingBySelector instead of the
	// global-default → regional fallback pattern.
	Selector string
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

	if opts.Selector != "" {
		hid, binding, err = bindings.ResolveUserBindingBySelector(ctx, platform, platformUserID, regionStr, opts.Selector)
	} else if !regionExplicit {
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
