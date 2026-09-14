package server

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"haruki-cloud/api"
	"haruki-cloud/internal/core/buildpolicy"
	"haruki-cloud/internal/core/secevent"
	"path/filepath"
	"strings"
	"time"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/core/trustsign"
	"haruki-cloud/internal/core/urlhost"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/pjsk/accountdata"
	pjskalias "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/meta"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	sekaiAPI "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/internal/storage"
	storages3 "haruki-cloud/internal/storage/s3"
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
		renderRuntime.Bindings.SetProfileBGStorage(profileBGStorageFor(renderRuntime))
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

// profileBGStorageFor builds the profile background store on the runtime's
// user_upload slot (E1), never on the asset primary root. An unset slot is
// Disabled and reports "profile background storage is not configured".
func profileBGStorageFor(renderRuntime *renderapp.App) *accountdata.ProfileBGStore {
	return accountdata.NewProfileBGStore(renderRuntime.Stores.UserUpload)
}

func initPJSKRenderIfEnabled(ctx context.Context, mainLogger *harukiLogger.Logger, sekaiClient *sekaiDB.Client, pjskClient *pjskDB.Client) *renderapp.App {
	if !harukiConfig.Cfg.PJSKRender.Enabled {
		return nil
	}
	if sekaiClient == nil {
		fatalStartup(mainLogger, "PJSK render runtime requires Sekai database")
	}
	ctx = ensureContext(ctx)

	stores, err := buildRenderStores(harukiConfig.Cfg.PJSKRender, mainLogger)
	if err != nil {
		fatalStartup(mainLogger, "storage configuration invalid", "error", err)
	}
	cacheTargets, err := resolveRenderCacheTargets(harukiConfig.Cfg.PJSKRender, stores, mainLogger)
	if err != nil {
		fatalStartup(mainLogger, "cache storage configuration invalid", "error", err)
	}

	metaRefreshInterval := harukiConfig.Cfg.PJSKRender.MusicMeta.RefreshInterval
	if metaRefreshInterval <= 0 {
		metaRefreshInterval = harukiConfig.MetaRefreshInterval
	}
	metaOutputDir := harukiConfig.Cfg.PJSKRender.MusicMeta.OutputDir
	metaSource := harukiConfig.Cfg.PJSKRender.MusicMeta.Source
	metaBaseURL := harukiConfig.Cfg.PJSKRender.MusicMeta.BaseURL
	metaLoader := meta.NewLoader(
		harukiLogger.NewLoggerFromGlobal("MusicMeta"),
		meta.WithStore(cacheTargets.musicMeta),
		meta.WithSource(metaSource),
		meta.WithBaseURL(metaBaseURL),
	)
	if err := metaLoader.LoadAll(ctx); err != nil {
		mainLogger.Warn("music meta initial load partially failed", "error_type", fmt.Sprintf("%T", err))
	}
	metaLoader.StartBackgroundRefresh(ctx, metaRefreshInterval)
	mainLogger.Info("music meta loader started", "refresh_interval", metaRefreshInterval, "has_output_dir", strings.TrimSpace(metaOutputDir) != "", "persisted", cacheTargets.musicMeta != nil, "source", strings.TrimSpace(metaSource), "has_base_url", strings.TrimSpace(metaBaseURL) != "")

	sekaiAPIClient := sekaiAPI.NewSekaiAPIClient(&harukiConfig.Cfg.SekaiAPI)
	toolboxClient := sekaiAPI.NewToolboxClient(&harukiConfig.Cfg.Toolbox)
	trackerClient := sekaiAPI.NewTrackerClient(&harukiConfig.Cfg.Tracker)

	imageHosts, assetHosts, err := buildRenderHosts(harukiConfig.Cfg.PJSKRender, mainLogger)
	if errors.Is(err, errAssetHostsRequired) {
		fatalStartup(mainLogger, "asset_dirs.assets_base_urls is required", "error", err)
	} else if err != nil {
		fatalStartup(mainLogger, "public host configuration invalid", "error", err)
	}

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
		MusicMetaStore:     cacheTargets.musicMeta,
		MusicMetaSource:    metaSource,
		MusicMetaBaseURL:   metaBaseURL,
		MetaLoader:         metaLoader,
		SKForecast: renderapp.SKForecastConfig{
			LocalBaseURL: harukiConfig.Cfg.PJSKRender.SKForecast.LocalBaseURL,
			CacheStore:   cacheTargets.forecast,
			CacheKey:     cacheTargets.forecastKey,
		},
		MySekaiHousingCompetitionCacheStore:      cacheTargets.housing,
		MySekaiHousingCompetitionCacheKey:        cacheTargets.housingKey,
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
			RegistryURL:               strings.TrimSpace(harukiConfig.Cfg.PJSKRender.DeckRecommend.RegistryURL),
			Timeout:                   harukiConfig.Cfg.PJSKRender.DeckRecommend.Timeout,
			MaxRetries:                harukiConfig.Cfg.PJSKRender.DeckRecommend.MaxRetries,
			RetryWaitTime:             harukiConfig.Cfg.PJSKRender.DeckRecommend.RetryWaitTime,
			DefaultAlgs:               harukiConfig.Cfg.PJSKRender.DeckRecommend.DefaultAlgs,
		},
		Stores:     stores,
		ImageHosts: imageHosts,
		AssetHosts: assetHosts,
	})

	if runtime.Drawing == nil {
		mainLogger.Warn("PJSK render runtime initialized without drawing service", "build_only", true)
	}
	mainLogger.Info("PJSK render runtime initialized", "asset_root_count", len(runtime.AssetRoots()))
	return runtime
}

