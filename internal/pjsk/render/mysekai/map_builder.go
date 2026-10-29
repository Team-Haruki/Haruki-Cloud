package mysekai

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/utils/usererror"
)

var (
	mysekaiMapRareOutlineColor      = []int{255, 50, 50, 150}
	mysekaiMapSmallIconOutlineColor = []int{50, 50, 255, 100}
	mysekaiMapRarePointOutlineColor = []int{255, 50, 50, 230}
)

const (
	mysekaiMapSpawnSize             = 20
	mysekaiMapLargeIconSize         = 35
	mysekaiMapSmallIconSize         = 17
	mysekaiMapIconZOffset           = -32
	mysekaiMapRareLargeLightSize    = 315
	mysekaiMapRareSmallLightSize    = 225
	mysekaiMapRarePointOutlineWidth = 3
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
		return nil, usererror.Invalid(i18n.M("mysekai.map.id_invalid"))
	}
	harvestMapsBySite := indexMysekaiHarvestMaps(merged)
	assets := c.loadMysekaiMapAssets()
	assets.highlightMaterials = intSet(query.HighlightMaterialIDs)
	assets.activeBirthdayParty = activeMysekaiBirthdayParty(assets.birthdayParties, mysekaiMapSnapshotTimeMs(merged))
	maps := make([]drawing.MysekaiMsrMapData, 0, len(siteOrder))
	for _, siteID := range siteOrder {
		if site := c.buildMysekaiMapSite(region, merged, siteID, harvestMapsBySite[siteID], assets); site != nil {
			maps = append(maps, *site)
		}
	}

	if len(maps) == 0 {
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("mysekai.map.no_data"))
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
	// birthdayDeliveries maps a birthday party delivery material ID (the
	// dewdrop dropped by birthday plants) to its game character ID.
	birthdayDeliveries map[int]int
	// birthdayPlantAssets maps a birthday_plant harvest fixture ID to the
	// party assetbundle (e.g. haruka_2026) that holds its refresh icon.
	birthdayPlantAssets map[int]string
	// birthdayParties are the region's party windows.
	birthdayParties []mysekaiBirthdayPartyWindow
	// activeBirthdayParty is the assetbundle of the only party whose window
	// holds the snapshot time, the last resort for an unbound plant.
	activeBirthdayParty string
	// highlightMaterials are mysekai_material IDs drawn as rare on this map.
	highlightMaterials map[int]struct{}
}

func (c *Controller) loadMysekaiMapAssets() mysekaiMapAssets {
	harvestFixtures := c.masterdata.loadMapByID("mysekaiSiteHarvestFixtures.json")
	parties := c.loadMysekaiBirthdayParties(harvestFixtures)
	return mysekaiMapAssets{
		birthdayDeliveries:  parties.deliveries,
		birthdayPlantAssets: parties.plantAssets,
		birthdayParties:     parties.windows,
		materials:           c.loadIconNameMap("mysekaiMaterials.json", "iconAssetbundleName"),
		materialRarity:      c.loadFieldMap("mysekaiMaterials.json", "mysekaiMaterialRarityType"),
		items:               c.loadIconNameMap("mysekaiItems.json", "iconAssetbundleName"),
		fixtures:            c.loadIconNameMap("mysekaiFixtures.json", "assetbundleName"),
		musicRecords:        c.loadMusicRecordJacketMap(),
		harvestFixtures:     harvestFixtures,
		characters:          c.masterdata.loadMapByID("gameCharacters.json"),
	}
}

// mysekaiBirthdayPlantFixtureBase is the offset of a party's plant fixture:
// every party that names its mysekaiSiteHarvestFixtureId (jp 28, tw 16, cn 15,
// kr 15 on 2026-10-30) uses 8000 + its own ID.
const mysekaiBirthdayPlantFixtureBase = 8000

type mysekaiBirthdayPartyWindow struct {
	startAt         int64
	closedAt        int64
	assetbundleName string
}

type mysekaiBirthdayPartyIndex struct {
	// deliveries maps a delivery material ID to its game character ID.
	deliveries map[int]int
	// plantAssets maps a birthday_plant harvest fixture ID to its party's
	// assetbundle.
	plantAssets map[int]string
	windows     []mysekaiBirthdayPartyWindow
}

