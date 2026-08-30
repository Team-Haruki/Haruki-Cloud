package mysekai

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

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

type mysekaiResourceGroupFlags struct {
	hasMaterialDrop   bool
	hasFixtureDrop    bool
	isCottonFlower    bool
	isBirthdaySapling bool
}

// buildMapResourceDrops builds the resource drops list for a single map site.
func (c *Controller) buildMapResourceDrops(
	region renderregion.Value,
	merged map[string]any,
	rawDrops []any,
	materialMap, materialRarityMap, itemMap, fixtureMap, musicRecordMap map[int]string,
) []drawing.MysekaiMsrMapResourceDrop {
	groupedDrops := c.groupMapResourceDrops(
		region,
		merged,
		rawDrops,
		materialMap,
		materialRarityMap,
		itemMap,
		fixtureMap,
		musicRecordMap,
	)
	resourceDrops := make([]drawing.MysekaiMsrMapResourceDrop, 0, 32)
	for _, grouped := range groupedDrops {
		resourceDrops = append(resourceDrops, c.buildMapResourceGroup(grouped)...)
	}
	slices.SortFunc(resourceDrops, compareMapResourceDrops)
	return resourceDrops
}

func (c *Controller) groupMapResourceDrops(
	region renderregion.Value,
	merged map[string]any,
	rawDrops []any,
	materialMap, materialRarityMap, itemMap, fixtureMap, musicRecordMap map[int]string,
) map[string]map[string]*groupedMysekaiResourceDrop {
	groups := make(map[string]map[string]*groupedMysekaiResourceDrop)
	for _, raw := range rawDrops {
		posKey, resourceKey, drop, ok := c.parseMapResourceDrop(
			region,
			merged,
			raw,
			materialMap,
			materialRarityMap,
			itemMap,
			fixtureMap,
			musicRecordMap,
		)
		if !ok {
			continue
		}
		if groups[posKey] == nil {
			groups[posKey] = make(map[string]*groupedMysekaiResourceDrop)
		}
		existing := groups[posKey][resourceKey]
		if existing == nil {
			groups[posKey][resourceKey] = drop
			continue
		}
		mergeGroupedMapResourceDrop(existing, drop)
	}
	return groups
}

func (c *Controller) parseMapResourceDrop(
	region renderregion.Value,
	merged map[string]any,
	raw any,
	materialMap, materialRarityMap, itemMap, fixtureMap, musicRecordMap map[int]string,
) (string, string, *groupedMysekaiResourceDrop, bool) {
	drop, ok := raw.(map[string]any)
	if !ok {
		return "", "", nil, false
	}
	resourceType := stringValue(drop["resourceType"])
	if resourceType == "" {
		resourceType = stringValue(drop["type"])
	}
	resourceType = mysekaiNormalizeResourceType(resourceType)
	resourceID := intNumber(drop["resourceId"], intNumber(drop["id"], 0))
	if resourceType == "" || resourceID == 0 {
		return "", "", nil, false
	}

	resourceKey := fmt.Sprintf("%s_%d", resourceType, resourceID)
	imagePath, hasRecord := c.resourceImagePath(region, resourceKey, materialMap, itemMap, fixtureMap, musicRecordMap, merged)
	if imagePath == "" {
		return "", "", nil, false
	}

	positionX := floatNumber(drop["positionX"], floatNumber(drop["position_x"], 0))
	positionZ := floatNumber(drop["positionZ"], floatNumber(drop["position_z"], 0))
	posKey := mysekaiHarvestPosKey(positionX, positionZ)
	if posKey == "" {
		return "", "", nil, false
	}

	attachmentImagePath := c.mapResourceAttachment(hasRecord)
	return posKey, resourceKey, &groupedMysekaiResourceDrop{
		ID:                  resourceID,
		Type:                resourceType,
		ImagePath:           imagePath,
		PositionX:           positionX,
		PositionZ:           positionZ,
		Quantity:            normalizedMapResourceQuantity(drop),
		Status:              normalizedMapResourceStatus(drop),
		Rarity:              normalizedMapResourceRarity(resourceKey, materialRarityMap),
		AttachmentImagePath: attachmentImagePath,
	}, true
}

func normalizedMapResourceStatus(drop map[string]any) string {
	status := stringValue(drop["mysekaiSiteHarvestResourceDropStatus"])
	if status == "" {
		status = stringValue(drop["status"])
	}
	if status == "" {
		return "before_drop"
	}
	return status
}

func normalizedMapResourceQuantity(drop map[string]any) int {
	quantity := intNumber(drop["quantity"], 1)
	if quantity <= 0 {
		return 1
	}
	return quantity
}

func normalizedMapResourceRarity(resourceKey string, materialRarityMap map[int]string) int {
	rarity := resourceRarity(resourceKey, materialRarityMap)
	if rarity < 1 {
		return 1
	}
	return rarity
}

func (c *Controller) mapResourceAttachment(hasRecord bool) *string {
	if !hasRecord {
		return nil
	}
	return new(c.staticPath("mysekai/music_record.png"))
}