// buildRenderStores opens the five storage slots from pjsk_render.storage,
// deriving absent slots from the legacy directory settings. Nothing reads the
// stores yet; an invalid block still fails startup so misconfiguration is
// caught before a consumer is migrated.
func buildRenderStores(cfg harukiConfig.PJSKRenderConfig, log *harukiLogger.Logger) (storage.Set, error) {
	return storage.BuildSet(cfg.Storage, storage.LegacyRoots{
		AssetPrimary:  cfg.AssetDirs.Primary,
		CacheDir:      cfg.DrawingCache.StorageDir,
		ImageCacheDir: cfg.ImageCache.Dir,
	}, storage.Backends{S3: storages3.Open}, log)
}

// errAssetHostsRequired reports an empty public asset host set after the
// assets_base_url derivation (addendum B6 / C4): startup must fail instead of
// silently degrading asset messages to byte responses.
var errAssetHostsRequired = errors.New("asset_dirs.assets_base_urls (or assets_base_url) must name at least one public asset base URL")

// buildRenderHosts builds the per-node image-cache host set (image_cache.hosts,
// derived as {"default": uri} when empty) and the ordered public asset host set
// (asset_dirs.assets_base_urls, derived as [assets_base_url] when empty). The
// asset set is mandatory; the image set stays optional, and a legacy uri that
// is not an absolute http(s) URL only warns because nothing selects image
// hosts yet.
func buildRenderHosts(cfg harukiConfig.PJSKRenderConfig, log *harukiLogger.Logger) (*urlhost.Set, *urlhost.Set, error) {
	imageOpts := urlhost.Options{
		Order:         cfg.ImageCache.HostOrder,
		ProbePath:     strings.TrimSpace(cfg.ImageCache.HostsProbePath),
		ProbeInterval: cfg.ImageCache.HostsProbeInterval,
	}
	imageHosts, err := urlhost.New(cfg.ImageCache.Hosts, imageOpts)
	if err != nil {
		return nil, nil, fmt.Errorf("image_cache.hosts: %w", err)
	}
	if imageHosts.Len() == 0 {
		if uri := strings.TrimSpace(cfg.ImageCache.URI); uri != "" {
			derived, derr := urlhost.New(map[string]string{"default": uri}, imageOpts)
			if derr != nil {
				log.Warn("image_cache.uri is not an absolute http(s) URL; image host set left empty", "error", derr)
			} else {
				imageHosts = derived
			}
		}
	}

	assetURLs := cfg.AssetDirs.AssetsBaseURLs
	if len(assetURLs) == 0 && strings.TrimSpace(cfg.AssetDirs.AssetsBaseURL) != "" {
		assetURLs = []string{cfg.AssetDirs.AssetsBaseURL}
	}
	assetHosts, err := urlhost.FromList(assetURLs, urlhost.Options{})
	if err != nil {
		return nil, nil, fmt.Errorf("asset_dirs.assets_base_urls: %w", err)
	}
	if assetHosts.Len() == 0 {
		return nil, nil, errAssetHostsRequired
	}
	log.Info("public host sets configured", "image_hosts", imageHosts.Len(), "asset_hosts", assetHosts.Len(),
		"image_hosts_probing", imageOpts.ProbePath != "")
	return imageHosts, assetHosts, nil
}

// startRenderHostProbers starts the optional host probers with the run
// context; both are no-ops unless a probe path is configured.
func startRenderHostProbers(ctx context.Context, runtime *renderapp.App) {
	if runtime == nil {
		return
	}
	runtime.ImageHosts.Start(ctx)
	runtime.AssetHosts.Start(ctx)
}

