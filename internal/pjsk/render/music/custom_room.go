package music

import (
	"fmt"
	"math"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/meta"
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

	wantedRates := make(map[int]struct{}, len(eventRates))
	for _, rate := range eventRates {
		if rate > 0 {
			wantedRates[rate] = struct{}{}
		}
	}
	if len(wantedRates) == 0 {
		return map[int][]map[string]any{}, nil
	}

	musicByID := make(map[int]*musicCandidate, len(source.GetMusics()))
	now := time.Now().UnixMilli()
	for _, item := range source.GetMusics() {
		if item == nil {
			continue
		}
		if _, blocked := hiddenMusicIDs[item.ID]; blocked {
			continue
		}
		if item.PublishedAt > now {
			continue
		}
		musicByID[item.ID] = &musicCandidate{
			id:        item.ID,
			title:     builder.buildDisplayMusicTitle(item, resolvedRegion),
			coverPath: builder.BuildMusicJacketPath(item.AssetBundleName, resolvedRegion),
		}
	}

	result := make(map[int][]map[string]any, len(wantedRates))
	seenByRate := make(map[int]map[int]struct{}, len(wantedRates))
	view.Range(func(entry meta.Entry) bool {
		if !strings.EqualFold(strings.TrimSpace(entry.Difficulty()), "master") {
			return true
		}

		rate := int(math.Round(entry.Float("event_rate")))
		if _, ok := wantedRates[rate]; !ok {
			return true
		}
		if limit > 0 && len(result[rate]) >= limit {
			return true
		}

		musicID := entry.MusicID()
		candidate, ok := musicByID[musicID]
		if !ok {
			return true
		}
		if _, ok := seenByRate[rate]; !ok {
			seenByRate[rate] = make(map[int]struct{})
		}
		if _, exists := seenByRate[rate][musicID]; exists {
			return true
		}
		seenByRate[rate][musicID] = struct{}{}

		result[rate] = append(result[rate], map[string]any{
			"music_id":    candidate.id,
			"music_title": candidate.title,
			"music_cover": candidate.coverPath,
		})
		return true
	})

	return result, nil
}

type musicCandidate struct {
	id        int
	title     string
	coverPath string
}
