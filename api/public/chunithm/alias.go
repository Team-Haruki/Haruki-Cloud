package chunithm

import (
	"haruki-cloud/api"
	"haruki-cloud/database/chunithm/maindb"
	"haruki-cloud/database/chunithm/maindb/chunithmmusicalias"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

func (h *AliasHandler) GetMusicIDByAlias(c fiber.Ctx) error {
	aliasStr := c.Query("alias")
	if aliasStr == "" {
		return api.JSONResponse(c, fiber.StatusBadRequest, "alias is required")
	}
	if !api.ValidateAlias(aliasStr) {
		return api.JSONResponse(c, fiber.StatusBadRequest, "invalid alias")
	}
	return api.WithCache(c, h.svc.redisClient, CacheNSAlias, func(_ string) (any, error) {
		rows, err := h.svc.client.ChunithmMusicAlias.
			Query().
			Where(chunithmmusicalias.AliasEQ(aliasStr)).
			All(c.Context())
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, &api.CacheBypassError{Response: api.JSONResponse(c, fiber.StatusNotFound, api.ErrAliasNotFound)}
		}
		ids := extractMusicIDs(rows)
		return AliasToMusicIDResponse{MatchIDs: ids}, nil
	})
}

func (h *AliasHandler) GetAliasesByMusicID(c fiber.Ctx) error {
	musicID := fiber.Params[int](c, "music_id", -1)
	if musicID <= 0 {
		return api.JSONResponse(c, fiber.StatusBadRequest, "invalid music_id")
	}
	return api.WithCache(c, h.svc.redisClient, CacheNSAlias, func(_ string) (any, error) {
		rows, err := h.svc.client.ChunithmMusicAlias.
			Query().
			Where(chunithmmusicalias.MusicIDEQ(musicID)).
			All(c.Context())
		if err != nil {
			return nil, err
		}
		aliases := extractAliasStrings(rows)
		return AllAliasesResponse{Aliases: aliases}, nil
	})
}

func registerAliasRoutes(router fiber.Router, client *maindb.Client, redisClient *redis.Client) {
	svc := NewAliasService(client, redisClient)
	h := NewAliasHandler(svc)
	r := router.Group("/alias")

	r.Get("/music-id", h.GetMusicIDByAlias)
	r.Get("/:music_id", h.GetAliasesByMusicID)
}
