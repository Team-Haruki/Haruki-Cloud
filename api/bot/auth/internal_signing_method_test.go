package auth

import (
	"fmt"
	"net/http"
	"testing"

	"haruki-cloud/config"
	json "haruki-cloud/internal/jsonutil"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

func TestVerifySessionRejectsNonHS256Tokens(t *testing.T) {
	previous := config.Cfg.HarukiBotDB.SessionSignToken
	config.Cfg.HarukiBotDB.SessionSignToken = "session-sign-token"
	t.Cleanup(func() { config.Cfg.HarukiBotDB.SessionSignToken = previous })

	app := fiber.New()
	app.Post("/verify-session", NewInternalHandler(NewInternalServiceWithStore(nil, nil)).VerifySession)

	for _, method := range []jwt.SigningMethod{jwt.SigningMethodHS384, jwt.SigningMethodHS512} {
		t.Run(method.Alg(), func(t *testing.T) {
			rawToken, err := jwt.NewWithClaims(method, jwt.MapClaims{"bot_id": "42"}).SignedString([]byte(config.Cfg.HarukiBotDB.SessionSignToken))
			if err != nil {
				t.Fatalf("sign token: %v", err)
			}
			body := fmt.Sprintf(`{"bot_id":"42","session_token":%q}`, rawToken)
			response := sendJSONRequest(t, app, http.MethodPost, "/verify-session", body, nil)
			if response.Status != fiber.StatusOK {
				t.Fatalf("status = %d, want %d", response.Status, fiber.StatusOK)
			}
			var result InternalVerifyResponse
			if err := json.Unmarshal(response.Data, &result); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if result.Valid {
				t.Fatalf("VerifySession accepted %s", method.Alg())
			}
		})
	}
}
