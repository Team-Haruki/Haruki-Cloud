package mysekai

import (
	"fmt"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

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
