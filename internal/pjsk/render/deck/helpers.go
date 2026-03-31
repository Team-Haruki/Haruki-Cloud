package deck

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/userdata"
)

// Option extractors

func optionString(option map[string]interface{}, key string) string {
	if option == nil {
		return ""
	}
	if value, ok := option[key].(string); ok {
		return value
	}
	return ""
}

func optionInt(option map[string]interface{}, key string) int {
	if option == nil {
		return 0
	}
	switch value := option[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	case float32:
		return int(value)
	default:
		return 0
	}
}

func optionFloat(option map[string]interface{}, key string) (float64, bool) {
	if option == nil {
		return 0, false
	}
	switch value := option[key].(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	default:
		return 0, false
	}
}

// Normalization functions

func normalizeRecommendAlgorithm(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "dfs", "sa", "ga", "all":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func normalizeRecommendLiveType(recType string, raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return ""
	case "multi", "solo", "auto":
		switch recType {
		case "challenge":
			if strings.EqualFold(strings.TrimSpace(raw), "auto") {
				return "challenge_auto"
			}
			return "challenge"
		case "mysekai":
			return "mysekai"
		default:
			return strings.ToLower(strings.TrimSpace(raw))
		}
	case "challenge", "challenge_auto", "mysekai":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func normalizeRecommendTarget(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "score", "power", "skill", "bonus":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func normalizeRecommendDifficulty(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "easy", "normal", "hard", "expert", "master", "append":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func normalizeRecommendAttr(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "cute", "cool", "pure", "happy", "mysterious":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func normalizeRecommendUnit(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "light_sound", "idol", "street", "theme_park", "school_refusal", "piapro":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func normalizeRecommendStrategy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "max", "min", "average":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

// Asset path resolvers

func (c *Controller) resolveCharacterIconPath(characterID int) string {
	if c == nil || c.assets == nil || characterID <= 0 {
		return ""
	}
	if nickname, ok := assets.CharacterIDToNickname[characterID]; ok {
		return assets.ResolveAssetPath(
			c.assets,
			assets.StaticImagesDir,
			filepath.Join("chara_icon", nickname+".png"),
			filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", characterID)),
		)
	}
	return assets.ResolveAssetPath(
		c.assets,
		assets.StaticImagesDir,
		filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", characterID)),
	)
}

func (c *Controller) resolveUnitIconPath(unit string) string {
	icon := assets.UnitIconFilename(unit)
	if icon == "" {
		return ""
	}
	return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, icon+".png")
}

func (c *Controller) resolveAttrIconPath(attr string) string {
	attr = normalizeRecommendAttr(attr)
	if attr == "" {
		return ""
	}
	return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, filepath.Join("card", fmt.Sprintf("attr_icon_%s.png", attr)))
}

// Deck config defaults

func defaultDeckConfig12() map[string]interface{} {
	return map[string]interface{}{
		"disable":      false,
		"level_max":    true,
		"episode_read": true,
		"master_max":   true,
		"skill_max":    true,
		"canvas":       false,
	}
}

func defaultDeckConfig34bd() map[string]interface{} {
	return map[string]interface{}{
		"disable":      false,
		"level_max":    true,
		"episode_read": false,
		"master_max":   false,
		"skill_max":    false,
		"canvas":       false,
	}
}

func noChangeDeckConfig() map[string]interface{} {
	return map[string]interface{}{
		"disable":      false,
		"level_max":    false,
		"episode_read": false,
		"master_max":   false,
		"skill_max":    false,
		"canvas":       false,
	}
}

func defaultEventBonus(recommendType string) float64 {
	if recommendType == "event" || recommendType == "bonus" {
		return 20.0
	}
	return 0.0
}

func pickBonusTargets(list []int, args string) []int {
	if len(list) > 0 {
		return list
	}
	parts := strings.Fields(strings.TrimSpace(args))
	var values []int
	for _, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value <= 0 {
			continue
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return []int{120}
	}
	return values
}

// Conversion helpers

func toInterfaceSlice(values []string) []interface{} {
	if len(values) == 0 {
		return nil
	}
	result := make([]interface{}, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func toInterfaceMap(values map[string]float64) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]interface{}, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func float64Ptr(value float64) *float64 {
	return &value
}

// Card helpers

func isAfterTraining(userCard userdata.RawUserCard) bool {
	return strings.EqualFold(userCard.SpecialTrainingStatus, "done")
}

func calculateDeckCardPower(card *masterdata.Card) int {
	if card == nil {
		return 0
	}
	var p1, p2, p3 int
	for _, param := range card.CardParameters {
		switch param.CardParameterType {
		case "param1":
			if param.Power > p1 {
				p1 = param.Power
			}
		case "param2":
			if param.Power > p2 {
				p2 = param.Power
			}
		case "param3":
			if param.Power > p3 {
				p3 = param.Power
			}
		}
	}
	return p1 + p2 + p3 + card.SpecialTrainingPower1BonusFixed + card.SpecialTrainingPower2BonusFixed + card.SpecialTrainingPower3BonusFixed
}
