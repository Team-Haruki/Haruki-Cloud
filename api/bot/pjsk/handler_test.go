package pjsk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	pjskenttest "haruki-cloud/database/pjsk/enttest"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/pjsk/render/assets"
	rendersk "haruki-cloud/internal/pjsk/render/sk"
	accountdata "haruki-cloud/internal/pjsk/userdata"
	"haruki-cloud/utils/drawing"
	sekaiapi "haruki-cloud/utils/sekai"

	"github.com/gofiber/fiber/v3"
	_ "github.com/mattn/go-sqlite3"
	zeromessage "github.com/wdvxdr1123/ZeroBot/message"
)

const testBotID = "11451419"

type botBindingValidator struct{}

func (botBindingValidator) GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	return nil, sekaiapi.ErrUserNotFound
}

type botBindingJPValidator struct{}

func (botBindingJPValidator) GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	if strings.EqualFold(server, "jp") {
		return &sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 1234567890,
				Name:   "JPBoundUser",
			},
		}, nil
	}
	return nil, sekaiapi.ErrUserNotFound
}

type botTrackerSource struct{}

func (botTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	score := 3000000 + rank
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    "10001",
			Score:     score,
			Rank:      rank,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "10001",
			Name:   "BotTrackerUser",
		},
	}, nil
}

func (botTrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	score := 5000000 + int(userID%1000)
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    "10002",
			Score:     score,
			Rank:      777,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "10002",
			Name:   "BotTrackerUIDUser",
		},
	}, nil
}

func (botTrackerSource) GetLatestWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	score := 4000000 + rank
	return &sekaiapi.WorldBloomLatestRankingResponse{
		RankData: sekaiapi.WorldBloomRankDataPoint{
			RankDataPoint: sekaiapi.RankDataPoint{
				UserID:    "20001",
				Score:     score,
				Rank:      rank,
				Timestamp: 1704067200,
			},
			CharacterID: &characterID,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "20001",
			Name:   "BotWLTrackerUser",
		},
	}, nil
}

func (botTrackerSource) GetLatestWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	score := 6000000 + int(userID%1000)
	return &sekaiapi.WorldBloomLatestRankingResponse{
		RankData: sekaiapi.WorldBloomRankDataPoint{
			RankDataPoint: sekaiapi.RankDataPoint{
				UserID:    "20002",
				Score:     score,
				Rank:      888,
				Timestamp: 1704067200,
			},
			CharacterID: &characterID,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "20002",
			Name:   "BotWLTrackerUIDUser",
		},
	}, nil
}

func (botTrackerSource) GetRankingScoreGrowth(server string, eventID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	earlier := int64(1704067200)
	diff := int64(interval)
	growthRank1 := 1200
	growthRank100 := 4500
	earlierRank1 := 3000001
	earlierRank100 := 3100000
	return []sekaiapi.ScoreGrowthPoint{
		{
			Rank:             1,
			ScoreLatest:      earlierRank1 + growthRank1,
			ScoreEarlier:     &earlierRank1,
			TimestampLatest:  earlier + diff,
			TimestampEarlier: &earlier,
			TimeDiff:         &diff,
			Growth:           &growthRank1,
		},
		{
			Rank:             100,
			ScoreLatest:      earlierRank100 + growthRank100,
			ScoreEarlier:     &earlierRank100,
			TimestampLatest:  earlier + diff,
			TimestampEarlier: &earlier,
			TimeDiff:         &diff,
			Growth:           &growthRank100,
		},
	}, nil
}

func (botTrackerSource) GetWorldBloomRankingScoreGrowth(server string, eventID, characterID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	return botTrackerSource{}.GetRankingScoreGrowth(server, eventID, interval)
}

func (botTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{
				UserID:    "10001",
				Score:     3000000 + rank,
				Rank:      rank,
				Timestamp: 1704067200,
			},
			{
				UserID:    "10001",
				Score:     3005000 + rank,
				Rank:      rank,
				Timestamp: 1704070800,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "10001",
			Name:   "BotTrackerUser",
		},
	}, nil
}

