package mysekai

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

func (c *Controller) obtainedMysekaiFixtureIDs(merged map[string]interface{}, blueprints map[int]map[string]interface{}) map[int]struct{} {
	result := map[int]struct{}{}
	for _, raw := range nestedList(merged, "userMysekaiBlueprints") {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		blueprintID := intNumber(item["mysekaiBlueprintId"], 0)
		blueprint := blueprints[blueprintID]
		if len(blueprint) == 0 || stringValue(blueprint["mysekaiCraftType"]) != "mysekai_fixture" {
			continue
		}
		targetID := intNumber(blueprint["craftTargetId"], 0)
		if targetID != 0 {
			result[targetID] = struct{}{}
		}
	}
	return result
}

func (c *Controller) craftableMysekaiFixtureIDs(blueprints map[int]map[string]interface{}) map[int]struct{} {
	result := map[int]struct{}{}
	for _, blueprint := range blueprints {
		if stringValue(blueprint["mysekaiCraftType"]) != "mysekai_fixture" {
			continue
		}
		targetID := intNumber(blueprint["craftTargetId"], 0)
		if targetID != 0 {
			result[targetID] = struct{}{}
		}
	}
	return result
}

func (c *Controller) extractVisitCharacters(region renderregion.Value, merged map[string]interface{}) []drawing.MysekaiVisitCharacter {
	visit, ok := merged["userMysekaiGateCharacterVisit"].(map[string]interface{})
	if !ok {
		return []drawing.MysekaiVisitCharacter{}
	}

	groupMap := c.masterdata.loadMapByID("mysekaiGameCharacterUnitGroups.json")
	characters, ok := visit["userMysekaiGateCharacters"].([]interface{})
	if !ok {
		return []drawing.MysekaiVisitCharacter{}
	}

	result := make([]drawing.MysekaiVisitCharacter, 0, len(characters))
	seen := map[int]struct{}{}
	for _, item := range characters {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		groupID := intNumber(entry["mysekaiGameCharacterUnitGroupId"], 0)
		group := groupMap[groupID]
		if len(group) == 0 || intNumber(group["gameCharacterUnitId2"], 0) != 0 {
			continue
		}
		displayUnitID := intNumber(group["gameCharacterUnitId1"], 0)
		if displayUnitID == 0 {
			continue
		}
		if _, ok := seen[displayUnitID]; ok {
			continue
		}
		seen[displayUnitID] = struct{}{}

		var memoriaPath *string
		if gameCharacterID := c.gameCharacterIDByUnitID(displayUnitID); gameCharacterID > 0 {
			path := c.regionPath(region, fmt.Sprintf("mysekai/item_preview/material/item_memoria_%d.png", gameCharacterID))
			memoriaPath = &path
		}
		var reservationIconPath *string
		if boolValue(entry["isReservation"]) {
			path := c.staticPath("mysekai/invitationcard.png")
			reservationIconPath = &path
		}

		result = append(result, drawing.MysekaiVisitCharacter{
			SdImagePath:         c.regionPath(region, fmt.Sprintf("character/character_sd_l/chr_sp_%d.png", displayUnitID)),
			MemoriaImagePath:    memoriaPath,
			IsRead:              false,
			IsReservation:       boolValue(entry["isReservation"]),
			ReservationIconPath: reservationIconPath,
		})
		if len(result) >= 6 {
			break
		}
	}
	return result
}

func (c *Controller) extractSiteResourceNumbers(region renderregion.Value, merged map[string]interface{}) []drawing.MysekaiSiteResourceNumber {
	updated := nestedList(merged, "userMysekaiHarvestMaps")
	if len(updated) == 0 {
		return []drawing.MysekaiSiteResourceNumber{}
	}

	counts := map[int]map[string]int{5: {}, 7: {}, 6: {}, 8: {}}
	for _, item := range updated {
		siteMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		siteID := intNumber(siteMap["mysekaiSiteId"], 0)
		if _, ok := counts[siteID]; !ok {
			counts[siteID] = map[string]int{}
		}

		drops, _ := siteMap["userMysekaiSiteHarvestResourceDrops"].([]interface{})
		for _, rawDrop := range drops {
			drop, ok := rawDrop.(map[string]interface{})
			if !ok {
				continue
			}
			status := stringValue(drop["mysekaiSiteHarvestResourceDropStatus"])
			if status == "" {
				status = stringValue(drop["status"])
			}
			if status != "before_drop" {
				continue
			}
			resourceType := stringValue(drop["resourceType"])
			if resourceType == "" {
				resourceType = stringValue(drop["type"])
			}
			resourceType = mysekaiNormalizeResourceType(resourceType)
			resourceID := intNumber(drop["resourceId"], intNumber(drop["id"], 0))
			if resourceType == "" || resourceID == 0 {
				continue
			}
			quantity := intNumber(drop["quantity"], 1)
			if quantity <= 0 {
				quantity = 1
			}
			key := fmt.Sprintf("%s_%d", resourceType, resourceID)
			counts[siteID][key] += quantity
		}
	}

	materialMap := c.loadIconNameMap("mysekaiMaterials.json", "iconAssetbundleName")
	materialRarityMap := c.loadFieldMap("mysekaiMaterials.json", "mysekaiMaterialRarityType")
	itemMap := c.loadIconNameMap("mysekaiItems.json", "iconAssetbundleName")
	fixtureMap := c.loadIconNameMap("mysekaiFixtures.json", "assetbundleName")
	musicRecordMap := c.loadMusicRecordJacketMap()

	order := []int{5, 7, 6, 8}
	result := make([]drawing.MysekaiSiteResourceNumber, 0, len(order))
	for _, siteID := range order {
		resMap := counts[siteID]
		keys := sortKeysByResource(resMap, materialRarityMap)
		resources := make([]drawing.MysekaiResourceNumber, 0, len(keys))
		for _, key := range keys {
			imagePath, hasRecord := c.resourceImagePath(region, key, materialMap, itemMap, fixtureMap, musicRecordMap, merged)
			if imagePath == "" {
				continue
			}
			resources = append(resources, drawing.MysekaiResourceNumber{
				ImagePath:           imagePath,
				Number:              resMap[key],
				TextColor:           resourceTextColor(key, materialRarityMap),
				HasMusicRecord:      hasRecord,
				MusicRecordIconPath: musicRecordIconPath(func(p string) string { return c.staticPath(p) }, hasRecord),
			})
		}
		if len(resources) == 0 {
			continue
		}
		result = append(result, drawing.MysekaiSiteResourceNumber{
			ImagePath:       c.regionPath(region, fmt.Sprintf("mysekai/site/sitemap/texture/img_harvest_site_%d.png", siteID)),
			ResourceNumbers: resources,
		})
	}
	return result
}

