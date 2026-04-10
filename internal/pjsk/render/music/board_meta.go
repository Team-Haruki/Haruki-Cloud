package music

import (
	"encoding/json"
	"sort"

	"haruki-cloud/utils/drawing"
)

func (c *Controller) loadMusicBoardMetaMap(region string) map[int][]drawing.MusicMetaInfo {
	payload := c.musicBoardMetaPayload(region)
	if len(payload) == 0 {
		return nil
	}

	var items []map[string]any
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil
	}

	result := make(map[int][]drawing.MusicMetaInfo)
	for _, item := range items {
		musicID := musicMetaID(item)
		if musicID <= 0 {
			continue
		}
		result[musicID] = append(result[musicID], drawing.MusicMetaInfo{
			Difficulty:      normalizedDifficultyValue(item["difficulty"]),
			MusicTime:       floatValue(item["music_time"]),
			TapCount:        intValue(item["tap_count"]),
			EventRate:       floatValue(item["event_rate"]),
			BaseScore:       floatValue(item["base_score"]),
			BaseScoreAuto:   floatValue(item["base_score_auto"]),
			SkillScoreSolo:  floatSliceValue(item["skill_score_solo"]),
			SkillScoreAuto:  floatSliceValue(item["skill_score_auto"]),
			SkillScoreMulti: floatSliceValue(item["skill_score_multi"]),
			FeverScore:      floatValue(item["fever_score"]),
		})
	}

	for musicID := range result {
		sort.SliceStable(result[musicID], func(i, j int) bool {
			return boardDifficultyPriority(result[musicID][i].Difficulty) > boardDifficultyPriority(result[musicID][j].Difficulty)
		})
	}
	return result
}

func (c *Controller) musicBoardMetaPayload(region string) []byte {
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
