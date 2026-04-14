package handler

import (
	"context"
	"fmt"
	"strings"

	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
)

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

	result := mySekaiRenderContext{Controller: app.MySekai, Region: regionWithDefault(regionStr)}
	if app.Bindings == nil || strings.TrimSpace(params.Platform) == "" || strings.TrimSpace(params.PlatformUserID) == "" {
		return result, nil
	}

	target, err := resolveGameTarget(ctx, params, regionStr, regionExplicit, app)
	if err != nil {
		return mySekaiRenderContext{}, err
	}
	regionStr = resolvedTargetRegion(regionStr, target)
	result.Region = regionStr

	platform, platformUserID := platformCredentials(params)
	if snapshot := resolveTargetSnapshot(ctx, app, regionStr, platform, platformUserID, target.PJSKUserID, true); snapshot != nil {
		result.Controller = app.MySekai.WithSnapshot(snapshot)
		result.Profile = snapshot.ProfileCard(renderregion.Normalize(regionStr))
		if result.Profile == nil {
			result.Profile = buildPublicProfileCardForTarget(ctx, target, regionStr, platform, platformUserID, app)
		}
		return result, nil
	}

	result.Profile = buildPublicProfileCardForTarget(ctx, target, regionStr, platform, platformUserID, app)
	if data := resolveTargetMySekaiPayload(ctx, app, regionStr, platform, platformUserID, target.PJSKUserID); len(data) > 0 {
		result.Controller = app.MySekai.WithMySekaiData(data)
	}
	return result, nil
}
