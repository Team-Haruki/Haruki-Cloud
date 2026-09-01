package music

import (
	"fmt"
	"math"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/meta"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func (c *Controller) ResolveCustomRoomMusicList(region string, eventRates []int, limit int) (map[int][]map[string]any, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	if len(eventRates) == 0 {
		return map[int][]map[string]any{}, nil
	}

	resolvedRegion, source, builder, err := c.resolveBuilder(region)
	if err != nil {
		return nil, err
	}

	view := c.resolveMusicMetaView(resolvedRegion.String())
	if view == nil {
		return nil, fmt.Errorf("music meta data is unavailable")
	}

	wantedRates := positiveEventRates(eventRates)
	if len(wantedRates) == 0 {
		return map[int][]map[string]any{}, nil
	}

	collector := customRoomMusicCollector{
		wantedRates: wantedRates,
		musicByID:   customRoomMusicCandidates(source, builder, resolvedRegion),
		limit:       limit,
		result:      make(map[int][]map[string]any, len(wantedRates)),
		seenByRate:  make(map[int]map[int]struct{}, len(wantedRates)),
	}
	view.Range(collector.collect)
	return collector.result, nil
}

type musicCandidate struct {
	id        int
	title     string
	coverPath string
}

func positiveEventRates(eventRates []int) map[int]struct{} {
	wantedRates := make(map[int]struct{}, len(eventRates))
	for _, rate := range eventRates {
		if rate > 0 {
			wantedRates[rate] = struct{}{}
		}
	}
	return wantedRates
}

func customRoomMusicCandidates(source DataSource, builder *Builder, region renderregion.Value) map[int]*musicCandidate {
	musicByID := make(map[int]*musicCandidate, len(source.GetMusics()))
	now := time.Now().UnixMilli()
	for _, item := range source.GetMusics() {
		if item == nil || item.PublishedAt > now {
			continue
		}
		if _, blocked := hiddenMusicIDs[item.ID]; blocked {
			continue
		}
		musicByID[item.ID] = &musicCandidate{
			id:        item.ID,
			title:     builder.buildDisplayMusicTitle(item, region),
			coverPath: builder.BuildMusicJacketPath(item.AssetBundleName, region),
		}
	}
	return musicByID
}

type customRoomMusicCollector struct {
	wantedRates map[int]struct{}
	musicByID   map[int]*musicCandidate
	limit       int
	result      map[int][]map[string]any
	seenByRate  map[int]map[int]struct{}
}

func (c *customRoomMusicCollector) collect(entry meta.Entry) bool {
	if !strings.EqualFold(strings.TrimSpace(entry.Difficulty()), "master") {
		return true
	}
	rate := int(math.Round(entry.Float("event_rate")))
	if _, ok := c.wantedRates[rate]; !ok || (c.limit > 0 && len(c.result[rate]) >= c.limit) {
		return true
	}
	musicID := entry.MusicID()
	candidate, ok := c.musicByID[musicID]
	if !ok {
		return true
	}
	if c.seenByRate[rate] == nil {
		c.seenByRate[rate] = make(map[int]struct{})
	}
	if _, exists := c.seenByRate[rate][musicID]; exists {
		return true
	}
	c.seenByRate[rate][musicID] = struct{}{}
	c.result[rate] = append(c.result[rate], map[string]any{
		"music_id":    candidate.id,
		"music_title": candidate.title,
		"music_cover": candidate.coverPath,
	})
	return true
}
