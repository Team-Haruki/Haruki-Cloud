package pjsk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	json "github.com/bytedance/sonic"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	botDB "haruki-cloud/database/bot"
	botcommandlog "haruki-cloud/database/bot/commandlog"
	botdailyrequests "haruki-cloud/database/bot/dailyrequests"
	botenttest "haruki-cloud/database/bot/enttest"
	bothourlyrequests "haruki-cloud/database/bot/hourlyrequests"
	botrequestsranking "haruki-cloud/database/bot/requestsranking"
	pjskenttest "haruki-cloud/database/pjsk/enttest"
	usersenttest "haruki-cloud/database/users/enttest"
	noiseCrypto "haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	commandhandler "haruki-cloud/internal/pjsk/handler"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	rendersk "haruki-cloud/internal/pjsk/render/sk"
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

type botCSBTrackerSource struct {
	botTrackerSource
}

func (botCSBTrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.FormatInt(userID, 10),
			Score:     5_000_001,
			Rank:      1,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "BotCSBUser",
		},
	}, nil
}

func (botCSBTrackerSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{
				UserID:    strconv.FormatInt(userID, 10),
				Score:     5_000_001,
				Rank:      1,
				Timestamp: 1704067200,
			},
			{
				UserID:    strconv.FormatInt(userID, 10),
				Score:     5_005_001,
				Rank:      1,
				Timestamp: 1704070800,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "BotCSBUser",
		},
	}, nil
}

// testBotApp registers bot routes on a fresh Fiber instance.
func testBotApp(t *testing.T, drawingURL string) *fiber.App {
	t.Helper()
	return testBotAppWithDependencies(t, drawingURL, nil, nil)
}

func testBotAppWithBindings(t *testing.T, drawingURL string, bindingService *accountdata.BindingService) *fiber.App {
	t.Helper()
	return testBotAppWithDependencies(t, drawingURL, bindingService, nil)
}

func testBotAppWithDependencies(t *testing.T, drawingURL string, bindingService *accountdata.BindingService, botDBClient *botDB.Client) *fiber.App {
	t.Helper()
	var client *drawing.HarukiDrawingClient
	if drawingURL != "" {
		client = drawing.NewHarukiDrawingClient(drawingURL)
	}
	app := fiber.New()
	runtime := testRenderApp(t, client)
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, botDBClient, nil)
	return app
}

func newBotCommandTestClient(t *testing.T, name string) *botDB.Client {
	t.Helper()
	dsn := fmt.Sprintf("file:bot_pjsk_%s_%d?mode=memory&cache=shared&_fk=1", name, time.Now().UnixNano())
	return botenttest.Open(t, "sqlite3", dsn)
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
	r.Host = "localhost"
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

func TestBotEndpointSuppressesParamEchoByDefault(t *testing.T) {
	app := testBotApp(t, "")
	secretParam := "super-secret-param"

	req := newBotPOSTRequest(botPJSKPath("event"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/查活动",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/查活动 " + secretParam}}},
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
	text := singleTextMessageText(t, body)
	if strings.Contains(text, secretParam) {
		t.Fatalf("expected response to redact param %q, got %q", secretParam, text)
	}
	if !strings.Contains(text, "活动查询参数错误") || !strings.Contains(text, "【查单个活动格式】") {
		t.Fatalf("expected redacted parse error with help text, got %q", text)
	}
}

