package server

import (
	"context"
	"encoding/hex"
	"fmt"
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
	rendercostume "haruki-cloud/internal/pjsk/render/costume"
)

func configureSekaiRuntime(mainLogger *harukiLogger.Logger, renderRuntime *renderapp.App, pjskClient *pjskDB.Client, usersClient *usersDB.Client, banChecker *accountdata.BanService, censorService *censor.Service) {
	if renderRuntime == nil || pjskClient == nil {
		return
	}

	var resolver *identity.Resolver
	if usersClient != nil {
		resolver = identity.NewResolver(usersClient)
		resolver.SetReadOnly(harukiConfig.Cfg.Node.ReadOnly)
		renderRuntime.Bindings = accountdata.NewBindingService(
			pjskClient,
			resolver,
			renderRuntime.SekaiAPI,
		)
		renderRuntime.Bindings.SetUsersDB(usersClient)
		renderRuntime.Bindings.SetReadOnly(harukiConfig.Cfg.Node.ReadOnly)
		renderRuntime.Bindings.SetFastVerificationProvider(renderRuntime.Toolbox)
		renderRuntime.PrivateDataCache = rendersnapshot.NewPrivateDataCache()
		renderRuntime.BuiltSnapshotCache = rendersnapshot.NewBuiltSnapshotCache()
		renderRuntime.Snapshots = rendersnapshot.NewFallbackSnapshotProvider(
			harukiConfig.Cfg.PJSKRender.UserSnapshot.AllowFallback,
			rendersnapshot.NewToolboxSnapshotProvider(
				renderRuntime.Bindings,
				renderRuntime.Toolbox,
				renderRuntime.Sekai,
				renderRuntime.Assets,
			).WithPrivateDataCache(renderRuntime.PrivateDataCache).
				WithBuiltSnapshotCache(renderRuntime.BuiltSnapshotCache),
			renderRuntime.Snapshots,
		)
		renderRuntime.MySekaiPayloads = rendersnapshot.NewFallbackMySekaiPayloadProvider(
			rendersnapshot.NewToolboxMySekaiPayloadProvider(
				renderRuntime.Bindings,
				renderRuntime.Toolbox,
			).WithPrivateDataCache(renderRuntime.PrivateDataCache),
		)
		if renderRuntime.Assets != nil {
			bgStore := accountdata.NewLocalProfileBGStore(renderRuntime.Assets.Primary())
			renderRuntime.Bindings.SetProfileBGStorage(bgStore)
		}
		if censorService != nil {
			renderRuntime.Bindings.SetCensorService(censorService)
		}
		renderRuntime.BanChecker = banChecker
	}

	renderRuntime.Aliases = pjskalias.NewService(renderRuntime.Sekai, pjskClient, resolver)
	if renderRuntime.Aliases != nil {
		renderRuntime.Aliases.SetReadOnly(harukiConfig.Cfg.Node.ReadOnly)
	}
	mainLogger.Info("Sekai runtime services configured")
}

