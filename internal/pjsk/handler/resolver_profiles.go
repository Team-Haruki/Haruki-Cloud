package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/profile"
	"haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func resolveCardBoxDetailedProfile(rc *RequestContext) *drawing.DetailedProfileCardRequest {
	if rc == nil || rc.App == nil {
		return nil
	}
	if snap := resolveLiveSnapshot(rc, false); snap != nil {
		if detail := snap.DetailedProfile(rc.Region); detail != nil && len(detail.UserCards) > 0 {
			return cloneDetailedProfileForCurrentTarget(rc, detail)
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
		if rc.bindingErr == nil || errors.Is(rc.bindingErr, accountdata.ErrNoBinding) {
			return stringPtr(CardCatalogTitleNoBinding)
		}
		return nil
	}
	if !binding.SuiteVisible {
		return stringPtr(CardCatalogTitleNoSuite)
	}

	snap := rc.ResolveSnapshot(false)
	if snap == nil {
		return stringPtr(CardCatalogTitleNoSuite)
	}
	detail := snap.DetailedProfile(rc.Region)
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

	resp, err := fetchCachedSekaiUserProfile(rc.Ctx, rc.App, region, target.PJSKUserID)
	if err != nil {
		return nil, nil
	}

	return buildPublicMusicProfilesFromResolvedTarget(rc.Ctx, target, region, rc.Platform, rc.PlatformUserID, resp, rc.App)
}

func resolveCurrentTargetPublicProfiles(rc *RequestContext) (*drawing.DetailedProfileCardRequest, *drawing.ProfileCardRequest) {
	if rc == nil {
		return nil, nil
	}
	target := rc.GetSelfTarget()
	if target == nil {
		return nil, nil
	}
	resp := rc.GetPublicProfileResponse()
	if resp == nil {
		return nil, nil
	}
	region := resolvedTargetRegion(rc.RegionStr, *target)
	return buildPublicMusicProfilesFromResolvedTarget(rc.Ctx, *target, region, rc.Platform, rc.PlatformUserID, resp, rc.App)
}

func cloneDetailedProfileForTarget(detail *drawing.DetailedProfileCardRequest, target ResolvedGameTarget, region string) *drawing.DetailedProfileCardRequest {
	if detail == nil {
		return nil
	}
	cloned := *detail
	cloned.Mode = commonCloneStringPtr(detail.Mode)
	cloned.FramePath = commonCloneStringPtr(detail.FramePath)
	cloned.UserCards = append([]any(nil), detail.UserCards...)
	cloned.IsHideUID = !target.Visible
	if resolvedRegion := strings.TrimSpace(resolvedTargetRegion(region, target)); resolvedRegion != "" {
		cloned.Region = strings.ToUpper(resolvedRegion)
	}
	return &cloned
}

func cloneDetailedProfileForCurrentTarget(rc *RequestContext, detail *drawing.DetailedProfileCardRequest) *drawing.DetailedProfileCardRequest {
	if rc == nil || detail == nil {
		return detail
	}
	target := rc.GetSelfTarget()
	if target == nil {
		return detail
	}
	return cloneDetailedProfileForTarget(detail, *target, rc.RegionStr)
}

func cloneProfileCardForCurrentTarget(rc *RequestContext, card *drawing.ProfileCardRequest) *drawing.ProfileCardRequest {
	if rc == nil || card == nil {
		return card
	}
	target := rc.GetSelfTarget()
	if target == nil {
		return card
	}
	return cloneProfileCardForTarget(card, *target, rc.RegionStr)
}

func cloneProfileCardForTarget(card *drawing.ProfileCardRequest, target ResolvedGameTarget, region string) *drawing.ProfileCardRequest {
	if card == nil {
		return nil
	}
	cloned := *card
	if card.Profile != nil {
		profile := *card.Profile
		profile.FramePath = commonCloneStringPtr(card.Profile.FramePath)
		profile.IsHideUID = !target.Visible
		if resolvedRegion := strings.TrimSpace(resolvedTargetRegion(region, target)); resolvedRegion != "" {
			profile.Region = strings.ToUpper(resolvedRegion)
		}
		cloned.Profile = &profile
	}
	if len(card.DataSources) > 0 {
		cloned.DataSources = make([]drawing.ProfileDataSource, 0, len(card.DataSources))
		for _, item := range card.DataSources {
			entry := item
			entry.Source = commonCloneStringPtr(item.Source)
			entry.Mode = commonCloneStringPtr(item.Mode)
			if item.UpdateTime != nil {
				entry.UpdateTime = new(int64)
				*entry.UpdateTime = *item.UpdateTime
			}
			cloned.DataSources = append(cloned.DataSources, entry)
		}
	}
	if card.MysekaiLevel != nil {
		cloned.MysekaiLevel = new(int)
		*cloned.MysekaiLevel = *card.MysekaiLevel
	}
	cloned.ErrorMessage = commonCloneStringPtr(card.ErrorMessage)
	return &cloned
}

func resolveCommandDisplayProfiles(rc *RequestContext, snap snapshot.Snapshot) (*drawing.DetailedProfileCardRequest, *drawing.ProfileCardRequest) {
	detail, card := resolveCurrentTargetPublicProfiles(rc)
	target := rc.GetSelfTarget()

	if target != nil && snap != nil {
		if detail == nil {
			detail = cloneDetailedProfileForTarget(snap.DetailedProfile(rc.Region), *target, rc.RegionStr)
		}
		if card == nil {
			card = cloneProfileCardForTarget(snap.ProfileCard(rc.Region), *target, rc.RegionStr)
		}
	}

	if detail == nil {
		detail = rc.GetDetailedProfile()
	}
	if card == nil {
		card = rc.GetProfileCard()
	}
	return detail, card
}

func commonCloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func buildPublicMusicProfilesFromResolvedTarget(
	ctx context.Context,
	target ResolvedGameTarget,
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

	var profileSnapshot snapshot.Snapshot
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
func buildPublicProfileCardForTarget(ctx context.Context, target ResolvedGameTarget, region string, app *renderapp.App, profileSnapshot snapshot.Snapshot) (*drawing.ProfileCardRequest, error) {
	if app == nil || app.Profiles == nil {
		return nil, nil
	}
	region = resolvedTargetRegion(region, target)

	resp, err := fetchCachedSekaiUserProfile(ctx, app, region, target.PJSKUserID)
	if err != nil {
		return nil, fmt.Errorf("sekaiapi profile fetch failed: %w", err)
	}
	q := profile.Query{
		Region:     region,
		Visible:    target.Visible,
		BgSettings: target.BgSettings,
	}
	var (
		card     *drawing.ProfileCardRequest
		buildErr error
	)
	if profileSnapshot != nil {
		card, buildErr = app.Profiles.BuildProfileCardFromAPIWithSnapshot(q, resp, profileSnapshot)
	} else {
		card, buildErr = app.Profiles.BuildProfileCardFromAPI(q, resp, nil)
	}
	if buildErr != nil {
		return nil, fmt.Errorf("sekaiapi profile build failed: %w", buildErr)
	}
	return card, nil
}
