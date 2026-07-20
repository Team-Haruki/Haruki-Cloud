package music

import (
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/meta"
)

func findMusicMeta(payload []byte, musicID int, difficulty string) map[string]any {
	if len(payload) == 0 || musicID <= 0 {
		return nil
	}

	view, err := meta.Parse(payload)
	if err != nil {
		return nil
	}
	return findMusicMetaInView(view, musicID, difficulty)
}

func findMusicMetaInView(view *meta.View, musicID int, difficulty string) map[string]any {
	entry, ok := view.Find(musicID, normalizeDifficulty(difficulty))
	if !ok {
		return nil
	}
	return entry.Map()
}

func musicMetaInfosFromView(view *meta.View, musicID int) []drawing.MusicMetaInfo {
	entries := view.FindAll(musicID)
	if len(entries) == 0 {
		return nil
	}
	result := make([]drawing.MusicMetaInfo, 0, len(entries))
	for _, entry := range entries {
		result = append(result, drawing.MusicMetaInfo{
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
	}
	sort.SliceStable(result, func(i, j int) bool {
		return difficultyOrder(result[i].Difficulty) < difficultyOrder(result[j].Difficulty)
	})
	return result
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func normalizedDifficultyValue(value any) string {
	raw := strings.TrimSpace(stringValue(value))
	if normalized := normalizeDifficulty(raw); normalized != "" {
		return normalized
	}
	return raw
}
