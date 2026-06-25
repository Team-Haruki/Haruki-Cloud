package inventory

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

func NewController(
	drawingClient *drawing.HarukiDrawingClient,
	assetHelper *assets.AssetHelper,
	snapshotService snapshot.Snapshot,
	defaultRegion renderregion.Value,
	options MasterdataOptions,
) *Controller {
	return &Controller{
		drawing:       drawingClient,
		assets:        assetHelper,
		snapshot:      snapshotService,
		defaultRegion: renderregion.WithDefault(defaultRegion),
		masterdata:    newMasterdataStore(options.LocalDir),
	}
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.drawing = c.drawing.WithContext(ctx)
	return &clone
}

func (c *Controller) BuildListRequestFromSnapshot(query Query) (*drawing.InventoryListRequest, error) {
	if c == nil {
		return nil, fmt.Errorf("inventory controller is not initialized")
	}

	snap := query.Snapshot
	if snap == nil {
		snap = c.snapshot
	}
	if snap == nil {
		return nil, fmt.Errorf("local user snapshot is not configured")
	}
	if err := snap.Require(); err != nil {
		return nil, err
	}

	raw := snap.RawData()
	if raw == nil {
		return nil, fmt.Errorf("user snapshot is missing raw data")
	}

	region := renderregion.Normalize(query.Region.String())
	if region.IsZero() {
		region = c.defaultRegion
	}
	if region.IsZero() {
		region = renderregion.JP
	}
	filter := normalizeFilter(query.Filter)
	if err := validateFilterForRegion(region, filter); err != nil {
		return nil, err
	}
	profile := query.Profile
	if profile == nil {
		profile = snap.DetailedProfile(region)
	}
	if profile == nil {
		return nil, fmt.Errorf("user snapshot is missing profile data")
	}

	md := c.masterdata.forRegion(region)
	items := make([]drawing.InventoryItem, 0,
		len(raw.UserMaterials)+
			len(raw.UserBoostItems)+
			len(raw.UserEventItems)+
			len(raw.UserGachaTickets)+
			len(raw.UserPracticeTickets)+
			len(raw.UserSkillPracticeTickets)+
			len(raw.UserGachaCeilItems)+
			len(raw.UserMysekaiMaterials)+
			4,
	)
	items = append(items, drawing.InventoryItem{
		ID:           0,
		Name:         "金币",
		Description:  "游戏内基础货币，可用于成员育成等消耗。",
		Category:     "currency",
		ResourceType: "coin",
		IconPath:     c.inventoryIconPath(region, "coin", 0),
		Quantity:     raw.UserGamedata.Coin,
		Seq:          0,
	})
	if raw.UserChargedCurrency.Free > 0 {
		items = append(items, drawing.InventoryItem{
			ID:           -1,
			Name:         "免费水晶",
			Description:  "免费获得的水晶，可用于招募等用途。",
			Category:     "currency",
			ResourceType: "jewel",
			IconPath:     c.inventoryIconPath(region, "jewel", 0),
			Quantity:     raw.UserChargedCurrency.Free,
			Seq:          1,
		})
	}
	if raw.UserChargedCurrency.Paid > 0 {
		items = append(items, drawing.InventoryItem{
			ID:           -2,
			Name:         "付费水晶",
			Description:  "购买获得的付费水晶，可用于招募等用途。",
			Category:     "currency",
			ResourceType: "jewel",
			IconPath:     c.inventoryIconPath(region, "jewel", 0),
			Quantity:     raw.UserChargedCurrency.Paid,
			Seq:          2,
		})
	}
	if raw.UserGamedata.VirtualCoin > 0 {
		items = append(items, drawing.InventoryItem{
			ID:           -3,
			Name:         "虚拟币",
			Description:  "虚拟演唱会等玩法中使用的货币。",
			Category:     "currency",
			ResourceType: "virtual_coin",
			IconPath:     c.inventoryIconPath(region, "virtual_coin", 0),
			Quantity:     raw.UserGamedata.VirtualCoin,
			Seq:          3,
		})
	}

	for _, mat := range raw.UserMaterials {
		if mat.MaterialID <= 0 || mat.Quantity <= 0 {
			continue
		}
		meta := md.materials[mat.MaterialID]
		name := strings.TrimSpace(meta.Name)
		if name == "" {
			name = fmt.Sprintf("材料 %d", mat.MaterialID)
		}
		category := inventoryCategoryForMaterial(meta.MaterialType, name)
		items = append(items, drawing.InventoryItem{
			ID:           mat.MaterialID,
			Name:         name,
			Description:  cleanInventoryDescription(meta.FlavorText),
			Category:     category,
			ResourceType: "material",
			IconPath:     c.inventoryIconPath(region, "material", mat.MaterialID),
			Quantity:     mat.Quantity,
			Seq:          fallbackSeq(meta.Seq, mat.MaterialID),
		})
	}

	for _, ticket := range raw.UserGachaTickets {
		if ticket.GachaTicketID <= 0 || ticket.Quantity <= 0 {
			continue
		}
		meta := md.gachaTickets[ticket.GachaTicketID]
		name := strings.TrimSpace(meta.Name)
		if name == "" {
			name = fmt.Sprintf("招募券 %d", ticket.GachaTicketID)
		}
		items = append(items, drawing.InventoryItem{
			ID:           ticket.GachaTicketID,
			Name:         name,
			Description:  cleanInventoryDescription(meta.FlavorText),
			Category:     "tickets",
			ResourceType: "gacha_ticket",
			IconPath:     c.inventoryIconByAssetName(region, "gacha_ticket", meta.AssetbundleName),
			Quantity:     ticket.Quantity,
			Seq:          fallbackSeq(meta.Seq, ticket.GachaTicketID),
		})
	}

	for _, ticket := range raw.UserPracticeTickets {
		if ticket.PracticeTicketID <= 0 || ticket.Quantity <= 0 {
			continue
		}
		meta := md.practiceTickets[ticket.PracticeTicketID]
		name := strings.TrimSpace(meta.Name)
		if name == "" {
			name = fmt.Sprintf("练习乐谱 %d", ticket.PracticeTicketID)
		}
		items = append(items, drawing.InventoryItem{
			ID:           ticket.PracticeTicketID,
			Name:         name,
			Description:  cleanInventoryDescription(meta.FlavorText),
			Category:     "training",
			ResourceType: "practice_ticket",
			IconPath:     c.inventoryIconPath(region, "practice_ticket", ticket.PracticeTicketID),
			Quantity:     ticket.Quantity,
			Seq:          fallbackSeq(meta.CharacterID*1000+meta.Exp, ticket.PracticeTicketID),
		})
	}

	for _, ticket := range raw.UserSkillPracticeTickets {
		if ticket.SkillPracticeTicketID <= 0 || ticket.Quantity <= 0 {
			continue
		}
		meta := md.skillPracticeTickets[ticket.SkillPracticeTicketID]
		name := strings.TrimSpace(meta.Name)
		if name == "" {
			name = fmt.Sprintf("技能升级乐谱 %d", ticket.SkillPracticeTicketID)
		}
		items = append(items, drawing.InventoryItem{
			ID:           ticket.SkillPracticeTicketID,
			Name:         name,
			Description:  cleanInventoryDescription(meta.FlavorText),
			Category:     "training",
			ResourceType: "skill_practice_ticket",
			IconPath:     c.inventoryIconPath(region, "skill_practice_ticket", ticket.SkillPracticeTicketID),
			Quantity:     ticket.Quantity,
			Seq:          fallbackSeq(meta.CharacterID*1000+meta.Exp, ticket.SkillPracticeTicketID),
		})
	}

	for _, item := range raw.UserGachaCeilItems {
		if item.GachaCeilItemID <= 0 || item.Quantity <= 0 {
			continue
		}
		meta := md.gachaCeilItems[item.GachaCeilItemID]
		name := strings.TrimSpace(meta.Name)
		if name == "" {
			name = fmt.Sprintf("招募贴纸 %d", item.GachaCeilItemID)
		}
		items = append(items, drawing.InventoryItem{
			ID:           item.GachaCeilItemID,
			Name:         name,
			Description:  cleanInventoryDescription(meta.FlavorText),
			Category:     "tickets",
			ResourceType: "gacha_ceil_item",
			IconPath:     c.inventoryIconByAssetName(region, "gacha_ceil_item", meta.AssetbundleName),
			Quantity:     item.Quantity,
			Seq:          fallbackSeq(meta.Seq, item.GachaCeilItemID),
		})
	}

	for _, material := range raw.UserMysekaiMaterials {
		if material.MysekaiMaterialID <= 0 || material.Quantity <= 0 {
			continue
		}
		meta := md.mysekaiMaterials[material.MysekaiMaterialID]
		name := strings.TrimSpace(meta.Name)
		if name == "" {
			name = fmt.Sprintf("MySekai 材料 %d", material.MysekaiMaterialID)
		}
		category := "mysekai"
		if isMysekaiMemory(meta) {
			category = "memory"
		}
		items = append(items, drawing.InventoryItem{
			ID:           material.MysekaiMaterialID,
			Name:         name,
			Description:  cleanInventoryDescription(meta.Description),
			Category:     category,
			ResourceType: "mysekai_material",
			IconPath:     c.inventoryIconByAssetName(region, "mysekai_material", meta.IconAssetbundleName),
			Quantity:     material.Quantity,
			Seq:          fallbackSeq(meta.Seq, material.MysekaiMaterialID),
		})
	}

	for _, boost := range raw.UserBoostItems {
		if boost.BoostItemID <= 0 || boost.Quantity <= 0 {
			continue
		}
		meta := md.boostItems[boost.BoostItemID]
		name := strings.TrimSpace(meta.Name)
		if name == "" {
			name = fmt.Sprintf("演出能量道具 %d", boost.BoostItemID)
		}
		var recovery *int
		if meta.RecoveryValue > 0 {
			value := meta.RecoveryValue
			recovery = &value
		}
		items = append(items, drawing.InventoryItem{
			ID:            boost.BoostItemID,
			Name:          name,
			Description:   cleanInventoryDescription(meta.FlavorText),
			Category:      "boost",
			ResourceType:  "boost_item",
			IconPath:      c.inventoryIconPath(region, "boost_item", boost.BoostItemID),
			Quantity:      boost.Quantity,
			Seq:           fallbackSeq(meta.Seq, boost.BoostItemID),
			RecoveryValue: recovery,
		})
	}

	items = filterInventoryItems(items, filter)
	sections := buildInventorySections(items)
	if len(sections) == 0 {
		return nil, fmt.Errorf("user snapshot has no inventory data")
	}

	return &drawing.InventoryListRequest{
		Profile:    *profile,
		Sections:   sections,
		TotalItems: countInventoryEntries(sections),
	}, nil
}

