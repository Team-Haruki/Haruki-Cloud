package mysekai

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/i18n"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/testutil"
)

func jp700Controller(lists map[string][]map[string]any) *Controller {
	maps := map[string]map[int]map[string]any{}
	for name, rows := range lists {
		byID := map[int]map[string]any{}
		for _, row := range rows {
			if id := intNumber(row["id"], 0); id > 0 {
				byID[id] = row
			}
		}
		maps[name] = byID
	}
	return &Controller{masterdata: &sonarMasterdataSource{lists: lists, maps: maps}, defaultRegion: renderregion.JP}
}

func TestBuildShopRequestGroupsJPShopRows(t *testing.T) {
	controller := jp700Controller(map[string][]map[string]any{
		"mysekaiShops.json": {
			{"id": 101, "mysekaiShopType": "tool", "seq": 1, "resourceBoxId": 101, "mysekaiShopExchangeLimitType": "limited_per_mysekai_colorful_pass", "mysekaiShopExchangeLimitValue": 99},
			{"id": 2, "mysekaiShopType": "material", "seq": 2, "resourceBoxId": 2, "mysekaiShopExchangeLimitType": "none"},
			{"id": 1, "mysekaiShopType": "material", "seq": 1, "resourceBoxId": 1, "mysekaiShopExchangeLimitType": "limited_per_mysekai_colorful_pass", "mysekaiShopExchangeLimitValue": 3},
		},
		"mysekaiShopCosts.json": {
			{"id": 1, "mysekaiShopId": 1, "seq": 1, "resourceType": "jewel", "quantity": 500},
			{"id": 2, "mysekaiShopId": 2, "seq": 1, "resourceType": "jewel", "quantity": 100},
		},
		"mysekaiMaterials.json": {
			{"id": 12, "name": "ダイヤモンド", "iconAssetbundleName": "item_diamond"},
			{"id": 5, "name": "夕桐", "iconAssetbundleName": "item_yugiri"},
		},
		"mysekaiMaterialPossessions.json": {{"id": 1, "level": 1, "possessionLimit": 1000}},
		"mysekaiTools.json":               {{"id": 10, "name": "チェーンソー", "assetbundleName": "ax0005"}},
	})
	controller = controller.WithMySekaiData([]byte(`{"userMysekaiShops":[],"userMysekaiColorfulPass":{"expiredAt":4102444800000},"userMysekaiGamedata":{"mysekaiMaterialPossessionLevel":1},"userMysekaiMaterialPossession":{"quantity":0}}`))
	boxes := map[int][]ShopResource{
		1:   {{ResourceType: "mysekai_material", ResourceID: 12, Quantity: 1}},
		2:   {{ResourceType: "mysekai_material", ResourceID: 5, Quantity: 3}},
		101: {{ResourceType: "mysekai_tool", ResourceID: 10, Quantity: 1}},
	}
	req, err := controller.BuildShopRequest(ShopQuery{Region: "jp", ShowAll: true, ResourceBox: func(id int) []ShopResource { return boxes[id] }})
	if err != nil {
		t.Fatalf("BuildShopRequest() error = %v", err)
	}
	if len(req.Shops) != 2 || req.Shops[0].ShopType != "tool" || req.Shops[1].ShopType != "material" {
		t.Fatalf("shop groups = %+v", req.Shops)
	}
	materials := req.Shops[1].Items
	if len(materials) != 2 || materials[0].ID != 1 || materials[1].ID != 2 {
		t.Fatalf("material items must follow seq: %+v", materials)
	}
	first := materials[0]
	if first.Name == nil || *first.Name != "ダイヤモンド" || !strings.Contains(first.ImagePath.First(), "mysekai/thumbnail/material/item_diamond.png") {
		t.Fatalf("material item = %+v", first)
	}
	if first.ExchangeLimitType != "limited_per_mysekai_colorful_pass" || first.ExchangeLimitValue == nil || *first.ExchangeLimitValue != 3 {
		t.Fatalf("exchange limit = %+v", first)
	}
	if len(first.Costs) != 1 || first.Costs[0].Quantity != 500 || !strings.Contains(first.Costs[0].ImagePath.First(), "common_material/jewel.png") {
		t.Fatalf("costs = %+v", first.Costs)
	}
	if materials[1].Quantity != 3 || materials[1].ExchangeLimitValue != nil || materials[1].ExchangeLimitType != "none" {
		t.Fatalf("unlimited item = %+v", materials[1])
	}
	tool := req.Shops[0].Items[0]
	if tool.Name == nil || *tool.Name != "チェーンソー" || !strings.Contains(tool.ImagePath.First(), "mysekai/thumbnail/tool/ax0005.png") {
		t.Fatalf("tool item = %+v", tool)
	}
}

