package pjsk

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"haruki-cloud/api"
	"haruki-cloud/config"
	"haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/middleware/secure"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/shamaton/msgpack/v3"
)

func payloadSessionTestSetup(t *testing.T) (*redis.Client, string) {
	t.Helper()
	prev := config.Cfg
	config.Cfg.HarukiBotDB.SessionSignToken = "payload-session-sign"
	t.Cleanup(func() { config.Cfg = prev })

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	token := signPayloadSessionToken(t, "42")
	if err := client.Set(t.Context(), fmt.Sprintf(api.RedisKeyBotSession, "42"), token, time.Hour).Err(); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return client, token
}

func signPayloadSessionToken(t *testing.T, botID string) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"bot_id": botID,
		"exp":    time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(config.Cfg.HarukiBotDB.SessionSignToken))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func postJSON(t *testing.T, app *fiber.App, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestVerifyBotSessionFromPayloadJSON(t *testing.T) {
	client, token := payloadSessionTestSetup(t)
	app := fiber.New()
	app.Post("/api/v2/bot/:botId/pjsk/x", verifyBotSessionFromPayload(client, nil, nil), func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	if status, body := postJSON(t, app, "/api/v2/bot/42/pjsk/x", `{"session_token":"`+token+`","platform":"qq"}`); status != fiber.StatusNoContent {
		t.Fatalf("valid token: status %d body %s", status, body)
	}
	if status, body := postJSON(t, app, "/api/v2/bot/42/pjsk/x", `{"platform":"qq"}`); status != fiber.StatusUnauthorized || !bytes.Contains([]byte(body), []byte(api.ErrBotSessionMissing)) {
		t.Fatalf("missing token: status %d body %s", status, body)
	}
	if status, _ := postJSON(t, app, "/api/v2/bot/43/pjsk/x", `{"session_token":"`+token+`"}`); status != fiber.StatusForbidden {
		t.Fatalf("token for another bot: status %d", status)
	}
	if status, _ := postJSON(t, app, "/api/v2/bot/42/pjsk/x", `{"session_token":"garbage"}`); status != fiber.StatusUnauthorized {
		t.Fatalf("garbage token: status %d", status)
	}
	if status, _ := postJSON(t, app, "/api/v2/bot/42/pjsk/x", `not json`); status != fiber.StatusUnauthorized {
		t.Fatalf("unparseable body: status %d", status)
	}

	// Revocation: once the stored session is gone the same token is refused.
	if err := client.Del(t.Context(), fmt.Sprintf(api.RedisKeyBotSession, "42")).Err(); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if status, _ := postJSON(t, app, "/api/v2/bot/42/pjsk/x", `{"session_token":"`+token+`"}`); status != fiber.StatusUnauthorized {
		t.Fatalf("revoked token: status %d", status)
	}
}

func TestVerifyBotSessionFromPayloadUnderNoiseIsEncrypted(t *testing.T) {
	client, token := payloadSessionTestSetup(t)
	pair, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	app := fiber.New()
	app.Post("/api/v2/bot/:botId/pjsk/x",
		secure.New(secure.Config{ServerPrivateKey: pair}),
		verifyBotSessionFromPayload(client, nil, nil),
		func(c fiber.Ctx) error { return botResponse(c, fiber.StatusOK, "ok") },
	)

	send := func(body any) (int, map[string]any) {
		t.Helper()
		plain, err := msgpack.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		initiator, _ := crypto.NewInitiator(pair.Public)
		ciphertext, _ := initiator.EncryptPacket(plain)
		resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/api/v2/bot/42/pjsk/x", bytes.NewReader(ciphertext)))
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.Header.Get("Content-Type") != "application/octet-stream" {
			t.Fatalf("response not Noise-wrapped: %q", resp.Header.Get("Content-Type"))
		}
		decrypted, err := initiator.DecryptPacket(raw)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		var envelope map[string]any
		if err := msgpack.Unmarshal(decrypted, &envelope); err != nil {
			t.Fatalf("decode msgpack envelope: %v (%q)", err, decrypted)
		}
		return resp.StatusCode, envelope
	}

	if status, env := send(map[string]any{"session_token": token, "platform": "qq"}); status != fiber.StatusOK || env["message"] != "ok" {
		t.Fatalf("valid: status %d envelope %v", status, env)
	}
	// The rejection travels inside the Noise ciphertext as a MsgPack envelope,
	// never as plaintext JSON.
	if status, env := send(map[string]any{"platform": "qq"}); status != fiber.StatusUnauthorized || env["message"] != api.ErrBotSessionMissing {
		t.Fatalf("missing: status %d envelope %v", status, env)
	}
}

func TestVerifyBotSessionFromPayloadBypassesWithoutRedis(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v2/bot/:botId/pjsk/x", verifyBotSessionFromPayload(nil, nil, nil), func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})
	if status, _ := postJSON(t, app, "/api/v2/bot/42/pjsk/x", `{}`); status != fiber.StatusNoContent {
		t.Fatalf("nil redis must bypass, got %d", status)
	}
}
