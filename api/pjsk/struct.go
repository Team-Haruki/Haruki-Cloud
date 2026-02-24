package pjsk

import (
	"haruki-cloud/database/pjsk"
	"haruki-cloud/utils/types"

	"github.com/redis/go-redis/v9"
)

// ================= Type Aliases =================

type AliasToObjectIdResponse = types.AliasToIDResponse
type AllAliasesResponse = types.AliasListResponse

// ================= Cache Namespace Constants =================

const (
	CacheNSAlias = "hdb:pjsk:alias"
)

// ================= Parameter Structs =================

type AliasParams struct {
	AliasType   string
	AliasTypeID int
	AliasStr    string
}

// ================= Service Structs =================

type AliasService struct {
	client      *pjsk.Client
	redisClient *redis.Client
}

// ================= Handler Structs =================

type AliasHandler struct {
	svc *AliasService
}