func initPJSKRenderIfEnabled(ctx context.Context, mainLogger *harukiLogger.Logger, sekaiClient *sekaiDB.Client, pjskClient *pjskDB.Client) *renderapp.App {
	if !harukiConfig.Cfg.PJSKRender.Enabled {
		return nil
	}
	if sekaiClient == nil {
		fatalStartup(mainLogger, "PJSK render runtime requires Sekai database")
	}
	ctx = ensureContext(ctx)

	metaRefreshInterval := harukiConfig.Cfg.PJSKRender.MusicMeta.RefreshInterval
	if metaRefreshInterval <= 0 {
		metaRefreshInterval = harukiConfig.MetaRefreshInterval
	}
	metaOutputDir := harukiConfig.Cfg.PJSKRender.MusicMeta.OutputDir
	metaLoader := meta.NewLoader(harukiLogger.NewLoggerFromGlobal("MusicMeta"), meta.WithOutputDir(metaOutputDir))
	if err := metaLoader.LoadAll(ctx); err != nil {
		mainLogger.Warn("music meta initial load partially failed", "error_type", fmt.Sprintf("%T", err))
	}
	metaLoader.StartBackgroundRefresh(ctx, metaRefreshInterval)
	mainLogger.Info("music meta loader started", "refresh_interval", metaRefreshInterval, "has_output_dir", strings.TrimSpace(metaOutputDir) != "")

	sekaiAPIClient := sekaiAPI.NewSekaiAPIClient(&harukiConfig.Cfg.SekaiAPI)
	toolboxClient := sekaiAPI.NewToolboxClient(&harukiConfig.Cfg.Toolbox)
	trackerClient := sekaiAPI.NewTrackerClient(&harukiConfig.Cfg.Tracker)

	runtime := renderapp.New(sekaiClient, pjskClient, renderapp.Config{
		InitContext:             ctx,
		SekaiAPI:                sekaiAPIClient,
		Toolbox:                 toolboxClient,
		Tracker:                 trackerClient,
		DrawingBaseURL:          harukiConfig.Cfg.PJSKRender.DrawingBaseURL,
		DrawingTargets:          harukiConfig.Cfg.PJSKRender.DrawingTargets,
		DrawingTimeout:          harukiConfig.Cfg.PJSKRender.DrawingTimeout,
		DrawingRetryCount:       harukiConfig.Cfg.PJSKRender.DrawingRetryCount,
		DrawingSKMaxConcurrency: harukiConfig.Cfg.PJSKRender.DrawingSKMaxConcurrency,
		DrawingSKAcquireTimeout: harukiConfig.Cfg.PJSKRender.DrawingSKAcquireTimeout,
		DrawingMaxConcurrency:   harukiConfig.Cfg.PJSKRender.DrawingMaxConcurrency,
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
		SekaiDBType: harukiConfig.Cfg.Sekai.DBType,
		SekaiDSN:    harukiConfig.Cfg.Sekai.DBURL,
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
		ReadOnly:                                 harukiConfig.Cfg.Node.ReadOnly,
		Preview3D: rendercostume.Preview3DConfig{
			Enabled:               harukiConfig.Cfg.PJSKRender.Preview3D.Enabled,
			EngineBaseURL:         harukiConfig.Cfg.PJSKRender.Preview3D.EngineBaseURL,
			EngineBaseURLs:        harukiConfig.Cfg.PJSKRender.Preview3D.EngineBaseURLs,
			StaticRelativeDir:     harukiConfig.Cfg.PJSKRender.Preview3D.StaticRelativeDir,
			StaticOutputDir:       harukiConfig.Cfg.PJSKRender.Preview3D.StaticOutputDir,
			Width:                 harukiConfig.Cfg.PJSKRender.Preview3D.Width,
			Height:                harukiConfig.Cfg.PJSKRender.Preview3D.Height,
			Scale:                 harukiConfig.Cfg.PJSKRender.Preview3D.Scale,
			Timeout:               harukiConfig.Cfg.PJSKRender.Preview3D.Timeout,
			RegistryCacheTTL:      harukiConfig.Cfg.PJSKRender.Preview3D.RegistryCacheTTL,
			CaptureExistsTTL:      harukiConfig.Cfg.PJSKRender.Preview3D.CaptureExistsTTL,
			CaptureMaxConcurrency: harukiConfig.Cfg.PJSKRender.Preview3D.CaptureMaxConcurrency,
			CaptureAcquireTimeout: harukiConfig.Cfg.PJSKRender.Preview3D.CaptureAcquireTimeout,
			TemporaryCaptureTTL:   harukiConfig.Cfg.PJSKRender.Preview3D.TemporaryCaptureTTL,
			CaptureCacheVersion:   harukiConfig.Cfg.PJSKRender.Preview3D.CaptureCacheVersion,
			CameraPreset:          harukiConfig.Cfg.PJSKRender.Preview3D.CameraPreset,
			CameraProfile:         harukiConfig.Cfg.PJSKRender.Preview3D.CameraProfile,
		},
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
		mainLogger.Warn("PJSK render runtime initialized without drawing service", "build_only", true)
	}
	mainLogger.Info("PJSK render runtime initialized", "asset_root_count", len(runtime.AssetRoots()))
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

// validateBotAuthSecrets fails fast when a bot JWT signing secret is empty.
// An empty HMAC key signs/verifies tokens with a zero-length key, which is
// forgeable — the Noise keys are already validated the same way, and bot auth
// routes are always registered, so these must be configured too.
func validateBotAuthSecrets(mainLogger *harukiLogger.Logger) {
	if strings.TrimSpace(harukiConfig.Cfg.HarukiBotDB.SessionSignToken) == "" {
		fatalStartup(mainLogger, "bot session signing token is not configured")
	}
	if strings.TrimSpace(harukiConfig.Cfg.HarukiBotDB.CredentialSignToken) == "" {
		fatalStartup(mainLogger, "bot credential signing token is not configured")
	}
}

func initNoiseKeyRing(mainLogger *harukiLogger.Logger) *crypto.KeyRing {
	botCfg := harukiConfig.Cfg.HarukiBotDB
	keys := make([]crypto.StaticKey, 0, 1+len(botCfg.NoiseKeys))
	if legacy := strings.TrimSpace(botCfg.NoisePrivateKey); legacy != "" {
		keys = append(keys, crypto.StaticKey{ID: crypto.DefaultKeyID, Pair: parseNoisePrivateKey(mainLogger, "noise_private_key", legacy)})
	}
	for i, entry := range botCfg.NoiseKeys {
		field := fmt.Sprintf("noise_keys[%d]", i)
		keyID := strings.TrimSpace(entry.KeyID)
		if keyID == "" {
			fatalStartup(mainLogger, "Noise key id is empty", "config_field", field)
		}
		keys = append(keys, crypto.StaticKey{ID: keyID, Pair: parseNoisePrivateKey(mainLogger, field, entry.PrivateKey)})
	}
	if len(keys) == 0 {
		fatalStartup(mainLogger, "Noise private key is not configured")
	}
	ring, err := crypto.NewKeyRing(keys...)
	if err != nil {
		fatalStartup(mainLogger, "invalid Noise key ring", "error_type", fmt.Sprintf("%T", err), "detail", err.Error())
	}
	for i, key := range ring.Keys() {
		role := "rotation"
		if i == 0 {
			role = "primary"
		}
		mainLogger.Info("Noise NK static key loaded", "key_id", key.ID, "role", role, "public_key", hex.EncodeToString(key.Pair.Public))
	}
	return ring
}

func parseNoisePrivateKey(mainLogger *harukiLogger.Logger, field string, keyHex string) *crypto.KeyPair {
	privBytes, err := hex.DecodeString(strings.TrimSpace(keyHex))
	if err != nil {
		fatalStartup(mainLogger, "invalid Noise private key hex", "config_field", field, "error_type", fmt.Sprintf("%T", err))
	}
	if len(privBytes) != 32 {
		fatalStartup(mainLogger, "Noise private key has invalid length", "config_field", field, "key_bytes", len(privBytes))
	}
	kp, err := crypto.KeyPairFromPrivate(privBytes)
	if err != nil {
		fatalStartup(mainLogger, "failed to derive Noise key pair", "config_field", field, "error_type", fmt.Sprintf("%T", err))
	}
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
		mainLogger.Error("failed to connect to Censor DB", "error_type", fmt.Sprintf("%T", err))
		return nil
	}
	if err := censorClient.Schema.Create(ctx); err != nil {
		mainLogger.Error("failed to create schema for Censor DB", "error_type", fmt.Sprintf("%T", err))
		_ = censorClient.Close()
		return nil
	}
	installEntTracing(censorClient)

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

	mainLogger.Info("censor service initialized")
	return svc
}
