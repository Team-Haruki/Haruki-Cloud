package api

import (
	"errors"
	"fmt"

	"haruki-cloud/config"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

const (
	// HeaderBotID is sent by the Bot client on every authenticated request.
	HeaderBotID = "X-Haruki-Bot-Id"
	// HeaderBotSessionToken carries the JWT session token issued by POST /bot/:bot_id/auth.
	HeaderBotSessionToken = "X-Haruki-Bot-Session-Token"

	// redisBotSessionKey is the Redis key pattern for stored session tokens.
	// Matches the pattern defined in api/bot/struct.go (RedisKeySessionToken).
	redisBotSessionKey = "hdb:bot:session:%s"
)

// VerifyBotSession returns a Fiber middleware that authenticates Bot requests by:
//  1. Requiring X-Haruki-Bot-Id and X-Haruki-Bot-Session-Token headers
//  2. Checking X-Haruki-Bot-Id matches the :botId URL parameter
//  3. Validating the JWT session token signature and expiry
//  4. Confirming the JWT bot_id claim matches X-Haruki-Bot-Id
//  5. Looking up the session token in Redis to ensure it has not been revoked
//
// If redisClient is nil the middleware returns 503 Service Unavailable.
// Use VerifyBotSessionTestBypass for testing without Redis.
func VerifyBotSession(redisClient *redis.Client) fiber.Handler {
	return func(c fiber.Ctx) error {
		if redisClient == nil {
			return JSONResponse(c, fiber.StatusServiceUnavailable, "session store unavailable")
		}

		headerBotID := c.Get(HeaderBotID)
		sessionToken := c.Get(HeaderBotSessionToken)

		if headerBotID == "" || sessionToken == "" {
			return JSONResponse(c, fiber.StatusUnauthorized,
				"missing "+HeaderBotID+" or "+HeaderBotSessionToken+" header")
		}

		// The URL parameter name is "botId" as registered by the route group.
		urlBotID := c.Params("botId")
		if urlBotID != headerBotID {
			return JSONResponse(c, fiber.StatusForbidden,
				"bot_id in header does not match URL parameter")
		}

		// Validate JWT signature and expiry.
		decoded, err := jwt.Parse(sessionToken, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(config.Cfg.HarukiBotDB.SessionSignToken), nil
		})
		if err != nil || !decoded.Valid {
			return JSONResponse(c, fiber.StatusUnauthorized, "invalid or expired session token")
		}

		claims, ok := decoded.Claims.(jwt.MapClaims)
		if !ok {
			return JSONResponse(c, fiber.StatusUnauthorized, "invalid token claims")
		}

		claimBotID, _ := claims["bot_id"].(string)
		if claimBotID != headerBotID {
			return JSONResponse(c, fiber.StatusForbidden,
				"token bot_id does not match request bot_id")
		}

		// Verify the session has not been revoked (Redis lookup).
		key := fmt.Sprintf(redisBotSessionKey, headerBotID)
		stored, err := redisClient.Get(c.Context(), key).Result()
		if errors.Is(err, redis.Nil) {
			return JSONResponse(c, fiber.StatusUnauthorized, "session expired or not found")
		}
		if err != nil {
			return InternalError(c)
		}
		if stored != sessionToken {
			return JSONResponse(c, fiber.StatusUnauthorized, "session token mismatch")
		}

		return c.Next()
	}
}

// VerifyBotSessionTestBypass returns a no-op middleware that skips session
// validation. Use ONLY in test code where no Redis connection is available.
func VerifyBotSessionTestBypass() fiber.Handler {
	return func(c fiber.Ctx) error {
		return c.Next()
	}
}