func TestBotEndpointAllowsParamEchoWhenEnabled(t *testing.T) {
	app := testBotApp(t, "")
	secretParam := "super-secret-param"

	req := newBotPOSTRequest(botPJSKPath("event"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/查活动",
		Message:         onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/查活动 " + secretParam}}},
		EnableParamEcho: true,
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
	text := singleTextMessageText(t, body)
	if !strings.Contains(text, secretParam) {
		t.Fatalf("expected response to echo param %q, got %q", secretParam, text)
	}
}

func TestBotEndpointRecordsDistributedStatisticsAndCommandLog(t *testing.T) {
	ctx := context.Background()
	botClient := newBotCommandTestClient(t, "telemetry")
	t.Cleanup(func() { _ = botClient.Close() })
	app := testBotAppWithDependencies(t, "", testBindingService(t), botClient)

	req := newBotPOSTRequest(botPJSKPath("profile/bind/list"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", PlatformGroupID: "67890",
		Server: "jp", MatchedCommand: "/绑定列表",
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

	rankingRow, err := botClient.RequestsRanking.Query().Where(botrequestsranking.BotIDEQ(11451419)).Only(ctx)
	if err != nil {
		t.Fatalf("load requests ranking: %v", err)
	}
	if rankingRow.Counts != 1 {
		t.Fatalf("unexpected requests ranking counts: got=%d want=1", rankingRow.Counts)
	}

	hourlyRows, err := botClient.HourlyRequests.Query().Where(bothourlyrequests.CountEQ(1)).All(ctx)
	if err != nil {
		t.Fatalf("load hourly requests: %v", err)
	}
	if len(hourlyRows) != 1 {
		t.Fatalf("expected 1 hourly row, got %+v", hourlyRows)
	}

	dailyRows, err := botClient.DailyRequests.Query().Where(botdailyrequests.CountEQ(1)).All(ctx)
	if err != nil {
		t.Fatalf("load daily requests: %v", err)
	}
	if len(dailyRows) != 1 {
		t.Fatalf("expected 1 daily row, got %+v", dailyRows)
	}

	logRow, err := botClient.CommandLog.Query().Only(ctx)
	if err != nil {
		t.Fatalf("load command log: %v", err)
	}
	if logRow.Platform != "qq" || logRow.Pid != testBotID || logRow.Gid != "67890" || logRow.UID != "12345" || logRow.Command != "/绑定列表" {
		t.Fatalf("unexpected command log row: %+v", logRow)
	}
	if logRow.CreatedAt.IsZero() {
		t.Fatalf("expected command log created_at to be set, got %+v", logRow)
	}

	count, err := botClient.CommandLog.Query().
		Where(
			botcommandlog.PlatformEQ("qq"),
			botcommandlog.PidEQ(testBotID),
			botcommandlog.GidEQ("67890"),
			botcommandlog.UIDEQ("12345"),
			botcommandlog.CommandEQ("/绑定列表"),
		).
		Count(ctx)
	if err != nil {
		t.Fatalf("count command logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 matching command log row, got %d", count)
	}
}

func TestResolveBotCommandFallsBackToMessageMatchForCompactTimeZoneCommand(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()

	resolved, err := resolveBotCommand(context.Background(), onebot11.Message{
		{Type: "text", Data: onebot11.TextData{Text: "/pjsktzHKT"}},
	}, "profile/timezone", BotCommandRequest{
		Platform:       "qq",
		PlatformUserID: "12345",
		MatchedCommand: "/pjsktzHKT",
	}, testBotID)
	if err != nil {
		t.Fatalf("resolveBotCommand() error = %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeSetTimeZone {
		t.Fatalf("unexpected mode: %s", resolved.Mode)
	}

	var params accountdata.ProfileSettingsCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !strings.EqualFold(params.TimeZone, "HKT") {
		t.Fatalf("unexpected timezone param: %q", params.TimeZone)
	}
	if resolved.RequesterPlatform != "qq" || resolved.RequesterUserID != "12345" {
		t.Fatalf("unexpected requester info: platform=%q user=%q", resolved.RequesterPlatform, resolved.RequesterUserID)
	}
	if resolved.RequesterBotID != testBotID {
		t.Fatalf("unexpected requester bot id: %q", resolved.RequesterBotID)
	}
}

func TestResolveBotCommandCorrectsShortMatchedCommandToArrestDifficulty(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()

	resolved, err := resolveBotCommand(context.Background(), onebot11.Message{
		{Type: "text", Data: onebot11.TextData{Text: "/逮捕难度 master关闭"}},
	}, "arrest", BotCommandRequest{
		Platform:       "qq",
		PlatformUserID: "12345",
		MatchedCommand: "/逮捕",
	}, testBotID)
	if err != nil {
		t.Fatalf("resolveBotCommand() error = %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeSetArrestDiff {
		t.Fatalf("unexpected mode: %s", resolved.Mode)
	}

	var params accountdata.ProfileSettingsCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if len(params.DifficultyToggles) != 1 {
		t.Fatalf("unexpected toggle count: %d", len(params.DifficultyToggles))
	}
	if params.DifficultyToggles[0].Difficulty != "master" || params.DifficultyToggles[0].Enabled {
		t.Fatalf("unexpected toggle: %+v", params.DifficultyToggles[0])
	}
}

func TestResolveBotCommandCorrectsEventsMatchedAcrossMessageSeparator(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()

	resolved, err := resolveBotCommand(context.Background(), onebot11.Message{
		{Type: "text", Data: onebot11.TextData{Text: "/event saki"}},
	}, "event/list", BotCommandRequest{
		Platform:       "qq",
		PlatformUserID: "12345",
		MatchedCommand: "/events",
	}, testBotID)
	if err != nil {
		t.Fatalf("resolveBotCommand() error = %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != "event-list" {
		t.Fatalf("unexpected mode: %s", resolved.Mode)
	}

	var params map[string]any
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if got, ok := params["character_id"].(float64); !ok || int(got) != 2 {
		t.Fatalf("unexpected character_id: %#v", params["character_id"])
	}
}

func TestResolveBotCommandCorrectsCardsMatchedAcrossMessageSeparator(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()

	resolved, err := resolveBotCommand(context.Background(), onebot11.Message{
		{Type: "text", Data: onebot11.TextData{Text: "/card saki"}},
	}, "card/list", BotCommandRequest{
		Platform:       "qq",
		PlatformUserID: "12345",
		MatchedCommand: "/cards",
	}, testBotID)
	if err != nil {
		t.Fatalf("resolveBotCommand() error = %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != "card-image" {
		t.Fatalf("unexpected mode: %s", resolved.Mode)
	}
	if resolved.Query != "saki" {
		t.Fatalf("unexpected query: %q", resolved.Query)
	}
}

func TestResolveBotCommandRejectsUnrelatedMatchedCommandAcrossEndpoint(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()

	_, err := resolveBotCommand(context.Background(), onebot11.Message{
		{Type: "text", Data: onebot11.TextData{Text: "/card saki"}},
	}, "event/list", BotCommandRequest{
		Platform:       "qq",
		PlatformUserID: "12345",
		MatchedCommand: "/events",
	}, testBotID)
	if err == nil {
		t.Fatal("expected resolveBotCommand() error")
	}
	var validationErr *botValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected botValidationError, got %T: %v", err, err)
	}
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
	assertSingleTextMessageContains(t, body, "没有找到有效的 mysekai 数据")
}

func TestBotEndpointMysekaiTalkListAcceptsMSBCommand(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	runtime.MySekai = rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true})
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("mysekai/talk-list"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/msb",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/msb"}}},
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
	assertSingleTextMessageContains(t, body, "烤森服务未就绪")
}

func TestBotEndpointMysekaiTalkListAcceptsLegacyBlueprintEndpoint(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	runtime.MySekai = rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true})
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("mysekai/blueprint"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/msb",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/msb"}}},
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
	assertSingleTextMessageContains(t, body, "烤森服务未就绪")
}

