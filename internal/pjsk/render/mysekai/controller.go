package mysekai

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
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
	snapshot       *userdata.Service
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
func NewController(drawingClient *drawing.HarukiDrawingClient, snapshot *userdata.Service, masterdataDir string, defaultRegion renderregion.Value, assetHelper *assets.AssetHelper, sekaiDSN ...string) *Controller {
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
func (c *Controller) WithSnapshot(s *userdata.Service) *Controller {
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

func (c *Controller) BuildResourceRequest(query ResourceQuery) (*drawing.MysekaiResourceRequest, error) {
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}

	profile := c.mysekaiProfileCard(region, merged, query.Profile)
	if profile == nil {
		return nil, fmt.Errorf("mysekai resource requires profile data")
	}

	gateID, gateLevel, gateSkinID := extractMysekaiGateInfo(merged)
	return &drawing.MysekaiResourceRequest{
		Profile:             *profile,
		Phenoms:             extractMysekaiPhenoms(func(p string) string { return c.regionPath(region, p) }, merged),
		GateID:              gateID,
		GateLevel:           gateLevel,
		GateIconPath:        c.resolveGateIconPath(region, gateID, gateSkinID),
		VisitCharacters:     c.extractVisitCharacters(region, merged),
		SiteResourceNumbers: c.extractSiteResourceNumbers(region, merged),
	}, nil
}

func (c *Controller) resolveGateIconPath(region renderregion.Value, gateID, gateSkinID int) string {
	if assetbundleName := c.resolveGateAssetbundleName(gateID, gateSkinID); assetbundleName != "" {
		return c.regionPath(region, fmt.Sprintf("mysekai/thumbnail/gate_large/%s.png", assetbundleName))
	}
	return c.staticPath(fmt.Sprintf("mysekai/gate_icon/gate_%d.png", gateID))
}

func (c *Controller) resolveGateAssetbundleName(gateID, gateSkinID int) string {
	if gateSkinID > 0 {
		skins := c.masterdata.loadMapByID("mysekaiGateSkins.json")
		skin := skins[gateSkinID]
		if len(skin) > 0 {
			skinType := stringValue(skin["mysekaiGateSkinType"])
			skinTypeID := intNumber(skin["mysekaiGateSkinTypeId"], 0)
			if skinTypeID > 0 {
				switch skinType {
				case "unit":
					if unitSkin := c.masterdata.loadMapByID("mysekaiGateUnitSkins.json")[skinTypeID]; len(unitSkin) > 0 {
						if name := stringValue(unitSkin["assetbundleName"]); name != "" {
							return name
						}
					}
				case "common":
					if commonSkin := c.masterdata.loadMapByID("mysekaiGateCommonSkins.json")[skinTypeID]; len(commonSkin) > 0 {
						if name := stringValue(commonSkin["assetbundleName"]); name != "" {
							return name
						}
					}
				}
			}
		}
	}

	if gateID <= 0 {
		return ""
	}
	gates := c.masterdata.loadMapByID("mysekaiGates.json")
	gate := gates[gateID]
	if len(gate) == 0 {
		return ""
	}
	return stringValue(gate["assetbundleName"])
}

func (c *Controller) RenderResource(query ResourceQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildResourceRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiResource(payload)
}

func (c *Controller) BuildMapRequest(query MapQuery) (*drawing.MysekaiMsrMapRequest, error) {
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}

	showHarvested := false
	if query.ShowHarvested != nil {
		showHarvested = *query.ShowHarvested
	}

	harvestMapsBySite := make(map[int]map[string]interface{}, 4)
	for _, raw := range nestedList(merged, "userMysekaiHarvestMaps") {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		siteID := intNumber(item["mysekaiSiteId"], 0)
		if siteID != 0 {
			harvestMapsBySite[siteID] = item
		}
	}

	materialMap := c.loadIconNameMap("mysekaiMaterials.json", "iconAssetbundleName")
	materialRarityMap := c.loadFieldMap("mysekaiMaterials.json", "mysekaiMaterialRarityType")
	itemMap := c.loadIconNameMap("mysekaiItems.json", "iconAssetbundleName")
	fixtureMap := c.loadIconNameMap("mysekaiFixtures.json", "assetbundleName")
	musicRecordMap := c.loadMusicRecordJacketMap()
	harvestFixtureMap := c.masterdata.loadMapByID("mysekaiSiteHarvestFixtures.json")

	siteOrder := resolveMysekaiMapSiteIDs(query.MapIDs)
	if len(siteOrder) == 0 {
		return nil, fmt.Errorf("mysekai map query contains no valid map ids")
	}

	maps := make([]drawing.MysekaiMsrMapData, 0, len(siteOrder))
	for _, siteID := range siteOrder {
		siteMap := harvestMapsBySite[siteID]
		if len(siteMap) == 0 {
			continue
		}
		config, ok := mysekaiMapSiteConfigs[siteID]
		if !ok {
			continue
		}

		site := drawing.MysekaiMsrMapSiteInfo{
			ImagePath: c.staticPath(fmt.Sprintf("mysekai/site/%s.png", config.SiteImageName)),
			GridSize:  config.GridSize,
			OffsetX:   config.OffsetX,
			OffsetZ:   config.OffsetZ,
			DirX:      config.DirX,
			DirZ:      config.DirZ,
			RevXZ:     config.RevXZ,
			Scale:     0.8,
		}
		if len(config.CropBBox) > 0 {
			site.CropBbox = append([]int(nil), config.CropBBox...)
		}

		harvestPoints := make([]drawing.MysekaiMsrMapHarvestPoint, 0, 16)
		characterMap := c.masterdata.loadMapByID("gameCharacters.json")
		birthdayCharacterByPos := make(map[string]int)
		rawDrops, _ := siteMap["userMysekaiSiteHarvestResourceDrops"].([]interface{})
		for _, raw := range rawDrops {
			drop, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			resourceID := intNumber(drop["resourceId"], intNumber(drop["id"], 0))
			if resourceID < 174 || resourceID > 199 {
				continue
			}
			posKey := mysekaiHarvestPosKey(
				floatNumber(drop["positionX"], floatNumber(drop["position_x"], 0)),
				floatNumber(drop["positionZ"], floatNumber(drop["position_z"], 0)),
			)
			if posKey == "" {
				continue
			}
			if _, exists := birthdayCharacterByPos[posKey]; !exists {
				birthdayCharacterByPos[posKey] = resourceID - 173
			}
		}

		rawHarvestPoints, _ := siteMap["userMysekaiSiteHarvestFixtures"].([]interface{})
		for _, raw := range rawHarvestPoints {
			point, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			fixtureID := intNumber(point["mysekaiSiteHarvestFixtureId"], 0)
			meta := harvestFixtureMap[fixtureID]
			rarityType := stringValue(meta["mysekaiSiteHarvestFixtureRarityType"])
			assetbundleName := stringValue(meta["assetbundleName"])
			fixtureType := stringValue(meta["mysekaiSiteHarvestFixtureType"])
			if rarityType == "" || assetbundleName == "" {
				continue
			}
			// Special-case: tone_gust harvest point should be skipped in msp map output.
			lowerAssetbundle := strings.ToLower(assetbundleName)
			if fixtureType == "tone_gust" || lowerAssetbundle == "tone_gust" || strings.Contains(lowerAssetbundle, "tone_gust") {
				continue
			}

			status := stringValue(point["userMysekaiSiteHarvestFixtureStatus"])
			if status == "" {
				status = stringValue(point["mysekaiSiteHarvestFixtureStatus"])
			}
			if status == "" {
				status = "spawned"
			}

			var pointID *int
			if fixtureID > 0 {
				idCopy := fixtureID
				pointID = &idCopy
			}

			positionX := floatNumber(point["positionX"], floatNumber(point["position_x"], 0))
			positionZ := floatNumber(point["positionZ"], floatNumber(point["position_z"], 0))
			posKey := mysekaiHarvestPosKey(positionX, positionZ)

			imageRelPath := fmt.Sprintf("mysekai/harvest_fixture_icon/%s/%s.png", rarityType, assetbundleName)
			var size *int
			var offsetX float64
			offsetZ := -48.0
			if fixtureType == "birthday_plant" {
				if characterID := birthdayCharacterByPos[posKey]; characterID > 0 {
					if imageName := mysekaiBirthdayCharacterImageName(characterMap[characterID]); imageName != "" {
						imageRelPath = fmt.Sprintf("mysekai/birthday/%s_%d/icon_refresh.png", imageName, time.Now().Year())
					}
				}
				sizeValue := 50
				size = &sizeValue
				offsetX = 7.5
				offsetZ = 0
			}

			harvestPoints = append(harvestPoints, drawing.MysekaiMsrMapHarvestPoint{
				ID:        pointID,
				ImagePath: c.staticPath(imageRelPath),
				PositionX: positionX,
				PositionZ: positionZ,
				Status:    status,
				Size:      size,
				OffsetX:   offsetX,
				OffsetZ:   offsetZ,
			})
		}

		type groupedMysekaiResourceDrop struct {
			ID                  int
			Type                string
			ImagePath           string
			PositionX           float64
			PositionZ           float64
			Quantity            int
			Status              string
			SmallIcon           *bool
			Hide                bool
			Rarity              int
			AttachmentImagePath *string
		}

		resourceDropsByPos := make(map[string]map[string]*groupedMysekaiResourceDrop)
		for _, raw := range rawDrops {
			drop, ok := raw.(map[string]interface{})
			if !ok {
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

			resourceKey := fmt.Sprintf("%s_%d", resourceType, resourceID)
			imagePath, hasRecord := c.resourceImagePath(region, resourceKey, materialMap, itemMap, fixtureMap, musicRecordMap, merged)
			if imagePath == "" {
				continue
			}

			status := stringValue(drop["mysekaiSiteHarvestResourceDropStatus"])
			if status == "" {
				status = stringValue(drop["status"])
			}
			if status == "" {
				status = "before_drop"
			}

			quantity := intNumber(drop["quantity"], 1)
			if quantity <= 0 {
				quantity = 1
			}

			rarity := resourceRarity(resourceKey, materialRarityMap)
			if rarity < 1 {
				rarity = 1
			}

			var attachmentImagePath *string
			if hasRecord {
				path := c.staticPath("mysekai/music_record.png")
				attachmentImagePath = &path
			}

			positionX := floatNumber(drop["positionX"], floatNumber(drop["position_x"], 0))
			positionZ := floatNumber(drop["positionZ"], floatNumber(drop["position_z"], 0))
			posKey := mysekaiHarvestPosKey(positionX, positionZ)
			if posKey == "" {
				continue
			}
			resourceGroupKey := fmt.Sprintf("%s_%d", resourceType, resourceID)
			if _, ok := resourceDropsByPos[posKey]; !ok {
				resourceDropsByPos[posKey] = make(map[string]*groupedMysekaiResourceDrop)
			}
			if existing := resourceDropsByPos[posKey][resourceGroupKey]; existing != nil {
				existing.Quantity += quantity
				if existing.AttachmentImagePath == nil && attachmentImagePath != nil {
					existing.AttachmentImagePath = attachmentImagePath
				}
				if existing.Status == "" {
					existing.Status = status
				}
				continue
			}
			resourceDropsByPos[posKey][resourceGroupKey] = &groupedMysekaiResourceDrop{
				ID:                  resourceID,
				Type:                resourceType,
				ImagePath:           imagePath,
				PositionX:           positionX,
				PositionZ:           positionZ,
				Quantity:            quantity,
				Status:              status,
				Rarity:              rarity,
				AttachmentImagePath: attachmentImagePath,
			}
		}

		resourceDrops := make([]drawing.MysekaiMsrMapResourceDrop, 0, 32)
		for _, grouped := range resourceDropsByPos {
			hasMaterialDrop := false
			hasFixtureDrop := false
			isCottonFlower := false
			isBirthdaySapling := false
			for key, item := range grouped {
				if (key == "mysekai_material_1" || key == "mysekai_material_6") && item.Quantity == 6 {
					item.Hide = true
				}
				if key == "mysekai_material_21" || key == "mysekai_material_22" {
					isCottonFlower = true
				}
				if strings.HasPrefix(key, "mysekai_material_") {
					hasMaterialDrop = true
				}
				if item.Type == "mysekai_fixture" {
					hasFixtureDrop = true
				}
				if mysekaiIsBirthdayDrop(item.Type, item.ID) && item.Quantity > 16 {
					isBirthdaySapling = true
				}
			}
			for key, item := range grouped {
				smallIcon := false
				smallIconSet := false
				if hasFixtureDrop {
					if item.Type == "mysekai_fixture" {
						// Keep seed/sapling drops as the primary large icon when they
						// share a tile with materials/items.
						smallIcon = false
						smallIconSet = true
					} else {
						smallIcon = true
						smallIconSet = true
					}
				} else if !strings.HasPrefix(key, "mysekai_material_") && hasMaterialDrop {
					smallIcon = true
					smallIconSet = true
				}
				if isCottonFlower && key != "mysekai_material_21" && key != "mysekai_material_22" {
					smallIcon = true
					smallIconSet = true
				}
				if isBirthdaySapling {
					smallIcon = !mysekaiIsBirthdayDrop(item.Type, item.ID)
					smallIconSet = true
				} else if mysekaiIsBirthdayDrop(item.Type, item.ID) {
					item.Hide = true
				}
				if smallIconSet {
					iconCopy := smallIcon
					item.SmallIcon = &iconCopy
				}
				resourceDrops = append(resourceDrops, drawing.MysekaiMsrMapResourceDrop{
					ID:                  item.ID,
					Type:                item.Type,
					ImagePath:           item.ImagePath,
					PositionX:           item.PositionX,
					PositionZ:           item.PositionZ,
					Quantity:            item.Quantity,
					Status:              item.Status,
					SmallIcon:           item.SmallIcon,
					Hide:                item.Hide,
					Rarity:              item.Rarity,
					AttachmentImagePath: item.AttachmentImagePath,
				})
			}
		}

		maps = append(maps, drawing.MysekaiMsrMapData{
			MapID:         siteID,
			Site:          site,
			HarvestPoints: harvestPoints,
			ResourceDrops: resourceDrops,
		})
	}

	if len(maps) == 0 {
		return nil, fmt.Errorf("mysekai map contains no harvest map data")
	}

	return &drawing.MysekaiMsrMapRequest{
		Maps:          maps,
		ShowHarvested: showHarvested,
		SpawnImagePath: drawing.StringPtr(
			c.staticPath("mysekai/mark.png"),
		),
	}, nil
}

func (c *Controller) RenderMap(query MapQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMapRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiMap(payload)
}

func resolveMysekaiMapSiteIDs(requested []int) []int {
	if len(requested) == 0 {
		return append([]int(nil), mysekaiMapSiteOrder...)
	}
	result := make([]int, 0, len(requested))
	seen := make(map[int]struct{}, len(requested))
	for _, siteID := range requested {
		if _, ok := mysekaiMapSiteConfigs[siteID]; !ok {
			continue
		}
		if _, ok := seen[siteID]; ok {
			continue
		}
		seen[siteID] = struct{}{}
		result = append(result, siteID)
	}
	return result
}

func (c *Controller) BuildFixtureListRequest(query FixtureListQuery) (*drawing.MysekaiFixtureListRequest, error) {
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}

	showID := true
	if query.ShowID != nil {
		showID = *query.ShowID
	}
	onlyCraftable := query.OnlyCraftable != nil && *query.OnlyCraftable

	fixturesData := c.masterdata.loadList("mysekaiFixtures.json")
	mainGenreMap := c.masterdata.loadMapByID("mysekaiFixtureMainGenres.json")
	subGenreMap := c.masterdata.loadMapByID("mysekaiFixtureSubGenres.json")
	blueprints := c.masterdata.loadMapByID("mysekaiBlueprints.json")
	characters := c.masterdata.loadMapByID("gameCharacters.json")
	obtainedFixtureIDs := c.obtainedMysekaiFixtureIDs(merged, blueprints)
	craftableFixtureIDs := c.craftableMysekaiFixtureIDs(blueprints)

	type fixtureRow struct {
		fixture drawing.MysekaiFixture
	}

	grouped := map[int]map[int][]fixtureRow{}
	mainProgressAll := map[int]int{}
	mainProgressObtained := map[int]int{}
	subProgressAll := map[int]map[int]int{}
	subProgressObtained := map[int]map[int]int{}
	totalAll := 0
	totalObtained := 0

	for _, item := range fixturesData {
		fixtureID := intNumber(item["id"], 0)
		if fixtureID == 0 || strings.EqualFold(stringValue(item["mysekaiFixtureType"]), "gate") {
			continue
		}
		if onlyCraftable {
			if _, ok := craftableFixtureIDs[fixtureID]; !ok {
				continue
			}
		}

		mainGenreID := intNumber(item["mysekaiFixtureMainGenreId"], -1)
		subGenreID := intNumber(item["mysekaiFixtureSubGenreId"], -1)
		if fixtureID == 4 {
			subGenreID = 14
		}
		if _, ok := map[int]struct{}{4: {}, 5: {}, 7: {}, 8: {}, 9: {}, 10: {}, 11: {}, 12: {}, 13: {}}[mainGenreID]; ok {
			subGenreID = -1
		}

		if _, ok := grouped[mainGenreID]; !ok {
			grouped[mainGenreID] = map[int][]fixtureRow{}
			subProgressAll[mainGenreID] = map[int]int{}
			subProgressObtained[mainGenreID] = map[int]int{}
		}

		obtained := hasFixture(obtainedFixtureIDs, fixtureID)
		var characterID *int
		if charID := birthdayCharacterID(characters, stringValue(item["name"])); charID != 0 {
			characterID = &charID
		}

		grouped[mainGenreID][subGenreID] = append(grouped[mainGenreID][subGenreID], fixtureRow{
			fixture: drawing.MysekaiFixture{
				ID:          fixtureID,
				ImagePath:   fixtureThumbnailPath(func(p string) string { return c.regionPath(region, p) }, item),
				CharacterID: characterID,
				Obtained:    obtained,
			},
		})

		if characterID == nil {
			totalAll++
			if obtained {
				totalObtained++
			}
			mainProgressAll[mainGenreID]++
			subProgressAll[mainGenreID][subGenreID]++
			if obtained {
				mainProgressObtained[mainGenreID]++
				subProgressObtained[mainGenreID][subGenreID]++
			}
		}
	}

	mainGenreIDs := make([]int, 0, len(grouped))
	for genreID := range grouped {
		mainGenreIDs = append(mainGenreIDs, genreID)
	}
	sort.Ints(mainGenreIDs)

	mainGenres := make([]drawing.MysekaiFixtureMainGenre, 0, len(mainGenreIDs))
	for _, genreID := range mainGenreIDs {
		subGenreIDs := make([]int, 0, len(grouped[genreID]))
		for subID := range grouped[genreID] {
			subGenreIDs = append(subGenreIDs, subID)
		}
		sort.Ints(subGenreIDs)

		subGenres := make([]drawing.MysekaiFixtureSubGenre, 0, len(subGenreIDs))
		for _, subID := range subGenreIDs {
			rows := grouped[genreID][subID]
			if len(rows) == 0 {
				continue
			}
			fixtures := make([]drawing.MysekaiFixture, 0, len(rows))
			for _, row := range rows {
				fixtures = append(fixtures, row.fixture)
			}
			subGenre := drawing.MysekaiFixtureSubGenre{
				Fixtures: fixtures,
			}
			if subID != -1 && len(grouped[genreID]) > 1 {
				if info := subGenreMap[subID]; len(info) > 0 {
					name := stringValue(info["name"])
					imagePath := c.regionPath(region, fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(info["assetbundleName"])))
					subGenre.Name = &name
					subGenre.ImagePath = &imagePath
					if total := subProgressAll[genreID][subID]; total > 0 {
						message := fmt.Sprintf("%d/%d (%.1f%%)", subProgressObtained[genreID][subID], total, percent(subProgressObtained[genreID][subID], total))
						subGenre.ProgressMessage = &message
					}
				}
			}
			subGenres = append(subGenres, subGenre)
		}
		if len(subGenres) == 0 {
			continue
		}

		mainInfo := mainGenreMap[genreID]
		mainGenre := drawing.MysekaiFixtureMainGenre{
			Name:      stringValue(mainInfo["name"]),
			ImagePath: c.regionPath(region, fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(mainInfo["assetbundleName"]))),
			SubGenres: subGenres,
		}
		if total := mainProgressAll[genreID]; total > 0 {
			message := fmt.Sprintf("%d/%d (%.1f%%)", mainProgressObtained[genreID], total, percent(mainProgressObtained[genreID], total))
			mainGenre.ProgressMessage = &message
		}
		mainGenres = append(mainGenres, mainGenre)
	}

	request := &drawing.MysekaiFixtureListRequest{
		Profile:    c.mysekaiProfileCard(region, merged, query.Profile),
		ShowID:     showID,
		MainGenres: mainGenres,
	}
	if totalAll > 0 {
		message := fmt.Sprintf("总收集进度（不含生日家具）: %d/%d (%.1f%%)", totalObtained, totalAll, percent(totalObtained, totalAll))
		request.ProgressMessage = &message
	}
	return request, nil
}

