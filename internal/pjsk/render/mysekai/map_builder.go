package mysekai

import (
	"fmt"
	"strings"
	"time"

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
