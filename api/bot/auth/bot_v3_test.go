package auth

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"haruki-cloud/api"
	"haruki-cloud/config"
	ent "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/user"
	"haruki-cloud/internal/core/crypto"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/middleware/secure"

	"github.com/gofiber/fiber/v3"
	noiseMP "github.com/shamaton/msgpack/v3"
	"golang.org/x/crypto/bcrypt"
)

type authV3TestEnv struct {
	app     *fiber.App
	client  *ent.Client
	ring    *crypto.KeyRing
	current *crypto.KeyPair
	next    *crypto.KeyPair
	botID   int
	botStr  string
	credJWT string
	ban     *stubGlobalBanChecker
}

func newAuthV3TestEnv(t *testing.T) *authV3TestEnv {
	t.Helper()
	ctx := context.Background()
	client := newBotTestClient(t, "v3-"+strings.ReplaceAll(t.Name(), "/", "-"))
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	prev := config.Cfg
	config.Cfg.HarukiBotDB.CredentialSignToken = "credential-sign-token"
	config.Cfg.HarukiBotDB.SessionSignToken = "session-sign-token"
	config.Cfg.HarukiBotDB.AuthV3SessionTTL = 0 // default 1h
	config.Cfg.Backend.AcceptAuthorization = "Bearer internal-test"
	config.Cfg.Backend.AcceptUserAgent = ""
	t.Cleanup(func() { config.Cfg = prev })

	current, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate current key: %v", err)
	}
	next, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate next key: %v", err)
	}
	ring, err := crypto.NewKeyRing(
		crypto.StaticKey{ID: "current", Pair: current},
		crypto.StaticKey{ID: "next", Pair: next},
	)
	if err != nil {
		t.Fatalf("NewKeyRing: %v", err)
	}

	store := newMemoryRedisStore()
	ban := &stubGlobalBanChecker{}
	userHandler := NewUserHandler(NewUserServiceWithDependencies(client, store).WithGlobalBanChecker(ban))
	internalHandler := NewInternalHandler(NewInternalServiceWithStore(client, store).WithGlobalBanChecker(ban))

	app := fiber.New()
	registerAuthV3Routes(app, userHandler, ring)
	// Deliberately unwrapped mount to prove the handler refuses plaintext.
	app.Post("/unwrapped/:bot_id/auth", userHandler.AuthV3)
	app.Group("/internal/bot", api.VerifyAPIAuthorization()).Post("/verify-session", internalHandler.VerifySession)

	const botID = 30042042
	credential := "v3-credential-value"
	hashed, err := bcrypt.GenerateFromPassword([]byte(credential), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash credential: %v", err)
	}
	if _, err := client.User.Create().SetOwnerUserID(987654321).SetBotID(botID).SetCredential(string(hashed)).Save(ctx); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	botStr := strconv.Itoa(botID)
	return &authV3TestEnv{
		app: app, client: client, ring: ring, current: current, next: next,
		botID: botID, botStr: botStr, credJWT: signTestCredential(t, botStr, credential), ban: ban,
	}
}

func (e *authV3TestEnv) basePayload(t *testing.T) AuthPayloadV3 {
	t.Helper()
	nonce, err := randomHex(AuthV3NonceSize)
	if err != nil {
		t.Fatalf("random nonce: %v", err)
	}
	return AuthPayloadV3{
		BotID:         e.botStr,
		Credential:    e.credJWT,
		Timestamp:     time.Now().Unix(),
		Nonce:         nonce,
		ClientVersion: "2.9.0",
		BuildID:       "build-abc123",
		Method:        http.MethodPost,
		Path:          AuthV3RouteBase + "/" + e.botStr + "/auth",
	}
}