func (c *Controller) RenderFixtureList(query FixtureListQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildFixtureListRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiFixtureList(payload)
}

func (c *Controller) BuildFixtureDetailRequests(query FixtureDetailQuery) ([]drawing.MysekaiFixtureDetailRequest, error) {
	if err := c.ensure(); err != nil {
		return nil, err
	}

	fixtureIDs := parseIntTokens(query.Query)
	if len(fixtureIDs) == 0 {
		return nil, fmt.Errorf("mysekai fixture detail invalid query: %s", query.Query)
	}

	fixtureMap := c.masterdata.loadMapByID("mysekaiFixtures.json")
	mainGenreMap := c.masterdata.loadMapByID("mysekaiFixtureMainGenres.json")
	subGenreMap := c.masterdata.loadMapByID("mysekaiFixtureSubGenres.json")
	blueprints := c.masterdata.loadList("mysekaiBlueprints.json")
	blueprintCosts := c.masterdata.loadList("mysekaiBlueprintMysekaiMaterialCosts.json")
	onlyDisassemble := c.masterdata.loadList("mysekaiFixtureOnlyDisassembleMaterials.json")
	tags := c.masterdata.loadMapByID("mysekaiFixtureTags.json")
	region := c.resolveRegion(query.Region)

	requests := make([]drawing.MysekaiFixtureDetailRequest, 0, len(fixtureIDs))
	for _, fixtureID := range fixtureIDs {
		fixture := fixtureMap[fixtureID]
		if len(fixture) == 0 {
			continue
		}
		mainGenreID := intNumber(fixture["mysekaiFixtureMainGenreId"], 0)
		subGenreID := intNumber(fixture["mysekaiFixtureSubGenreId"], 0)
		mainGenre := mainGenreMap[mainGenreID]
		subGenre := subGenreMap[subGenreID]

		request := drawing.MysekaiFixtureDetailRequest{
			Title:              fmt.Sprintf("【%s-%d】%s", strings.ToUpper(region.String()), fixtureID, stringValue(fixture["name"])),
			Images:             fixtureColorImages(func(p string) string { return c.regionPath(region, p) }, fixture),
			MainGenreName:      stringValue(mainGenre["name"]),
			MainGenreImagePath: c.regionPath(region, fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(mainGenre["assetbundleName"]))),
			Size: map[string]int{
				"width":  nestedInt(fixture, "gridSize", "width"),
				"depth":  nestedInt(fixture, "gridSize", "depth"),
				"height": nestedInt(fixture, "gridSize", "height"),
			},
			FirstPutCost:            intNumber(fixture["firstPutCost"], 0),
			SecondPutCost:           intNumber(fixture["secondPutCost"], 0),
			BasicInfo:               fixtureBasicInfo(fixture),
			Tags:                    fixtureTags(fixture, tags),
			ReactionCharacterGroups: c.fixtureReactionCharacterGroups(fixtureID),
			RecycleMaterials:        c.fixtureRecycleMaterials(region, fixtureID, onlyDisassemble),
		}

		if subGenreID != 0 {
			subName := stringValue(subGenre["name"])
			subPath := c.regionPath(region, fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(subGenre["assetbundleName"])))
			request.SubGenreName = &subName
			request.SubGenreImagePath = &subPath
		}
		if blueprint := findFixtureBlueprint(blueprints, fixtureID); blueprint != nil {
			request.BasicInfo = append(request.BasicInfo, fixtureBlueprintInfo(blueprint)...)
			request.CostMaterials = c.fixtureCostMaterials(region, intNumber(blueprint["id"], 0), blueprintCosts)
		}
		requests = append(requests, request)
	}

	if len(requests) == 0 {
		return nil, fmt.Errorf("mysekai fixture detail found no valid fixtures")
	}
	return requests, nil
}

