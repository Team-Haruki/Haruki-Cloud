package vlive

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	regionsource "haruki-cloud/internal/pjsk/render/source"
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
	clone.drawing = c.drawing.WithContext(ctx)
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
		return nil, renderregion.Unknown, fmt.Errorf("vlive controller is not configured")
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
		if live == nil {
			continue
		}

		startAt := unixTime(live.StartAt)
		endAt := unixTime(live.EndAt)
		if startAt.IsZero() || endAt.IsZero() {
			continue
		}
		if !now.Before(endAt) {
			continue
		}
		if startAt.Sub(now) >= 7*24*time.Hour {
			continue
		}
		if endAt.Sub(startAt) >= 30*24*time.Hour {
			continue
		}

		schedules := normalizeSchedules(live.Schedules)
		resolved := ResolvedLive{
			ID:              live.ID,
			Name:            strings.TrimSpace(live.Name),
			AssetBundleName: strings.TrimSpace(live.AssetBundleName),
			StartAt:         startAt,
			EndAt:           endAt,
			Rewards:         append([]Reward(nil), live.Rewards...),
			Characters:      append([]Character(nil), live.Characters...),
		}
		for _, schedule := range schedules {
			if now.Before(schedule.EndAt) {
				resolved.Current = new(Window)
				*resolved.Current = schedule
				resolved.Living = !now.Before(schedule.StartAt)
				break
			}
		}
		for _, schedule := range schedules {
			if now.Before(schedule.StartAt) {
				resolved.RestCount++
			}
		}
		if resolved.Current == nil {
			if now.Before(startAt) {
				resolved.Current = &Window{StartAt: startAt, EndAt: endAt}
			} else if now.Before(endAt) {
				resolved.Current = &Window{StartAt: startAt, EndAt: endAt}
				resolved.Living = true
			}
		}
		result = append(result, resolved)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].StartAt.Equal(result[j].StartAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].StartAt.Before(result[j].StartAt)
	})
	return result, region, nil
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

	for _, live := range lives {
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
		req.Lives = append(req.Lives, item)
	}

	return req, nil
}

func (c *Controller) RenderList(query ListQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	req, err := c.BuildListRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateVLiveList(req)
}

func (c *Controller) RenderText(query ListQuery) (string, error) {
	lives, region, err := c.ResolveLives(query)
	if err != nil {
		return "", err
	}
	if len(lives) == 0 {
		return "当前没有虚拟Live", nil
	}

	loc, timeZone := displaytime.LoadLocation(query.TimeZone)

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("%s 虚拟Live列表", strings.ToUpper(region.String())))
	for _, live := range lives {
		builder.WriteString("\n\n")
		builder.WriteString(fmt.Sprintf("【%d】%s\n", live.ID, fallbackLiveName(live.Name, live.ID)))
		builder.WriteString(fmt.Sprintf("开始: %s\n", displaytime.FormatTime(live.StartAt.In(loc), "2006-01-02 15:04")))
		builder.WriteString(fmt.Sprintf("结束: %s\n", displaytime.FormatTime(live.EndAt.In(loc), "2006-01-02 15:04")))
		builder.WriteString("状态: ")
		switch {
		case live.Living:
			builder.WriteString("当前Live进行中")
		case live.Current != nil:
			builder.WriteString(fmt.Sprintf("下一场: %s", displaytime.FormatTime(live.Current.StartAt.In(loc), "2006-01-02 15:04")))
		default:
			builder.WriteString("已结束")
		}
		builder.WriteString(fmt.Sprintf(" | 剩余场次: %d", live.RestCount))
	}
	builder.WriteString(fmt.Sprintf("\n\n时区: %s", timeZone))
	return builder.String(), nil
}

func (c *Controller) resolveRegion(region string) renderregion.Value {
	if c != nil && c.sources != nil {
		return c.sources.ResolveRegion(renderregion.Normalize(region))
	}
	return renderregion.WithDefault(renderregion.Normalize(region))
}

