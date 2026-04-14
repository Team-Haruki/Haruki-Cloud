package provider

import "strings"

func normalizeCardSkillType(skillType string) string {
	switch strings.ToLower(strings.TrimSpace(skillType)) {
	case "judgment_accuracy_up":
		return "judgment_up"
	default:
		return strings.ToLower(strings.TrimSpace(skillType))
	}
}

func cardSkillTypesMatch(filterSkillType, actualSkillType string) bool {
	filterSkillType = normalizeCardSkillType(filterSkillType)
	actualSkillType = normalizeCardSkillType(actualSkillType)
	if filterSkillType == "" || actualSkillType == "" {
		return false
	}
	return filterSkillType == actualSkillType
}