func (c *Controller) RenderList(query Query) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildListRequestFromSnapshot(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateInventoryList(payload)
}

func (c *Controller) inventoryIconPath(region renderregion.Value, resourceType string, id int) string {
	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	switch resourceType {
	case "coin", "virtual_coin", "jewel":
		return c.resolveInventoryAssetPath(region,
			filepath.Join("thumbnail", "common_material", resourceType+".png"))
	case "material":
		if id <= 0 {
			return ""
		}
		return c.resolveInventoryAssetPath(region,
			filepath.Join("thumbnail", "material", fmt.Sprintf("material%d.png", id)))
	case "boost_item":
		if id <= 0 {
			return ""
		}
		return c.resolveInventoryAssetPath(region,
			filepath.Join("thumbnail", "boost_item", fmt.Sprintf("boost_item%d.png", id)))
	case "practice_ticket":
		if id <= 0 {
			return ""
		}
		return c.resolveInventoryAssetPath(region,
			filepath.Join("thumbnail", "practice_ticket", fmt.Sprintf("ticket%d.png", id)))
	case "skill_practice_ticket":
		if id <= 0 {
			return ""
		}
		return c.resolveInventoryAssetPath(region,
			filepath.Join("thumbnail", "skill_practice_ticket", fmt.Sprintf("ticket%d.png", id)))
	default:
		return ""
	}
}

