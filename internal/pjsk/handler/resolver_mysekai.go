package handler

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

type mySekaiRenderContextOptions struct {
	NeedProfile          bool
	PreferMySekaiPayload bool
	MySekaiPayloadOnly   bool
	SuiteOnlySnapshot    bool
}

func resolveMySekaiPayloadBySelector(ctx context.Context, app *renderapp.App, selector snapshot.Selector, preferGlobalDefault bool) ([]byte, error) {
	if app == nil || app.MySekaiPayloads == nil {
		return nil, nil
	}
	payload, err := app.MySekaiPayloads.Resolve(ctx, selector, preferGlobalDefault)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, nil
	}
	return payload, nil
}

func resolveMySekaiRenderContext(
	ctx context.Context,
	app *renderapp.App,
	params userQueryParams,
	regionStr string,
	regionExplicit bool,
) (mySekaiRenderContext, error) {
	return resolveMySekaiRenderContextWithOptions(ctx, app, params, regionStr, regionExplicit, mySekaiRenderContextOptions{
		NeedProfile: true,
	})
}

func resolveMySekaiRenderContextWithOptions(
	ctx context.Context,
	app *renderapp.App,
	params userQueryParams,
	regionStr string,
	regionExplicit bool,
	opts mySekaiRenderContextOptions,
) (mySekaiRenderContext, error) {
	if app == nil || app.MySekai == nil {
		return mySekaiRenderContext{}, fmt.Errorf("mysekai service unavailable: mysekai controller is not configured")
	}

	result := mySekaiRenderContext{Controller: app.MySekai.WithContext(ctx), Region: regionWithDefault(regionStr)}
	if app.Bindings == nil || strings.TrimSpace(params.Platform) == "" || strings.TrimSpace(params.PlatformUserID) == "" {
		return result, nil
	}

	target, err := resolveGameTarget(ctx, params, regionStr, regionExplicit, app)
	if err != nil {
		return mySekaiRenderContext{}, err
	}
	regionStr = resolvedTargetRegion(regionStr, target)
	result.Region = regionStr
	result.HarukiUserID = target.HarukiUserID

	platform, platformUserID := platformCredentials(params)
	if handled, preferredResult, preferredErr := tryPreferredMySekaiPayload(ctx, app, result, target, platform, platformUserID, regionStr, opts); handled {
		return preferredResult, preferredErr
	}

	snap, snapshotErr := resolveTargetSnapshotWithError(ctx, app, regionStr, platform, platformUserID, target.PJSKUserID, !opts.SuiteOnlySnapshot)
	if snapshotErr != nil {
		return mySekaiRenderContext{}, normalizeToolboxDataFetchError(snapshotErr, mySekaiSnapshotLabel(opts), target.Binding)
	}
	if snap != nil {
		result.Controller = result.Controller.WithSnapshot(snap)
		if err := resolveMySekaiProfile(ctx, app, &result, target, regionStr, snap, opts.NeedProfile); err != nil {
			return mySekaiRenderContext{}, err
		}
		return result, nil
	}
	return resolveMySekaiContextWithoutSnapshot(ctx, app, result, target, platform, platformUserID, regionStr, opts)
}

func resolveMySekaiContextWithoutSnapshot(
	ctx context.Context,
	app *renderapp.App,
	result mySekaiRenderContext,
	target ResolvedGameTarget,
	platform string,
	platformUserID string,
	regionStr string,
	opts mySekaiRenderContextOptions,
) (mySekaiRenderContext, error) {
	if err := resolveMySekaiProfile(ctx, app, &result, target, regionStr, nil, opts.NeedProfile); err != nil {
		return mySekaiRenderContext{}, err
	}
	if !opts.PreferMySekaiPayload {
		resolved, payloadErr := attachMySekaiPayload(ctx, app, &result, target, platform, platformUserID, regionStr)
		if payloadErr != nil {
			return mySekaiRenderContext{}, payloadErr
		}
		if resolved {
			return result, nil
		}
	}
	if target.Binding != nil {
		return mySekaiRenderContext{}, newMySekaiDataNotFoundReplayErrorForBinding(target.Binding)
	}
	return result, nil
}

