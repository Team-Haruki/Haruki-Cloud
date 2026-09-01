package music

import (
	"slices"
	"strings"

	"haruki-cloud/internal/pjsk/render/common"
)

func normalizeMusicBoardQuery(query BoardQuery) musicBoardResolvedQuery {
	liveType := normalizeMusicBoardLiveType(query.LiveType)
	skills := normalizeMusicBoardSkills(query.Skills, liveType)
	return musicBoardResolvedQuery{
		LiveType:      liveType,
		Target:        normalizeMusicBoardTarget(query.Target, liveType),
		Ascend:        query.Ascend,
		Page:          positiveMusicBoardValue(query.Page, 1),
		SkillStrategy: normalizeMusicBoardStrategy(query.SkillStrategy, liveType),
		Skills:        skills,
		Power:         positiveMusicBoardValue(query.Power, musicBoardDefaultPower),
		DeckBonus:     positiveMusicBoardFloat(query.DeckBonus, musicBoardDefaultDeckBonus),
		PlayInterval:  normalizeMusicBoardPlayInterval(query.PlayInterval, liveType),
		DiffFilter:    normalizeMusicBoardDiffFilter(query.DiffFilter),
		LevelFilter:   strings.TrimSpace(query.LevelFilter),
		SpecQueries:   compactMusicBoardStrings(query.SpecQueries),
	}
}

func normalizeMusicBoardLiveType(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := musicBoardLiveTypes[value]; ok {
		return value
	}
	return "solo"
}

func normalizeMusicBoardTarget(raw, liveType string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := musicBoardTargets[value]; ok {
		return value
	}
	if liveType == "multi" {
		return "pt/time"
	}
	return "score"
}

func normalizeMusicBoardStrategy(raw, liveType string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := musicBoardStrategies[value]; ok {
		return value
	}
	if liveType == "solo" {
		return "max"
	}
	return "avg"
}

func positiveMusicBoardValue(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func positiveMusicBoardFloat(value, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}

func normalizeMusicBoardPlayInterval(value float64, liveType string) float64 {
	if value > 0 {
		return value
	}
	if liveType == "multi" {
		return musicBoardDefaultMultiInterval
	}
	return musicBoardDefaultSoloInterval
}

func normalizeMusicBoardDiffFilter(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeDifficulty(value)
		if normalized != "" && !common.ContainsString(result, normalized) {
			result = append(result, normalized)
		}
	}
	return result
}

func compactMusicBoardStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func normalizeMusicBoardSkills(skills []float64, liveType string) []float64 {
	clean := make([]float64, 0, 5)
	for _, skill := range skills {
		if skill <= 0 {
			continue
		}
		clean = append(clean, skill)
	}

	switch {
	case liveType == "multi" && len(clean) == 1:
		return []float64{clean[0], clean[0], clean[0], clean[0], clean[0]}
	case len(clean) >= 5:
		return slices.Clone(clean[:5])
	case liveType == "multi":
		return []float64{
			musicBoardDefaultMultiSkill,
			musicBoardDefaultMultiSkill,
			musicBoardDefaultMultiSkill,
			musicBoardDefaultMultiSkill,
			musicBoardDefaultMultiSkill,
		}
	default:
		return []float64{
			musicBoardDefaultSoloSkill,
			musicBoardDefaultSoloSkill,
			musicBoardDefaultSoloSkill,
			musicBoardDefaultSoloSkill,
			musicBoardDefaultSoloSkill,
		}
	}
}
