package sk

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	renderassets "haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func (m *eventMeta) applyOverrides(req TrackerRankQuery) {
	if req.EventName != nil && strings.TrimSpace(*req.EventName) != "" {
		m.name = strings.TrimSpace(*req.EventName)
	}
	if req.EventStartAt != nil && *req.EventStartAt > 0 {
		m.startAt = *req.EventStartAt
	}
	if req.EventAggregateAt != nil && *req.EventAggregateAt > 0 {
		m.aggregateAt = *req.EventAggregateAt
	}
	if req.BannerImgPath != nil && strings.TrimSpace(*req.BannerImgPath) != "" {
		m.bannerPath = strings.TrimSpace(*req.BannerImgPath)
	}
}

func (c *Controller) resolveEventMeta(eventID int, region renderregion.Value) eventMeta {
	const defaultWindow = int64(6 * time.Hour / time.Millisecond)
	now := time.Now().UnixMilli()
	meta := eventMeta{
		name:        fmt.Sprintf("Event #%d", eventID),
		startAt:     now - defaultWindow,
		aggregateAt: now + defaultWindow,
	}
	eventSource := c.eventSourceForRegion(region.String())
	if c == nil || eventSource == nil {
		return meta
	}
	eventInfo, err := eventSource.GetEventByID(eventID)
	if err != nil || eventInfo == nil {
		return meta
	}
	if strings.TrimSpace(eventInfo.Name) != "" {
		meta.name = strings.TrimSpace(eventInfo.Name)
	}
	if eventInfo.StartAt > 0 {
		meta.startAt = eventInfo.StartAt
	}
	if eventInfo.AggregateAt > 0 {
		meta.aggregateAt = eventInfo.AggregateAt
	}
	if path := c.resolveEventBannerPath(eventInfo.AssetBundleName, region); path != "" {
		meta.bannerPath = path
	}
	return meta
}

func (c *Controller) resolveEventBannerPath(assetBundleName string, region renderregion.Value) string {
	if c == nil || c.assets == nil || strings.TrimSpace(assetBundleName) == "" {
		return ""
	}
	return renderassets.ResolveRegionAssetPath(
		c.assets, renderregion.WithDefault(region).String(),
		filepath.Join("home", "banner", assetBundleName, assetBundleName+".png"),
		filepath.Join("event", assetBundleName, "banner.png"),
	)
}

func (c *Controller) resolveCharacterIconPath(characterID int, _ renderregion.Value) string {
	if c == nil || c.assets == nil || characterID <= 0 {
		return ""
	}
	if nickname := renderassets.CharacterIDToNickname[characterID]; nickname != "" {
		return renderassets.ResolveAssetPath(
			c.assets,
			renderassets.StaticImagesDir,
			filepath.Join("chara_icon", nickname+".png"),
			filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", characterID)),
		)
	}
	return renderassets.ResolveAssetPath(
		c.assets,
		renderassets.StaticImagesDir,
		filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", characterID)),
	)
}

func (c *Controller) pickCurrentOrNextEventID(region string) int {
	eventSource := c.eventSourceForRegion(region)
	if c == nil || eventSource == nil {
		return 0
	}
	now := time.Now().UnixMilli()
	var current *masterdata.Event
	var next *masterdata.Event
	var latest *masterdata.Event
	for _, eventInfo := range eventSource.GetEvents() {
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
		return current.ID
	}
	if next != nil {
		return next.ID
	}
	if latest != nil {
		return latest.ID
	}
	return 0
}

func (c *Controller) eventSourceForRegion(region string) EventSource {
	if c == nil || c.events == nil {
		return nil
	}
	src, ok := c.events.SourceForRegion(renderregion.Normalize(region))
	if !ok {
		return nil
	}
	return src
}

func normalizeTrackerServer(region string) string {
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "jp", "cn", "tw", "kr", "en":
		return strings.ToLower(strings.TrimSpace(region))
	default:
		return ""
	}
}

func normalizeRanks(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	out := make([]int, 0, len(values))
	seen := make(map[int]struct{}, len(values))
	for _, rank := range values {
		if rank <= 0 {
			continue
		}
		if _, ok := seen[rank]; ok {
			continue
		}
		seen[rank] = struct{}{}
		out = append(out, rank)
	}
	sort.Ints(out)
	return out
}

func formatTrackerTimestamp(ts int64) string {
	if ts <= 0 {
		return time.Now().UTC().Format(time.RFC3339)
	}
	if ts > 1_000_000_000_000 {
		return time.UnixMilli(ts).UTC().Format(time.RFC3339)
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}

func pickTrackerDisplayName(name string, rank int) string {
	clean := strings.TrimSpace(name)
	if clean != "" {
		return clean
	}
	return fmt.Sprintf("Rank %d", rank)
}
