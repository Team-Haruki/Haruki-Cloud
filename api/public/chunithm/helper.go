package chunithm

import (
	"context"
	"fmt"
	entchuniMain "haruki-cloud/database/chunithm/maindb"
	entchuniMusic "haruki-cloud/database/chunithm/music"
	harukiRedis "haruki-cloud/utils/redis"

	"github.com/redis/go-redis/v9"
)

// ================= Service Constructors =================

func NewAliasService(client *entchuniMain.Client, redisClient *redis.Client) *AliasService {
	return &AliasService{client: client, redisClient: redisClient}
}

func NewMusicService(client *entchuniMusic.Client, redisClient *redis.Client) *MusicService {
	return &MusicService{client: client, redisClient: redisClient}
}

// ================= Handler Constructors =================

func NewAliasHandler(svc *AliasService) *AliasHandler {
	return &AliasHandler{svc: svc}
}

func NewMusicHandler(svc *MusicService) *MusicHandler {
	return &MusicHandler{svc: svc}
}

// ================= AliasService Methods =================

func (s *AliasService) ClearCache(ctx context.Context, musicID int, alias string) {
	_ = harukiRedis.ClearCache(ctx, s.redisClient, CacheNSAlias, fmt.Sprintf("/chunithm/alias/%d", musicID), nil)
	cacheKey := fmt.Sprintf("alias=%s", alias)
	_ = harukiRedis.ClearCache(ctx, s.redisClient, CacheNSAlias, "/chunithm/alias/music-id", &cacheKey)
}

// ================= Extract Helpers =================

func extractMusicIDs(rows []*entchuniMain.ChunithmMusicAlias) []int {
	ids := make([]int, len(rows))
	for i, r := range rows {
		ids[i] = r.MusicID
	}
	return ids
}

func extractAliasStrings(rows []*entchuniMain.ChunithmMusicAlias) []string {
	aliases := make([]string, len(rows))
	for i, r := range rows {
		aliases[i] = r.Alias
	}
	return aliases
}