// loadMysekaiBirthdayParties indexes birthdayParties.json. Each yearly party
// has its own delivery material and harvest fixture (Haruka 2025 drops 179 from
// fixture 8001, Haruka 2026 drops 297 from fixture 8027), so neither can be
// derived from a fixed ID range. EN's parties carry no
// mysekaiSiteHarvestFixtureId at all; for a party without it the plant is
// 8000 + the party ID, taken only when the region's harvest fixtures hold a
// birthday_plant with that ID. A named fixture always wins over a derived one.
func (c *Controller) loadMysekaiBirthdayParties(harvestFixtures map[int]map[string]any) mysekaiBirthdayPartyIndex {
	parties := c.masterdata.loadMapByID("birthdayParties.json")
	index := mysekaiBirthdayPartyIndex{
		deliveries:  make(map[int]int, len(parties)),
		plantAssets: make(map[int]string, len(parties)),
		windows:     make([]mysekaiBirthdayPartyWindow, 0, len(parties)),
	}
	derived := make(map[int]string)
	for id, party := range parties {
		characterID := c.gameCharacterIDByUnitID(intNumber(party["gameCharacterUnitId"], 0))
		if materialID := intNumber(party["deliveryItemMaterialId"], 0); materialID > 0 && characterID > 0 {
			index.deliveries[materialID] = characterID
		}
		assetbundleName := strings.TrimSpace(stringValue(party["assetbundleName"]))
		if assetbundleName == "" {
			continue
		}
		index.windows = append(index.windows, mysekaiBirthdayPartyWindow{
			startAt:         int64Number(party["startAt"], 0),
			closedAt:        int64Number(party["closedAt"], 0),
			assetbundleName: assetbundleName,
		})
		if fixtureID := intNumber(party["mysekaiSiteHarvestFixtureId"], 0); fixtureID > 0 {
			index.plantAssets[fixtureID] = assetbundleName
			continue
		}
		fixtureID := mysekaiBirthdayPlantFixtureBase + id
		if id > 0 && stringValue(harvestFixtures[fixtureID]["mysekaiSiteHarvestFixtureType"]) == "birthday_plant" {
			derived[fixtureID] = assetbundleName
		}
	}
	for fixtureID, assetbundleName := range derived {
		if _, named := index.plantAssets[fixtureID]; !named {
			index.plantAssets[fixtureID] = assetbundleName
		}
	}
	return index
}

// mysekaiMapSnapshotTimeMs is the time the snapshot describes, or now when it
// carries none.
func mysekaiMapSnapshotTimeMs(merged map[string]any) int64 {
	if snapshotMs := resolveMysekaiSnapshotTimeMs(merged); snapshotMs > 0 {
		return snapshotMs
	}
	return time.Now().UnixMilli()
}

// activeMysekaiBirthdayParty returns the assetbundle of the party whose
// [startAt, closedAt] holds atMs. Overlapping parties (Rin and Len share a
// birthday) are ambiguous, so it returns "" unless exactly one matches.
func activeMysekaiBirthdayParty(windows []mysekaiBirthdayPartyWindow, atMs int64) string {
	active := ""
	for _, window := range windows {
		if window.startAt <= 0 || window.closedAt < window.startAt || atMs < window.startAt || atMs > window.closedAt {
			continue
		}
		if active != "" {
			return ""
		}
		active = window.assetbundleName
	}
	return active
}