func (c *Controller) RenderFixtureDetail(query FixtureDetailQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	requests, err := c.BuildFixtureDetailRequests(query)
	if err != nil {
		return nil, err
	}
	if len(requests) != 1 {
		return nil, fmt.Errorf("mysekai fixture detail render requires exactly one fixture id")
	}
	return c.drawing.GenerateMysekaiFixtureDetail(&requests[0])
}

func (c *Controller) BuildDoorUpgradeRequest(query DoorUpgradeQuery) (*drawing.MysekaiDoorUpgradeRequest, error) {
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}

	specGateID := 0
	if ids := parseIntTokens(query.Query); len(ids) > 0 {
		specGateID = ids[0]
	}

	userMaterials := map[int]int{}
	for _, raw := range nestedList(merged, "userMysekaiMaterials") {
		item, _ := raw.(map[string]interface{})
		userMaterials[intNumber(item["mysekaiMaterialId"], 0)] = intNumber(item["quantity"], 0)
	}

	specLevels := map[int]int{}
	for _, raw := range nestedList(merged, "userMysekaiGates") {
		item, _ := raw.(map[string]interface{})
		gateID := intNumber(item["mysekaiGateId"], 0)
		if gateID != 0 {
			specLevels[gateID] = intNumber(item["mysekaiGateLevel"], 0)
		}
	}

	const gateMaxLevel = 40

	type tempItem struct {
		MaterialID int
		Quantity   int
	}

	gateTemp := map[int][][]tempItem{}
	for _, item := range c.masterdata.loadList("mysekaiGateMaterialGroups.json") {
		groupID := intNumber(item["groupId"], 0)
		if groupID == 0 {
			continue
		}
		gateID := groupID / 1000
		level := groupID % 1000
		if gateID == 0 || level <= 0 || level > gateMaxLevel {
			continue
		}
		if _, ok := gateTemp[gateID]; !ok {
			gateTemp[gateID] = make([][]tempItem, gateMaxLevel)
		}
		gateTemp[gateID][level-1] = append(gateTemp[gateID][level-1], tempItem{
			MaterialID: intNumber(item["mysekaiMaterialId"], 0),
			Quantity:   intNumber(item["quantity"], 0),
		})
	}

	if specGateID == 0 {
		bestLevel := 0
		for gateID, level := range specLevels {
			if level == gateMaxLevel || level <= bestLevel {
				continue
			}
			bestLevel = level
			specGateID = gateID
		}
	}
	if specGateID != 0 {
		if level := specLevels[specGateID]; level == gateMaxLevel {
			return nil, fmt.Errorf("queried gate already max level")
		}
		if mats, ok := gateTemp[specGateID]; ok {
			gateTemp = map[int][][]tempItem{specGateID: mats}
		}
	}

	materialIcons := c.loadIconNameMap("mysekaiMaterials.json", "iconAssetbundleName")
	gateIDs := make([]int, 0, len(gateTemp))
	for gateID := range gateTemp {
		gateIDs = append(gateIDs, gateID)
	}
	sort.Ints(gateIDs)

	green := []int{0, 200, 0}
	red := []int{200, 0, 0}

	gateMaterials := make([]drawing.MysekaiGateMaterials, 0, len(gateIDs))
	for _, gateID := range gateIDs {
		levelMats := gateTemp[gateID]
		currentLevel := specLevels[gateID]
		if currentLevel > 0 && currentLevel < len(levelMats) {
			levelMats = levelMats[currentLevel:]
		}

		sumMaterials := map[int]int{}
		outLevels := make([]drawing.MysekaiGateLevelMaterials, 0, len(levelMats))
		for index, items := range levelMats {
			if len(items) == 0 {
				continue
			}
			levelColor := []int{50, 50, 50}
			outItems := make([]drawing.MysekaiGateMaterialItem, 0, len(items))
			for _, item := range items {
				sumMaterials[item.MaterialID] += item.Quantity
				userQty := userMaterials[item.MaterialID]
				color := green
				if userQty < sumMaterials[item.MaterialID] {
					color = red
					levelColor = red
				}
				outItems = append(outItems, drawing.MysekaiGateMaterialItem{
					ImagePath:   c.regionPath(region, fmt.Sprintf("mysekai/thumbnail/material/%s.png", materialIcons[item.MaterialID])),
					Quantity:    item.Quantity,
					Color:       color,
					SumQuantity: fmt.Sprintf("%s/%d", formatMysekaiQuantity(userQty), sumMaterials[item.MaterialID]),
				})
			}
			outLevels = append(outLevels, drawing.MysekaiGateLevelMaterials{
				Level: currentLevel + index + 1,
				Color: levelColor,
				Items: outItems,
			})
		}
		levelCopy := currentLevel
		gateMaterials = append(gateMaterials, drawing.MysekaiGateMaterials{
			ID:             gateID,
			Level:          &levelCopy,
			LevelMaterials: outLevels,
		})
	}

	return &drawing.MysekaiDoorUpgradeRequest{
		Profile:       c.mysekaiProfileCard(region, merged, query.Profile),
		GateMaterials: gateMaterials,
	}, nil
}

