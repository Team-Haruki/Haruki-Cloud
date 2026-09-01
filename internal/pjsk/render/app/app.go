package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	pjskDB "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/core/upstream"
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
	"haruki-cloud/utils/imagecache"
	"haruki-cloud/utils/logger"
)

func New(sekaiClient *sekaiDB.Client, pjskClient *pjskDB.Client, cfg Config) *App {
	initCtx := normalizeAppConfig(&cfg)
	dependencies := newAppDependencies(initCtx, sekaiClient, cfg)
	assetHelper := dependencies.assets
	snapshotService := dependencies.snapshots
	staticSnapshotProvider := dependencies.staticSnapshots
	drawingClient := dependencies.drawing
	imgStore := dependencies.imageStore
	miscController := misc.NewController(drawingClient)
	localMasterdataFallback := dependencies.localMasterdataFallback
	localMasterdataDir := dependencies.localMasterdataDir
	inventoryMasterdataDir := dependencies.inventoryMasterdataDir
	mysekaiController := mysekai.NewController(drawingClient, snapshotService, cfg.DefaultRegion, assetHelper, mysekai.MasterdataOptions{
		SekaiDSN:                          cfg.SekaiDSN,
		LocalDir:                          localMasterdataDir,
		AllowFallback:                     localMasterdataFallback && cfg.LocalMasterdata.AllowFallback,
		AssetsBaseURL:                     cfg.AssetsBaseURL,
		HousingCompetitionStatsCachePath:  cfg.MySekaiHousingCompetitionCachePath,
		HousingCompetitionRefreshInterval: cfg.MySekaiHousingCompetitionRefreshInterval,
	})
	inventoryController := inventory.NewController(drawingClient, assetHelper, snapshotService, cfg.DefaultRegion, inventory.MasterdataOptions{
		LocalDir: inventoryMasterdataDir,
	})
	deckController := newAppDeckController(nil, nil, drawingClient, assetHelper, snapshotService, cfg)
	educationController := education.NewController(drawingClient, assetHelper, snapshotService, cfg.DefaultRegion)
	scoreController := score.NewController(drawingClient)
	skController := sk.NewControllerWithConfig(drawingClient, cfg.SKForecast)
	skController.SetTrackerIntegration(cfg.Tracker, nil, assetHelper)

	databaseControllers := configureAppDatabaseControllers(
		sekaiClient, cfg, localMasterdataFallback, localMasterdataDir, drawingClient, assetHelper,
		snapshotService, deckController, educationController, skController,
	)
	deckController = databaseControllers.decks
	musicController := databaseControllers.music
	cardController := databaseControllers.cards
	costumeController := databaseControllers.costumes
	eventController := databaseControllers.events
	gachaController := databaseControllers.gachas
	honorController := databaseControllers.honors
	profileController := databaseControllers.profiles
	stampController := databaseControllers.stamps
	vliveController := databaseControllers.virtual
	masterProvider := databaseControllers.provider
	providersByRegion := databaseControllers.providers

	aliasService := pjskalias.NewService(sekaiClient, pjskClient, nil)
	if aliasService != nil {
		aliasService.SetReadOnly(cfg.ReadOnly)
	}
	if musicController != nil {
		musicController.SetAliasResolver(aliasService)
	}
	if cardController != nil && aliasService != nil {
		if nicknames, err := aliasService.ListApprovedCharacterAliasMap(initCtx); err != nil {
			logger.WarnContext(initCtx, "approved character aliases load failed",
				"consumer", "card_controller",
				"error_type", fmt.Sprintf("%T", err),
			)
		} else {
			cardController.MergeNicknames(nicknames)
		}
	}

	if cfg.CensorService != nil {
		skController.SetCensor(cfg.CensorService)
		if profileController != nil {
			profileController.SetCensor(cfg.CensorService)
		}
	}

	skController.StartDefaultPredictWarmup()

	runtime := &App{
		Sekai:      sekaiClient,
		PJSK:       pjskClient,
		Drawing:    drawingClient,
		Assets:     assetHelper,
		MetaLoader: cfg.MetaLoader,
		Provider:   masterProvider,
		Providers:  providersByRegion,
		Cards:      cardController,
		Costumes:   costumeController,
		Decks:      deckController,
		Edu:        educationController,
		Events:     eventController,
		Gachas:     gachaController,
		Honors:     honorController,
		Inventory:  inventoryController,
		Misc:       miscController,
		MySekai:    mysekaiController,
		Music:      musicController,
		Aliases:    aliasService,
		Profiles:   profileController,
		Score:      scoreController,
		SK:         skController,
		Stamps:     stampController,
		VLive:      vliveController,
		Snapshots:  staticSnapshotProvider,
		ImageCache: imagecache.NewWithStore(cfg.ImageCacheURI, cfg.ImageCacheDir, imgStore),
		Censor:     cfg.CensorService,
		SekaiAPI:   cfg.SekaiAPI,
		Toolbox:    cfg.Toolbox,
		Tracker:    cfg.Tracker,
		Config:     cfg,
	}
	if localMasterdataFallback {
		runtime.startLocalMasterdataRefresh(initCtx, localMasterdataDir, cfg.LocalMasterdata.RefreshInterval)
	}
	mysekaiController.StartHousingCompetitionStatsRefresh(initCtx, cfg.SekaiAPI, cfg.DefaultRegion.String())
	return runtime
}

