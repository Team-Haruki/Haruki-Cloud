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
	"haruki-cloud/internal/core/crypto"
	json "haruki-cloud/internal/jsonutil"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	_ "github.com/mattn/go-sqlite3"
	"github.com/redis/go-redis/v9"
	noiseMP "github.com/shamaton/msgpack/v3"
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

	pair, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	ring, err := crypto.SingleKeyRing(pair)
	if err != nil {
		t.Fatalf("build key ring: %v", err)
	}
	app := fiber.New()
	RegisterBotRoutesWithBanChecker(app, client, nil, ring, nil)

	// The login route sits behind the Noise middleware: garbage is refused there.
	authHTTPResp := sendRawRequest(t, app, http.MethodPost, AuthV3RouteBase+"/12345678/auth", []byte("garbage"))
	authHTTPResp.Body.Close()
	if authHTTPResp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected %s/:bot_id/auth to reject plaintext with 400, got %d", AuthV3RouteBase, authHTTPResp.StatusCode)
	}

	logoutResp := sendRawRequest(t, app, http.MethodDelete, AuthV3RouteBase+"/12345678/logout", nil)
	logoutResp.Body.Close()
	if logoutResp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected %s/:bot_id/logout without a token to return 401, got %d", AuthV3RouteBase, logoutResp.StatusCode)
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
	env := newAuthV3TestEnv(t)
	statsHandler := NewStatisticsHandler(NewStatisticsService(env.client))
	env.app.Post("/internal/bot/statistics/record/:botID", api.VerifyAPIAuthorization(), statsHandler.RecordStatistics)

	status, body := env.send(t, env.current, "", env.basePayload(t))
	if status != fiber.StatusOK {
		t.Fatalf("auth failed: status=%d body=%s", status, body)
	}
	var authData AuthResponseV3
	if err := noiseMP.Unmarshal(body, &authData); err != nil {
		t.Fatalf("decode auth response: %v", err)
	}
	if authData.SessionToken == "" || authData.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("invalid auth response: %+v", authData)
	}

	verifyBody := fmt.Sprintf(`{"bot_id":"%s","session_token":"%s"}`, env.botStr, authData.SessionToken)
	verifyResp := sendJSONRequest(t, env.app, http.MethodPost, "/internal/bot/verify-session", verifyBody, map[string]string{
		"Authorization": "Bearer internal-test",
	})
	if verifyResp.Status != fiber.StatusOK {
		t.Fatalf("verify-session failed: status=%d message=%s", verifyResp.Status, verifyResp.Message)
	}
	var verifyData InternalVerifyResponse
	if err := json.Unmarshal(verifyResp.Data, &verifyData); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if !verifyData.Valid || verifyData.BotID != env.botID || verifyData.OwnerUserID != 987654321 {
		t.Fatalf("unexpected verify response: %+v", verifyData)
	}

	env.ban.banned = true
	verifyResp = sendJSONRequest(t, env.app, http.MethodPost, "/internal/bot/verify-session", verifyBody, map[string]string{
		"Authorization": "Bearer internal-test",
	})
	if err := json.Unmarshal(verifyResp.Data, &verifyData); err != nil {
		t.Fatalf("decode banned verify response: %v", err)
	}
	if verifyData.Valid {
		t.Fatalf("globally banned Bot owner retained a valid session: %+v", verifyData)
	}
	env.ban.banned = false

	for i := 0; i < 2; i++ {
		statsResp := sendJSONRequest(t, env.app, http.MethodPost, "/internal/bot/statistics/record/"+env.botStr, `{}`, map[string]string{
			"Authorization": "Bearer internal-test",
		})
		if statsResp.Status != fiber.StatusOK {
			t.Fatalf("statistics call %d failed: status=%d message=%s", i+1, statsResp.Status, statsResp.Message)
		}
	}

	rankingRow, err := env.client.RequestsRanking.Query().Where(requestsranking.BotIDEQ(env.botID)).Only(ctx)
	if err != nil {
		t.Fatalf("load requests ranking: %v", err)
	}
	if rankingRow.Counts != 2 {
		t.Fatalf("requests ranking count mismatch: got=%d want=2", rankingRow.Counts)
	}

	hourlyRow, err := env.client.HourlyRequests.Query().Only(ctx)
	if err != nil {
		t.Fatalf("load hourly requests: %v", err)
	}
	if hourlyRow.Count != 2 {
		t.Fatalf("hourly requests count mismatch: got=%d want=2", hourlyRow.Count)
	}

	dailyRow, err := env.client.DailyRequests.Query().Only(ctx)
	if err != nil {
		t.Fatalf("load daily requests: %v", err)
	}
	if dailyRow.Count != 2 {
		t.Fatalf("daily requests count mismatch: got=%d want=2", dailyRow.Count)
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

func TestParseCredentialJWTOnlyAcceptsHS256(t *testing.T) {
	const signingToken = "credential-sign-token"
	claims := jwt.MapClaims{
		"bot_id":     "10042042",
		"credential": "credential",
	}

	tests := []struct {
		name    string
		method  jwt.SigningMethod
		wantErr bool
	}{
		{name: "HS256", method: jwt.SigningMethodHS256},
		{name: "HS384", method: jwt.SigningMethodHS384, wantErr: true},
		{name: "HS512", method: jwt.SigningMethodHS512, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawToken, err := jwt.NewWithClaims(tt.method, claims).SignedString([]byte(signingToken))
			if err != nil {
				t.Fatalf("sign credential JWT: %v", err)
			}
			parsed, err := parseCredentialJWT(rawToken, signingToken)
			if tt.wantErr {
				if err == nil || parsed != nil && parsed.Valid {
					t.Fatalf("parseCredentialJWT accepted %s", tt.method.Alg())
				}
				return
			}
			if err != nil || !parsed.Valid {
				t.Fatalf("parseCredentialJWT rejected HS256: token=%v err=%v", parsed, err)
			}
		})
	}
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
