package mysekai

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
)

type Controller struct {
	drawing        *drawing.HarukiDrawingClient
	snapshot       userdata.Snapshot
	rawMySekaiJSON []byte // direct mysekai JSON (bypasses snapshot merge)
	masterdata     masterdataSource
	defaultRegion  renderregion.Value
	nicknames      map[string]int
	assets         *assets.AssetHelper
}

type mysekaiMapSiteConfig struct {
	SiteImageName string
	GridSize      float64
	OffsetX       float64
	OffsetZ       float64
	DirX          float64
	DirZ          float64
	RevXZ         bool
	CropBBox      []int
}

var (
	mysekaiMapSiteOrder   = []int{5, 6, 7, 8}
	mysekaiMapSiteConfigs = map[int]mysekaiMapSiteConfig{
		5: {
			SiteImageName: "grassland",
			GridSize:      33.333,
			OffsetX:       0,
			OffsetZ:       -60,
			DirX:          -1,
			DirZ:          -1,
			RevXZ:         true,
			CropBBox:      []int{300, 0, 1280, 1080},
		},
		6: {
			SiteImageName: "beach",
			GridSize:      20.513,
			OffsetX:       0,
			OffsetZ:       80,
			DirX:          1,
			DirZ:          -1,
			RevXZ:         false,
			CropBBox:      []int{300, 0, 1280, 1080},
		},
		7: {
			SiteImageName: "flowergarden",
			GridSize:      24.806,
			OffsetX:       -62.015,
			OffsetZ:       20.672,
			DirX:          -1,
			DirZ:          -1,
			RevXZ:         true,
			CropBBox:      []int{350, 0, 1280, 1080},
		},
		8: {
			SiteImageName: "memorialplace",
			GridSize:      21.333,
			OffsetX:       0,
			OffsetZ:       -130,
			DirX:          1,
			DirZ:          -1,
			RevXZ:         false,
			CropBBox:      []int{200, 0, 1280, 1080},
		},
	}
)

// NewController creates a mysekai Controller. If sekaiDSN is non-empty the
// controller queries the sekai database for masterdata; otherwise it falls
// back to reading JSON files from masterdataDir.
func NewController(drawingClient *drawing.HarukiDrawingClient, snapshot userdata.Snapshot, masterdataDir string, defaultRegion renderregion.Value, assetHelper *assets.AssetHelper, sekaiDSN ...string) *Controller {
	region := renderregion.WithDefault(defaultRegion)
	var md masterdataSource
	if len(sekaiDSN) > 0 && strings.TrimSpace(sekaiDSN[0]) != "" {
		md = newDBMasterdataStore(sekaiDSN[0], region.String())
	}
	if md == nil || !md.Configured() {
		md = newLocalMasterdataStore(masterdataDir)
	}
	return &Controller{
		drawing:       drawingClient,
		snapshot:      snapshot,
		masterdata:    md,
		defaultRegion: region,
		nicknames:     cloneNicknames(defaultNicknames),
		assets:        assetHelper,
	}
}

// regionPath resolves a region-specific asset path through the AssetHelper.
// For paths starting with "mysekai/", "event/", "gacha/" the ondemand mode is
// tried first; for others startapp is tried first.
func (c *Controller) regionPath(region renderregion.Value, relPath string) string {
	return assets.ResolveRegionAssetPath(c.assets, region.String(), relPath)
}

// staticPath resolves a path under the Drawing API's static_images directory.
func (c *Controller) staticPath(relPath string) string {
	relPath = strings.TrimSpace(strings.TrimPrefix(relPath, "/"))
	if relPath == "" {
		return ""
	}

	resolved := filepath.ToSlash(strings.TrimSpace(assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, relPath)))
	if resolved == "" {
		return filepath.ToSlash(filepath.Join(assets.StaticImagesDir, relPath))
	}
	if strings.HasPrefix(resolved, "http://") || strings.HasPrefix(resolved, "https://") {
		return resolved
	}
	if strings.HasPrefix(resolved, assets.StaticImagesDir+"/") {
		return resolved
	}

	if c.assets != nil {
		for _, root := range c.assets.Roots() {
			root = strings.TrimSpace(root)
			if root == "" {
				continue
			}
			if strings.HasPrefix(root, "http://") || strings.HasPrefix(root, "https://") {
				continue
			}

			rel := filepath.ToSlash(strings.TrimPrefix(assets.MakeRelative(root, resolved), "./"))
			if strings.HasPrefix(rel, assets.StaticImagesDir+"/") {
				return rel
			}

			// Local roots may point directly at "static_images". Normalize those
			// back to "static_images/..." payload paths.
			base := filepath.Base(filepath.ToSlash(root))
			if rel != resolved && rel != "" && base == assets.StaticImagesDir {
				return filepath.ToSlash(filepath.Join(assets.StaticImagesDir, strings.TrimPrefix(rel, "/")))
			}
		}
	}

	// Never return local absolute filesystem paths to Drawing API. Cloud and
	// Drawing may run in different containers, so absolute paths here are often
	// unreadable on the Drawing side.
	if filepath.IsAbs(resolved) {
		return filepath.ToSlash(filepath.Join(assets.StaticImagesDir, relPath))
	}

	return resolved
}