func TestBuildShopRequestOldRegionReportsUnavailable(t *testing.T) {
	controller := jp700Controller(map[string][]map[string]any{"mysekaiMaterials.json": {{"id": 1}}})
	if _, err := controller.BuildShopRequest(ShopQuery{Region: "tw"}); err == nil || testutil.MessageID(err) != "mysekai.shop.region_unavailable" {
		t.Fatalf("expected shop unavailable, got %v", err)
	}
}

func blueprintTermMasterdata(terms []map[string]any) map[string][]map[string]any {
	return map[string][]map[string]any{
		"mysekaiBlueprintTerms.json": terms,
		"mysekaiBlueprints.json": {
			{"id": 844, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 7001},
			{"id": 845, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 7002},
		},
		"mysekaiFixtures.json": {
			{"id": 7001, "name": "バースデーケーキ", "assetbundleName": "mdl_cake"},
			{"id": 7002, "name": "記念の花束", "assetbundleName": "mdl_bouquet"},
		},
		"mysekaiBlueprintTermMysekaiMaterialCosts.json": {
			{"id": 1, "groupId": 26844, "mysekaiMaterialId": 103, "seq": 1, "quantity": 3},
		},
		"mysekaiMaterials.json": {{"id": 103, "name": "思い出のかけら", "iconAssetbundleName": "item_memory"}},
	}
}

func TestBuildBlueprintTermRequestJPTermsWithTabsCostsAndLimits(t *testing.T) {
	controller := jp700Controller(blueprintTermMasterdata([]map[string]any{
		{"id": 1, "mysekaiBlueprintId": 844, "startAt": 1000, "endAt": 2000, "mysekaiBlueprintTermTabType": "birthday_anniversary"},
		{"id": 136, "mysekaiBlueprintId": 844, "startAt": 5000, "endAt": 9000, "mysekaiBlueprintTermTabType": "birthday_anniversary", "mysekaiBlueprintTermMysekaiMaterialCostGroupId": 26844, "craftLimit": 1},
		{"id": 137, "mysekaiBlueprintId": 845, "startAt": 4000, "endAt": 9000, "mysekaiBlueprintTermTabType": "limited_term"},
	}))
	req, err := controller.BuildBlueprintTermRequest(BlueprintTermQuery{Region: "jp", NowMillis: 3000})
	if err != nil {
		t.Fatalf("BuildBlueprintTermRequest() error = %v", err)
	}
	if len(req.Tabs) != 2 || req.Tabs[0].TabType != "limited_term" || req.Tabs[1].TabType != "birthday_anniversary" {
		t.Fatalf("tabs = %+v", req.Tabs)
	}
	birthday := req.Tabs[1]
	if birthday.Title != "生日/周年蓝图" || len(birthday.Blueprints) != 1 {
		t.Fatalf("ended terms must be hidden by default: %+v", birthday)
	}
	entry := birthday.Blueprints[0]
	if entry.Name != "バースデーケーキ" || entry.CraftLimit == nil || *entry.CraftLimit != 1 || len(entry.CostMaterials) != 1 || entry.CostMaterials[0].Quantity != 3 {
		t.Fatalf("birthday entry = %+v", entry)
	}
	all, err := controller.BuildBlueprintTermRequest(BlueprintTermQuery{Region: "jp", NowMillis: 3000, ShowAll: new(true)})
	if err != nil || len(all.Tabs[1].Blueprints) != 2 {
		t.Fatalf("show all = %+v err=%v", all, err)
	}
}