func (c *Controller) bannerPath(source DataSource, region renderregion.Value, live ResolvedLive) string {
	if bannerPath := c.eventBannerPath(source, region, live.ID); bannerPath != "" {
		return bannerPath
	}
	if bannerPath := c.birthdayBannerPath(source, region, live); bannerPath != "" {
		return bannerPath
	}

	assetBundleName := strings.TrimSpace(live.AssetBundleName)
	if assetBundleName == "" {
		return ""
	}
	return assets.ResolveRegionAssetPath(
		c.assets,
		region.String(),
		filepath.Join("home", "banner", assetBundleName, assetBundleName+".png"),
		filepath.Join("virtual_live", "select", "banner", assetBundleName, assetBundleName+".png"),
		filepath.Join("virtual_live", "select", "banner", assetBundleName+"_rip", assetBundleName+".png"),
	)
}

func (c *Controller) eventBannerPath(source DataSource, region renderregion.Value, liveID int) string {
	if source == nil || liveID <= 0 {
		return ""
	}
	eventSource, ok := source.(eventBannerDataSource)
	if !ok {
		return ""
	}
	eventInfo, err := eventSource.GetEventByVirtualLiveID(liveID)
	if err != nil || eventInfo == nil {
		return ""
	}
	return assets.ResolveEventBannerPath(c.assets, region.String(), eventInfo.AssetBundleName)
}

func (c *Controller) birthdayBannerPath(source DataSource, region renderregion.Value, live ResolvedLive) string {
	if !isBirthdayLive(live.Name) {
		return ""
	}
	nickname := primaryLiveCharacterNickname(source, live.Characters)
	if nickname == "" {
		return ""
	}
	assetBundleName := fmt.Sprintf("banner_birthday_%s_%d", nickname, live.StartAt.Year())
	return assets.ResolveRegionAssetPath(
		c.assets,
		region.String(),
		filepath.Join("home", "banner", assetBundleName, assetBundleName+".png"),
	)
}

func isBirthdayLive(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	return strings.Contains(normalized, "birthday") ||
		strings.Contains(normalized, "生日") ||
		strings.Contains(normalized, "誕生日")
}

func primaryLiveCharacterNickname(source DataSource, characters []Character) string {
	if source == nil || len(characters) == 0 {
		return ""
	}
	for _, wantType := range []string{"main_only", "both", ""} {
		for _, item := range characters {
			performanceType := strings.TrimSpace(item.VirtualLivePerformanceType)
			if wantType != "" && performanceType != wantType {
				continue
			}
			if wantType == "" && performanceType != "" && performanceType != "both" {
				continue
			}
			unit, err := source.GetGameCharacterUnit(item.GameCharacterUnitID)
			if err != nil || unit == nil {
				continue
			}
			if nickname, ok := assets.CharacterIDToNickname[unit.GameCharacterID]; ok && nickname != "" {
				return nickname
			}
		}
	}
	return ""
}

func (c *Controller) buildRewardItems(source DataSource, live ResolvedLive) []drawing.VLiveRewardItem {
	if source == nil {
		return nil
	}
	var items []drawing.VLiveRewardItem
	for _, reward := range live.Rewards {
		if kind := strings.ToLower(strings.TrimSpace(reward.VirtualLiveType)); kind != "" && kind != "normal" {
			continue
		}
		box := source.GetResourceBoxByPurpose("virtual_live_reward", reward.ResourceBoxID)
		if box == nil {
			continue
		}
		for _, detail := range box.Details {
			imagePath := c.rewardImagePath(detail.ResourceType, detail.ResourceID)
			if strings.TrimSpace(imagePath) == "" {
				continue
			}
			quantity := detail.ResourceQuantity
			if quantity <= 0 {
				quantity = 1
			}
			items = append(items, drawing.VLiveRewardItem{
				ImagePath: imagePath,
				Quantity:  quantity,
			})
		}
		if len(items) > 0 {
			break
		}
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
		return fmt.Sprintf("Virtual Live #%d", id)
	}
	return name
}