// WithSnapshot returns a shallow copy of this Controller that uses the given
// snapshot instead of the one configured at construction time. This is used by
// the bridge layer to inject a live Toolbox snapshot on a per-request basis.
func (c *Controller) WithSnapshot(s userdata.Snapshot) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.snapshot = s
	return &clone
}

// WithMySekaiData returns a shallow copy that uses raw mysekai JSON bytes
// directly, without going through the userdata.Service merge path. This is
// the preferred injection for mysekai-only commands: suite data is not needed
// because the profile card comes from the public API via query.Profile.
func (c *Controller) WithMySekaiData(data []byte) *Controller {
	if c == nil || len(data) == 0 {
		return nil
	}
	clone := *c
	clone.rawMySekaiJSON = data
	return &clone
}

func (c *Controller) ResolvePhoto(query PhotoQuery) (*PhotoResult, error) {
	if query.Seq == 0 {
		return nil, fmt.Errorf("请输入正确的照片编号（从1或-1开始）")
	}

	merged, region, err := c.prepareSnapshotOnly(query.Region)
	if err != nil {
		return nil, err
	}

	photos := nestedList(merged, "userMysekaiPhotos")
	if len(photos) == 0 {
		return nil, fmt.Errorf("当前账号没有可用的 MySekai 照片数据")
	}

	seq := query.Seq
	if seq < 0 {
		seq = len(photos) + seq + 1
	}
	if seq < 1 {
		return nil, fmt.Errorf("照片编号超出范围（当前共有%d张）", len(photos))
	}
	if seq > len(photos) {
		return nil, fmt.Errorf("照片编号大于照片数量(%d)", len(photos))
	}

	photo, ok := photos[seq-1].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("照片数据格式错误")
	}

	imagePath := stringValue(photo["imagePath"])
	if imagePath == "" {
		return nil, fmt.Errorf("该照片缺少 imagePath，无法下载")
	}

	result := &PhotoResult{
		Region:    region.String(),
		Seq:       seq,
		Total:     len(photos),
		ImagePath: imagePath,
	}
	if obtainedAt := int64Number(photo["obtainedAt"], 0); obtainedAt > 0 {
		result.ObtainedAt = time.UnixMilli(obtainedAt)
	}
	return result, nil
}

func (c *Controller) ensure() error {
	if err := c.ensureSnapshot(); err != nil {
		return err
	}
	if c.masterdata == nil || !c.masterdata.Configured() {
		return fmt.Errorf("mysekai masterdata is not configured")
	}
	return nil
}

func (c *Controller) ensureSnapshot() error {
	if c == nil {
		return fmt.Errorf("mysekai controller is not initialized")
	}
	// Direct mysekai JSON takes priority (no suite data required).
	if len(c.rawMySekaiJSON) > 0 {
		return nil
	}
	if c.snapshot == nil {
		return fmt.Errorf("user snapshot is not available (bind Toolbox or provide snapshot)")
	}
	if err := c.snapshot.Require(); err != nil {
		return err
	}
	return nil
}

func (c *Controller) resolveRegion(region string) renderregion.Value {
	normalized := renderregion.Normalize(region)
	if !normalized.IsZero() {
		return normalized
	}
	if !c.defaultRegion.IsZero() {
		return c.defaultRegion
	}
	return renderregion.JP
}

func (c *Controller) prepareSnapshot(region string) (map[string]interface{}, renderregion.Value, error) {
	if err := c.ensure(); err != nil {
		return nil, renderregion.Unknown, err
	}
	return c.decodeSnapshot(region)
}

func (c *Controller) prepareSnapshotOnly(region string) (map[string]interface{}, renderregion.Value, error) {
	if err := c.ensureSnapshot(); err != nil {
		return nil, renderregion.Unknown, err
	}
	return c.decodeSnapshot(region)
}

