package education

import (
	"errors"
	"fmt"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/usererror"
	"path/filepath"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

func hasAreaItemFilter(query AreaItemQuery) bool {
	return normalizeUnit(query.Unit) != "" ||
		normalizeAttr(query.Attr) != "" ||
		query.Cid > 0 ||
		query.Tree ||
		query.Flower ||
		query.AllCharacter
}

func areaItemMatchesFilter(
	item *AreaItem,
	levels []*AreaItemLevel,
	filterUnit string,
	filterAttr string,
	filterCID int,
	filterTree bool,
	filterFlower bool,
	filterPiapro bool,
	filterAllCharacter bool,
) bool {
	if item == nil {
		return false
	}

	matched, isVSItem := areaItemLevelsMatchFilter(levels, filterAttr, filterCID, filterPiapro)
	if filterTree && item.AreaID == areaTreeAreaID {
		matched = true
	}
	if filterFlower && item.AreaID == areaFlowerAreaID {
		matched = true
	}
	if filterAllCharacter && areaItemHasUntargetedLevels(levels) {
		matched = true
	}
	if filterUnit != "" {
		areaID, ok := areaFilterUnitAreaIDs[filterUnit]
		if ok && item.AreaID == areaID && !isVSItem {
			matched = true
		}
	}
	return matched
}

func areaItemLevelsMatchFilter(levels []*AreaItemLevel, filterAttr string, filterCID int, filterPiapro bool) (bool, bool) {
	matched := false
	isVSItem := false
	for _, level := range levels {
		if level == nil {
			continue
		}
		levelMatched, levelIsVS := areaItemLevelMatchesFilter(level, filterAttr, filterCID, filterPiapro)
		matched = matched || levelMatched
		isVSItem = isVSItem || levelIsVS
	}
	return matched, isVSItem
}

func areaItemLevelMatchesFilter(level *AreaItemLevel, filterAttr string, filterCID int, filterPiapro bool) (bool, bool) {
	isVSItem := normalizeUnit(level.TargetUnit) == "piapro"
	if _, ok := piaproCharacterIDs[level.TargetGameCharacterID]; ok {
		isVSItem = true
	}
	matched := isVSItem && filterPiapro
	if filterCID > 0 && level.TargetGameCharacterID == filterCID {
		matched = true
	}
	if filterAttr != "" && normalizeAttr(level.TargetCardAttr) == filterAttr {
		matched = true
	}
	return matched, isVSItem
}

func (c *Controller) areaItemTargetIcon(levels []*AreaItemLevel) string {
	for _, level := range levels {
		if level == nil || isMultiUnitAreaItemLevel(level) {
			continue
		}
		if level.TargetGameCharacterID > 0 {
			return c.characterIconPath(level.TargetGameCharacterID)
		}
		if unit := normalizeUnit(level.TargetUnit); unit != "" {
			return c.unitIconPath(unit)
		}
		if attr := normalizeAttr(level.TargetCardAttr); attr != "" {
			return c.attrIconPath(attr)
		}
	}
	return ""
}

// areaItemTargetLabel names the target of an item that has no target icon:
// an every-character item (JP 7.0.0 想いの大樹). Empty otherwise.
func areaItemTargetLabel(levels []*AreaItemLevel, targetIconPath string) string {
	if targetIconPath != "" {
		return ""
	}
	for _, level := range levels {
		if isAllTargetAreaItemLevel(level) && !isMultiUnitAreaItemLevel(level) {
			return i18n.T("education.area.all_characters")
		}
	}
	return ""
}

// areaItemHasUntargetedLevels reports whether any row of the item boosts
// every character or depends on the deck's unit mix rather than a single
// character, unit or attribute.
func areaItemHasUntargetedLevels(levels []*AreaItemLevel) bool {
	for _, level := range levels {
		if isMultiUnitAreaItemLevel(level) || isAllTargetAreaItemLevel(level) {
			return true
		}
	}
	return false
}

// multiUnitAreaItemLevelByLevel indexes the deck-conditional multi_unit rows
// by level; empty for items without them.
func multiUnitAreaItemLevelByLevel(levels []*AreaItemLevel) map[int]*AreaItemLevel {
	result := make(map[int]*AreaItemLevel)
	for _, level := range levels {
		if isMultiUnitAreaItemLevel(level) {
			if _, exists := result[level.Level]; !exists {
				result[level.Level] = level
			}
		}
	}
	return result
}

func (c *Controller) unitIconPath(unit string) string {
	icon := assets.UnitIconFilename(unit)
	if icon == "" {
		return ""
	}
	return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, icon+".png")
}

