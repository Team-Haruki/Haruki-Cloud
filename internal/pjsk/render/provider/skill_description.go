package provider

import (
	"context"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

type skillCharacterLookup func(context.Context, int) (*masterdata.Character, error)

func formatSkillDescription(ctx context.Context, skillInfo *masterdata.Skill, cardCharacterID int, characters skillCharacterLookup) string {
	if skillInfo == nil {
		return ""
	}
	return skillPlaceholder.ReplaceAllStringFunc(skillInfo.Description, func(match string) string {
		return resolveSkillDescriptionPlaceholder(ctx, skillInfo, cardCharacterID, characters, match)
	})
}

func resolveSkillDescriptionPlaceholder(ctx context.Context, skillInfo *masterdata.Skill, cardCharacterID int, characters skillCharacterLookup, match string) string {
	parts := strings.Split(match[2:len(match)-2], ";")
	if len(parts) != 2 {
		return match
	}
	ids := parseSkillEffectIDs(parts[0])
	if len(ids) == 0 {
		return match
	}
	if parts[1] == "c" {
		return skillCharacterName(ctx, characters, cardCharacterID)
	}
	effects := skillEffectsByIDs(skillInfo, ids)
	if len(effects) != len(ids) {
		return "?"
	}
	switch len(effects) {
	case 1:
		return formatSingleEffect(effects[0], parts[1])
	case 2:
		return formatDualEffects(effects[0], effects[1], parts[1])
	default:
		return match
	}
}

func parseSkillEffectIDs(raw string) []int {
	ids := make([]int, 0, 2)
	for _, rawID := range strings.Split(raw, ",") {
		if value, err := strconv.Atoi(strings.TrimSpace(rawID)); err == nil {
			ids = append(ids, value)
		}
	}
	return ids
}

func skillCharacterName(ctx context.Context, characters skillCharacterLookup, characterID int) string {
	if characters != nil {
		if character, err := characters(ctx, characterID); err == nil && character != nil {
			return character.FirstName + character.GivenName
		}
	}
	return "???"
}

func skillEffectsByIDs(skillInfo *masterdata.Skill, ids []int) []*masterdata.SkillEffect {
	effects := make([]*masterdata.SkillEffect, 0, len(ids))
	for _, effectID := range ids {
		for index := range skillInfo.SkillEffects {
			if skillInfo.SkillEffects[index].ID == effectID {
				effects = append(effects, &skillInfo.SkillEffects[index])
				break
			}
		}
	}
	return effects
}
