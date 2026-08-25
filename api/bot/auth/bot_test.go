package auth

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"haruki-cloud/api"
	"haruki-cloud/config"
	ent "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/requestsranking"
	json "haruki-cloud/internal/jsonutil"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	_ "github.com/mattn/go-sqlite3"
	"github.com/redis/go-redis/v9"
	noiseMP "github.com/shamaton/msgpack/v3"
	"golang.org/x/crypto/bcrypt"
)

type testEnvelope struct {
	Status  int             `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type memoryRedisStore struct {
	mu    sync.Mutex
	value map[string]string
}

type stubGlobalBanChecker struct {
	banned bool
	err    error
}

func (s *stubGlobalBanChecker) IsGloballyBanned(context.Context, string, string) (bool, error) {
	return s.banned, s.err
}

func newMemoryRedisStore() *memoryRedisStore {
	return &memoryRedisStore{value: make(map[string]string)}
}

func (s *memoryRedisStore) Set(_ context.Context, key string, value string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value[key] = value
	return nil
}

func (s *memoryRedisStore) Get(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.value[key]
	if !ok {
		return "", redis.Nil
	}
	return v, nil
}

func (s *memoryRedisStore) Del(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.value, key)
	return nil
}

func (s *memoryRedisStore) Incr(_ context.Context, key string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.value[key]
	if !ok {
		s.value[key] = "1"
		return 1, nil
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	n++
	s.value[key] = strconv.FormatInt(n, 10)
	return n, nil
}

func (s *memoryRedisStore) Expire(_ context.Context, _ string, _ time.Duration) error {
	return nil
}

func TestRegisterBotRoutes_ReopensPublicAndInternalRoutes(t *testing.T) {
	ctx := context.Background()
	client := newBotTestClient(t, "route")
	defer func() { _ = client.Close() }()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	prev := config.Cfg
	config.Cfg.Backend.AcceptAuthorization = "Bearer route-test"
	t.Cleanup(func() { config.Cfg = prev })

	app := fiber.New()
	RegisterBotRoutes(app, client, nil, nil, "")

	authHTTPResp := sendRawRequest(t, app, http.MethodPost, "/api/v2/bot/12345678/auth", []byte("garbage"))
	authHTTPResp.Body.Close()
	// Auth route is registered if we get a non-404 response (400 or 500 both ok)
	if authHTTPResp.StatusCode == fiber.StatusNotFound {
		t.Fatalf("expected /api/v2/bot/:bot_id/auth to be registered, got 404")
	}

	verifyResp := sendJSONRequest(t, app, http.MethodPost, "/internal/bot/verify-session", `{}`, map[string]string{
		"Authorization": "Bearer route-test",
	})
	if verifyResp.Status != fiber.StatusBadRequest {
		t.Fatalf("expected /internal/bot/verify-session to be registered, got status=%d message=%s", verifyResp.Status, verifyResp.Message)
	}
}

func TestBotAuthFlow_WithSeededUser(t *testing.T) {
	ctx := context.Background()
	client := newBotTestClient(t, "flow")
	defer func() { _ = client.Close() }()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	prev := config.Cfg
	config.Cfg.HarukiBotDB.CredentialSignToken = "credential-sign-token"
	config.Cfg.HarukiBotDB.SessionSignToken = "session-sign-token"
	config.Cfg.HarukiBotDB.SessionTTLDays = 7
	config.Cfg.Backend.AcceptAuthorization = "Bearer internal-test"
	config.Cfg.Backend.AcceptUserAgent = ""
	t.Cleanup(func() { config.Cfg = prev })

	store := newMemoryRedisStore()
	testAuthKey := []byte("01234567890123456789012345678901") // 32 bytes
	banChecker := &stubGlobalBanChecker{}
	userHandler := NewUserHandler(NewUserServiceWithDependencies(client, store, testAuthKey, "deadbeef").WithGlobalBanChecker(banChecker))
	internalHandler := NewInternalHandler(NewInternalServiceWithStore(client, store).WithGlobalBanChecker(banChecker))
	statsHandler := NewStatisticsHandler(NewStatisticsService(client))

	app := fiber.New()
	public := app.Group("/bot")
	public.Post("/:bot_id/auth", userHandler.Auth)
	internal := app.Group("/internal/bot", api.VerifyAPIAuthorization())
	internal.Post("/verify-session", internalHandler.VerifySession)
	app.Post("/internal/bot/statistics/record/:botID", api.VerifyAPIAuthorization(), statsHandler.RecordStatistics)

	// Seed a user directly in the database
	const qqNumber int64 = 123456789
	const botID = 10042042
	credential := "test-credential-value"
	hashedCred, err := bcrypt.GenerateFromPassword([]byte(credential), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash credential: %v", err)
	}
	_, err = client.User.Create().
		SetOwnerUserID(qqNumber).
		SetBotID(botID).
		SetCredential(string(hashedCred)).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	botIDStr := strconv.Itoa(botID)

	// Sign credential as JWT (mimicking what register would return)
	credentialJWT := signTestCredential(t, botIDStr, credential)

	// Auth flow
	authPayload := HarukiAuthPayload{
		BotID:      botIDStr,
		Credential: credentialJWT,
		Timestamp:  time.Now().Unix(),
	}
	payloadBytes, err := noiseMP.Marshal(authPayload)
	if err != nil {
		t.Fatalf("marshal auth payload: %v", err)
	}
	encryptedBody, err := EncryptRaw(payloadBytes, testAuthKey)
	if err != nil {
		t.Fatalf("encrypt auth payload: %v", err)
	}

	authHTTPResp := sendRawRequest(t, app, http.MethodPost, "/bot/"+botIDStr+"/auth", encryptedBody)
	if authHTTPResp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(authHTTPResp.Body)
		t.Fatalf("auth failed: status=%d body=%s", authHTTPResp.StatusCode, string(body))
	}
	authRespBody, _ := io.ReadAll(authHTTPResp.Body)
	authHTTPResp.Body.Close()

	decryptedResp, err := DecryptRaw(authRespBody, testAuthKey)
	if err != nil {
		t.Fatalf("decrypt auth response: %v", err)
	}
	var authData HarukiAuthResponse
	if err := noiseMP.Unmarshal(decryptedResp, &authData); err != nil {
		t.Fatalf("decode auth response: %v", err)
	}
	if authData.SessionToken == "" || authData.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("invalid auth response: %+v", authData)
	}
	if authData.NoiseServerPubKey != "deadbeef" {
		t.Fatalf("expected noise pubkey 'deadbeef', got '%s'", authData.NoiseServerPubKey)
	}

	verifyBody := fmt.Sprintf(`{"bot_id":"%s","session_token":"%s"}`, botIDStr, authData.SessionToken)
	verifyResp := sendJSONRequest(t, app, http.MethodPost, "/internal/bot/verify-session", verifyBody, map[string]string{
		"Authorization": "Bearer internal-test",
	})
	if verifyResp.Status != fiber.StatusOK {
		t.Fatalf("verify-session failed: status=%d message=%s", verifyResp.Status, verifyResp.Message)
	}
	var verifyData InternalVerifyResponse
	if err := json.Unmarshal(verifyResp.Data, &verifyData); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if !verifyData.Valid || verifyData.BotID != botID || verifyData.OwnerUserID != qqNumber {
		t.Fatalf("unexpected verify response: %+v", verifyData)
	}

	banChecker.banned = true
	verifyResp = sendJSONRequest(t, app, http.MethodPost, "/internal/bot/verify-session", verifyBody, map[string]string{
		"Authorization": "Bearer internal-test",
	})
	if err := json.Unmarshal(verifyResp.Data, &verifyData); err != nil {
		t.Fatalf("decode banned verify response: %v", err)
	}
	if verifyData.Valid {
		t.Fatalf("globally banned Bot owner retained a valid session: %+v", verifyData)
	}
	banChecker.banned = false

	statsResp := sendJSONRequest(t, app, http.MethodPost, "/internal/bot/statistics/record/"+botIDStr, `{}`, map[string]string{
		"Authorization": "Bearer internal-test",
	})
	if statsResp.Status != fiber.StatusOK {
		t.Fatalf("statistics failed: status=%d message=%s", statsResp.Status, statsResp.Message)
	}
	statsResp = sendJSONRequest(t, app, http.MethodPost, "/internal/bot/statistics/record/"+botIDStr, `{}`, map[string]string{
		"Authorization": "Bearer internal-test",
	})
	if statsResp.Status != fiber.StatusOK {
		t.Fatalf("second statistics failed: status=%d message=%s", statsResp.Status, statsResp.Message)
	}

	rankingRow, err := client.RequestsRanking.Query().Where(requestsranking.BotIDEQ(botID)).Only(ctx)
	if err != nil {
		t.Fatalf("load requests ranking: %v", err)
	}
	if rankingRow.Counts != 2 {
		t.Fatalf("requests ranking count mismatch: got=%d want=2", rankingRow.Counts)
	}

	hourlyRow, err := client.HourlyRequests.Query().Only(ctx)
	if err != nil {
		t.Fatalf("load hourly requests: %v", err)
	}
	if hourlyRow.Count != 2 {
		t.Fatalf("hourly requests count mismatch: got=%d want=2", hourlyRow.Count)
	}

	dailyRow, err := client.DailyRequests.Query().Only(ctx)
	if err != nil {
		t.Fatalf("load daily requests: %v", err)
	}
	if dailyRow.Count != 2 {
		t.Fatalf("daily requests count mismatch: got=%d want=2", dailyRow.Count)
	}
}

func TestBotAuthRejectsGloballyBannedOwner(t *testing.T) {
	ctx := context.Background()
	client := newBotTestClient(t, "banned_owner")
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	prev := config.Cfg
	config.Cfg.HarukiBotDB.CredentialSignToken = "credential-sign-token"
	t.Cleanup(func() { config.Cfg = prev })

	const botID = 10043043
	const credential = "banned-owner-credential"
	hashedCred, err := bcrypt.GenerateFromPassword([]byte(credential), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash credential: %v", err)
	}
	_, err = client.User.Create().
		SetOwnerUserID(123456789).
		SetBotID(botID).
		SetCredential(string(hashedCred)).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	store := newMemoryRedisStore()
	key := []byte("01234567890123456789012345678901")
	handler := NewUserHandler(
		NewUserServiceWithDependencies(client, store, key, "").
			WithGlobalBanChecker(&stubGlobalBanChecker{banned: true}),
	)
	app := fiber.New()
	app.Post("/bot/:bot_id/auth", handler.Auth)

	botIDStr := strconv.Itoa(botID)
	payloadBytes, err := noiseMP.Marshal(HarukiAuthPayload{
		BotID:      botIDStr,
		Credential: signTestCredential(t, botIDStr, credential),
		Timestamp:  time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	encryptedBody, err := EncryptRaw(payloadBytes, key)
	if err != nil {
		t.Fatalf("encrypt payload: %v", err)
	}
	resp := sendRawRequest(t, app, http.MethodPost, "/bot/"+botIDStr+"/auth", encryptedBody)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusForbidden || string(body) != ErrOwnerBanned {
		t.Fatalf("unexpected banned auth response: status=%d body=%q", resp.StatusCode, body)
	}
}

func signTestCredential(t *testing.T, botID, credential string) string {
	t.Helper()
	payload := jwt.MapClaims{
		"bot_id":     botID,
		"credential": credential,
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, payload).
		SignedString([]byte(config.Cfg.HarukiBotDB.CredentialSignToken))
	if err != nil {
		t.Fatalf("sign test credential: %v", err)
	}
	return token
}

func sendJSONRequest(t *testing.T, app *fiber.App, method, path, body string, headers map[string]string) testEnvelope {
	t.Helper()

	req, err := http.NewRequest(method, path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true})
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	var envelope testEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		t.Fatalf("decode response body: %v\nraw=%s", err, string(respBody))
	}
	return envelope
}

func sendRawRequest(t *testing.T, app *fiber.App, method, path string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true})
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	return resp
}

func newBotTestClient(t *testing.T, name string) *ent.Client {
	t.Helper()
	dsn := fmt.Sprintf("file:bot_%s_%d?mode=memory&cache=shared&_fk=1", name, time.Now().UnixNano())
	client, err := ent.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open bot sqlite: %v", err)
	}
	return client
}
