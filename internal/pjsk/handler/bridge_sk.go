package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/api/bot/onebot11"
	sekaidb "haruki-cloud/database/sekai"
	eventdb "haruki-cloud/database/sekai/event"
	worldbloomdb "haruki-cloud/database/sekai/worldbloom"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/sk"
	accountdata "haruki-cloud/internal/pjsk/userdata"
	"haruki-cloud/utils/drawing"
)

func executeSK(rc *RequestContext) (message onebot11.Message, err error) {
	if rc == nil || rc.App == nil || rc.App.SK == nil {
		return nil, fmt.Errorf("sk service unavailable: tracker controller is not configured")
	}
	skCtrl := rc.App.SK.WithContext(rc.Ctx)
	data, err := executeSKMode(rc, skCtrl)
	if err != nil {
		return nil, err
	}
	return imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
}

func trackerRankQueryFromParams(r *parser.ResolvedCommand) (sk.TrackerRankQuery, bool) {
	if r == nil || len(r.Params) == 0 {
		return sk.TrackerRankQuery{}, false
	}
	var req sk.TrackerRankQuery
	if err := json.Unmarshal(r.Params, &req); err != nil {
		return sk.TrackerRankQuery{}, false
	}
	resolvedRegion := strings.TrimSpace(r.Region)
	if !req.RegionExplicit && resolvedRegion != "" {
		req.Region = resolvedRegion
	} else if req.Region == "" {
		req.Region = resolvedRegion
	}
	if len(req.Ranks) == 0 &&
		req.EventID == 0 &&
		req.WlCharacterID == nil &&
		strings.TrimSpace(req.WlCharacterQuery) == "" &&
		req.UserID == nil &&
		strings.TrimSpace(req.TargetUserID) == "" {
		return sk.TrackerRankQuery{}, false
	}
	return req, true
}

func prepareTrackerRankQuery(ctx context.Context, app *renderapp.App, req *sk.TrackerRankQuery, requesterPlatform, requesterUserID string) error {
	if err := resolveTrackerCharacterSelection(ctx, app, req); err != nil {
		return err
	}
	return resolveTrackerTargetUser(ctx, app, req, requesterPlatform, requesterUserID)
}

func resolveTrackerCharacterSelection(ctx context.Context, app *renderapp.App, req *sk.TrackerRankQuery) error {
	if req == nil {
		return nil
	}

	if req.WlCharacterID != nil && *req.WlCharacterID <= 0 {
		req.WlCharacterID = nil
	}
	query := strings.TrimSpace(req.WlCharacterQuery)
	if req.WlCharacterID == nil && query == "" {
		return nil
	}

	region := renderregion.WithDefault(renderregion.Normalize(req.Region))
	eventInfo, chapters, err := resolveTrackerWorldBloomEvent(ctx, app, region, req.EventID)
	if err != nil {
		return err
	}

	req.EventID = int(eventInfo.GameID)
	if req.WlCharacterID != nil {
		if !trackerWorldBloomHasCharacter(chapters, *req.WlCharacterID) {
			return fmt.Errorf("活动 %s-%d 没有角色 %d 的 World Link 章节", strings.ToUpper(region.String()), req.EventID, *req.WlCharacterID)
		}
		req.WlCharacterQuery = ""
		return nil
	}

	chapter, err := resolveTrackerWorldBloomChapterSelection(ctx, app, region, eventInfo, chapters, query)
	if err != nil {
		return err
	}
	if chapter.GameCharacterID <= 0 {
		return fmt.Errorf("活动 %s-%d 的 World Link 章节缺少角色信息", strings.ToUpper(region.String()), req.EventID)
	}

	charID := int(chapter.GameCharacterID)
	req.WlCharacterID = drawing.IntPtr(charID)
	req.WlCharacterQuery = ""
	return nil
}

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

// resolveRequesterGameUID resolves the game user ID from the requester's binding.
func resolveRequesterGameUID(rc *RequestContext) int64 {
	_, binding, _ := resolveBindingWithFallback(
		rc.Ctx, rc.App.Bindings, rc.Platform, rc.PlatformUserID, rc.RegionStr, rc.Cmd.RegionExplicit,
		bindingResolutionOptions{},
	)
	if binding == nil {
		return 0
	}
	uid, parseErr := strconv.ParseInt(strings.TrimSpace(binding.PJSKUserID), 10, 64)
	if parseErr != nil || uid <= 0 {
		return 0
	}
	return uid
}

func resolveTrackerTargetUser(ctx context.Context, app *renderapp.App, req *sk.TrackerRankQuery, requesterPlatform, requesterUserID string) error {
	if req == nil || req.UserID != nil {
		return nil
	}

	targetPlatform := strings.TrimSpace(req.TargetPlatform)
	targetUserID := strings.TrimSpace(req.TargetUserID)
	targetSelector := strings.TrimSpace(req.TargetSelector)
	if targetPlatform == "" || targetUserID == "" {
		return nil
	}

	if app == nil || app.Bindings == nil || !app.Bindings.IsReady() {
		return fmt.Errorf("绑定服务未就绪，无法解析目标用户")
	}

	var (
		binding *accountdata.ResolvedBinding
		err     error
	)

	// Selector mode: pick a specific bound account directly (u1/u2...).
	if targetSelector != "" {
		_, binding, err = app.Bindings.ResolveUserBindingBySelector(ctx, targetPlatform, targetUserID, selectorBindingServer(normalizeTrackerRegion(req.Region), req.RegionExplicit), targetSelector)
		if err != nil {
			return fmt.Errorf("无法解析账号选择器 %s: %w", targetSelector, err)
		}
	} else if req.RegionExplicit {
		region := normalizeTrackerRegion(req.Region)
		_, binding, err = app.Bindings.ResolveUserBinding(ctx, targetPlatform, targetUserID, region)
		if err != nil {
			return fmt.Errorf("@用户 %s 在 %s 服没有绑定账号", targetUserID, strings.ToUpper(region))
		}
	} else {
		// No explicit region prefix: use global default binding first, then JP.
		_, binding, err = app.Bindings.ResolveUserBinding(ctx, targetPlatform, targetUserID, accountdata.GlobalDefaultBindingScope)
		if err != nil || binding == nil {
			_, binding, err = app.Bindings.ResolveUserBinding(ctx, targetPlatform, targetUserID, DefaultRegionStr)
			if err != nil {
				return fmt.Errorf("@用户 %s 没有可用绑定", targetUserID)
			}
		}
	}

	if binding == nil {
		return fmt.Errorf("@用户 %s 没有可用绑定", targetUserID)
	}

	isSelfTarget := strings.EqualFold(strings.TrimSpace(targetPlatform), strings.TrimSpace(requesterPlatform)) &&
		strings.TrimSpace(targetUserID) != "" &&
		strings.TrimSpace(targetUserID) == strings.TrimSpace(requesterUserID)

	// @target should respect visible=false and deny query, except self query.
	if targetSelector == "" && !binding.Visible && !isSelfTarget {
		return fmt.Errorf("@用户 %s 已隐藏个人信息，无法查询", targetUserID)
	}

	uid, parseErr := strconv.ParseInt(strings.TrimSpace(binding.PJSKUserID), 10, 64)
	if parseErr != nil || uid <= 0 {
		return fmt.Errorf("@用户 %s 的绑定UID无效: %s", targetUserID, binding.PJSKUserID)
	}
	req.UserID = &uid
	if !req.RegionExplicit {
		req.Region = normalizeTrackerRegion(binding.Server)
	}
	return nil
}

func normalizeTrackerRegion(region string) string {
	return regionWithDefault(region)
}