type appDependencies struct {
	assets                  *assets.AssetHelper
	snapshots               snapshot.Snapshot
	staticSnapshots         snapshot.HarukiSnapshotProvider
	drawing                 *drawing.HarukiDrawingClient
	imageStore              *imagecache.PGStore
	localMasterdataFallback bool
	localMasterdataDir      string
	inventoryMasterdataDir  string
}

type appDatabaseControllers struct {
	cards     *card.Controller
	costumes  *costume.Controller
	decks     *deck.Controller
	events    *event.Controller
	gachas    *gacha.Controller
	honors    *honor.Controller
	music     *music.Controller
	profiles  *profile.Controller
	stamps    *stamp.Controller
	virtual   *vlive.Controller
	provider  provider.MasterDataProvider
	providers map[renderregion.Value]provider.MasterDataProvider
}

func configureAppDatabaseControllers(sekaiClient *sekaiDB.Client, cfg Config, localFallback bool, localDir string, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshotService snapshot.Snapshot, decks *deck.Controller, educationController *education.Controller, skController *sk.Controller) appDatabaseControllers {
	controllers := appDatabaseControllers{decks: decks, providers: make(map[renderregion.Value]provider.MasterDataProvider)}
	if sekaiClient == nil {
		return controllers
	}
	controllers.configureDefaultProvider(sekaiClient, cfg, localFallback, localDir, drawingClient, assetHelper, snapshotService, educationController, skController)
	controllers.configureRegionProviders(sekaiClient, cfg, localFallback, localDir, educationController, skController)
	return controllers
}

func (c *appDatabaseControllers) registerProvider(source provider.MasterDataProvider) {
	if source == nil {
		return
	}
	region := renderregion.WithDefault(source.Region())
	c.providers[region] = source
	if c.provider == nil {
		c.provider = source
	}
}

