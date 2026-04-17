package auth

import (
	"bytes"
	"context"
	"encoding/json"
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
	"haruki-cloud/database/bot/user"

	"github.com/gofiber/fiber/v3"
	_ "github.com/mattn/go-sqlite3"
	"github.com/redis/go-redis/v9"
	noiseMP "github.com/shamaton/msgpack/v3"
)

type testEnvelope struct {
	Status  int             `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type mockTurnstile struct {
	valid bool
	err   error
	token string
	ip    string
}

func (m *mockTurnstile) VerifyToken(token, remoteIP string) (bool, error) {
	m.token = token
	m.ip = remoteIP
	if m.err != nil {
		return false, m.err
	}
	return m.valid, nil
}

type mockSMTP struct {
	err  error
	sent []mailEvent
}

type mailEvent struct {
	qq   int64
	code string
}

func (m *mockSMTP) SendVerificationCode(qqNumber int64, code string) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, mailEvent{qq: qqNumber, code: code})
	return nil
}

type memoryRedisStore struct {
	mu    sync.Mutex
	value map[string]string
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
	config.Cfg.HarukiBotDB.EnableRegistration = true
	t.Cleanup(func() { config.Cfg = prev })

	app := fiber.New()
	RegisterBotRoutes(app, client, nil, nil, "")

	sendResp := sendJSONRequest(t, app, http.MethodPost, "/api/v2/bot/send-mail", `{"qq_number":0}`, nil)
	if sendResp.Status != fiber.StatusBadRequest {
		t.Fatalf("expected /api/v2/bot/send-mail to be registered, got status=%d message=%s", sendResp.Status, sendResp.Message)
	}

	registerResp := sendJSONRequest(t, app, http.MethodPost, "/api/v2/bot/register", `{"qq_number":0}`, nil)
	if registerResp.Status != fiber.StatusBadRequest {
		t.Fatalf("expected /api/v2/bot/register to be registered, got status=%d message=%s", registerResp.Status, registerResp.Message)
	}

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

func TestBotAuthFlow_WithMockMailAndTurnstile(t *testing.T) {
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
	config.Cfg.HarukiBotDB.EnableRegistration = true
	config.Cfg.Backend.AcceptAuthorization = "Bearer internal-test"
	config.Cfg.Backend.AcceptUserAgent = ""
	t.Cleanup(func() { config.Cfg = prev })

	store := newMemoryRedisStore()
	turnstileMock := &mockTurnstile{valid: true}
	smtpMock := &mockSMTP{}

	testAuthKey := []byte("01234567890123456789012345678901") // 32 bytes
	userHandler := NewUserHandler(NewUserServiceWithDependencies(client, store, turnstileMock, smtpMock, testAuthKey, "deadbeef"))
	internalHandler := NewInternalHandler(NewInternalServiceWithStore(client, store))
	statsHandler := NewStatisticsHandler(NewStatisticsService(client))

	app := fiber.New()
	public := app.Group("/bot")
	public.Post("/send-mail", userHandler.SendMail)
	public.Post("/register", userHandler.Register)
	public.Post("/:bot_id/auth", userHandler.Auth)
	internal := app.Group("/internal/bot", api.VerifyAPIAuthorization())
	internal.Post("/verify-session", internalHandler.VerifySession)
	app.Post("/internal/bot/statistics/record/:botID", api.VerifyAPIAuthorization(), statsHandler.RecordStatistics)

	const qqNumber int64 = 123456789
	sendMailResp := sendJSONRequest(t, app, http.MethodPost, "/bot/send-mail", fmt.Sprintf(`{"qq_number":%d,"turnstile_token":"ts-token"}`, qqNumber), nil)
	if sendMailResp.Status != fiber.StatusOK {
		t.Fatalf("send-mail failed: status=%d message=%s", sendMailResp.Status, sendMailResp.Message)
	}
	if turnstileMock.token != "ts-token" {
		t.Fatalf("turnstile token mismatch: %q", turnstileMock.token)
	}
	if len(smtpMock.sent) != 1 {
		t.Fatalf("expected one mock mail, got %d", len(smtpMock.sent))
	}
	verificationCode := smtpMock.sent[0].code
	if verificationCode == "" {
		t.Fatalf("verification code should not be empty")
	}

	registerResp := sendJSONRequest(t, app, http.MethodPost, "/bot/register", fmt.Sprintf(`{"qq_number":%d,"verification_code":"%s"}`, qqNumber, verificationCode), nil)
	if registerResp.Status != fiber.StatusCreated {
		t.Fatalf("register failed: status=%d message=%s", registerResp.Status, registerResp.Message)
	}

	var registerData CredentialResponse
	if err := json.Unmarshal(registerResp.Data, &registerData); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	if registerData.BotID == "" || registerData.Credential == "" {
		t.Fatalf("register response missing bot_id or credential")
	}

	botID, err := strconv.Atoi(registerData.BotID)
	if err != nil {
		t.Fatalf("parse bot_id: %v", err)
	}

	dbUser, err := client.User.Query().Where(user.BotIDEQ(botID)).Only(ctx)
	if err != nil {
		t.Fatalf("load bot user: %v", err)
	}
	if dbUser.OwnerUserID != qqNumber {
		t.Fatalf("owner user mismatch: got=%d want=%d", dbUser.OwnerUserID, qqNumber)
	}

	authPayload := HarukiAuthPayload{
		BotID:      registerData.BotID,
		Credential: registerData.Credential,
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

	authHTTPResp := sendRawRequest(t, app, http.MethodPost, "/bot/"+registerData.BotID+"/auth", encryptedBody)
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

	verifyBody := fmt.Sprintf(`{"bot_id":"%s","session_token":"%s"}`, registerData.BotID, authData.SessionToken)
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

	statsResp := sendJSONRequest(t, app, http.MethodPost, "/internal/bot/statistics/record/"+registerData.BotID, `{}`, map[string]string{
		"Authorization": "Bearer internal-test",
	})
	if statsResp.Status != fiber.StatusOK {
		t.Fatalf("statistics failed: status=%d message=%s", statsResp.Status, statsResp.Message)
	}
	statsResp = sendJSONRequest(t, app, http.MethodPost, "/internal/bot/statistics/record/"+registerData.BotID, `{}`, map[string]string{
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

func TestBotSendMail_TurnstileFailure(t *testing.T) {
	ctx := context.Background()
	client := newBotTestClient(t, "turnstile-fail")
	defer func() { _ = client.Close() }()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	prev := config.Cfg
	config.Cfg.HarukiBotDB.EnableRegistration = true
	t.Cleanup(func() { config.Cfg = prev })

	store := newMemoryRedisStore()
	turnstileMock := &mockTurnstile{valid: false}
	smtpMock := &mockSMTP{}

	h := NewUserHandler(NewUserServiceWithDependencies(client, store, turnstileMock, smtpMock, nil, ""))
	app := fiber.New()
	app.Post("/bot/send-mail", h.SendMail)

	resp := sendJSONRequest(t, app, http.MethodPost, "/bot/send-mail", `{"qq_number":10001,"turnstile_token":"invalid"}`, nil)
	if resp.Status != fiber.StatusBadRequest {
		t.Fatalf("expected bad request for invalid turnstile, got status=%d message=%s", resp.Status, resp.Message)
	}
	if len(smtpMock.sent) != 0 {
		t.Fatalf("smtp mock should not send mail on turnstile failure")
	}
}

func sendJSONRequest(t *testing.T, app *fiber.App, method, path, body string, headers map[string]string) testEnvelope {
	t.Helper()

	req, err := http.NewRequest(method, path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := app.Test(req)
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
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := app.Test(req)
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