func tryPreferredMySekaiPayload(
	ctx context.Context,
	app *renderapp.App,
	result mySekaiRenderContext,
	target ResolvedGameTarget,
	platform string,
	platformUserID string,
	regionStr string,
	opts mySekaiRenderContextOptions,
) (bool, mySekaiRenderContext, error) {
	if !opts.PreferMySekaiPayload {
		return false, result, nil
	}
	resolved, err := attachMySekaiPayload(ctx, app, &result, target, platform, platformUserID, regionStr)
	if err != nil {
		return true, mySekaiRenderContext{}, err
	}
	if resolved {
		err = resolveMySekaiProfile(ctx, app, &result, target, regionStr, nil, opts.NeedProfile)
		if err != nil {
			return true, mySekaiRenderContext{}, err
		}
		return true, result, nil
	}
	if !opts.MySekaiPayloadOnly {
		return false, result, nil
	}
	if target.Binding != nil {
		return true, mySekaiRenderContext{}, newMySekaiDataNotFoundReplayErrorForBinding(target.Binding)
	}
	return true, result, nil
}

func attachMySekaiPayload(
	ctx context.Context,
	app *renderapp.App,
	result *mySekaiRenderContext,
	target ResolvedGameTarget,
	platform string,
	platformUserID string,
	regionStr string,
) (bool, error) {
	data, err := resolveMySekaiPayloadBySelector(ctx, app, snapshot.Selector{
		IMPlatform: strings.TrimSpace(platform),
		IMUserID:   strings.TrimSpace(platformUserID),
		Region:     renderregion.Normalize(regionStr),
		PJSKUserID: strings.TrimSpace(target.PJSKUserID),
	}, false)
	if err != nil {
		return false, normalizeToolboxDataFetchError(err, "mysekai", target.Binding)
	}
	if len(data) == 0 {
		return false, nil
	}
	result.Controller = result.Controller.WithMySekaiData(data)
	return result.Controller != nil, nil
}

func resolveMySekaiProfile(
	ctx context.Context,
	app *renderapp.App,
	result *mySekaiRenderContext,
	target ResolvedGameTarget,
	regionStr string,
	snap snapshot.Snapshot,
	needed bool,
) error {
	if !needed {
		return nil
	}
	profile, err := buildPublicProfileCardForTarget(ctx, target, regionStr, app, snap)
	if err != nil {
		return normalizeSekaiAPIFetchError(err)
	}
	result.Profile = forceMySekaiProfileBindingID(profile, target, regionStr)
	if result.Profile == nil && snap != nil {
		result.Profile = forceMySekaiProfileBindingID(snap.ProfileCard(renderregion.Normalize(regionStr)), target, regionStr)
	}
	return nil
}

func mySekaiSnapshotLabel(opts mySekaiRenderContextOptions) string {
	if opts.SuiteOnlySnapshot {
		return "suite"
	}
	return "mysekai"
}

func forceMySekaiProfileBindingID(profile *drawing.ProfileCardRequest, target ResolvedGameTarget, regionStr string) *drawing.ProfileCardRequest {
	if profile == nil {
		return nil
	}

	cloned := cloneProfileCardForTarget(profile, target, regionStr)
	if cloned == nil {
		return nil
	}

	uid := strings.TrimSpace(target.PJSKUserID)
	normalizedRegion := renderregion.Normalize(regionStr)
	if profile.Profile == nil {
		if uid == "" && normalizedRegion.IsZero() {
			return cloned
		}
		cloned.Profile = &drawing.BasicProfile{}
	}

	if uid != "" {
		cloned.Profile.ID = uid
	}
	if !normalizedRegion.IsZero() {
		cloned.Profile.Region = strings.ToUpper(normalizedRegion.String())
	}
	if len(profile.DataSources) > 0 {
		cloned.DataSources = slices.Clone(profile.DataSources)
	}
	return cloned
}
