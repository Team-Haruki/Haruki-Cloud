package education

import (
	"fmt"
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
		query.Flower
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
) bool {
	if item == nil {
		return false
	}

	matched := false
	isVSItem := false

	for _, level := range levels {
		if level == nil {
			continue
		}

		if normalized := normalizeUnit(level.TargetUnit); normalized == "piapro" {
			isVSItem = true
			if filterPiapro {
				matched = true
			}
		}

		if level.TargetGameCharacterID > 0 {
			if _, ok := piaproCharacterIDs[level.TargetGameCharacterID]; ok {
				isVSItem = true
				if filterPiapro {
					matched = true
				}
			}
			if filterCID > 0 && level.TargetGameCharacterID == filterCID {
				matched = true
			}
		}

		if filterAttr != "" && normalizeAttr(level.TargetCardAttr) == filterAttr {
			matched = true
		}
	}

	if filterTree && item.AreaID == areaTreeAreaID {
		matched = true
	}
	if filterFlower && item.AreaID == areaFlowerAreaID {
		matched = true
	}
	if filterUnit != "" {
		if areaID, ok := areaFilterUnitAreaIDs[filterUnit]; ok && item.AreaID == areaID && !isVSItem {
			matched = true
		}
	}

	return matched
}

func (c *Controller) areaItemTargetIcon(levels []*AreaItemLevel) string {
	for _, level := range levels {
		if level == nil {
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