func (c *Controller) decodeSnapshot(region string) (map[string]interface{}, renderregion.Value, error) {
	var rawBytes []byte
	var err error

	if len(c.rawMySekaiJSON) > 0 {
		rawBytes = c.rawMySekaiJSON
	} else {
		rawBytes, err = c.snapshot.RawBytes()
		if err != nil {
			return nil, renderregion.Unknown, err
		}
	}

	var merged map[string]interface{}
	if err := json.Unmarshal(rawBytes, &merged); err != nil {
		return nil, renderregion.Unknown, fmt.Errorf("decode mysekai data: %w", err)
	}

	// When using raw mysekai JSON directly (not merged via userdata.Service),
	// flatten updatedResources so that keys like userMysekaiFixtures are
	// accessible at the top level, matching the merged-snapshot layout.
	if len(c.rawMySekaiJSON) > 0 {
		if updated, ok := merged["updatedResources"].(map[string]interface{}); ok {
			for key, value := range updated {
				merged[key] = value
			}
		}
	}

	return merged, c.resolveRegion(region), nil
}

func (c *Controller) mysekaiProfileCard(region renderregion.Value, merged map[string]interface{}, override *drawing.ProfileCardRequest) *drawing.ProfileCardRequest {
	var profile *drawing.ProfileCardRequest
	if override != nil {
		cloned := *override
		if override.Profile != nil {
			basic := *override.Profile
			cloned.Profile = &basic
		}
		if len(override.DataSources) > 0 {
			cloned.DataSources = append([]drawing.ProfileDataSource(nil), override.DataSources...)
		}
		profile = &cloned
	} else {
		if c == nil || c.snapshot == nil {
			return nil
		}
		profile = c.snapshot.ProfileCard(region)
	}
	if profile == nil {
		return nil
	}
	if updated, ok := merged["userMysekaiGamedata"].(map[string]interface{}); ok {
		if level := intNumber(updated["mysekaiRank"], 0); level > 0 {
			profile.MysekaiLevel = &level
		}
	}
	return profile
}

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

var mysekaiTalkUnitAliases = map[string]string{
	"l/n":                    "light_sound",
	"ln":                     "light_sound",
	"leoneed":                "light_sound",
	"light_sound":            "light_sound",
	"lightsound":             "light_sound",
	"light_sound_club":       "light_sound",
	"leo/need":               "light_sound",
	"mmj":                    "idol",
	"moremorejump":           "idol",
	"more_more_jump":         "idol",
	"idol":                   "idol",
	"vbs":                    "street",
	"vividbadsquad":          "street",
	"vivid_bad_squad":        "street",
	"street":                 "street",
	"ws":                     "theme_park",
	"wxs":                    "theme_park",
	"wonderlands":            "theme_park",
	"wonderlandsxshowtime":   "theme_park",
	"wonderlands_x_showtime": "theme_park",
	"theme_park":             "theme_park",
	"themepark":              "theme_park",
	"25":                     "school_refusal",
	"25h":                    "school_refusal",
	"25ji":                   "school_refusal",
	"niigo":                  "school_refusal",
	"nightcord":              "school_refusal",
	"school_refusal":         "school_refusal",
	"schoolrefusal":          "school_refusal",
	"25_ji_night_cord_de":    "school_refusal",
	"vs":                     "piapro",
	"piapro":                 "piapro",
	"virtualsinger":          "piapro",
}

var mysekaiFixedVirtualSingerUnits = map[int]string{
	22: "idol",
	23: "street",
	24: "light_sound",
	25: "street",
	26: "theme_park",
}

func (c *Controller) resolveTalkCharacter(query string) (int, int, error) {
	unit, cleanedQuery := extractMysekaiTalkUnit(query)
	cleanedQuery = strings.TrimSpace(cleanedQuery)
	if cleanedQuery == "" {
		return 0, 0, nil
	}

	gameCharacterUnits := c.masterdata.loadList("gameCharacterUnits.json")
	if len(strings.Fields(cleanedQuery)) == 1 {
		if target, err := strconv.Atoi(cleanedQuery); err == nil && target > 0 {
			for _, item := range gameCharacterUnits {
				if intNumber(item["id"], 0) == target {
					return intNumber(item["gameCharacterId"], 0), target, nil
				}
			}
			return c.resolveTalkCharacterUnit(cleanedQuery, unit, target, gameCharacterUnits)
		}
	}

	characterID := c.lookupTalkCharacterID(cleanedQuery)
	if characterID == 0 {
		return 0, 0, fmt.Errorf("找不到要查询的角色")
	}
	return c.resolveTalkCharacterUnit(cleanedQuery, unit, characterID, gameCharacterUnits)
}