func resolveDeckRecommendMasterdataDir() string {
	return strings.TrimSpace(harukiConfig.Cfg.PJSKRender.DeckRecommend.MasterdataDir)
}

// Keys of the Cloud-private cache objects on the cache slot.
const (
	skForecastCacheKey         storage.Key = "sk_forecast_cache.json"
	housingCompetitionCacheKey storage.Key = "mysekai_housing_competition_stats.json"
)

// renderCacheTargets names where each Cloud-private cache is persisted. A nil
// store means the cache is not persisted, exactly as when no path was set.
type renderCacheTargets struct {
	forecast    storage.Store
	forecastKey storage.Key
	housing     storage.Store
	housingKey  storage.Key
	musicMeta   storage.Store
}

// resolveRenderCacheTargets picks the store and key of each Cloud-private
// cache. An explicit per-feature path (sk_forecast.cache_path,
// mysekai_housing_competition.cache_path, music_meta.output_dir) keeps
// precedence and lands at exactly its old location through a local store; a
// configured storage.cache slot then only earns a Warn. Without an explicit
// path the forecast and housing caches use the cache slot, which itself
// derives from drawing_cache.storage_dir. music_metas were never persisted
// under drawing_cache.storage_dir, so they use the slot only when
// storage.cache is configured explicitly.
func resolveRenderCacheTargets(cfg harukiConfig.PJSKRenderConfig, stores storage.Set, log *harukiLogger.Logger) (renderCacheTargets, error) {
	slotConfigured := !cfg.Storage.Cache.IsZero()
	slot := enabledStore(stores.Cache)
	var targets renderCacheTargets
	var err error
	targets.forecast, targets.forecastKey, err = resolveCacheFile(cfg.SKForecast.CachePath, "sk_forecast.cache_path", slot, skForecastCacheKey, cfg.Storage.Cache, log)
	if err != nil {
		return renderCacheTargets{}, err
	}
	targets.housing, targets.housingKey, err = resolveCacheFile(cfg.MySekaiHousingCompetition.CachePath, "mysekai_housing_competition.cache_path", slot, housingCompetitionCacheKey, cfg.Storage.Cache, log)
	if err != nil {
		return renderCacheTargets{}, err
	}
	if dir := strings.TrimSpace(cfg.MusicMeta.OutputDir); dir != "" {
		warnExplicitCachePath(log, "music_meta.output_dir", dir, cfg.Storage.Cache)
		targets.musicMeta, err = storage.NewLocalAt(dir, 0)
		if err != nil {
			return renderCacheTargets{}, fmt.Errorf("music_meta.output_dir: %w", err)
		}
	} else if slotConfigured {
		targets.musicMeta = slot
	}
	return targets, nil
}

func resolveCacheFile(explicitPath, legacyKey string, slot storage.Store, slotKey storage.Key, slotCfg storage.ProviderConfig, log *harukiLogger.Logger) (storage.Store, storage.Key, error) {
	explicitPath = strings.TrimSpace(explicitPath)
	if explicitPath == "" {
		if slot == nil {
			return nil, "", nil
		}
		return slot, slotKey, nil
	}
	warnExplicitCachePath(log, legacyKey, explicitPath, slotCfg)
	store, err := storage.NewLocalAt(filepath.Dir(explicitPath), 0)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", legacyKey, err)
	}
	return store, storage.Key(filepath.Base(explicitPath)), nil
}

// warnExplicitCachePath logs the one startup Warn for an explicit legacy path
// that shadows a configured storage.cache slot.
func warnExplicitCachePath(log *harukiLogger.Logger, legacyKey, legacyPath string, slotCfg storage.ProviderConfig) {
	if slotCfg.IsZero() {
		return
	}
	log.Warn("storage slot root disagrees with legacy path",
		"slot", string(storage.SlotCache), "slot_root", strings.TrimSpace(slotCfg.Root),
		"legacy_key", legacyKey, "legacy_path", legacyPath)
}

