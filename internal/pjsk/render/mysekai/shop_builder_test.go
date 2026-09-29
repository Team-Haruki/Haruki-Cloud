package mysekai

import (
	"strings"
	"testing"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

const shopTestNow int64 = 1800000000000

func playerShopFixture(t *testing.T) (*Controller, ShopQuery, map[string]any) {
	t.Helper()
	c := jp700Controller(map[string][]map[string]any{
		"mysekaiBlueprintShops.json": {
			{"mysekaiBlueprintShopItemLotteryType": "daily", "consumeJewelQuantity": 100, "purchaseLimit": 5},
			{"mysekaiBlueprintShopItemLotteryType": "weekly", "consumeJewelQuantity": 200, "purchaseLimit": 3},
		},
		"mysekaiBlueprints.json": {{"id": 11, "craftTargetId": 21}, {"id": 12, "craftTargetId": 22}, {"id": 13, "craftTargetId": 23}},
		"mysekaiFixtures.json":   {{"id": 21, "name": "桌子", "assetbundleName": "table"}, {"id": 22, "name": "椅子", "assetbundleName": "chair"}, {"id": 23, "name": "灯", "assetbundleName": "lamp"}},
		"mysekaiShops.json": {
			{"id": 101, "mysekaiShopType": "tool", "resourceBoxId": 101, "mysekaiShopExchangeLimitType": "limited_per_mysekai_colorful_pass", "mysekaiShopExchangeLimitValue": 99},
			{"id": 102, "mysekaiShopType": "tool", "resourceBoxId": 102, "mysekaiShopExchangeLimitType": "limited_per_mysekai_colorful_pass", "mysekaiShopExchangeLimitValue": 99},
			{"id": 1, "mysekaiShopType": "material", "resourceBoxId": 1, "mysekaiShopExchangeLimitType": "none"},
		},
		"mysekaiTools.json":               {{"id": 10, "name": "チェーンソー", "assetbundleName": "ax0005"}, {"id": 5, "name": "石掘りマシン", "assetbundleName": "pickax0005"}},
		"mysekaiMaterials.json":           {{"id": 1, "name": "木材", "mysekaiMaterialType": "wood", "iconAssetbundleName": "wood"}},
		"mysekaiMaterialPossessions.json": {{"id": 1, "level": 2, "possessionLimit": 100}},
		"mysekaiShopCosts.json":           {{"mysekaiShopId": 101, "resourceType": "jewel", "quantity": 10}},
	})
	q := ShopQuery{Region: "jp", NowMillis: shopTestNow, ResourceBox: func(id int) []ShopResource {
		switch id {
		case 101:
			return []ShopResource{{"mysekai_tool", 10, 1}}
		case 102:
			return []ShopResource{{"mysekai_tool", 5, 1}}
		case 1:
			return []ShopResource{{"mysekai_material", 1, 3}}
		}
		return nil
	}}
	data := map[string]any{
		"userMysekaiColorfulPass": map[string]any{"expiredAt": shopTestNow + 1000},
		"userMysekaiBlueprintShopItems": []any{
			map[string]any{"mysekaiBlueprintId": 13, "mysekaiBlueprintShopItemLotteryType": "weekly", "seq": 1, "isBought": false},
			map[string]any{"mysekaiBlueprintId": 12, "mysekaiBlueprintShopItemLotteryType": "daily", "seq": 2, "isBought": true},
			map[string]any{"mysekaiBlueprintId": 11, "mysekaiBlueprintShopItemLotteryType": "daily", "seq": 1, "isBought": false},
		},
		"userMysekaiBlueprints":         []any{map[string]any{"mysekaiBlueprintId": 11}},
		"userMysekaiShops":              []any{map[string]any{"mysekaiShopId": 101, "count": 98, "totalCount": 900}, map[string]any{"mysekaiShopId": 102, "count": 99}},
		"userMysekaiGamedata":           map[string]any{"mysekaiMaterialPossessionLevel": 2},
		"userMysekaiMaterialPossession": map[string]any{"quantity": 95},
		"userGamedata":                  map[string]any{"jewel": 0},
	}
	return c, q, data
}

func shopWithData(t *testing.T, c *Controller, data map[string]any) *Controller {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return c.WithMySekaiData(raw)
}

func TestPlayerShopAvailabilityAndOwnership(t *testing.T) {
	c, q, data := playerShopFixture(t)
	request, err := shopWithData(t, c, data).BuildShopRequest(q)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Shops) != 4 {
		t.Fatalf("groups: %+v", request.Shops)
	}
	daily := request.Shops[0]
	if daily.ShopType != "blueprint_daily" || len(daily.Items) != 1 || daily.Items[0].ID != 11 || !*daily.Items[0].Owned || *daily.Items[0].IsBought {
		t.Fatalf("daily: %+v", daily)
	}
	if daily.Items[0].Costs[0].Quantity != 100 || request.Shops[1].Items[0].Costs[0].Quantity != 200 {
		t.Fatal("blueprint prices must follow period rules")
	}
	tool := request.Shops[2].Items
	if len(tool) != 1 || tool[0].ID != 101 || *tool[0].RemainingCount != 1 || *tool[0].ExchangedCount != 98 {
		t.Fatalf("tool: %+v", tool)
	}
	if !strings.HasSuffix(tool[0].ImagePath.First(), "mysekai/thumbnail/tool/ax0005.png") {
		t.Fatal(tool[0].ImagePath)
	}
	for _, group := range request.Shops {
		for _, item := range group.Items {
			for _, cost := range item.Costs {
				if cost.HaveQuantity != nil {
					t.Fatal("must not expose balance")
				}
			}
		}
	}
}

