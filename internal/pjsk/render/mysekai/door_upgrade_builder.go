package mysekai

import (
	"fmt"
	"sort"

	"haruki-cloud/utils/drawing"
)

// BuildDoorUpgradeRequest builds the request for rendering MySekai door upgrade view.
func (c *Controller) BuildDoorUpgradeRequest(query DoorUpgradeQuery) (*drawing.MysekaiDoorUpgradeRequest, error) {
	c = c.withRegion(query.Region)
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
		item, _ := raw.(map[string]any)
		userMaterials[intNumber(item["mysekaiMaterialId"], 0)] = intNumber(item["quantity"], 0)
	}

	specLevels := map[int]int{}
	for _, raw := range nestedList(merged, "userMysekaiGates") {
		item, _ := raw.(map[string]any)
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
		Profile:       c.mysekaiProfileCard(region, merged, query.Profile, false),
		GateMaterials: gateMaterials,
	}, nil
}

// RenderDoorUpgrade renders the MySekai door upgrade view.
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
