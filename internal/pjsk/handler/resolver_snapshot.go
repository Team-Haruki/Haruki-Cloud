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
	provider = provider.WithPrivateDataCache(app.PrivateDataCache).WithBuiltSnapshotCache(app.BuiltSnapshotCache)
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
	snap, _ := resolveTargetSnapshotWithError(ctx, app, regionStr, platform, platformUserID, pjskUserID, needMySekai)
	return snap
}

func resolveTargetSnapshotWithError(
	ctx context.Context,
	app *renderapp.App,
	regionStr string,
	platform string,
	platformUserID string,
	pjskUserID string,
	needMySekai bool,
) (snapshot.Snapshot, error) {
	return resolveSnapshotBySelectorWithError(ctx, app, snapshot.Selector{
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

type bindingResolutionState struct {
	options        bindingResolutionOptions
	invalidHID     int
	invalidBinding *accountdata.ResolvedBinding
	unexpectedErr  error
	sawNoBinding   bool
}

func (s *bindingResolutionState) record(hid int, binding *accountdata.ResolvedBinding, err error) bool {
	if err == nil && s.valid(binding) {
		return true
	}
	if err == nil {
		if binding == nil {
			s.sawNoBinding = true
		} else {
			s.invalidHID = hid
			s.invalidBinding = binding
		}
		return false
	}
	if errors.Is(err, accountdata.ErrNoBinding) {
		s.sawNoBinding = true
	} else {
		s.unexpectedErr = err
	}
	return false
}

func (s *bindingResolutionState) valid(binding *accountdata.ResolvedBinding) bool {
	if binding == nil {
		return false
	}
	if s.options.RequireSuite && !hasUsableSuiteData(binding) {
		return false
	}
	return !s.options.RequireMySekai || hasUsableMySekaiData(binding)
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

	state := bindingResolutionState{options: opts}
	if opts.Selector != "" {
		hid, binding, err := bindings.ResolveUserBindingBySelector(ctx, platform, platformUserID, selectorBindingServer(regionStr, regionExplicit), opts.Selector)
		if state.record(hid, binding, err) {
			return hid, binding, nil
		}
	} else {
		scopes := []string{regionStr}
		if !regionExplicit {
			scopes = append([]string{accountdata.GlobalDefaultBindingScope}, scopes...)
		}
		for _, scope := range scopes {
			hid, binding, err := bindings.ResolveUserBinding(ctx, platform, platformUserID, scope)
			if state.record(hid, binding, err) {
				return hid, binding, nil
			}
		}
	}
	return state.result(ctx, regionStr, regionExplicit)
}

func (s *bindingResolutionState) result(ctx context.Context, regionStr string, regionExplicit bool) (int, *accountdata.ResolvedBinding, error) {
	switch {
	case s.unexpectedErr != nil:
		snapshotDebugLogger.WarnContext(ctx, "binding resolve failed",
			"region", strings.TrimSpace(regionStr),
			"region_explicit", regionExplicit,
			"has_selector", strings.TrimSpace(s.options.Selector) != "",
			"error_type", fmt.Sprintf("%T", s.unexpectedErr),
		)
		return 0, nil, s.unexpectedErr
	case s.invalidBinding != nil:
		snapshotDebugLogger.DebugContext(ctx, "binding resolve missing required private data",
			"region", strings.TrimSpace(regionStr),
			"region_explicit", regionExplicit,
			"has_selector", strings.TrimSpace(s.options.Selector) != "",
		)
		return s.invalidHID, s.invalidBinding, nil
	case s.sawNoBinding:
		snapshotDebugLogger.DebugContext(ctx, "binding resolve completed",
			"outcome", "not_found",
			"region", strings.TrimSpace(regionStr),
			"region_explicit", regionExplicit,
			"has_selector", strings.TrimSpace(s.options.Selector) != "",
		)
		return 0, nil, accountdata.ErrNoBinding
	default:
		snapshotDebugLogger.DebugContext(ctx, "binding resolve completed",
			"outcome", "invalid_private_data",
			"region", strings.TrimSpace(regionStr),
			"region_explicit", regionExplicit,
			"has_selector", strings.TrimSpace(s.options.Selector) != "",
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
