package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	sekaidb "haruki-cloud/database/sekai"
	eventdb "haruki-cloud/database/sekai/event"
	worldbloomdb "haruki-cloud/database/sekai/worldbloom"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

func resolveTrackerWorldBloomEvent(ctx context.Context, app *renderapp.App, region renderregion.Value, eventID int) (*sekaidb.Event, []*sekaidb.Worldbloom, error) {
	if app == nil || app.Sekai == nil {
		return nil, nil, fmt.Errorf("sk service unavailable: sekai client not configured")
	}

	if eventID > 0 {
		eventInfo, err := app.Sekai.Event.Query().
			Where(eventdb.ServerRegionEQ(region.String()), eventdb.GameIDEQ(int64(eventID))).
			Only(ctx)
		if err != nil {
			if sekaidb.IsNotFound(err) {
				return nil, nil, fmt.Errorf("未找到活动：%s-%d", strings.ToUpper(region.String()), eventID)
			}
			return nil, nil, fmt.Errorf("query sk event %d failed: %w", eventID, err)
		}
		if !strings.EqualFold(eventInfo.EventType, "world_bloom") {
			return nil, nil, fmt.Errorf("活动 %s-%d 不是 World Link 活动", strings.ToUpper(region.String()), eventID)
		}
		chapters, err := queryTrackerWorldBloomChapters(ctx, app, region, eventID)
		if err != nil {
			return nil, nil, err
		}
		return eventInfo, chapters, nil
	}

	eventInfo, err := pickCurrentOrPreviousWorldBloomEvent(ctx, app, region)
	if err != nil {
		return nil, nil, err
	}

	chapters, err := queryTrackerWorldBloomChapters(ctx, app, region, int(eventInfo.GameID))
	if err != nil {
		return nil, nil, err
	}
	return eventInfo, chapters, nil
}

func pickCurrentOrPreviousWorldBloomEvent(ctx context.Context, app *renderapp.App, region renderregion.Value) (*sekaidb.Event, error) {
	events, err := app.Sekai.Event.Query().
		Where(eventdb.ServerRegionEQ(region.String()), eventdb.EventTypeEQ("world_bloom")).
		Order(eventdb.ByStartAt()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query World Link events failed: %w", err)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("当前没有可用的 World Link 活动")
	}

	now := time.Now().UnixMilli()
	var current *sekaidb.Event
	var previous *sekaidb.Event
	for _, eventInfo := range events {
		if eventInfo == nil {
			continue
		}
		if eventInfo.StartAt <= now && now < eventInfo.AggregateAt+1000 {
			current = eventInfo
			continue
		}
		if eventInfo.AggregateAt+1000 < now {
			previous = eventInfo
		}
	}
	if current != nil {
		return current, nil
	}
	if previous != nil {
		return previous, nil
	}
	return nil, fmt.Errorf("当前没有进行中或已结束的 World Link 活动")
}

func queryTrackerWorldBloomChapters(ctx context.Context, app *renderapp.App, region renderregion.Value, eventID int) ([]*sekaidb.Worldbloom, error) {
	chapters, err := app.Sekai.Worldbloom.Query().
		Where(worldbloomdb.ServerRegionEQ(region.String()), worldbloomdb.EventIDEQ(int64(eventID))).
		Order(worldbloomdb.ByChapterNo()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query World Link chapters for event %s-%d failed: %w", strings.ToUpper(region.String()), eventID, err)
	}
	if len(chapters) == 0 {
		return nil, fmt.Errorf("活动 %s-%d 没有可用的 World Link 章节", strings.ToUpper(region.String()), eventID)
	}
	return chapters, nil
}

func resolveTrackerWorldBloomChapterSelection(
	ctx context.Context,
	app *renderapp.App,
	region renderregion.Value,
	eventInfo *sekaidb.Event,
	chapters []*sekaidb.Worldbloom,
	query string,
) (*sekaidb.Worldbloom, error) {
	query = strings.TrimSpace(query)
	eventID := int(eventInfo.GameID)

	if strings.EqualFold(query, "wl") {
		chapter := pickCurrentWorldBloomChapter(chapters)
		if chapter == nil {
			return nil, fmt.Errorf("活动 %s-%d 还没有开始任何 World Link 章节，请使用 wl2 或 wlmiku 指定章节", strings.ToUpper(region.String()), eventID)
		}
		return chapter, nil
	}

	if chapterNo, ok := parseTrackerWorldBloomChapterNo(query); ok {
		chapter := findWorldBloomChapterByNo(chapters, chapterNo)
		if chapter == nil {
			return nil, fmt.Errorf("活动 %s-%d 没有第 %d 章 World Link 单榜", strings.ToUpper(region.String()), eventID, chapterNo)
		}
		return chapter, nil
	}

	charQuery := query
	if strings.HasPrefix(strings.ToLower(charQuery), "wl") {
		charQuery = strings.TrimSpace(charQuery[2:])
	}
	if charQuery == "" {
		return nil, fmt.Errorf("查询 World Link 榜线需要指定章节，可使用 wl / wl2 / wlmiku")
	}

	charID, err := resolveGameCharacterIDByQuery(ctx, app, region, charQuery, "sk")
	if err != nil {
		return nil, err
	}

	chapter := findWorldBloomChapterByCharacterID(chapters, charID)
	if chapter == nil {
		return nil, fmt.Errorf("活动 %s-%d 没有角色 %s 的 World Link 章节", strings.ToUpper(region.String()), eventID, charQuery)
	}
	return chapter, nil
}

func parseTrackerWorldBloomChapterNo(query string) (int, bool) {
	query = strings.ToLower(strings.TrimSpace(query))
	if !strings.HasPrefix(query, "wl") || len(query) <= 2 {
		return 0, false
	}

	chapterNo, err := strconv.Atoi(strings.TrimSpace(query[2:]))
	if err != nil || chapterNo <= 0 {
		return 0, false
	}
	return chapterNo, true
}

func pickCurrentWorldBloomChapter(chapters []*sekaidb.Worldbloom) *sekaidb.Worldbloom {
	now := time.Now().UnixMilli()
	var selected *sekaidb.Worldbloom
	for _, chapter := range chapters {
		if chapter == nil || chapter.ChapterStartAt > now {
			continue
		}
		if selected == nil || chapter.ChapterStartAt > selected.ChapterStartAt {
			selected = chapter
		}
	}
	return selected
}

func findWorldBloomChapterByNo(chapters []*sekaidb.Worldbloom, chapterNo int) *sekaidb.Worldbloom {
	for _, chapter := range chapters {
		if chapter == nil || int(chapter.ChapterNo) != chapterNo {
			continue
		}
		return chapter
	}
	return nil
}

func findWorldBloomChapterByCharacterID(chapters []*sekaidb.Worldbloom, characterID int) *sekaidb.Worldbloom {
	for _, chapter := range chapters {
		if chapter == nil || chapter.GameCharacterID <= 0 || int(chapter.GameCharacterID) != characterID {
			continue
		}
		return chapter
	}
	return nil
}

func trackerWorldBloomHasCharacter(chapters []*sekaidb.Worldbloom, characterID int) bool {
	return findWorldBloomChapterByCharacterID(chapters, characterID) != nil
}
