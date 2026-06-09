package server

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/pjsk/accountdata"
	pjskalias "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/meta"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	sekaiAPI "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/censor"
	harukiLogger "haruki-cloud/utils/logger"

	censorDB "haruki-cloud/database/censor"
	pjskDB "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	usersDB "haruki-cloud/database/users"

	renderapp "haruki-cloud/internal/pjsk/render/app"
)

func configureSekaiRuntime(mainLogger *harukiLogger.Logger, renderRuntime *renderapp.App, pjskClient *pjskDB.Client, usersClient *usersDB.Client, censorService *censor.Service) {
	if renderRuntime == nil || pjskClient == nil {
		return
	}

	var resolver *identity.Resolver
	if usersClient != nil {
		resolver = identity.NewResolver(usersClient)
		renderRuntime.Bindings = accountdata.NewBindingService(
			pjskClient,
			resolver,
			renderRuntime.SekaiAPI,
		)
		renderRuntime.Bindings.SetFastVerificationProvider(renderRuntime.Toolbox)
		renderRuntime.Snapshots = rendersnapshot.NewFallbackSnapshotProvider(
			harukiConfig.Cfg.PJSKRender.UserSnapshot.AllowFallback,
			rendersnapshot.NewToolboxSnapshotProvider(
				renderRuntime.Bindings,
				renderRuntime.Toolbox,
				renderRuntime.Sekai,
				renderRuntime.Assets,
			),
			renderRuntime.Snapshots,
		)
		renderRuntime.MySekaiPayloads = rendersnapshot.NewFallbackMySekaiPayloadProvider(
			rendersnapshot.NewToolboxMySekaiPayloadProvider(
				renderRuntime.Bindings,
				renderRuntime.Toolbox,
			),
		)
		if renderRuntime.Assets != nil {
			bgStore := accountdata.NewLocalProfileBGStore(renderRuntime.Assets.Primary())
			renderRuntime.Bindings.SetProfileBGStorage(bgStore)
		}
		if censorService != nil {
			renderRuntime.Bindings.SetCensorService(censorService)
		}
		renderRuntime.BanChecker = accountdata.NewBanService(usersClient)
	}

	renderRuntime.Aliases = pjskalias.NewService(renderRuntime.Sekai, pjskClient, resolver)
	mainLogger.Infof("Sekai runtime services configured")
}

