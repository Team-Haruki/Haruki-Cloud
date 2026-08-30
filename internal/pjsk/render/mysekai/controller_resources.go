package mysekai

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
)

const mysekaiBirthdayRefreshIconCacheTTL = 30 * time.Second

type mysekaiBirthdayRefreshIconCacheEntry struct {
	path      string
	expiresAt time.Time
}

var mysekaiBirthdayRefreshIconCache sync.Map

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

func (c *Controller) resolveMysekaiBirthdayRefreshIconPath(region renderregion.Value, item map[string]any, now time.Time) string {
	if c == nil || c.assets == nil {
		return ""
	}
	imageName := mysekaiBirthdayCharacterImageName(item)
	if imageName == "" {
		return ""
	}

	currentYear := now.Year()
	cacheKey := mysekaiBirthdayIconCacheKey(c.assets.Roots(), region, imageName, currentYear)
	if cachedPath := loadMysekaiBirthdayRefreshIcon(cacheKey); cachedPath != "" {
		return cachedPath
	}
	selection := mysekaiBirthdayIconSelection{}
	for _, root := range c.assets.Roots() {
		candidate, exact := c.scanMysekaiBirthdayIconRoot(root, region, imageName, currentYear)
		if exact != "" {
			storeMysekaiBirthdayRefreshIcon(cacheKey, exact)
			return exact
		}
		selection = preferredMysekaiBirthdayIcon(selection, candidate)
	}
	if selection.name == "" {
		return ""
	}
	resolved := c.resolveMysekaiBirthdayIcon(region, selection.name)
	storeMysekaiBirthdayRefreshIcon(cacheKey, resolved)
	return resolved
}

type mysekaiBirthdayIconSelection struct {
	name     string
	year     int
	isFuture bool
}

func mysekaiBirthdayIconCacheKey(roots []string, region renderregion.Value, imageName string, year int) string {
	return strings.Join(roots, "\x00") + "|" + region.String() + "|" + imageName + "|" + strconv.Itoa(year)
}

func loadMysekaiBirthdayRefreshIcon(cacheKey string) string {
	cached, ok := mysekaiBirthdayRefreshIconCache.Load(cacheKey)
	if !ok {
		return ""
	}
	entry, valid := cached.(mysekaiBirthdayRefreshIconCacheEntry)
	if valid && entry.path != "" && time.Now().Before(entry.expiresAt) {
		return entry.path
	}
	mysekaiBirthdayRefreshIconCache.Delete(cacheKey)
	return ""
}

func (c *Controller) scanMysekaiBirthdayIconRoot(root string, region renderregion.Value, imageName string, currentYear int) (mysekaiBirthdayIconSelection, string) {
	baseDir, ok := localMysekaiBirthdayAssetDir(root, region)
	if !ok {
		return mysekaiBirthdayIconSelection{}, ""
	}
	finishReadDir := commandtrace.MeasureOperation(c.requestCtx, "asset.readdir")
	entries, err := os.ReadDir(baseDir)
	finishReadDir()
	if err != nil {
		return mysekaiBirthdayIconSelection{}, ""
	}
	prefix := imageName + "_"
	selection := mysekaiBirthdayIconSelection{}
	for _, entry := range entries {
		candidate, ok := c.readMysekaiBirthdayIconCandidate(baseDir, prefix, entry.Name(), entry.IsDir(), currentYear)
		if !ok {
			continue
		}
		if candidate.year == currentYear {
			return candidate, c.resolveMysekaiBirthdayIcon(region, candidate.name)
		}
		selection = preferredMysekaiBirthdayIcon(selection, candidate)
	}
	return selection, ""
}

func localMysekaiBirthdayAssetDir(root string, region renderregion.Value) (string, bool) {
	root = strings.TrimSpace(root)
	if root == "" || strings.HasPrefix(root, "http://") || strings.HasPrefix(root, "https://") {
		return "", false
	}
	return filepath.Join(root, strings.ToLower(strings.TrimSpace(region.String()))+"-assets", assets.RegionAssetOnDemand, "mysekai", "birthday"), true
}

func (c *Controller) readMysekaiBirthdayIconCandidate(baseDir, prefix, entryName string, isDirectory bool, currentYear int) (mysekaiBirthdayIconSelection, bool) {
	if !isDirectory || !strings.HasPrefix(entryName, prefix) {
		return mysekaiBirthdayIconSelection{}, false
	}
	iconPath := filepath.Join(baseDir, entryName, refreshIconFileName)
	finishStat := commandtrace.MeasureOperation(c.requestCtx, "asset.stat")
	_, statErr := os.Stat(iconPath)
	finishStat()
	if statErr != nil {
		return mysekaiBirthdayIconSelection{}, false
	}
	year, err := strconv.Atoi(strings.TrimPrefix(entryName, prefix))
	if err != nil {
		return mysekaiBirthdayIconSelection{}, false
	}
	return mysekaiBirthdayIconSelection{name: entryName, year: year, isFuture: year > currentYear}, true
}

func preferredMysekaiBirthdayIcon(current, candidate mysekaiBirthdayIconSelection) mysekaiBirthdayIconSelection {
	if candidate.name == "" {
		return current
	}
	if current.name == "" || (!candidate.isFuture && current.isFuture) {
		return candidate
	}
	if candidate.isFuture != current.isFuture {
		return current
	}
	if (!candidate.isFuture && candidate.year > current.year) || (candidate.isFuture && candidate.year < current.year) {
		return candidate
	}
	return current
}

func (c *Controller) resolveMysekaiBirthdayIcon(region renderregion.Value, directory string) string {
	return assets.ResolveRegionAssetPath(c.assets, region.String(), filepath.ToSlash(filepath.Join("mysekai", "birthday", directory, refreshIconFileName)))
}

func storeMysekaiBirthdayRefreshIcon(cacheKey, resolved string) {
	if resolved == "" {
		return
	}
	mysekaiBirthdayRefreshIconCache.Store(cacheKey, mysekaiBirthdayRefreshIconCacheEntry{
		path:      resolved,
		expiresAt: time.Now().Add(mysekaiBirthdayRefreshIconCacheTTL),
	})
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
