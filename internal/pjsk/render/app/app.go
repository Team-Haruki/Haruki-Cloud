package app

import (
	"context"
	"strings"
	"time"

	pjskDB "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	pjskalias "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/meta"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/card"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/education"
	"haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/gacha"
	"haruki-cloud/internal/pjsk/render/honor"
	"haruki-cloud/internal/pjsk/render/misc"
	"haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/internal/pjsk/render/mysekai"
	"haruki-cloud/internal/pjsk/render/profile"
	"haruki-cloud/internal/pjsk/render/provider"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/score"
	"haruki-cloud/internal/pjsk/render/sk"
	"haruki-cloud/internal/pjsk/render/stamp"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/internal/pjsk/render/vlive"
	accountdata "haruki-cloud/internal/pjsk/userdata"
	"haruki-cloud/utils/censor"
	"haruki-cloud/utils/drawing"
	"haruki-cloud/utils/imagecache"
	sekaiutil "haruki-cloud/utils/sekai"
)

type Config struct {
	DefaultRegion     renderregion.Value
	DrawingBaseURL    string
	DrawingTimeout    time.Duration
	DrawingRetryCount int
	DrawingCache      drawing.RenderCacheConfig
	ImageCacheURI     string
	ImageCacheDir     string
	ImageCachePGURL   string // PostgreSQL DSN for image cache deduplication (optional)
	CensorService     *censor.Service
	AssetPrimaryDir   string
	AssetLegacyDirs   []string
	AssetsBaseURL     string // CDN base URL for direct asset serving; skips imagecache for region assets
	LocalMasterdata   LocalMasterdataConfig
	SekaiDSN          string // sekai DB DSN — when set, mysekai reads masterdata from DB instead of local files
	UserSnapshot      UserSnapshotConfig
	MetaLoader        *meta.Loader
	DeckRecommend     DeckRecommendConfig
}

type LocalMasterdataConfig struct {
	Enabled bool
	Dir     string
}

type UserSnapshotConfig struct {
	Provider      string
	UserJSON      string
	MusicMetaJSON string
	MySekaiJSON   string
}

type DeckRecommendConfig struct {
	Enabled          bool
	UseLocalEngine   bool
	ServiceBaseURL   string
	LocalPoolSize    int
	LocalLibraryDirs []string
	StaticDataDir    string
	MasterdataDir    string
	Timeout          time.Duration
	DefaultAlgs      []string
}

type App struct {
	Sekai      *sekaiDB.Client
	PJSK       *pjskDB.Client
	Drawing    *drawing.HarukiDrawingClient
	Assets     *assets.AssetHelper
	MetaLoader *meta.Loader
	Provider   provider.MasterDataProvider
	Cards      *card.Controller
	Decks      *deck.Controller
	Edu        *education.Controller
	Events     *event.Controller
	Gachas     *gacha.Controller
	Honors     *honor.Controller
	Misc       *misc.Controller
	MySekai    *mysekai.Controller
	Music      *music.Controller
	Aliases    *pjskalias.Service
	Profiles   *profile.Controller
	Score      *score.Controller
	SK         *sk.Controller
	Stamps     *stamp.Controller
	VLive      *vlive.Controller
	Bindings   *accountdata.BindingService
	BanChecker *accountdata.BanService
	ImageCache *imagecache.Client
	Censor     *censor.Service
	Config     Config
}