func TestBotEndpointSKQueryTreatsRequestServerAsExplicitRegion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
	if _, err := bindingService.SetBindingVisible(context.Background(), "qq", "67890", "jp", true); err != nil {
		t.Fatalf("set binding visible: %v", err)
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
	if _, err := bindingService.SetBindingVisible(context.Background(), "qq", "67890", "jp", true); err != nil {
		t.Fatalf("set binding visible: %v", err)
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
	assertSingleTextMessageContains(t, body, "当前没有可推断的活动，请指定活动ID")
}

func TestBotEndpointSKQueryDefaultsToSelfBinding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 1 || req.Ranks[0].Rank != 777 {
			t.Fatalf("expected self-bound rank payload, got %+v", req.Ranks)
		}
		if req.PrevRanks == nil || req.PrevRanks.Rank != 500 {
			t.Fatalf("unexpected prev ranks: %+v", req.PrevRanks)
		}
		if req.NextRanks == nil || req.NextRanks.Rank != 1000 {
			t.Fatalf("unexpected next ranks: %+v", req.NextRanks)
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
		if len(req.Ranks) == 0 {
			t.Fatalf("expected non-empty ranks in speed request")
		}
		foundRank100 := false
		for _, r := range req.Ranks {
			if r.Rank == 100 {
				foundRank100 = true
				break
			}
		}
		if !foundRank100 {
			t.Fatalf("expected rank 100 in speed request, got %+v", req.Ranks)
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
	text := singleTextMessageText(t, body)
	if !strings.Contains(text, wantPart) {
		t.Fatalf("expected text to contain %q, got %q", wantPart, text)
	}
}

func singleTextMessageText(t *testing.T, body []byte) string {
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
	return text
}

func TestBotEndpointSKCheckRoomUsesTrackerPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/check-room" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.CFRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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

func TestBotEndpointSKCheckRoomDefaultsToSelfBinding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/check-room" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.CFRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.Eid != 101 {
			t.Fatalf("unexpected event id: %d", req.Eid)
		}
		if len(req.Ranks) != 1 || req.Ranks[0].Rank != 1 {
			t.Fatalf("expected self-bound check-room payload, got %+v", req.Ranks)
		}
		if req.PrevRank != nil {
			t.Fatalf("unexpected prev rank: %+v", req.PrevRank)
		}
		if req.NextRank == nil || req.NextRank.Rank != 2 {
			t.Fatalf("unexpected next rank: %+v", req.NextRank)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKCHECKSELFPNG"))
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botCSBTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/check-room"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/cf",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/cf event101"}}},
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

func TestBotEndpointSKCheckRoomLiteUsesFixedRanks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/check-room" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.CFRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		wantRanks := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 20, 30, 40, 50, 100}
		if len(req.Ranks) != len(wantRanks) {
			t.Fatalf("unexpected /cfl rank count: %d", len(req.Ranks))
		}
		for i, want := range wantRanks {
			if req.Ranks[i].Rank != want {
				t.Fatalf("unexpected /cfl ranks: %+v", req.Ranks)
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKCHECKLITEPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/check-room"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/cfl",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/cfl event101"}}},
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

func TestBotEndpointSKCheckRoomLegacyCSBCompat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/csb" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.CSBRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.Eid != 101 {
			t.Fatalf("unexpected event id: %d", req.Eid)
		}
		if len(req.Ranks) == 0 {
			t.Fatalf("expected csb trace payload, got empty ranks")
		}
		if req.Ranks[len(req.Ranks)-1].Rank != 1 {
			t.Fatalf("unexpected latest rank payload: %+v", req.Ranks[len(req.Ranks)-1])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKCSBPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	runtime.SK.SetTrackerIntegration(botCSBTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/check-room"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/csb",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/csb event101 1"}}},
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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

func TestBotEndpointProfileTimeZoneCompatReroutesLegacyProfilePath(t *testing.T) {
	app := testBotAppWithBindings(t, "", testBindingService(t))

	req := newBotPOSTRequest(botPJSKPath("profile/check-data"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/pjsktz",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/pjsktz HKT"}}},
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
	assertSingleTextMessage(t, body, "已设置PJSK时区为 Asia/Hong_Kong")
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
	if envelope.Message != "当前接口不允许使用该 matched_command" {
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
	req.Host = "localhost"

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
	req.Host = "localhost"
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
	if !strings.Contains(envelope.Message, "指令清单不可用") {
		t.Fatalf("expected unavailable manifest message, got: %s", envelope.Message)
	}
}

func TestBotNilRenderAppSkipsRegistration(t *testing.T) {
	app := fiber.New()
	RegisterPJSKBotRoutes(app, nil, nil, nil, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/v2/bot/"+testBotID+"/command/manifests", nil)
	req.Host = "localhost"
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
	httpReq.Host = "localhost"
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
