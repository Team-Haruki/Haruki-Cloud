package mysekai

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
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

	siteOrder := resolveMysekaiMapSiteIDs(query.MapIDs)
	if len(siteOrder) == 0 {
		return nil, fmt.Errorf("mysekai map query contains no valid map ids")
	}
	harvestMapsBySite := indexMysekaiHarvestMaps(merged)
	assets := c.loadMysekaiMapAssets()
	maps := make([]drawing.MysekaiMsrMapData, 0, len(siteOrder))
	for _, siteID := range siteOrder {
		if site := c.buildMysekaiMapSite(region, merged, siteID, harvestMapsBySite[siteID], assets); site != nil {
			maps = append(maps, *site)
		}
	}

	if len(maps) == 0 {
		return nil, fmt.Errorf("mysekai map contains no harvest map data")
	}

	return &drawing.MysekaiMsrMapRequest{
		Maps:                 maps,
		ShowHarvested:        query.ShowHarvested != nil && *query.ShowHarvested,
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

type mysekaiMapAssets struct {
	materials       map[int]string
	materialRarity  map[int]string
	items           map[int]string
	fixtures        map[int]string
	musicRecords    map[int]string
	harvestFixtures map[int]map[string]any
	characters      map[int]map[string]any
}

func (c *Controller) loadMysekaiMapAssets() mysekaiMapAssets {
	return mysekaiMapAssets{
		materials:       c.loadIconNameMap("mysekaiMaterials.json", "iconAssetbundleName"),
		materialRarity:  c.loadFieldMap("mysekaiMaterials.json", "mysekaiMaterialRarityType"),
		items:           c.loadIconNameMap("mysekaiItems.json", "iconAssetbundleName"),
		fixtures:        c.loadIconNameMap("mysekaiFixtures.json", "assetbundleName"),
		musicRecords:    c.loadMusicRecordJacketMap(),
		harvestFixtures: c.masterdata.loadMapByID("mysekaiSiteHarvestFixtures.json"),
		characters:      c.masterdata.loadMapByID("gameCharacters.json"),
	}
}

func indexMysekaiHarvestMaps(merged map[string]any) map[int]map[string]any {
	result := make(map[int]map[string]any, 4)
	for _, raw := range nestedList(merged, "userMysekaiHarvestMaps") {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if siteID := intNumber(item["mysekaiSiteId"], 0); siteID != 0 {
			result[siteID] = item
		}
	}
	return result
}

func (c *Controller) buildMysekaiMapSite(region renderregion.Value, merged map[string]any, siteID int, siteMap map[string]any, assets mysekaiMapAssets) *drawing.MysekaiMsrMapData {
	config, configured := mysekaiMapSiteConfigs[siteID]
	if len(siteMap) == 0 || !configured {
		return nil
	}
	site := drawing.MysekaiMsrMapSiteInfo{
		ImagePath: c.staticPath(fmt.Sprintf("mysekai/site/%s.png", config.SiteImageName)),
		GridSize:  config.GridSize, OffsetX: config.OffsetX, OffsetZ: config.OffsetZ,
		DirX: config.DirX, DirZ: config.DirZ, RevXZ: config.RevXZ, Scale: 0.8,
	}
	if len(config.CropBBox) > 0 {
		site.CropBbox = slices.Clone(config.CropBBox)
	}
	rawDrops, _ := siteMap["userMysekaiSiteHarvestResourceDrops"].([]any)
	rawPoints, _ := siteMap["userMysekaiSiteHarvestFixtures"].([]any)
	return &drawing.MysekaiMsrMapData{
		MapID: siteID, Site: site,
		HarvestPoints: c.buildMysekaiMapHarvestPoints(region, rawPoints, rawDrops, assets),
		ResourceDrops: c.buildMapResourceDrops(region, merged, rawDrops, assets.materials, assets.materialRarity, assets.items, assets.fixtures, assets.musicRecords),
	}
}

func birthdayCharactersByHarvestPosition(rawDrops []any) map[string]int {
	result := make(map[string]int)
	for _, raw := range rawDrops {
		drop, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		resourceID := intNumber(drop["resourceId"], intNumber(drop["id"], 0))
		position := mysekaiHarvestPosKey(floatNumber(drop["positionX"], floatNumber(drop["position_x"], 0)), floatNumber(drop["positionZ"], floatNumber(drop["position_z"], 0)))
		if resourceID >= 174 && resourceID <= 199 && position != "" && result[position] == 0 {
			result[position] = resourceID - 173
		}
	}
	return result
}

func (c *Controller) buildMysekaiMapHarvestPoints(region renderregion.Value, rawPoints, rawDrops []any, assets mysekaiMapAssets) []drawing.MysekaiMsrMapHarvestPoint {
	birthdayCharacters := birthdayCharactersByHarvestPosition(rawDrops)
	result := make([]drawing.MysekaiMsrMapHarvestPoint, 0, len(rawPoints))
	for _, raw := range rawPoints {
		point, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if built := c.buildMysekaiMapHarvestPoint(region, point, birthdayCharacters, assets); built != nil {
			result = append(result, *built)
		}
	}
	return result
}

func (c *Controller) buildMysekaiMapHarvestPoint(region renderregion.Value, point map[string]any, birthdayCharacters map[string]int, assets mysekaiMapAssets) *drawing.MysekaiMsrMapHarvestPoint {
	fixtureID := intNumber(point["mysekaiSiteHarvestFixtureId"], 0)
	meta := assets.harvestFixtures[fixtureID]
	rarityType := stringValue(meta["mysekaiSiteHarvestFixtureRarityType"])
	assetbundleName := stringValue(meta["assetbundleName"])
	fixtureType := stringValue(meta["mysekaiSiteHarvestFixtureType"])
	if rarityType == "" || assetbundleName == "" || isToneGustHarvestFixture(fixtureType, assetbundleName) {
		return nil
	}
	positionX := floatNumber(point["positionX"], floatNumber(point["position_x"], 0))
	positionZ := floatNumber(point["positionZ"], floatNumber(point["position_z"], 0))
	imagePath, fallback, size, offsetX, offsetZ := c.mysekaiHarvestPointImage(region, fixtureType, rarityType, assetbundleName, positionX, positionZ, birthdayCharacters, assets.characters)
	return &drawing.MysekaiMsrMapHarvestPoint{
		ID: positiveIntPointer(fixtureID), ImagePath: imagePath, FallbackImagePath: fallback,
		PositionX: positionX, PositionZ: positionZ, Status: mysekaiHarvestPointStatus(point),
		Size: size, OffsetX: offsetX, OffsetZ: offsetZ,
	}
}

func isToneGustHarvestFixture(fixtureType, assetbundleName string) bool {
	lowerAssetbundle := strings.ToLower(assetbundleName)
	return fixtureType == "tone_gust" || lowerAssetbundle == "tone_gust" || strings.Contains(lowerAssetbundle, "tone_gust")
}

func positiveIntPointer(value int) *int {
	if value <= 0 {
		return nil
	}
	return new(value)
}

func mysekaiHarvestPointStatus(point map[string]any) string {
	status := stringValue(point["userMysekaiSiteHarvestFixtureStatus"])
	if status == "" {
		status = stringValue(point["mysekaiSiteHarvestFixtureStatus"])
	}
	if status == "" {
		return "spawned"
	}
	return status
}

func (c *Controller) mysekaiHarvestPointImage(region renderregion.Value, fixtureType, rarityType, assetbundleName string, positionX, positionZ float64, birthdayCharacters map[string]int, characters map[int]map[string]any) (string, *string, *int, float64, float64) {
	imageRelPath := fmt.Sprintf("mysekai/harvest_fixture_icon/%s/%s.png", rarityType, assetbundleName)
	if fixtureType != "birthday_plant" {
		return c.staticPath(imageRelPath), nil, nil, 0, -48
	}
	fallback := new(c.staticPath("mysekai/harvest_fixture_icon/rarity_1/mdl_site_wood_common_fieldtree01.png"))
	characterID := birthdayCharacters[mysekaiHarvestPosKey(positionX, positionZ)]
	if characterID > 0 {
		if birthdayPath := c.resolveMysekaiBirthdayRefreshIconPath(region, characters[characterID], time.Now()); birthdayPath != "" {
			imageRelPath = birthdayPath
		}
	}
	if strings.HasPrefix(imageRelPath, "asset/") {
		return imageRelPath, fallback, new(50), 7.5, 0
	}
	return c.staticPath(imageRelPath), fallback, new(50), 7.5, 0
}

// HasRemainingHarvestResources reports whether the current map request contains
// visible resource drops before asking the drawing service to render it.
func (c *Controller) HasRemainingHarvestResources(query MapQuery) (bool, error) {
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	payload, err := c.BuildMapRequest(query)
	finishBuild()
	if err != nil {
		return false, err
	}
	return MapRequestHasRemainingHarvestResources(payload), nil
}

// MapRequestHasRemainingHarvestResources reports whether an already-built map
// request contains a visible resource drop.
func MapRequestHasRemainingHarvestResources(payload *drawing.MysekaiMsrMapRequest) bool {
	if payload == nil {
		return false
	}
	for _, site := range payload.Maps {
		for _, drop := range site.ResourceDrops {
			if !drop.Hide {
				return true
			}
		}
	}
	return false
}

// RenderMapRequest renders a map request that has already been built.
func (c *Controller) RenderMapRequest(payload *drawing.MysekaiMsrMapRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	if payload == nil {
		return nil, fmt.Errorf("mysekai map request is nil")
	}
	return c.drawing.GenerateMysekaiMap(payload)
}

// RenderMap renders the MySekai map view.
func (c *Controller) RenderMap(query MapQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	payload, err := c.BuildMapRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.RenderMapRequest(payload)
}