// send performs one Noise NK round trip against the AuthV3 route and returns
// the HTTP status plus the decrypted response body.
func (e *authV3TestEnv) send(t *testing.T, pair *crypto.KeyPair, keyIDHint string, payload AuthPayloadV3) (int, []byte) {
	t.Helper()
	initiator, err := crypto.NewInitiator(pair.Public)
	if err != nil {
		t.Fatalf("NewInitiator: %v", err)
	}
	plain, err := noiseMP.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	ciphertext, err := initiator.EncryptPacket(plain)
	if err != nil {
		t.Fatalf("encrypt payload: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, AuthV3RouteBase+"/"+e.botStr+"/auth", bytes.NewReader(ciphertext))
	if keyIDHint != "" {
		request.Header.Set(secure.HeaderNoiseKeyID, keyIDHint)
	}
	response, err := e.app.Test(request, fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true})
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("response was not Noise-wrapped: status %d type %q body %q", response.StatusCode, response.Header.Get("Content-Type"), body)
	}
	decrypted, err := initiator.DecryptPacket(body)
	if err != nil {
		t.Fatalf("decrypt response: %v", err)
	}
	return response.StatusCode, decrypted
}

func (e *authV3TestEnv) expectReject(t *testing.T, pair *crypto.KeyPair, hint string, payload AuthPayloadV3, wantMsg string) {
	t.Helper()
	status, body := e.send(t, pair, hint, payload)
	if status != fiber.StatusBadRequest || string(body) != wantMsg {
		t.Fatalf("status %d body %q, want 400 %q", status, body, wantMsg)
	}
}

func TestAuthV3FlowIssuesShortSessionOverNoise(t *testing.T) {
	env := newAuthV3TestEnv(t)
	payload := env.basePayload(t)

	before := time.Now()
	status, body := env.send(t, env.current, "", payload)
	if status != fiber.StatusOK {
		t.Fatalf("auth v3 failed: status %d body %s", status, body)
	}
	var resp AuthResponseV3
	if err := noiseMP.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SessionToken == "" || resp.SessionID == "" {
		t.Fatalf("missing session fields: %+v", resp)
	}
	if resp.EchoNonce != payload.Nonce {
		t.Fatalf("echo_nonce %q != nonce %q", resp.EchoNonce, payload.Nonce)
	}
	if resp.AcceptedBuildID != payload.BuildID {
		t.Fatalf("accepted_build_id = %q", resp.AcceptedBuildID)
	}
	wantExpiry := before.Add(DefaultAuthV3SessionTTL).Unix()
	if resp.ExpiresAt < wantExpiry-5 || resp.ExpiresAt > wantExpiry+5 {
		t.Fatalf("expires_at %d, want about %d (1h)", resp.ExpiresAt, wantExpiry)
	}
	if resp.ServerTime < before.Unix() || resp.ServerTime > time.Now().Unix() {
		t.Fatalf("server_time %d out of range", resp.ServerTime)
	}

	verifyBody := fmt.Sprintf(`{"bot_id":"%s","session_token":"%s"}`, env.botStr, resp.SessionToken)
	verifyResp := sendJSONRequest(t, env.app, http.MethodPost, "/internal/bot/verify-session", verifyBody, map[string]string{
		"Authorization": "Bearer internal-test",
	})
	var verified InternalVerifyResponse
	if err := json.Unmarshal(verifyResp.Data, &verified); err != nil {
		t.Fatalf("decode verify: %v", err)
	}
	if !verified.Valid || verified.BotID != env.botID {
		t.Fatalf("session not accepted by verify-session: %+v", verified)
	}

	row, err := env.client.User.Query().Where(user.BotIDEQ(env.botID)).Only(context.Background())
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	if row.LastLoginAt.IsZero() {
		t.Fatal("last_login_at was not recorded")
	}

	// Replaying the exact same nonce must be refused.
	env.expectReject(t, env.current, "", payload, ErrReplayDetected)
}

