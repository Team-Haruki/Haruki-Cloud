package chunithm

import (
	"time"

	entchuniMain "haruki-cloud/database/chunithm/maindb"
	entchuniMusic "haruki-cloud/database/chunithm/music"

	"github.com/redis/go-redis/v9"
)

// ================= Response Types =================

type AliasToMusicIDResponse struct {
	MatchIDs []int `json:"match_ids"`
}

type AllAliasesResponse struct {
	Aliases []string `json:"aliases"`
}

type AliasRequest struct {
	Alias string `json:"alias"`
}

// ================= Music Schemas =================

type MusicInfoSchema struct {
	MusicID        int        `json:"music_id"`
	Title          string     `json:"title"`
	Artist         string     `json:"artist"`
	Category       *string    `json:"category,omitempty"`
	Version        *string    `json:"version,omitempty"`
	ReleaseDate    *time.Time `json:"release_date,omitempty"`
	IsDeleted      *bool      `json:"is_deleted,omitempty"`
	DeletedVersion *string    `json:"deleted_version,omitempty"`
}

type MusicDifficultySchema struct {
	MusicID int      `json:"music_id"`
	Version string   `json:"version"`
	Diff0   *float64 `json:"diff0_const,omitempty"`
	Diff1   *float64 `json:"diff1_const,omitempty"`
	Diff2   *float64 `json:"diff2_const,omitempty"`
	Diff3   *float64 `json:"diff3_const,omitempty"`
	Diff4   *float64 `json:"diff4_const,omitempty"`
}

type ChartDataSchema struct {
	Difficulty int      `json:"difficulty"`
	Creator    *string  `json:"creator,omitempty"`
	BPM        *float64 `json:"bpm,omitempty"`
	TapCount   *int     `json:"tap_count,omitempty"`
	HoldCount  *int     `json:"hold_count,omitempty"`
	SlideCount *int     `json:"slide_count,omitempty"`
	AirCount   *int     `json:"air_count,omitempty"`
	FlickCount *int     `json:"flick_count,omitempty"`
	TotalCount *int     `json:"total_count,omitempty"`
}

type MusicBatchItemSchema struct {
	Version    *string         `json:"version,omitempty"`
	Difficulty []*float64      `json:"difficulty"`
	Info       MusicInfoSchema `json:"info"`
}

type MusicAliasSchema struct {
	ID    int64  `json:"id,omitempty"`
	Alias string `json:"alias"`
}

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
