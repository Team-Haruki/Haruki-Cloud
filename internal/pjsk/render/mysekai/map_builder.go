package mysekai

import (
	"fmt"
	"strings"
	"time"

	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

// BuildMapRequest builds the request for rendering MySekai map view.
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

		resourceDrops := c.buildMapResourceDrops(region, merged, rawDrops, materialMap, materialRarityMap, itemMap, fixtureMap, musicRecordMap)

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

// buildMapResourceDrops builds the resource drops list for a single map site.
func (c *Controller) buildMapResourceDrops(
	region renderregion.Value,
	merged map[string]interface{},
	rawDrops []interface{},
	materialMap, materialRarityMap, itemMap, fixtureMap, musicRecordMap map[int]string,
) []drawing.MysekaiMsrMapResourceDrop {
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
				if hasMaterialDrop {
					smallIcon = !strings.HasPrefix(key, "mysekai_material_")
					smallIconSet = true
				} else if item.Type == "mysekai_fixture" {
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
	return resourceDrops
}

// RenderMap renders the MySekai map view.
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

// resolveMysekaiMapSiteIDs resolves the map site IDs from the query.
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