func initPJSKRenderIfEnabled(ctx context.Context, mainLogger *harukiLogger.Logger, sekaiClient *sekaiDB.Client, pjskClient *pjskDB.Client) *renderapp.App {
	if !harukiConfig.Cfg.PJSKRender.Enabled {
		return nil
	}
	if sekaiClient == nil {
		mainLogger.Errorf("PJSK render runtime requires sekai.enabled=true")
		os.Exit(1)
	}
	ctx = ensureContext(ctx)

	metaRefreshInterval := harukiConfig.Cfg.PJSKRender.MusicMeta.RefreshInterval
	if metaRefreshInterval <= 0 {
		metaRefreshInterval = harukiConfig.MetaRefreshInterval
	}
	metaOutputDir := harukiConfig.Cfg.PJSKRender.MusicMeta.OutputDir
	metaLoader := meta.NewLoader(harukiLogger.NewLoggerFromGlobal("MusicMeta"), meta.WithOutputDir(metaOutputDir))
	if err := metaLoader.LoadAll(ctx); err != nil {
		mainLogger.Warnf("music meta initial load partially failed: %v", err)
	}
	metaLoader.StartBackgroundRefresh(ctx, metaRefreshInterval)
	mainLogger.Infof("Music meta loader started (refresh=%s output_dir=%q)", metaRefreshInterval, metaOutputDir)

	sekaiAPIClient := sekaiAPI.NewSekaiAPIClient(&harukiConfig.Cfg.SekaiAPI)
	toolboxClient := sekaiAPI.NewToolboxClient(&harukiConfig.Cfg.Toolbox)
	trackerClient := sekaiAPI.NewTrackerClient(&harukiConfig.Cfg.Tracker)

	runtime := renderapp.New(sekaiClient, pjskClient, renderapp.Config{
		InitContext:       ctx,
		SekaiAPI:          sekaiAPIClient,
		Toolbox:           toolboxClient,
		Tracker:           trackerClient,
		DrawingBaseURL:    harukiConfig.Cfg.PJSKRender.DrawingBaseURL,
		DrawingTargets:    harukiConfig.Cfg.PJSKRender.DrawingTargets,
		DrawingTimeout:    harukiConfig.Cfg.PJSKRender.DrawingTimeout,
		DrawingRetryCount: harukiConfig.Cfg.PJSKRender.DrawingRetryCount,
		DrawingCache: drawing.RenderCacheConfig{
			BaseURL:    harukiConfig.Cfg.PJSKRender.DrawingCache.BaseURL,
			StorageDir: harukiConfig.Cfg.PJSKRender.DrawingCache.StorageDir,
			TTL:        harukiConfig.Cfg.PJSKRender.DrawingCache.TTL,
		},
		ImageCacheURI:   harukiConfig.Cfg.PJSKRender.ImageCache.URI,
		ChartsBaseURL:   harukiConfig.Cfg.PJSKRender.ImageCache.ChartsURI,
		ImageCacheDir:   harukiConfig.Cfg.PJSKRender.ImageCache.Dir,
		ImageCachePGURL: harukiConfig.Cfg.PJSKRender.ImageCache.PGURL,
		AssetPrimaryDir: harukiConfig.Cfg.PJSKRender.AssetDirs.Primary,
		AssetLegacyDirs: harukiConfig.Cfg.PJSKRender.AssetDirs.Legacy,
		AssetsBaseURL:   harukiConfig.Cfg.PJSKRender.AssetDirs.AssetsBaseURL,
		LocalMasterdata: renderapp.LocalMasterdataConfig{
			Enabled:         harukiConfig.Cfg.PJSKRender.LocalMasterdata.Enabled,
			AllowFallback:   harukiConfig.Cfg.PJSKRender.LocalMasterdata.AllowFallback,
			AllowLeaks:      harukiConfig.Cfg.PJSKRender.LocalMasterdata.AllowLeaks,
			Dir:             harukiConfig.Cfg.PJSKRender.LocalMasterdata.Dir,
			RefreshInterval: harukiConfig.Cfg.PJSKRender.LocalMasterdata.RefreshInterval,
		},
		SekaiDSN: harukiConfig.Cfg.Sekai.DBURL,
		UserSnapshot: renderapp.UserSnapshotConfig{
			Provider:      harukiConfig.Cfg.PJSKRender.UserSnapshot.Provider,
			AllowFallback: harukiConfig.Cfg.PJSKRender.UserSnapshot.AllowFallback,
			UserJSON:      harukiConfig.Cfg.PJSKRender.UserSnapshot.UserJSON,
			MusicMetaJSON: harukiConfig.Cfg.PJSKRender.UserSnapshot.MusicMetaJSON,
			MySekaiJSON:   harukiConfig.Cfg.PJSKRender.UserSnapshot.MySekaiJSON,
		},
		MusicMetaOutputDir: metaOutputDir,
		MetaLoader:         metaLoader,
		SKForecast: renderapp.SKForecastConfig{
			LocalBaseURL: harukiConfig.Cfg.PJSKRender.SKForecast.LocalBaseURL,
			CachePath:    resolveSKForecastCachePath(),
		},
		MySekaiHousingCompetitionCachePath:       resolveMySekaiHousingCompetitionCachePath(),
		MySekaiHousingCompetitionRefreshInterval: harukiConfig.Cfg.PJSKRender.MySekaiHousingCompetition.RefreshInterval,
		DeckRecommend: renderapp.DeckRecommendConfig{
			Enabled:                   harukiConfig.Cfg.PJSKRender.DeckRecommend.Enabled,
			Disable:                   harukiConfig.Cfg.PJSKRender.DeckRecommend.Disable,
			DisableReason:             harukiConfig.Cfg.PJSKRender.DeckRecommend.DisableReason,
			ServiceBaseURL:            harukiConfig.Cfg.PJSKRender.DeckRecommend.ServiceBaseURL,
			Targets:                   harukiConfig.Cfg.PJSKRender.DeckRecommend.Targets,
			MasterdataDir:             resolveDeckRecommendMasterdataDir(),
			MasterdataRefreshInterval: harukiConfig.Cfg.PJSKRender.DeckRecommend.MasterdataRefreshInterval,
			Timeout:                   harukiConfig.Cfg.PJSKRender.DeckRecommend.Timeout,
			MaxRetries:                harukiConfig.Cfg.PJSKRender.DeckRecommend.MaxRetries,
			RetryWaitTime:             harukiConfig.Cfg.PJSKRender.DeckRecommend.RetryWaitTime,
			DefaultAlgs:               harukiConfig.Cfg.PJSKRender.DeckRecommend.DefaultAlgs,
		},
	})

	if runtime.Drawing == nil {
		mainLogger.Warnf("PJSK render runtime initialized without drawing_base_url; build-only mode")
	}
	mainLogger.Infof("PJSK render asset roots: %v", runtime.AssetRoots())
	return runtime
}

