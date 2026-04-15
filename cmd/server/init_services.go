package main

import (
	"context"
	"encoding/hex"
	"os"
	"strings"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/identity"
	pjskalias "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/meta"
	renderuserdata "haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/utils/censor"
	"haruki-cloud/internal/pjsk/drawing"
	harukiLogger "haruki-cloud/utils/logger"
	sekaiAPI "haruki-cloud/internal/pjsk/sekai"

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
			sekaiAPI.GetSekaiAPIClient(),
		)
		renderRuntime.Bindings.SetFastVerificationProvider(sekaiAPI.GetToolboxClient())
		renderRuntime.Snapshots = renderuserdata.NewFallbackSnapshotProvider(
			harukiConfig.Cfg.PJSKRender.UserSnapshot.AllowFallback,
			renderuserdata.NewToolboxSnapshotProvider(
				renderRuntime.Bindings,
				sekaiAPI.GetToolboxClient(),
				renderRuntime.Sekai,
				renderRuntime.Assets,
			),
			renderRuntime.Snapshots,
		)
		renderRuntime.MySekaiPayloads = renderuserdata.NewFallbackMySekaiPayloadProvider(
			renderuserdata.NewToolboxMySekaiPayloadProvider(
				renderRuntime.Bindings,
				sekaiAPI.GetToolboxClient(),
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
	metaLoader := meta.NewLoader(harukiLogger.NewLoggerFromGlobal("MusicMeta"))
	if err := metaLoader.LoadAll(ctx); err != nil {
		mainLogger.Warnf("music meta initial load partially failed: %v", err)
	}
	metaLoader.StartBackgroundRefresh(ctx, metaRefreshInterval)
	mainLogger.Infof("Music meta loader started (refresh=%s)", metaRefreshInterval)

	runtime := renderapp.New(sekaiClient, pjskClient, renderapp.Config{
		InitContext:       ctx,
		DrawingBaseURL:    harukiConfig.Cfg.PJSKRender.DrawingBaseURL,
		DrawingTimeout:    harukiConfig.Cfg.PJSKRender.DrawingTimeout,
		DrawingRetryCount: harukiConfig.Cfg.PJSKRender.DrawingRetryCount,
		DrawingCache: drawing.RenderCacheConfig{
			BaseURL:    harukiConfig.Cfg.PJSKRender.DrawingCache.BaseURL,
			StorageDir: harukiConfig.Cfg.PJSKRender.DrawingCache.StorageDir,
			TTL:        harukiConfig.Cfg.PJSKRender.DrawingCache.TTL,
		},
		ImageCacheURI:   harukiConfig.Cfg.PJSKRender.ImageCache.URI,
		ImageCacheDir:   harukiConfig.Cfg.PJSKRender.ImageCache.Dir,
		ImageCachePGURL: harukiConfig.Cfg.PJSKRender.ImageCache.PGURL,
		AssetPrimaryDir: harukiConfig.Cfg.PJSKRender.AssetDirs.Primary,
		AssetLegacyDirs: harukiConfig.Cfg.PJSKRender.AssetDirs.Legacy,
		AssetsBaseURL:   harukiConfig.Cfg.PJSKRender.AssetDirs.AssetsBaseURL,
		LocalMasterdata: renderapp.LocalMasterdataConfig{
			Enabled:       harukiConfig.Cfg.PJSKRender.LocalMasterdata.Enabled,
			AllowFallback: harukiConfig.Cfg.PJSKRender.LocalMasterdata.AllowFallback,
			Dir:           harukiConfig.Cfg.PJSKRender.LocalMasterdata.Dir,
		},
		SekaiDSN: harukiConfig.Cfg.Sekai.DBURL,
		UserSnapshot: renderapp.UserSnapshotConfig{
			Provider:      harukiConfig.Cfg.PJSKRender.UserSnapshot.Provider,
			AllowFallback: harukiConfig.Cfg.PJSKRender.UserSnapshot.AllowFallback,
			UserJSON:      harukiConfig.Cfg.PJSKRender.UserSnapshot.UserJSON,
			MusicMetaJSON: harukiConfig.Cfg.PJSKRender.UserSnapshot.MusicMetaJSON,
			MySekaiJSON:   harukiConfig.Cfg.PJSKRender.UserSnapshot.MySekaiJSON,
		},
		MetaLoader: metaLoader,
		DeckRecommend: renderapp.DeckRecommendConfig{
			Enabled:        harukiConfig.Cfg.PJSKRender.DeckRecommend.Enabled,
			ServiceBaseURL: harukiConfig.Cfg.PJSKRender.DeckRecommend.ServiceBaseURL,
			MasterdataDir:  resolveDeckRecommendMasterdataDir(),
			Timeout:        harukiConfig.Cfg.PJSKRender.DeckRecommend.Timeout,
			MaxRetries:     harukiConfig.Cfg.PJSKRender.DeckRecommend.MaxRetries,
			RetryWaitTime:  harukiConfig.Cfg.PJSKRender.DeckRecommend.RetryWaitTime,
			DefaultAlgs:    harukiConfig.Cfg.PJSKRender.DeckRecommend.DefaultAlgs,
		},
	})

	if runtime.Drawing == nil {
		mainLogger.Warnf("PJSK render runtime initialized without drawing_base_url; build-only mode")
	}
	mainLogger.Infof("PJSK render asset roots: %v", runtime.AssetRoots())
	return runtime
}

func resolveDeckRecommendMasterdataDir() string {
	dir := strings.TrimSpace(harukiConfig.Cfg.PJSKRender.DeckRecommend.MasterdataDir)
	if dir != "" {
		return dir
	}
	return strings.TrimSpace(harukiConfig.Cfg.PJSKRender.LocalMasterdata.Dir)
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
