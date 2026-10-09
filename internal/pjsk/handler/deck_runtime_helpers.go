package handler

import (
	"errors"
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/profile"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	"golang.org/x/sync/errgroup"
	"haruki-cloud/internal/core/upstreamerr"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/notfound"
	"haruki-cloud/utils/usererror"
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
	parts = appendNonEmpty(parts, firstNonEmpty(q.MusicTitle, q.MusicQuery))
	if q.MusicDiff != "" {
		parts = append(parts, strings.ToUpper(q.MusicDiff))
	}
	parts = appendNonEmpty(parts, deckCharacterSummary(q.WorldBloomCharacterQuery, q.WorldBloomCharacterID, "wl角色"))
	parts = appendNonEmpty(parts, deckCharacterSummary(q.ForcedLeaderCharacterQuery, q.ForcedLeaderCharacterID, "队长角色"))
	parts = appendNonEmpty(parts, deckCharacterSummary(q.ChallengeLiveCharacterQuery, q.ChallengeLiveCharacterID, "挑战角色"))
	return strings.Join(parts, " / ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func appendNonEmpty(values []string, value string) []string {
	if value == "" {
		return values
	}
	return append(values, value)
}

func deckCharacterSummary(query string, id *int, prefix string) string {
	if query != "" {
		return query
	}
	if id == nil || *id <= 0 {
		return ""
	}
	return fmt.Sprintf("%s%d", prefix, *id)
}

func applyDefaultChallengeDeckAutoQueryMusic(q *deck.AutoQuery) {
	if q == nil ||
		!strings.EqualFold(strings.TrimSpace(q.RecommendType), "challenge") ||
		q.MusicCompare ||
		q.MusicID != nil ||
		strings.TrimSpace(q.MusicQuery) != "" {
		return
	}
	q.MusicQuery = defaultChallengeDeckMusicQuery
	if strings.TrimSpace(q.MusicDiff) == "" {
		q.MusicDiff = defaultChallengeDeckMusicDiff
	}
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
		// Resolve the binding first, single-threaded. GetBinding is the only path
		// that creates the user identity on first contact, and both warmers below
		// funnel through it (ResolveSnapshot->GetBinding and
		// GetPublicProfileResponse->GetSelfTarget->GetBinding). Pre-resolving it
		// here means the concurrent warmers only read the already-created identity
		// instead of racing first-time creation for an unbound user.
		rc.GetBinding()
		// The suite snapshot (Toolbox) and the public profile (SekaiAPI) are
		// both required here and are independent once the binding is known, so
		// warm them concurrently. Both accessors memoize via sync.Once, so the
		// serial logic below consumes the already-resolved results.
		var warm errgroup.Group
		warm.Go(func() error { rc.ResolveSnapshot(false); return nil })
		warm.Go(func() error { rc.GetPublicProfileResponse(); return nil })
		_ = warm.Wait()

		binding, snapshot, err := rc.requireVisibleSuiteSnapshot()
		if err != nil {
			return nil, nil, "", nil, err
		}
		region := rc.RegionStr
		if binding != nil {
			region = resolvedTargetRegion(region, ResolvedGameTarget{Binding: binding})
			if snapshot == nil {
				return nil, nil, region, nil, suiteDataNotFoundError(binding)
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

	// The target snapshot (Toolbox) and public profile (SekaiAPI) both depend
	// only on the resolved target, so fetch them concurrently.
	var (
		snapshot    rendersnapshot.Snapshot
		snapshotErr error
		resp        *sekaiapi.GetAnotherProfileResponse
	)
	var group errgroup.Group
	group.Go(func() error {
		snapshot, snapshotErr = resolveTargetSnapshotWithError(rc.Ctx, rc.App, region, rc.Platform, rc.PlatformUserID, target.PJSKUserID, false)
		return nil
	})
	group.Go(func() error {
		resp = resolveDeckPublicProfileForTarget(rc, target, region)
		return nil
	})
	_ = group.Wait()
	if snapshotErr != nil {
		return nil, nil, region, nil, normalizeToolboxDataFetchError(snapshotErr, "suite", target.Binding)
	}

	if target.Binding != nil && snapshot == nil {
		return nil, nil, region, nil, suiteDataNotFoundError(target.Binding)
	}
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
	case deckEventCommand, "deck-challenge", "deck-no-event", "deck-bonus":
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
	finishBuild := measurePayloadBuild(rc.Ctx)
	detail, err := rc.App.Profiles.WithContext(rc.Ctx).BuildDetailedProfileCardFromAPIWithSnapshot(q, resp, snapshot)
	finishBuild()
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
	if errors.Is(err, deck.ErrUserDataRequired) || errors.Is(err, rendersnapshot.ErrNotConfigured) {
		return withCause(suiteDataNotFoundError(nil), err)
	}
	if isUserFacingError(err) {
		return notfound.InRegion(err, regionWithDefault(region), "")
	}
	if mode == deckEventCommand && upstreamerr.Is(err, upstreamerr.KindDataNotSynced) {
		return usererror.Wrap(usererror.CodeUnavailable, i18n.M("deck.event.data_not_synced"), err)
	}
	if wrapped := WrapDomainError(err); wrapped != err {
		return wrapped
	}
	return normalizeDeckServiceUserFacingError(err)
}

func validateDeckCharacterIDs(values []int) error {
	if len(values) == 0 {
		return nil
	}
	if len(values) > 5 {
		return usererror.Invalid(i18n.M("deck.fixed.too_many_characters"))
	}
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			return usererror.Invalid(i18n.M("deck.fixed.character_id_positive"))
		}
		if _, ok := seen[value]; ok {
			return usererror.Invalid(i18n.M("deck.fixed.duplicate_characters"))
		}
		seen[value] = struct{}{}
	}
	return nil
}

// isCharacterNotFoundError reports the typed "no character matches" error of
// resolveGameCharacterIDByQuery (by message ID).
func isCharacterNotFoundError(err error) bool {
	typed, ok := usererror.As(err)
	return ok && typed.Message.ID == "character.not_found"
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
