package handler

import (
	"errors"
	"fmt"
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/profile"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func formatDeckQuerySummary(q deck.AutoQuery) string {
	parts := make([]string, 0, 6)
	if region := strings.ToUpper(strings.TrimSpace(q.Region)); region != "" {
		parts = append(parts, region)
	}
	switch strings.ToLower(strings.TrimSpace(q.RecommendType)) {
	case "event":
		parts = append(parts, "活动组卡")
	case "challenge":
		parts = append(parts, "挑战组卡")
	case "no_event":
		parts = append(parts, "长草组卡")
	case "bonus":
		parts = append(parts, "加成组卡")
	case "mysekai":
		parts = append(parts, "烤森组卡")
	default:
		parts = append(parts, "组卡")
	}
	if q.EventID != nil && *q.EventID > 0 {
		parts = append(parts, fmt.Sprintf("event%d", *q.EventID))
	}
	if q.MusicTitle != "" {
		parts = append(parts, q.MusicTitle)
	} else if q.MusicQuery != "" {
		parts = append(parts, q.MusicQuery)
	}
	if q.MusicDiff != "" {
		parts = append(parts, strings.ToUpper(q.MusicDiff))
	}
	if q.WorldBloomCharacterQuery != "" {
		parts = append(parts, q.WorldBloomCharacterQuery)
	} else if q.WorldBloomCharacterID != nil && *q.WorldBloomCharacterID > 0 {
		parts = append(parts, fmt.Sprintf("wl角色%d", *q.WorldBloomCharacterID))
	}
	if q.ForcedLeaderCharacterQuery != "" {
		parts = append(parts, q.ForcedLeaderCharacterQuery)
	} else if q.ForcedLeaderCharacterID != nil && *q.ForcedLeaderCharacterID > 0 {
		parts = append(parts, fmt.Sprintf("队长角色%d", *q.ForcedLeaderCharacterID))
	}
	if q.ChallengeLiveCharacterQuery != "" {
		parts = append(parts, q.ChallengeLiveCharacterQuery)
	} else if q.ChallengeLiveCharacterID != nil && *q.ChallengeLiveCharacterID > 0 {
		parts = append(parts, fmt.Sprintf("挑战角色%d", *q.ChallengeLiveCharacterID))
	}
	return strings.Join(parts, " / ")
}

func resolveDeckRenderProfileAndSnapshot(rc *RequestContext, selector string) (*drawing.DetailedProfileCardRequest, rendersnapshot.Snapshot, string, error) {
	detail, snapshot, region, _, err := resolveDeckRenderProfileSnapshotAndPublic(rc, selector)
	return detail, snapshot, region, err
}

func resolveDeckRenderProfileSnapshotAndPublic(rc *RequestContext, selector string) (*drawing.DetailedProfileCardRequest, rendersnapshot.Snapshot, string, *sekaiapi.GetAnotherProfileResponse, error) {
	if rc == nil {
		return nil, nil, "", nil, nil
	}

	if isTheoreticalDeckRequest(rc) {
		return nil, nil, regionWithDefault(rc.RegionStr), nil, nil
	}

	selector = strings.TrimSpace(selector)
	if selector == "" {
		binding, snapshot, err := rc.requireVisibleSuiteSnapshot()
		if err != nil {
			return nil, nil, "", nil, err
		}
		region := rc.RegionStr
		if binding != nil {
			region = resolvedTargetRegion(region, ResolvedGameTarget{Binding: binding})
			if snapshot == nil {
				return nil, nil, region, nil, newSuiteDataNotFoundReplayErrorForBinding(binding)
			}
		}
		detail := rc.GetDetailedProfile()
		if detail == nil && snapshot != nil {
			detail = snapshot.DetailedProfile(renderregion.Normalize(region))
		}
		return detail, snapshot, region, rc.GetPublicProfileResponse(), nil
	}

	target, err := resolveGameTarget(rc.Ctx, userQueryParams{
		Mode:           "self",
		Platform:       rc.Platform,
		PlatformUserID: rc.PlatformUserID,
		Selector:       selector,
	}, rc.RegionStr, rc.Cmd.RegionExplicit, rc.App)
	if err != nil {
		return nil, nil, "", nil, err
	}
	region := resolvedTargetRegion(rc.RegionStr, target)

	snapshot := resolveTargetSnapshot(rc.Ctx, rc.App, region, rc.Platform, rc.PlatformUserID, target.PJSKUserID, false)
	if target.Binding != nil && snapshot == nil {
		return nil, nil, region, nil, newSuiteDataNotFoundReplayErrorForBinding(target.Binding)
	}
	resp := resolveDeckPublicProfileForTarget(rc, target, region)
	detail := buildDeckDetailedProfileForTargetWithResponse(rc, target, region, snapshot, resp)
	if detail == nil && snapshot != nil {
		detail = snapshot.DetailedProfile(renderregion.Normalize(region))
	}
	return detail, snapshot, region, resp, nil
}

