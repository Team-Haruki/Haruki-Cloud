package snapshot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/accountdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/utils/logger"
)

type bindingLookup interface {
	ResolveUserBinding(ctx context.Context, platform, platformUserID, server string) (int, *accountdata.ResolvedBinding, error)
	List(ctx context.Context, platform, platformUserID string) ([]accountdata.BindingListItem, error)
}

type privateDataClient interface {
	GetSuiteDataContext(ctx context.Context, server string, userID int64, platform, platformUserID string) ([]byte, error)
	GetMySekaiDataContext(ctx context.Context, server string, userID int64, platform, platformUserID string) ([]byte, error)
	GetSuiteUploadTimeContext(ctx context.Context, server string, userID int64, platform, platformUserID string) (int64, error)
	GetMySekaiUploadTimeContext(ctx context.Context, server string, userID int64, platform, platformUserID string) (int64, error)
}

type musicMetaSource interface {
	Get(region string) []byte
}

// ToolboxSnapshotProvider is the current request-scoped provider implementation.
// It keeps the existing Toolbox live-data path behind a stable provider
// contract, so future DB-backed snapshot stores can replace it without forcing
// controller and bridge layers to change again.
type ToolboxSnapshotProvider struct {
	bindings     bindingLookup
	client       privateDataClient
	factory      HarukiSnapshotFactory
	metas        musicMetaSource
	privateCache *PrivateDataCache
	logger       *logger.Logger
}

func NewToolboxSnapshotProvider(bindings bindingLookup, client privateDataClient, sekai *sekaiDB.Client, assetHelper *assets.AssetHelper) *ToolboxSnapshotProvider {
	return &ToolboxSnapshotProvider{
		bindings: bindings,
		client:   client,
		factory:  NewDefaultSnapshotFactory(sekai, assetHelper),
		logger:   logger.NewLoggerFromGlobal("ToolboxSnapshot"),
	}
}

func (p *ToolboxSnapshotProvider) WithMusicMetaSource(source musicMetaSource) *ToolboxSnapshotProvider {
	if p == nil {
		return nil
	}
	p.metas = source
	return p
}

// WithPrivateDataCache attaches a process-wide, upload_time-validated cache of
// Toolbox private-data payloads shared across bot commands. A nil cache leaves
// the provider fetching every payload directly (the pre-cache behavior).
func (p *ToolboxSnapshotProvider) WithPrivateDataCache(cache *PrivateDataCache) *ToolboxSnapshotProvider {
	if p == nil {
		return nil
	}
	p.privateCache = cache
	return p
}

