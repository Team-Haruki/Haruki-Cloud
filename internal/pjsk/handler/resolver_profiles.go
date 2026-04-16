package handler

import (
	"context"
	"errors"

	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/profile"
	"haruki-cloud/internal/pjsk/render/userdata"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func resolveCardBoxDetailedProfile(rc *RequestContext) *drawing.DetailedProfileCardRequest {
	if rc == nil || rc.App == nil {
		return nil
	}
	if snapshot := resolveLiveSnapshot(rc, false); snapshot != nil {
		if detail := snapshot.DetailedProfile(rc.Region); detail != nil && len(detail.UserCards) > 0 {
			return detail
		}
	}
	return nil
}

func resolveCardCatalogTitle(rc *RequestContext) *string {
	if rc == nil || rc.App == nil {
		return nil
	}
	if rc.Platform == "" || rc.PlatformUserID == "" {
		return nil
	}

	binding, _ := rc.GetBinding()
	if binding == nil {
		if errors.Is(rc.bindingErr, accountdata.ErrNoBinding) {
			return stringPtr(CardCatalogTitleNoBinding)
		}
		return nil
	}
	if !binding.SuiteVisible {
		return stringPtr(CardCatalogTitleNoSuite)
	}

	snapshot := rc.ResolveSnapshot(false)
	if snapshot == nil {
		return stringPtr(CardCatalogTitleNoSuite)
	}
	detail := snapshot.DetailedProfile(rc.Region)
	if detail == nil || len(detail.UserCards) == 0 {
		return stringPtr(CardCatalogTitleNoSuite)
	}
	return nil
}

func buildPublicMusicProfiles(rc *RequestContext) (*drawing.DetailedProfileCardRequest, *drawing.ProfileCardRequest) {
	if rc == nil || rc.App == nil || rc.App.Profiles == nil || rc.App.Bindings == nil {
		return nil, nil
	}
	if rc.Platform == "" || rc.PlatformUserID == "" {
		return nil, nil
	}

	queryParams := rc.requestScopedSelfQuery()
	target, err := resolveGameTarget(rc.Ctx, queryParams, rc.RegionStr, rc.Cmd.RegionExplicit, rc.App)
	if err != nil {
		return nil, nil
	}
	region := resolvedTargetRegion(rc.RegionStr, target)

	resp, err := rc.App.SekaiAPI.GetUserProfile(region, target.PJSKUserID)
	if err != nil {
		return nil, nil
	}

	return buildPublicMusicProfilesFromResolvedTarget(rc.Ctx, target, region, rc.Platform, rc.PlatformUserID, resp, rc.App)
}

func buildPublicMusicProfilesFromResolvedTarget(
	ctx context.Context,
	target resolvedGameTarget,
	region string,
	platform string,
	platformUserID string,
	resp *sekaiapi.GetAnotherProfileResponse,
	app *renderapp.App,
) (*drawing.DetailedProfileCardRequest, *drawing.ProfileCardRequest) {
	if app == nil || app.Profiles == nil || resp == nil {
		return nil, nil
	}
	region = resolvedTargetRegion(region, target)

	var profileSnapshot userdata.Snapshot
	if hasUsableSuiteData(target.Binding) {
		profileSnapshot = resolveTargetSnapshot(ctx, app, region, platform, platformUserID, target.PJSKUserID, false)
	}

	q := profile.Query{
		Region:     region,
		Visible:    target.Visible,
		BgSettings: target.BgSettings,
	}
	detail, err := app.Profiles.BuildDetailedProfileCardFromAPIWithSnapshot(q, resp, profileSnapshot)
	if err != nil {
		return nil, nil
	}
	card, err := app.Profiles.BuildProfileCardFromAPIWithSnapshot(q, resp, profileSnapshot)
	if err != nil {
		return detail, nil
	}
	return detail, card
}

// buildPublicProfileCardForTarget builds a ProfileCardRequest for a resolved
// game target. Used by mysekai commands where the target is already resolved
// through userQueryParams (supporting u[i] selectors and region binding).
func buildPublicProfileCardForTarget(ctx context.Context, target resolvedGameTarget, region, platform, platformUserID string, app *renderapp.App) *drawing.ProfileCardRequest {
	if app == nil || app.Profiles == nil {
		return nil
	}
	region = resolvedTargetRegion(region, target)

	resp, err := app.SekaiAPI.GetUserProfile(region, target.PJSKUserID)
	if err != nil {
		return nil
	}
	_, card := buildPublicMusicProfilesFromResolvedTarget(ctx, target, region, platform, platformUserID, resp, app)
	return card
}