func isTheoreticalDeckRequest(rc *RequestContext) bool {
	if rc == nil || rc.Cmd == nil || len(rc.Cmd.Params) == 0 {
		return false
	}
	switch rc.Cmd.Mode {
	case "deck-event", "deck-challenge", "deck-no-event", "deck-bonus":
	default:
		return false
	}

	var q deck.AutoQuery
	mergeParams(rc.Cmd.Params, &q)
	return isTheoreticalDeckQuery(q)
}

func resolveDeckPublicProfileForTarget(rc *RequestContext, target ResolvedGameTarget, region string) *sekaiapi.GetAnotherProfileResponse {
	if rc == nil || rc.App == nil || rc.App.SekaiAPI == nil {
		return nil
	}
	region = resolvedTargetRegion(region, target)
	resp, err := fetchCachedSekaiUserProfile(rc.Ctx, rc.App, region, target.PJSKUserID)
	if err != nil {
		return nil
	}
	return resp
}

func buildDeckDetailedProfileForTargetWithResponse(rc *RequestContext, target ResolvedGameTarget, region string, snapshot rendersnapshot.Snapshot, resp *sekaiapi.GetAnotherProfileResponse) *drawing.DetailedProfileCardRequest {
	if rc == nil || rc.App == nil || rc.App.Profiles == nil {
		return nil
	}
	region = resolvedTargetRegion(region, target)
	if resp == nil {
		return nil
	}

	q := profile.Query{
		Region:     region,
		Visible:    target.Visible,
		BgSettings: target.BgSettings,
	}
	detail, err := rc.App.Profiles.BuildDetailedProfileCardFromAPIWithSnapshot(q, resp, snapshot)
	if err != nil {
		return nil
	}
	return detail
}

func normalizeDeckUserFacingError(err error) error {
	return normalizeDeckUserFacingErrorForCommand(err, DefaultRegionStr, "")
}

func normalizeDeckUserFacingErrorForCommand(err error, region string, mode string) error {
	if err == nil {
		return nil
	}

	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}

	var deckLocked *deckEventLockedError
	if errors.As(err, &deckLocked) {
		return onebot11.NewReplayError("该活动组卡将于卡池开放后解禁")
	}

	message := strings.TrimSpace(err.Error())
	if _, ok := extractMusicNotFoundQuery(err, ""); ok {
		if mode == "deck-event" {
			return onebot11.NewReplayError("当前区服没有该歌曲")
		}
		musicQuery, _ := extractMusicNotFoundQuery(err, "")
		return newMusicNotFoundReplayError(region, musicQuery)
	}

	switch {
	case strings.Contains(strings.ToLower(message), "event not found for eventid:"):
		return onebot11.NewReplayError("组卡服务找不到该活动的 masterdata，请更新 masterdata 后重试")
	case strings.Contains(message, "local user snapshot is not configured"),
		strings.Contains(message, "user data is required for deck auto recommend"):
		return newSuiteDataNotFoundReplayError()
	case strings.Contains(message, "解析绑定账号失败"):
		return newBindingRequiredReplayError()
	case strings.Contains(message, "未找到该用户的绑定账号"):
		return newBindingRequiredReplayError()
	case strings.Contains(message, "toolbox: request failed after retries"),
		strings.Contains(message, "sekai api: request failed after retries"),
		strings.Contains(message, "context deadline exceeded"):
		return onebot11.NewReplayError("获取组卡所需数据超时，请稍后重试")
	}

	if wrapped := WrapDomainError(err); wrapped != err {
		return wrapped
	}

	if normalized := normalizeDeckServiceUserFacingError(err); normalized != err {
		return normalized
	}

	if normalized := normalizeDrawingUserFacingError(err); normalized != err {
		return normalized
	}

	return err
}

func validateDeckCharacterIDs(values []int) error {
	if len(values) == 0 {
		return nil
	}
	if len(values) > 5 {
		return fmt.Errorf("固定角色最多只能指定5个")
	}
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			return fmt.Errorf("固定角色ID必须为正整数")
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("固定角色ID不能重复")
		}
		seen[value] = struct{}{}
	}
	return nil
}

func isCharacterNotFoundError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "未找到角色")
}

func resolveDeckCharacterUnit(charID int) string {
	switch {
	case charID >= 1 && charID <= 4:
		return "light_sound"
	case charID >= 5 && charID <= 8:
		return "idol"
	case charID >= 9 && charID <= 12:
		return "street"
	case charID >= 13 && charID <= 16:
		return "theme_park"
	case charID >= 17 && charID <= 20:
		return "school_refusal"
	case charID >= 21 && charID <= 26:
		return "piapro"
	default:
		return ""
	}
}

func normalizeDeckUnit(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "light_sound", "idol", "street", "theme_park", "school_refusal", "piapro":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}