func (c *Controller) resourceImagePath(region renderregion.Value, key string, materialMap, itemMap, fixtureMap, musicRecordMap map[int]string, merged map[string]interface{}) (string, bool) {
	parts := strings.Split(key, "_")
	if len(parts) < 2 {
		return "", false
	}
	id := intNumber(parts[len(parts)-1], 0)
	typeKey := strings.TrimSuffix(key, fmt.Sprintf("_%d", id))
	switch typeKey {
	case "mysekai_material":
		if icon := materialMap[id]; icon != "" {
			return c.regionPath(region, fmt.Sprintf("mysekai/thumbnail/material/%s.png", icon)), false
		}
	case "material":
		return c.regionPath(region, fmt.Sprintf("thumbnail/material/material%d.png", id)), false
	case "mysekai_item":
		if icon := itemMap[id]; icon != "" {
			return c.regionPath(region, fmt.Sprintf("mysekai/thumbnail/item/%s.png", icon)), false
		}
	case "mysekai_fixture":
		if assetbundleName := fixtureMap[id]; assetbundleName != "" {
			// Some plant seeds/saplings share the same base assetbundleName and
			// require an id-suffixed thumbnail to distinguish icon variants.
			// Prefer "<name>_<id>_1.png", then fall back to "<name>_1.png".
			return assets.ResolveRegionAssetPath(
				c.assets,
				region.String(),
				fmt.Sprintf("mysekai/thumbnail/fixture/%s_%d_1.png", assetbundleName, id),
				fmt.Sprintf("mysekai/thumbnail/fixture/%s_1.png", assetbundleName),
			), false
		}
	case "mysekai_music_record":
		if jacket := musicRecordMap[id]; jacket != "" {
			return c.regionPath(region, fmt.Sprintf("music/jacket/%s/%s.png", jacket, jacket)), c.hasMysekaiMusicRecord(merged, id)
		}
	}
	return "", false
}

func (c *Controller) hasMysekaiMusicRecord(merged map[string]interface{}, recordID int) bool {
	for _, item := range nestedList(merged, "userMysekaiMusicRecords") {
		entry, ok := item.(map[string]interface{})
		if ok && intNumber(entry["mysekaiMusicRecordId"], 0) == recordID {
			return true
		}
	}
	return false
}

func (c *Controller) loadIconNameMap(filename, field string) map[int]string {
	items := c.masterdata.loadMapByID(filename)
	result := make(map[int]string, len(items))
	for id, item := range items {
		if value := stringValue(item[field]); value != "" {
			result[id] = value
		}
	}
	return result
}

func (c *Controller) loadFieldMap(filename, field string) map[int]string {
	items := c.masterdata.loadMapByID(filename)
	result := make(map[int]string, len(items))
	for id, item := range items {
		if value := stringValue(item[field]); value != "" {
			result[id] = value
		}
	}
	return result
}

func mysekaiHarvestPosKey(x, z float64) string {
	return fmt.Sprintf("%.3f_%.3f", x, z)
}

func mysekaiNormalizeResourceType(resourceType string) string {
	switch strings.ToLower(strings.TrimSpace(resourceType)) {
	case "mysekai_material":
		return "mysekai_material"
	case "material":
		return "material"
	case "item", "mysekai_item":
		return "mysekai_item"
	case "fixture", "mysekai_fixture":
		return "mysekai_fixture"
	case "music_record", "mysekai_music_record":
		return "mysekai_music_record"
	default:
		return strings.TrimSpace(resourceType)
	}
}

func mysekaiBirthdayCharacterImageName(item map[string]interface{}) string {
	if len(item) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(stringValue(item["givenNameEnglish"])))
}

func mysekaiIsBirthdayDrop(resourceType string, resourceID int) bool {
	return (resourceType == "material" || resourceType == "mysekai_material") && resourceID >= 174 && resourceID <= 199
}

func (c *Controller) loadMusicRecordJacketMap() map[int]string {
	records := c.masterdata.loadMapByID("mysekaiMusicRecords.json")
	musics := c.masterdata.loadMapByID("musics.json")
	result := make(map[int]string, len(records))
	for id, record := range records {
		externalID := intNumber(record["externalId"], 0)
		if externalID == 0 {
			continue
		}
		if music := musics[externalID]; len(music) > 0 {
			if assetbundleName := stringValue(music["assetbundleName"]); assetbundleName != "" {
				result[id] = assetbundleName
			}
		}
	}
	return result
}

func (c *Controller) gameCharacterIDByUnitID(unitID int) int {
	if item := c.masterdata.loadMapByID("gameCharacterUnits.json")[unitID]; len(item) > 0 {
		return intNumber(item["gameCharacterId"], 0)
	}
	return 0
}
