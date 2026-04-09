package chunithm

import (
	"haruki-cloud/api"
	"haruki-cloud/config"
	"haruki-cloud/database/chunithm/maindb"
	"haruki-cloud/database/chunithm/maindb/chunithmmusicalias"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

func (h *AliasHandler) GetMusicIDByAlias(c fiber.Ctx) error {
	ctx := c.Context()
	aliasStr := c.Query("alias")
	if aliasStr == "" {
		return api.JSONResponse(c, fiber.StatusBadRequest, "alias is required")
	}
	if !api.ValidateAlias(aliasStr) {
		return api.JSONResponse(c, fiber.StatusBadRequest, "invalid alias")
	}
	key, cached, hit, err := api.CacheQuery(ctx, c, h.svc.redisClient, CacheNSAlias)
	if err != nil {
		return api.InternalError(c)
	}
	if hit {
		return c.Status(fiber.StatusOK).JSON(cached)
	}
	rows, err := h.svc.client.ChunithmMusicAlias.
		Query().
		Where(chunithmmusicalias.AliasEQ(aliasStr)).
		All(ctx)
	if err != nil {
		return api.InternalError(c)
	}
	if len(rows) == 0 {
		return api.JSONResponse(c, fiber.StatusNotFound, api.ErrAliasNotFound)
	}
	ids := extractMusicIDs(rows)
	return api.CachedJSONResponse(ctx, c, h.svc.redisClient, config.Cfg.Backend.APICacheTTL, key, fiber.StatusOK, "ok", AliasToMusicIDResponse{MatchIDs: ids})
}

func (h *AliasHandler) GetAliasesByMusicID(c fiber.Ctx) error {
	ctx := c.Context()
	musicID := fiber.Params[int](c, "music_id", -1)
	if musicID <= 0 {
		return api.JSONResponse(c, fiber.StatusBadRequest, "invalid music_id")
	}
	key, cached, hit, err := api.CacheQuery(ctx, c, h.svc.redisClient, CacheNSAlias)
	if err != nil {
		return api.InternalError(c)
	}
	if hit {
		return c.Status(fiber.StatusOK).JSON(cached)
	}
	rows, err := h.svc.client.ChunithmMusicAlias.
		Query().
		Where(chunithmmusicalias.MusicIDEQ(musicID)).
		All(ctx)
	if err != nil {
		return api.InternalError(c)
	}
	aliases := extractAliasStrings(rows)
	return api.CachedJSONResponse(ctx, c, h.svc.redisClient, config.Cfg.Backend.APICacheTTL, key, fiber.StatusOK, "ok", AllAliasesResponse{Aliases: aliases})
}

func registerAliasRoutes(router fiber.Router, client *maindb.Client, redisClient *redis.Client) {
	svc := NewAliasService(client, redisClient)
	h := NewAliasHandler(svc)
	r := router.Group("/alias")

	r.Get("/music-id", h.GetMusicIDByAlias)
	r.Get("/:music_id", h.GetAliasesByMusicID)
}
