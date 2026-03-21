package pjsk

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"haruki-cloud/config"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	rendergacha "haruki-cloud/internal/pjsk/render/gacha"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"

	"github.com/gofiber/fiber/v3"
)

type renderEnvelope struct {
	Status  int             `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type routeEventSource struct {
	region renderregion.Value
	events []*masterdata.Event
}

func (s *routeEventSource) DefaultRegion() renderregion.Value { return s.region }

func (s *routeEventSource) GetEventByID(id int) (*masterdata.Event, error) {
	for _, eventInfo := range s.events {
		if eventInfo.ID == id {
			copy := *eventInfo
			return &copy, nil
		}
	}
	return nil, fiber.ErrNotFound
}

func (s *routeEventSource) GetEventByCardID(cardID int) (*masterdata.Event, error) {
	return nil, fiber.ErrNotFound
}

func (s *routeEventSource) GetEvents() []*masterdata.Event {
	out := make([]*masterdata.Event, 0, len(s.events))
	for _, eventInfo := range s.events {
		copy := *eventInfo
		out = append(out, &copy)
	}
	return out
}

func (s *routeEventSource) GetEventCards(eventID int) ([]*masterdata.Card, error) {
	return nil, nil
}

func (s *routeEventSource) GetEventBannerCharacterID(eventID int) (int, error) {
	return 0, fiber.ErrNotFound
}

func (s *routeEventSource) GetEventDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	return nil, nil
}

func (s *routeEventSource) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	return nil, fiber.ErrNotFound
}

func (s *routeEventSource) GetBanEvents(charID int) []*masterdata.Event { return nil }

func (s *routeEventSource) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom {
	return nil
}

func (s *routeEventSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	return nil, fiber.ErrNotFound
}

type routeGachaSource struct {
	region   renderregion.Value
	gachas   []*masterdata.Gacha
	gachaMap map[int]*masterdata.Gacha
	cardMap  map[int]*masterdata.Card
}

func (s *routeGachaSource) DefaultRegion() renderregion.Value { return s.region }

func (s *routeGachaSource) GetGachaByID(id int) (*masterdata.Gacha, error) {
	if item, ok := s.gachaMap[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func (s *routeGachaSource) GetGachas() []*masterdata.Gacha {
	out := make([]*masterdata.Gacha, 0, len(s.gachas))
	for _, item := range s.gachas {
		copy := *item
		out = append(out, &copy)
	}
	return out
}

func (s *routeGachaSource) GetCardByID(id int) (*masterdata.Card, error) {
	if item, ok := s.cardMap[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fiber.ErrNotFound
}

func TestPJSKEventBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/event/list/build", `{"region":"jp","include_past":true,"include_future":true}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			EventInfo []struct {
				EventName string `json:"event_name"`
			} `json:"event_info"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != eventListDrawingEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if len(data.Payload.EventInfo) != 1 || data.Payload.EventInfo[0].EventName != "JP Event" {
		t.Fatalf("unexpected payload: %+v", data.Payload.EventInfo)
	}
}

func TestPJSKEventRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != eventListDrawingEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/event/list/render", strings.NewReader(`{"region":"jp","include_past":true,"include_future":true}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "PNGDATA" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKGachaBuildRouteReturnsBuiltPayload(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/gacha/list/build", `{"region":"jp","include_past":true}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			Gachas []struct {
				Name string `json:"name"`
			} `json:"gachas"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != gachaListDrawingEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if len(data.Payload.Gachas) != 1 || data.Payload.Gachas[0].Name != "JP Gacha" {
		t.Fatalf("unexpected payload: %+v", data.Payload.Gachas)
	}
}

func TestPJSKGachaRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != gachaListDrawingEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("GACHAPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(app, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/gacha/list/render", strings.NewReader(`{"region":"jp","include_past":true}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "GACHAPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKRenderRoutesRequireAuthorizationWhenConfigured(t *testing.T) {
	oldAuth := config.Cfg.Backend.AcceptAuthorization
	oldUA := config.Cfg.Backend.AcceptUserAgent
	config.Cfg.Backend.AcceptAuthorization = "Bearer internal-token"
	config.Cfg.Backend.AcceptUserAgent = ""
	defer func() {
		config.Cfg.Backend.AcceptAuthorization = oldAuth
		config.Cfg.Backend.AcceptUserAgent = oldUA
	}()

	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(app, runtime)

	resp := requestRenderRoute(t, app, http.MethodPost, "/internal/pjsk/event/list/build", `{"region":"jp","include_past":true,"include_future":true}`)
	if resp.Status != fiber.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got status=%d message=%s", resp.Status, resp.Message)
	}
}

func requestRenderRoute(t *testing.T, app *fiber.App, method, path, body string) renderEnvelope {
	t.Helper()

	req, err := http.NewRequest(method, path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	var envelope renderEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode response: %v raw=%s", err, string(payload))
	}
	return envelope
}

func testRenderApp(t *testing.T, drawingClient *drawing.HarukiDrawingClient) *renderapp.App {
	t.Helper()

	eventSource := &routeEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{
			{ID: 1, EventType: "marathon", Name: "JP Event", AssetBundleName: "jp_event", StartAt: 100, AggregateAt: 200},
		},
	}
	eventController := renderevent.NewController(eventSource, drawingClient, assets.NewAssetHelper("", nil))

	gachaItem := &masterdata.Gacha{
		ID:              1,
		Name:            "JP Gacha",
		GachaType:       "ceil",
		AssetBundleName: "jp_gacha",
		StartAt:         100,
		EndAt:           200,
		GachaDetails: []masterdata.GachaDetail{
			{CardID: 1001, Weight: 100},
		},
		GachaCardRarityRates: []masterdata.GachaCardRarityRate{
			{CardRarityType: "rarity_4", LotteryType: "normal", Rate: 100},
		},
	}
	gachaSource := &routeGachaSource{
		region: renderregion.JP,
		gachas: []*masterdata.Gacha{gachaItem},
		gachaMap: map[int]*masterdata.Gacha{
			1: gachaItem,
		},
		cardMap: map[int]*masterdata.Card{
			1001: {ID: 1001, CardRarityType: "rarity_4", Attr: "cool", AssetBundleName: "card_1001"},
		},
	}
	gachaController := rendergacha.NewController(gachaSource, drawingClient, assets.NewAssetHelper("", nil))

	return &renderapp.App{
		Drawing: drawingClient,
		Assets:  assets.NewAssetHelper("", nil),
		Events:  eventController,
		Gachas:  gachaController,
	}
}
