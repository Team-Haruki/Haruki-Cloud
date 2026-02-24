package chunithm

import (
	entchuniMain "haruki-cloud/database/chunithm/maindb"
	entchuniMusic "haruki-cloud/database/chunithm/music"
	"haruki-cloud/utils/types"

	"github.com/redis/go-redis/v9"
)

// ================= Type Aliases =================

type AliasToMusicIDResponse = types.AliasToIDResponse
type AllAliasesResponse = types.AliasListResponse
type AliasRequest = types.AliasRequest

type MusicInfoSchema = types.ChunithmMusicInfo
type MusicDifficultySchema = types.ChunithmMusicDifficulty
type ChartDataSchema = types.ChunithmChartData
type MusicBatchItemSchema = types.ChunithmMusicBatchItem

type MusicAliasSchema = types.ChunithmMusicAlias

// ================= Cache Namespace Constants =================

const (
	CacheNSAlias = "hdb:chunithm:alias"
	CacheNSMusic = "hdb:chunithm:music"
)

// ================= Service Structs =================

type AliasService struct {
	client      *entchuniMain.Client
	redisClient *redis.Client
}

type MusicService struct {
	client      *entchuniMusic.Client
	redisClient *redis.Client
}

// ================= Handler Structs =================

type AliasHandler struct {
	svc *AliasService
}

type MusicHandler struct {
	svc *MusicService
}