// enabledStore returns store, or nil when it is absent or Disabled(), so a
// cache with nowhere to persist skips encoding entirely.
func enabledStore(store storage.Store) storage.Store {
	if store == nil || store == storage.Disabled() {
		return nil
	}
	return store
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

// initManifestSigner loads the online Ed25519 manifest signing key. A missing
// key is tolerated so local development works unsigned, but production logs a
// warning: AuthV3 clients are expected to verify manifests.
func initManifestSigner(mainLogger *harukiLogger.Logger) *trustsign.Signer {
	botCfg := harukiConfig.Cfg.HarukiBotDB
	seedHex := strings.TrimSpace(botCfg.ManifestSigningKey)
	if seedHex == "" {
		if harukiConfig.Cfg.Profile.IsProduction() {
			mainLogger.Warn("manifest signing key is not configured; command manifests are served unsigned")
		}
		return nil
	}
	keyID := strings.TrimSpace(botCfg.ManifestSigningKeyID)
	if keyID == "" {
		fatalStartup(mainLogger, "manifest_signing_key_id is required when manifest_signing_key is set")
	}
	signer, err := trustsign.NewSignerFromHex(keyID, seedHex)
	if err != nil {
		fatalStartup(mainLogger, "invalid manifest signing key", "error_type", fmt.Sprintf("%T", err))
	}
	mainLogger.Info("manifest signing enabled", "algorithm", trustsign.Algorithm, "key_id", keyID, "public_key", signer.PublicKeyHex())
	return signer
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

// initBuildPolicy loads the client build policy (release allowlist and
// revocations). Without a path the policy is off; with a path but no explicit
// mode it runs log-only so unlisted builds are measured before being refused.
func initBuildPolicy(mainLogger *harukiLogger.Logger) *buildpolicy.Store {
	botCfg := harukiConfig.Cfg.HarukiBotDB
	path := strings.TrimSpace(botCfg.BuildPolicyPath)
	mode, err := buildpolicy.ParseMode(botCfg.BuildPolicyMode, path != "")
	if err != nil {
		fatalStartup(mainLogger, "invalid build_policy_mode", "error_type", fmt.Sprintf("%T", err))
	}
	if mode == buildpolicy.ModeOff {
		if harukiConfig.Cfg.Profile.IsProduction() {
			mainLogger.Warn("client build policy is off; build_id is recorded but never enforced")
		}
		return nil
	}
	if path == "" {
		fatalStartup(mainLogger, "build_policy_path is required when build_policy_mode is not off")
	}
	var rootPub ed25519.PublicKey
	if pubHex := strings.TrimSpace(botCfg.BuildPolicyRootPublicKey); pubHex != "" {
		rootPub, err = trustsign.ParsePublicKeyHex(pubHex)
		if err != nil {
			fatalStartup(mainLogger, "invalid build_policy_root_public_key", "error_type", fmt.Sprintf("%T", err))
		}
	} else if harukiConfig.Cfg.Profile.IsProduction() {
		mainLogger.Warn("build policy is not signature-verified; set build_policy_root_public_key to pin the offline root")
	}
	store := buildpolicy.NewStore(path, mode, rootPub)
	if doc, err := store.Document(); err != nil {
		mainLogger.Warn("client build policy could not be loaded; logins are admitted fail-open until it is",
			"build_policy_path", path, "error_type", fmt.Sprintf("%T", err))
	} else {
		mainLogger.Info("client build policy loaded",
			"build_policy_mode", string(mode), "policy_version", doc.Version, "builds", len(doc.Builds),
			"revoked_versions", len(doc.RevokedVersions), "revoked_bots", len(doc.RevokedBots),
			"blocked_sources", len(doc.BlockedSources), "signature_verified", rootPub != nil)
	}
	return store
}

// sessionPolicyFor converts a possibly-nil store into the api.SessionPolicy
// interface without smuggling a typed nil through it.
func sessionPolicyFor(store *buildpolicy.Store) api.SessionPolicy {
	if store == nil {
		return nil
	}
	return store
}

// initSecurityMonitor wires the security event funnel to Redis-backed
// counters and the alert webhook.
func initSecurityMonitor(mainLogger *harukiLogger.Logger, redisClient *redis.Client) *secevent.Monitor {
	secCfg := harukiConfig.Cfg.Security
	var counter secevent.Counter
	if redisClient != nil {
		counter = redisSecurityCounter{rc: redisClient}
	}
	monitor := secevent.New(secevent.Config{
		WebhookURL: secCfg.AlertWebhookURL,
		Threshold:  secCfg.AlertThreshold,
		Window:     secCfg.AlertWindow,
		Node:       harukiConfig.Cfg.Node.Name,
	}, counter)
	if strings.TrimSpace(secCfg.AlertWebhookURL) == "" && harukiConfig.Cfg.Profile.IsProduction() {
		mainLogger.Warn("security alert webhook is not configured; alerts are logged only")
	}
	return monitor
}

type redisSecurityCounter struct{ rc *redis.Client }

func (c redisSecurityCounter) Incr(ctx context.Context, key string) (int64, error) {
	return c.rc.Incr(ctx, key).Result()
}

func (c redisSecurityCounter) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.rc.Expire(ctx, key, ttl).Err()
}