func resolveDeckRecommendMasterdataDir() string {
	return strings.TrimSpace(harukiConfig.Cfg.PJSKRender.DeckRecommend.MasterdataDir)
}

func resolveSKForecastCachePath() string {
	if path := strings.TrimSpace(harukiConfig.Cfg.PJSKRender.SKForecast.CachePath); path != "" {
		return path
	}
	if dir := strings.TrimSpace(harukiConfig.Cfg.PJSKRender.DrawingCache.StorageDir); dir != "" {
		return filepath.Join(dir, "sk_forecast_cache.json")
	}
	return ""
}

func resolveMySekaiHousingCompetitionCachePath() string {
	if path := strings.TrimSpace(harukiConfig.Cfg.PJSKRender.MySekaiHousingCompetition.CachePath); path != "" {
		return path
	}
	if dir := strings.TrimSpace(harukiConfig.Cfg.PJSKRender.DrawingCache.StorageDir); dir != "" {
		return filepath.Join(dir, "mysekai_housing_competition_stats.json")
	}
	return ""
}

func initAuthEncryptionKey(mainLogger *harukiLogger.Logger) []byte {
	keyHex := strings.TrimSpace(harukiConfig.Cfg.HarukiBotDB.AuthEncryptionKey)
	if keyHex == "" {
		mainLogger.Errorf("auth_encryption_key is required but not configured")
		os.Exit(1)
	}
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		mainLogger.Errorf("Invalid auth_encryption_key hex: %v", err)
		os.Exit(1)
	}
	if len(keyBytes) != 32 {
		mainLogger.Errorf("auth_encryption_key must be 32 bytes (got %d)", len(keyBytes))
		os.Exit(1)
	}
	mainLogger.Infof("Auth encryption key loaded (AES-256-GCM)")
	return keyBytes
}

func initNoiseKeyPair(mainLogger *harukiLogger.Logger) *crypto.KeyPair {
	keyHex := strings.TrimSpace(harukiConfig.Cfg.HarukiBotDB.NoisePrivateKey)
	if keyHex == "" {
		mainLogger.Errorf("noise_private_key is required but not configured")
		os.Exit(1)
	}
	privBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		mainLogger.Errorf("Invalid noise_private_key hex: %v", err)
		os.Exit(1)
	}
	if len(privBytes) != 32 {
		mainLogger.Errorf("noise_private_key must be 32 bytes (got %d)", len(privBytes))
		os.Exit(1)
	}
	kp, err := crypto.KeyPairFromPrivate(privBytes)
	if err != nil {
		mainLogger.Errorf("Failed to derive Noise key pair: %v", err)
		os.Exit(1)
	}
	mainLogger.Infof("Noise IK transport encryption enabled (pubkey=%x)", kp.Public)
	return kp
}

func initCensorIfEnabled(ctx context.Context, mainLogger *harukiLogger.Logger, renderRuntime *renderapp.App) *censor.Service {
	cfg := harukiConfig.Cfg.Censor
	if strings.TrimSpace(cfg.CensorDBType) == "" || strings.TrimSpace(cfg.CensorDBURL) == "" {
		return nil
	}
	ctx = ensureContext(ctx)

	censorClient, err := censorDB.Open(cfg.CensorDBType, cfg.CensorDBURL)
	if err != nil {
		mainLogger.Errorf("Failed to connect to Censor DB: %v", err)
		return nil
	}
	if err := censorClient.Schema.Create(ctx); err != nil {
		mainLogger.Errorf("Failed to create schema for Censor DB: %v", err)
		_ = censorClient.Close()
		return nil
	}

	svc := censor.NewService(
		cfg.BaiduAPIKey, cfg.BaiduSecret,
		cfg.TencentSecretID, cfg.TencentSecretKey, cfg.TencentRegion, cfg.TencentBizType,
		censorClient,
	)

	if renderRuntime != nil {
		renderRuntime.Censor = svc
		if renderRuntime.Profiles != nil {
			renderRuntime.Profiles.SetCensor(svc)
		}
	}

	mainLogger.Infof("Censor service initialized")
	return svc
}
