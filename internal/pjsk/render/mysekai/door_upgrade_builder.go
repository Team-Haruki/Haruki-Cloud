package mysekai

import (
	"fmt"
	"sort"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

// BuildDoorUpgradeRequest builds the request for rendering MySekai door upgrade view.
func (c *Controller) BuildDoorUpgradeRequest(query DoorUpgradeQuery) (*drawing.MysekaiDoorUpgradeRequest, error) {
	c = c.withRegion(query.Region)
	showFull := query.ShowFull != nil && *query.ShowFull
	merged, region, err := c.prepareDoorUpgradeData(query.Region, showFull)
	if err != nil {
		return nil, err
	}
	specGateID := 0
	if ids := parseIntTokens(query.Query); len(ids) > 0 {
		specGateID = ids[0]
	}
	showAll := query.ShowAll != nil && *query.ShowAll
	userMaterials := doorUpgradeUserMaterials(merged, showFull)
	specLevels := doorUpgradeGateLevels(merged, showFull)
	gateTemp := c.loadDoorUpgradeGateMaterials()
	gateTemp, err = selectDoorUpgradeGates(gateTemp, specLevels, specGateID, showAll, showFull)
	if err != nil {
		return nil, err
	}
	materialIcons := c.loadIconNameMap("mysekaiMaterials.json", "iconAssetbundleName")
	gateMaterials := c.buildDoorUpgradeGates(region, gateTemp, specLevels, userMaterials, materialIcons, showFull)
	profile := c.doorUpgradeProfile(region, merged, query.Profile, showFull)
	return &drawing.MysekaiDoorUpgradeRequest{
		Profile:       profile,
		GateMaterials: gateMaterials,
	}, nil
}

type doorUpgradeMaterial struct {
	materialID int
	quantity   int
}

func (c *Controller) prepareDoorUpgradeData(region string, showFull bool) (map[string]any, renderregion.Value, error) {
	resolvedRegion := c.resolveRegion(region)
	if showFull {
		return map[string]any{}, resolvedRegion, c.ensureMasterdata()
	}
	return c.prepareSnapshot(region)
}

func doorUpgradeUserMaterials(merged map[string]any, showFull bool) map[int]int {
	materials := map[int]int{}
	if showFull {
		return materials
	}
	for _, raw := range nestedList(merged, "userMysekaiMaterials") {
		item, _ := raw.(map[string]any)
		materials[intNumber(item["mysekaiMaterialId"], 0)] = intNumber(item["quantity"], 0)
	}
	return materials
}

func doorUpgradeGateLevels(merged map[string]any, showFull bool) map[int]int {
	levels := map[int]int{}
	if showFull {
		return levels
	}
	for _, raw := range nestedList(merged, "userMysekaiGates") {
		item, _ := raw.(map[string]any)
		gateID := intNumber(item["mysekaiGateId"], 0)
		if gateID != 0 {
			levels[gateID] = intNumber(item["mysekaiGateLevel"], 0)
		}
	}
	return levels
}

// loadDoorUpgradeGateMaterials indexes mysekaiGateMaterialGroups by gate and
// level (groupId = gateId*1000 + level). Each gate's slice is sized to the
// highest level present in the region's master data, so the per-gate max
// level is len(gates[gateID]) — 40 on most regions, 70 on JP since 7.0.0.
func (c *Controller) loadDoorUpgradeGateMaterials() map[int][][]doorUpgradeMaterial {
	byLevel := map[int]map[int][]doorUpgradeMaterial{}
	maxLevels := map[int]int{}
	for _, item := range c.masterdata.loadList("mysekaiGateMaterialGroups.json") {
		groupID := intNumber(item["groupId"], 0)
		gateID, level := groupID/1000, groupID%1000
		if gateID <= 0 || level <= 0 {
			continue
		}
		if byLevel[gateID] == nil {
			byLevel[gateID] = map[int][]doorUpgradeMaterial{}
		}
		byLevel[gateID][level] = append(byLevel[gateID][level], doorUpgradeMaterial{
			materialID: intNumber(item["mysekaiMaterialId"], 0), quantity: intNumber(item["quantity"], 0),
		})
		maxLevels[gateID] = max(maxLevels[gateID], level)
	}
	gates := make(map[int][][]doorUpgradeMaterial, len(byLevel))
	for gateID, levels := range byLevel {
		materials := make([][]doorUpgradeMaterial, maxLevels[gateID])
		for level, items := range levels {
			materials[level-1] = items
		}
		gates[gateID] = materials
	}
	return gates
}

// doorUpgradeGateMaxLevel is the highest upgradable level known for a gate,
// or 0 when the master data has no upgrade materials for it.
func doorUpgradeGateMaxLevel(gates map[int][][]doorUpgradeMaterial, gateID int) int {
	return len(gates[gateID])
}

func doorUpgradeGateIsMax(gates map[int][][]doorUpgradeMaterial, gateID, level int) bool {
	maxLevel := doorUpgradeGateMaxLevel(gates, gateID)
	return maxLevel > 0 && level >= maxLevel
}

func selectDoorUpgradeGates(gates map[int][][]doorUpgradeMaterial, levels map[int]int, requestedID int, showAll, showFull bool) (map[int][][]doorUpgradeMaterial, error) {
	if requestedID == 0 && !showAll && !showFull {
		requestedID = lowestIncompleteDoorUpgradeGate(gates, levels)
	}
	if requestedID == 0 {
		return gates, nil
	}
	if !showFull && doorUpgradeGateIsMax(gates, requestedID, levels[requestedID]) {
		return nil, fmt.Errorf("queried gate already max level")
	}
	if materials, ok := gates[requestedID]; ok {
		return map[int][][]doorUpgradeMaterial{requestedID: materials}, nil
	}
	return gates, nil
}

// lowestIncompleteDoorUpgradeGate picks the user's highest-levelled gate that
// still has upgrade levels left in the region's master data. Gates without
// upgrade materials (e.g. the JP 7.0.0 shuffle gate 6) are never selected.
func lowestIncompleteDoorUpgradeGate(gates map[int][][]doorUpgradeMaterial, levels map[int]int) int {
	selectedID, selectedLevel := 0, 0
	for gateID, level := range levels {
		if doorUpgradeGateMaxLevel(gates, gateID) == 0 || doorUpgradeGateIsMax(gates, gateID, level) {
			continue
		}
		if level > selectedLevel {
			selectedID, selectedLevel = gateID, level
		}
	}
	return selectedID
}

func (c *Controller) buildDoorUpgradeGates(region renderregion.Value, gates map[int][][]doorUpgradeMaterial, levels, userMaterials map[int]int, materialIcons map[int]string, showFull bool) []drawing.MysekaiGateMaterials {
	gateIDs := make([]int, 0, len(gates))
	for gateID := range gates {
		gateIDs = append(gateIDs, gateID)
	}
	sort.Ints(gateIDs)
	result := make([]drawing.MysekaiGateMaterials, 0, len(gateIDs))
	for _, gateID := range gateIDs {
		result = append(result, c.buildDoorUpgradeGate(region, gateID, gates[gateID], levels[gateID], userMaterials, materialIcons, showFull))
	}
	return result
}

func (c *Controller) buildDoorUpgradeGate(region renderregion.Value, gateID int, levelMaterials [][]doorUpgradeMaterial, currentLevel int, userMaterials map[int]int, materialIcons map[int]string, showFull bool) drawing.MysekaiGateMaterials {
	if showFull {
		currentLevel = 0
	} else if currentLevel > 0 && currentLevel < len(levelMaterials) {
		levelMaterials = levelMaterials[currentLevel:]
	} else if currentLevel >= len(levelMaterials) {
		levelMaterials = nil
	}
	levels := c.buildDoorUpgradeLevels(region, currentLevel, levelMaterials, userMaterials, materialIcons, showFull)
	var level *int
	if !showFull {
		level = new(currentLevel)
	}
	return drawing.MysekaiGateMaterials{
		ID: gateID, Level: level, GateIconPath: new(c.resolveGateIconPath(region, gateID, 0)), LevelMaterials: levels,
	}
}

func (c *Controller) buildDoorUpgradeLevels(region renderregion.Value, currentLevel int, levels [][]doorUpgradeMaterial, userMaterials map[int]int, materialIcons map[int]string, showFull bool) []drawing.MysekaiGateLevelMaterials {
	sums := map[int]int{}
	result := make([]drawing.MysekaiGateLevelMaterials, 0, len(levels))
	for index, materials := range levels {
		if len(materials) == 0 {
			continue
		}
		items, color := c.buildDoorUpgradeLevelItems(region, materials, sums, userMaterials, materialIcons, showFull)
		result = append(result, drawing.MysekaiGateLevelMaterials{Level: currentLevel + index + 1, Color: color, Items: items})
	}
	return result
}

func (c *Controller) buildDoorUpgradeLevelItems(region renderregion.Value, materials []doorUpgradeMaterial, sums, userMaterials map[int]int, materialIcons map[int]string, showFull bool) ([]drawing.MysekaiGateMaterialItem, []int) {
	levelColor := []int{50, 50, 50}
	items := make([]drawing.MysekaiGateMaterialItem, 0, len(materials))
	for _, material := range materials {
		sums[material.materialID] += material.quantity
		userQuantity := userMaterials[material.materialID]
		color, sumQuantity := doorUpgradeMaterialDisplay(userQuantity, sums[material.materialID], showFull, levelColor)
		if !showFull && userQuantity < sums[material.materialID] {
			levelColor = []int{200, 0, 0}
		}
		items = append(items, drawing.MysekaiGateMaterialItem{
			ImagePath: c.regionPath(region, fmt.Sprintf("mysekai/thumbnail/material/%s.png", materialIcons[material.materialID])),
			Quantity:  material.quantity, Color: color, SumQuantity: sumQuantity,
		})
	}
	return items, levelColor
}

func doorUpgradeMaterialDisplay(userQuantity, requiredQuantity int, showFull bool, defaultColor []int) ([]int, string) {
	if showFull {
		return defaultColor, formatMysekaiQuantity(requiredQuantity)
	}
	quantity := fmt.Sprintf("%s/%d", formatMysekaiQuantity(userQuantity), requiredQuantity)
	if userQuantity < requiredQuantity {
		return []int{200, 0, 0}, quantity
	}
	return []int{0, 200, 0}, quantity
}

func (c *Controller) doorUpgradeProfile(region renderregion.Value, merged map[string]any, query *drawing.ProfileCardRequest, showFull bool) *drawing.ProfileCardRequest {
	if showFull {
		return nil
	}
	profile := c.mysekaiProfileCard(region, merged, query, false)
	if profile != nil && len(profile.DataSources) > 0 {
		profile.DataSources[0].Name = "Suite数据"
	}
	return profile
}

// RenderDoorUpgrade renders the MySekai door upgrade view.
func (c *Controller) RenderDoorUpgrade(query DoorUpgradeQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	payload, err := c.BuildDoorUpgradeRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiDoorUpgrade(payload)
}
