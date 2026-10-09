package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/profile"
	"haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/usererror"
)

func resolveCardBoxDetailedProfile(rc *RequestContext) *drawing.DetailedProfileCardRequest {
	if rc == nil || rc.App == nil {
		return nil
	}
	if snap := rc.ResolveSnapshot(false); snap != nil {
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
			return stringPtr(i18n.T("card.catalog_notice.no_binding"))
		}
		return nil
	}
	if !binding.SuiteVisible {
		return stringPtr(i18n.T("card.catalog_notice.no_suite"))
	}

	snap := rc.ResolveSnapshot(false)
	if snap == nil {
		if snapshotErr := rc.SnapshotError(false); snapshotErr != nil {
			return stringPtr(cardCatalogSnapshotErrorTitle(snapshotErr, binding))
		}
		return stringPtr(i18n.T("card.catalog_notice.no_suite"))
	}
	detail := snap.DetailedProfile(rc.Region)
	if detail == nil || len(detail.UserCards) == 0 {
		return stringPtr(i18n.T("card.catalog_notice.no_suite"))
	}
	return nil
}

func cardCatalogSnapshotErrorTitle(err error, binding *accountdata.ResolvedBinding) string {
	if errors.Is(err, sekaiapi.ErrGameDataNotFound) || errors.Is(err, sekaiapi.ErrAccountBindingNotFound) {
		return i18n.T("card.catalog_notice.no_suite")
	}
	typed, ok := usererror.As(normalizeToolboxDataFetchError(err, privateDataSuite, binding))
	if !ok || typed.Code != usererror.CodeSetup && typed.Code != usererror.CodeForbidden {
		return i18n.T("card.catalog_notice.suite_unavailable")
	}
	reason := strings.TrimSpace(strings.SplitN(typed.Error(), "\n", 2)[0])
	return i18n.T("card.catalog_notice.with_reason", i18n.Data{"Reason": reason})
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
	if detail.FramePaths != nil {
		cloned.FramePaths = new(*detail.FramePaths)
	}
	cloned.Rank = commonCloneIntPtr(detail.Rank)
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
		if card.Profile.FramePaths != nil {
			profile.FramePaths = new(*card.Profile.FramePaths)
		}
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
	cloned.Rank = commonCloneIntPtr(card.Rank)
	cloned.MysekaiLevel = commonCloneIntPtr(card.MysekaiLevel)
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

func commonCloneIntPtr(value *int) *int {
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
	profileCtrl := app.Profiles.WithContext(ctx)
	finishBuild := measurePayloadBuild(ctx)
	detail, err := profileCtrl.BuildDetailedProfileCardFromAPIWithSnapshot(q, resp, profileSnapshot)
	finishBuild()
	if err != nil {
		return nil, nil
	}
	finishBuild = measurePayloadBuild(ctx)
	card, err := profileCtrl.BuildProfileCardFromAPIWithSnapshot(q, resp, profileSnapshot)
	finishBuild()
	if err != nil {
		return detail, nil
	}
	return detail, card
}

// buildPublicProfileCardForTarget builds a ProfileCardRequest for a resolved
// game target. Used by mysekai commands where the target is already resolved
// through userQueryParams (supporting u[i] selectors and region binding).
func buildPublicProfileCardForTarget(ctx context.Context, target ResolvedGameTarget, region string, app *renderapp.App, profileSnapshot snapshot.Snapshot) (*drawing.ProfileCardRequest, error) {
	return buildPublicProfileCardForTargetWithPrefetch(ctx, target, region, app, profileSnapshot, nil)
}

// buildPublicProfileCardForTargetWithPrefetch is buildPublicProfileCardForTarget
// that consumes an already started profile fetch when one is given.
func buildPublicProfileCardForTargetWithPrefetch(ctx context.Context, target ResolvedGameTarget, region string, app *renderapp.App, profileSnapshot snapshot.Snapshot, prefetch *sekaiProfilePrefetch) (*drawing.ProfileCardRequest, error) {
	if app == nil || app.Profiles == nil {
		return nil, nil
	}
	region = resolvedTargetRegion(region, target)

	var (
		resp *sekaiapi.GetAnotherProfileResponse
		err  error
	)
	if prefetch != nil {
		resp, err = prefetch.wait()
	} else {
		resp, err = fetchCachedSekaiUserProfile(ctx, app, region, target.PJSKUserID)
	}
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
	profileCtrl := app.Profiles.WithContext(ctx)
	finishBuild := measurePayloadBuild(ctx)
	if profileSnapshot != nil {
		card, buildErr = profileCtrl.BuildProfileCardFromAPIWithSnapshot(q, resp, profileSnapshot)
	} else {
		card, buildErr = profileCtrl.BuildProfileCardFromAPI(q, resp, nil)
	}
	finishBuild()
	if buildErr != nil {
		return nil, fmt.Errorf("sekaiapi profile build failed: %w", buildErr)
	}
	return card, nil
}
