package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	sekaidb "haruki-cloud/database/sekai"
	eventdb "haruki-cloud/database/sekai/event"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/deck"

	"haruki-cloud/internal/pjsk/drawing"
)

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
