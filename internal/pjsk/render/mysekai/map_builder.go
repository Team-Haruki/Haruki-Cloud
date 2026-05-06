package mysekai

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
)

var (
	mysekaiMapRareOutlineColor      = []int{255, 50, 50, 150}
	mysekaiMapSmallIconOutlineColor = []int{50, 50, 255, 100}
)

const (
	mysekaiMapSpawnSize          = 20
	mysekaiMapLargeIconSize      = 35
	mysekaiMapSmallIconSize      = 17
	mysekaiMapIconZOffset        = -32
	mysekaiMapRareLargeLightSize = 315
	mysekaiMapRareSmallLightSize = 225
)

// BuildMapRequest builds the request for rendering MySekai map view.
func (c *Controller) BuildMapRequest(query MapQuery) (*drawing.MysekaiMsrMapRequest, error) {
	c = c.withRegion(query.Region)
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}

	showHarvested := false
	if query.ShowHarvested != nil {
		showHarvested = *query.ShowHarvested
	}

	harvestMapsBySite := make(map[int]map[string]any, 4)
	for _, raw := range nestedList(merged, "userMysekaiHarvestMaps") {
		item, ok := raw.(map[string]any)
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
			site.CropBbox = slices.Clone(config.CropBBox)
		}

		harvestPoints := make([]drawing.MysekaiMsrMapHarvestPoint, 0, 16)
		characterMap := c.masterdata.loadMapByID("gameCharacters.json")
		birthdayCharacterByPos := make(map[string]int)
		rawDrops, _ := siteMap["userMysekaiSiteHarvestResourceDrops"].([]any)
		for _, raw := range rawDrops {
			drop, ok := raw.(map[string]any)
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

		rawHarvestPoints, _ := siteMap["userMysekaiSiteHarvestFixtures"].([]any)
		for _, raw := range rawHarvestPoints {
			point, ok := raw.(map[string]any)
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
				pointID = new(fixtureID)
			}

			positionX := floatNumber(point["positionX"], floatNumber(point["position_x"], 0))
			positionZ := floatNumber(point["positionZ"], floatNumber(point["position_z"], 0))
			posKey := mysekaiHarvestPosKey(positionX, positionZ)

			imageRelPath := fmt.Sprintf("mysekai/harvest_fixture_icon/%s/%s.png", rarityType, assetbundleName)
			var fallbackImagePath *string
			var size *int
			var offsetX float64
			offsetZ := -48.0
			if fixtureType == "birthday_plant" {
				fallbackImagePath = new(c.staticPath("mysekai/harvest_fixture_icon/rarity_1/mdl_site_wood_common_fieldtree01.png"))
				if characterID := birthdayCharacterByPos[posKey]; characterID > 0 {
					if birthdayPath := c.resolveMysekaiBirthdayRefreshIconPath(region, characterMap[characterID], time.Now()); birthdayPath != "" {
						imageRelPath = birthdayPath
					}
				}
				size = new(50)
				offsetX = 7.5
				offsetZ = 0
			}

			imagePath := c.staticPath(imageRelPath)
			if strings.HasPrefix(imageRelPath, "asset/") {
				imagePath = imageRelPath
			}

			harvestPoints = append(harvestPoints, drawing.MysekaiMsrMapHarvestPoint{
				ID:                pointID,
				ImagePath:         imagePath,
				FallbackImagePath: fallbackImagePath,
				PositionX:         positionX,
				PositionZ:         positionZ,
				Status:            status,
				Size:              size,
				OffsetX:           offsetX,
				OffsetZ:           offsetZ,
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
		Maps:                 maps,
		ShowHarvested:        showHarvested,
		PhenomenaGroundColor: c.currentMysekaiPhenomenaGroundColor(region, merged),
		SpawnImagePath: drawing.StringPtr(
			c.staticPath("mysekai/mark.png"),
		),
		SpawnSize:          mysekaiMapSpawnSize,
		RareLightImagePath: drawing.StringPtr(c.staticPath("mysekai/light.png")),
		LargeIconSize:      mysekaiMapLargeIconSize,
		SmallIconSize:      mysekaiMapSmallIconSize,
		IconZOffset:        mysekaiMapIconZOffset,
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