func (c *Controller) attrIconPath(attr string) string {
	attr = normalizeAttr(attr)
	if attr == "" {
		return ""
	}
	return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir,
		filepath.Join("card", fmt.Sprintf("attr_icon_%s.png", attr)))
}

func (c *Controller) materialIconPath(resourceType string, materialID int) string {
	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	if resourceType == "paid_jewel" {
		resourceType = "jewel"
	}
	region := "jp"
	switch resourceType {
	case "coin", "virtual_coin", "jewel":
		return assets.ResolveRegionAssetPath(c.assets, region,
			filepath.Join("thumbnail", "common_material", resourceType+".png"))
	case "material":
		if materialID <= 0 {
			return ""
		}
		return assets.ResolveRegionAssetPath(c.assets, region,
			filepath.Join("thumbnail", "material", fmt.Sprintf("material%d.png", materialID)))
	default:
		return ""
	}
}

func normalizeUnit(unit string) string {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "", "any":
		return ""
	case "light_sound_club":
		return "light_sound"
	case "more_more_jump":
		return "idol"
	case "vivid_bad_squad":
		return "street"
	case "wonderlands_x_showtime":
		return "theme_park"
	case "25_ji_night_cord_de":
		return "school_refusal"
	default:
		return strings.ToLower(strings.TrimSpace(unit))
	}
}

func normalizeAttr(attr string) string {
	attr = strings.ToLower(strings.TrimSpace(attr))
	if attr == "" || attr == "any" {
		return ""
	}
	return attr
}

// areaItemTargetUnitMultiUnit is the targetUnit of area item level rows whose
// bonus is conditional on the deck containing two or more units (JP 7.0.0
// areaItemId 56). It is not a real unit and never maps onto one.
const areaItemTargetUnitMultiUnit = "multi_unit"

// isMultiUnitAreaItemLevel reports whether the row is a deck-composition
// conditional bonus rather than a character/unit/attribute one.
func isMultiUnitAreaItemLevel(level *AreaItemLevel) bool {
	return level != nil && strings.EqualFold(strings.TrimSpace(level.TargetUnit), areaItemTargetUnitMultiUnit)
}

// isAllTargetAreaItemLevel reports whether the row targets every character
// (unit "any", attribute "any" and no game character).
func isAllTargetAreaItemLevel(level *AreaItemLevel) bool {
	return level != nil &&
		level.TargetGameCharacterID <= 0 &&
		normalizeUnit(level.TargetUnit) == "" &&
		normalizeAttr(level.TargetCardAttr) == ""
}

// unconditionalAreaItemLevelByLevel picks one row per level for displays
// that show a single bonus value, preferring the first row that is not a
// deck-conditional "multi_unit" row.
func unconditionalAreaItemLevelByLevel(levels []*AreaItemLevel) map[int]*AreaItemLevel {
	result := make(map[int]*AreaItemLevel, len(levels))
	for _, level := range levels {
		if level == nil {
			continue
		}
		current, exists := result[level.Level]
		if !exists || (isMultiUnitAreaItemLevel(current) && !isMultiUnitAreaItemLevel(level)) {
			result[level.Level] = level
		}
	}
	return result
}

func defaultBondColor() []int {
	return []int{100, 100, 100}
}

func parseBondColorCode(code string) []int {
	colorCode := strings.TrimSpace(strings.TrimPrefix(code, "#"))
	if len(colorCode) != 6 {
		return defaultBondColor()
	}

	result := make([]int, 3)
	for idx := 0; idx < 3; idx++ {
		value, err := strconv.ParseInt(colorCode[idx*2:idx*2+2], 16, 64)
		if err != nil {
			return defaultBondColor()
		}
		result[idx] = int(value)
	}
	return result
}

func leaderMissionRequirementForSeq(requirements []LeaderMissionRequirement, seq int) int {
	if seq <= 0 || len(requirements) == 0 {
		return 0
	}

	result := 0
	for _, item := range requirements {
		if item.Seq > seq {
			break
		}
		result = item.Requirement
	}
	return result
}

func collectUserAreaItemLevels(areas []snapshot.RawUserArea) map[int]int {
	levels := make(map[int]int)
	for _, area := range areas {
		for _, item := range area.AreaItems {
			if item.AreaItemID <= 0 {
				continue
			}
			if item.Level > levels[item.AreaItemID] {
				levels[item.AreaItemID] = item.Level
			}
		}
	}
	return levels
}

// errSuiteIncomplete is returned when the requester's suite data lacks a part
// a command needs; the user is asked to upload it again.
func errSuiteIncomplete(detail string) error {
	return usererror.Wrap(usererror.CodeSetup, i18n.M("education.suite_incomplete"), errors.New(detail))
}
