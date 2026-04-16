package pjsk

import (
	"fmt"

	"haruki-cloud/api"
	"haruki-cloud/database/pjsk"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

// ================= Alias Type Validation =================

const (
	aliasTypeMusic     = "music"
	aliasTypeCharacter = "character"
)

// parseAliasType validates an alias_type path parameter.
func parseAliasType(t string) error {
	switch t {
	case aliasTypeMusic, aliasTypeCharacter:
		return nil
	default:
		return fmt.Errorf("invalid alias type: %s", t)
	}
}

// ================= Context Keys =================

const (
	aliasParamsKey = "alias_params"
)

// ================= Service Constructors =================

func NewAliasService(client *pjsk.Client, redisClient *redis.Client) *AliasService {
	return &AliasService{client: client, redisClient: redisClient}
}

// ================= Handler Constructors =================

func NewAliasHandler(svc *AliasService) *AliasHandler {
	return &AliasHandler{svc: svc}
}

// ================= Alias Middleware =================

func parseAliasParams(requireID bool, requireAlias bool) fiber.Handler {
	return func(c fiber.Ctx) error {
		params := AliasParams{
			AliasType: c.Params("alias_type"),
		}
		if err := parseAliasType(params.AliasType); err != nil {
			return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
		}
		if requireID {
			params.AliasTypeID = fiber.Params[int](c, "alias_type_id", -1)
			if params.AliasTypeID < 0 {
				return api.JSONResponse(c, fiber.StatusBadRequest, "invalid alias_type_id")
			}
		}
		if requireAlias {
			params.AliasStr = c.Query("alias")
			if !api.ValidateAlias(params.AliasStr) {
				return api.JSONResponse(c, fiber.StatusBadRequest, "invalid alias")
			}
		}
		c.Locals(aliasParamsKey, &params)
		return c.Next()
	}
}

// ================= Context Getters =================

func getAliasParams(c fiber.Ctx) *AliasParams {
	if p, ok := c.Locals(aliasParamsKey).(*AliasParams); ok {
		return p
	}
	return nil
}

// ================= Extract Helpers =================

func extractGlobalAliasTypeIDs(rows []*pjsk.Alias) []int {
	ids := make([]int, len(rows))
	for i, r := range rows {
		ids[i] = r.AliasTypeID
	}
	return ids
}

func extractGlobalAliasStrings(rows []*pjsk.Alias) []string {
	aliases := make([]string, len(rows))
	for i, r := range rows {
		aliases[i] = r.Alias
	}
	return aliases
}