func (c *Controller) RenderDoorUpgrade(query DoorUpgradeQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildDoorUpgradeRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiDoorUpgrade(payload)
}

func (c *Controller) BuildMusicRecordRequest(query MusicRecordQuery) (*drawing.MysekaiMusicrecordRequest, error) {
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}

	showID := false
	if query.ShowID != nil {
		showID = *query.ShowID
	}

	obtainedRecords := map[int]int64{}
	for _, raw := range nestedList(merged, "userMysekaiMusicRecords") {
		item, _ := raw.(map[string]interface{})
		obtainedRecords[intNumber(item["mysekaiMusicRecordId"], 0)] = int64Number(item["obtainedAt"], 0)
	}

	records := c.masterdata.loadList("mysekaiMusicRecords.json")
	musicTags := c.masterdata.loadList("musicTags.json")
	musics := c.masterdata.loadMapByID("musics.json")
	limitedTimes := c.masterdata.loadList("limitedTimeMusics.json")

	categoryMusicIDs := map[string][]int{
		"light_music_club": {},
		"idol":             {},
		"street":           {},
		"theme_park":       {},
		"school_refusal":   {},
		"vocaloid":         {},
		"other":            {},
	}

	limitedByMusic := map[int][]map[string]interface{}{}
	for _, item := range limitedTimes {
		musicID := intNumber(item["musicId"], 0)
		if musicID != 0 {
			limitedByMusic[musicID] = append(limitedByMusic[musicID], item)
		}
	}

	tagByMusicID := map[int]string{}
	for _, item := range musicTags {
		musicID := intNumber(item["musicId"], 0)
		tag := stringValue(item["musicTag"])
		if musicID == 0 || tag == "" || tag == "all" || tag == "vocaloid" {
			continue
		}
		if _, ok := tagByMusicID[musicID]; !ok {
			tagByMusicID[musicID] = tag
		}
	}

	musicObtainedAt := map[int]int64{}
	nowMs := time.Now().UnixMilli()
	for _, record := range records {
		if stringValue(record["mysekaiMusicTrackType"]) != "music" {
			continue
		}
		recordID := intNumber(record["id"], 0)
		musicID := intNumber(record["externalId"], 0)
		if recordID == 0 || musicID == 0 {
			continue
		}
		if musicID == 241 || musicID == 290 {
			continue
		}

		music := musics[musicID]
		if len(music) == 0 {
			continue
		}
		if int64Number(music["publishedAt"], 0) > nowMs {
			continue
		}
		if windows := limitedByMusic[musicID]; len(windows) > 0 && !isMusicAvailableNow(windows, nowMs) {
			continue
		}

		if ts, ok := obtainedRecords[recordID]; ok {
			musicObtainedAt[musicID] = ts
		}

		tag := tagByMusicID[musicID]
		if tag == "" {
			tag = "vocaloid"
		}
		categoryMusicIDs[tag] = append(categoryMusicIDs[tag], musicID)
	}

	tagIcons := map[string]string{
		"light_music_club": c.staticPath("icon_light_sound.png"),
		"idol":             c.staticPath("icon_idol.png"),
		"street":           c.staticPath("icon_street.png"),
		"theme_park":       c.staticPath("icon_theme_park.png"),
		"school_refusal":   c.staticPath("icon_school_refusal.png"),
		"vocaloid":         c.staticPath("icon_piapro.png"),
		"other":            "",
	}
	order := []string{"light_music_club", "street", "idol", "theme_park", "school_refusal", "vocaloid", "other"}

	totalCount := 0
	obtainedCount := 0
	categories := make([]drawing.MysekaiCategoryMusicrecord, 0, len(order))
	for _, tag := range order {
		musicIDs := categoryMusicIDs[tag]
		sort.Slice(musicIDs, func(i, j int) bool {
			left, leftObtained := musicObtainedAt[musicIDs[i]]
			right, rightObtained := musicObtainedAt[musicIDs[j]]
			if leftObtained && rightObtained {
				return left < right
			}
			if leftObtained != rightObtained {
				return leftObtained
			}
			return musicIDs[i] < musicIDs[j]
		})

		categoryTotal := len(musicIDs)
		categoryObtained := 0
		records := make([]drawing.MysekaiMusicrecord, 0, len(musicIDs))
		for _, musicID := range musicIDs {
			totalCount++
			if musicObtainedAt[musicID] != 0 {
				obtainedCount++
				categoryObtained++
			}
			assetbundleName := stringValue(musics[musicID]["assetbundleName"])
			if assetbundleName == "" {
				continue
			}
			record := drawing.MysekaiMusicrecord{
				ImagePath: c.regionPath(region, fmt.Sprintf("music/jacket/%s/%s.png", assetbundleName, assetbundleName)),
				Obtained:  musicObtainedAt[musicID] != 0,
			}
			if showID {
				idCopy := musicID
				record.ID = &idCopy
			}
			records = append(records, record)
		}
		if categoryTotal == 0 {
			continue
		}
		message := fmt.Sprintf("%d/%d (%.1f%%)", categoryObtained, categoryTotal, percent(categoryObtained, categoryTotal))
		categories = append(categories, drawing.MysekaiCategoryMusicrecord{
			Tag:             tag,
			TagIconPath:     tagIcons[tag],
			ProgressMessage: &message,
			Musicrecords:    records,
		})
	}

	profile := c.mysekaiProfileCard(region, merged, query.Profile)
	if profile == nil {
		return nil, fmt.Errorf("mysekai music record requires profile data")
	}
	request := &drawing.MysekaiMusicrecordRequest{
		Profile:              *profile,
		CategoryMusicrecords: categories,
	}
	if totalCount > 0 {
		message := fmt.Sprintf("总收集进度: %d/%d (%.1f%%)", obtainedCount, totalCount, percent(obtainedCount, totalCount))
		request.ProgressMessage = &message
	}
	return request, nil
}

