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

func resolveTargetMySekaiPayload(
	ctx context.Context,
	app *renderapp.App,
	regionStr string,
	platform string,
	platformUserID string,
	pjskUserID string,
) []byte {
	data, _ := resolveMySekaiPayloadBySelector(ctx, app, snapshot.Selector{
		IMPlatform: strings.TrimSpace(platform),
		IMUserID:   strings.TrimSpace(platformUserID),
		Region:     renderregion.Normalize(regionStr),
		PJSKUserID: strings.TrimSpace(pjskUserID),
	}, false)
	return data
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
	if snap := resolveTargetSnapshot(ctx, app, regionStr, platform, platformUserID, target.PJSKUserID, true); snap != nil {
		result.Controller = result.Controller.WithSnapshot(snap)
		result.Profile = forceMySekaiProfileBindingID(snap.ProfileCard(renderregion.Normalize(regionStr)), target, regionStr)
		if result.Profile == nil {
			result.Profile = forceMySekaiProfileBindingID(buildPublicProfileCardForTarget(ctx, target, regionStr, platform, platformUserID, app), target, regionStr)
		}
		return result, nil
	}

	result.Profile = forceMySekaiProfileBindingID(buildPublicProfileCardForTarget(ctx, target, regionStr, platform, platformUserID, app), target, regionStr)
	if data, payloadErr := resolveMySekaiPayloadBySelector(ctx, app, snapshot.Selector{
		IMPlatform: strings.TrimSpace(platform),
		IMUserID:   strings.TrimSpace(platformUserID),
		Region:     renderregion.Normalize(regionStr),
		PJSKUserID: strings.TrimSpace(target.PJSKUserID),
	}, false); payloadErr != nil {
		return mySekaiRenderContext{}, normalizeToolboxDataFetchError(payloadErr, "mysekai", target.Binding)
	} else if len(data) > 0 {
		result.Controller = result.Controller.WithMySekaiData(data)
		return result, nil
	}
	if target.Binding != nil {
		return mySekaiRenderContext{}, newMySekaiDataNotFoundReplayErrorForBinding(target.Binding)
	}
	return result, nil
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
