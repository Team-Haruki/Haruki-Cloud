package pjsk

import (
	"haruki-cloud/api"
	"haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/alias"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

func (h *AliasHandler) GetGlobalAliasToID(c fiber.Ctx) error {
	params := getAliasParams(c)
	return api.WithCache(c, h.svc.redisClient, CacheNSAlias, func(_ string) (interface{}, error) {
		rows, err := h.svc.client.Alias.Query().
			Where(
				alias.AliasTypeEQ(params.AliasType),
				alias.AliasEQ(params.AliasStr),
			).
			All(c.Context())
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, &api.CacheBypassError{Response: api.JSONResponse(c, fiber.StatusNotFound, api.ErrAliasNotFound)}
		}
		ids := extractGlobalAliasTypeIDs(rows)
		return AliasToObjectIdResponse{MatchIDs: ids}, nil
	})
}

func (h *AliasHandler) GetGlobalAliasesByID(c fiber.Ctx) error {
	params := getAliasParams(c)
	return api.WithCache(c, h.svc.redisClient, CacheNSAlias, func(_ string) (interface{}, error) {
		rows, err := h.svc.client.Alias.Query().
			Where(
				alias.AliasTypeEQ(params.AliasType),
				alias.AliasTypeIDEQ(params.AliasTypeID),
			).
			All(c.Context())
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, &api.CacheBypassError{Response: api.JSONResponse(c, fiber.StatusNotFound, api.ErrAliasNotFound)}
		}
		aliases := extractGlobalAliasStrings(rows)
		return AllAliasesResponse{Aliases: aliases}, nil
	})
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
