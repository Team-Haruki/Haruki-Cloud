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
	writeMysekaiJP700JSON(t, jp, "mysekaiShopCosts.json", []map[string]any{
		{"id": 1, "mysekaiShopId": 1, "seq": 1, "resourceType": "jewel", "quantity": 100},
	})
	writeMysekaiJP700JSON(t, jp, "mysekaiMaterials.json", []map[string]any{{"id": 1, "iconAssetbundleName": "mat_1"}})
	writeMysekaiJP700JSON(t, filepath.Join(root, "en"), "mysekaiMaterials.json", []map[string]any{{"id": 1, "iconAssetbundleName": "mat_1"}})
	return &renderapp.App{
		MySekai: rendermysekai.NewController(drawing.NewHarukiDrawingClient(drawingURL), nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{LocalDir: root, AllowFallback: true}),
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

	// Drawing without the endpoint yet: a clear message instead of a raw 404.
	err := executeMysekaiJP700(app, mySekaiShopCommand, "jp")
	assertReplayErrorText(t, err, "绘图服务暂不支持该功能，请稍后再试")
	if len(got.Shops) != 1 || got.Shops[0].ShopType != "material" || len(got.Shops[0].Items) != 1 || got.Shops[0].Items[0].Costs[0].Quantity != 100 {
		t.Fatalf("shop request = %+v", got)
	}
}

func TestExecuteMysekaiNewViewsOnOldRegionReplyNotOpen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("an old region must not reach Drawing: %s", r.URL.Path)
	}))
	defer server.Close()
	app := newMysekaiJP700App(t, server.URL)

	assertReplayErrorText(t, executeMysekaiJP700(app, mySekaiShopCommand, "en"), "该区服暂未开放烤森商店")
	assertReplayErrorText(t, executeMysekaiJP700(app, mySekaiBulkHarvestCommand, "en"), "该区服暂未开放烤森一键采集")
	assertReplayErrorText(t, executeMysekaiJP700(app, mySekaiBlueprintTermCommand, "en"), "该区服暂无限时蓝图数据")
}

func TestMysekaiJP700CommandsParse(t *testing.T) {
	for _, tc := range []struct {
		handler HarukiSekaiCommandHandler
		mode    string
	}{
		{sekaiHandlers{}.MysekaiShopHandle(), mySekaiShopCommand},
		{sekaiHandlers{}.MysekaiBulkHarvestHandle(), mySekaiBulkHarvestCommand},
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
