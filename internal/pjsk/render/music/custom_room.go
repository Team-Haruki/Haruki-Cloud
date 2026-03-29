package music

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

func (c *Controller) ResolveCustomRoomMusicList(region string, eventRates []int, limit int) (map[int][]map[string]interface{}, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	if len(eventRates) == 0 {
		return map[int][]map[string]interface{}{}, nil
	}

	resolvedRegion, source, builder, err := c.resolveBuilder(region)
	if err != nil {
		return nil, err
	}

	payload := c.resolveMusicMetaPayload(resolvedRegion.String())
	if len(payload) == 0 {
		return nil, fmt.Errorf("music meta data is unavailable")
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, fmt.Errorf("decode music meta data: %w", err)
	}

	wantedRates := make(map[int]struct{}, len(eventRates))
	for _, rate := range eventRates {
		if rate > 0 {
			wantedRates[rate] = struct{}{}
		}
	}
	if len(wantedRates) == 0 {
		return map[int][]map[string]interface{}{}, nil
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

	result := make(map[int][]map[string]interface{}, len(wantedRates))
	seenByRate := make(map[int]map[int]struct{}, len(wantedRates))
	for _, item := range items {
		if !strings.EqualFold(strings.TrimSpace(stringValue(item["difficulty"])), "master") {
			continue
		}

		rate := int(math.Round(floatValue(item["event_rate"])))
		if _, ok := wantedRates[rate]; !ok {
			continue
		}
		if limit > 0 && len(result[rate]) >= limit {
			continue
		}

		musicID := musicMetaID(item)
		candidate, ok := musicByID[musicID]
		if !ok {
			continue
		}
		if _, ok := seenByRate[rate]; !ok {
			seenByRate[rate] = make(map[int]struct{})
		}
		if _, exists := seenByRate[rate][musicID]; exists {
			continue
		}
		seenByRate[rate][musicID] = struct{}{}

		result[rate] = append(result[rate], map[string]interface{}{
			"music_id":    candidate.id,
			"music_title": candidate.title,
			"music_cover": candidate.coverPath,
		})
	}

	return result, nil
}

func (c *Controller) resolveMusicMetaPayload(region string) []byte {
	if c != nil && c.metaLoader != nil {
		if payload := c.metaLoader.Get(region); len(payload) > 0 {
			return payload
		}
	}

	if snapshot := c.currentSnapshot(); snapshot != nil {
		if payload := snapshot.MusicMetaBytes(); len(payload) > 0 {
			return payload
		}
	}

	return nil
}

type musicCandidate struct {
	id        int
	title     string
	coverPath string
}
