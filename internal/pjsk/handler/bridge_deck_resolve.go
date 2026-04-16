package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/onebot11"
	sekaidb "haruki-cloud/database/sekai"
	eventdb "haruki-cloud/database/sekai/event"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/internal/pjsk/render/profile"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderuserdata "haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/internal/pjsk/drawing"
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
	if q.ChallengeLiveCharacterQuery != "" {
		parts = append(parts, q.ChallengeLiveCharacterQuery)
	} else if q.ChallengeLiveCharacterID != nil && *q.ChallengeLiveCharacterID > 0 {
		parts = append(parts, fmt.Sprintf("挑战角色%d", *q.ChallengeLiveCharacterID))
	}
	return strings.Join(parts, " / ")
}

func resolveDeckRenderProfileAndSnapshot(rc *RequestContext, selector string) (*drawing.DetailedProfileCardRequest, renderuserdata.Snapshot, string, error) {
	if rc == nil {
		return nil, nil, "", nil
	}

	selector = strings.TrimSpace(selector)
	if selector == "" {
		return rc.GetDetailedProfile(), rc.ResolveSnapshot(false), rc.RegionStr, nil
	}

	target, err := resolveGameTarget(rc.Ctx, userQueryParams{
		Mode:           "self",
		Platform:       rc.Platform,
		PlatformUserID: rc.PlatformUserID,
		Selector:       selector,
	}, rc.RegionStr, rc.Cmd.RegionExplicit, rc.App)
	if err != nil {
		return nil, nil, "", err
	}
	region := resolvedTargetRegion(rc.RegionStr, target)

	snapshot := resolveTargetSnapshot(rc.Ctx, rc.App, region, rc.Platform, rc.PlatformUserID, target.PJSKUserID, false)
	detail := buildDeckDetailedProfileForTarget(rc, target, region, snapshot)
	if detail == nil && snapshot != nil {
		detail = snapshot.DetailedProfile(renderregion.Normalize(region))
	}
	return detail, snapshot, region, nil
}

