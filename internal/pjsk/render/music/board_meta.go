package music

import (
	"sort"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/meta"
)

func (c *Controller) loadMusicBoardMetaMap(region string) map[int][]drawing.MusicMetaInfo {
	view := c.resolveMusicMetaView(region)
	if view == nil {
		return nil
	}

	result := make(map[int][]drawing.MusicMetaInfo)
	view.Range(func(entry meta.Entry) bool {
		musicID := entry.MusicID()
		if musicID <= 0 {
			return true
		}
		result[musicID] = append(result[musicID], drawing.MusicMetaInfo{
			Difficulty:      normalizedDifficultyValue(entry.Difficulty()),
			MusicTime:       entry.Float("music_time"),
			TapCount:        entry.Int("tap_count"),
			EventRate:       entry.Float("event_rate"),
			BaseScore:       entry.Float("base_score"),
			BaseScoreAuto:   entry.Float("base_score_auto"),
			SkillScoreSolo:  entry.FloatSlice("skill_score_solo"),
			SkillScoreAuto:  entry.FloatSlice("skill_score_auto"),
			SkillScoreMulti: entry.FloatSlice("skill_score_multi"),
			FeverScore:      entry.Float("fever_score"),
		})
		return true
	})

	for musicID := range result {
		sort.SliceStable(result[musicID], func(i, j int) bool {
			return boardDifficultyPriority(result[musicID][i].Difficulty) > boardDifficultyPriority(result[musicID][j].Difficulty)
		})
	}
	return result
}