func (botTrackerSource) TraceWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomTraceRankingResponse, error) {
	return &sekaiapi.WorldBloomTraceRankingResponse{
		RankData: []sekaiapi.WorldBloomRankDataPoint{
			{
				RankDataPoint: sekaiapi.RankDataPoint{
					UserID:    "20001",
					Score:     4000000 + rank,
					Rank:      rank,
					Timestamp: 1704067200,
				},
				CharacterID: &characterID,
			},
			{
				RankDataPoint: sekaiapi.RankDataPoint{
					UserID:    "20001",
					Score:     4005000 + rank,
					Rank:      rank,
					Timestamp: 1704070800,
				},
				CharacterID: &characterID,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "20001",
			Name:   "BotWLTrackerUser",
		},
	}, nil
}

// testBotApp registers bot routes on a fresh Fiber instance.
func testBotApp(t *testing.T, drawingURL string) *fiber.App {
	t.Helper()
	return testBotAppWithBindings(t, drawingURL, nil)
}

func testBotAppWithBindings(t *testing.T, drawingURL string, bindingService *accountdata.BindingService) *fiber.App {
	t.Helper()
	var client *drawing.HarukiDrawingClient
	if drawingURL != "" {
		client = drawing.NewHarukiDrawingClient(drawingURL)
	}
	app := fiber.New()
	runtime := testRenderApp(t, client)
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil)
	return app
}

func testBindingService(t *testing.T) *accountdata.BindingService {
	return testBindingServiceWithValidator(t, botBindingValidator{})
}

func testBindingServiceWithValidator(t *testing.T, validator accountdata.ProfileValidator) *accountdata.BindingService {
	t.Helper()
	pjskClient := pjskenttest.Open(t, "sqlite3", "file:bot_api_bind_test?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = pjskClient.Close() })
	usersClient := usersenttest.Open(t, "sqlite3", "file:bot_api_users_test?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = usersClient.Close() })
	return accountdata.NewBindingService(
		pjskClient,
		identity.NewResolver(usersClient),
		validator,
	)
}

// encodeOneBotPayload creates a Base64-encoded OneBot v11 JSON payload from command text.
func encodeOneBotPayload(command string) string {
	payload := map[string]interface{}{
		"post_type":    "message",
		"message_type": "private",
		"raw_message":  command,
		"message": []map[string]interface{}{
			{"type": "text", "data": map[string]string{"text": command}},
		},
	}
	b, _ := json.Marshal(payload)
	return base64.StdEncoding.EncodeToString(b)
}

// botPJSKPath returns the full URL for a PJSK bot endpoint.
func botPJSKPath(path string) string {
	return "/api/v2/bot/" + testBotID + "/pjsk/" + path
}

func newBotGETRequest(path, commandPayload, matchedCommand string) *http.Request {
	params := url.Values{}
	if commandPayload != "" {
		params.Set(botQueryCommandPayload, commandPayload)
	}
	req, _ := http.NewRequest(http.MethodGet, path+"?"+params.Encode(), nil)
	req.Header.Set(botHeaderPlatform, "qq")
	req.Header.Set(botHeaderPlatformUserID, "12345")
	req.Header.Set(botHeaderPJSKServer, "jp")
	if matchedCommand != "" {
		req.Header.Set(botHeaderMatchedCommand, matchedCommand)
	}
	return req
}

func assertSegmentsText(t *testing.T, got []zeromessage.Segment, want string) {
	t.Helper()
	if text := flattenOneBotSegments(got); text != want {
		t.Fatalf("expected %q, got %q", want, text)
	}
}

// ── Endpoint tests ──────────────────────────────────────────────────────────

func TestBotEndpointGetReturnsImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer srv.Close()
	app := testBotApp(t, srv.URL)

	req := newBotGETRequest(botPJSKPath("card/detail"), encodeOneBotPayload("/卡面 1001"), "/卡面")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	if resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("expected image/png, got %s", resp.Header.Get("Content-Type"))
	}
	if string(body) != "PNGDATA" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestBotEndpointGetReturnsTextJSON(t *testing.T) {
	app := testBotAppWithBindings(t, "", testBindingService(t))

	req := newBotGETRequest(botPJSKPath("profile/bind"), encodeOneBotPayload("/绑定列表"), "/绑定列表")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}

	var envelope renderEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode response: %v raw=%s", err, body)
	}
	if envelope.Message != "你还没有绑定任何PJSK账号" {
		t.Fatalf("unexpected message: %s", envelope.Message)
	}
}

func TestBotEndpointGetWithGroupHeadersReturnsImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGGROUP"))
	}))
	defer srv.Close()
	app := testBotApp(t, srv.URL)

	req := newBotGETRequest(botPJSKPath("card/detail"), encodeOneBotPayload("/卡面 1001"), "/卡面")
	req.Header.Set(botHeaderPlatformGroupID, "67890")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, respBody)
	}
	if string(respBody) != "PNGGROUP" {
		t.Fatalf("unexpected body: %s", respBody)
	}
}

func TestBotEndpointPlainTextFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNG"))
	}))
	defer srv.Close()
	app := testBotApp(t, srv.URL)

	req := newBotGETRequest(botPJSKPath("card/detail"), "/卡面 1001", "/卡面")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, respBody)
	}
}

func TestBotEndpointOneBotMessageArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGSEG"))
	}))
	defer srv.Close()
	app := testBotApp(t, srv.URL)

	// OneBot payload with only message array (no raw_message)
	payload := map[string]interface{}{
		"message": []map[string]interface{}{
			{"type": "text", "data": map[string]string{"text": "/卡面 "}},
			{"type": "text", "data": map[string]string{"text": "1001"}},
		},
	}
	b, _ := json.Marshal(payload)
	encoded := base64.StdEncoding.EncodeToString(b)

	req := newBotGETRequest(botPJSKPath("card/detail"), encoded, "/卡面")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, respBody)
	}
}

func TestBotEndpointSKQueryUsesTrackerPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 2 || req.Ranks[0].Rank != 1 || req.Ranks[1].Rank != 100 {
			t.Fatalf("unexpected ranks: %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKTRACKERPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil)

	req := newBotGETRequest(botPJSKPath("sk/query"), encodeOneBotPayload("/sk event101 100 1"), "/sk")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	if string(body) != "SKTRACKERPNG" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestBotEndpointSKLineUsesTrackerPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/line" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SklRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 2 || req.Ranks[0].Rank != 1 || req.Ranks[1].Rank != 100 {
			t.Fatalf("unexpected ranks: %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKLINEPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil)

	req := newBotGETRequest(botPJSKPath("sk/line"), encodeOneBotPayload("/skl event101 100 1"), "/skl")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	if string(body) != "SKLINEPNG" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestBotEndpointSKQueryUsesTrackerUIDPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 1 {
			t.Fatalf("unexpected ranks len: %d", len(req.Ranks))
		}
		if req.Ranks[0].Rank != 777 {
			t.Fatalf("unexpected rank: %+v", req.Ranks[0])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKUIDPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil)

	req := newBotGETRequest(botPJSKPath("sk/query"), encodeOneBotPayload("/sk event101 1234567890"), "/sk")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	if string(body) != "SKUIDPNG" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestBotEndpointSKLineUsesTrackerUIDPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/line" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SklRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 1 {
			t.Fatalf("unexpected ranks len: %d", len(req.Ranks))
		}
		if req.Ranks[0].Rank != 777 {
			t.Fatalf("unexpected rank: %+v", req.Ranks[0])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKLUIDPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil)

	req := newBotGETRequest(botPJSKPath("sk/line"), encodeOneBotPayload("/skl event101 1234567890"), "/skl")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	if string(body) != "SKLUIDPNG" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestBotEndpointSKQueryUsesTrackerAtBindingPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 1 || req.Ranks[0].Rank != 777 {
			t.Fatalf("unexpected ranks: %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKATPNG"))
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "67890", "1234567890"); err != nil {
		t.Fatalf("bind test account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil)

	payload := map[string]interface{}{
		"post_type":    "message",
		"message_type": "group",
		"message": []map[string]interface{}{
			{"type": "text", "data": map[string]string{"text": "/sk event101 "}},
			{"type": "at", "data": map[string]string{"qq": "67890"}},
		},
	}
	b, _ := json.Marshal(payload)
	encoded := base64.StdEncoding.EncodeToString(b)

	req := newBotGETRequest(botPJSKPath("sk/query"), encoded, "/sk")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	if string(body) != "SKATPNG" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestBotEndpointSKSpeedUsesTrackerPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/speed" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SpeedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.EventID != 101 {
			t.Fatalf("unexpected event id: %d", req.EventID)
		}
		if req.RequestType != "tracker" {
			t.Fatalf("unexpected request type: %s", req.RequestType)
		}
		if req.Period <= 0 {
			t.Fatalf("unexpected period: %d", req.Period)
		}
		if len(req.Ranks) != 2 || req.Ranks[0].Rank != 1 || req.Ranks[1].Rank != 100 {
			t.Fatalf("unexpected ranks: %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKSPEEDPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil)

	req := newBotGETRequest(botPJSKPath("sk/speed"), encodeOneBotPayload("/sks event101 100 1"), "/sks")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	if string(body) != "SKSPEEDPNG" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestBotEndpointSKCheckRoomUsesTrackerPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/check-room" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.CFRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.Eid != 101 {
			t.Fatalf("unexpected event id: %d", req.Eid)
		}
		if len(req.Ranks) != 2 || req.Ranks[0].Rank != 1 || req.Ranks[1].Rank != 100 {
			t.Fatalf("unexpected ranks: %+v", req.Ranks)
		}
		if req.AggregateAt <= 0 {
			t.Fatalf("aggregate_at should be set")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKCHECKPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil)

	req := newBotGETRequest(botPJSKPath("sk/check-room"), encodeOneBotPayload("/cf event101 100 1"), "/cf")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	if string(body) != "SKCHECKPNG" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestBotEndpointSKRankTraceUsesTrackerPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/rank-trace" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.RankTraceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.EventID != 101 {
			t.Fatalf("unexpected event id: %d", req.EventID)
		}
		if req.TargetRank != 100 {
			t.Fatalf("unexpected target rank: %d", req.TargetRank)
		}
		if len(req.Ranks) == 0 {
			t.Fatalf("rank trace points should not be empty")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKTRACEPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil)

	req := newBotGETRequest(botPJSKPath("sk/rank-trace"), encodeOneBotPayload("/skt event101 100"), "/skt")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	if string(body) != "SKTRACEPNG" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestBotEndpointWrongCommandRejects400(t *testing.T) {
	app := testBotApp(t, "")

	// /卡面 resolves to card-detail, but we send it to card/list
	req := newBotGETRequest(botPJSKPath("card/list"), encodeOneBotPayload("/卡面 1001"), "/卡面")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", resp.StatusCode, respBody)
	}

	var envelope renderEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		t.Fatalf("decode response: %v raw=%s", err, respBody)
	}
	if envelope.Message != "command does not match this endpoint" {
		t.Fatalf("unexpected message: %s", envelope.Message)
	}
}

