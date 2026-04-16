package deck

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/userdata"
)

// Option extractors

func optionString(option map[string]any, key string) string {
	if option == nil {
		return ""
	}
	if value, ok := option[key].(string); ok {
		return value
	}
	return ""
}

func optionInt(option map[string]any, key string) int {
	value, _ := optionIntValue(option, key)
	return value
}

func optionIntValue(option map[string]any, key string) (int, bool) {
	if option == nil {
		return 0, false
	}
	switch value := option[key].(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case int32:
		return int(value), true
	case float64:
		return int(value), true
	case float32:
		return int(value), true
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return int(parsed), true
		}
		if parsed, err := value.Float64(); err == nil {
			return int(parsed), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func optionFloat(option map[string]any, key string) (float64, bool) {
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
	case "max", "min", "average", "specific":
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

func (c *Controller) resolveCharacterName(region renderregion.Value, characterID int) string {
	if c == nil || characterID <= 0 {
		return ""
	}
	_, cardSource, err := c.resolveCardSource(region)
	if err != nil {
		return ""
	}
	resolver, ok := cardSource.(interface {
		GetCharacterByID(id int) (*masterdata.Character, error)
	})
	if !ok {
		return ""
	}
	character, err := resolver.GetCharacterByID(characterID)
	if err != nil || character == nil {
		return ""
	}
	return strings.TrimSpace(character.FirstName + character.GivenName)
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

func defaultDeckConfig12() map[string]any {
	return map[string]any{
		"disable":      false,
		"level_max":    true,
		"episode_read": true,
		"master_max":   true,
		"skill_max":    true,
		"canvas":       false,
	}
}

func defaultDeckConfig34bd() map[string]any {
	return map[string]any{
		"disable":      false,
		"level_max":    true,
		"episode_read": false,
		"master_max":   false,
		"skill_max":    false,
		"canvas":       false,
	}
}

func noChangeDeckConfig() map[string]any {
	return map[string]any{
		"disable":      false,
		"level_max":    false,
		"episode_read": false,
		"master_max":   false,
		"skill_max":    false,
		"canvas":       false,
	}
}

func pickBonusTargets(list []int, args string) []int {
	if len(list) > 0 {
		return list
	}
	parts := strings.Fields(strings.TrimSpace(args))
	var values []int
	for _, part := range parts {
		value, err := parseBonusTarget(strings.TrimSpace(part))
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

func parseBonusTarget(raw string) (int, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimSuffix(cleaned, "%")
	cleaned = strings.TrimSuffix(cleaned, "％")
	cleaned = strings.TrimSpace(strings.ReplaceAll(cleaned, "加成", ""))
	return strconv.Atoi(strings.TrimSpace(cleaned))
}

// Conversion helpers

func toInterfaceSlice(values []string) []any {
	if len(values) == 0 {
		return nil
	}
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func toInterfaceMap(values map[string]float64) map[string]any {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]any, len(values))
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
