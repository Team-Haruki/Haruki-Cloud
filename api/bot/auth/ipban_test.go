package auth

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"haruki-cloud/api"
	"haruki-cloud/internal/core/authban"
	"haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/i18n"
	json "haruki-cloud/internal/jsonutil"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	noiseMP "github.com/shamaton/msgpack/v3"
)

const banTestIP = "203.0.113.50"

type ipBanTestEnv struct {
	*authV3TestEnv
	app   *fiber.App
	guard *authban.Guard
	mr    *miniredis.Miniredis
}

// newIPBanTestEnv mounts the real AuthV3 route (IP ban, Noise, handler) and
// the admin routes on an app that trusts the test peer as its reverse
// proxy, so X-Forwarded-For carries the client address like in production.
func newIPBanTestEnv(t *testing.T, cfg authban.Config) *ipBanTestEnv {
	t.Helper()
	base := newAuthV3TestEnv(t)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	guard, err := authban.New(cfg, rdb, nil)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New(fiber.Config{
		ProxyHeader:        fiber.HeaderXForwardedFor,
		EnableIPValidation: true,
		TrustProxy:         true,
		TrustProxyConfig:   fiber.TrustProxyConfig{Proxies: []string{"0.0.0.0/32"}},
	})
	registerAuthV3Routes(app, NewUserHandler(base.svc), base.ring, guard)
	registerIPBanAdminRoutes(app, guard)
	return &ipBanTestEnv{authV3TestEnv: base, app: app, guard: guard, mr: mr}
}

type rawAuthResponse struct {
	status     int
	body       []byte
	retryAfter string
	wrapped    bool
}

func (e *ipBanTestEnv) post(t *testing.T, ip, botID string, body []byte) rawAuthResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, AuthV3RouteBase+"/"+botID+"/auth", bytes.NewReader(body))
	req.Header.Set(fiber.HeaderXForwardedFor, ip)
	resp, err := e.app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return rawAuthResponse{
		status: resp.StatusCode, body: raw, retryAfter: resp.Header.Get(fiber.HeaderRetryAfter),
		wrapped: resp.Header.Get("Content-Type") == "application/octet-stream",
	}
}

