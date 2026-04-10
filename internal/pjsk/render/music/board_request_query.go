package music

import (
	"strings"

	"haruki-cloud/internal/pjsk/render/common"
)

func normalizeMusicBoardQuery(query BoardQuery) musicBoardResolvedQuery {
	liveType := strings.ToLower(strings.TrimSpace(query.LiveType))
	if _, ok := musicBoardLiveTypes[liveType]; !ok {
		liveType = "solo"
	}

	target := strings.ToLower(strings.TrimSpace(query.Target))
	if _, ok := musicBoardTargets[target]; !ok {
		switch liveType {
		case "multi":
			target = "pt/time"
		default:
			target = "score"
		}
	}

	strategy := strings.ToLower(strings.TrimSpace(query.SkillStrategy))
	if _, ok := musicBoardStrategies[strategy]; !ok {
		switch liveType {
		case "solo":
			strategy = "max"
		default:
			strategy = "avg"
		}
	}

	page := query.Page
	if page <= 0 {
		page = 1
	}

	power := query.Power
	if power <= 0 {
		power = musicBoardDefaultPower
	}

	deckBonus := query.DeckBonus
	if deckBonus <= 0 {
		deckBonus = musicBoardDefaultDeckBonus
	}

	playInterval := query.PlayInterval
	if playInterval <= 0 {
		switch liveType {
		case "multi":
			playInterval = musicBoardDefaultMultiInterval
		default:
			playInterval = musicBoardDefaultSoloInterval
		}
	}

	skills := normalizeMusicBoardSkills(query.Skills, liveType)

	diffFilter := make([]string, 0, len(query.DiffFilter))
	for _, diff := range query.DiffFilter {
		normalized := normalizeDifficulty(diff)
		if normalized == "" {
			continue
		}
		if common.ContainsString(diffFilter, normalized) {
			continue
		}
		diffFilter = append(diffFilter, normalized)
	}

	specQueries := make([]string, 0, len(query.SpecQueries))
	for _, raw := range query.SpecQueries {
		text := strings.TrimSpace(raw)
		if text == "" {
			continue
		}
		specQueries = append(specQueries, text)
	}

	return musicBoardResolvedQuery{
		LiveType:      liveType,
		Target:        target,
		Ascend:        query.Ascend,
		Page:          page,
		SkillStrategy: strategy,
		Skills:        skills,
		Power:         power,
		DeckBonus:     deckBonus,
		PlayInterval:  playInterval,
		DiffFilter:    diffFilter,
		LevelFilter:   strings.TrimSpace(query.LevelFilter),
		SpecQueries:   specQueries,
	}
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
		return append([]float64(nil), clean[:5]...)
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