func (c *Controller) RenderMusicRecord(query MusicRecordQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildMusicRecordRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiMusicRecord(payload)
}

func (c *Controller) BuildTalkListRequest(query TalkListQuery) (*drawing.MysekaiTalkListRequest, error) {
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(query.Query) == "" {
		return nil, fmt.Errorf("mysekai talk list requires character query")
	}
	showAllTalks := query.ShowAllTalks != nil && *query.ShowAllTalks

	_, characterUnitID, err := c.resolveTalkCharacter(query.Query)
	if err != nil {
		return nil, err
	}
	if characterUnitID == 0 {
		return nil, fmt.Errorf("mysekai talk list invalid character query: %s", query.Query)
	}

	obtainedFixtureIDs := c.obtainedMysekaiFixtureIDs(merged, c.masterdata.loadMapByID("mysekaiBlueprints.json"))
	fixturesData := c.masterdata.loadList("mysekaiFixtures.json")
	fixtureMap := c.masterdata.loadMapByID("mysekaiFixtures.json")
	mainGenreMap := c.masterdata.loadMapByID("mysekaiFixtureMainGenres.json")
	gameCharacterUnitGroups := c.masterdata.loadMapByID("mysekaiGameCharacterUnitGroups.json")
	archiveGroups := c.masterdata.loadMapByID("characterArchiveMysekaiCharacterTalkGroups.json")
	conditions := c.masterdata.loadList("mysekaiCharacterTalkConditions.json")
	conditionGroups := c.masterdata.loadList("mysekaiCharacterTalkConditionGroups.json")
	talks := c.masterdata.loadList("mysekaiCharacterTalks.json")

	userTalkReads := map[int]bool{}
	if !showAllTalks {
		for _, raw := range nestedList(merged, "userMysekaiCharacterTalks") {
			item, _ := raw.(map[string]interface{})
			userTalkReads[intNumber(item["mysekaiCharacterTalkId"], 0)] = boolValue(item["isRead"])
		}
	}

	type talkRead struct {
		fixtureIDs []int
		read       int
		total      int
		cuidsSet   [][]int
		hasRead    bool
		cuids      []int
	}

	conditionIDsByFixture := map[int][]int{}
	for _, condition := range conditions {
		if stringValue(condition["mysekaiCharacterTalkConditionType"]) != "mysekai_fixture_id" {
			continue
		}
		fixtureID := intNumber(condition["mysekaiCharacterTalkConditionTypeValue"], 0)
		if fixtureID != 0 {
			conditionIDsByFixture[fixtureID] = append(conditionIDsByFixture[fixtureID], intNumber(condition["id"], 0))
		}
	}

	groupIDsByCondition := map[int][]int{}
	for _, group := range conditionGroups {
		conditionID := intNumber(group["mysekaiCharacterTalkConditionId"], 0)
		groupIDsByCondition[conditionID] = append(groupIDsByCondition[conditionID], intNumber(group["id"], 0))
	}

	talksByGroup := map[int][]map[string]interface{}{}
	for _, talk := range talks {
		groupID := intNumber(talk["mysekaiCharacterTalkConditionGroupId"], 0)
		talksByGroup[groupID] = append(talksByGroup[groupID], talk)
	}

	archiveReads := map[int]*talkRead{}
	for _, fixture := range fixturesData {
		fixtureID := intNumber(fixture["id"], 0)
		if fixtureID == 0 || stringValue(fixture["mysekaiFixtureType"]) == "gate" {
			continue
		}

		groupIDs := map[int]struct{}{}
		for _, conditionID := range conditionIDsByFixture[fixtureID] {
			for _, groupID := range groupIDsByCondition[conditionID] {
				groupIDs[groupID] = struct{}{}
			}
		}
		for groupID := range groupIDs {
			for _, talk := range talksByGroup[groupID] {
				talkID := intNumber(talk["id"], 0)
				group := gameCharacterUnitGroups[intNumber(talk["mysekaiGameCharacterUnitGroupId"], 0)]
				if len(group) == 0 {
					continue
				}
				groupCuids := extractGroupCuids(group)
				if !containsInt(groupCuids, characterUnitID) {
					continue
				}

				archiveID := intNumber(talk["characterArchiveMysekaiCharacterTalkGroupId"], 0)
				archive := archiveGroups[archiveID]
				if len(archive) > 0 && stringValue(archive["archiveDisplayType"]) != "normal" {
					continue
				}
				if _, ok := archiveReads[archiveID]; !ok {
					archiveReads[archiveID] = &talkRead{}
				}
				read := archiveReads[archiveID]
				if !containsInt(read.fixtureIDs, fixtureID) {
					read.fixtureIDs = append(read.fixtureIDs, fixtureID)
				}
				read.cuids = groupCuids
				if userTalkReads[talkID] {
					read.hasRead = true
				}
			}
		}
	}

	singleReads := map[string]*talkRead{}
	multiReadsMap := map[string]*talkRead{}
	for _, item := range archiveReads {
		sort.Ints(item.fixtureIDs)
		keyParts := make([]string, 0, len(item.fixtureIDs))
		for _, fixtureID := range item.fixtureIDs {
			keyParts = append(keyParts, strconv.Itoa(fixtureID))
		}
		key := strings.Join(keyParts, " ")
		target := singleReads
		if len(item.cuids) > 1 {
			target = multiReadsMap
		}
		if _, ok := target[key]; !ok {
			target[key] = &talkRead{}
		}
		target[key].fixtureIDs = item.fixtureIDs
		target[key].total++
		if item.hasRead {
			target[key].read++
			continue
		}
		if len(item.cuids) > 1 {
			duplicate := false
			for _, existing := range target[key].cuidsSet {
				if intsEqual(existing, item.cuids) {
					duplicate = true
					break
				}
			}
			if !duplicate {
				target[key].cuidsSet = append(target[key].cuidsSet, item.cuids)
			}
		}
	}

	groupedSingle := map[int][]drawing.MysekaiTalkFixtures{}
	for key, item := range singleReads {
		if item.total == item.read {
			continue
		}
		fixtureIDs := parseIntTokens(key)
		if len(fixtureIDs) == 0 {
			continue
		}
		mainGenreID := intNumber(fixtureMap[fixtureIDs[0]]["mysekaiFixtureMainGenreId"], 0)
		fixtures := make([]drawing.MysekaiFixture, 0, len(fixtureIDs))
		for _, fixtureID := range fixtureIDs {
			fixture := fixtureMap[fixtureID]
			fixtures = append(fixtures, drawing.MysekaiFixture{
				ID:        fixtureID,
				ImagePath: fixtureThumbnailPath(func(p string) string { return c.regionPath(region, p) }, fixture),
				Obtained:  hasFixture(obtainedFixtureIDs, fixtureID),
			})
		}
		groupedSingle[mainGenreID] = append(groupedSingle[mainGenreID], drawing.MysekaiTalkFixtures{
			Fixtures:  fixtures,
			NoreadNum: item.total - item.read,
		})
	}

	mainGenreIDs := make([]int, 0, len(groupedSingle))
	for mainGenreID := range groupedSingle {
		mainGenreIDs = append(mainGenreIDs, mainGenreID)
	}
	sort.Ints(mainGenreIDs)

	singleMainGenres := make([]drawing.MysekaiSingleTalkMainGenre, 0, len(mainGenreIDs))
	for _, mainGenreID := range mainGenreIDs {
		info := mainGenreMap[mainGenreID]
		singleMainGenres = append(singleMainGenres, drawing.MysekaiSingleTalkMainGenre{
			Name:      stringValue(info["name"]),
			ImagePath: c.regionPath(region, fmt.Sprintf("mysekai/icon/category_icon/%s.png", stringValue(info["assetbundleName"]))),
			SubGenres: [][]drawing.MysekaiTalkFixtures{groupedSingle[mainGenreID]},
		})
	}

	totalTalks := 0
	totalReads := 0
	for _, item := range singleReads {
		totalTalks += item.total
		totalReads += item.read
	}

	multiReads := make([]drawing.MysekaiTalkFixtures, 0, len(multiReadsMap))
	for key, item := range multiReadsMap {
		totalTalks += item.total
		totalReads += item.read
		if item.total == item.read {
			continue
		}
		fixtureIDs := parseIntTokens(key)
		if len(fixtureIDs) == 0 {
			continue
		}

		iconGroups := make([][]string, 0, len(item.cuidsSet))
		for _, cuids := range item.cuidsSet {
			icons := make([]string, 0, len(cuids))
			for _, cuid := range cuids {
				icons = append(icons, c.staticPath(fmt.Sprintf("chara_icon/%s.png", charaIconName(cuid))))
			}
			iconGroups = append(iconGroups, icons)
		}

		fixtures := make([]drawing.MysekaiFixture, 0, len(fixtureIDs))
		for _, fixtureID := range fixtureIDs {
			fixture := fixtureMap[fixtureID]
			fixtures = append(fixtures, drawing.MysekaiFixture{
				ID:        fixtureID,
				ImagePath: fixtureThumbnailPath(func(p string) string { return c.regionPath(region, p) }, fixture),
				Obtained:  hasFixture(obtainedFixtureIDs, fixtureID),
			})
		}
		multiReads = append(multiReads, drawing.MysekaiTalkFixtures{
			Fixtures:            fixtures,
			NoreadNum:           item.total - item.read,
			CharacterIDs:        item.cuidsSet,
			CharaIconPathGroups: iconGroups,
		})
	}
	sort.SliceStable(multiReads, func(i, j int) bool {
		if len(multiReads[i].Fixtures) != len(multiReads[j].Fixtures) {
			return len(multiReads[i].Fixtures) > len(multiReads[j].Fixtures)
		}
		if len(multiReads[i].Fixtures) == 0 || len(multiReads[j].Fixtures) == 0 {
			return false
		}
		return multiReads[i].Fixtures[0].ID < multiReads[j].Fixtures[0].ID
	})

	var progressMessage string
	var promptMessage *string
	if showAllTalks {
		progressMessage = fmt.Sprintf("对话家具列表 - 共 %d 条对话", totalTalks)
	} else {
		progressMessage = fmt.Sprintf("未读对话家具列表 - 进度: %d/%d (%.1f%%)", totalReads, totalTalks, percent(totalReads, totalTalks))
		prompt := "*仅展示未读对话家具，灰色表示未获得蓝图"
		promptMessage = &prompt
	}
	return &drawing.MysekaiTalkListRequest{
		Profile:          c.mysekaiProfileCard(region, merged, query.Profile),
		SdImagePath:      c.regionPath(region, fmt.Sprintf("character/character_sd_l/chr_sp_%d.png", characterUnitID)),
		ProgressMessage:  &progressMessage,
		PromptMessage:    promptMessage,
		ShowID:           true,
		SingleMainGenres: singleMainGenres,
		MultiReads:       multiReads,
	}, nil
}