// login performs one Noise round trip and returns the status and the
// decrypted body.
func (e *ipBanTestEnv) login(t *testing.T, ip string, payload AuthPayloadV3) (int, []byte) {
	t.Helper()
	initiator, err := crypto.NewInitiator(e.current.Public)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := noiseMP.Marshal(payload)
	ciphertext, err := initiator.EncryptPacket(plain)
	if err != nil {
		t.Fatal(err)
	}
	resp := e.post(t, ip, payload.BotID, ciphertext)
	if !resp.wrapped {
		return resp.status, resp.body
	}
	decrypted, err := initiator.DecryptPacket(resp.body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.status, decrypted
}

func (e *ipBanTestEnv) wrongCredential(t *testing.T) AuthPayloadV3 {
	t.Helper()
	payload := e.basePayload(t)
	payload.Credential = signTestCredential(t, e.botStr, "not-the-credential")
	return payload
}

func TestIPBanCountsLoginFailuresAndRejectsBeforeNoise(t *testing.T) {
	env := newIPBanTestEnv(t, authban.Config{Threshold: 4})

	// Wrong credential, malformed Noise body, replayed nonce, wrong credential.
	if status, body := env.login(t, banTestIP, env.wrongCredential(t)); status != fiber.StatusBadRequest {
		t.Fatalf("wrong credential: %d %q", status, body)
	}
	if resp := env.post(t, banTestIP, env.botStr, []byte("not a noise message")); resp.status != fiber.StatusBadRequest {
		t.Fatalf("garbage body: %d", resp.status)
	}
	replayed := env.wrongCredential(t)
	env.login(t, banTestIP, replayed)
	if status, body := env.login(t, banTestIP, replayed); status != fiber.StatusBadRequest || string(body) != ErrReplayDetected {
		t.Fatalf("replay: %d %q", status, body)
	}
	st, _ := env.guard.Inspect(context.Background(), banTestIP)
	if st.Ban == nil || st.Ban.Failures != 4 || len(st.Ban.BotIDs) != 1 || st.Ban.BotIDs[0] != env.botStr {
		t.Fatalf("after four failures: %+v", st)
	}

	// Banned: refused in plaintext before the Noise middleware, even with a
	// valid login and even with a body Noise would reject.
	for _, body := range [][]byte{[]byte("garbage"), nil} {
		resp := env.post(t, banTestIP, env.botStr, body)
		if resp.status != fiber.StatusTooManyRequests || resp.wrapped || resp.retryAfter == "" {
			t.Fatalf("banned request: %+v", resp)
		}
		if string(resp.body) != i18n.T("account.api.auth_banned") || strings.Contains(string(resp.body), banTestIP) {
			t.Fatalf("banned body = %q", resp.body)
		}
	}
	if status, _ := env.login(t, banTestIP, env.basePayload(t)); status != fiber.StatusTooManyRequests {
		t.Fatalf("valid login from banned address: %d", status)
	}

	// The same bot from another address is fine.
	if status, body := env.login(t, "198.51.100.8", env.basePayload(t)); status != fiber.StatusOK {
		t.Fatalf("other address: %d %q", status, body)
	}
}

func TestIPBanIgnoresRejectionsThatAreNotTheClientsFault(t *testing.T) {
	env := newIPBanTestEnv(t, authban.Config{Threshold: 1, ExemptKnownBots: true})
	env.ban.banned = true // owner globally banned: 403, not counted
	if status, _ := env.login(t, banTestIP, env.basePayload(t)); status != fiber.StatusForbidden {
		t.Fatalf("owner banned: %d", status)
	}
	env.ban.banned = false
	if st, _ := env.guard.Inspect(context.Background(), banTestIP); st.Failures != 0 || st.Ban != nil {
		t.Fatalf("owner ban counted: %+v", st)
	}
	// A success does not count either, and makes the bot known.
	if status, _ := env.login(t, banTestIP, env.basePayload(t)); status != fiber.StatusOK {
		t.Fatalf("login: %d", status)
	}
	st, _ := env.guard.Inspect(context.Background(), banTestIP)
	if st.Failures != 0 || len(st.KnownBotIDs) != 1 || st.KnownBotIDs[0] != env.botStr {
		t.Fatalf("after success: %+v", st)
	}
}

func TestIPBanLeavesIssuedSessionsWorking(t *testing.T) {
	env := newIPBanTestEnv(t, authban.Config{Threshold: 2, ExemptKnownBots: false})
	status, body := env.login(t, banTestIP, env.basePayload(t))
	if status != fiber.StatusOK {
		t.Fatalf("login: %d %q", status, body)
	}
	var issued AuthResponseV3
	if err := noiseMP.Unmarshal(body, &issued); err != nil {
		t.Fatal(err)
	}

	// Another bot on the same address fails until the address is banned.
	for range 2 {
		env.post(t, banTestIP, "424242", []byte("bad"))
	}
	if resp := env.post(t, banTestIP, env.botStr, []byte("x")); resp.status != fiber.StatusTooManyRequests {
		t.Fatalf("address not banned: %d", resp.status)
	}

	// The session issued before the ban still verifies and can log out:
	// only the login route is guarded by default.
	verify := sendJSONRequest(t, env.authV3TestEnv.app, http.MethodPost, "/internal/bot/verify-session",
		`{"bot_id":"`+env.botStr+`","session_token":"`+issued.SessionToken+`"}`,
		map[string]string{"Authorization": "Bearer internal-test"})
	var verified InternalVerifyResponse
	if err := json.Unmarshal(verify.Data, &verified); err != nil || !verified.Valid {
		t.Fatalf("session after ban: %+v %v", verified, err)
	}
	logoutApp := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor, EnableIPValidation: true, TrustProxy: true,
		TrustProxyConfig: fiber.TrustProxyConfig{Proxies: []string{"0.0.0.0/32"}}})
	registerAuthV3Routes(logoutApp, NewUserHandler(env.svc), env.ring, env.guard)
	req := httptest.NewRequest(http.MethodDelete, AuthV3RouteBase+"/"+env.botStr+"/logout", nil)
	req.Header.Set(fiber.HeaderXForwardedFor, banTestIP)
	req.Header.Set(api.HeaderBotSessionToken, issued.SessionToken)
	resp, err := logoutApp.Test(req)
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("logout from banned address: %v %v", resp, err)
	}
}

func TestMarkLoginFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    *authResponseError
		reason authban.Reason
		want   int // expected guard failures after one request
	}{
		{"bad request", &authResponseError{status: fiber.StatusBadRequest}, authban.ReasonAuthFailed, 1},
		{"replay", &authResponseError{status: fiber.StatusBadRequest, replay: true}, authban.ReasonAuthFailed, 1},
		{"build rejected", &authResponseError{status: fiber.StatusForbidden}, authban.ReasonBuildRejected, 1},
		{"owner banned", &authResponseError{status: fiber.StatusForbidden}, authban.ReasonAuthFailed, 0},
		{"server error", &authResponseError{status: fiber.StatusInternalServerError}, authban.ReasonAuthFailed, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mr := miniredis.RunT(t)
			rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			defer rdb.Close()
			guard, err := authban.New(authban.Config{CountBuildRejected: true}, rdb, nil)
			if err != nil {
				t.Fatal(err)
			}
			app := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor, EnableIPValidation: true, TrustProxy: true,
				TrustProxyConfig: fiber.TrustProxyConfig{Proxies: []string{"0.0.0.0/32"}}})
			app.Post("/:bot_id", guard.LoginMiddleware("bot_id"), func(c fiber.Ctx) error {
				markLoginFailure(c, tc.err, tc.reason)
				return c.SendStatus(tc.err.status)
			})
			req := httptest.NewRequest(http.MethodPost, "/1", nil)
			req.Header.Set(fiber.HeaderXForwardedFor, banTestIP)
			if _, err := app.Test(req); err != nil {
				t.Fatal(err)
			}
			st, _ := guard.Inspect(context.Background(), banTestIP)
			if st.Failures != int64(tc.want) {
				t.Fatalf("failures = %d, want %d", st.Failures, tc.want)
			}
		})
	}
}

func TestIPBanAdminRoutes(t *testing.T) {
	env := newIPBanTestEnv(t, authban.Config{Threshold: 1})
	env.post(t, banTestIP, env.botStr, []byte("bad"))
	auth := map[string]string{"Authorization": "Bearer internal-test"}

	list := sendJSONRequest(t, env.app, http.MethodGet, IPBanAdminPath, "", auth)
	var bans ipBanAdminResponse
	if err := json.Unmarshal(list.Data, &bans); err != nil || len(bans.Bans) != 1 || bans.Bans[0].IP != banTestIP {
		t.Fatalf("list = %s %v", list.Data, err)
	}

	one := sendJSONRequest(t, env.app, http.MethodGet, IPBanAdminPath+"?ip="+banTestIP, "", auth)
	var st authban.Status
	if err := json.Unmarshal(one.Data, &st); err != nil || st.Ban == nil || st.Ban.Level != 1 {
		t.Fatalf("inspect = %s %v", one.Data, err)
	}

	bad := sendJSONRequest(t, env.app, http.MethodGet, IPBanAdminPath+"?ip=nope", "", auth)
	if bad.Status != fiber.StatusBadRequest {
		t.Fatalf("bad ip status = %d", bad.Status)
	}
	if res := sendJSONRequest(t, env.app, http.MethodDelete, IPBanAdminPath, "", auth); res.Status != fiber.StatusBadRequest {
		t.Fatalf("unban without ip = %d", res.Status)
	}
	if res := sendJSONRequest(t, env.app, http.MethodGet, IPBanAdminPath, "", nil); res.Status != fiber.StatusUnauthorized {
		t.Fatalf("unauthenticated list = %d", res.Status)
	}

	del := sendJSONRequest(t, env.app, http.MethodDelete, IPBanAdminPath+"?ip="+banTestIP, "", auth)
	var lifted unbanResponse
	if err := json.Unmarshal(del.Data, &lifted); err != nil || !lifted.WasBanned || lifted.IP != banTestIP {
		t.Fatalf("unban = %s %v", del.Data, err)
	}
	if resp := env.post(t, banTestIP, env.botStr, []byte("bad")); resp.status == fiber.StatusTooManyRequests {
		t.Fatal("still banned after unban")
	}

	env.mr.Close()
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, IPBanAdminPath},
		{http.MethodGet, IPBanAdminPath + "?ip=" + banTestIP},
		{http.MethodDelete, IPBanAdminPath + "?ip=" + banTestIP},
	} {
		if res := sendJSONRequest(t, env.app, tc.method, tc.path, "", auth); res.Status != fiber.StatusInternalServerError {
			t.Fatalf("%s %s with Redis down = %d", tc.method, tc.path, res.Status)
		}
	}
}

func TestIPBanAdminRoutesNeedAGuard(t *testing.T) {
	app := fiber.New()
	registerIPBanAdminRoutes(app, nil)
	resp := sendRawRequest(t, app, http.MethodGet, IPBanAdminPath, nil)
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("admin route without guard: %d", resp.StatusCode)
	}
}
