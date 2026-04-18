package handler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	sekaidb "haruki-cloud/database/sekai"
	eventdb "haruki-cloud/database/sekai/event"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/masterdata"

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
			return err
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
	if q != nil && q.EventID != nil && *q.EventID == 180 {
		q.WorldBloomCharacterID = nil
		q.WorldBloomCharacterQuery = ""
		return nil
	}

	if q == nil || app == nil {
		return nil
	}

	if q.WorldBloomEventTurn != nil && *q.WorldBloomEventTurn > 0 {
		eventInfo, err := resolveDeckWorldBloomEventByTurn(ctx, app, region, *q.WorldBloomEventTurn)
		if err != nil {
			return err
		}
		q.EventID = drawing.IntPtr(int(eventInfo.GameID))
		q.WorldBloomEventTurn = nil
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
			if shouldFallbackDeckEventRecommendToNoEvent(q) {
				clearDeckAutoEventSelection(q)
				q.RecommendType = "no_event"
			}
			return nil
		}
		q.EventID = drawing.IntPtr(int(eventInfo.GameID))
	}

	eventID := *q.EventID
	eventInfo, err := queryDeckEventByID(ctx, app, region, eventID)
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

	chapters, err := queryDeckWorldBloomChapters(ctx, app, region, eventID)
	if err != nil {
		return err
	}

	if q.WorldBloomCharacterID == nil && strings.TrimSpace(q.WorldBloomCharacterQuery) == "" {
		if err := tryResolveDeckMusicQueryAsWorldBloomCharacter(ctx, q, app, region, chapters); err != nil {
			return err
		}
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
			return err
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
		return missingDeckWorldBloomChapterError(q, eventID)
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

func tryResolveDeckMusicQueryAsWorldBloomCharacter(
	ctx context.Context,
	q *deck.AutoQuery,
	app *renderapp.App,
	region renderregion.Value,
	chapters []*sekaidb.Worldbloom,
) error {
	if q == nil || app == nil {
		return nil
	}
	if strings.TrimSpace(q.MusicDiff) != "" {
		return nil
	}

	query := strings.TrimSpace(q.MusicQuery)
	if query == "" {
		return nil
	}

	charID, err := resolveGameCharacterIDByQuery(ctx, app, region, query, "deck")
	if err != nil {
		if isCharacterNotFoundError(err) {
			return nil
		}
		return err
	}
	if !trackerWorldBloomHasCharacter(chapters, charID) {
		return nil
	}

	q.WorldBloomCharacterID = drawing.IntPtr(charID)
	q.MusicQuery = ""
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
	if app == nil {
		return nil, fmt.Errorf("deck event resolve requires sekai client")
	}

	events, err := queryDeckEvents(ctx, app, region)
	if err != nil {
		return nil, fmt.Errorf("query deck events failed: %w", err)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("当前没有可用活动")
	}

	dbEventIDs, err := queryDeckDBEventIDs(ctx, app, region)
	if err != nil {
		return nil, err
	}

	now := time.Now().UnixMilli()
	var current *sekaidb.Event
	var next *sekaidb.Event
	for _, eventInfo := range events {
		if eventInfo == nil {
			continue
		}
		if eventInfo.StartAt <= now && now <= eventInfo.AggregateAt {
			if current == nil || eventInfo.StartAt > current.StartAt {
				current = eventInfo
			}
			continue
		}
		if eventInfo.StartAt > now {
			if !isDeckFutureEventAvailable(ctx, app, region, eventInfo, dbEventIDs, now) {
				continue
			}
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
	return nil, fmt.Errorf("当前没有可用活动")
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

func shouldFallbackDeckEventRecommendToNoEvent(q *deck.AutoQuery) bool {
	if q == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(q.RecommendType), "event")
}

func clearDeckAutoEventSelection(q *deck.AutoQuery) {
	if q == nil {
		return
	}
	q.EventID = nil
	q.EventUnit = ""
	q.EventAttr = ""
	q.WorldBloomEventTurn = nil
	q.WorldBloomCharacterID = nil
	q.WorldBloomCharacterQuery = ""
}

func missingDeckWorldBloomChapterError(q *deck.AutoQuery, eventID int) error {
	switch strings.ToLower(strings.TrimSpace(q.RecommendType)) {
	case "mysekai":
		return fmt.Errorf("请指定一个要查询的WL角色章节，例如 烤森组卡 wl1 miku 或 烤森组卡 event%d miku", eventID)
	default:
		return fmt.Errorf("请指定一个要查询的WL角色章节，例如 wl1 miku 或 event%d miku", eventID)
	}
}

func resolveDeckWorldBloomEventByTurn(ctx context.Context, app *renderapp.App, region renderregion.Value, turn int) (*sekaidb.Event, error) {
	if turn <= 0 {
		return nil, fmt.Errorf("无效的 WL 活动序号: %d", turn)
	}
	events, err := queryDeckEvents(ctx, app, region)
	if err != nil {
		return nil, fmt.Errorf("query deck world bloom events failed: %w", err)
	}
	worldBloomEvents := make([]*sekaidb.Event, 0, len(events))
	for _, eventInfo := range events {
		if eventInfo == nil || !strings.EqualFold(eventInfo.EventType, "world_bloom") {
			continue
		}
		worldBloomEvents = append(worldBloomEvents, eventInfo)
	}
	if len(worldBloomEvents) == 0 {
		return nil, fmt.Errorf("当前没有可用的 WL 活动")
	}
	if turn > len(worldBloomEvents) {
		return nil, fmt.Errorf("当前仅有 %d 个 WL 活动，无法解析 wl%d", len(worldBloomEvents), turn)
	}
	return worldBloomEvents[turn-1], nil
}

func queryDeckEvents(ctx context.Context, app *renderapp.App, region renderregion.Value) ([]*sekaidb.Event, error) {
	if app == nil {
		return nil, fmt.Errorf("deck event resolve requires app")
	}
	if deckEventProviderSupportsRegion(app, region) {
		items := app.Provider.Events().GetAll(ctx)
		events := make([]*sekaidb.Event, 0, len(items))
		for _, item := range items {
			if item == nil {
				continue
			}
			events = append(events, deckEventFromMasterdata(item))
		}
		sort.Slice(events, func(i, j int) bool {
			return events[i].StartAt < events[j].StartAt
		})
		return events, nil
	}
	if app.Sekai == nil {
		return nil, fmt.Errorf("deck event resolve requires sekai client")
	}
	return app.Sekai.Event.Query().
		Where(eventdb.ServerRegionEQ(region.String())).
		Order(eventdb.ByStartAt()).
		All(ctx)
}

func queryDeckEventByID(ctx context.Context, app *renderapp.App, region renderregion.Value, eventID int) (*sekaidb.Event, error) {
	if eventID <= 0 {
		return nil, fmt.Errorf("event id is required")
	}
	if deckEventProviderSupportsRegion(app, region) {
		item, err := app.Provider.Events().GetByID(ctx, eventID)
		if err == nil && item != nil {
			return deckEventFromMasterdata(item), nil
		}
	}
	if app == nil || app.Sekai == nil {
		return nil, fmt.Errorf("event %d not found", eventID)
	}
	return app.Sekai.Event.Query().
		Where(eventdb.ServerRegionEQ(region.String()), eventdb.GameIDEQ(int64(eventID))).
		Only(ctx)
}

func queryDeckWorldBloomChapters(ctx context.Context, app *renderapp.App, region renderregion.Value, eventID int) ([]*sekaidb.Worldbloom, error) {
	if deckEventProviderSupportsRegion(app, region) {
		items := app.Provider.Events().GetWorldBloomChapters(ctx, eventID)
		if len(items) > 0 {
			chapters := make([]*sekaidb.Worldbloom, 0, len(items))
			for _, item := range items {
				if item == nil {
					continue
				}
				chapters = append(chapters, deckWorldBloomFromMasterdata(item))
			}
			return chapters, nil
		}
	}
	return queryTrackerWorldBloomChapters(ctx, app, region, eventID)
}

func queryDeckDBEventIDs(ctx context.Context, app *renderapp.App, region renderregion.Value) (map[int]struct{}, error) {
	if app == nil || app.Sekai == nil {
		return nil, nil
	}
	items, err := app.Sekai.Event.Query().
		Where(eventdb.ServerRegionEQ(region.String())).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query deck db events failed: %w", err)
	}
	result := make(map[int]struct{}, len(items))
	for _, item := range items {
		if item == nil || item.GameID <= 0 {
			continue
		}
		result[int(item.GameID)] = struct{}{}
	}
	return result, nil
}

func isDeckFutureEventAvailable(ctx context.Context, app *renderapp.App, region renderregion.Value, eventInfo *sekaidb.Event, dbEventIDs map[int]struct{}, now int64) bool {
	if eventInfo == nil {
		return false
	}
	if eventInfo.StartAt <= now {
		return true
	}
	if dbEventIDs == nil {
		return true
	}
	if _, ok := dbEventIDs[int(eventInfo.GameID)]; ok {
		return true
	}
	if region != renderregion.JP {
		return false
	}
	return deckEventLeakReleased(ctx, app, int(eventInfo.GameID), now)
}

func deckEventLeakReleased(ctx context.Context, app *renderapp.App, eventID int, now int64) bool {
	if !deckEventProviderSupportsRegion(app, renderregion.JP) {
		return false
	}
	cards, err := app.Provider.Events().GetCards(ctx, eventID)
	if err != nil || len(cards) == 0 {
		return false
	}
	earliest := int64(0)
	for _, cardInfo := range cards {
		if cardInfo == nil || cardInfo.ReleaseAt <= 0 {
			continue
		}
		if earliest == 0 || cardInfo.ReleaseAt < earliest {
			earliest = cardInfo.ReleaseAt
		}
	}
	return earliest > 0 && earliest <= now
}

func deckEventProviderSupportsRegion(app *renderapp.App, region renderregion.Value) bool {
	if app == nil || app.Provider == nil || app.Provider.Events() == nil {
		return false
	}
	providerRegion := renderregion.Normalize(app.Provider.Region().String())
	if providerRegion.IsZero() {
		return false
	}
	return providerRegion == renderregion.Normalize(region.String())
}

func deckEventFromMasterdata(item *masterdata.Event) *sekaidb.Event {
	if item == nil {
		return nil
	}
	return &sekaidb.Event{
		GameID:          int64(item.ID),
		EventType:       item.EventType,
		Name:            item.Name,
		AssetbundleName: item.AssetBundleName,
		StartAt:         item.StartAt,
		AggregateAt:     item.AggregateAt,
		ClosedAt:        item.ClosedAt,
	}
}

func deckWorldBloomFromMasterdata(item *masterdata.WorldBloom) *sekaidb.Worldbloom {
	if item == nil {
		return nil
	}
	charID := int64(0)
	if item.GameCharacterID != nil {
		charID = int64(*item.GameCharacterID)
	}
	return &sekaidb.Worldbloom{
		EventID:               int64(item.EventID),
		GameCharacterID:       charID,
		WorldBloomChapterType: item.ChapterType,
		ChapterNo:             int64(item.ChapterNo),
		ChapterStartAt:        item.ChapterStartAt,
		AggregateAt:           item.AggregateAt,
		ChapterEndAt:          item.ChapterEndAt,
		IsSupplemental:        item.IsSupplemental,
	}
}
