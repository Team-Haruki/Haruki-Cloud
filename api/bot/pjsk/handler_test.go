package pjsk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/onebot11"
	pjskenttest "haruki-cloud/database/pjsk/enttest"
	usersenttest "haruki-cloud/database/users/enttest"
	noiseCrypto "haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/pjsk/render/assets"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	rendersk "haruki-cloud/internal/pjsk/render/sk"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	"github.com/gofiber/fiber/v3"
	_ "github.com/mattn/go-sqlite3"
	noiseMP "github.com/shamaton/msgpack/v3"
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

func (botTrackerSource) GetUserEventData(server string, eventID int, userID int64) (*sekaiapi.UserEventData, error) {
	return &sekaiapi.UserEventData{
		UserID: strconv.FormatInt(userID, 10),
		Name:   "BotTrackerEventUser",
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

func (botTrackerSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{
				UserID:    strconv.FormatInt(userID, 10),
				Score:     5000000 + int(userID%1000),
				Rank:      777,
				Timestamp: 1704067200,
			},
			{
				UserID:    strconv.FormatInt(userID, 10),
				Score:     5005000 + int(userID%1000),
				Rank:      777,
				Timestamp: 1704070800,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "BotTrackerUIDUser",
		},
	}, nil
}

func (botTrackerSource) TraceWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomTraceRankingResponse, error) {
	return &sekaiapi.WorldBloomTraceRankingResponse{
		RankData: []sekaiapi.WorldBloomRankDataPoint{
			{
				RankDataPoint: sekaiapi.RankDataPoint{
					UserID:    strconv.FormatInt(userID, 10),
					Score:     6000000 + int(userID%1000),
					Rank:      888,
					Timestamp: 1704067200,
				},
				CharacterID: &characterID,
			},
			{
				RankDataPoint: sekaiapi.RankDataPoint{
					UserID:    strconv.FormatInt(userID, 10),
					Score:     6005000 + int(userID%1000),
					Rank:      888,
					Timestamp: 1704070800,
				},
				CharacterID: &characterID,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "BotWLTrackerUIDUser",
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
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)
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

// botPJSKPath returns the full URL for a PJSK bot endpoint.
func botPJSKPath(path string) string {
	return "/api/v2/bot/" + testBotID + "/pjsk/" + path
}

func newBotPOSTRequest(path string, req BotCommandRequest) *http.Request {
	body, _ := json.Marshal(req)
	r, _ := http.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func decodeSuccessMessage(t *testing.T, body []byte) onebot11.Message {
	t.Helper()
	var envelope renderEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode response: %v raw=%s", err, body)
	}
	var message onebot11.Message
	if err := json.Unmarshal(envelope.Data, &message); err != nil {
		t.Fatalf("decode onebot message: %v raw=%s", err, envelope.Data)
	}
	return message
}

func assertSingleImageMessage(t *testing.T, body []byte) {
	t.Helper()
	message := decodeSuccessMessage(t, body)
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("expected single image message, got %+v", message)
	}
	data, ok := message[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected image segment data: %#v", message[0].Data)
	}
	file, _ := data["file"].(string)
	if !strings.HasPrefix(file, "https://image-cache.test/pjsk/") {
		t.Fatalf("unexpected image url: %q", file)
	}
}

func assertSingleTextMessage(t *testing.T, body []byte, want string) {
	t.Helper()
	message := decodeSuccessMessage(t, body)
	if len(message) != 1 || message[0].Type != "text" {
		t.Fatalf("expected single text message, got %+v", message)
	}
	data, ok := message[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected text segment data: %#v", message[0].Data)
	}
	text, _ := data["text"].(string)
	if text != want {
		t.Fatalf("expected text %q, got %q", want, text)
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

	req := newBotPOSTRequest(botPJSKPath("card/detail"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/查卡",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/查卡 1001"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointGetReturnsTextJSON(t *testing.T) {
	app := testBotAppWithBindings(t, "", testBindingService(t))

	req := newBotPOSTRequest(botPJSKPath("profile/bind/list"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/绑定列表",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/绑定列表"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}

	assertSingleTextMessage(t, body, "你还没有绑定任何PJSK账号")
}

func TestBotEndpointGetWithGroupHeadersReturnsImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGGROUP"))
	}))
	defer srv.Close()
	app := testBotApp(t, srv.URL)

	req := newBotPOSTRequest(botPJSKPath("card/detail"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", PlatformGroupID: "67890",
		Server: "jp", MatchedCommand: "/查卡",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/查卡 1001"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, respBody)
	}
	assertSingleImageMessage(t, respBody)
}

func TestBotEndpointPlainTextFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNG"))
	}))
	defer srv.Close()
	app := testBotApp(t, srv.URL)

	req := newBotPOSTRequest(botPJSKPath("card/detail"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/查卡",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/查卡 1001"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, respBody)
	}
	assertSingleImageMessage(t, respBody)
}

func TestBotEndpointOneBotMessageArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGSEG"))
	}))
	defer srv.Close()
	app := testBotApp(t, srv.URL)

	req := newBotPOSTRequest(botPJSKPath("card/detail"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/查卡",
		Message: onebot11.Message{
			{Type: "text", Data: onebot11.TextData{Text: "/查卡 "}},
			{Type: "text", Data: onebot11.TextData{Text: "1001"}},
		},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, respBody)
	}
	assertSingleImageMessage(t, respBody)
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
		if strings.TrimSpace(req.Ranks[0].Name) == "" {
			t.Fatalf("expected rank 1 name to be present, got %+v", req.Ranks[0])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKTRACKERPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101 100 1"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKQuerySupportsRegionPrefixedCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.Region != "cn" {
			t.Fatalf("unexpected region: %s", req.Region)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 2 || req.Ranks[0].Rank != 1 || req.Ranks[1].Rank != 100 {
			t.Fatalf("unexpected ranks: %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKCNPING"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/cnsk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/cnsk event101 100 1"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKQueryAcceptsBaseMatchedCommandForRegionPrefixedInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.Region != "cn" {
			t.Fatalf("unexpected region: %s", req.Region)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKMATCHEDBASE"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/cnsk event101 100"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointMysekaiOverviewAcceptsLegacyResourceEndpoint(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	runtime.MySekai = rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true})
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("mysekai/resource"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/msa",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/msam"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessage(t, body, "drawing client is not configured")
}

func TestBotEndpointSKQueryTreatsRequestServerAsExplicitRegion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.Region != "en" {
			t.Fatalf("unexpected region: %s", req.Region)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKSERVEREN"))
	}))
	defer srv.Close()

	bindings := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindings.Bind(context.Background(), "qq", "12345", "12345678901234"); err != nil {
		t.Fatalf("bind jp account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.Bindings = bindings
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "en", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101 100"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
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
		if strings.TrimSpace(req.Ranks[0].Name) != "" || strings.TrimSpace(req.Ranks[1].Name) != "" {
			t.Fatalf("expected line request to omit names, got %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKLINEPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/line"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/skl",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/skl event101 100 1"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
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
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101 1234567890"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKQueryRankOneShowsPlayerName(t *testing.T) {
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
		if len(req.Ranks) != 1 || req.Ranks[0].Rank != 1 {
			t.Fatalf("unexpected ranks: %+v", req.Ranks)
		}
		if strings.TrimSpace(req.Ranks[0].Name) == "" {
			t.Fatalf("expected rank name to be present, got %+v", req.Ranks[0])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKRANK1PNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101 1"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
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
		if strings.TrimSpace(req.Ranks[0].Name) != "" {
			t.Fatalf("expected line request to omit names, got %+v", req.Ranks[0])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKLUIDPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/line"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/skl",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/skl event101 1234567890"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKLineDefaultsToExpandedRanksAndOmitsNames(t *testing.T) {
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
		if len(req.Ranks) != 34 {
			t.Fatalf("unexpected default line count: %d", len(req.Ranks))
		}
		for _, rank := range req.Ranks {
			if strings.TrimSpace(rank.Name) != "" {
				t.Fatalf("expected line request names to be omitted, got %+v", rank)
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKLDEFAULTPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/line"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/skl",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/skl event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
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
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{
			{Type: "text", Data: onebot11.TextData{Text: "/sk event101 "}},
			{Type: "at", Data: onebot11.AtData{QQ: "67890"}},
		},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKQueryHandlesInlineCQAtInTextSegment(t *testing.T) {
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
		_, _ = w.Write([]byte("SKINLINECQAT"))
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
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{
			{Type: "text", Data: onebot11.TextData{Text: "/sk event101 [CQ:at,qq=67890]"}},
			{Type: "at", Data: onebot11.AtData{QQ: "67890"}},
		},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKQueryReturnsTextWhenTrackerQueryFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("drawing endpoint should not be called on tracker validation error")
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	// Intentionally keep events=nil so /sk without event id triggers tracker-side validation error.
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessageContains(t, body, "event_id is required")
}

func TestBotEndpointSKQueryDefaultsToSelfBinding(t *testing.T) {
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
			t.Fatalf("expected self-bound rank payload, got %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKSELFPNG"))
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKQueryReturnsTextWhenTargetUserIsHidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("drawing endpoint should not be called when target user is hidden")
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "67890", "1234567890"); err != nil {
		t.Fatalf("bind test account: %v", err)
	}
	if _, err := bindingService.SetBindingVisible(context.Background(), "qq", "67890", "jp", false); err != nil {
		t.Fatalf("set binding invisible: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{
			{Type: "text", Data: onebot11.TextData{Text: "/sk [CQ:at,qq=67890]"}},
			{Type: "at", Data: onebot11.AtData{QQ: "67890"}},
		},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessageContains(t, body, "已隐藏个人信息")
}

func TestBotEndpointSKQueryAllowsHiddenSelfBinding(t *testing.T) {
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
			t.Fatalf("expected self-bound rank payload, got %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKHIDDENSELFPNG"))
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}
	if _, err := bindingService.SetBindingVisible(context.Background(), "qq", "12345", "jp", false); err != nil {
		t.Fatalf("set requester binding invisible: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
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
		if req.RequestType != "时" {
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
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/speed"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sks",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sks event101 100 1"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func assertSingleTextMessageContains(t *testing.T, body []byte, wantPart string) {
	t.Helper()
	message := decodeSuccessMessage(t, body)
	if len(message) != 1 || message[0].Type != "text" {
		t.Fatalf("expected single text message, got %+v", message)
	}
	data, ok := message[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected text segment data: %#v", message[0].Data)
	}
	text, _ := data["text"].(string)
	if !strings.Contains(text, wantPart) {
		t.Fatalf("expected text to contain %q, got %q", wantPart, text)
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
		if strings.TrimSpace(req.Ranks[0].Name) == "" || strings.TrimSpace(req.Ranks[1].Name) == "" {
			t.Fatalf("expected rank names to be present, got %+v", req.Ranks)
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
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/check-room"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/cf",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/cf event101 100 1"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
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
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/rank-trace"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/skt",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/skt event101 100"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKPlayerTraceSupportsTwoRanks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/player-trace" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.PlayerTraceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.EventID != 101 {
			t.Fatalf("unexpected event id: %d", req.EventID)
		}
		if len(req.Ranks) == 0 {
			t.Fatalf("first rank trace should not be empty")
		}
		if len(req.Ranks2) == 0 {
			t.Fatalf("second rank trace should not be empty")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKPTRPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/player-trace"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/ptr",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/ptr event101 1 2"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointWrongCommandRejects400(t *testing.T) {
	app := testBotApp(t, "")

	// /卡面 resolves to card/image, but we send it to card/list
	req := newBotPOSTRequest(botPJSKPath("card/list"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/卡面",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/卡面 1001"}}},
	})

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
	if envelope.Message != "matched command is not allowed for this endpoint" {
		t.Fatalf("unexpected message: %s", envelope.Message)
	}
}

func TestBotEndpointEmptyCommandRejects400(t *testing.T) {
	app := testBotApp(t, "")

	req := newBotPOSTRequest(botPJSKPath("card/image"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/卡面",
		// Message is empty
	})

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

	req := newBotPOSTRequest(botPJSKPath("card/image"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/不存在的命令",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/卡面 1001"}}},
	})

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

	req := newBotPOSTRequest(botPJSKPath("card/image"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp",
		// MatchedCommand is empty
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/卡面 1001"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBotEndpointGetRejected(t *testing.T) {
	app := testBotApp(t, "")

	req, _ := http.NewRequest(http.MethodGet, botPJSKPath("card/detail"), nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404/405 for GET, got %d", resp.StatusCode)
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
	RegisterPJSKBotRoutes(app, nil, nil, nil, nil)

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

// TestBotNoiseIKRoundTrip verifies the full Noise IK encrypt→decrypt→process→encrypt→decrypt
// round trip: the client encrypts a MsgPack-encoded BotCommandRequest with Noise IK,
// the server decrypts, processes, and returns a Noise-encrypted MsgPack response.
func TestBotNoiseIKRoundTrip(t *testing.T) {
	serverKP, err := noiseCrypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate server key pair: %v", err)
	}

	var client *drawing.HarukiDrawingClient
	app := fiber.New()
	runtime := testRenderApp(t, client)
	RegisterPJSKBotRoutes(app, runtime, nil, nil, serverKP)

	// Build request payload
	cmdReq := BotCommandRequest{
		Platform:       "qq",
		PlatformUserID: "999",
		Server:         "jp",
		MatchedCommand: "/查卡",
		Message: onebot11.Message{
			{Type: "text", Data: onebot11.TextData{Text: "/查卡 1001"}},
		},
	}
	plaintext, err := noiseMP.Marshal(cmdReq)
	if err != nil {
		t.Fatalf("msgpack marshal: %v", err)
	}

	// Noise NK: client is initiator, knows server public key
	clientNC, err := noiseCrypto.NewInitiator(serverKP.Public)
	if err != nil {
		t.Fatalf("client handshake init: %v", err)
	}
	ciphertext, err := clientNC.EncryptPacket(plaintext)
	if err != nil {
		t.Fatalf("client encrypt: %v", err)
	}

	httpReq, _ := http.NewRequest(http.MethodPost, botPJSKPath("card/detail"), bytes.NewReader(ciphertext))
	httpReq.Header.Set("Content-Type", "application/octet-stream")

	resp, err := app.Test(httpReq)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	// The raw HTTP response body is Noise-encrypted
	if len(respBody) == 0 {
		t.Fatalf("expected non-empty encrypted response")
	}

	// Decrypt the response (client reads server's Message 2)
	decrypted, err := clientNC.DecryptPacket(respBody)
	if err != nil {
		t.Fatalf("client decrypt response: %v", err)
	}

	// Decode MsgPack response
	var envelope map[string]any
	if err := noiseMP.Unmarshal(decrypted, &envelope); err != nil {
		t.Fatalf("msgpack unmarshal response: %v raw_len=%d", err, len(decrypted))
	}

	// The handler should have processed the card query
	message, _ := envelope["message"].(string)
	status := envelope["status"]
	t.Logf("Noise IK response: status=%v message=%s", status, message)

	if message != "ok" && message != "render failed" {
		t.Fatalf("unexpected message: %s (full envelope: %+v)", message, envelope)
	}
}
