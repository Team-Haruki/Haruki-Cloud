package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/core/buildpolicy"
	"haruki-cloud/internal/core/secevent"
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
	return VerifyBotSessionWithPolicy(redisClient, nil, nil)
}

// SessionPolicy re-checks an issued session against build-policy revocations
// (bot, version, build). *buildpolicy.Store implements it; nil means no policy.
type SessionPolicy interface {
	SessionAllowed(botID, clientVersion, buildID string, now time.Time) buildpolicy.Decision
}

// ErrSessionRevokedByPolicy is returned when the build policy has revoked the
// bot, client version or build a live session was issued to.
const ErrSessionRevokedByPolicy = "会话已被撤销，请更新客户端后重新登录"

// VerifyBotSessionWithPolicy is VerifyBotSession plus the policy revocation
// check; policy and reporter may be nil.
func VerifyBotSessionWithPolicy(redisClient *redis.Client, policy SessionPolicy, reporter secevent.Reporter) fiber.Handler {
	return func(c fiber.Ctx) error {
		finish := commandtrace.MeasurePhase(c.Context(), "session_auth")
		defer finish()
		if failure := validateBotSession(c, redisClient, policy, reporter); failure != nil {
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
	return VerifyBotSessionTokenWithPolicy(ctx, redisClient, nil, nil, botID, sessionToken)
}

// VerifyBotSessionTokenWithPolicy is VerifyBotSessionToken followed by the
// policy revocation check on the build identity pinned in the token (`bid`,
// `cv` claims). A revoked session is refused with 403 and reported.
func VerifyBotSessionTokenWithPolicy(ctx context.Context, redisClient *redis.Client, policy SessionPolicy, reporter secevent.Reporter, botID, sessionToken string) *BotSessionFailure {
	if redisClient == nil {
		return &BotSessionFailure{Status: fiber.StatusServiceUnavailable, Message: "会话存储不可用"}
	}
	if botID == "" || sessionToken == "" {
		return &BotSessionFailure{Status: fiber.StatusUnauthorized, Message: ErrBotSessionMissing}
	}
	claims, failure := parseBotSessionClaims(sessionToken)
	if failure != nil {
		return failure
	}
	if claims.botID != botID {
		return &BotSessionFailure{Status: fiber.StatusForbidden, Message: "会话令牌中的 bot_id 与请求 bot_id 不一致"}
	}
	if failure := validateStoredBotSession(ctx, redisClient, botID, sessionToken); failure != nil {
		return failure
	}
	return checkSessionPolicy(ctx, policy, reporter, claims)
}

func checkSessionPolicy(ctx context.Context, policy SessionPolicy, reporter secevent.Reporter, claims botSessionClaims) *BotSessionFailure {
	if policy == nil {
		return nil
	}
	decision := policy.SessionAllowed(claims.botID, claims.clientVersion, claims.buildID, time.Now())
	if decision.Passed || decision.Code == buildpolicy.CodePolicyUnavailable {
		return nil
	}
	secevent.Report(ctx, reporter, secevent.Event{
		Kind: secevent.KindSessionRevoked, BotID: claims.botID, BuildID: claims.buildID,
		ClientVersion: claims.clientVersion, Reason: decision.Code + ": " + decision.Reason, Enforced: !decision.Allowed,
	})
	if decision.Allowed {
		return nil
	}
	return &BotSessionFailure{Status: fiber.StatusForbidden, Message: ErrSessionRevokedByPolicy}
}

func validateBotSession(c fiber.Ctx, redisClient *redis.Client, policy SessionPolicy, reporter secevent.Reporter) *botSessionFailure {
	if redisClient == nil {
		return &botSessionFailure{Status: fiber.StatusServiceUnavailable, Message: "会话存储不可用"}
	}
	headerBotID, sessionToken, failure := botSessionHeaders(c)
	if failure != nil {
		return failure
	}
	return VerifyBotSessionTokenWithPolicy(c.Context(), redisClient, policy, reporter, headerBotID, sessionToken)
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

// botSessionClaims is the subset of session JWT claims the middleware uses.
type botSessionClaims struct {
	botID         string
	buildID       string
	clientVersion string
}

func parseBotSessionClaims(sessionToken string) (botSessionClaims, *botSessionFailure) {
	decoded, err := jwt.Parse(sessionToken, botSessionSigningKey, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !decoded.Valid {
		return botSessionClaims{}, &botSessionFailure{Status: fiber.StatusUnauthorized, Message: "会话令牌无效或已过期"}
	}
	claims, ok := decoded.Claims.(jwt.MapClaims)
	if !ok {
		return botSessionClaims{}, &botSessionFailure{Status: fiber.StatusUnauthorized, Message: "会话令牌声明无效"}
	}
	botID, _ := claims["bot_id"].(string)
	buildID, _ := claims["bid"].(string)
	clientVersion, _ := claims["cv"].(string)
	return botSessionClaims{botID: botID, buildID: buildID, clientVersion: clientVersion}, nil
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
