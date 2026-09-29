package mysekai

import (
	"fmt"
	"sort"

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

// ShopQuery requests the MySekai shop view (JP 7.0.0 mysekaiShops).
type ShopQuery struct {
	Region string `json:"region,omitempty"`
	// ResourceBox resolves the contents of a mysekai_shop resource box. The
	// MySekai master store does not hold resource boxes, so the caller wires
	// the region's education provider in; nil lists items without contents.
	ResourceBox func(resourceBoxID int) []ShopResource `json:"-"`
}

var mysekaiShopTypeOrder = map[string]int{"material": 0, "tool": 1}

var mysekaiShopTypeTitles = map[string]string{"material": "素材", "tool": "工具"}

// BuildShopRequest lists the region's MySekai shop. A region whose master
// data has no mysekaiShops rows (every region before JP 7.0.0) reports the
// shop as unavailable.
func (c *Controller) BuildShopRequest(query ShopQuery) (*drawing.MysekaiShopRequest, error) {
	c = c.withRegion(query.Region)
	if err := c.ensureMasterdata(); err != nil {
		return nil, err
	}
	region := c.resolveRegion(query.Region)
	shops := c.masterdata.loadList("mysekaiShops.json")
	if len(shops) == 0 {
		return nil, fmt.Errorf("mysekai shop is not available in region %s", region)
	}
	costsByShop := map[int][]map[string]any{}
	for _, cost := range c.masterdata.loadList("mysekaiShopCosts.json") {
		shopID := intNumber(cost["mysekaiShopId"], 0)
		costsByShop[shopID] = append(costsByShop[shopID], cost)
	}
	resolver := c.newMysekaiResourceResolver(region)

	groups := map[string]*drawing.MysekaiShopGroup{}
	sorted := append([]map[string]any(nil), shops...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return intNumber(sorted[i]["seq"], 0) < intNumber(sorted[j]["seq"], 0)
	})
	for _, shop := range sorted {
		shopType := stringValue(shop["mysekaiShopType"])
		group := groups[shopType]
		if group == nil {
			group = &drawing.MysekaiShopGroup{ShopType: shopType}
			if title, ok := mysekaiShopTypeTitles[shopType]; ok {
				group.Title = &title
			}
			groups[shopType] = group
		}
		group.Items = append(group.Items, c.buildShopItem(shop, costsByShop[intNumber(shop["id"], 0)], query.ResourceBox, resolver))
	}

	request := &drawing.MysekaiShopRequest{Title: "烤森商店", Shops: make([]drawing.MysekaiShopGroup, 0, len(groups))}
	for _, group := range groups {
		request.Shops = append(request.Shops, *group)
	}
	sort.SliceStable(request.Shops, func(i, j int) bool {
		return mysekaiShopTypeRank(request.Shops[i].ShopType) < mysekaiShopTypeRank(request.Shops[j].ShopType) ||
			mysekaiShopTypeRank(request.Shops[i].ShopType) == mysekaiShopTypeRank(request.Shops[j].ShopType) && request.Shops[i].ShopType < request.Shops[j].ShopType
	})
	return request, nil
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
	if limit := intNumber(shop["mysekaiShopExchangeLimitValue"], 0); limit > 0 {
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
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	request, err := c.BuildShopRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiShop(request)
}

// mysekaiResourceResolver names and pictures the resources the JP 7.0.0
// MySekai views list (shop goods and costs, bulk-harvest tools).
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
		// mysekaiTools has no database table yet; it resolves from the local
		// master files when the fallback is on and is otherwise empty.
		tools: c.masterdata.loadMapByID("mysekaiTools.json"),
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
