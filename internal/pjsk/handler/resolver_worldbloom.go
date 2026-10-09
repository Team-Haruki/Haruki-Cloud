package handler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	sekaidb "haruki-cloud/database/sekai"
	eventdb "haruki-cloud/database/sekai/event"
	worldbloomdb "haruki-cloud/database/sekai/worldbloom"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/eventutil"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/usererror"
)

func resolveTrackerWorldBloomEvent(ctx context.Context, app *renderapp.App, region renderregion.Value, eventID int) (*sekaidb.Event, []*sekaidb.Worldbloom, error) {
	if app == nil || app.Sekai == nil {
		return nil, nil, usererror.Misconfigured(errors.New("sk service unavailable: sekai client not configured"))
	}

	if eventID > 0 {
		eventInfo, err := app.Sekai.Event.Query().
			Where(eventdb.ServerRegionEQ(region.String()), eventdb.GameIDEQ(int64(eventID))).
			Only(ctx)
		if err != nil {
			if sekaidb.IsNotFound(err) {
				return nil, nil, usererror.New(usererror.CodeNotFound, i18n.M("event.not_found_in_region", i18n.Data{"Region": i18n.RegionLabel(region.String()), "ID": eventID}))
			}
			return nil, nil, fmt.Errorf("query sk event %d failed: %w", eventID, err)
		}
		if !strings.EqualFold(eventInfo.EventType, "world_bloom") {
			return nil, nil, usererror.Invalid(i18n.M("sk.wl.not_wl_event", i18n.Data{"Event": eventLabel(region.String(), eventID)}))
		}
		chapters, err := queryTrackerWorldBloomChapters(ctx, app, region, eventID)
		if err != nil {
			return nil, nil, err
		}
		return eventInfo, chapters, nil
	}

	eventInfo, err := pickCurrentWorldBloomEvent(ctx, app, region)
	if err != nil {
		return nil, nil, err
	}

	chapters, err := queryTrackerWorldBloomChapters(ctx, app, region, int(eventInfo.GameID))
	if err != nil {
		return nil, nil, err
	}
	return eventInfo, chapters, nil
}

func pickCurrentWorldBloomEvent(ctx context.Context, app *renderapp.App, region renderregion.Value) (*sekaidb.Event, error) {
	events, err := app.Sekai.Event.Query().
		Where(eventdb.ServerRegionEQ(region.String()), eventdb.EventTypeEQ("world_bloom")).
		Order(eventdb.ByStartAt()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query World Link events failed: %w", err)
	}
	if len(events) == 0 {
		return nil, currentWorldBloomUnavailableError(region)
	}

	now := time.Now().UnixMilli()
	for _, eventInfo := range events {
		if eventInfo == nil {
			continue
		}
		if eventutil.IsCurrent(eventInfo.StartAt, eventInfo.AggregateAt, eventInfo.ClosedAt, now) {
			return eventInfo, nil
		}
	}
	return nil, currentWorldBloomUnavailableError(region)
}

func currentWorldBloomUnavailableError(region renderregion.Value) error {
	return usererror.New(usererror.CodeNotFound, i18n.M("sk.wl.no_current", i18n.Data{
		"Region":  i18n.RegionLabel(region.String()),
		"Command": "/" + strings.ToLower(region.String()) + "sk",
	}))
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
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("sk.wl.no_chapters", i18n.Data{"Event": eventLabel(region.String(), eventID)}))
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
			return nil, usererror.New(usererror.CodeNotFound, i18n.M("sk.wl.no_started_chapter", i18n.Data{"Event": eventLabel(region.String(), eventID)}))
		}
		return chapter, nil
	}

	if chapterNo, ok := parseTrackerWorldBloomChapterNo(query); ok {
		chapter := findWorldBloomChapterByNo(chapters, chapterNo)
		if chapter == nil {
			return nil, usererror.Invalid(i18n.M("sk.wl.no_chapter_no", i18n.Data{"Event": eventLabel(region.String(), eventID), "Chapter": chapterNo}))
		}
		return chapter, nil
	}

	charQuery := query
	if strings.HasPrefix(strings.ToLower(charQuery), "wl") {
		charQuery = strings.TrimSpace(charQuery[2:])
	}
	if charQuery == "" {
		return nil, usererror.Misuse(i18n.M("sk.wl.chapter_required"))
	}

	charID, err := resolveGameCharacterIDByQuery(ctx, app, region, charQuery, "sk")
	if err != nil {
		return nil, err
	}

	chapter := findWorldBloomChapterByCharacterID(chapters, charID)
	if chapter == nil {
		return nil, usererror.Invalid(i18n.M("sk.wl.no_character_chapter", i18n.Data{"Event": eventLabel(region.String(), eventID), "Character": characterLabel(ctx, app, charID)}))
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