func (c *Controller) RenderTalkList(query TalkListQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildTalkListRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiTalkList(payload)
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

func (c *Controller) fixtureCostMaterials(region renderregion.Value, blueprintID int, costs []map[string]interface{}) []drawing.MysekaiFixtureMaterial {
	var result []drawing.MysekaiFixtureMaterial
	iconMap := c.loadIconNameMap("mysekaiMaterials.json", "iconAssetbundleName")
	for _, item := range costs {
		if intNumber(item["mysekaiBlueprintId"], 0) != blueprintID {
			continue
		}
		materialID := intNumber(item["mysekaiMaterialId"], 0)
		icon := iconMap[materialID]
		if icon == "" {
			continue
		}
		result = append(result, drawing.MysekaiFixtureMaterial{
			ImagePath: c.regionPath(region, fmt.Sprintf("mysekai/thumbnail/material/%s.png", icon)),
			Quantity:  intNumber(item["quantity"], 0),
		})
	}
	return result
}

func (c *Controller) fixtureRecycleMaterials(region renderregion.Value, fixtureID int, items []map[string]interface{}) []drawing.MysekaiFixtureMaterial {
	var result []drawing.MysekaiFixtureMaterial
	iconMap := c.loadIconNameMap("mysekaiMaterials.json", "iconAssetbundleName")
	for _, item := range items {
		if intNumber(item["mysekaiFixtureId"], 0) != fixtureID {
			continue
		}
		materialID := intNumber(item["mysekaiMaterialId"], 0)
		icon := iconMap[materialID]
		if icon == "" {
			continue
		}
		result = append(result, drawing.MysekaiFixtureMaterial{
			ImagePath: c.regionPath(region, fmt.Sprintf("mysekai/thumbnail/material/%s.png", icon)),
			Quantity:  intNumber(item["quantity"], 0),
		})
	}
	return result
}

func (c *Controller) fixtureReactionCharacterGroups(fixtureID int) []drawing.MysekaiReactionCharacterGroups {
	var parsed map[string]interface{}
	if !c.masterdata.loadObject("mysekai/system/fixture_reaction_data/fixture_reaction_data.json", &parsed) {
		return nil
	}

	rawItems, _ := parsed["FixturerRactions"].([]interface{})
	grouped := map[int][][]int{}
	for _, raw := range rawItems {
		item, ok := raw.(map[string]interface{})
		if !ok || intNumber(item["FixtureId"], 0) != fixtureID {
			continue
		}
		reactions, _ := item["ReactionCharacter"].([]interface{})
		for _, rawReaction := range reactions {
			entry, ok := rawReaction.(map[string]interface{})
			if !ok {
				continue
			}
			characters, _ := entry["CharacterUnitIds"].([]interface{})
			var characterIDs []int
			for _, character := range characters {
				id := intNumber(character, 0)
				if id != 0 {
					characterIDs = append(characterIDs, id)
				}
			}
			if len(characterIDs) > 0 {
				grouped[len(characterIDs)] = append(grouped[len(characterIDs)], characterIDs)
			}
		}
	}

	counts := make([]int, 0, len(grouped))
	for count := range grouped {
		counts = append(counts, count)
	}
	sort.Ints(counts)

	result := make([]drawing.MysekaiReactionCharacterGroups, 0, len(counts))
	for _, count := range counts {
		iconGroups := make([][]string, 0, len(grouped[count]))
		for _, ids := range grouped[count] {
			icons := make([]string, 0, len(ids))
			for _, id := range ids {
				icons = append(icons, c.staticPath(fmt.Sprintf("chara_icon/%s.png", charaIconName(id))))
			}
			iconGroups = append(iconGroups, icons)
		}
		result = append(result, drawing.MysekaiReactionCharacterGroups{
			Number:                count,
			CharacterUintIDGroups: grouped[count],
			CharaIconPathGroups:   iconGroups,
		})
	}
	return result
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