func TestPlayerShopFiltersAndFull(t *testing.T) {
	for _, kind := range []string{"", "blueprint", "tool", "material"} {
		for _, all := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "/available", true: "/all"}[all], func(t *testing.T) {
				c, q, data := playerShopFixture(t)
				q.ShopType = kind
				q.ShowAll = all
				req, err := shopWithData(t, c, data).BuildShopRequest(q)
				if err != nil {
					t.Fatal(err)
				}
				total := 0
				for _, g := range req.Shops {
					if kind != "" && g.ShopType != kind && !(kind == "blueprint" && strings.HasPrefix(g.ShopType, "blueprint_")) {
						t.Fatal(g.ShopType)
					}
					total += len(g.Items)
				}
				want := map[string]int{"": 4, "blueprint": 2, "tool": 1, "material": 1}[kind]
				if all {
					want = map[string]int{"": 6, "blueprint": 3, "tool": 2, "material": 1}[kind]
				}
				if total != want {
					t.Fatalf("got %d items want %d", total, want)
				}
			})
		}
	}
}

func TestPlayerShopPassBoundaryAndCapacity(t *testing.T) {
	c, q, data := playerShopFixture(t)
	data["userMysekaiColorfulPass"] = map[string]any{"expiredAt": shopTestNow}
	req, err := shopWithData(t, c, data).BuildShopRequest(q)
	if err != nil {
		t.Fatal(err)
	}
	if *req.PassActive || len(req.Shops) != 0 {
		t.Fatal("expired pass must exclude all categories")
	}
	q.ShowAll = true
	req, err = shopWithData(t, c, data).BuildShopRequest(q)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range req.Shops {
		for _, item := range g.Items {
			if *item.Available {
				t.Fatal("unavailable pass")
			}
		}
	}
	data["userMysekaiColorfulPass"] = map[string]any{"expiredAt": shopTestNow + 1}
	data["userMysekaiMaterialPossession"] = map[string]any{"quantity": 98}
	q.ShowAll = false
	q.ShopType = "material"
	req, err = shopWithData(t, c, data).BuildShopRequest(q)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Shops) != 0 {
		t.Fatal("must have room for a complete exchange")
	}
	q.ShowAll = true
	req, err = shopWithData(t, c, data).BuildShopRequest(q)
	if err != nil {
		t.Fatal(err)
	}
	if *req.Shops[0].Items[0].MaterialCapacityCount != 0 || *req.Shops[0].Items[0].Available {
		t.Fatal("capacity state missing")
	}
}

