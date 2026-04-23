package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"haruki-cloud/api"
	botAPI "haruki-cloud/api/bot"
	chunithmAPI "haruki-cloud/api/chunithm"
	pjskAPI "haruki-cloud/api/pjsk"
	"haruki-cloud/config"
	botDB "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/dailyrequests"
	"haruki-cloud/database/bot/hourlyrequests"
	"haruki-cloud/database/bot/requestsranking"
	"haruki-cloud/database/bot/user"
	chunithmMainDB "haruki-cloud/database/chunithm/maindb"
	chunithmMusicDB "haruki-cloud/database/chunithm/music"
	pjskDB "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/alias"
	"haruki-cloud/database/pjsk/groupalias"
	"haruki-cloud/database/pjsk/userbinding"
	"haruki-cloud/database/pjsk/userdefaultbinding"
	"haruki-cloud/database/pjsk/userpreference"
	usersDB "haruki-cloud/database/users"
	usersEntity "haruki-cloud/database/users/user"
	"haruki-cloud/utils/crypto"
	"haruki-cloud/utils/query"

	"github.com/gofiber/fiber/v3"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

type httpEnvelope struct {
	Status  int             `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type mockTurnstile struct{}

func (m *mockTurnstile) VerifyToken(token, remoteIP string) (bool, error) {
	if token == "deny" {
		return false, nil
	}
	return true, nil
}

type mockSMTP struct {
	lastQQ   int64
	lastCode string
}

func (m *mockSMTP) SendVerificationCode(qqNumber int64, code string) error {
	m.lastQQ = qqNumber
	m.lastCode = code
	return nil
}

type redisStoreAdapter struct {
	client *redis.Client
}

func (s *redisStoreAdapter) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s *redisStoreAdapter) Get(ctx context.Context, key string) (string, error) {
	return s.client.Get(ctx, key).Result()
}

func (s *redisStoreAdapter) Del(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

func TestFullIntegrationWithPostgresAndRedis(t *testing.T) {
	env := loadIntegrationEnv(t)

	ctx := context.Background()
	redisClient := redis.NewClient(&redis.Options{Addr: env.redisAddr})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping failed: %v", err)
	}
	defer func() { _ = redisClient.Close() }()
	if err := redisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("redis flush failed: %v", err)
	}

	botClient := mustOpenBotClient(t, env.botURL)
	defer func() { _ = botClient.Close() }()
	chuniMainClient := mustOpenChuniMainClient(t, env.chuniMainURL)
	defer func() { _ = chuniMainClient.Close() }()
	chuniMusicClient := mustOpenChuniMusicClient(t, env.chuniMusicURL)
	defer func() { _ = chuniMusicClient.Close() }()
	pjskClient := mustOpenPJSKClient(t, env.pjskURL)
	defer func() { _ = pjskClient.Close() }()
	usersClient := mustOpenUsersClient(t, env.usersURL)
	defer func() { _ = usersClient.Close() }()

	mustCreateSchemas(t, ctx, botClient, chuniMainClient, chuniMusicClient, pjskClient, usersClient)
	mustResetData(t, ctx, botClient, chuniMainClient, chuniMusicClient, pjskClient, usersClient)

	prevCfg := config.Cfg
	config.Cfg.Backend.AcceptAuthorization = "Bearer integration-token"
	config.Cfg.Backend.AcceptUserAgent = ""
	config.Cfg.Backend.APICacheTTL = time.Minute
	config.Cfg.HarukiBotDB.CredentialSignToken = "integration-credential-sign"
	config.Cfg.HarukiBotDB.SessionSignToken = "integration-session-sign"
	config.Cfg.HarukiBotDB.SessionTTLDays = 7
	t.Cleanup(func() {
		config.Cfg = prevCfg
	})

	seedCoreData(t, ctx, chuniMainClient, chuniMusicClient, pjskClient, usersClient)

	t.Run("QueryPackage", func(t *testing.T) {
		qc := query.NewClient(chuniMainClient, chuniMusicClient, pjskClient, usersClient)

		aliasToID, err := qc.GetChunithmMusicIDByAlias(ctx, "it-song")
		if err != nil || len(aliasToID.MatchIDs) != 1 || aliasToID.MatchIDs[0] != 4101 {
			t.Fatalf("GetChunithmMusicIDByAlias failed: resp=%+v err=%v", aliasToID, err)
		}

		batch, err := qc.QueryChunithmMusicDataBatch(ctx, []int{4101, 9999}, "v1")
		if err != nil {
			t.Fatalf("QueryChunithmMusicDataBatch failed: %v", err)
		}
		if batch[4101].Info.Title != "Integration Song" || batch[9999].Info.Title != "Unknown" {
			t.Fatalf("unexpected batch result: %+v", batch)
		}

		groupAliasResp, err := qc.GetPJSKGroupAliasToID(ctx, "qq", "it-group", "character", "miku")
		if err != nil || len(groupAliasResp.MatchIDs) != 1 || groupAliasResp.MatchIDs[0] != 7301 {
			t.Fatalf("GetPJSKGroupAliasToID failed: resp=%+v err=%v", groupAliasResp, err)
		}

		prefResp, err := qc.GetPJSKPreference(ctx, 610001, "theme")
		if err != nil || prefResp.Option == nil || prefResp.Option.Value != "light" {
			t.Fatalf("GetPJSKPreference failed: resp=%+v err=%v", prefResp, err)
		}
		allPrefs, err := qc.GetPJSKPreferences(ctx, 610001)
		if err != nil || len(allPrefs.Options) == 0 {
			t.Fatalf("GetPJSKPreferences failed: resp=%+v err=%v", allPrefs, err)
		}

		chuniDefault, err := qc.GetChunithmDefaultServer(ctx, 610001)
		if err != nil || chuniDefault.Server != "jp" {
			t.Fatalf("GetChunithmDefaultServer failed: resp=%+v err=%v", chuniDefault, err)
		}
		chuniBinding, err := qc.GetChunithmBinding(ctx, 610001, "jp")
		if err != nil || chuniBinding.AimeID == nil {
			t.Fatalf("GetChunithmBinding failed: resp=%+v err=%v", chuniBinding, err)
		}

		pjskBindings, err := qc.GetPJSKBindings(ctx, 610001, "jp")
		if err != nil || len(pjskBindings.Bindings) != 1 {
			t.Fatalf("GetPJSKBindings failed: resp=%+v err=%v", pjskBindings, err)
		}
		pjskDefault, err := qc.GetPJSKDefaultBinding(ctx, 610001, "default")
		if err != nil || pjskDefault.Binding == nil {
			t.Fatalf("GetPJSKDefaultBinding failed: resp=%+v err=%v", pjskDefault, err)
		}

		userResp, err := qc.GetUserByPlatform(ctx, "qq", "it-user")
		if err != nil || userResp.ID != 610001 {
			t.Fatalf("GetUserByPlatform failed: resp=%+v err=%v", userResp, err)
		}
		userByID, err := qc.GetUserByID(ctx, 610001)
		if err != nil || userByID.UserID != "it-user" {
			t.Fatalf("GetUserByID failed: resp=%+v err=%v", userByID, err)
		}
	})

	t.Run("BotFlowWithMockMail", func(t *testing.T) {
		smtpMock := &mockSMTP{}
		turnstileMock := &mockTurnstile{}
		store := &redisStoreAdapter{client: redisClient}

		userHandler := botAPI.NewUserHandler(botAPI.NewUserServiceWithDependencies(botClient, store, turnstileMock, smtpMock))
		internalHandler := botAPI.NewInternalHandler(botAPI.NewInternalServiceWithStore(botClient, store))
		statisticsHandler := botAPI.NewStatisticsHandler(botAPI.NewStatisticsService(botClient))

		app := fiber.New()
		public := app.Group("/bot")
		public.Post("/send-mail", userHandler.SendMail)
		public.Post("/register", userHandler.Register)
		public.Post("/:bot_id/auth", userHandler.Auth)
		internal := app.Group("/internal/bot", api.VerifyAPIAuthorization())
		internal.Post("/verify-session", internalHandler.VerifySession)
		app.Post("/bot/statistics/record/:botID", api.VerifyAPIAuthorization(), statisticsHandler.RecordStatistics)

		qq := int64(77889900)
		sendResp := sendJSON(t, app, http.MethodPost, "/bot/send-mail", fmt.Sprintf(`{"qq_number":%d,"turnstile_token":"allow"}`, qq), nil)
		if sendResp.Status != fiber.StatusOK || smtpMock.lastCode == "" {
			t.Fatalf("send-mail failed: %+v code=%q", sendResp, smtpMock.lastCode)
		}

		registerResp := sendJSON(t, app, http.MethodPost, "/bot/register", fmt.Sprintf(`{"qq_number":%d,"verification_code":"%s"}`, qq, smtpMock.lastCode), nil)
		if registerResp.Status != fiber.StatusCreated {
			t.Fatalf("register failed: %+v", registerResp)
		}
		var credentialData botAPI.CredentialResponse
		mustUnmarshal(t, registerResp.Data, &credentialData)

		botID, err := strconv.Atoi(credentialData.BotID)
		if err != nil {
			t.Fatalf("invalid bot_id: %v", err)
		}
		dbUser, err := botClient.User.Query().Where(user.BotIDEQ(botID)).Only(ctx)
		if err != nil {
			t.Fatalf("query bot user failed: %v", err)
		}

		authPayload := botAPI.AuthPayload{Credential: credentialData.Credential, Timestamp: time.Now().Unix()}
		payloadBytes, _ := json.Marshal(authPayload)
		encryptedPayload, err := crypto.Encrypt(payloadBytes, deriveKeyFromCredentialForTest(dbUser.Credential))
		if err != nil {
			t.Fatalf("encrypt payload failed: %v", err)
		}

		authResp := sendJSON(t, app, http.MethodPost, "/bot/"+credentialData.BotID+"/auth", fmt.Sprintf(`{"encrypted_payload":"%s"}`, encryptedPayload), nil)
		if authResp.Status != fiber.StatusOK {
			t.Fatalf("auth failed: %+v", authResp)
		}
		var authData botAPI.AuthResponse
		mustUnmarshal(t, authResp.Data, &authData)

		verifyResp := sendJSON(t, app, http.MethodPost, "/internal/bot/verify-session", fmt.Sprintf(`{"bot_id":"%s","session_token":"%s"}`, credentialData.BotID, authData.SessionToken), map[string]string{"Authorization": "Bearer integration-token"})
		if verifyResp.Status != fiber.StatusOK {
			t.Fatalf("verify-session failed: %+v", verifyResp)
		}
		var verifyData botAPI.InternalVerifyResponse
		mustUnmarshal(t, verifyResp.Data, &verifyData)
		if !verifyData.Valid || verifyData.BotID != botID {
			t.Fatalf("verify-session invalid result: %+v", verifyData)
		}

		statsResp := sendJSON(t, app, http.MethodPost, "/bot/statistics/record/"+credentialData.BotID, `{}`, map[string]string{"Authorization": "Bearer integration-token"})
		if statsResp.Status != fiber.StatusOK {
			t.Fatalf("statistics failed: %+v", statsResp)
		}

		if _, err := botClient.RequestsRanking.Query().Where(requestsranking.BotIDEQ(botID)).Only(ctx); err != nil {
			t.Fatalf("requests ranking not written: %v", err)
		}
		if _, err := botClient.HourlyRequests.Query().Where(hourlyrequests.CountEQ(1)).Only(ctx); err != nil {
			t.Fatalf("hourly requests not written: %v", err)
		}
		if _, err := botClient.DailyRequests.Query().Where(dailyrequests.CountEQ(1)).Only(ctx); err != nil {
			t.Fatalf("daily requests not written: %v", err)
		}
	})

	t.Run("PublicQueryAPI", func(t *testing.T) {
		chuniApp := fiber.New()
		chunithmAPI.RegisterChunithmRoutes(chuniApp, chuniMainClient, chuniMusicClient, redisClient)

		resp := sendJSON(t, chuniApp, http.MethodGet, "/chunithm/alias/music-id?alias=it-song", "", nil)
		if resp.Status != fiber.StatusOK {
			t.Fatalf("chunithm alias query failed: %+v", resp)
		}

		batchResp := sendJSON(t, chuniApp, http.MethodPost, "/chunithm/query-batch", `{"music_ids":[4101],"version":"v1"}`, nil)
		if batchResp.Status != fiber.StatusOK {
			t.Fatalf("chunithm query-batch failed: %+v", batchResp)
		}

		pjskApp := fiber.New()
		pjskAPI.RegisterPJSKRoutes(pjskApp, pjskClient, redisClient)
		pjskResp := sendJSON(t, pjskApp, http.MethodGet, "/pjsk/alias/music/by-alias?alias=it-sekai", "", nil)
		if pjskResp.Status != fiber.StatusOK {
			t.Fatalf("pjsk alias query failed: %+v", pjskResp)
		}
	})
}

type integrationEnv struct {
	botURL        string
	chuniMainURL  string
	chuniMusicURL string
	pjskURL       string
	usersURL      string
	redisAddr     string
}

func loadIntegrationEnv(t *testing.T) integrationEnv {
	t.Helper()
	if os.Getenv("HARUKI_INTEGRATION") != "1" {
		t.Skip("set HARUKI_INTEGRATION=1 to run postgres/redis integration tests")
	}
	required := []string{
		"HARUKI_TEST_BOT_DB_URL",
		"HARUKI_TEST_CHUNI_MAIN_DB_URL",
		"HARUKI_TEST_CHUNI_MUSIC_DB_URL",
		"HARUKI_TEST_PJSK_DB_URL",
		"HARUKI_TEST_USERS_DB_URL",
		"HARUKI_TEST_REDIS_ADDR",
	}
	for _, key := range required {
		if os.Getenv(key) == "" {
			t.Fatalf("missing required env %s", key)
		}
	}
	return integrationEnv{
		botURL:        os.Getenv("HARUKI_TEST_BOT_DB_URL"),
		chuniMainURL:  os.Getenv("HARUKI_TEST_CHUNI_MAIN_DB_URL"),
		chuniMusicURL: os.Getenv("HARUKI_TEST_CHUNI_MUSIC_DB_URL"),
		pjskURL:       os.Getenv("HARUKI_TEST_PJSK_DB_URL"),
		usersURL:      os.Getenv("HARUKI_TEST_USERS_DB_URL"),
		redisAddr:     os.Getenv("HARUKI_TEST_REDIS_ADDR"),
	}
}

func mustOpenBotClient(t *testing.T, url string) *botDB.Client {
	t.Helper()
	c, err := botDB.Open("postgres", url)
	if err != nil {
		t.Fatalf("open bot db failed: %v", err)
	}
	return c
}

func mustOpenChuniMainClient(t *testing.T, url string) *chunithmMainDB.Client {
	t.Helper()
	c, err := chunithmMainDB.Open("postgres", url)
	if err != nil {
		t.Fatalf("open chunithm main db failed: %v", err)
	}
	return c
}

func mustOpenChuniMusicClient(t *testing.T, url string) *chunithmMusicDB.Client {
	t.Helper()
	c, err := chunithmMusicDB.Open("postgres", url)
	if err != nil {
		t.Fatalf("open chunithm music db failed: %v", err)
	}
	return c
}

func mustOpenPJSKClient(t *testing.T, url string) *pjskDB.Client {
	t.Helper()
	c, err := pjskDB.Open("postgres", url)
	if err != nil {
		t.Fatalf("open pjsk db failed: %v", err)
	}
	return c
}

func mustOpenUsersClient(t *testing.T, url string) *usersDB.Client {
	t.Helper()
	c, err := usersDB.Open("postgres", url)
	if err != nil {
		t.Fatalf("open users db failed: %v", err)
	}
	return c
}

func mustCreateSchemas(t *testing.T, ctx context.Context, botClient *botDB.Client, chuniMain *chunithmMainDB.Client, chuniMusic *chunithmMusicDB.Client, pjskClient *pjskDB.Client, usersClient *usersDB.Client) {
	t.Helper()
	if err := botClient.Schema.Create(ctx); err != nil {
		t.Fatalf("create bot schema failed: %v", err)
	}
	if err := chuniMain.Schema.Create(ctx); err != nil {
		t.Fatalf("create chunithm main schema failed: %v", err)
	}
	if err := chuniMusic.Schema.Create(ctx); err != nil {
		t.Fatalf("create chunithm music schema failed: %v", err)
	}
	if err := pjskClient.Schema.Create(ctx); err != nil {
		t.Fatalf("create pjsk schema failed: %v", err)
	}
	if err := usersClient.Schema.Create(ctx); err != nil {
		t.Fatalf("create users schema failed: %v", err)
	}
}

func mustResetData(t *testing.T, ctx context.Context, botClient *botDB.Client, chuniMain *chunithmMainDB.Client, chuniMusic *chunithmMusicDB.Client, pjskClient *pjskDB.Client, usersClient *usersDB.Client) {
	t.Helper()

	_, _ = botClient.DailyRequests.Delete().Exec(ctx)
	_, _ = botClient.HourlyRequests.Delete().Exec(ctx)
	_, _ = botClient.RequestsRanking.Delete().Exec(ctx)
	_, _ = botClient.User.Delete().Exec(ctx)

	_, _ = chuniMain.ChunithmBinding.Delete().Exec(ctx)
	_, _ = chuniMain.ChunithmDefaultServer.Delete().Exec(ctx)
	_, _ = chuniMain.ChunithmMusicAlias.Delete().Exec(ctx)

	_, _ = chuniMusic.ChunithmChartData.Delete().Exec(ctx)
	_, _ = chuniMusic.ChunithmMusicDifficulty.Delete().Exec(ctx)
	_, _ = chuniMusic.ChunithmMusic.Delete().Exec(ctx)

	_, _ = pjskClient.UserDefaultBinding.Delete().Exec(ctx)
	_, _ = pjskClient.UserBinding.Delete().Exec(ctx)
	_, _ = pjskClient.UserPreference.Delete().Exec(ctx)
	_, _ = pjskClient.GroupAlias.Delete().Exec(ctx)
	_, _ = pjskClient.Alias.Delete().Exec(ctx)

	_, _ = usersClient.User.Delete().Exec(ctx)
}

func seedCoreData(t *testing.T, ctx context.Context, chuniMain *chunithmMainDB.Client, chuniMusic *chunithmMusicDB.Client, pjskClient *pjskDB.Client, usersClient *usersDB.Client) {
	t.Helper()

	releaseAt := time.Now().Add(-2 * time.Hour)
	chuniMusic.ChunithmMusic.Create().
		SetMusicID(4101).
		SetTitle("Integration Song").
		SetArtist("Integration Artist").
		SetCategory("POPS").
		SetVersion("v1").
		SetReleaseDate(releaseAt).
		SaveX(ctx)
	chuniMusic.ChunithmMusicDifficulty.Create().
		SetMusicID(4101).
		SetVersion("v1").
		SetDiff0Const(12.1).
		SetDiff1Const(12.8).
		SetDiff2Const(13.3).
		SetDiff3Const(14.0).
		SetDiff4Const(14.5).
		SaveX(ctx)
	chuniMusic.ChunithmChartData.Create().
		SetMusicID(4101).
		SetDifficulty(3).
		SetCreator("IT-Creator").
		SetBpm(180).
		SetTapCount(500).
		SetHoldCount(100).
		SetSlideCount(80).
		SetAirCount(50).
		SetFlickCount(30).
		SetTotalCount(760).
		SaveX(ctx)
	chuniMain.ChunithmMusicAlias.Create().SetMusicID(4101).SetAlias("it-song").SaveX(ctx)
	chuniMain.ChunithmDefaultServer.Create().SetHarukiUserID(610001).SetServer("jp").SaveX(ctx)
	chuniMain.ChunithmBinding.Create().SetHarukiUserID(610001).SetServer("jp").SetAimeID("AIME-SEED").SaveX(ctx)

	pjskClient.Alias.Create().SetAliasType("music").SetAliasTypeID(5201).SetAlias("it-sekai").SaveX(ctx)
	pjskClient.Alias.Create().SetAliasType("music").SetAliasTypeID(5201).SetAlias("it-ss").SaveX(ctx)
	pjskClient.GroupAlias.Create().SetPlatform("qq").SetGroupID("it-group").SetAliasType("character").SetAliasTypeID(7301).SetAlias("miku").SaveX(ctx)

	binding := pjskClient.UserBinding.Create().
		SetHarukiUserID(610001).
		SetServer("jp").
		SetUserID("it-user-jp").
		SetVisible(true).
		SaveX(ctx)
	pjskClient.UserDefaultBinding.Create().SetHarukiUserID(610001).SetServer("default").SetBinding(binding).SaveX(ctx)
	pjskClient.UserPreference.Create().SetHarukiUserID(610001).SetOption("theme").SetValue("light").SaveX(ctx)

	usersClient.User.Create().
		SetID(610001).
		SetPlatform("qq").
		SetUserID("it-user").
		SetBanState(false).
		SaveX(ctx)

	if _, err := pjskClient.Alias.Query().Where(alias.AliasEQ("it-sekai")).Only(ctx); err != nil {
		t.Fatalf("seed check alias failed: %v", err)
	}
	if _, err := pjskClient.GroupAlias.Query().Where(groupalias.GroupIDEQ("it-group")).Only(ctx); err != nil {
		t.Fatalf("seed check group alias failed: %v", err)
	}
	if _, err := pjskClient.UserBinding.Query().Where(userbinding.HarukiUserIDEQ(610001)).Only(ctx); err != nil {
		t.Fatalf("seed check binding failed: %v", err)
	}
	if _, err := pjskClient.UserDefaultBinding.Query().Where(userdefaultbinding.HarukiUserIDEQ(610001)).Only(ctx); err != nil {
		t.Fatalf("seed check default binding failed: %v", err)
	}
	if _, err := pjskClient.UserPreference.Query().Where(userpreference.HarukiUserIDEQ(610001)).Only(ctx); err != nil {
		t.Fatalf("seed check preference failed: %v", err)
	}
	if _, err := usersClient.User.Query().Where(usersEntity.IDEQ(610001)).Only(ctx); err != nil {
		t.Fatalf("seed check user failed: %v", err)
	}
}

func deriveKeyFromCredentialForTest(credential string) []byte {
	key := make([]byte, 32)
	copy(key, []byte(credential))
	return key
}

func mustUnmarshal(t *testing.T, raw json.RawMessage, out interface{}) {
	t.Helper()
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("json unmarshal failed: %v raw=%s", err, string(raw))
	}
}

func sendJSON(t *testing.T, app *fiber.App, method, path, body string, headers map[string]string) httpEnvelope {
	t.Helper()
	req, err := http.NewRequest(method, path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	var out httpEnvelope
	if err := json.Unmarshal(payload, &out); err != nil {
		out.Status = resp.StatusCode
		out.Message = string(payload)
		return out
	}
	return out
}
