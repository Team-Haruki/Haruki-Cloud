package mysekai

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
)

func (c *Controller) obtainedMysekaiFixtureIDs(merged map[string]any, blueprints map[int]map[string]any) map[int]struct{} {
	if fixtures := nestedList(merged, "userMysekaiFixtures"); fixtures != nil {
		return userMysekaiFixtureIDs(fixtures)
	}
	return userMysekaiBlueprintFixtureIDs(merged, blueprints)
}

func userMysekaiFixtureIDs(fixtures []any) map[int]struct{} {
	result := map[int]struct{}{}
	for _, raw := range fixtures {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if fixtureID := userMysekaiFixtureID(item); fixtureID != 0 {
			result[fixtureID] = struct{}{}
		}
	}
	return result
}

func userMysekaiBlueprintFixtureIDs(merged map[string]any, blueprints map[int]map[string]any) map[int]struct{} {
	result := map[int]struct{}{}
	for _, raw := range nestedList(merged, "userMysekaiBlueprints") {
		item, ok := raw.(map[string]any)
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

func userMysekaiFixtureID(item map[string]any) int {
	for _, key := range []string{
		"mysekaiFixtureId",
		"mysekaiFixtureID",
		"mysekai_fixture_id",
		"fixtureId",
		"fixtureID",
		"id",
	} {
		if fixtureID := intNumber(item[key], 0); fixtureID != 0 {
			return fixtureID
		}
	}
	if fixture, ok := item["mysekaiFixture"].(map[string]any); ok {
		return intNumber(fixture["id"], 0)
	}
	return 0
}

func (c *Controller) craftableMysekaiFixtureIDs(blueprints map[int]map[string]any) map[int]struct{} {
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

func (c *Controller) extractVisitCharacters(region renderregion.Value, merged map[string]any) []drawing.MysekaiVisitCharacter {
	visit, ok := merged["userMysekaiGateCharacterVisit"].(map[string]any)
	if !ok {
		return []drawing.MysekaiVisitCharacter{}
	}

	groupMap := c.masterdata.loadMapByID("mysekaiGameCharacterUnitGroups.json")
	characters, ok := visit["userMysekaiGateCharacters"].([]any)
	if !ok {
		return []drawing.MysekaiVisitCharacter{}
	}

	result := make([]drawing.MysekaiVisitCharacter, 0, len(characters))
	seen := map[int]struct{}{}
	for _, item := range characters {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		character, ok := c.buildVisitCharacter(region, entry, groupMap, seen)
		if !ok {
			continue
		}
		result = append(result, character)
		if len(result) >= 6 {
			break
		}
	}
	return result
}

func (c *Controller) buildVisitCharacter(region renderregion.Value, entry map[string]any, groupMap map[int]map[string]any, seen map[int]struct{}) (drawing.MysekaiVisitCharacter, bool) {
	groupID := intNumberFrom(entry, 0, "mysekaiGameCharacterUnitGroupId", "mysekai_game_character_unit_group_id")
	group := groupMap[groupID]
	if len(group) == 0 || intNumberFrom(group, 0, "gameCharacterUnitId2", "game_character_unit_id2", "game_character_unit_id_2") != 0 {
		return drawing.MysekaiVisitCharacter{}, false
	}
	displayUnitID := intNumberFrom(group, 0, "gameCharacterUnitId1", "game_character_unit_id1", "game_character_unit_id_1")
	if displayUnitID == 0 {
		return drawing.MysekaiVisitCharacter{}, false
	}
	if _, duplicated := seen[displayUnitID]; duplicated {
		return drawing.MysekaiVisitCharacter{}, false
	}
	seen[displayUnitID] = struct{}{}
	reservation := boolValue(entry["isReservation"])
	return drawing.MysekaiVisitCharacter{
		SdImagePath:         c.regionPath(region, fmt.Sprintf("character/character_sd_l/chr_sp_%d.png", displayUnitID)),
		MemoriaImagePath:    c.visitCharacterMemoriaPath(region, displayUnitID),
		IsRead:              false,
		IsReservation:       reservation,
		ReservationIconPath: c.visitCharacterReservationIconPath(reservation),
	}, true
}

func (c *Controller) visitCharacterMemoriaPath(region renderregion.Value, displayUnitID int) *string {
	gameCharacterID := c.gameCharacterIDByUnitID(displayUnitID)
	if gameCharacterID <= 0 {
		return nil
	}
	path := c.regionPath(region, fmt.Sprintf("mysekai/item_preview/material/item_memoria_%d.png", gameCharacterID))
	return &path
}

func (c *Controller) visitCharacterReservationIconPath(reservation bool) *string {
	if !reservation {
		return nil
	}
	path := c.staticPath("mysekai/invitationcard.png")
	return &path
}

func (c *Controller) extractSiteResourceNumbers(region renderregion.Value, merged map[string]any) []drawing.MysekaiSiteResourceNumber {
	updated := nestedList(merged, "userMysekaiHarvestMaps")
	if len(updated) == 0 {
		return []drawing.MysekaiSiteResourceNumber{}
	}

	counts := collectMysekaiSiteResourceCounts(updated)

	materialMap := c.loadIconNameMap("mysekaiMaterials.json", "iconAssetbundleName")
	materialRarityMap := c.loadFieldMap("mysekaiMaterials.json", "mysekaiMaterialRarityType")
	itemMap := c.loadIconNameMap("mysekaiItems.json", "iconAssetbundleName")
	fixtureMap := c.loadIconNameMap("mysekaiFixtures.json", "assetbundleName")
	musicRecordMap := c.loadMusicRecordJacketMap()

	order := []int{5, 7, 6, 8}
	result := make([]drawing.MysekaiSiteResourceNumber, 0, len(order))
	for _, siteID := range order {
		resMap := counts[siteID]
		resources := c.buildMysekaiSiteResources(region, resMap, materialMap, materialRarityMap, itemMap, fixtureMap, musicRecordMap, merged)
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

func collectMysekaiSiteResourceCounts(updated []any) map[int]map[string]int {
	counts := map[int]map[string]int{5: {}, 7: {}, 6: {}, 8: {}}
	for _, item := range updated {
		siteMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		siteID := intNumber(siteMap["mysekaiSiteId"], 0)
		if counts[siteID] == nil {
			counts[siteID] = map[string]int{}
		}
		for _, rawDrop := range mysekaiSiteResourceDrops(siteMap) {
			key, quantity, ok := parseMysekaiSiteResourceDrop(rawDrop)
			if ok {
				counts[siteID][key] += quantity
			}
		}
	}
	return counts
}

func mysekaiSiteResourceDrops(siteMap map[string]any) []any {
	drops, _ := siteMap["userMysekaiSiteHarvestResourceDrops"].([]any)
	return drops
}

func parseMysekaiSiteResourceDrop(rawDrop any) (string, int, bool) {
	drop, ok := rawDrop.(map[string]any)
	if !ok || mysekaiResourceDropStatus(drop) != "before_drop" {
		return "", 0, false
	}
	resourceType := mysekaiNormalizeResourceType(firstMysekaiResourceDropString(drop, "resourceType", "type"))
	resourceID := intNumber(drop["resourceId"], intNumber(drop["id"], 0))
	if resourceType == "" || resourceID == 0 {
		return "", 0, false
	}
	quantity := intNumber(drop["quantity"], 1)
	if quantity <= 0 {
		quantity = 1
	}
	return fmt.Sprintf("%s_%d", resourceType, resourceID), quantity, true
}

func mysekaiResourceDropStatus(drop map[string]any) string {
	return firstMysekaiResourceDropString(drop, "mysekaiSiteHarvestResourceDropStatus", "status")
}

func firstMysekaiResourceDropString(drop map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(drop[key]); value != "" {
			return value
		}
	}
	return ""
}

func (c *Controller) buildMysekaiSiteResources(region renderregion.Value, resources map[string]int, materialMap, materialRarityMap, itemMap, fixtureMap, musicRecordMap map[int]string, merged map[string]any) []drawing.MysekaiResourceNumber {
	keys := sortKeysByResource(resources, materialRarityMap)
	result := make([]drawing.MysekaiResourceNumber, 0, len(keys))
	for _, key := range keys {
		imagePath, hasRecord := c.resourceImagePath(region, key, materialMap, itemMap, fixtureMap, musicRecordMap, merged)
		if imagePath == "" {
			continue
		}
		result = append(result, drawing.MysekaiResourceNumber{
			ImagePath:           imagePath,
			Number:              resources[key],
			TextColor:           resourceTextColor(key, materialRarityMap),
			HasMusicRecord:      hasRecord,
			MusicRecordIconPath: musicRecordIconPath(func(path string) string { return c.staticPath(path) }, hasRecord),
		})
	}
	return result
}

func (c *Controller) resourceImagePath(region renderregion.Value, key string, materialMap, itemMap, fixtureMap, musicRecordMap map[int]string, merged map[string]any) (string, bool) {
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
		return assets.ResolveRegionAssetPath(
			c.assets,
			region.String(),
			fmt.Sprintf("thumbnail/material/material%d.png", id),
			fmt.Sprintf("thumbnail/material_rip/material%d.png", id),
		), false
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

func (c *Controller) hasMysekaiMusicRecord(merged map[string]any, recordID int) bool {
	for _, item := range nestedList(merged, "userMysekaiMusicRecords") {
		entry, ok := item.(map[string]any)
		if ok && intNumber(entry["mysekaiMusicRecordId"], 0) == recordID {
			return true
		}
	}
	return false
}

func (c *Controller) loadIconNameMap(filename, field string) map[int]string {
	if c == nil || c.masterdata == nil {
		return map[int]string{}
	}
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
	if c == nil || c.masterdata == nil {
		return map[int]string{}
	}
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

func mysekaiBirthdayCharacterImageName(item map[string]any) string {
	if len(item) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(stringValue(item["givenNameEnglish"])))
}

// mysekaiBirthdayIconCandidateYearOffsets is the FROZEN year window (addendum
// A4): the current year, the two most recent past years, then the nearest
// future year. Drawing pins the identical order in its parity replica.
var mysekaiBirthdayIconCandidateYearOffsets = [...]int{0, -1, -2, 1}

// mysekaiBirthdayIconCandidates returns the bounded year window Drawing must
// probe, in the preference order the deleted directory scan used. It does no
// I/O: Drawing takes the first candidate that exists (C1).
func mysekaiBirthdayIconCandidates(region, imageName string, now time.Time) []string {
	imageName = strings.TrimSpace(imageName)
	if imageName == "" {
		return nil
	}
	year := now.Year()
	candidates := make([]string, 0, len(mysekaiBirthdayIconCandidateYearOffsets))
	for _, offset := range mysekaiBirthdayIconCandidateYearOffsets {
		rel := path.Join("mysekai", "birthday", imageName+"_"+strconv.Itoa(year+offset), refreshIconFileName)
		if candidate := assets.ResolveRegionAssetPath(nil, region, rel); candidate != "" {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
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
