package vlive

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/provider"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/utils/usererror"
)

var ErrNoLives = errors.New("no virtual lives")

func NewController(source DataSource, defaultRegion renderregion.Value) *Controller {
	return NewControllerWithDrawing(source, nil, nil, defaultRegion)
}

func NewControllerWithDrawing(
	source DataSource,
	drawingClient *drawing.HarukiDrawingClient,
	assetHelper *assets.AssetHelper,
	defaultRegion renderregion.Value,
) *Controller {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	ctrl := &Controller{
		sources: regionsource.NewRegistry[DataSource](renderregion.WithDefault(defaultRegion)),
		drawing: drawingClient,
		assets:  assetHelper,
	}
	ctrl.RegisterSource(source)
	return ctrl
}

func (c *Controller) RegisterSource(source DataSource) {
	if c == nil || source == nil {
		return
	}
	if c.sources == nil {
		c.sources = regionsource.NewRegistry[DataSource](source.DefaultRegion())
	}
	c.sources.RegisterSource(source)
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.requestCtx = ctx
	clone.drawing = c.drawing.WithContext(ctx)
	clone.assets = c.assets.WithContext(ctx)
	clone.sources = regionsource.NewRegistry[DataSource](c.resolveRegion(""))
	if c.sources != nil {
		for _, source := range c.sources.OrderedSources() {
			if contextual, ok := any(source).(contextualDataSource); ok {
				clone.sources.RegisterSource(contextual.WithContext(ctx))
				continue
			}
			clone.sources.RegisterSource(source)
		}
	}
	return &clone
}

func (c *Controller) ResolveLives(query ListQuery) ([]ResolvedLive, renderregion.Value, error) {
	if c == nil || c.sources == nil {
		return nil, renderregion.Unknown, usererror.Misconfigured(errors.New("vlive controller is not configured"))
	}

	region := c.resolveRegion(query.Region)
	source, ok := c.sources.SourceForRegion(region)
	if !ok {
		return nil, region, fmt.Errorf("no vlive data source for region %s", region)
	}

	now := query.Now
	if now.IsZero() {
		now = time.Now()
	}

	lives, err := source.GetLives(region)
	if err != nil {
		return nil, region, err
	}

	result := make([]ResolvedLive, 0, len(lives))
	for _, live := range lives {
		resolved, ok := resolveLiveAt(live, now)
		if ok {
			result = append(result, resolved)
		}
	}
	sortResolvedLives(result)
	return result, region, nil
}

func resolveLiveAt(live *Live, now time.Time) (ResolvedLive, bool) {
	if live == nil {
		return ResolvedLive{}, false
	}
	startAt := unixTime(live.StartAt)
	endAt := unixTime(live.EndAt)
	if !liveWindowVisible(startAt, endAt, now) {
		return ResolvedLive{}, false
	}
	resolved := ResolvedLive{
		ID:              live.ID,
		Name:            strings.TrimSpace(live.Name),
		AssetBundleName: strings.TrimSpace(live.AssetBundleName),
		StartAt:         startAt,
		EndAt:           endAt,
		Rewards:         append([]Reward(nil), live.Rewards...),
		Characters:      append([]Character(nil), live.Characters...),
		VirtualLiveType: strings.TrimSpace(live.VirtualLiveType),
		GroupID:         live.GroupID,
	}
	applyLiveSchedules(&resolved, normalizeSchedules(live.Schedules), now)
	applyLiveWindowFallback(&resolved, now)
	return resolved, true
}

func liveWindowVisible(startAt, endAt, now time.Time) bool {
	return !startAt.IsZero() &&
		!endAt.IsZero() &&
		now.Before(endAt) &&
		startAt.Sub(now) < 7*24*time.Hour &&
		endAt.Sub(startAt) < 30*24*time.Hour
}

func applyLiveSchedules(resolved *ResolvedLive, schedules []Window, now time.Time) {
	for _, schedule := range schedules {
		if resolved.Current == nil && now.Before(schedule.EndAt) {
			resolved.Current = new(schedule)
			resolved.Living = !now.Before(schedule.StartAt)
		}
		if now.Before(schedule.StartAt) {
			resolved.RestCount++
		}
	}
}

func applyLiveWindowFallback(resolved *ResolvedLive, now time.Time) {
	if resolved.Current != nil {
		return
	}
	if now.Before(resolved.StartAt) {
		resolved.Current = &Window{StartAt: resolved.StartAt, EndAt: resolved.EndAt}
		return
	}
	if now.Before(resolved.EndAt) {
		resolved.Current = &Window{StartAt: resolved.StartAt, EndAt: resolved.EndAt}
		resolved.Living = true
	}
}

