package mysekai

import (
	"context"
	"sync"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

// ── Controller ──────────────────────────────────────────────────────────────

type Controller struct {
	drawing        *drawing.HarukiDrawingClient
	requestCtx     context.Context
	snapshot       snapshot.Snapshot
	rawMySekaiJSON []byte
	masterdata     masterdataSource
	resolver       *masterdataResolver
	defaultRegion  renderregion.Value
	nicknames      map[string]int
	assets         *assets.AssetHelper
}

type masterdataResolver struct {
	dsn           string
	localDir      string
	allowFallback bool

	mu    sync.Mutex
	cache map[string]masterdataSource
}

type mysekaiMapSiteConfig struct {
	SiteImageName string
	GridSize      float64
	OffsetX       float64
	OffsetZ       float64
	DirX          float64
	DirZ          float64
	RevXZ         bool
	CropBBox      []int
}

type MasterdataOptions struct {
	SekaiDSN      string
	LocalDir      string
	AllowFallback bool
}

// ── Helpers ─────────────────────────────────────────────────────────────────

type pathResolver func(relPath string) string

// ── Data source ─────────────────────────────────────────────────────────────

type masterdataSource interface {
	Configured() bool
	loadList(filename string) []map[string]any
	loadMapByID(filename string) map[int]map[string]any
	loadObject(filename string, target any) bool
}

// ── Query types ─────────────────────────────────────────────────────────────

type ResourceQuery struct {
	Region  string                      `json:"region,omitempty"`
	Profile *drawing.ProfileCardRequest `json:"-"`
}

type MapQuery struct {
	Region        string `json:"region,omitempty"`
	ShowHarvested *bool  `json:"show_harvested,omitempty"`
	MapIDs        []int  `json:"map_ids,omitempty"`
}

type FixtureListQuery struct {
	Region        string                      `json:"region,omitempty"`
	ShowID        *bool                       `json:"show_id,omitempty"`
	OnlyCraftable *bool                       `json:"only_craftable,omitempty"`
	ShowProfile   *bool                       `json:"show_profile,omitempty"`
	ShowProgress  *bool                       `json:"show_progress,omitempty"`
	ShowObtained  *bool                       `json:"show_obtained,omitempty"`
	Profile       *drawing.ProfileCardRequest `json:"-"`
}

type FixtureDetailQuery struct {
	Region string `json:"region,omitempty"`
	Query  string `json:"query"`
}

type DoorUpgradeQuery struct {
	Region  string                      `json:"region,omitempty"`
	Query   string                      `json:"query,omitempty"`
	ShowAll *bool                       `json:"show_all,omitempty"`
	Profile *drawing.ProfileCardRequest `json:"-"`
}

type MusicRecordQuery struct {
	Region  string                      `json:"region,omitempty"`
	ShowID  *bool                       `json:"show_id,omitempty"`
	Profile *drawing.ProfileCardRequest `json:"-"`
}

type TalkListQuery struct {
	Region       string                      `json:"region,omitempty"`
	Query        string                      `json:"query"`
	ShowAllTalks *bool                       `json:"show_all_talks,omitempty"`
	Profile      *drawing.ProfileCardRequest `json:"-"`
}

type PhotoQuery struct {
	Region string `json:"region,omitempty"`
	Seq    int    `json:"seq"`
}

type PhotoResult struct {
	Region     string    `json:"region"`
	Seq        int       `json:"seq"`
	Total      int       `json:"total"`
	ImagePath  string    `json:"image_path"`
	ObtainedAt time.Time `json:"obtained_at"`
}