func extractMysekaiTalkUnit(query string) (string, string) {
	fields := strings.Fields(strings.TrimSpace(query))
	if len(fields) == 0 {
		return "", ""
	}

	unit := ""
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		if resolved, ok := mysekaiTalkUnitAliases[strings.ToLower(strings.TrimSpace(field))]; ok && unit == "" {
			unit = resolved
			continue
		}
		remaining = append(remaining, field)
	}
	return unit, strings.TrimSpace(strings.Join(remaining, " "))
}

func (c *Controller) lookupTalkCharacterID(query string) int {
	normalized := normalizeMysekaiTalkCharacterQuery(query)
	if normalized == "" {
		return 0
	}
	if characterID, ok := c.nicknames[normalized]; ok {
		return characterID
	}

	characters := c.masterdata.loadMapByID("gameCharacters.json")
	for characterID, item := range characters {
		candidates := []string{
			stringValue(item["firstName"]),
			stringValue(item["givenName"]),
			strings.TrimSpace(stringValue(item["firstName"]) + stringValue(item["givenName"])),
			strings.TrimSpace(stringValue(item["firstName"]) + " " + stringValue(item["givenName"])),
			stringValue(item["firstNameEnglish"]),
			stringValue(item["givenNameEnglish"]),
			strings.TrimSpace(stringValue(item["firstNameEnglish"]) + stringValue(item["givenNameEnglish"])),
			strings.TrimSpace(stringValue(item["firstNameEnglish"]) + " " + stringValue(item["givenNameEnglish"])),
		}
		for _, candidate := range candidates {
			if normalizeMysekaiTalkCharacterQuery(candidate) == normalized {
				return characterID
			}
		}
	}
	return 0
}

func (c *Controller) resolveTalkCharacterUnit(query, unit string, characterID int, gameCharacterUnits []map[string]interface{}) (int, int, error) {
	candidates := make([]map[string]interface{}, 0, 6)
	for _, item := range gameCharacterUnits {
		if intNumber(item["gameCharacterId"], 0) != characterID {
			continue
		}
		candidates = append(candidates, item)
	}
	if len(candidates) == 0 {
		return 0, 0, fmt.Errorf("找不到要查询的角色")
	}

	candidates = c.filterMysekaiVirtualSingerCandidates(characterID, candidates)
	if fixedUnit, ok := mysekaiFixedVirtualSingerUnits[characterID]; ok {
		if unit != "" && normalizeMysekaiTalkUnit(unit) != fixedUnit {
			return 0, 0, fmt.Errorf("找不到要查询的角色")
		}
		unit = fixedUnit
	}

	if unit != "" {
		normalizedUnit := normalizeMysekaiTalkUnit(unit)
		for _, item := range candidates {
			if normalizeMysekaiTalkUnit(stringValue(item["unit"])) == normalizedUnit {
				return characterID, intNumber(item["id"], 0), nil
			}
		}
		return 0, 0, fmt.Errorf("找不到要查询的角色")
	}

	if len(candidates) == 1 {
		return characterID, intNumber(candidates[0]["id"], 0), nil
	}
	if characterID == 21 {
		return 0, 0, fmt.Errorf("查询存在多个组合的V家角色时需要同时指定组合，例如\"%s ln\"", strings.TrimSpace(query))
	}
	return characterID, intNumber(candidates[0]["id"], 0), nil
}

func (c *Controller) filterMysekaiVirtualSingerCandidates(characterID int, candidates []map[string]interface{}) []map[string]interface{} {
	if characterID < 21 || characterID > 26 || len(candidates) == 0 {
		return candidates
	}
	available := c.availableMysekaiCharacterUnitIDs()
	if len(available) == 0 {
		return candidates
	}
	filtered := make([]map[string]interface{}, 0, len(candidates))
	for _, item := range candidates {
		if _, ok := available[intNumber(item["id"], 0)]; ok {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) == 0 {
		return candidates
	}
	return filtered
}

func (c *Controller) availableMysekaiCharacterUnitIDs() map[int]struct{} {
	items := c.masterdata.loadList("mysekaiGateCharacterLotteries.json")
	if len(items) == 0 {
		return nil
	}
	result := make(map[int]struct{}, len(items))
	for _, item := range items {
		unitID := intNumber(item["gameCharacterUnitId"], 0)
		if unitID == 0 {
			unitID = intNumber(item["game_character_unit_id"], 0)
		}
		if unitID == 0 {
			continue
		}
		result[unitID] = struct{}{}
	}
	return result
}

func normalizeMysekaiTalkUnit(unit string) string {
	if resolved, ok := mysekaiTalkUnitAliases[strings.ToLower(strings.TrimSpace(unit))]; ok {
		return resolved
	}
	return strings.ToLower(strings.TrimSpace(unit))
}

func normalizeMysekaiTalkCharacterQuery(query string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(query)), ""))
}
