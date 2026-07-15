package app

import (
	"context"
	"time"

	pjskDB "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/pjsk/accountdata"
	pjskalias "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/meta"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/card"
	"haruki-cloud/internal/pjsk/render/costume"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/education"
	"haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/gacha"
	"haruki-cloud/internal/pjsk/render/honor"
	"haruki-cloud/internal/pjsk/render/inventory"
	"haruki-cloud/internal/pjsk/render/misc"
	"haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/internal/pjsk/render/mysekai"
	"haruki-cloud/internal/pjsk/render/profile"
	"haruki-cloud/internal/pjsk/render/provider"
	"haruki-cloud/internal/pjsk/render/score"
	"haruki-cloud/internal/pjsk/render/sk"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/pjsk/render/stamp"
	"haruki-cloud/internal/pjsk/render/vlive"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/censor"
	"haruki-cloud/utils/imagecache"
)

// ── Config types ────────────────────────────────────────────────────────────

type Config struct {
	InitContext                              context.Context
	DefaultRegion                            renderregion.Value
	DrawingBaseURL                           string
	DrawingTargets                           []upstream.TargetConfig
	DrawingTimeout                           time.Duration
	DrawingRetryCount                        int
	DrawingCache                             drawing.RenderCacheConfig
	DrawingSKMaxConcurrency                  int
	DrawingSKAcquireTimeout                  time.Duration
	DrawingMaxConcurrency                    int
	ImageCacheURI                            string
	ChartsBaseURL                            string
	ImageCacheDir                            string
	ImageCachePGURL                          string // PostgreSQL DSN for image cache deduplication (optional)
	CensorService                            *censor.Service
	AssetPrimaryDir                          string
	AssetLegacyDirs                          []string
	AssetsBaseURL                            string // CDN base URL for direct asset serving; skips imagecache for region assets
	LocalMasterdata                          LocalMasterdataConfig
	SekaiDBType                              string
	SekaiDSN                                 string // sekai DB DSN — when set, mysekai reads masterdata from DB instead of local files
	UserSnapshot                             UserSnapshotConfig
	MusicMetaRefreshInterval                 time.Duration
	MusicMetaOutputDir                       string
	MetaLoader                               *meta.Loader
	SharedUpstreamResources                  *upstream.SharedResources
	SKForecast                               sk.ForecastConfig
	MySekaiHousingCompetitionCachePath       string
	MySekaiHousingCompetitionRefreshInterval time.Duration
	ReadOnly                                 bool
	DeckRecommend                            DeckRecommendConfig
	Preview3D                                costume.Preview3DConfig
	// Upstream HTTP clients. Caller constructs these from its own config
	// (see cmd/server) and passes them here so the render runtime does not
	// depend on package-level singletons.
	SekaiAPI *sekaiapi.HarukiSekaiAPIClient
	Toolbox  *sekaiapi.HarukiToolboxClient
	Tracker  *sekaiapi.TrackerClient
}

const defaultMusicMetaRefreshInterval = 30 * time.Minute

type LocalMasterdataConfig struct {
	Enabled         bool
	AllowFallback   bool // when false, DB failure is fatal; when true, fallback to local files
	AllowLeaks      bool // when true, unopened event/worldbloom deck queries may fall back to local masterdata
	Dir             string
	RefreshInterval time.Duration
}

type UserSnapshotConfig struct {
	Provider      string
	AllowFallback bool // when false, Toolbox failure is fatal; when true, fallback to local snapshot
	UserJSON      string
	MusicMetaJSON string
	MySekaiJSON   string
}

type DeckRecommendConfig struct {
	Enabled                   bool
	Disable                   bool
	DisableReason             string
	ServiceBaseURL            string
	Targets                   []upstream.TargetConfig
	SharedResources           *upstream.SharedResources
	MasterdataDir             string
	MasterdataRefreshInterval time.Duration
	Timeout                   time.Duration
	MaxRetries                int
	RetryWaitTime             time.Duration
	DefaultAlgs               []string
}

type SKForecastConfig = sk.ForecastConfig

// ── App composite root ──────────────────────────────────────────────────────

type App struct {
	Sekai              *sekaiDB.Client
	PJSK               *pjskDB.Client
	Drawing            *drawing.HarukiDrawingClient
	Assets             *assets.AssetHelper
	MetaLoader         *meta.Loader
	Provider           provider.MasterDataProvider
	Providers          map[renderregion.Value]provider.MasterDataProvider
	Cards              *card.Controller
	Costumes           *costume.Controller
	Decks              *deck.Controller
	Edu                *education.Controller
	Events             *event.Controller
	Gachas             *gacha.Controller
	Honors             *honor.Controller
	Inventory          *inventory.Controller
	Misc               *misc.Controller
	MySekai            *mysekai.Controller
	Music              *music.Controller
	Aliases            *pjskalias.Service
	Profiles           *profile.Controller
	Score              *score.Controller
	SK                 *sk.Controller
	Stamps             *stamp.Controller
	VLive              *vlive.Controller
	Bindings           *accountdata.BindingService
	BanChecker         *accountdata.BanService
	Snapshots          snapshot.HarukiSnapshotProvider
	MySekaiPayloads    snapshot.MySekaiPayloadProvider
	PrivateDataCache   *snapshot.PrivateDataCache
	BuiltSnapshotCache *snapshot.BuiltSnapshotCache
	ImageCache         *imagecache.Client
	Censor             *censor.Service
	SekaiAPI           *sekaiapi.HarukiSekaiAPIClient
	Toolbox            *sekaiapi.HarukiToolboxClient
	Tracker            *sekaiapi.TrackerClient
	Config             Config
}

// ── Masterdata types ────────────────────────────────────────────────────────

type renderMasterdataDirKind int

const (
	masterdataDirInvalid renderMasterdataDirKind = iota
	masterdataDirFlat
	masterdataDirRegionRoot
	masterdataDirRepoRoot
)