func mergeGroupedMapResourceDrop(current, additional *groupedMysekaiResourceDrop) {
	current.Quantity += additional.Quantity
	if current.AttachmentImagePath == nil {
		current.AttachmentImagePath = additional.AttachmentImagePath
	}
	if current.Status == "" {
		current.Status = additional.Status
	}
}

func (c *Controller) buildMapResourceGroup(
	grouped map[string]*groupedMysekaiResourceDrop,
) []drawing.MysekaiMsrMapResourceDrop {
	flags := analyzeMapResourceGroup(grouped)
	result := make([]drawing.MysekaiMsrMapResourceDrop, 0, len(grouped))
	for key, item := range grouped {
		applyMapResourceGroupFlags(key, item, flags)
		result = append(result, mapResourceDrawingDrop(key, item))
	}
	return result
}

func analyzeMapResourceGroup(grouped map[string]*groupedMysekaiResourceDrop) mysekaiResourceGroupFlags {
	flags := mysekaiResourceGroupFlags{}
	for key, item := range grouped {
		if (key == "mysekai_material_1" || key == "mysekai_material_6") && item.Quantity == 6 {
			item.Hide = true
		}
		if key == "mysekai_material_21" || key == "mysekai_material_22" {
			flags.isCottonFlower = true
		}
		if strings.HasPrefix(key, "mysekai_material_") {
			flags.hasMaterialDrop = true
		}
		if item.Type == "mysekai_fixture" {
			flags.hasFixtureDrop = true
		}
		if mysekaiIsBirthdayDrop(item.Type, item.ID) && item.Quantity > 16 {
			flags.isBirthdaySapling = true
		}
	}
	return flags
}

func applyMapResourceGroupFlags(
	key string,
	item *groupedMysekaiResourceDrop,
	flags mysekaiResourceGroupFlags,
) {
	if smallIcon, set := fixtureMaterialSmallIcon(key, item, flags); set {
		item.SmallIcon = new(smallIcon)
	}
	if flags.isCottonFlower && key != "mysekai_material_21" && key != "mysekai_material_22" {
		item.SmallIcon = new(true)
	}
	applyBirthdayDropFlags(item, flags.isBirthdaySapling)
}

func fixtureMaterialSmallIcon(
	key string,
	item *groupedMysekaiResourceDrop,
	flags mysekaiResourceGroupFlags,
) (bool, bool) {
	isMaterial := strings.HasPrefix(key, "mysekai_material_")
	if flags.hasFixtureDrop && flags.hasMaterialDrop {
		return !isMaterial, true
	}
	if flags.hasFixtureDrop {
		return item.Type != "mysekai_fixture", true
	}
	if !isMaterial && flags.hasMaterialDrop {
		return true, true
	}
	return false, false
}

func applyBirthdayDropFlags(item *groupedMysekaiResourceDrop, birthdaySapling bool) {
	isBirthdayDrop := mysekaiIsBirthdayDrop(item.Type, item.ID)
	if birthdaySapling {
		item.SmallIcon = new(!isBirthdayDrop)
		return
	}
	if isBirthdayDrop {
		item.Hide = true
	}
}

func mapResourceDrawingDrop(key string, item *groupedMysekaiResourceDrop) drawing.MysekaiMsrMapResourceDrop {
	outlineColor, outlineWidth := mapResourceOutline(item)
	return drawing.MysekaiMsrMapResourceDrop{
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
		OutlineColor:        outlineColor,
		OutlineWidth:        outlineWidth,
		LightSize:           mapResourceLightSize(key, item),
	}
}

func mapResourceOutline(item *groupedMysekaiResourceDrop) ([]int, *int) {
	if item.Rarity >= 2 {
		return slices.Clone(mysekaiMapRareOutlineColor), drawing.IntPtr(2)
	}
	if item.SmallIcon != nil && *item.SmallIcon {
		return slices.Clone(mysekaiMapSmallIconOutlineColor), drawing.IntPtr(1)
	}
	return nil, nil
}

func mapResourceLightSize(key string, item *groupedMysekaiResourceDrop) *int {
	if item.Rarity < 2 || strings.HasPrefix(key, "material_") {
		return nil
	}
	if item.SmallIcon != nil && *item.SmallIcon {
		return drawing.IntPtr(mysekaiMapRareSmallLightSize)
	}
	return drawing.IntPtr(mysekaiMapRareLargeLightSize)
}

func compareMapResourceDrops(a, b drawing.MysekaiMsrMapResourceDrop) int {
	if result := cmp.Compare(a.PositionX, b.PositionX); result != 0 {
		return result
	}
	if result := cmp.Compare(a.PositionZ, b.PositionZ); result != 0 {
		return result
	}
	if result := cmp.Compare(a.Type, b.Type); result != 0 {
		return result
	}
	return cmp.Compare(a.ID, b.ID)
}

// resolveMysekaiMapSiteIDs resolves the map site IDs from the query.
func resolveMysekaiMapSiteIDs(requested []int) []int {
	if len(requested) == 0 {
		return slices.Clone(mysekaiMapSiteOrder)
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