func TestPlayerShopMissingDataAndBlueprintOnlyRegion(t *testing.T) {
	c, q, data := playerShopFixture(t)
	delete(data, "userMysekaiShops")
	if _, err := shopWithData(t, c, data).BuildShopRequest(q); err == nil || !strings.Contains(err.Error(), "snapshot missing") {
		t.Fatal(err)
	}
	q.ShopType = "blueprint"
	if _, err := shopWithData(t, c, data).BuildShopRequest(q); err != nil {
		t.Fatal(err)
	}
	delete(data, "userMysekaiBlueprintShopItems")
	if _, err := shopWithData(t, c, data).BuildShopRequest(q); err == nil {
		t.Fatal("missing lineup must not look empty")
	}
	c, q, data = playerShopFixture(t)
	c.masterdata.(*sonarMasterdataSource).lists["mysekaiShops.json"] = nil
	req, err := shopWithData(t, c, data).BuildShopRequest(q)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Shops) != 2 {
		t.Fatal("old regions with blueprints must remain supported")
	}
	c.masterdata.(*sonarMasterdataSource).lists["mysekaiBlueprintShops.json"] = nil
	if _, err := shopWithData(t, c, data).BuildShopRequest(q); err == nil {
		t.Fatal("no regional shop data must be unavailable")
	}
}

func TestShopDrawingCompatibilityLabels(t *testing.T) {
	c, q, data := playerShopFixture(t)
	q.ShowAll = true
	req, err := shopWithData(t, c, data).BuildShopRequest(q)
	if err != nil {
		t.Fatal(err)
	}
	decorateShopRequest(req)
	if !strings.Contains(*req.Shops[0].Items[0].Name, "已持有") || !strings.Contains(*req.Shops[0].Items[1].Name, "本期已购买") {
		t.Fatal("legacy drawing labels missing")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var decoded drawing.MysekaiShopRequest
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Shops[0].Items[0].Owned == nil || !*decoded.Shops[0].Items[0].Owned {
		t.Fatal("new Drawing state missing")
	}
	if strings.Contains(string(raw), "have_quantity") {
		t.Fatal("balance leaked")
	}
}

func TestShopSuiteSnapshotPreservesUserFields(t *testing.T) {
	c, q, data := playerShopFixture(t)
	data["userGamedata"] = map[string]any{"userId": 12345678901234}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := snapshot.NewFromBytes(nil, nil, renderregion.JP, raw, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	req, err := c.WithSnapshot(snap).BuildShopRequest(q)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Shops) != 4 || req.Profile == nil {
		t.Fatalf("suite fields/profile lost: %+v", req)
	}
}

func TestShopCharacterMaterialsIgnoreWarehouseLimit(t *testing.T) {
	c, q, data := playerShopFixture(t)
	c.masterdata.(*sonarMasterdataSource).maps["mysekaiMaterials.json"][1]["mysekaiMaterialType"] = "game_character"
	data["userMysekaiMaterialPossession"] = map[string]any{"quantity": 1000}
	q.ShopType = "material"
	req, err := shopWithData(t, c, data).BuildShopRequest(q)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Shops) != 1 || !*req.Shops[0].Items[0].Available {
		t.Fatal("character material incorrectly counted toward warehouse")
	}
}

func TestShopMissingMasterDataFailsExplicitly(t *testing.T) {
	for _, file := range []string{"mysekaiTools.json", "mysekaiBlueprintShops.json", "mysekaiMaterialPossessions.json"} {
		c, q, data := playerShopFixture(t)
		source := c.masterdata.(*sonarMasterdataSource)
		delete(source.maps, file)
		delete(source.lists, file)
		if _, err := shopWithData(t, c, data).BuildShopRequest(q); err == nil || !strings.Contains(err.Error(), "masterdata missing") {
			t.Fatalf("%s: %v", file, err)
		}
	}
}
