package handler

import (
	"context"
	"fmt"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/userdata"

	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/utils/logger"
)

var snapshotDebugLogger = logger.NewLoggerFromGlobal("PJSKSnapshot")

// resolveLiveSnapshot resolves the requester's bound snapshot provider. In
// production this is expected to be the Toolbox/internal-cloud provider only.
// When allow_fallback=true (dev/test), a configured static snapshot provider
// may also be included in the provider chain.
func resolveLiveSnapshot(rc *RequestContext, needMySekai bool) userdata.Snapshot {
	selector, opts := rc.snapshotSelector(needMySekai)
	return resolveSnapshotBySelector(rc.Ctx, rc.App, selector, opts)
}

func resolveSnapshotBySelector(ctx context.Context, app *renderapp.App, selector userdata.Selector, opts userdata.ResolveOptions) userdata.Snapshot {
	provider := snapshotProviderFactory(app)
	if provider == nil {
		snapshotDebugLogger.Debugf("snapshot resolve skipped: provider is unavailable platform=%s user=%s region=%s pjsk_user=%s need_mysekai=%t prefer_global_default=%t",
			strings.TrimSpace(selector.IMPlatform), maskDebugID(selector.IMUserID), selector.Region.String(), maskDebugID(selector.PJSKUserID), opts.NeedMySekai, opts.PreferGlobalDefault)
		return nil
	}
	snapshot, err := provider.Resolve(ctx, selector, opts)
	if err != nil {
		snapshotDebugLogger.Warnf("snapshot resolve failed: platform=%s user=%s region=%s pjsk_user=%s need_mysekai=%t prefer_global_default=%t err=%v",
			strings.TrimSpace(selector.IMPlatform), maskDebugID(selector.IMUserID), selector.Region.String(), maskDebugID(selector.PJSKUserID), opts.NeedMySekai, opts.PreferGlobalDefault, err)
		return nil
	}
	snapshotDebugLogger.Debugf("snapshot resolve succeeded: platform=%s user=%s region=%s pjsk_user=%s need_mysekai=%t prefer_global_default=%t raw_path=%q",
		strings.TrimSpace(selector.IMPlatform), maskDebugID(selector.IMUserID), selector.Region.String(), maskDebugID(selector.PJSKUserID), opts.NeedMySekai, opts.PreferGlobalDefault, snapshot.RawFilePath())
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
	if app.Config.UserSnapshot.AllowFallback && app.Snapshots != nil {
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

	if app.Toolbox == nil {
		return nil
	}
	provider := userdata.NewToolboxSnapshotProvider(app.Bindings, app.Toolbox, app.Sekai, app.Assets)
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

	snapshotDebugLogger.Debugf("binding resolve start: platform=%s user=%s region=%s region_explicit=%t selector=%q require_suite=%t require_mysekai=%t",
		strings.TrimSpace(platform), maskDebugID(platformUserID), strings.TrimSpace(regionStr), regionExplicit, strings.TrimSpace(opts.Selector), opts.RequireSuite, opts.RequireMySekai)

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
		hid, binding, err = bindings.ResolveUserBindingBySelector(ctx, platform, platformUserID, selectorBindingServer(regionStr, regionExplicit), opts.Selector)
		snapshotDebugLogger.Debugf("binding resolve selector result: platform=%s user=%s region=%s selector=%q hid=%d binding=%s err=%v",
			strings.TrimSpace(platform), maskDebugID(platformUserID), strings.TrimSpace(regionStr), strings.TrimSpace(opts.Selector), hid, formatBindingDebug(binding), err)
	} else if !regionExplicit {
		hid, binding, err = bindings.ResolveUserBinding(ctx, platform, platformUserID, accountdata.GlobalDefaultBindingScope)
		snapshotDebugLogger.Debugf("binding resolve global-default result: platform=%s user=%s hid=%d binding=%s err=%v",
			strings.TrimSpace(platform), maskDebugID(platformUserID), hid, formatBindingDebug(binding), err)
		if err != nil || !isValid(binding) {
			hid, binding, err = bindings.ResolveUserBinding(ctx, platform, platformUserID, regionStr)
			snapshotDebugLogger.Debugf("binding resolve region fallback result: platform=%s user=%s region=%s hid=%d binding=%s err=%v",
				strings.TrimSpace(platform), maskDebugID(platformUserID), strings.TrimSpace(regionStr), hid, formatBindingDebug(binding), err)
		}
	} else {
		hid, binding, err = bindings.ResolveUserBinding(ctx, platform, platformUserID, regionStr)
		snapshotDebugLogger.Debugf("binding resolve explicit-region result: platform=%s user=%s region=%s hid=%d binding=%s err=%v",
			strings.TrimSpace(platform), maskDebugID(platformUserID), strings.TrimSpace(regionStr), hid, formatBindingDebug(binding), err)
	}

	if err != nil || !isValid(binding) {
		if err == nil {
			err = fmt.Errorf("binding invalid for requested private data flags")
		}
		snapshotDebugLogger.Warnf("binding resolve failed: platform=%s user=%s region=%s region_explicit=%t selector=%q hid=%d binding=%s err=%v",
			strings.TrimSpace(platform), maskDebugID(platformUserID), strings.TrimSpace(regionStr), regionExplicit, strings.TrimSpace(opts.Selector), hid, formatBindingDebug(binding), err)
		return 0, nil, nil
	}
	snapshotDebugLogger.Debugf("binding resolve succeeded: platform=%s user=%s region=%s hid=%d binding=%s",
		strings.TrimSpace(platform), maskDebugID(platformUserID), strings.TrimSpace(regionStr), hid, formatBindingDebug(binding))
	return hid, binding, nil
}

func formatBindingDebug(binding *accountdata.ResolvedBinding) string {
	if binding == nil {
		return "<nil>"
	}
	return fmt.Sprintf("{binding_id=%d server=%s pjsk_user=%s visible=%t suite_visible=%t mysekai_visible=%t verified=%t}",
		binding.BindingID, strings.TrimSpace(binding.Server), maskDebugID(binding.PJSKUserID), binding.Visible, binding.SuiteVisible, binding.MySekaiVisible, binding.Verified)
}

func maskDebugID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 6 {
		return value
	}
	return value[:3] + "***" + value[len(value)-3:]
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