func sortResolvedLives(result []ResolvedLive) {
	sort.Slice(result, func(i, j int) bool {
		if result[i].StartAt.Equal(result[j].StartAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].StartAt.Before(result[j].StartAt)
	})
}

func (c *Controller) BuildListRequest(query ListQuery) (*drawing.VLiveListRequest, error) {
	lives, region, err := c.ResolveLives(query)
	if err != nil {
		return nil, err
	}
	if len(lives) == 0 {
		return nil, ErrNoLives
	}
	source, ok := c.sources.SourceForRegion(region)
	if !ok {
		return nil, fmt.Errorf("no vlive data source for region %s", region)
	}

	req := &drawing.VLiveListRequest{
		Region:   region.String(),
		TimeZone: query.TimeZone,
	}
	if !query.Now.IsZero() {
		req.DT = query.Now.UnixMilli()
	}

	visible := collapseSoloGroups(lives, c.groupsFor(source, region, lives))
	tasks := make([]func(*assets.AssetHelper), 0, len(visible))
	for _, live := range visible {
		candidates := c.bannerCandidates(source, live)
		tasks = append(tasks, func(helper *assets.AssetHelper) {
			assets.ResolveRegionAssetPath(helper, region.String(), candidates...)
		})
	}
	if err := c.assets.Prefetch(tasks); err != nil {
		return nil, err
	}
	for _, live := range visible {
		item := drawing.VLiveBrief{
			ID:         live.ID,
			Name:       fallbackLiveName(live.Name, live.ID),
			StartAt:    live.StartAt.UnixMilli(),
			EndAt:      live.EndAt.UnixMilli(),
			Living:     live.Living,
			RestCount:  live.RestCount,
			BannerPath: c.bannerPath(source, region, live),
			Rewards:    c.buildRewardItems(source, live),
			Characters: c.buildCharacterItems(source, live),
		}
		if live.Current != nil {
			item.CurrentStartAt = live.Current.StartAt.UnixMilli()
			item.CurrentEndAt = live.Current.EndAt.UnixMilli()
		}
		if len(live.Members) > 0 {
			item.VirtualLiveType = live.VirtualLiveType
			item.GroupID = new(live.GroupID)
			item.GroupName = live.GroupName
			item.GroupCount = new(len(live.Members))
		}
		req.Lives = append(req.Lives, item)
	}

	return req, nil
}

func (c *Controller) RenderList(query ListQuery) ([]byte, error) {
	image, err := c.RenderListImage(query)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.requestCtx)
}

func (c *Controller) RenderListImage(query ListQuery) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	req, err := c.BuildListRequest(query)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateVLiveListImage(req)
}

func (c *Controller) resolveRegion(region string) renderregion.Value {
	if c != nil && c.sources != nil {
		return c.sources.ResolveRegion(renderregion.Normalize(region))
	}
	return renderregion.WithDefault(renderregion.Normalize(region))
}

func (c *Controller) bannerPath(source DataSource, region renderregion.Value, live ResolvedLive) string {
	candidates := c.bannerCandidates(source, live)
	if len(candidates) == 0 {
		return ""
	}
	return assets.ResolveRegionAssetPath(c.assets, region.String(), candidates...)
}

func (c *Controller) bannerCandidates(source DataSource, live ResolvedLive) []string {
	if len(live.Members) > 0 {
		candidates := make([]string, 0, 2)
		if group := strings.TrimSpace(live.GroupBannerAsset); group != "" {
			candidates = append(candidates, filepath.Join("virtual_live", "select", "banner", group, group+".png"))
		}
		return append(candidates, c.bannerCandidates(source, live.Members[0])...)
	}
	if assetBundleName := strings.TrimSpace(live.AssetBundleName); assetBundleName != "" {
		return []string{filepath.Join("virtual_live", "select", "banner", assetBundleName, assetBundleName+".png")}
	}
	return c.eventBannerCandidates(source, live.ID)
}

func (c *Controller) eventBannerCandidates(source DataSource, liveID int) []string {
	if source == nil || liveID <= 0 {
		return nil
	}
	eventSource, ok := source.(eventBannerDataSource)
	if !ok {
		return nil
	}
	eventInfo, err := eventSource.GetEventByVirtualLiveID(liveID)
	if err != nil || eventInfo == nil {
		return nil
	}
	assetBundleName := strings.TrimSpace(eventInfo.AssetBundleName)
	if assetBundleName == "" {
		return nil
	}
	return []string{
		filepath.Join("home", "banner", assetBundleName, assetBundleName+".png"),
		filepath.Join("event", assetBundleName, "banner.png"),
		filepath.Join("event_story", assetBundleName, "screen_image", "banner_event_story.png"),
	}
}

