package pjsk

import (
	"context"
	"net/url"
	"strconv"

	"haruki-cloud/api"
	"haruki-cloud/config"
	pjskalias "haruki-cloud/internal/pjsk/alias"
	harukiRedis "haruki-cloud/utils/redis"

	"github.com/redis/go-redis/v9"
)

const aliasRoutePrefix = "/api/v2/public/pjsk/alias/"

func aliasCacheOptions() api.CacheOptions {
	return api.CacheOptions{
		TTL:         config.Cfg.Backend.AliasAPICacheTTL,
		NotFoundTTL: config.Cfg.Backend.AliasAPINotFoundCacheTTL,
	}
}

// AliasCacheInvalidator returns a listener that drops the cached public
// by-id and by-alias responses (including cached 404s) for every changed
// alias, so an approval or deletion is visible immediately despite the long
// alias cache TTL.
func AliasCacheInvalidator(redisClient *redis.Client) pjskalias.ChangeListener {
	if redisClient == nil {
		return nil
	}
	return func(ctx context.Context, changes []pjskalias.AliasChange) {
		InvalidateAliasCache(ctx, redisClient, changes)
	}
}

// InvalidateAliasCache deletes the hdb:pjsk:alias keys the public routes
// would read for changes.
func InvalidateAliasCache(ctx context.Context, redisClient *redis.Client, changes []pjskalias.AliasChange) {
	if redisClient == nil {
		return
	}
	for _, change := range changes {
		byID := aliasRoutePrefix + change.AliasType + "/" + strconv.Itoa(change.AliasTypeID)
		_ = harukiRedis.ClearCache(ctx, redisClient, CacheNSAlias, byID, nil)
		if change.Alias == "" {
			continue
		}
		query := url.Values{"alias": {change.Alias}}.Encode()
		_ = harukiRedis.ClearCache(ctx, redisClient, CacheNSAlias, aliasRoutePrefix+change.AliasType+"/by-alias", &query)
	}
}
