package api

import (
	"errors"
	"fmt"

	"haruki-cloud/config"
	"haruki-cloud/internal/observability/commandtrace"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

const (
	// HeaderBotID is sent by the Bot client on every authenticated request.
	HeaderBotID = "X-Haruki-Bot-Id"
	// HeaderBotSessionToken carries the JWT session token issued by POST /bot/:bot_id/auth.
	HeaderBotSessionToken = "X-Haruki-Bot-Session-Token"
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
		finish := commandtrace.MeasurePhase(c.Context(), "session_auth")
		defer finish()
		if redisClient == nil {
			finish()
			return JSONResponse(c, fiber.StatusServiceUnavailable, "会话存储不可用")
		}

		headerBotID := c.Get(HeaderBotID)
		sessionToken := c.Get(HeaderBotSessionToken)

		if headerBotID == "" || sessionToken == "" {
			finish()
			return JSONResponse(c, fiber.StatusUnauthorized,
				"缺少 "+HeaderBotID+" 或 "+HeaderBotSessionToken+" 请求头")
		}

		// The URL parameter name is "botId" as registered by the route group.
		urlBotID := c.Params("botId")
		if urlBotID != headerBotID {
			finish()
			return JSONResponse(c, fiber.StatusForbidden,
				"请求头中的 bot_id 与 URL 参数不一致")
		}

		// Validate JWT signature and expiry.
		decoded, err := jwt.Parse(sessionToken, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(config.Cfg.HarukiBotDB.SessionSignToken), nil
		})
		if err != nil || !decoded.Valid {
			finish()
			return JSONResponse(c, fiber.StatusUnauthorized, "会话令牌无效或已过期")
		}

		claims, ok := decoded.Claims.(jwt.MapClaims)
		if !ok {
			finish()
			return JSONResponse(c, fiber.StatusUnauthorized, "会话令牌声明无效")
		}

		claimBotID, _ := claims["bot_id"].(string)
		if claimBotID != headerBotID {
			finish()
			return JSONResponse(c, fiber.StatusForbidden,
				"会话令牌中的 bot_id 与请求 bot_id 不一致")
		}

		// Verify the session has not been revoked (Redis lookup).
		key := fmt.Sprintf(RedisKeyBotSession, headerBotID)
		stored, err := redisClient.Get(c.Context(), key).Result()
		if errors.Is(err, redis.Nil) {
			finish()
			return JSONResponse(c, fiber.StatusUnauthorized, "会话已过期或不存在")
		}
		if err != nil {
			finish()
			return InternalError(c)
		}
		if stored != sessionToken {
			finish()
			return JSONResponse(c, fiber.StatusUnauthorized, "会话令牌不匹配")
		}

		finish()
		return c.Next()
	}
}

// VerifyBotSessionTestBypass returns a no-op middleware that skips session
// validation. Use ONLY in test code where no Redis connection is available.
func VerifyBotSessionTestBypass() fiber.Handler {
	return func(c fiber.Ctx) error {
		finish := commandtrace.MeasurePhase(c.Context(), "session_auth")
		finish()
		return c.Next()
	}
}