func buildDeckDetailedProfileForTarget(rc *RequestContext, target resolvedGameTarget, region string, snapshot renderuserdata.Snapshot) *drawing.DetailedProfileCardRequest {
	if rc == nil || rc.App == nil || rc.App.Profiles == nil {
		return nil
	}
	region = resolvedTargetRegion(region, target)

	resp, err := sekaiapi.GetSekaiAPIClient().GetUserProfile(region, target.PJSKUserID)
	if err != nil {
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

func resolveDeckMusicSelection(q *deck.AutoQuery, app *renderapp.App) error {
	if q == nil {
		return nil
	}
	if app == nil || app.Music == nil {
		return fmt.Errorf("deck music resolve requires music controller")
	}
	if q.MusicCompare {
		if len(q.MusicCompareQueries) == 0 {
			q.MusicCompareSelections = nil
			return nil
		}
		selections, err := resolveDeckMusicCompareSelections(q.Region, q.MusicCompareQueries, app)
		if err != nil {
			return err
		}
		q.MusicCompareSelections = selections
		return nil
	}

	var (
		result *music.CoverResult
		err    error
	)
	if q.MusicID != nil && *q.MusicID > 0 {
		result, err = app.Music.ResolveMusicCover(music.Query{
			Query:  fmt.Sprintf("music%d", *q.MusicID),
			Region: q.Region,
		})
	} else {
		query := strings.TrimSpace(q.MusicQuery)
		if query == "" {
			return nil
		}
		result, err = app.Music.ResolveMusicCoverByTitleOrAlias(music.Query{
			Query:  query,
			Region: q.Region,
		})
	}
	if err != nil {
		return err
	}
	if result == nil || result.Music == nil || result.Music.ID <= 0 {
		return fmt.Errorf("failed to resolve deck music selection")
	}

	q.MusicID = drawing.IntPtr(result.Music.ID)
	q.MusicTitle = result.Music.Title
	q.MusicCoverPath = result.JacketPath
	return nil
}

func resolveDeckMusicCompareSelections(region string, queries []string, app *renderapp.App) ([]deck.MusicCompareSelection, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	if app == nil || app.Music == nil {
		return nil, fmt.Errorf("deck music resolve requires music controller")
	}

	result := make([]deck.MusicCompareSelection, 0, len(queries))
	for _, raw := range queries {
		query := strings.TrimSpace(raw)
		if query == "" {
			continue
		}

		diff, cleaned := music.ExtractMusicDifficulty(query)
		if diff == "" {
			diff = "master"
		}
		if cleaned == "" {
			return nil, fmt.Errorf("无法解析要比较的歌曲 %q", query)
		}

		coverResult, err := app.Music.ResolveMusicCoverByTitleOrAlias(music.Query{
			Query:  cleaned,
			Region: region,
		})
		if err != nil {
			return nil, err
		}
		if coverResult == nil || coverResult.Music == nil || coverResult.Music.ID <= 0 {
			return nil, fmt.Errorf("failed to resolve compare music selection %q", query)
		}

		result = append(result, deck.MusicCompareSelection{
			MusicID:        coverResult.Music.ID,
			MusicDiff:      diff,
			MusicTitle:     coverResult.Music.Title,
			MusicCoverPath: coverResult.JacketPath,
			MusicQuery:     query,
		})
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func assignDeckFallbackMusicQuery(q *deck.AutoQuery, raw string) {
	if q == nil {
		return
	}

	query := strings.TrimSpace(raw)
	if query == "" {
		return
	}
	if strings.TrimSpace(q.MusicDiff) == "" {
		if diff, cleaned := music.ExtractMusicDifficulty(query); diff != "" && strings.TrimSpace(cleaned) != "" {
			q.MusicDiff = diff
			query = cleaned
		}
	}
	q.MusicQuery = strings.TrimSpace(query)
}

func resolveDeckCharacterSelections(ctx context.Context, q *deck.AutoQuery, app *renderapp.App) error {
	if q == nil {
		return nil
	}

	region := renderregion.WithDefault(renderregion.Normalize(q.Region))

	if err := resolveDeckEventAndWorldBloomSelection(ctx, q, app, region); err != nil {
		return err
	}

	if q.WorldBloomCharacterID == nil && strings.TrimSpace(q.WorldBloomCharacterQuery) != "" {
		charID, err := resolveGameCharacterIDByQuery(ctx, app, region, q.WorldBloomCharacterQuery, "deck")
		if err != nil {
			if q.WorldBloomEventTurn == nil && strings.TrimSpace(q.MusicQuery) == "" && isCharacterNotFoundError(err) {
				assignDeckFallbackMusicQuery(q, q.WorldBloomCharacterQuery)
				q.WorldBloomCharacterQuery = ""
			} else {
				return err
			}
		} else {
			q.WorldBloomCharacterID = drawing.IntPtr(charID)
			q.WorldBloomCharacterQuery = ""
			if strings.TrimSpace(q.EventUnit) == "" {
				q.EventUnit = resolveDeckCharacterUnit(charID)
			}
		}
	}

	if q.ChallengeLiveCharacterID == nil && strings.TrimSpace(q.ChallengeLiveCharacterQuery) != "" {
		charID, err := resolveGameCharacterIDByQuery(ctx, app, region, q.ChallengeLiveCharacterQuery, "deck")
		if err != nil {
			if strings.TrimSpace(q.MusicQuery) == "" && isCharacterNotFoundError(err) {
				assignDeckFallbackMusicQuery(q, q.ChallengeLiveCharacterQuery)
				q.ChallengeLiveCharacterQuery = ""
			} else {
				return err
			}
		} else {
			q.ChallengeLiveCharacterID = drawing.IntPtr(charID)
			q.ChallengeLiveCharacterQuery = ""
		}
	}

	if len(q.FixedCharacterQueries) > 0 {
		for _, raw := range q.FixedCharacterQueries {
			charID, err := resolveGameCharacterIDByQuery(ctx, app, region, raw, "deck")
			if err != nil {
				return err
			}
			q.FixedCharacters = append(q.FixedCharacters, charID)
		}
		q.FixedCharacterQueries = nil
	}

	if err := validateDeckCharacterIDs(q.FixedCharacters); err != nil {
		return err
	}
	return nil
}

func resolveDeckEventAndWorldBloomSelection(ctx context.Context, q *deck.AutoQuery, app *renderapp.App, region renderregion.Value) error {
	if q == nil || app == nil || app.Sekai == nil {
		return nil
	}

	if q.WorldBloomEventTurn != nil && *q.WorldBloomEventTurn > 0 {
		return nil
	}
	if strings.TrimSpace(q.EventUnit) != "" || strings.TrimSpace(q.EventAttr) != "" {
		return nil
	}

	if q.EventID == nil || *q.EventID <= 0 {
		if !shouldResolveDeckEventByRecommendType(q.RecommendType) {
			return nil
		}
		eventInfo, err := pickCurrentOrNextDeckEvent(ctx, app, region)
		if err != nil || eventInfo == nil {
			return nil
		}
		q.EventID = drawing.IntPtr(int(eventInfo.GameID))
	}

	eventID := *q.EventID
	eventInfo, err := app.Sekai.Event.Query().
		Where(eventdb.ServerRegionEQ(region.String()), eventdb.GameIDEQ(int64(eventID))).
		Only(ctx)
	if err != nil {
		if sekaidb.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("query deck event %d failed: %w", eventID, err)
	}

	if !strings.EqualFold(eventInfo.EventType, "world_bloom") {
		if q.WorldBloomCharacterQuery != "" && isDeckWorldBloomSelectorQuery(q.WorldBloomCharacterQuery) {
			return fmt.Errorf("活动 %s-%d 不是WL活动，无法指定章节", strings.ToUpper(region.String()), eventID)
		}
		q.WorldBloomCharacterID = nil
		if q.WorldBloomCharacterQuery != "" && isDeckWorldBloomSelectorQuery(q.WorldBloomCharacterQuery) {
			q.WorldBloomCharacterQuery = ""
		}
		return nil
	}

	chapters, err := queryTrackerWorldBloomChapters(ctx, app, region, eventID)
	if err != nil {
		return err
	}

	if q.WorldBloomCharacterID != nil && *q.WorldBloomCharacterID > 0 {
		if !trackerWorldBloomHasCharacter(chapters, *q.WorldBloomCharacterID) {
			return fmt.Errorf("活动 %s-%d 没有角色 %d 的 World Link 章节", strings.ToUpper(region.String()), eventID, *q.WorldBloomCharacterID)
		}
		return nil
	}

	query := strings.TrimSpace(q.WorldBloomCharacterQuery)
	if query != "" {
		chapter, err := resolveTrackerWorldBloomChapterSelection(ctx, app, region, eventInfo, chapters, query)
		if err != nil {
			if strings.TrimSpace(q.MusicQuery) == "" && !isDeckWorldBloomSelectorQuery(query) && isCharacterNotFoundError(err) {
				assignDeckFallbackMusicQuery(q, query)
				q.WorldBloomCharacterQuery = ""
				query = ""
			} else {
				return err
			}
		} else {
			if chapter.GameCharacterID <= 0 {
				return fmt.Errorf("活动 %s-%d 的 World Link 章节缺少角色信息", strings.ToUpper(region.String()), eventID)
			}
			charID := int(chapter.GameCharacterID)
			q.WorldBloomCharacterID = drawing.IntPtr(charID)
			q.WorldBloomCharacterQuery = ""
			if strings.TrimSpace(q.EventUnit) == "" {
				q.EventUnit = resolveDeckCharacterUnit(charID)
			}
			return nil
		}
	}

	chapter := pickDeckDefaultWorldBloomChapter(eventInfo, chapters)
	if chapter == nil {
		return fmt.Errorf("请指定一个要查询的WL章节，例如 event%d wl1 或 event%d miku", eventID, eventID)
	}
	if chapter.GameCharacterID <= 0 {
		return fmt.Errorf("活动 %s-%d 的 World Link 章节缺少角色信息", strings.ToUpper(region.String()), eventID)
	}

	charID := int(chapter.GameCharacterID)
	q.WorldBloomCharacterID = drawing.IntPtr(charID)
	if strings.TrimSpace(q.EventUnit) == "" {
		q.EventUnit = resolveDeckCharacterUnit(charID)
	}
	return nil
}

func shouldResolveDeckEventByRecommendType(recommendType string) bool {
	switch strings.ToLower(strings.TrimSpace(recommendType)) {
	case "event", "bonus", "mysekai":
		return true
	default:
		return false
	}
}

func pickCurrentOrNextDeckEvent(ctx context.Context, app *renderapp.App, region renderregion.Value) (*sekaidb.Event, error) {
	if app == nil || app.Sekai == nil {
		return nil, fmt.Errorf("deck event resolve requires sekai client")
	}

	events, err := app.Sekai.Event.Query().
		Where(eventdb.ServerRegionEQ(region.String())).
		Order(eventdb.ByStartAt()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query deck events failed: %w", err)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("当前没有可用活动")
	}

	now := time.Now().UnixMilli()
	var current *sekaidb.Event
	var next *sekaidb.Event
	var latest *sekaidb.Event
	for _, eventInfo := range events {
		if eventInfo == nil {
			continue
		}
		if latest == nil || eventInfo.StartAt > latest.StartAt {
			latest = eventInfo
		}
		if eventInfo.StartAt <= now && now <= eventInfo.AggregateAt {
			if current == nil || eventInfo.StartAt > current.StartAt {
				current = eventInfo
			}
			continue
		}
		if eventInfo.StartAt > now {
			if next == nil || eventInfo.StartAt < next.StartAt {
				next = eventInfo
			}
		}
	}
	if current != nil {
		return current, nil
	}
	if next != nil {
		return next, nil
	}
	return latest, nil
}

func pickDeckDefaultWorldBloomChapter(eventInfo *sekaidb.Event, chapters []*sekaidb.Worldbloom) *sekaidb.Worldbloom {
	if len(chapters) == 0 || eventInfo == nil {
		return nil
	}
	if len(chapters) == 1 {
		return chapters[0]
	}

	now := time.Now().UnixMilli()
	if now > eventInfo.AggregateAt {
		var last *sekaidb.Worldbloom
		for _, chapter := range chapters {
			if chapter == nil {
				continue
			}
			if last == nil || chapter.ChapterStartAt > last.ChapterStartAt {
				last = chapter
			}
		}
		return last
	}
	if now < eventInfo.StartAt {
		var first *sekaidb.Worldbloom
		for _, chapter := range chapters {
			if chapter == nil {
				continue
			}
			if first == nil || chapter.ChapterStartAt < first.ChapterStartAt {
				first = chapter
			}
		}
		return first
	}

	var selected *sekaidb.Worldbloom
	for _, chapter := range chapters {
		if chapter == nil {
			continue
		}
		chapterEnd := chapter.AggregateAt + 1000
		if chapter.ChapterStartAt <= now && now <= chapterEnd {
			if selected == nil || chapter.ChapterStartAt > selected.ChapterStartAt {
				selected = chapter
			}
		}
	}
	return selected
}

func isDeckWorldBloomSelectorQuery(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "wl" {
		return true
	}
	if _, ok := parseTrackerWorldBloomChapterNo(query); ok {
		return true
	}
	return strings.HasPrefix(query, "wl")
}

func normalizeDeckUserFacingError(err error) error {
	if err == nil {
		return nil
	}

	var replyErr onebot11.ReplayError
	if errors.As(err, &replyErr) {
		return err
	}

	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(message, "failed to search music by title or alias: music not found:"):
		musicQuery := strings.TrimSpace(strings.TrimPrefix(message, "failed to search music by title or alias: music not found:"))
		if musicQuery == "" {
			return onebot11.NewReplayError("未找到对应歌曲，请检查歌曲名或别名后重试")
		}
		return onebot11.NewReplayError("未找到歌曲：%s", musicQuery)
	case strings.Contains(message, "local user snapshot is not configured"),
		strings.Contains(message, "user data is required for deck auto recommend"):
		return onebot11.NewReplayError("组卡需要用户卡牌持有数据，请先绑定账号并上传 Suite 抓包或本地快照")
	case strings.Contains(message, "解析绑定账号失败"):
		return onebot11.NewReplayError("组卡需要先绑定账号；如果已绑定，请确认该账号已上传 Suite 抓包数据")
	case strings.Contains(message, "未找到该用户的绑定账号"):
		return onebot11.NewReplayError("未找到目标用户的绑定账号")
	case strings.Contains(message, "toolbox: request failed after retries"),
		strings.Contains(message, "sekai api: request failed after retries"),
		strings.Contains(message, "context deadline exceeded"):
		return onebot11.NewReplayError("获取组卡所需数据超时，请稍后重试")
	default:
		return err
	}
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
