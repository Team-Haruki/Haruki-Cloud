package app

import (
	"context"
	"slices"
	"strings"
	"time"

	pjskDB "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	pjskalias "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/meta"
	renderregion "haruki-cloud/internal/pjsk/region"
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
	"haruki-cloud/internal/pjsk/render/score"
	"haruki-cloud/internal/pjsk/render/sk"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/pjsk/render/stamp"
	"haruki-cloud/internal/pjsk/render/vlive"
	"haruki-cloud/utils/imagecache"
	"haruki-cloud/utils/logger"
)

func New(sekaiClient *sekaiDB.Client, pjskClient *pjskDB.Client, cfg Config) *App {
	cfg.DefaultRegion = renderregion.WithDefault(cfg.DefaultRegion)
	initCtx := cfg.InitContext
	if initCtx == nil {
		initCtx = context.Background()
	}
	cfg.MetaLoader = resolveMetaLoader(initCtx, cfg.MetaLoader, cfg.MusicMetaRefreshInterval)

	assetHelper := assets.NewAssetHelper(cfg.AssetPrimaryDir, cfg.AssetLegacyDirs)
	var snapshotService snapshot.Snapshot
	if shouldEnableLocalSnapshotFallback(cfg) {
		snapshotService = snapshot.NewLocalFileServiceWithContext(initCtx, sekaiClient, assetHelper, snapshot.LocalFileConfig{
			DefaultRegion: cfg.DefaultRegion,
			UserJSON:      cfg.UserSnapshot.UserJSON,
			MusicMetaJSON: cfg.UserSnapshot.MusicMetaJSON,
			MySekaiJSON:   cfg.UserSnapshot.MySekaiJSON,
		})
	}
	var staticSnapshotProvider snapshot.HarukiSnapshotProvider
	if snapshotService != nil {
		staticSnapshotProvider = snapshot.NewStaticSnapshotProvider(snapshotService)
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
	mysekaiController := mysekai.NewController(drawingClient, snapshotService, cfg.DefaultRegion, assetHelper, mysekai.MasterdataOptions{
		SekaiDSN:      cfg.SekaiDSN,
		LocalDir:      cfg.LocalMasterdata.Dir,
		AllowFallback: cfg.LocalMasterdata.AllowFallback,
	})
	musicController := (*music.Controller)(nil)
	deckController := deck.NewControllerWithConfig(nil, nil, drawingClient, assetHelper, snapshotService, cfg.DefaultRegion, deck.RecommendConfig{
		Enabled:        cfg.DeckRecommend.Enabled,
		ServiceBaseURL: cfg.DeckRecommend.ServiceBaseURL,
		MasterdataDir:  cfg.DeckRecommend.MasterdataDir,
		Timeout:        cfg.DeckRecommend.Timeout,
		MaxRetries:     cfg.DeckRecommend.MaxRetries,
		RetryWaitTime:  cfg.DeckRecommend.RetryWaitTime,
		DefaultAlgs:    slices.Clone(cfg.DeckRecommend.DefaultAlgs),
	}, cfg.MetaLoader)
	educationController := education.NewController(drawingClient, assetHelper, snapshotService, cfg.DefaultRegion)
	scoreController := score.NewController(drawingClient)
	skController := sk.NewController(drawingClient)
	skController.SetTrackerIntegration(cfg.Tracker, nil, assetHelper)

	var cardController *card.Controller
	var eventController *event.Controller
	var gachaController *gacha.Controller
	var honorController *honor.Controller
	var profileController *profile.Controller
	var stampController *stamp.Controller
	var vliveController *vlive.Controller
	var masterProvider provider.MasterDataProvider
	providersByRegion := make(map[renderregion.Value]provider.MasterDataProvider)
	registerProvider := func(src provider.MasterDataProvider) {
		if src == nil {
			return
		}
		region := renderregion.WithDefault(src.Region())
		providersByRegion[region] = src
		if masterProvider == nil {
			masterProvider = src
		}
	}
	if sekaiClient != nil {
		renderMasterdataDir := resolveRenderProviderMasterdataDir(cfg)
		masterDBProvider := provider.NewDatabaseProvider(sekaiClient, cfg.DefaultRegion)
		masterDBProvider.SetLocalMasterdataDir(renderMasterdataDir, cfg.LocalMasterdata.AllowLeaks)
		registerProvider(masterDBProvider)

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

		// Initialize controllers with provider adapters.
		skController.SetTrackerIntegration(cfg.Tracker, eventAdapter, assetHelper)
		deckController = deck.NewControllerWithConfig(cardAdapter, eventAdapter, drawingClient, assetHelper, snapshotService, cfg.DefaultRegion, deck.RecommendConfig{
			Enabled:        cfg.DeckRecommend.Enabled,
			ServiceBaseURL: cfg.DeckRecommend.ServiceBaseURL,
			MasterdataDir:  cfg.DeckRecommend.MasterdataDir,
			Timeout:        cfg.DeckRecommend.Timeout,
			MaxRetries:     cfg.DeckRecommend.MaxRetries,
			RetryWaitTime:  cfg.DeckRecommend.RetryWaitTime,
			DefaultAlgs:    slices.Clone(cfg.DeckRecommend.DefaultAlgs),
		}, cfg.MetaLoader)
		deckController.RegisterMusicSource(musicAdapter)
		cardController = card.NewController(cardAdapter, eventAdapter, drawingClient, assetHelper)
		educationController.RegisterSource(educationAdapter)
		eventController = event.NewController(eventAdapter, drawingClient, assetHelper)
		gachaController = gacha.NewController(gachaAdapter, drawingClient, assetHelper)
		honorController = honor.NewController(honorAdapter, drawingClient, assetHelper)
		musicController = music.NewController(musicAdapter, drawingClient, assetHelper, snapshotService, cfg.MetaLoader)
		profileController = profile.NewController(profileAdapter, drawingClient, assetHelper, snapshotService)
		stampController = stamp.NewController(stampAdapter, drawingClient, assetHelper)
		vliveController = vlive.NewController(vliveAdapter, cfg.DefaultRegion)

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
			regionProvider := provider.NewDatabaseProvider(sekaiClient, region)
			regionProvider.SetLocalMasterdataDir(renderMasterdataDir, cfg.LocalMasterdata.AllowLeaks)
			registerProvider(regionProvider)
			regionCardAdapter := card.NewProviderAdapter(regionProvider)
			regionEventAdapter := event.NewProviderAdapter(regionProvider)
			regionMusicAdapter := music.NewProviderAdapter(regionProvider)
			regionGachaAdapter := gacha.NewProviderAdapter(regionProvider)
			regionHonorAdapter := honor.NewProviderAdapter(regionProvider)
			regionStampAdapter := stamp.NewProviderAdapter(regionProvider)
			regionProfileAdapter := profile.NewProviderAdapter(regionProvider)
			regionEducationAdapter := education.NewProviderAdapter(regionProvider)

			cardController.RegisterSource(regionCardAdapter)
			cardController.RegisterEventSource(regionEventAdapter)
			deckController.RegisterCardSource(regionCardAdapter)
			deckController.RegisterEventSource(regionEventAdapter)
			deckController.RegisterMusicSource(regionMusicAdapter)
			educationController.RegisterSource(regionEducationAdapter)
			eventController.RegisterSource(regionEventAdapter)
			gachaController.RegisterSource(regionGachaAdapter)
			honorController.RegisterSource(regionHonorAdapter)
			musicController.RegisterSource(regionMusicAdapter)
			profileController.RegisterSource(regionProfileAdapter)
			skController.RegisterEventSource(regionEventAdapter)
			stampController.RegisterSource(regionStampAdapter)
		}
	}

	aliasService := pjskalias.NewService(sekaiClient, pjskClient, nil)
	if musicController != nil {
		musicController.SetAliasResolver(aliasService)
	}
	if cardController != nil && aliasService != nil {
		if nicknames, err := aliasService.ListApprovedCharacterAliasMap(initCtx); err != nil {
			logger.Warnf("load approved character aliases for card controller failed: %v", err)
		} else {
			cardController.MergeNicknames(nicknames)
		}
	}

	var imgStore *imagecache.PGStore
	if cfg.ImageCachePGURL != "" {
		if store, err := imagecache.NewPGStore(cfg.ImageCachePGURL); err == nil {
			if err := store.Init(initCtx); err == nil {
				imgStore = store
			} else {
				_ = store.Close()
			}
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
		Providers:  providersByRegion,
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
		Snapshots:  staticSnapshotProvider,
		ImageCache: imagecache.NewWithStore(cfg.ImageCacheURI, cfg.ImageCacheDir, imgStore),
		Censor:     cfg.CensorService,
		SekaiAPI:   cfg.SekaiAPI,
		Toolbox:    cfg.Toolbox,
		Tracker:    cfg.Tracker,
		Config:     cfg,
	}
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

func resolveMetaLoader(initCtx context.Context, configured *meta.Loader, refreshInterval time.Duration) *meta.Loader {
	if configured != nil {
		return configured
	}

	if initCtx == nil {
		initCtx = context.Background()
	}

	if refreshInterval <= 0 {
		refreshInterval = defaultMusicMetaRefreshInterval
	}

	loader := meta.NewLoader(logger.NewLoggerFromGlobal("PJSKMeta"))
	if err := loader.LoadAll(initCtx); err != nil {
		logger.Warnf("meta: initial load failed: %v", err)
	}
	loader.StartBackgroundRefresh(context.Background(), refreshInterval)
	return loader
}

func (a *App) AssetRoots() []string {
	if a == nil || a.Assets == nil {
		return nil
	}
	return a.Assets.Roots()
}

func (a *App) Close() error {
	if a == nil || a.ImageCache == nil {
		return nil
	}
	return a.ImageCache.Close()
}