func (p *ToolboxSnapshotProvider) Resolve(ctx context.Context, selector Selector, opts ResolveOptions) (Snapshot, error) {
	if p == nil || p.bindings == nil || p.client == nil || p.factory == nil {
		return nil, ErrProviderUnavailable
	}

	platform := strings.TrimSpace(selector.IMPlatform)
	imUserID := strings.TrimSpace(selector.IMUserID)
	if platform == "" || imUserID == "" {
		return nil, fmt.Errorf("snapshot: snapshot selector is incomplete")
	}

	tResolve := time.Now()
	region := renderregion.WithDefault(selector.Region)
	finishBinding := commandtrace.MeasureOperation(ctx, "snapshot.binding")
	binding, err := resolveSnapshotBinding(ctx, p.bindings, platform, imUserID, region, selector.PJSKUserID, opts)
	finishBinding()
	if err != nil {
		p.logger.WarnContext(ctx, "toolbox snapshot binding failed",
			"upstream", "toolbox",
			"region", region.String(),
			"need_mysekai", opts.NeedMySekai,
			"error_type", fmt.Sprintf("%T", err),
		)
		return nil, err
	}
	p.logger.DebugContext(ctx, "toolbox snapshot binding selected",
		"upstream", "toolbox",
		"region", region.String(),
		"binding_region", binding.Server,
	)

	uid, err := strconv.ParseInt(binding.PJSKUserID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("snapshot: invalid bound pjsk user id: %w", err)
	}

	tSuite := time.Now()
	var suiteCacheHit, suiteCrossHit bool
	suiteJSON, err, suiteCacheHit := cachedPrivateData(ctx, privateDataCacheKey{
		Server:         binding.Server,
		DataType:       "suite",
		UserID:         uid,
		Platform:       platform,
		PlatformUserID: imUserID,
	}, func() ([]byte, error) {
		data, cross, ferr := p.privateCache.Fetch(
			PrivateDataKey{Server: binding.Server, DataType: "suite", UID: uid},
			func() (int64, error) {
				return p.client.GetSuiteUploadTimeContext(ctx, binding.Server, uid, platform, imUserID)
			},
			func() ([]byte, error) {
				return p.client.GetSuiteDataContext(ctx, binding.Server, uid, platform, imUserID)
			},
		)
		suiteCrossHit = cross
		return data, ferr
	})
	suiteElapsed := time.Since(tSuite)
	if err != nil {
		p.logger.WarnContext(ctx, "toolbox private data fetch failed",
			"upstream", "toolbox",
			"data_type", "suite",
			"region", binding.Server,
			"duration_ms", commandtrace.Milliseconds(suiteElapsed),
			"error_type", fmt.Sprintf("%T", err),
		)
		return nil, err
	}
	if len(suiteJSON) == 0 {
		p.logger.WarnContext(ctx, "toolbox private data fetch returned empty payload",
			"upstream", "toolbox",
			"data_type", "suite",
			"region", binding.Server,
		)
		return nil, fmt.Errorf("snapshot: suite snapshot is empty")
	}
	p.logger.DebugContext(ctx, "toolbox private data fetch completed",
		"upstream", "toolbox",
		"data_type", "suite",
		"region", binding.Server,
		"cache_hit", suiteCacheHit,
		"cross_request_cache_hit", suiteCrossHit,
		"duration_ms", commandtrace.Milliseconds(suiteElapsed),
		"response_bytes", len(suiteJSON),
	)

	var mysekaiJSON []byte
	if opts.NeedMySekai {
		tMysekai := time.Now()
		var mysekaiCacheHit, mysekaiCrossHit bool
		mysekaiJSON, err, mysekaiCacheHit = cachedPrivateData(ctx, privateDataCacheKey{
			Server:         binding.Server,
			DataType:       "mysekai",
			UserID:         uid,
			Platform:       platform,
			PlatformUserID: imUserID,
		}, func() ([]byte, error) {
			data, cross, ferr := p.privateCache.Fetch(
				PrivateDataKey{Server: binding.Server, DataType: "mysekai", UID: uid},
				func() (int64, error) {
					return p.client.GetMySekaiUploadTimeContext(ctx, binding.Server, uid, platform, imUserID)
				},
				func() ([]byte, error) {
					return p.client.GetMySekaiDataContext(ctx, binding.Server, uid, platform, imUserID)
				},
			)
			mysekaiCrossHit = cross
			return data, ferr
		})
		mysekaiElapsed := time.Since(tMysekai)
		if err != nil {
			p.logger.WarnContext(ctx, "toolbox private data fetch failed",
				"upstream", "toolbox",
				"data_type", "mysekai",
				"region", binding.Server,
				"duration_ms", commandtrace.Milliseconds(mysekaiElapsed),
				"error_type", fmt.Sprintf("%T", err),
			)
			return nil, err
		}
		p.logger.DebugContext(ctx, "toolbox private data fetch completed",
			"upstream", "toolbox",
			"data_type", "mysekai",
			"region", binding.Server,
			"cache_hit", mysekaiCacheHit,
			"cross_request_cache_hit", mysekaiCrossHit,
			"duration_ms", commandtrace.Milliseconds(mysekaiElapsed),
			"response_bytes", len(mysekaiJSON),
		)
	}

	metaRegion := region
	if bindingRegion := renderregion.Normalize(binding.Server); !bindingRegion.IsZero() {
		metaRegion = bindingRegion
	}
	snapshotRegion := metaRegion
	var musicMetaJSON []byte
	if opts.NeedMusicMeta && p.metas != nil {
		musicMetaJSON = p.metas.Get(metaRegion.String())
	}

	snapshot, err := p.factory.Build(ctx, BuildInput{
		Region:        snapshotRegion,
		Source:        "toolbox_live",
		SuiteJSON:     suiteJSON,
		MySekaiJSON:   mysekaiJSON,
		MusicMetaJSON: musicMetaJSON,
	})
	if err != nil {
		p.logger.WarnContext(ctx, "toolbox snapshot build failed",
			"upstream", "toolbox",
			"region", snapshotRegion.String(),
			"suite_bytes", len(suiteJSON),
			"mysekai_bytes", len(mysekaiJSON),
			"error_type", fmt.Sprintf("%T", err),
		)
		return nil, err
	}
	p.logger.DebugContext(ctx, "toolbox snapshot resolved",
		"upstream", "toolbox",
		"region", snapshotRegion.String(),
		"duration_ms", commandtrace.Milliseconds(time.Since(tResolve)),
	)
	return snapshot, nil
}
