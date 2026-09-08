package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"haruki-cloud/config"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"
	harukiRedis "haruki-cloud/utils/redis"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/shamaton/msgpack/v3"
)

func BuildResponseMap(status int, message string, data any) fiber.Map {
	return fiber.Map{
		"status":  status,
		"message": message,
		"data":    data,
	}
}

func JSONResponse(c fiber.Ctx, status int, message string, data ...any) error {
	finish := commandtrace.MeasurePhase(c.Context(), "response_encode")
	defer finish()
	var resp fiber.Map
	if len(data) > 0 {
		resp = BuildResponseMap(status, message, data[0])
	} else {
		resp = BuildResponseMap(status, message, nil)
	}
	return c.Status(status).JSON(resp)
}

// MsgPackResponse writes a MsgPack-encoded response envelope.
// Used when the request arrived through the Noise IK transport layer.
func MsgPackResponse(c fiber.Ctx, status int, message string, data ...any) error {
	finish := commandtrace.MeasurePhase(c.Context(), "response_encode")
	defer finish()
	var resp fiber.Map
	if len(data) > 0 {
		resp = BuildResponseMap(status, message, data[0])
	} else {
		resp = BuildResponseMap(status, message, nil)
	}
	encoded, err := msgpack.Marshal(resp)
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	c.Set("Content-Type", ContentTypeMsgPack)
	return c.Status(status).Send(encoded)
}

func ErrorResponse(c fiber.Ctx, status int, message string) error {
	return JSONResponse(c, status, message)
}

func InternalError(c fiber.Ctx) error {
	return JSONResponse(c, fiber.StatusInternalServerError, ErrInternalServer)
}

func CachedJSONResponse(
	ctx context.Context,
	c fiber.Ctx,
	redisClient *redis.Client,
	ttl time.Duration,
	key string,
	status int,
	message string,
	data any,
) error {
	finish := commandtrace.MeasurePhase(c.Context(), "response_encode")
	defer finish()
	resp := BuildResponseMap(status, message, data)
	encoded, err := c.App().Config().JSONEncoder(resp)
	if err != nil {
		return err
	}
	if redisClient != nil {
		_ = redisClient.Set(ctx, key, encoded, ttl).Err() // best-effort cache write
	}
	return SendCachedJSON(c, status, encoded)
}

func VerifyAPIAuthorization() fiber.Handler {
	return func(c fiber.Ctx) error {
		expectedAuth := configuredInternalAPIAuthorization()
		expectedUserAgent := strings.TrimSpace(config.Cfg.Backend.AcceptUserAgent)

		// Require at least an Authorization token to be configured.
		// User-Agent alone is not sufficient as it can be trivially forged.
		if expectedAuth == "" && !config.Cfg.Backend.AllowInsecureInternalAPI {
			return JSONResponse(c, fiber.StatusServiceUnavailable, "Internal API authorization is not configured")
		}

		authHeader := strings.TrimSpace(c.Get("Authorization"))
		userAgent := c.Get("User-Agent")

		if expectedAuth != "" && subtle.ConstantTimeCompare([]byte(authHeader), []byte(expectedAuth)) != 1 {
			return JSONResponse(c, fiber.StatusUnauthorized, "Invalid Authorization header")
		}

		if expectedUserAgent != "" && !strings.Contains(userAgent, expectedUserAgent) {
			return JSONResponse(c, fiber.StatusForbidden, "Invalid User-Agent")
		}

		return c.Next()
	}
}

func configuredInternalAPIAuthorization() string {
	if authorization := strings.TrimSpace(config.Cfg.Backend.AcceptAuthorization); authorization != "" {
		return authorization
	}
	token := strings.TrimSpace(config.Cfg.HarukiBotDB.InternalAPIToken)
	if token == "" || strings.HasPrefix(strings.ToLower(token), strings.ToLower(AuthBearerPrefix)) {
		return token
	}
	return AuthBearerPrefix + token
}

func CacheQuery(ctx context.Context, c fiber.Ctx, redisClient *redis.Client, namespace string) (string, []byte, bool, error) {
	key := cacheKeyFromFiberCtx(c, namespace)
	if redisClient == nil {
		return key, nil, false, nil
	}
	cached, err := redisClient.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return key, nil, false, nil
	}
	if err != nil {
		return key, nil, false, err
	}
	trimmed := bytes.TrimSpace(cached)
	if !json.Valid(trimmed) || (trimmed[0] != '{' && !bytes.Equal(trimmed, []byte("null"))) {
		return key, nil, false, fmt.Errorf("invalid cached JSON response")
	}
	return key, cached, true, nil
}

func SendCachedJSON(c fiber.Ctx, status int, encoded []byte) error {
	c.Set(fiber.HeaderContentType, ContentTypeJSON)
	return c.Status(status).Send(encoded)
}

// WithCache is a convenience wrapper that handles the cache-check boilerplate.
// fetchFn receives the cache key and should return (data, error). On success
// the data is written as a cached JSON response; on cache hit the handler
// returns immediately. If fetchFn returns a *CacheBypassError the handler
// returns that error's response directly (for 404 / 400 cases).
func WithCache(c fiber.Ctx, redisClient *redis.Client, namespace string, fetchFn func(key string) (any, error)) error {
	ctx := c.Context()
	key, cached, hit, err := CacheQuery(ctx, c, redisClient, namespace)
	if err != nil {
		return InternalError(c)
	}
	if hit {
		return SendCachedJSON(c, fiber.StatusOK, cached)
	}
	data, err := fetchFn(key)
	if err != nil {
		var bypass *CacheBypassError
		if errors.As(err, &bypass) {
			return bypass.Response
		}
		return InternalError(c)
	}
	return CachedJSONResponse(ctx, c, redisClient, config.Cfg.Backend.APICacheTTL, key, fiber.StatusOK, ResponseOK, data)
}

// CacheBypassError wraps a pre-built fiber response for WithCache fetch functions
// that need to return non-200 responses (e.g., 404).
type CacheBypassError struct {
	Response error
}

func (e *CacheBypassError) Error() string { return "cache bypass" }

// ================= User ID Extraction =================

func GetHarukiUserIDFromPath(c fiber.Ctx) int {
	return fiber.Params[int](c, "haruki_user_id", 0)
}

func GetHarukiUserIDFromQuery(c fiber.Ctx) int {
	userIDStr := c.Query("haruki_user_id", "0")
	id, err := strconv.Atoi(userIDStr)
	if err != nil {
		return 0
	}
	return id
}

// ================= Validation Functions =================

func ValidateStringLength(s string, maxLen int) bool {
	return utf8.RuneCountInString(s) <= maxLen
}

func ValidateAlias(alias string) bool {
	if alias == "" {
		return false
	}
	return ValidateStringLength(alias, MaxAliasLength)
}

func ValidateServer(server string) bool {
	if server == "" {
		return false
	}
	return ValidateStringLength(server, MaxServerLength)
}

func cacheKeyFromFiberCtx(c fiber.Ctx, namespace string) string {
	fullPath := c.Path()
	queryString := c.RequestCtx().QueryArgs().String()
	canonicalQuery := harukiRedis.CanonicalizeQueryString(queryString)

	queryHash := "none"
	if canonicalQuery != "" {
		queryHash = harukiRedis.CanonicalQueryHash(canonicalQuery)
	}

	return fmt.Sprintf("%s:%s:query=%s", namespace, fullPath, queryHash)
}
