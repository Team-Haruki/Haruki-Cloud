package api

import (
	"context"
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
		if failure := validateBotSession(c, redisClient); failure != nil {
			return JSONResponse(c, failure.Status, failure.Message)
		}
		return c.Next()
	}
}

// ErrBotSessionMissing is the message returned when a request carries no
// session token at all (header or payload, depending on the route).
const ErrBotSessionMissing = "缺少会话令牌"

// BotSessionFailure describes why a session token was rejected.
type BotSessionFailure struct {
	Status  int
	Message string
}

type botSessionFailure = BotSessionFailure

// VerifyBotSessionToken validates a session token against a bot id: JWT
// signature and expiry, bot_id claim equality, and the Redis-stored session
// (revocation). It is transport agnostic so both the header-based manifest
// route and the Noise-payload-based command routes share one rule set.
func VerifyBotSessionToken(ctx context.Context, redisClient *redis.Client, botID, sessionToken string) *BotSessionFailure {
	if redisClient == nil {
		return &BotSessionFailure{Status: fiber.StatusServiceUnavailable, Message: "会话存储不可用"}
	}
	if botID == "" || sessionToken == "" {
		return &BotSessionFailure{Status: fiber.StatusUnauthorized, Message: ErrBotSessionMissing}
	}
	claimBotID, failure := parseBotSessionClaim(sessionToken)
	if failure != nil {
		return failure
	}
	if claimBotID != botID {
		return &BotSessionFailure{Status: fiber.StatusForbidden, Message: "会话令牌中的 bot_id 与请求 bot_id 不一致"}
	}
	return validateStoredBotSession(ctx, redisClient, botID, sessionToken)
}

func validateBotSession(c fiber.Ctx, redisClient *redis.Client) *botSessionFailure {
	if redisClient == nil {
		return &botSessionFailure{Status: fiber.StatusServiceUnavailable, Message: "会话存储不可用"}
	}
	headerBotID, sessionToken, failure := botSessionHeaders(c)
	if failure != nil {
		return failure
	}
	return VerifyBotSessionToken(c.Context(), redisClient, headerBotID, sessionToken)
}

func botSessionHeaders(c fiber.Ctx) (string, string, *botSessionFailure) {
	headerBotID := c.Get(HeaderBotID)
	sessionToken := c.Get(HeaderBotSessionToken)
	if headerBotID == "" || sessionToken == "" {
		return "", "", &botSessionFailure{Status: fiber.StatusUnauthorized, Message: "缺少 " + HeaderBotID + " 或 " + HeaderBotSessionToken + " 请求头"}
	}
	if c.Params("botId") != headerBotID {
		return "", "", &botSessionFailure{Status: fiber.StatusForbidden, Message: "请求头中的 bot_id 与 URL 参数不一致"}
	}
	return headerBotID, sessionToken, nil
}

func parseBotSessionClaim(sessionToken string) (string, *botSessionFailure) {
	decoded, err := jwt.Parse(sessionToken, botSessionSigningKey, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !decoded.Valid {
		return "", &botSessionFailure{Status: fiber.StatusUnauthorized, Message: "会话令牌无效或已过期"}
	}
	claims, ok := decoded.Claims.(jwt.MapClaims)
	if !ok {
		return "", &botSessionFailure{Status: fiber.StatusUnauthorized, Message: "会话令牌声明无效"}
	}
	claimBotID, _ := claims["bot_id"].(string)
	return claimBotID, nil
}

func botSessionSigningKey(token *jwt.Token) (any, error) {
	if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, errors.New("unexpected signing method")
	}
	return []byte(config.Cfg.HarukiBotDB.SessionSignToken), nil
}

func validateStoredBotSession(ctx context.Context, redisClient *redis.Client, botID, sessionToken string) *botSessionFailure {
	stored, err := redisClient.Get(ctx, fmt.Sprintf(RedisKeyBotSession, botID)).Result()
	if errors.Is(err, redis.Nil) {
		return &botSessionFailure{Status: fiber.StatusUnauthorized, Message: "会话已过期或不存在"}
	}
	if err != nil {
		return &botSessionFailure{Status: fiber.StatusInternalServerError, Message: ErrInternalServer}
	}
	if stored != sessionToken {
		return &botSessionFailure{Status: fiber.StatusUnauthorized, Message: "会话令牌不匹配"}
	}
	return nil
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
