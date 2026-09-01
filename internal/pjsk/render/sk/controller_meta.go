package sk

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/eventutil"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderassets "haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
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

func (c *Controller) applyWorldBloomChapterMeta(req TrackerRankQuery, meta eventMeta) eventMeta {
	if req.WlCharacterID == nil || *req.WlCharacterID <= 0 {
		return meta
	}
	eventSource := c.eventSourceForRegion(req.Region)
	chapterSource, ok := eventSource.(WorldBloomChapterSource)
	if !ok || chapterSource == nil {
		return meta
	}
	for _, chapter := range chapterSource.GetWorldBloomChapters(c.contextOrBackground(), req.EventID) {
		if chapter == nil || chapter.GameCharacterID == nil || *chapter.GameCharacterID != *req.WlCharacterID {
			continue
		}
		if chapter.ChapterStartAt > 0 {
			meta.startAt = chapter.ChapterStartAt
		}
		if chapter.AggregateAt > 0 {
			meta.aggregateAt = chapter.AggregateAt
		}
		return meta
	}
	return meta
}

func (c *Controller) resolveEventBannerPath(assetBundleName string, region renderregion.Value) string {
	if c == nil || c.assets == nil || strings.TrimSpace(assetBundleName) == "" {
		return ""
	}
	return renderassets.ResolveEventBannerPath(c.assets, renderregion.WithDefault(region).String(), assetBundleName)
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
	candidates := eventCandidates{}
	for _, eventInfo := range eventSource.GetEvents() {
		candidates.consider(eventInfo, now)
	}
	return candidates.preferredID()
}

type eventCandidates struct {
	current *masterdata.Event
	next    *masterdata.Event
	latest  *masterdata.Event
}

func (c *eventCandidates) consider(eventInfo *masterdata.Event, now int64) {
	if eventInfo == nil {
		return
	}
	if c.latest == nil || eventInfo.StartAt > c.latest.StartAt {
		c.latest = eventInfo
	}
	if eventutil.IsCurrent(eventInfo.StartAt, eventInfo.AggregateAt, eventInfo.ClosedAt, now) {
		if c.current == nil || eventInfo.StartAt > c.current.StartAt {
			c.current = eventInfo
		}
		return
	}
	if eventInfo.StartAt > now && (c.next == nil || eventInfo.StartAt < c.next.StartAt) {
		c.next = eventInfo
	}
}

func (c eventCandidates) preferredID() int {
	switch {
	case c.current != nil:
		return c.current.ID
	case c.next != nil:
		return c.next.ID
	case c.latest != nil:
		return c.latest.ID
	default:
		return 0
	}
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
	switch renderregion.Normalize(region) {
	case renderregion.JP, renderregion.CN, renderregion.TW, renderregion.KR, renderregion.EN:
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

func formatTrackerTimestamp(ts int64) int64 {
	if ts <= 0 {
		return time.Now().UTC().UnixMilli()
	}
	if ts > 1_000_000_000_000 {
		return ts
	}
	return time.Unix(ts, 0).UTC().UnixMilli()
}