func (c *appDatabaseControllers) configureDefaultProvider(sekaiClient *sekaiDB.Client, cfg Config, localFallback bool, localDir string, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshotService snapshot.Snapshot, educationController *education.Controller, skController *sk.Controller) {
	databaseProvider := provider.NewDatabaseProvider(sekaiClient, cfg.DefaultRegion, provider.WithSekaiDatabase(cfg.SekaiDBType, cfg.SekaiDSN))
	if localFallback {
		databaseProvider.SetLocalMasterdataDir(localDir, cfg.LocalMasterdata.AllowLeaks)
	}
	c.registerProvider(databaseProvider)
	cardAdapter := card.NewProviderAdapter(c.provider)
	costumeAdapter := costume.NewProviderAdapter(c.provider)
	eventAdapter := event.NewProviderAdapter(c.provider)
	musicAdapter := music.NewProviderAdapter(c.provider)
	skController.SetTrackerIntegration(cfg.Tracker, eventAdapter, assetHelper)
	c.decks = newAppDeckController(cardAdapter, eventAdapter, drawingClient, assetHelper, snapshotService, cfg)
	c.decks.RegisterMusicSource(musicAdapter)
	c.cards = card.NewController(cardAdapter, eventAdapter, drawingClient, assetHelper)
	c.costumes = costume.NewController(costumeAdapter, drawingClient, assetHelper)
	c.costumes.Set3DPreviewConfig(cfg.Preview3D)
	educationController.RegisterSource(education.NewProviderAdapter(c.provider))
	c.events = event.NewController(eventAdapter, drawingClient, assetHelper)
	c.gachas = gacha.NewController(gacha.NewProviderAdapter(c.provider), drawingClient, assetHelper)
	c.honors = honor.NewController(honor.NewProviderAdapter(c.provider), drawingClient, assetHelper)
	c.music = music.NewController(musicAdapter, drawingClient, assetHelper, snapshotService, cfg.MetaLoader)
	c.music.SetCustomMusicScoreClient(cfg.SekaiAPI)
	c.profiles = profile.NewController(profile.NewProviderAdapter(c.provider), drawingClient, assetHelper, snapshotService)
	c.stamps = stamp.NewController(stamp.NewProviderAdapter(c.provider), drawingClient, assetHelper)
	c.virtual = vlive.NewControllerWithDrawing(vlive.NewProviderAdapter(c.provider), drawingClient, assetHelper, cfg.DefaultRegion)
}

func (c *appDatabaseControllers) configureRegionProviders(sekaiClient *sekaiDB.Client, cfg Config, localFallback bool, localDir string, educationController *education.Controller, skController *sk.Controller) {
	regions := []renderregion.Value{renderregion.JP, renderregion.CN, renderregion.TW, renderregion.KR, renderregion.EN}
	for _, region := range regions {
		if renderregion.WithDefault(region) == renderregion.WithDefault(cfg.DefaultRegion) {
			continue
		}
		c.configureRegionProvider(sekaiClient, region, cfg, localFallback, localDir, educationController, skController)
	}
}

func (c *appDatabaseControllers) configureRegionProvider(sekaiClient *sekaiDB.Client, region renderregion.Value, cfg Config, localFallback bool, localDir string, educationController *education.Controller, skController *sk.Controller) {
	regionProvider := provider.NewDatabaseProvider(sekaiClient, region, provider.WithSekaiDatabase(cfg.SekaiDBType, cfg.SekaiDSN))
	if localFallback {
		regionProvider.SetLocalMasterdataDir(localDir, cfg.LocalMasterdata.AllowLeaks)
	}
	c.registerProvider(regionProvider)
	cardAdapter := card.NewProviderAdapter(regionProvider)
	eventAdapter := event.NewProviderAdapter(regionProvider)
	c.cards.RegisterSource(cardAdapter)
	c.cards.RegisterEventSource(eventAdapter)
	c.costumes.RegisterSource(costume.NewProviderAdapter(regionProvider))
	c.decks.RegisterCardSource(cardAdapter)
	c.decks.RegisterEventSource(eventAdapter)
	c.decks.RegisterMusicSource(music.NewProviderAdapter(regionProvider))
	educationController.RegisterSource(education.NewProviderAdapter(regionProvider))
	c.events.RegisterSource(eventAdapter)
	c.gachas.RegisterSource(gacha.NewProviderAdapter(regionProvider))
	c.honors.RegisterSource(honor.NewProviderAdapter(regionProvider))
	c.music.RegisterSource(music.NewProviderAdapter(regionProvider))
	c.profiles.RegisterSource(profile.NewProviderAdapter(regionProvider))
	skController.RegisterEventSource(eventAdapter)
	c.stamps.RegisterSource(stamp.NewProviderAdapter(regionProvider))
	c.virtual.RegisterSource(vlive.NewProviderAdapter(regionProvider))
}

