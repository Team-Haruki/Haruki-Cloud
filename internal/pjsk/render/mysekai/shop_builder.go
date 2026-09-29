package mysekai

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

// ShopResource is one entry of a mysekai_shop resource box.
type ShopResource struct {
	ResourceType string
	ResourceID   int
	Quantity     int
}

// ShopQuery requests the player's current MySekai shop lineup.
type ShopQuery struct {
	Region    string                      `json:"region,omitempty"`
	ShopType  string                      `json:"shop_type,omitempty"`
	ShowAll   bool                        `json:"show_all,omitempty"`
	Profile   *drawing.ProfileCardRequest `json:"-"`
	NowMillis int64                       `json:"-"`
	// ResourceBox resolves the contents of a mysekai_shop resource box. The
	// MySekai master store does not hold resource boxes, so the caller wires
	// the region's education provider in. Required for tool/material entries.
	ResourceBox func(resourceBoxID int) []ShopResource `json:"-"`
}

var mysekaiShopTypeOrder = map[string]int{"blueprint_daily": 0, "blueprint_weekly": 1, "tool": 2, "material": 3}

var mysekaiShopTypeTitles = map[string]string{"blueprint_daily": "每日蓝图", "blueprint_weekly": "每周蓝图", "material": "材料", "tool": "工具"}

// BuildShopRequest uses the uploaded lineup, never inventing a new rotation
// from master data when the player has not uploaded their refreshed shop.
func (c *Controller) BuildShopRequest(query ShopQuery) (*drawing.MysekaiShopRequest, error) {
	if query.ShopType == "" && !query.ShowAll {
		query.ShopType = "blueprint"
	}
	c = c.withRegion(query.Region)
	if err := c.ensureMasterdata(); err != nil {
		return nil, err
	}
	if query.ShopType != "" && query.ShopType != "blueprint" && query.ShopType != "tool" && query.ShopType != "material" {
		return nil, fmt.Errorf("mysekai shop invalid type: %s", query.ShopType)
	}
	shops := c.masterdata.loadList("mysekaiShops.json")
	blueprintShops := c.masterdata.loadList("mysekaiBlueprintShops.json")
	if len(shops) == 0 && len(blueprintShops) == 0 {
		return nil, fmt.Errorf("mysekai shop is not available in region %s", c.resolveRegion(query.Region))
	}
	merged, region, err := c.prepareSnapshotOnly(query.Region)
	if err != nil {
		return nil, err
	}
	now := query.NowMillis
	if now == 0 {
		now = time.Now().UnixMilli()
	}
	pass, _ := merged["userMysekaiColorfulPass"].(map[string]any)
	passActive := int64Number(pass["expiredAt"], 0) > now
	request := &drawing.MysekaiShopRequest{
		Title: "烤森商店（按上传数据）", PassActive: &passActive,
		Profile: c.mysekaiProfileCard(region, merged, query.Profile, true),
		Shops:   make([]drawing.MysekaiShopGroup, 0),
	}
	if !passActive {
		request.Title += "（通行证未生效）"
	}
	resolver := c.newMysekaiResourceResolver(region)
	if query.ShopType == "" || query.ShopType == "blueprint" {
		groups, err := c.buildBlueprintShopGroups(query, merged, blueprintShops, passActive, resolver)
		if err != nil {
			return nil, err
		}
		request.Shops = append(request.Shops, groups...)
	}
	if query.ShopType != "blueprint" {
		groups, err := c.buildResourceShopGroups(query, merged, shops, passActive, resolver)
		if err != nil {
			return nil, err
		}
		request.Shops = append(request.Shops, groups...)
	}
	sort.SliceStable(request.Shops, func(i, j int) bool {
		return mysekaiShopTypeRank(request.Shops[i].ShopType) < mysekaiShopTypeRank(request.Shops[j].ShopType)
	})
	return request, nil
}

