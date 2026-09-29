package handler

import (
	"context"
	json "haruki-cloud/internal/jsonutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	"haruki-cloud/internal/pjsk/render/provider"
)

func writeMysekaiJP700JSON(t *testing.T, dir, name string, data any) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// newMysekaiJP700App serves JP shop rows from <root>/jp and an EN master
// without them from <root>/en.
func newMysekaiJP700App(t *testing.T, drawingURL string) *renderapp.App {
	t.Helper()
	root := t.TempDir()
	jp := filepath.Join(root, "jp")
	writeMysekaiJP700JSON(t, jp, "mysekaiShops.json", []map[string]any{
		{"id": 1, "mysekaiShopType": "material", "seq": 1, "resourceBoxId": 1, "mysekaiShopExchangeLimitType": "none"},
	})
	writeMysekaiJP700JSON(t, jp, "mysekaiBlueprintShops.json", []map[string]any{
		{"mysekaiBlueprintShopItemLotteryType": "daily", "consumeJewelQuantity": 100, "purchaseLimit": 5},
		{"mysekaiBlueprintShopItemLotteryType": "weekly", "consumeJewelQuantity": 200, "purchaseLimit": 3},
	})
	writeMysekaiJP700JSON(t, jp, "mysekaiShopCosts.json", []map[string]any{
		{"id": 1, "mysekaiShopId": 1, "seq": 1, "resourceType": "jewel", "quantity": 100},
	})
	writeMysekaiJP700JSON(t, jp, "mysekaiMaterials.json", []map[string]any{{"id": 1, "iconAssetbundleName": "mat_1"}})
	writeMysekaiJP700JSON(t, filepath.Join(root, "en"), "mysekaiMaterials.json", []map[string]any{{"id": 1, "iconAssetbundleName": "mat_1"}})
	writeMysekaiJP700JSON(t, jp, "mysekaiMaterialPossessions.json", []map[string]any{{"id": 1, "level": 1, "possessionLimit": 100}})
	writeMysekaiJP700JSON(t, jp, "resourceBoxes.json", []map[string]any{
		{"id": 1, "resourceBoxPurpose": "mysekai_shop", "details": []map[string]any{{"resourceType": "mysekai_material", "resourceId": 1, "resourceQuantity": 1}}},
		{"id": 1, "resourceBoxPurpose": "mysekai_recycle", "details": []map[string]any{{"resourceType": "coin", "resourceQuantity": 999}}},
	})
	return &renderapp.App{
		Provider: provider.NewLocalProvider(jp, renderregion.JP),
		MySekai:  rendermysekai.NewController(drawing.NewHarukiDrawingClient(drawingURL), nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{LocalDir: root, AllowFallback: true}),
	}
}

func executeMysekaiJP700(app *renderapp.App, mode, region string) error {
	_, err := executeMysekai(NewRequestContext(context.Background(), &CommandRequest{
		Module: parser.ModuleMysekai,
		Mode:   mode,
		Region: region,
		Params: []byte(`{}`),
	}, app))
	return err
}

func TestExecuteMysekaiShopRendersJPShop(t *testing.T) {
	var got drawing.MysekaiShopRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/mysekai/shop" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode shop request: %v", err)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	app := newMysekaiJP700App(t, server.URL)
	app.MySekai = app.MySekai.WithMySekaiData([]byte(`{"userMysekaiShops":[],"userMysekaiColorfulPass":{"expiredAt":4102444800000},"userMysekaiGamedata":{"mysekaiMaterialPossessionLevel":1},"userMysekaiMaterialPossession":{"quantity":0}}`))

	// Drawing without the endpoint yet: a clear message instead of a raw 404.
	_, err := executeMysekai(NewRequestContext(context.Background(), &CommandRequest{
		Module: parser.ModuleMysekai, Mode: mySekaiShopCommand, Region: "jp", Params: []byte(`{"shop_type":"material"}`),
	}, app))
	assertReplayErrorText(t, err, "绘图服务暂不支持该功能，请稍后再试")
	if len(got.Shops) != 1 || got.Shops[0].ShopType != "material" || len(got.Shops[0].Items) != 1 || got.Shops[0].Items[0].Costs[0].Quantity != 100 {
		t.Fatalf("shop request = %+v", got)
	}
	if got.Shops[0].Items[0].Quantity != 1 || !strings.Contains(got.Shops[0].Items[0].ImagePath.First(), "mat_1.png") {
		t.Fatal("resource box purpose mismatch")
	}
}