func intSet(values []int) map[int]struct{} {
	if len(values) == 0 {
		return nil
	}
	result := make(map[int]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
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
	resourceDrops := c.buildMapResourceDrops(region, merged, rawDrops, assets)
	harvestPoints := c.buildMysekaiMapHarvestPoints(region, rawPoints, rawDrops, assets)
	outlineRareHarvestPoints(harvestPoints, resourceDrops, assets)
	return &drawing.MysekaiMsrMapData{
		MapID: siteID, Site: site,
		HarvestPoints: harvestPoints,
		ResourceDrops: resourceDrops,
	}
}

// outlineRareHarvestPoints outlines in red every harvest point that still holds
// a visible, unharvested rare or highlighted drop. Highlighted materials keep
// their own icon plain; only the point is marked. Birthday deliveries are
// skipped: their plants are distinct already.
func outlineRareHarvestPoints(points []drawing.MysekaiMsrMapHarvestPoint, drops []drawing.MysekaiMsrMapResourceDrop, assets mysekaiMapAssets) {
	rarePositions := make(map[string]struct{})
	for _, drop := range drops {
		if drop.Hide || drop.Status != "before_drop" || mysekaiIsBirthdayDrop(drop.Type, drop.ID, assets.birthdayDeliveries) {
			continue
		}
		_, highlighted := assets.highlightMaterials[drop.ID]
		if drop.Rarity >= 2 || (highlighted && drop.Type == "mysekai_material") {
			rarePositions[mysekaiHarvestPosKey(drop.PositionX, drop.PositionZ)] = struct{}{}
		}
	}
	for i := range points {
		if _, ok := rarePositions[mysekaiHarvestPosKey(points[i].PositionX, points[i].PositionZ)]; ok {
			points[i].OutlineColor = slices.Clone(mysekaiMapRarePointOutlineColor)
			points[i].OutlineWidth = drawing.IntPtr(mysekaiMapRarePointOutlineWidth)
		}
	}
}

func birthdayCharactersByHarvestPosition(rawDrops []any, deliveries map[int]int) map[string]int {
	result := make(map[string]int)
	for _, raw := range rawDrops {
		drop, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		resourceID := intNumber(drop["resourceId"], intNumber(drop["id"], 0))
		position := mysekaiHarvestPosKey(floatNumber(drop["positionX"], floatNumber(drop["position_x"], 0)), floatNumber(drop["positionZ"], floatNumber(drop["position_z"], 0)))
		if characterID := birthdayDeliveryCharacter(resourceID, deliveries); characterID > 0 && position != "" && result[position] == 0 {
			result[position] = characterID
		}
	}
	return result
}

func (c *Controller) buildMysekaiMapHarvestPoints(region renderregion.Value, rawPoints, rawDrops []any, assets mysekaiMapAssets) []drawing.MysekaiMsrMapHarvestPoint {
	birthdayCharacters := birthdayCharactersByHarvestPosition(rawDrops, assets.birthdayDeliveries)
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
	parties := mysekaiPlantParties{bound: assets.birthdayPlantAssets[fixtureID], active: assets.activeBirthdayParty}
	imagePath, fallback, size, offsetX, offsetZ := c.mysekaiHarvestPointImage(region, fixtureType, rarityType, assetbundleName, parties, positionX, positionZ, birthdayCharacters, assets.characters)
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

// mysekaiPlantParties names the party icons a birthday plant can use.
type mysekaiPlantParties struct {
	// bound is the party the plant fixture belongs to.
	bound string
	// active is the only party running at the snapshot time.
	active string
}

func (c *Controller) mysekaiHarvestPointImage(region renderregion.Value, fixtureType, rarityType, assetbundleName string, parties mysekaiPlantParties, positionX, positionZ float64, birthdayCharacters map[string]int, characters map[int]map[string]any) (drawing.AssetKey, *string, *int, float64, float64) {
	imageRelPath := fmt.Sprintf("mysekai/harvest_fixture_icon/%s/%s.png", rarityType, assetbundleName)
	if fixtureType != "birthday_plant" {
		return drawing.AssetPath(c.staticPath(imageRelPath)), nil, nil, 0, -48
	}
	fallback := new(c.staticPath("mysekai/harvest_fixture_icon/rarity_1/mdl_site_wood_common_fieldtree01.png"))
	// The party bound to this fixture names its own icon directory; the
	// character's year window stays behind it for regions lacking that party.
	// An unbound plant (no party maps to its fixture) also tries the party
	// running at the snapshot time.
	candidates := mysekaiBirthdayPartyIconCandidates(region.String(), parties.bound)
	appendCandidates := func(paths []string) {
		for _, candidate := range paths {
			if !slices.Contains(candidates, candidate) {
				candidates = append(candidates, candidate)
			}
		}
	}
	if characterID := birthdayCharacters[mysekaiHarvestPosKey(positionX, positionZ)]; characterID > 0 {
		imageName := mysekaiBirthdayCharacterImageName(characters[characterID])
		appendCandidates(mysekaiBirthdayIconCandidates(region.String(), imageName, time.Now()))
	}
	if parties.bound == "" {
		appendCandidates(mysekaiBirthdayPartyIconCandidates(region.String(), parties.active))
	}
	if len(candidates) > 0 {
		// C1: Drawing takes the first existing candidate and falls back to
		// fallback_image_path when none exists.
		return drawing.AssetCandidates(candidates...), fallback, new(50), 7.5, 0
	}
	return drawing.AssetPath(c.staticPath(imageRelPath)), fallback, new(50), 7.5, 0
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
	image, err := c.RenderMapRequestImage(payload)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.requestCtx)
}

func (c *Controller) RenderMapRequestImage(payload *drawing.MysekaiMsrMapRequest) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	if payload == nil {
		return drawing.ImageResult{}, fmt.Errorf("mysekai map request is nil")
	}
	return c.drawing.GenerateMysekaiMapImage(payload)
}

// RenderMap renders the MySekai map view.
func (c *Controller) RenderMap(query MapQuery) ([]byte, error) {
	image, err := c.RenderMapImage(query)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.requestCtx)
}

func (c *Controller) RenderMapImage(query MapQuery) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	payload, err := c.BuildMapRequest(query)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.RenderMapRequestImage(payload)
}
