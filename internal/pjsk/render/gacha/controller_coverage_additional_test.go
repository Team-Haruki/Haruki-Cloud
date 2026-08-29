package gacha

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"

	_ "github.com/mattn/go-sqlite3"
)

func TestGachaProviderAdapterEmptyDatabase(t *testing.T) {
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:gacha_adapter_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = client.Close() })
	adapter := NewProviderAdapter(provider.NewDatabaseProvider(client, renderregion.JP))
	ctx := context.WithValue(context.Background(), gachaContextKey("adapter"), "request")
	withContext := adapter.WithContext(ctx)
	if withContext == nil || withContext.(*ProviderAdapter).Context() != ctx {
		t.Fatal("adapter did not retain context")
	}
	if (*ProviderAdapter)(nil).WithContext(ctx) != nil {
		t.Fatal("nil adapter returned a source")
	}
	if _, err := adapter.GetGachaByID(1); err == nil {
		t.Fatal("missing gacha returned no error")
	}
	if _, err := adapter.GetGachaByEventID(0); err == nil {
		t.Fatal("zero event ID returned no error")
	}
	if _, err := adapter.GetGachaByEventID(1); err == nil {
		t.Fatal("event without cards returned a gacha")
	}
	if got := adapter.GetGachas(); len(got) != 0 {
		t.Fatalf("empty gachas = %#v", got)
	}
	if _, err := adapter.GetCardByID(1); err == nil {
		t.Fatal("missing card returned no error")
	}
	if _, err := adapter.GetGachaCeilItemAssetbundleName(1); err == nil {
		t.Fatal("missing ceiling item returned no error")
	}
}

func TestGachaRenderEntrypoints(t *testing.T) {
	now := time.Now().UnixMilli()
	source := newTestGachaSource(renderregion.JP)
	item := &masterdata.Gacha{
		ID: 1, Name: "Current Gacha", GachaType: "ceil", AssetBundleName: "gacha_1",
		StartAt: now - 1_000, EndAt: now + 10_000,
		GachaDetails:         []masterdata.GachaDetail{{CardID: 100, Weight: 100}},
		GachaPickups:         []masterdata.GachaPickup{{CardID: 100}},
		GachaCardRarityRates: []masterdata.GachaCardRarityRate{{CardRarityType: "rarity_4", LotteryType: "normal", Rate: 100}},
	}
	source.gachas = []*masterdata.Gacha{item}
	source.gachaByID[item.ID] = item
	source.cardByID[100] = &masterdata.Card{ID: 100, CardRarityType: "rarity_4", Attr: "cool", AssetBundleName: "card_100"}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("gacha-image"))
	}))
	defer server.Close()
	controller := NewController(source, drawing.NewHarukiDrawingClient(server.URL), assets.NewAssetHelper("", nil))
	for name, render := range map[string]func() ([]byte, error){
		"list": func() ([]byte, error) {
			return controller.RenderGachaList(ListQuery{Region: renderregion.JP, IncludePast: true})
		},
		"detail": func() ([]byte, error) {
			return controller.RenderGachaDetail(DetailQuery{Region: renderregion.JP, GachaID: 1})
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := render()
			if err != nil || !bytes.Equal(got, []byte("gacha-image")) {
				t.Fatalf("render = %q, %v", got, err)
			}
		})
	}
	withoutDrawing := NewController(source, nil, nil)
	if _, err := withoutDrawing.RenderGachaList(ListQuery{}); err == nil {
		t.Fatal("list without drawing client succeeded")
	}
	if _, err := withoutDrawing.RenderGachaDetail(DetailQuery{}); err == nil {
		t.Fatal("detail without drawing client succeeded")
	}
	if _, err := controller.RenderGachaList(ListQuery{Region: renderregion.JP, Year: 1900}); err == nil {
		t.Fatal("empty list render unexpectedly succeeded")
	}
	if _, err := controller.RenderGachaDetail(DetailQuery{Region: renderregion.JP, GachaID: 99}); err == nil {
		t.Fatal("missing detail render unexpectedly succeeded")
	}
	if (*Controller)(nil).WithContext(context.Background()) != nil {
		t.Fatal("nil controller WithContext returned a controller")
	}
}

func TestGachaResolutionAndDetailHelperBranches(t *testing.T) {
	now := time.Now().UnixMilli()
	source := newTestGachaSource(renderregion.JP)
	future := &masterdata.Gacha{ID: 3, StartAt: now + 10_000}
	source.gachas = []*masterdata.Gacha{future}
	source.gachaByID[3] = future
	controller := NewController(source, nil, nil)
	for name, query := range map[string]DetailQuery{
		"future explicit":  {Region: renderregion.JP, GachaID: 3},
		"no started":       {Region: renderregion.JP, NegIndex: -1},
		"missing event":    {Region: renderregion.JP, EventID: 99},
		"missing selector": {Region: renderregion.JP},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := controller.resolveDetailQuery(query); err == nil {
				t.Fatal("invalid detail query unexpectedly succeeded")
			}
		})
	}
	if _, _, err := NewController(nil, nil, nil).resolveDetailQuery(DetailQuery{Region: renderregion.JP}); err == nil {
		t.Fatal("missing source unexpectedly resolved")
	}
	if _, err := NewController(nil, nil, nil).BuildGachaListRequest(ListQuery{Region: renderregion.JP}); err == nil {
		t.Fatal("missing source unexpectedly built a list")
	}

	builder := NewBuilder(source, nil)
	if _, err := builder.BuildGachaDetailRequest(DetailQuery{}); err == nil {
		t.Fatal("zero gacha ID unexpectedly built a detail request")
	}
	if _, err := builder.BuildGachaDetailRequest(DetailQuery{GachaID: 99}); err == nil {
		t.Fatal("missing gacha unexpectedly built a detail request")
	}
	withDetail := &masterdata.Gacha{GachaDetails: []masterdata.GachaDetail{{CardID: 5}}}
	withPickup := &masterdata.Gacha{GachaPickups: []masterdata.GachaPickup{{CardID: 6}}}
	if !gachaContainsCard(withDetail, 5) || !gachaContainsCard(withPickup, 6) || gachaContainsCard(withPickup, 7) {
		t.Fatal("gacha card containment failed")
	}
	if builder.buildGachaBannerPath(nil, renderregion.JP) != "" || builder.buildCeilItemIconPath(0, renderregion.JP) != "" || builder.buildCeilItemIconPath(99, renderregion.JP) != "" {
		t.Fatal("empty gacha asset helper returned a path")
	}
	source.ceilByID[1] = "ceil_item"
	if builder.buildCeilItemIconPath(1, renderregion.JP) == "" {
		t.Fatal("valid ceiling item path is empty")
	}
	for raw, want := range map[string]string{"abc": "", "gacha12x34": "34", "tail56": "56"} {
		if got := extractNumericToken(raw); got != want {
			t.Errorf("extractNumericToken(%q) = %q", raw, got)
		}
	}
	past := &masterdata.Gacha{ID: 4, StartAt: now - 1_000}
	source.gachas = []*masterdata.Gacha{past}
	source.gachaByID[past.ID] = past
	if _, _, err := controller.resolveDetailQuery(DetailQuery{Region: renderregion.JP, NegIndex: -2}); err == nil {
		t.Fatal("out-of-range negative index unexpectedly resolved")
	}
}