func (c *Controller) buildRewardItems(source DataSource, live ResolvedLive) []drawing.VLiveRewardItem {
	if source == nil {
		return nil
	}
	for _, reward := range live.Rewards {
		if !isNormalVLiveReward(reward) {
			continue
		}
		box := source.GetResourceBoxByPurpose("virtual_live_reward", reward.ResourceBoxID)
		items := c.buildRewardBoxItems(source, box)
		if len(items) > 0 {
			return items
		}
	}
	return nil
}

func isNormalVLiveReward(reward Reward) bool {
	kind := strings.ToLower(strings.TrimSpace(reward.VirtualLiveType))
	return kind == "" || kind == "normal"
}

func (c *Controller) buildRewardBoxItems(source DataSource, box *provider.ResourceBox) []drawing.VLiveRewardItem {
	if box == nil {
		return nil
	}
	items := make([]drawing.VLiveRewardItem, 0, len(box.Details))
	for _, detail := range box.Details {
		imagePath := c.rewardImagePath(detail.ResourceType, detail.ResourceID)
		if strings.TrimSpace(imagePath) == "" {
			imagePath = c.newResourceRewardImagePath(source, detail.ResourceType, detail.ResourceID)
		}
		if strings.TrimSpace(imagePath) == "" {
			continue
		}
		items = append(items, drawing.VLiveRewardItem{
			ImagePath: imagePath,
			Quantity:  max(detail.ResourceQuantity, 1),
		})
	}
	return items
}

func (c *Controller) buildCharacterItems(source DataSource, live ResolvedLive) []drawing.VLiveCharacterItem {
	if source == nil {
		return nil
	}
	items := make([]drawing.VLiveCharacterItem, 0, len(live.Characters))
	seen := make(map[string]struct{}, len(live.Characters))
	for _, character := range live.Characters {
		performanceType := strings.ToLower(strings.TrimSpace(character.VirtualLivePerformanceType))
		if performanceType != "" && performanceType != "main_only" && performanceType != "both" {
			continue
		}
		gameCharacterUnit, err := source.GetGameCharacterUnit(character.GameCharacterUnitID)
		if err != nil || gameCharacterUnit == nil || gameCharacterUnit.GameCharacterID <= 0 {
			continue
		}
		iconPath := c.characterIconPath(gameCharacterUnit.GameCharacterID)
		if iconPath == "" {
			continue
		}
		if _, ok := seen[iconPath]; ok {
			continue
		}
		seen[iconPath] = struct{}{}
		items = append(items, drawing.VLiveCharacterItem{IconPath: iconPath})
	}
	return items
}

func (c *Controller) characterIconPath(characterID int) string {
	if nickname, ok := assets.CharacterIDToNickname[characterID]; ok {
		return assets.ResolveAssetPath(
			c.assets,
			assets.StaticImagesDir,
			filepath.Join("chara_icon", nickname+".png"),
			filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", characterID)),
		)
	}
	return assets.ResolveAssetPath(
		c.assets,
		assets.StaticImagesDir,
		filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", characterID)),
	)
}

func (c *Controller) rewardImagePath(resourceType string, resourceID int) string {
	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	if resourceType == "paid_jewel" {
		resourceType = "jewel"
	}
	switch resourceType {
	case "coin", "virtual_coin", "jewel":
		return assets.ResolveRegionAssetPath(
			c.assets,
			"jp",
			filepath.Join("thumbnail", "common_material", resourceType+".png"),
		)
	case "material":
		if resourceID <= 0 {
			return ""
		}
		return assets.ResolveRegionAssetPath(
			c.assets,
			"jp",
			filepath.Join("thumbnail", "material", fmt.Sprintf("material%d.png", resourceID)),
		)
	default:
		return ""
	}
}

func normalizeSchedules(items []Schedule) []Window {
	out := make([]Window, 0, len(items))
	for _, item := range items {
		startAt := unixTime(item.StartAt)
		endAt := unixTime(item.EndAt)
		if startAt.IsZero() || endAt.IsZero() || !startAt.Before(endAt) {
			continue
		}
		out = append(out, Window{StartAt: startAt, EndAt: endAt})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartAt.Equal(out[j].StartAt) {
			return out[i].EndAt.Before(out[j].EndAt)
		}
		return out[i].StartAt.Before(out[j].StartAt)
	})
	return out
}

func unixTime(value int64) time.Time {
	switch {
	case value <= 0:
		return time.Time{}
	case value < 1_000_000_000_000:
		return time.Unix(value, 0)
	default:
		return time.UnixMilli(value)
	}
}

func fallbackLiveName(name string, id int) string {
	if strings.TrimSpace(name) == "" {
		return i18n.T("vlive.fallback_name", i18n.Data{"ID": id})
	}
	return name
}
