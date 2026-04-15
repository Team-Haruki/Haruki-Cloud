package deck

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/drawing"
)

func (c *Controller) resolveProfile(region renderregion.Value, override *drawing.DetailedProfileCardRequest, source string) *drawing.DetailedProfileCardRequest {
	if override != nil {
		profile := *override
		return &profile
	}
	if c.snapshot != nil {
		if profile := c.snapshot.DetailedProfile(region); profile != nil {
			return profile
		}
	}
	return &drawing.DetailedProfileCardRequest{
		ID:              "1",
		Region:          strings.ToUpper(region.String()),
		Nickname:        "Unknown",
		Source:          source,
		UpdateTime:      time.Now().UnixMilli(),
		IsHideUID:       true,
		LeaderImagePath: "",
		HasFrame:        false,
	}
}

func (c *Controller) resolveMusicMeta(region renderregion.Value) []byte {
	if c != nil && c.metaLoader != nil {
		if payload := c.metaLoader.Get(region.String()); len(payload) > 0 {
			return payload
		}
	}
	if c != nil && c.snapshot != nil {
		return c.snapshot.MusicMetaBytes()
	}
	return nil
}

func (c *Controller) resolveUserDataFilePath() string {
	if c == nil || c.snapshot == nil {
		return ""
	}
	return c.snapshot.RawFilePath()
}

func (c *Controller) resolveMusicMetaFilePath() string {
	if c == nil || c.snapshot == nil {
		return ""
	}
	return c.snapshot.MusicMetaPath()
}

func (c *Controller) pickCurrentOrNextEventID(region renderregion.Value) int {
	_, eventSource, ok := c.resolveEventSource(region)
	if !ok {
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

func (c *Controller) resolveCardSource(requested renderregion.Value) (renderregion.Value, CardSource, error) {
	if c == nil || c.cardSources == nil {
		return renderregion.WithDefault(requested), nil, fmt.Errorf("deck card source is not configured")
	}
	normalized := renderregion.Normalize(requested.String())
	if !normalized.IsZero() {
		source, ok := c.cardSources.SourceForRegion(normalized)
		if !ok {
			return normalized, nil, fmt.Errorf("no deck card source for region %s", normalized)
		}
		return normalized, source, nil
	}
	source, ok := c.cardSources.SourceForRegion(renderregion.Unknown)
	if !ok {
		return c.cardSources.ResolveRegion(renderregion.Unknown), nil, fmt.Errorf("deck card source is not configured")
	}
	return renderregion.WithDefault(source.DefaultRegion()), source, nil
}

func (c *Controller) resolveEventSource(requested renderregion.Value) (renderregion.Value, EventSource, bool) {
	if c == nil || c.eventSources == nil {
		return renderregion.WithDefault(requested), nil, false
	}
	normalized := renderregion.Normalize(requested.String())
	if !normalized.IsZero() {
		source, ok := c.eventSources.SourceForRegion(normalized)
		return normalized, source, ok
	}
	source, ok := c.eventSources.SourceForRegion(renderregion.Unknown)
	if !ok {
		return c.eventSources.ResolveRegion(renderregion.Unknown), nil, false
	}
	return renderregion.WithDefault(source.DefaultRegion()), source, true
}

func (c *Controller) resolveEventBannerPath(assetBundleName string, region renderregion.Value) string {
	if c == nil || c.assets == nil || strings.TrimSpace(assetBundleName) == "" {
		return ""
	}
	return assets.ResolveRegionAssetPath(
		c.assets, region.String(),
		filepath.Join("home", "banner", assetBundleName, assetBundleName+".png"),
		filepath.Join("event", assetBundleName, "banner.png"),
	)
}