func newAppDeckController(cardProvider deck.CardSource, eventProvider deck.EventSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshotService snapshot.Snapshot, cfg Config) *deck.Controller {
	return deck.NewControllerWithConfig(cardProvider, eventProvider, drawingClient, assetHelper, snapshotService, cfg.DefaultRegion, deck.RecommendConfig{
		Enabled: cfg.DeckRecommend.Enabled, Disable: cfg.DeckRecommend.Disable,
		DisableReason: cfg.DeckRecommend.DisableReason, ServiceBaseURL: cfg.DeckRecommend.ServiceBaseURL,
		Targets: slices.Clone(cfg.DeckRecommend.Targets), SharedResources: cfg.SharedUpstreamResources,
		MasterdataDir: cfg.DeckRecommend.MasterdataDir, MasterdataRefreshInterval: cfg.DeckRecommend.MasterdataRefreshInterval,
		Timeout: cfg.DeckRecommend.Timeout, MaxRetries: cfg.DeckRecommend.MaxRetries,
		RetryWaitTime: cfg.DeckRecommend.RetryWaitTime, DefaultAlgs: slices.Clone(cfg.DeckRecommend.DefaultAlgs),
	}, cfg.MetaLoader)
}

func normalizeAppConfig(cfg *Config) context.Context {
	cfg.DefaultRegion = renderregion.WithDefault(cfg.DefaultRegion)
	initCtx := cfg.InitContext
	if initCtx == nil {
		initCtx = context.Background()
	}
	cfg.MetaLoader = resolveMetaLoader(initCtx, cfg.MetaLoader, cfg.MusicMetaRefreshInterval, cfg.MusicMetaOutputDir)
	if cfg.SharedUpstreamResources == nil {
		cfg.SharedUpstreamResources = &upstream.SharedResources{}
	}
	return initCtx
}

func newAppDependencies(initCtx context.Context, sekaiClient *sekaiDB.Client, cfg Config) appDependencies {
	assetHelper := assets.NewAssetHelper(cfg.AssetPrimaryDir, cfg.AssetLegacyDirs)
	snapshotService, staticSnapshotProvider := newAppSnapshotServices(initCtx, sekaiClient, assetHelper, cfg)
	drawingClient, imageStore := newAppDrawingClient(initCtx, cfg)
	localFallback, localDir, inventoryDir := appMasterdataDirs(cfg)
	return appDependencies{
		assets: assetHelper, snapshots: snapshotService, staticSnapshots: staticSnapshotProvider,
		drawing: drawingClient, imageStore: imageStore, localMasterdataFallback: localFallback,
		localMasterdataDir: localDir, inventoryMasterdataDir: inventoryDir,
	}
}

func newAppSnapshotServices(initCtx context.Context, sekaiClient *sekaiDB.Client, assetHelper *assets.AssetHelper, cfg Config) (snapshot.Snapshot, snapshot.HarukiSnapshotProvider) {
	if !shouldEnableLocalSnapshotFallback(cfg) {
		return nil, nil
	}
	service := snapshot.NewLocalFileServiceWithContext(initCtx, sekaiClient, assetHelper, snapshot.LocalFileConfig{
		DefaultRegion: cfg.DefaultRegion, UserJSON: cfg.UserSnapshot.UserJSON,
		MusicMetaJSON: cfg.UserSnapshot.MusicMetaJSON, MySekaiJSON: cfg.UserSnapshot.MySekaiJSON,
	})
	return service, snapshot.NewStaticSnapshotProvider(service)
}

func newAppDrawingClient(initCtx context.Context, cfg Config) (*drawing.HarukiDrawingClient, *imagecache.PGStore) {
	imageStore := openAppImageStore(initCtx, cfg.ImageCachePGURL)
	cacheConfig := cfg.DrawingCache
	cacheConfig.ImageCacheDir = cfg.ImageCacheDir
	cacheConfig.ImageStore = imageStore
	client := drawing.NewHarukiDrawingClientWithTargetsAndResources(
		cfg.DrawingBaseURL, cfg.DrawingTargets, cfg.SharedUpstreamResources, appDrawingOptions(cfg)...,
	)
	if client != nil {
		client.SetRenderCache(drawing.NewRenderCacheClient(cacheConfig))
	}
	return client, imageStore
}

