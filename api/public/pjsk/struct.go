package pjsk

import (
	"haruki-cloud/database/pjsk"

	"github.com/redis/go-redis/v9"
)

// ================= Response Types =================

type AliasToObjectIdResponse struct {
	MatchIDs []int `json:"match_ids"`
}

type AllAliasesResponse struct {
	Aliases []string `json:"aliases"`
}

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
