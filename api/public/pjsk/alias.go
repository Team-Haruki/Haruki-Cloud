package pjsk

import (
	"haruki-cloud/api"
	"haruki-cloud/config"
	"haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/alias"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

func (h *AliasHandler) GetGlobalAliasToID(c fiber.Ctx) error {
	ctx := c.Context()
	params := getAliasParams(c)
	key, cached, hit, err := api.CacheQuery(ctx, c, h.svc.redisClient, CacheNSAlias)
	if err != nil {
		return api.InternalError(c)
	}
	if hit {
		return c.Status(fiber.StatusOK).JSON(cached)
	}
	rows, err := h.svc.client.Alias.Query().
		Where(
			alias.AliasTypeEQ(params.AliasType),
			alias.AliasEQ(params.AliasStr),
		).
		All(ctx)
	if err != nil {
		return api.InternalError(c)
	}
	if len(rows) == 0 {
		return api.JSONResponse(c, fiber.StatusNotFound, api.ErrAliasNotFound)
	}
	ids := extractGlobalAliasTypeIDs(rows)
	return api.CachedJSONResponse(ctx, c, h.svc.redisClient, config.Cfg.Backend.APICacheTTL, key, fiber.StatusOK, "ok", AliasToObjectIdResponse{MatchIDs: ids})
}

func (h *AliasHandler) GetGlobalAliasesByID(c fiber.Ctx) error {
	ctx := c.Context()
	params := getAliasParams(c)
	key, cached, hit, err := api.CacheQuery(ctx, c, h.svc.redisClient, CacheNSAlias)
	if err != nil {
		return api.InternalError(c)
	}
	if hit {
		return c.Status(fiber.StatusOK).JSON(cached)
	}
	rows, err := h.svc.client.Alias.Query().
		Where(
			alias.AliasTypeEQ(params.AliasType),
			alias.AliasTypeIDEQ(params.AliasTypeID),
		).
		All(ctx)
	if err != nil {
		return api.InternalError(c)
	}
	if len(rows) == 0 {
		return api.JSONResponse(c, fiber.StatusNotFound, api.ErrAliasNotFound)
	}
	aliases := extractGlobalAliasStrings(rows)
	return api.CachedJSONResponse(ctx, c, h.svc.redisClient, config.Cfg.Backend.APICacheTTL, key, fiber.StatusOK, "ok", AllAliasesResponse{Aliases: aliases})
}

func registerAliasRoutes(router fiber.Router, client *pjsk.Client, redisClient *redis.Client) {
	svc := NewAliasService(client, redisClient)
	h := NewAliasHandler(svc)
	r := router.Group("/alias")

	// Group alias and all alias mutation/audit operations are intentionally moved out of public API.
	r.Get("/:alias_type/by-alias",
		parseAliasParams(false, true),
		h.GetGlobalAliasToID)
	r.Get("/:alias_type/:alias_type_id",
		parseAliasParams(true, false),
		h.GetGlobalAliasesByID)
}