func (c *Controller) buildResourceShopGroups(query ShopQuery, merged map[string]any, shops []map[string]any, passActive bool, resolver mysekaiResourceResolver) ([]drawing.MysekaiShopGroup, error) {
	if len(shops) == 0 {
		return nil, nil
	}
	records, err := shopSnapshotList(merged, "userMysekaiShops")
	if err != nil {
		return nil, err
	}
	counts := make(map[int]int, len(records))
	for _, raw := range records {
		row, _ := raw.(map[string]any)
		counts[intNumber(row["mysekaiShopId"], 0)] = max(0, intNumber(row["count"], 0))
	}
	costsByShop := map[int][]map[string]any{}
	for _, cost := range c.masterdata.loadList("mysekaiShopCosts.json") {
		id := intNumber(cost["mysekaiShopId"], 0)
		costsByShop[id] = append(costsByShop[id], cost)
	}
	groups := map[string]*drawing.MysekaiShopGroup{}
	sorted := append([]map[string]any(nil), shops...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if intNumber(sorted[i]["seq"], 0) != intNumber(sorted[j]["seq"], 0) {
			return intNumber(sorted[i]["seq"], 0) < intNumber(sorted[j]["seq"], 0)
		}
		return intNumber(sorted[i]["id"], 0) < intNumber(sorted[j]["id"], 0)
	})
	for _, shop := range sorted {
		shopType := stringValue(shop["mysekaiShopType"])
		if (shopType != "material" && shopType != "tool") || (query.ShopType != "" && shopType != query.ShopType) {
			continue
		}
		if query.ResourceBox == nil {
			return nil, fmt.Errorf("mysekai shop masterdata missing resource boxes")
		}
		contents := query.ResourceBox(intNumber(shop["resourceBoxId"], 0))
		if len(contents) == 0 {
			return nil, fmt.Errorf("mysekai shop masterdata missing resource box %d", intNumber(shop["resourceBoxId"], 0))
		}
		for _, resource := range contents {
			if resource.ResourceType == "mysekai_tool" && resolver.tools[resource.ResourceID] == nil {
				return nil, fmt.Errorf("mysekai shop masterdata missing tool %d", resource.ResourceID)
			}
		}
		item := c.buildShopItem(shop, costsByShop[intNumber(shop["id"], 0)], func(int) []ShopResource { return contents }, resolver)
		count := counts[item.ID]
		item.ExchangedCount = &count
		available := passActive
		if item.ExchangeLimitType != "none" {
			remaining := 0
			if item.ExchangeLimitValue != nil {
				remaining = max(0, *item.ExchangeLimitValue-count)
			}
			item.RemainingCount = &remaining
			available = available && remaining > 0
		}
		capacity, err := c.shopMaterialCapacity(merged, contents, resolver)
		if err != nil {
			return nil, err
		}
		if capacity != nil {
			available = available && *capacity > 0
		}
		item.MaterialCapacityCount = capacity
		item.Available = &available
		if !query.ShowAll && !available {
			continue
		}
		group := groups[shopType]
		if group == nil {
			title := mysekaiShopTypeTitles[shopType]
			group = &drawing.MysekaiShopGroup{ShopType: shopType, Title: &title}
			groups[shopType] = group
		}
		group.Items = append(group.Items, item)
	}
	result := make([]drawing.MysekaiShopGroup, 0, len(groups))
	for _, group := range groups {
		result = append(result, *group)
	}
	return result, nil
}

func shopSnapshotList(merged map[string]any, key string) ([]any, error) {
	rows, ok := merged[key].([]any)
	if !ok {
		return nil, fmt.Errorf("mysekai shop snapshot missing %s", key)
	}
	return rows, nil
}

func mysekaiShopTypeRank(shopType string) int {
	if rank, ok := mysekaiShopTypeOrder[shopType]; ok {
		return rank
	}
	return len(mysekaiShopTypeOrder)
}

func (c *Controller) buildShopItem(shop map[string]any, costs []map[string]any, resourceBox func(int) []ShopResource, resolver mysekaiResourceResolver) drawing.MysekaiShopItem {
	item := drawing.MysekaiShopItem{
		ID:                intNumber(shop["id"], 0),
		Quantity:          1,
		ExchangeLimitType: stringValue(shop["mysekaiShopExchangeLimitType"]),
	}
	if item.ExchangeLimitType == "" {
		item.ExchangeLimitType = "none"
	}
	if limit := max(0, intNumber(shop["mysekaiShopExchangeLimitValue"], 0)); item.ExchangeLimitType != "none" {
		item.ExchangeLimitValue = &limit
	}
	if resourceBox != nil {
		if contents := resourceBox(intNumber(shop["resourceBoxId"], 0)); len(contents) > 0 {
			first := contents[0]
			name, image := resolver.resolve(first.ResourceType, first.ResourceID)
			if name != "" {
				item.Name = &name
			}
			item.ImagePath = drawing.AssetPath(image)
			if first.Quantity > 0 {
				item.Quantity = first.Quantity
			}
		}
	}
	sort.SliceStable(costs, func(i, j int) bool { return intNumber(costs[i]["seq"], 0) < intNumber(costs[j]["seq"], 0) })
	for _, cost := range costs {
		_, image := resolver.resolve(stringValue(cost["resourceType"]), intNumber(cost["resourceId"], 0))
		item.Costs = append(item.Costs, drawing.MysekaiShopCost{
			ImagePath: drawing.AssetPath(image),
			Quantity:  intNumber(cost["quantity"], 0),
		})
	}
	return item
}