func New(sekaiClient *sekaiDB.Client, pjskClient *pjskDB.Client, cfg Config) *App {
	cfg.DefaultRegion = renderregion.WithDefault(cfg.DefaultRegion)

	assetHelper := assets.NewAssetHelper(cfg.AssetPrimaryDir, cfg.AssetLegacyDirs)
	var snapshotService *userdata.Service
	if provider := strings.TrimSpace(cfg.UserSnapshot.Provider); provider == "" || strings.EqualFold(provider, "local_file") {
		snapshotService = userdata.NewLocalFileService(sekaiClient, assetHelper, userdata.LocalFileConfig{
			DefaultRegion: cfg.DefaultRegion,
			UserJSON:      cfg.UserSnapshot.UserJSON,
			MusicMetaJSON: cfg.UserSnapshot.MusicMetaJSON,
			MySekaiJSON:   cfg.UserSnapshot.MySekaiJSON,
		})
	}

	var drawingClient *drawing.HarukiDrawingClient
	if strings.TrimSpace(cfg.DrawingBaseURL) != "" {
		var options []drawing.ClientOption
		if cfg.DrawingTimeout > 0 {
			options = append(options, drawing.WithTimeout(cfg.DrawingTimeout))
		}
		if cfg.DrawingRetryCount > 0 {
			options = append(options, drawing.WithRetryCount(cfg.DrawingRetryCount))
		}
		drawingClient = drawing.NewHarukiDrawingClient(cfg.DrawingBaseURL, options...)
		drawingClient.SetRenderCache(drawing.NewRenderCacheClient(cfg.DrawingCache))
	}

	miscController := misc.NewController(drawingClient)
	mysekaiController := mysekai.NewController(drawingClient, snapshotService, cfg.LocalMasterdata.Dir, cfg.DefaultRegion, assetHelper, cfg.SekaiDSN)
	musicController := (*music.Controller)(nil)
	deckController := deck.NewControllerWithConfig(nil, nil, drawingClient, assetHelper, snapshotService, cfg.DefaultRegion, deck.RecommendConfig{
		Enabled:          cfg.DeckRecommend.Enabled,
		UseLocalEngine:   cfg.DeckRecommend.UseLocalEngine,
		ServiceBaseURL:   cfg.DeckRecommend.ServiceBaseURL,
		LocalPoolSize:    cfg.DeckRecommend.LocalPoolSize,
		LocalLibraryDirs: append([]string(nil), cfg.DeckRecommend.LocalLibraryDirs...),
		StaticDataDir:    cfg.DeckRecommend.StaticDataDir,
		MasterdataDir:    cfg.DeckRecommend.MasterdataDir,
		Timeout:          cfg.DeckRecommend.Timeout,
		DefaultAlgs:      append([]string(nil), cfg.DeckRecommend.DefaultAlgs...),
	}, cfg.MetaLoader)
	educationController := education.NewController(drawingClient, assetHelper, snapshotService, cfg.DefaultRegion)
	scoreController := score.NewController(drawingClient)
	skController := sk.NewController(drawingClient)
	skController.SetTrackerIntegration(sekaiutil.GetTrackerClient(), nil, assetHelper)

	var cardController *card.Controller
	var eventController *event.Controller
	var gachaController *gacha.Controller
	var honorController *honor.Controller
	var profileController *profile.Controller
	var stampController *stamp.Controller
	var vliveController *vlive.Controller
	var masterProvider provider.MasterDataProvider
	if sekaiClient != nil {
		masterProvider = provider.NewDatabaseProvider(sekaiClient, cfg.DefaultRegion)

		// Create module adapters from the unified provider
		cardAdapter := card.NewProviderAdapter(masterProvider)
		eventAdapter := event.NewProviderAdapter(masterProvider)
		musicAdapter := music.NewProviderAdapter(masterProvider)
		gachaAdapter := gacha.NewProviderAdapter(masterProvider)
		honorAdapter := honor.NewProviderAdapter(masterProvider)
		stampAdapter := stamp.NewProviderAdapter(masterProvider)
		vliveAdapter := vlive.NewProviderAdapter(masterProvider)
		profileAdapter := profile.NewProviderAdapter(masterProvider)
		educationAdapter := education.NewProviderAdapter(masterProvider)

		// SK event source for default region + multi-region registration
		skController.SetTrackerIntegration(sekaiutil.GetTrackerClient(), eventAdapter, assetHelper)
		for _, region := range []renderregion.Value{
			renderregion.JP,
			renderregion.CN,
			renderregion.TW,
			renderregion.KR,
			renderregion.EN,
		} {
			if renderregion.WithDefault(region) == renderregion.WithDefault(cfg.DefaultRegion) {
				continue
			}
			skController.RegisterEventSource(event.NewCloudSource(sekaiClient, region))
		}

		// Initialize controllers with provider adapters
		deckController = deck.NewControllerWithConfig(cardAdapter, eventAdapter, drawingClient, assetHelper, snapshotService, cfg.DefaultRegion, deck.RecommendConfig{
			Enabled:          cfg.DeckRecommend.Enabled,
			UseLocalEngine:   cfg.DeckRecommend.UseLocalEngine,
			ServiceBaseURL:   cfg.DeckRecommend.ServiceBaseURL,
			LocalPoolSize:    cfg.DeckRecommend.LocalPoolSize,
			LocalLibraryDirs: append([]string(nil), cfg.DeckRecommend.LocalLibraryDirs...),
			StaticDataDir:    cfg.DeckRecommend.StaticDataDir,
			MasterdataDir:    cfg.DeckRecommend.MasterdataDir,
			Timeout:          cfg.DeckRecommend.Timeout,
			DefaultAlgs:      append([]string(nil), cfg.DeckRecommend.DefaultAlgs...),
		}, cfg.MetaLoader)
		cardController = card.NewController(cardAdapter, eventAdapter, drawingClient, assetHelper)
		educationController.RegisterSource(educationAdapter)
		eventController = event.NewController(eventAdapter, drawingClient, assetHelper)
		gachaController = gacha.NewController(gachaAdapter, drawingClient, assetHelper)
		honorController = honor.NewController(honorAdapter, drawingClient, assetHelper)
		musicController = music.NewController(musicAdapter, drawingClient, assetHelper, snapshotService, cfg.MetaLoader)
		profileController = profile.NewController(profileAdapter, drawingClient, assetHelper, snapshotService)
		stampController = stamp.NewController(stampAdapter, drawingClient, assetHelper)
		vliveController = vlive.NewController(vliveAdapter, cfg.DefaultRegion)
	}

	aliasService := pjskalias.NewService(sekaiClient, pjskClient, nil)
	if musicController != nil {
		musicController.SetAliasResolver(aliasService)
	}

	var imgStore *imagecache.PGStore
	if cfg.ImageCachePGURL != "" {
		if store, err := imagecache.NewPGStore(cfg.ImageCachePGURL); err == nil {
			_ = store.Init(context.Background())
			imgStore = store
		}
	}

	if cfg.CensorService != nil {
		skController.SetCensor(cfg.CensorService)
		if profileController != nil {
			profileController.SetCensor(cfg.CensorService)
		}
	}

	return &App{
		Sekai:      sekaiClient,
		PJSK:       pjskClient,
		Drawing:    drawingClient,
		Assets:     assetHelper,
		MetaLoader: cfg.MetaLoader,
		Provider:   masterProvider,
		Cards:      cardController,
		Decks:      deckController,
		Edu:        educationController,
		Events:     eventController,
		Gachas:     gachaController,
		Honors:     honorController,
		Misc:       miscController,
		MySekai:    mysekaiController,
		Music:      musicController,
		Aliases:    aliasService,
		Profiles:   profileController,
		Score:      scoreController,
		SK:         skController,
		Stamps:     stampController,
		VLive:      vliveController,
		ImageCache: imagecache.NewWithStore(cfg.ImageCacheURI, cfg.ImageCacheDir, imgStore),
		Censor:     cfg.CensorService,
		Config:     cfg,
	}
}

func (a *App) AssetRoots() []string {
	if a == nil || a.Assets == nil {
		return nil
	}
	return a.Assets.Roots()
}