func TestExecuteMysekaiNewViewsOnOldRegionReplyNotOpen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("an old region must not reach Drawing: %s", r.URL.Path)
	}))
	defer server.Close()
	app := newMysekaiJP700App(t, server.URL)

	assertReplayErrorText(t, executeMysekaiJP700(app, mySekaiShopCommand, "en"), "该区服暂未开放烤森商店")
	assertReplayErrorText(t, executeMysekaiJP700(app, mySekaiBlueprintTermCommand, "en"), "该区服暂无限时蓝图数据")
}

func TestMysekaiJP700CommandsParse(t *testing.T) {
	for _, tc := range []struct {
		handler HarukiSekaiCommandHandler
		mode    string
	}{
		{sekaiHandlers{}.MysekaiShopHandle(), mySekaiShopCommand},
		{sekaiHandlers{}.MysekaiBlueprintTermHandle(), mySekaiBlueprintTermCommand},
	} {
		req, err := tc.handler.handleFunc(mysekaiEdgeContext(""))
		if err != nil || req == nil || req.Mode != tc.mode {
			t.Fatalf("%s parse = %+v, %v", tc.handler.Path, req, err)
		}
	}
	req, err := sekaiHandlers{}.MysekaiBlueprintTermHandle().handleFunc(mysekaiEdgeContext("all"))
	if err != nil || !strings.Contains(string(req.Params), `"show_all":true`) {
		t.Fatalf("blueprint term all = %+v, %v", req, err)
	}
}

func TestMysekaiShopParameters(t *testing.T) {
	for _, tc := range []struct {
		args, kind string
		all        bool
	}{
		{"", "blueprint", false}, {"ALL", "", true}, {"全部", "", true}, {"full", "", true}, {"工具 全部", "tool", true}, {"full BLUEPRINT", "blueprint", true}, {"素材", "material", false}, {"材料 all", "material", true}, {"tool", "tool", false}, {"material", "material", false}, {"蓝图", "blueprint", false},
	} {
		req, err := sekaiHandlers{}.MysekaiShopHandle().handleFunc(mysekaiEdgeContext(tc.args))
		if err != nil {
			t.Fatal(err)
		}
		var query rendermysekai.ShopQuery
		if err := json.Unmarshal(req.Params, &query); err != nil {
			t.Fatal(err)
		}
		if query.ShopType != tc.kind || query.ShowAll != tc.all {
			t.Fatalf("%q: %+v", tc.args, query)
		}
	}
	for _, args := range []string{"unknown", "工具 材料", "alltool", "全部 蓝图 material"} {
		if _, err := parseMysekaiShopArgs(args); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
}

func TestMysekaiShopRequiresPlayerData(t *testing.T) {
	app := newMysekaiJP700App(t, "")
	assertReplayErrorText(t, executeMysekaiJP700(app, mySekaiShopCommand, "jp"), newMySekaiDataNotFoundReplayError().Error())
	app.MySekai = app.MySekai.WithMySekaiData([]byte(`{"userMysekaiColorfulPass":null}`))
	assertReplayErrorText(t, executeMysekaiJP700(app, mySekaiShopCommand, "jp"), "上传的数据缺少烤森商店信息，请重新上传完整游戏数据后再试")
}

func TestMysekaiShopResolvesMergedSnapshot(t *testing.T) {
	app := newMysekaiJP700App(t, "")
	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(context.Background(), "qq", "42", "12345678901234"); err != nil {
		t.Fatal(err)
	}
	source := &runtimeSnapshotProviderStub{snapshot: &runtimeSnapshotStub{
		rawBytes: []byte(`{"userMysekaiBlueprintShopItems":[],"userMysekaiBlueprints":[],"userMysekaiShops":[],"userMysekaiColorfulPass":null,"userMysekaiGamedata":{"mysekaiMaterialPossessionLevel":1},"userMysekaiMaterialPossession":{"quantity":0}}`),
	}}
	app.Bindings = service
	app.Config.UserSnapshot.AllowFallback = true
	app.Snapshots = source
	rc := NewRequestContext(context.Background(), &CommandRequest{Module: parser.ModuleMysekai, Mode: mySekaiShopCommand, Region: "jp", Params: []byte(`{"mode":"self","platform":"qq","platform_user_id":"42"}`)}, app)
	message, err := executeMysekai(rc)
	if err != nil {
		t.Fatal(err)
	}
	if len(message) == 0 || source.resolveCount != 1 || len(source.resolveNeedFlags) != 1 || !source.resolveNeedFlags[0] {
		t.Fatalf("merged routing: count=%d flags=%v message=%v", source.resolveCount, source.resolveNeedFlags, message)
	}
}