func TestBotEndpointEmptyCommandRejects400(t *testing.T) {
	app := testBotApp(t, "")

	req := newBotGETRequest(botPJSKPath("card/detail"), "", "/卡面")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBotEndpointUnknownMatchedCommandRejects400(t *testing.T) {
	app := testBotApp(t, "")

	req := newBotGETRequest(botPJSKPath("card/detail"), encodeOneBotPayload("/卡面 1001"), "/不存在的命令")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBotEndpointMissingMatchedCommandRejects400(t *testing.T) {
	app := testBotApp(t, "")

	req := newBotGETRequest(botPJSKPath("card/detail"), encodeOneBotPayload("/卡面 1001"), "")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBotEndpointPostRejected(t *testing.T) {
	app := testBotApp(t, "")

	req, _ := http.NewRequest(http.MethodPost, botPJSKPath("card/detail"), strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404/405 for POST, got %d", resp.StatusCode)
	}
}

func TestBotManifestEndpoint(t *testing.T) {
	app := testBotApp(t, "")

	req, _ := http.NewRequest(http.MethodGet, "/api/v2/bot/"+testBotID+"/command/manifests", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	// With nil botDBClient, the endpoint returns 501 Not Implemented.
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no DB client), got %d body=%s", resp.StatusCode, respBody)
	}

	var envelope renderEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		t.Fatalf("decode manifest: %v raw=%s", err, respBody)
	}
	if !strings.Contains(envelope.Message, "not available") {
		t.Fatalf("expected 'not available' message, got: %s", envelope.Message)
	}
}

func TestBotNilRenderAppSkipsRegistration(t *testing.T) {
	app := fiber.New()
	RegisterPJSKBotRoutes(app, nil, nil, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/v2/bot/"+testBotID+"/command/manifests", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 (no routes), got %d", resp.StatusCode)
	}
}

// ── decodeCommand unit tests ────────────────────────────────────────────────

func TestDecodeCommandOneBotRawMessage(t *testing.T) {
	payload := `{"raw_message":"/卡面 1001","message_type":"private"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))

	result, err := decodeCommand(encoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	assertSegmentsText(t, result, "/卡面 1001")
}

func TestDecodeCommandOneBotMessageSegments(t *testing.T) {
	payload := `{"message":[{"type":"text","data":{"text":"/查卡 "}},{"type":"text","data":{"text":"初音"}}]}`
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))

	result, err := decodeCommand(encoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	assertSegmentsText(t, result, "/查卡 初音")
}

func TestDecodeCommandOneBotMessageSegmentsWithAt(t *testing.T) {
	payload := `{"message":[{"type":"text","data":{"text":"/sk "}},{"type":"at","data":{"qq":"12345"}},{"type":"text","data":{"text":" 20"}}]}`
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))

	result, err := decodeCommand(encoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	assertSegmentsText(t, result, "/sk @12345 20")
}

func TestDecodeCommandOneBotMessageSegmentsPreferredOverRawMessage(t *testing.T) {
	payload := `{"raw_message":"/sk @测试用户","message":[{"type":"text","data":{"text":"/sk "}},{"type":"at","data":{"qq":"12345"}}]}`
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))

	result, err := decodeCommand(encoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	assertSegmentsText(t, result, "/sk @12345")
}

func TestDecodeCommandPlainTextFallback(t *testing.T) {
	result, err := decodeCommand("/卡面 1001")
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	assertSegmentsText(t, result, "/卡面 1001")
}
