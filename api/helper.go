package api

import (
	"context"
	"haruki-cloud/config"
	harukiRedis "haruki-cloud/utils/redis"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/shamaton/msgpack/v3"
)

func BuildResponseMap(status int, message string, data interface{}) fiber.Map {
	return fiber.Map{
		"status":  status,
		"message": message,
		"data":    data,
	}
}

func JSONResponse(c fiber.Ctx, status int, message string, data ...interface{}) error {
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
func MsgPackResponse(c fiber.Ctx, status int, message string, data ...interface{}) error {
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
	c.Set("Content-Type", "application/msgpack")
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
	data interface{},
) error {
	resp := BuildResponseMap(status, message, data)
	if redisClient != nil {
		if err := harukiRedis.SetCache(ctx, redisClient, key, resp, ttl); err != nil {
		}
	}
	return c.Status(status).JSON(resp)
}

func VerifyAPIAuthorization() fiber.Handler {
	return func(c fiber.Ctx) error {
		expectedAuth := strings.TrimSpace(config.Cfg.Backend.AcceptAuthorization)
		if expectedAuth == "" {
			if token := strings.TrimSpace(config.Cfg.HarukiBotDB.InternalAPIToken); token != "" {
				if strings.HasPrefix(strings.ToLower(token), "bearer ") {
					expectedAuth = token
				} else {
					expectedAuth = "Bearer " + token
				}
			}
		}
		expectedUserAgent := strings.TrimSpace(config.Cfg.Backend.AcceptUserAgent)

		if expectedAuth == "" && expectedUserAgent == "" && !config.Cfg.Backend.AllowInsecureInternalAPI {
			return JSONResponse(c, fiber.StatusServiceUnavailable, "Internal API authorization is not configured")
		}

		authHeader := strings.TrimSpace(c.Get("Authorization"))
		userAgent := c.Get("User-Agent")

		if expectedAuth != "" && authHeader != expectedAuth {
			return JSONResponse(c, fiber.StatusUnauthorized, "Invalid Authorization header")
		}

		if expectedUserAgent != "" && !strings.Contains(userAgent, expectedUserAgent) {
			return JSONResponse(c, fiber.StatusForbidden, "Invalid User-Agent")
		}

		return c.Next()
	}
}

func CacheQuery(ctx context.Context, c fiber.Ctx, redisClient *redis.Client, namespace string) (string, map[string]any, bool, error) {
	key := harukiRedis.CacheKeyBuilder(c, namespace)
	if redisClient == nil {
		return key, nil, false, nil
	}
	var cached map[string]any
	found, err := harukiRedis.GetCache(ctx, redisClient, key, &cached)
	if err != nil {
		return key, nil, false, err
	}
	if found {
		return key, cached, true, nil
	}
	return key, nil, false, nil
}

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