func appDrawingOptions(cfg Config) []drawing.ClientOption {
	var options []drawing.ClientOption
	if cfg.DrawingTimeout > 0 {
		options = append(options, drawing.WithTimeout(cfg.DrawingTimeout))
	}
	if cfg.DrawingRetryCount > 0 {
		options = append(options, drawing.WithRetryCount(cfg.DrawingRetryCount))
	}
	if cfg.DrawingSKMaxConcurrency > 0 || cfg.DrawingSKAcquireTimeout > 0 || cfg.DrawingMaxConcurrency > 0 {
		options = append(options, drawing.WithLimiter(drawing.LimiterConfig{
			SKMaxConcurrency: cfg.DrawingSKMaxConcurrency, SKAcquireTimeout: cfg.DrawingSKAcquireTimeout,
			MaxConcurrency: cfg.DrawingMaxConcurrency,
		}))
	}
	return options
}

func openAppImageStore(initCtx context.Context, url string) *imagecache.PGStore {
	if url == "" {
		return nil
	}
	store, err := imagecache.NewPGStore(url)
	if err != nil {
		return nil
	}
	if err := store.Init(initCtx); err != nil {
		_ = store.Close()
		return nil
	}
	return store
}

func appMasterdataDirs(cfg Config) (bool, string, string) {
	localFallback := shouldEnableLocalMasterdataFallback(cfg)
	localDir := ""
	if localFallback {
		localDir = resolveRenderProviderMasterdataDir(cfg)
	}
	inventoryDir := localDir
	if inventoryDir == "" && strings.TrimSpace(cfg.LocalMasterdata.Dir) != "" {
		inventoryDir = resolveRenderProviderMasterdataDirFromWD(cfg, currentWorkingDir())
	}
	return localFallback, localDir, inventoryDir
}

func shouldEnableLocalMasterdataFallback(cfg Config) bool {
	if !cfg.LocalMasterdata.Enabled {
		return false
	}
	if !cfg.LocalMasterdata.AllowFallback && !cfg.LocalMasterdata.AllowLeaks {
		return false
	}
	return strings.TrimSpace(cfg.LocalMasterdata.Dir) != ""
}

func shouldEnableLocalSnapshotFallback(cfg Config) bool {
	if !cfg.UserSnapshot.AllowFallback {
		return false
	}
	if strings.TrimSpace(cfg.UserSnapshot.UserJSON) == "" {
		return false
	}

	switch strings.ToLower(strings.TrimSpace(cfg.UserSnapshot.Provider)) {
	case "", "local_file", "toolbox", "internal_cloud":
		return true
	default:
		return false
	}
}

func resolveMetaLoader(initCtx context.Context, configured *meta.Loader, refreshInterval time.Duration, outputDir string) *meta.Loader {
	if configured != nil {
		return configured
	}

	if initCtx == nil {
		initCtx = context.Background()
	}

	if refreshInterval <= 0 {
		refreshInterval = defaultMusicMetaRefreshInterval
	}

	loader := meta.NewLoader(logger.NewLoggerFromGlobal("PJSKMeta"), meta.WithOutputDir(outputDir))
	if err := loader.LoadAll(initCtx); err != nil {
		logger.WarnContext(initCtx, "music metadata initial load failed", "error_type", fmt.Sprintf("%T", err))
	}
	loader.StartBackgroundRefresh(initCtx, refreshInterval)
	return loader
}

func (a *App) AssetRoots() []string {
	if a == nil || a.Assets == nil {
		return nil
	}
	return a.Assets.Roots()
}

func (a *App) Close() error {
	if a == nil {
		return nil
	}
	var err error
	if a.ImageCache != nil {
		err = errors.Join(err, a.ImageCache.Close())
	}
	if a.MySekai != nil {
		a.MySekai.Close()
	}
	closeProvider := func(p provider.MasterDataProvider) {
		if closer, ok := p.(interface{ Close() error }); ok {
			err = errors.Join(err, closer.Close())
		}
	}
	if len(a.Providers) > 0 {
		for _, p := range a.Providers {
			closeProvider(p)
		}
	} else {
		closeProvider(a.Provider)
	}
	return err
}
