package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/snapshot"

	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/utils/logger"
)

var snapshotDebugLogger = logger.NewLoggerFromGlobal("PJSKSnapshot")

func resolveSnapshotBySelector(ctx context.Context, app *renderapp.App, selector snapshot.Selector, opts snapshot.ResolveOptions) snapshot.Snapshot {
	snap, _ := resolveSnapshotBySelectorWithError(ctx, app, selector, opts)
	return snap
}

func resolveSnapshotBySelectorWithError(ctx context.Context, app *renderapp.App, selector snapshot.Selector, opts snapshot.ResolveOptions) (snapshot.Snapshot, error) {
	providerApp := app
	if app != nil {
		contextualApp := *app
		contextualApp.Assets = app.Assets.WithContext(ctx)
		providerApp = &contextualApp
	}
	provider := snapshotProviderFactory(providerApp)
	if provider == nil {
		snapshotDebugLogger.DebugContext(ctx, "snapshot resolve skipped",
			"reason", "provider_unavailable",
			"region", selector.Region.String(),
			"need_mysekai", opts.NeedMySekai,
			"prefer_global_default", opts.PreferGlobalDefault,
		)
		return nil, nil
	}
	tProvider := time.Now()
	snap, err := provider.Resolve(ctx, selector, opts)
	recordCommandStage(ctx, "snapshot.provider", time.Since(tProvider))
	if err != nil {
		snapshotDebugLogger.WarnContext(ctx, "snapshot resolve failed",
			"region", selector.Region.String(),
			"need_mysekai", opts.NeedMySekai,
			"prefer_global_default", opts.PreferGlobalDefault,
			"error_type", fmt.Sprintf("%T", err),
		)
		return nil, err
	}
	snapshotDebugLogger.DebugContext(ctx, "snapshot resolve succeeded",
		"region", selector.Region.String(),
		"need_mysekai", opts.NeedMySekai,
		"prefer_global_default", opts.PreferGlobalDefault,
	)
	return snap, nil
}

var snapshotProviderFactory = defaultSnapshotProviderFactory

func defaultSnapshotProviderFactory(app *renderapp.App) snapshot.HarukiSnapshotProvider {
	if app == nil {
		return nil
	}

	providers := make([]snapshot.HarukiSnapshotProvider, 0, 2)
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
		return snapshot.NewFallbackSnapshotProvider(app.Config.UserSnapshot.AllowFallback, providers...)
	}
}