func (c *Controller) inventoryIconByAssetName(region renderregion.Value, resourceType string, assetName string) string {
	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	assetName = strings.TrimSpace(assetName)
	if assetName == "" {
		return ""
	}
	switch resourceType {
	case "event_item":
		return c.resolveInventoryAssetPath(region,
			filepath.Join("thumbnail", "material", assetName+".png"),
			filepath.Join("event", "item", assetName+".png"))
	case "gacha_ticket":
		return c.resolveInventoryAssetPath(region,
			filepath.Join("thumbnail", "gacha_ticket", assetName+".png"),
			filepath.Join("thumbnail", "material", assetName+".png"),
			filepath.Join("thumbnail", "common_material", assetName+".png"))
	case "gacha_ceil_item":
		return c.resolveInventoryAssetPath(region,
			filepath.Join("thumbnail", "gacha_item", assetName+".png"),
			filepath.Join("thumbnail", "material", assetName+".png"),
			filepath.Join("thumbnail", "common_material", assetName+".png"))
	case "mysekai_material":
		return c.resolveInventoryAssetPath(region,
			filepath.Join("mysekai", "thumbnail", "material", assetName+".png"))
	default:
		return ""
	}
}

func (c *Controller) resolveInventoryAssetPath(region renderregion.Value, relPaths ...string) string {
	regionKey := renderregion.WithDefault(region).String()
	if strings.TrimSpace(regionKey) == "" {
		regionKey = renderregion.JP.String()
	}
	if c != nil && c.assets != nil {
		for _, candidateRegion := range []string{regionKey, renderregion.JP.String()} {
			path := assets.ResolveRegionAssetPath(c.assets, candidateRegion, relPaths...)
			if path != "" && c.assets.FirstExisting(path) != "" {
				return path
			}
		}
	}
	return assets.ResolveRegionAssetPath(c.assets, regionKey, relPaths...)
}