func TestAuthV3AcceptsRotationKeyAndChecksKeyID(t *testing.T) {
	env := newAuthV3TestEnv(t)

	payload := env.basePayload(t)
	payload.NoiseKeyID = "next"
	if status, body := env.send(t, env.next, "next", payload); status != fiber.StatusOK {
		t.Fatalf("rotation key auth failed: status %d body %s", status, body)
	}

	// Handshake against "next" while claiming "current" in the payload.
	payload = env.basePayload(t)
	payload.NoiseKeyID = "current"
	env.expectReject(t, env.next, "", payload, ErrNoiseKeyMismatch)

	// Empty noise_key_id skips the check.
	payload = env.basePayload(t)
	if status, body := env.send(t, env.next, "", payload); status != fiber.StatusOK {
		t.Fatalf("rotation key without key id failed: status %d body %s", status, body)
	}
}

func TestAuthV3RejectsBrokenBindingsAndStalePayloads(t *testing.T) {
	env := newAuthV3TestEnv(t)

	t.Run("path", func(t *testing.T) {
		payload := env.basePayload(t)
		payload.Path = "/api/v2/bot/" + env.botStr + "/auth"
		env.expectReject(t, env.current, "", payload, ErrRequestBindingBroken)
	})
	t.Run("method", func(t *testing.T) {
		payload := env.basePayload(t)
		payload.Method = http.MethodGet
		env.expectReject(t, env.current, "", payload, ErrRequestBindingBroken)
	})
	t.Run("bot id", func(t *testing.T) {
		payload := env.basePayload(t)
		payload.BotID = "1"
		env.expectReject(t, env.current, "", payload, ErrBotIDMismatch)
	})
	t.Run("stale timestamp", func(t *testing.T) {
		payload := env.basePayload(t)
		payload.Timestamp = time.Now().Unix() - MaxAuthTimestampAge - 10
		env.expectReject(t, env.current, "", payload, ErrAuthTimestampExpired)
	})
	t.Run("future timestamp", func(t *testing.T) {
		payload := env.basePayload(t)
		payload.Timestamp = time.Now().Unix() + MaxAuthTimestampAge + 10
		env.expectReject(t, env.current, "", payload, ErrAuthTimestampExpired)
	})
	t.Run("short nonce", func(t *testing.T) {
		payload := env.basePayload(t)
		payload.Nonce = "abcd"
		env.expectReject(t, env.current, "", payload, ErrInvalidNonce)
	})
	t.Run("non hex nonce", func(t *testing.T) {
		payload := env.basePayload(t)
		payload.Nonce = strings.Repeat("zz", AuthV3NonceSize)
		env.expectReject(t, env.current, "", payload, ErrInvalidNonce)
	})
	t.Run("wrong credential", func(t *testing.T) {
		payload := env.basePayload(t)
		payload.Credential = signTestCredential(t, env.botStr, "not-the-credential")
		env.expectReject(t, env.current, "", payload, ErrAuthFailed)
	})
	t.Run("banned owner", func(t *testing.T) {
		env.ban.banned = true
		t.Cleanup(func() { env.ban.banned = false })
		status, body := env.send(t, env.current, "", env.basePayload(t))
		if status != fiber.StatusForbidden || string(body) != ErrOwnerBanned {
			t.Fatalf("status %d body %q", status, body)
		}
	})
	t.Run("garbage payload", func(t *testing.T) {
		initiator, _ := crypto.NewInitiator(env.current.Public)
		ciphertext, _ := initiator.EncryptPacket([]byte("not msgpack at all"))
		response, err := env.app.Test(httptest.NewRequest(http.MethodPost, AuthV3RouteBase+"/"+env.botStr+"/auth", bytes.NewReader(ciphertext)), fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true})
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		body, _ := io.ReadAll(response.Body)
		plain, err := initiator.DecryptPacket(body)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if response.StatusCode != fiber.StatusBadRequest || string(plain) != ErrInvalidEncryptedData {
			t.Fatalf("status %d body %q", response.StatusCode, plain)
		}
	})
}

