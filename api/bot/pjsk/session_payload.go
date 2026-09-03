package pjsk

import (
	"strings"

	"haruki-cloud/api"
	"haruki-cloud/internal/core/secevent"
	"haruki-cloud/internal/observability/commandtrace"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/shamaton/msgpack/v3"
)

// payloadSessionEnvelope is the minimal view of any bot POST body: every
// request under /pjsk, whatever its full shape, carries the session token at
// the top level so it travels inside the Noise ciphertext rather than in a
// header visible to intermediaries.
type payloadSessionEnvelope struct {
	SessionToken string `json:"session_token" msgpack:"session_token"`
}

// verifyBotSessionFromPayload authenticates bot POST requests from the
// session_token field of the (already decrypted) body. It must run after the
// secure middleware so the body is plaintext, and its rejections are rendered
// through botResponse so they are re-encrypted on the way out.
//
// The bot identity comes from the :botId path parameter; the session token's
// bot_id claim and the Redis-stored session must both match it. A nil Redis
// client bypasses the check (unit tests without Redis), mirroring
// api.VerifyBotSessionTestBypass.
func verifyBotSessionFromPayload(redisClient *redis.Client, policy api.SessionPolicy, reporter secevent.Reporter) fiber.Handler {
	if redisClient == nil {
		return api.VerifyBotSessionTestBypass()
	}
	return func(c fiber.Ctx) error {
		finish := commandtrace.MeasurePhase(c.Context(), "session_auth")
		defer finish()

		var envelope payloadSessionEnvelope
		if strings.Contains(string(c.Request().Header.ContentType()), "msgpack") {
			_ = msgpack.Unmarshal(c.Body(), &envelope)
		} else {
			_ = c.Bind().Body(&envelope)
		}
		token := strings.TrimSpace(envelope.SessionToken)
		if token == "" {
			return botResponse(c, fiber.StatusUnauthorized, api.ErrBotSessionMissing)
		}
		if failure := api.VerifyBotSessionTokenWithPolicy(c.Context(), redisClient, policy, reporter, c.Params("botId"), token); failure != nil {
			return botResponse(c, failure.Status, failure.Message)
		}
		return c.Next()
	}
}