func liveSnapshotProvider(app *renderapp.App) snapshot.HarukiSnapshotProvider {
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
	provider := snapshot.NewToolboxSnapshotProvider(app.Bindings, app.Toolbox, app.Sekai, app.Assets)
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
) snapshot.Snapshot {
	return resolveSnapshotBySelector(ctx, app, snapshot.Selector{
		IMPlatform: strings.TrimSpace(platform),
		IMUserID:   strings.TrimSpace(platformUserID),
		Region:     renderregion.Normalize(regionStr),
		PJSKUserID: strings.TrimSpace(pjskUserID),
	}, snapshot.ResolveOptions{
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
// Returns accountdata.ErrNoBinding when the user has no matching binding.
// Returns accountdata.ErrBindingServiceUnavailable when the binding service is unavailable.
// When RequireSuite/RequireMySekai is set, the function prefers a binding with
// usable private data, but still returns the best resolved binding when all
// candidates exist yet fail the private-data check so callers can surface a
// more specific "no suite/mysekai data" message.
func resolveBindingWithFallback(
	ctx context.Context,
	bindings *accountdata.BindingService,
	platform, platformUserID, regionStr string,
	regionExplicit bool,
	opts bindingResolutionOptions,
) (int, *accountdata.ResolvedBinding, error) {
	if bindings == nil {
		return 0, nil, accountdata.ErrBindingServiceUnavailable
	}
	if platform == "" || platformUserID == "" {
		return 0, nil, nil
	}

	snapshotDebugLogger.DebugContext(ctx, "binding resolve started",
		"region", strings.TrimSpace(regionStr),
		"region_explicit", regionExplicit,
		"has_selector", strings.TrimSpace(opts.Selector) != "",
		"require_suite", opts.RequireSuite,
		"require_mysekai", opts.RequireMySekai,
	)

	var hid int
	var binding *accountdata.ResolvedBinding
	var err error
	var invalidHID int
	var invalidBinding *accountdata.ResolvedBinding
	var unexpectedErr error
	var sawNoBinding bool

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

	recordAttempt := func(attemptHID int, attemptBinding *accountdata.ResolvedBinding, attemptErr error) (int, *accountdata.ResolvedBinding, error, bool) {
		if attemptErr == nil && isValid(attemptBinding) {
			return attemptHID, attemptBinding, nil, true
		}
		if attemptErr == nil {
			if attemptBinding != nil {
				invalidHID = attemptHID
				invalidBinding = attemptBinding
			} else {
				sawNoBinding = true
			}
			return 0, nil, nil, false
		}
		if errors.Is(attemptErr, accountdata.ErrNoBinding) {
			sawNoBinding = true
			return 0, nil, nil, false
		}
		unexpectedErr = attemptErr
		return 0, nil, attemptErr, false
	}

	if opts.Selector != "" {
		hid, binding, err = bindings.ResolveUserBindingBySelector(ctx, platform, platformUserID, selectorBindingServer(regionStr, regionExplicit), opts.Selector)
		if resolvedHID, resolvedBinding, resolvedErr, ok := recordAttempt(hid, binding, err); ok {
			return resolvedHID, resolvedBinding, resolvedErr
		}
	} else if !regionExplicit {
		hid, binding, err = bindings.ResolveUserBinding(ctx, platform, platformUserID, accountdata.GlobalDefaultBindingScope)
		if resolvedHID, resolvedBinding, resolvedErr, ok := recordAttempt(hid, binding, err); ok {
			return resolvedHID, resolvedBinding, resolvedErr
		}
		hid, binding, err = bindings.ResolveUserBinding(ctx, platform, platformUserID, regionStr)
		if resolvedHID, resolvedBinding, resolvedErr, ok := recordAttempt(hid, binding, err); ok {
			return resolvedHID, resolvedBinding, resolvedErr
		}
	} else {
		hid, binding, err = bindings.ResolveUserBinding(ctx, platform, platformUserID, regionStr)
		if resolvedHID, resolvedBinding, resolvedErr, ok := recordAttempt(hid, binding, err); ok {
			return resolvedHID, resolvedBinding, resolvedErr
		}
	}

	switch {
	case unexpectedErr != nil:
		snapshotDebugLogger.WarnContext(ctx, "binding resolve failed",
			"region", strings.TrimSpace(regionStr),
			"region_explicit", regionExplicit,
			"has_selector", strings.TrimSpace(opts.Selector) != "",
			"error_type", fmt.Sprintf("%T", unexpectedErr),
		)
		return 0, nil, unexpectedErr
	case invalidBinding != nil:
		snapshotDebugLogger.DebugContext(ctx, "binding resolve missing required private data",
			"region", strings.TrimSpace(regionStr),
			"region_explicit", regionExplicit,
			"has_selector", strings.TrimSpace(opts.Selector) != "",
		)
		return invalidHID, invalidBinding, nil
	case sawNoBinding:
		snapshotDebugLogger.DebugContext(ctx, "binding resolve completed",
			"outcome", "not_found",
			"region", strings.TrimSpace(regionStr),
			"region_explicit", regionExplicit,
			"has_selector", strings.TrimSpace(opts.Selector) != "",
		)
		return 0, nil, accountdata.ErrNoBinding
	default:
		snapshotDebugLogger.DebugContext(ctx, "binding resolve completed",
			"outcome", "invalid_private_data",
			"region", strings.TrimSpace(regionStr),
			"region_explicit", regionExplicit,
			"has_selector", strings.TrimSpace(opts.Selector) != "",
		)
		return 0, nil, nil
	}
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