// RenderShop renders the MySekai shop view.
func (c *Controller) RenderShop(query ShopQuery) ([]byte, error) {
	image, err := c.RenderShopImage(query)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.requestCtx)
}

func (c *Controller) RenderShopImage(query ShopQuery) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	request, err := c.BuildShopRequest(query)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.RenderShopRequestImage(request)
}

// mysekaiResourceResolver names and pictures the resources the JP 7.0.0
// MySekai views list (shop goods and costs).
type mysekaiResourceResolver struct {
	c         *Controller
	region    renderregion.Value
	materials map[int]map[string]any
	items     map[int]map[string]any
	tools     map[int]map[string]any
}

func (c *Controller) newMysekaiResourceResolver(region renderregion.Value) mysekaiResourceResolver {
	return mysekaiResourceResolver{
		c:         c,
		region:    region,
		materials: c.masterdata.loadMapByID("mysekaiMaterials.json"),
		items:     c.masterdata.loadMapByID("mysekaiItems.json"),
		tools:     c.masterdata.loadMapByID("mysekaiTools.json"),
	}
}

func (r mysekaiResourceResolver) resolve(resourceType string, resourceID int) (string, string) {
	switch resourceType {
	case "mysekai_material":
		row := r.materials[resourceID]
		name := stringValue(row["name"])
		if icon := stringValue(row["iconAssetbundleName"]); icon != "" {
			return name, r.c.regionPath(r.region, fmt.Sprintf("mysekai/thumbnail/material/%s.png", icon))
		}
		return name, ""
	case "mysekai_item":
		row := r.items[resourceID]
		name := stringValue(row["name"])
		if icon := stringValue(row["iconAssetbundleName"]); icon != "" {
			return name, r.c.regionPath(r.region, fmt.Sprintf("mysekai/thumbnail/item/%s.png", icon))
		}
		return name, ""
	case "mysekai_tool":
		return r.tool(resourceID)
	case "jewel", "paid_jewel", "coin", "virtual_coin":
		if resourceType == "paid_jewel" {
			resourceType = "jewel"
		}
		return "", r.c.regionPath(r.region, fmt.Sprintf("thumbnail/common_material/%s.png", resourceType))
	case "material":
		if resourceID > 0 {
			return "", r.c.regionPath(r.region, fmt.Sprintf("thumbnail/material/material%d.png", resourceID))
		}
	}
	return "", ""
}

func (r mysekaiResourceResolver) tool(toolID int) (string, string) {
	row := r.tools[toolID]
	name := stringValue(row["name"])
	if bundle := stringValue(row["assetbundleName"]); bundle != "" {
		return name, r.c.regionPath(r.region, fmt.Sprintf("mysekai/thumbnail/tool/%s.png", bundle))
	}
	return name, ""
}

// RenderShopRequest renders an already built, request-scoped shop payload.
func (c *Controller) RenderShopRequest(request *drawing.MysekaiShopRequest) ([]byte, error) {
	image, err := c.RenderShopRequestImage(request)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.requestCtx)
}

func (c *Controller) RenderShopRequestImage(request *drawing.MysekaiShopRequest) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, fmt.Errorf("drawing client is not configured")
	}
	if request == nil {
		return drawing.ImageResult{}, fmt.Errorf("mysekai shop request is nil")
	}
	cloned := *request
	cloned.Shops = slices.Clone(request.Shops)
	for i := range cloned.Shops {
		cloned.Shops[i].Items = slices.Clone(cloned.Shops[i].Items)
	}
	decorateShopRequest(&cloned)
	return c.drawing.GenerateMysekaiShopImage(&cloned)
}