func TestAuthV3IsFailClosed(t *testing.T) {
	env := newAuthV3TestEnv(t)
	plain, _ := noiseMP.Marshal(env.basePayload(t))

	// Plaintext MsgPack straight at the Noise route: refused by the middleware.
	response := sendRawRequest(t, env.app, http.MethodPost, AuthV3RouteBase+"/"+env.botStr+"/auth", plain)
	response.Body.Close()
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("plaintext to Noise route: status %d", response.StatusCode)
	}

	// Foreign static key: refused by the middleware.
	stranger, _ := crypto.GenerateKeyPair()
	initiator, _ := crypto.NewInitiator(stranger.Public)
	ciphertext, _ := initiator.EncryptPacket(plain)
	response = sendRawRequest(t, env.app, http.MethodPost, AuthV3RouteBase+"/"+env.botStr+"/auth", ciphertext)
	response.Body.Close()
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("foreign key to Noise route: status %d", response.StatusCode)
	}

	// Handler mounted without the middleware refuses to run on plaintext.
	response = sendRawRequest(t, env.app, http.MethodPost, "/unwrapped/"+env.botStr+"/auth", plain)
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != fiber.StatusBadRequest || string(body) != ErrSecureChannelMissing {
		t.Fatalf("unwrapped handler: status %d body %q", response.StatusCode, body)
	}

	// Non-numeric bot id.
	response = sendRawRequest(t, env.app, http.MethodPost, "/unwrapped/not-a-number/auth", plain)
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != fiber.StatusBadRequest || string(body) != ErrAuthFailed {
		t.Fatalf("bad bot id: status %d body %q", response.StatusCode, body)
	}
}

func TestAuthV3SharesRateLimitBucketWithV2(t *testing.T) {
	env := newAuthV3TestEnv(t)
	for i := 0; i < RateLimitAuth; i++ {
		if status, body := env.send(t, env.current, "", env.basePayload(t)); status != fiber.StatusOK {
			t.Fatalf("request %d: status %d body %s", i, status, body)
		}
	}
	status, body := env.send(t, env.current, "", env.basePayload(t))
	if status != fiber.StatusTooManyRequests || string(body) != ErrRateLimitExceeded {
		t.Fatalf("over limit: status %d body %q", status, body)
	}
}

func TestGetAuthV3SessionTTLClamps(t *testing.T) {
	prev := config.Cfg
	t.Cleanup(func() { config.Cfg = prev })
	for _, tc := range []struct {
		in, want time.Duration
	}{
		{0, DefaultAuthV3SessionTTL},
		{-time.Second, DefaultAuthV3SessionTTL},
		{time.Second, MinAuthV3SessionTTL},
		{45 * time.Minute, 45 * time.Minute},
		{400 * 24 * time.Hour, MaxAuthV3SessionTTL},
	} {
		config.Cfg.HarukiBotDB.AuthV3SessionTTL = tc.in
		if got := getAuthV3SessionTTL(); got != tc.want {
			t.Fatalf("ttl(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestRegisterBotRoutesMountsAuthV3OnlyWithKeyRing(t *testing.T) {
	ctx := context.Background()
	client := newBotTestClient(t, "v3-routes")
	defer func() { _ = client.Close() }()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	withoutRing := fiber.New()
	RegisterBotRoutes(withoutRing, client, nil)
	resp := sendRawRequest(t, withoutRing, http.MethodPost, AuthV3RouteBase+"/1/auth", []byte("x"))
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("v3 route without key ring: status %d, want 404", resp.StatusCode)
	}

	pair, _ := crypto.GenerateKeyPair()
	ring, _ := crypto.SingleKeyRing(pair)
	withRing := fiber.New()
	RegisterBotRoutesWithBanChecker(withRing, client, nil, ring, nil)
	resp = sendRawRequest(t, withRing, http.MethodPost, AuthV3RouteBase+"/1/auth", []byte("x"))
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("v3 route with key ring: status %d, want 400 from Noise middleware", resp.StatusCode)
	}
}
