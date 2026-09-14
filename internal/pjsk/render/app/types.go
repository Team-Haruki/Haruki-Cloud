package app

import (
	"context"
	"time"

	pjskDB "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/core/urlhost"
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
	"haruki-cloud/internal/storage"
	"haruki-cloud/utils/censor"
	"haruki-cloud/utils/imagecache"
)

// ── Config types ────────────────────────────────────────────────────────────

type Config struct {
	InitContext             context.Context
	DefaultRegion           renderregion.Value
	DrawingBaseURL          string
	DrawingTargets          []upstream.TargetConfig
	DrawingTimeout          time.Duration
	DrawingRetryCount       int
	DrawingCache            drawing.RenderCacheConfig
	DrawingSKMaxConcurrency int
	DrawingSKAcquireTimeout time.Duration
	DrawingMaxConcurrency   int
	// DrawingArtifact enables Drawing artifact mode; nil Objects/Hosts are
	// filled from Stores.ImageCache / ImageHosts.
	DrawingArtifact     drawing.ArtifactConfig
	ImageCacheURI       string
	ChartsBaseURL       string
	ImageCacheDir       string
	ImageCachePGURL     string // PostgreSQL DSN for image cache deduplication (optional)
	ImageCachePGMaxOpen int    // image cache index pool bound; <= 0 selects the default (8)
	// ImageCacheRenderIndexDDL runs the render index DDL at index Init.
	ImageCacheRenderIndexDDL bool
	// ImageCacheRenderIndexLookup serves render cache hits from the render
	// index (needs the index DSN); ImageCacheRenderIndexTouchInterval is its
	// per-key sliding-TTL throttle (0 = default 60s).
	ImageCacheRenderIndexLookup        bool
	ImageCacheRenderIndexTouchInterval time.Duration
	// ImageCacheLocalRoot is the absolute directory of the image_cache slot
	// when it resolved to local, "" otherwise.
	ImageCacheLocalRoot      string
	CensorService            *censor.Service
	AssetPrimaryDir          string
	AssetLegacyDirs          []string
	AssetsBaseURL            string // CDN base URL for direct asset serving; skips imagecache for region assets
	LocalMasterdata          LocalMasterdataConfig
	SekaiDBType              string
	SekaiDSN                 string // sekai DB DSN — when set, mysekai reads masterdata from DB instead of local files
	UserSnapshot             UserSnapshotConfig
	MusicMetaRefreshInterval time.Duration
	MusicMetaOutputDir       string
	// MusicMetaStore, when non-nil, replaces MusicMetaOutputDir for the
	// loader New builds when MetaLoader is nil.
	MusicMetaStore                     storage.Store
	MusicMetaSource                    string
	MusicMetaBaseURL                   string
	MetaLoader                         *meta.Loader
	SharedUpstreamResources            *upstream.SharedResources
	SKForecast                         sk.ForecastConfig
	MySekaiHousingCompetitionCachePath string
	// MySekaiHousingCompetitionCacheStore/Key name the housing stats object
	// (and hold the banner cache); nil falls back to the path above.
	MySekaiHousingCompetitionCacheStore      storage.Store
	MySekaiHousingCompetitionCacheKey        storage.Key
	MySekaiHousingCompetitionRefreshInterval time.Duration
	ReadOnly                                 bool
	DeckRecommend                            DeckRecommendConfig
	Preview3D                                costume.Preview3DConfig
	// Stores holds one storage.Store per slot, built by storage.BuildSet at
	// the composition root. Zero fields are normalised to storage.Disabled().
	Stores storage.Set
	// ImageHosts (image_cache.hosts, named by Drawing node) and AssetHosts
	// (asset_dirs.assets_base_urls, ordered) select public base URLs. Nil
	// fields derive a single-host set from ImageCacheURI / AssetsBaseURL.
	ImageHosts *urlhost.Set
	AssetHosts *urlhost.Set
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
	RegistryURL               string
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
	Stores             storage.Set
	ImageHosts         *urlhost.Set
	AssetHosts         *urlhost.Set
	AssetReader        *assets.AssetReader
	Config             Config

	// initErr records a non-fatal initialisation failure (today: the image
	// cache index) for startup to classify.
	initErr error
}

// InitError reports the initialisation failure New recorded, or nil. A
// zero-value App returns nil.
func (a *App) InitError() error {
	if a == nil {
		return nil
	}
	return a.initErr
}

// ── Masterdata types ────────────────────────────────────────────────────────

type renderMasterdataDirKind int

const (
	masterdataDirInvalid renderMasterdataDirKind = iota
	masterdataDirFlat
	masterdataDirRegionRoot
	masterdataDirRepoRoot
)