func TestBuildBlueprintTermRequestENTermsWithoutNewColumns(t *testing.T) {
	// EN 6.0.0 serves terms without tab type, cost group or craft limit.
	controller := jp700Controller(blueprintTermMasterdata([]map[string]any{
		{"id": 1, "mysekaiBlueprintId": 844, "startAt": 1000, "endAt": 9000},
	}))
	req, err := controller.BuildBlueprintTermRequest(BlueprintTermQuery{Region: "en", NowMillis: 3000})
	if err != nil {
		t.Fatalf("BuildBlueprintTermRequest() error = %v", err)
	}
	if len(req.Tabs) != 1 || req.Tabs[0].TabType != "" || req.Tabs[0].Title != "限时蓝图" {
		t.Fatalf("tabs = %+v", req.Tabs)
	}
	entry := req.Tabs[0].Blueprints[0]
	if entry.CraftLimit != nil || entry.CostMaterials != nil {
		t.Fatalf("EN entry must not invent limits or costs: %+v", entry)
	}
	if _, err := controller.BuildBlueprintTermRequest(BlueprintTermQuery{Region: "en", NowMillis: 10000}); err == nil || testutil.MessageID(err) != "mysekai.blueprint_term.none_current" {
		t.Fatalf("expected no current term, got %v", err)
	}
}

func TestBuildBlueprintTermRequestRegionWithoutTerms(t *testing.T) {
	controller := jp700Controller(map[string][]map[string]any{"mysekaiBlueprints.json": {{"id": 1}}})
	if _, err := controller.BuildBlueprintTermRequest(BlueprintTermQuery{Region: "tw"}); err == nil || testutil.MessageID(err) != "mysekai.blueprint_term.region_unavailable" {
		t.Fatalf("expected terms unavailable, got %v", err)
	}
}

func fixtureDetailTermMasterdata(terms []map[string]any) map[string][]map[string]any {
	lists := blueprintTermMasterdata(terms)
	lists["mysekaiFixtures.json"] = []map[string]any{{"id": 7001, "name": "バースデーケーキ", "assetbundleName": "mdl_cake", "mysekaiFixtureMainGenreId": 1}}
	lists["mysekaiBlueprints.json"] = []map[string]any{{"id": 844, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 7001, "craftCountLimit": 0}}
	return lists
}

func TestFixtureDetailShowsBlueprintTermLines(t *testing.T) {
	controller := jp700Controller(fixtureDetailTermMasterdata([]map[string]any{
		{"id": 1, "mysekaiBlueprintId": 844, "startAt": 1000, "endAt": 2000, "mysekaiBlueprintTermTabType": "birthday_anniversary"},
		{"id": 136, "mysekaiBlueprintId": 844, "startAt": 5000, "endAt": 9000, "mysekaiBlueprintTermTabType": "birthday_anniversary", "mysekaiBlueprintTermMysekaiMaterialCostGroupId": 26844, "craftLimit": 1},
	}))
	reqs, err := controller.BuildFixtureDetailRequests(FixtureDetailQuery{Region: "jp", Query: "7001", NowMillis: 3000})
	if err != nil {
		t.Fatalf("BuildFixtureDetailRequests() error = %v", err)
	}
	info := strings.Join(reqs[0].BasicInfo, "\n")
	for _, want := range []string{
		i18n.T("render_mysekai.blueprint_term.period_upcoming", i18n.Data{
			"Tab":   i18n.T("render_mysekai.blueprint_term.tab.birthday_anniversary"),
			"Start": i18n.FormatUserTime(time.UnixMilli(5000), nil),
			"End":   i18n.FormatUserTime(time.UnixMilli(9000), nil),
		}),
		i18n.T("render_mysekai.blueprint_term.craft_limit", i18n.Data{"Count": 1}),
		i18n.T("render_mysekai.blueprint_term.extra_materials", i18n.Data{"Materials": "思い出のかけら×3"}),
	} {
		if !strings.Contains(info, want) {
			t.Fatalf("basic info missing %q:\n%s", want, info)
		}
	}
}

func TestFixtureDetailWithoutTermsIsUnchanged(t *testing.T) {
	controller := jp700Controller(fixtureDetailTermMasterdata(nil))
	reqs, err := controller.BuildFixtureDetailRequests(FixtureDetailQuery{Region: "tw", Query: "7001", NowMillis: 3000})
	if err != nil {
		t.Fatalf("BuildFixtureDetailRequests() error = %v", err)
	}
	want := append(fixtureBasicInfo(map[string]any{"id": 7001}), fixtureBlueprintInfo(map[string]any{"craftCountLimit": 0})...)
	if !reflect.DeepEqual(reqs[0].BasicInfo, want) {
		t.Fatalf("basic info = %v, want %v", reqs[0].BasicInfo, want)
	}
}