func buildInventorySections(items []drawing.InventoryItem) []drawing.InventorySection {
	grouped := make(map[string][]drawing.InventoryItem)
	for _, item := range items {
		if item.Quantity < 0 {
			continue
		}
		key := strings.TrimSpace(item.Category)
		if key == "" {
			key = "other"
		}
		grouped[key] = append(grouped[key], item)
	}

	sections := make([]drawing.InventorySection, 0, len(inventorySectionOrder))
	for _, def := range inventorySectionOrder {
		group := grouped[def.key]
		if len(group) == 0 {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].Seq != group[j].Seq {
				return group[i].Seq < group[j].Seq
			}
			if group[i].ID != group[j].ID {
				return group[i].ID < group[j].ID
			}
			return group[i].Name < group[j].Name
		})
		sections = append(sections, drawing.InventorySection{
			Key:   def.key,
			Title: def.title,
			Items: group,
		})
	}
	return sections
}

func countInventoryEntries(sections []drawing.InventorySection) int {
	total := 0
	for _, section := range sections {
		total += len(section.Items)
	}
	return total
}

func normalizeFilter(filter Filter) Filter {
	switch strings.ToLower(strings.TrimSpace(string(filter))) {
	case "":
		return FilterDefault
	case string(FilterJewel):
		return FilterJewel
	case string(FilterBoost):
		return FilterBoost
	case string(FilterMysekai):
		return FilterMysekai
	case string(FilterMemory):
		return FilterMemory
	default:
		return Filter(filter)
	}
}

func validateFilterForRegion(region renderregion.Value, filter Filter) error {
	if renderregion.WithDefault(region) != renderregion.CN {
		return nil
	}
	switch normalizeFilter(filter) {
	case FilterMysekai:
		return fmt.Errorf("国服暂不支持查询 MySekai 材料")
	case FilterMemory:
		return fmt.Errorf("国服暂不支持查询记忆")
	default:
		return nil
	}
}

func filterInventoryItems(items []drawing.InventoryItem, filter Filter) []drawing.InventoryItem {
	filter = normalizeFilter(filter)
	filtered := items[:0]
	for _, item := range items {
		if inventoryItemMatchesFilter(item, filter) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func inventoryItemMatchesFilter(item drawing.InventoryItem, filter Filter) bool {
	switch filter {
	case FilterJewel:
		return item.ResourceType == "jewel"
	case FilterBoost:
		return item.ResourceType == "boost_item"
	case FilterMysekai:
		return item.Category == "mysekai"
	case FilterMemory:
		return item.Category == "memory"
	default:
		return !isSpecialInventoryItem(item)
	}
}

func isSpecialInventoryItem(item drawing.InventoryItem) bool {
	switch {
	case item.ResourceType == "jewel":
		return true
	case item.ResourceType == "boost_item":
		return true
	case item.Category == "mysekai":
		return true
	case item.Category == "memory":
		return true
	default:
		return false
	}
}

func fallbackSeq(seq int, id int) int {
	if seq > 0 {
		return seq
	}
	return id
}

func isMysekaiMemory(meta mysekaiMaterialMeta) bool {
	typ := strings.ToLower(strings.TrimSpace(meta.Type))
	icon := strings.ToLower(strings.TrimSpace(meta.IconAssetbundleName))
	name := strings.TrimSpace(meta.Name)
	return typ == "game_character" ||
		strings.Contains(icon, "memoria") ||
		strings.Contains(icon, "memory") ||
		strings.Contains(name, "メモリア") ||
		strings.Contains(name, "记忆")
}

func cleanInventoryDescription(description string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(description)), " ")
}
