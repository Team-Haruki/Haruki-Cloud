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

// privateDataClient reads private game-data snapshots with the conditional
// contract: a positive knownUploadTime lets upstream answer notModified=true
// instead of resending an unchanged payload (see
// sekai.GetPrivateDataConditionalContext, which also emulates the contract for
// Toolbox deployments without conditional read support).
type privateDataClient interface {
	GetSuiteDataConditionalContext(ctx context.Context, server string, userID int64, platform, platformUserID string, knownUploadTime int64) ([]byte, bool, error)
	GetMySekaiDataConditionalContext(ctx context.Context, server string, userID int64, platform, platformUserID string, knownUploadTime int64) ([]byte, bool, error)
}

type musicMetaSource interface {
	Get(region string) []byte
}

type conditionalPrivateDataFetcher func(knownUploadTime int64) ([]byte, bool, error)

type toolboxPrivateDataResult struct {
	data                 []byte
	requestCacheHit      bool
	crossRequestCacheHit bool
	elapsed              time.Duration
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
	builtCache   *BuiltSnapshotCache
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

// WithBuiltSnapshotCache attaches a process-wide memo of fully built snapshots,
// keyed by region + account + source upload_times, so warm renders of unchanged
// data skip factory.Build. A nil cache leaves every resolve rebuilding.
func (p *ToolboxSnapshotProvider) WithBuiltSnapshotCache(cache *BuiltSnapshotCache) *ToolboxSnapshotProvider {
	if p == nil {
		return nil
	}
	p.builtCache = cache
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
	binding, uid, err := p.resolveAccount(ctx, selector, opts, platform, imUserID, region)
	if err != nil {
		return nil, err
	}

	suiteResult, err := p.fetchPrivateData(ctx, binding.Server, "suite", uid, platform, imUserID, func(knownUploadTime int64) ([]byte, bool, error) {
		return p.client.GetSuiteDataConditionalContext(ctx, binding.Server, uid, platform, imUserID, knownUploadTime)
	})
	if err != nil {
		return nil, err
	}
	if len(suiteResult.data) == 0 {
		p.logEmptyPrivateData(ctx, binding.Server, "suite")
		return nil, fmt.Errorf("snapshot: suite snapshot is empty")
	}

	mysekaiJSON, err := p.resolveMySekaiData(ctx, binding.Server, uid, platform, imUserID, opts.NeedMySekai)
	if err != nil {
		return nil, err
	}
	snapshotRegion, musicMetaJSON := p.resolveSupplementalData(region, binding.Server, opts)
	return p.resolveBuiltSnapshot(ctx, tResolve, snapshotRegion, uid, suiteResult.data, mysekaiJSON, musicMetaJSON, opts)
}

func (p *ToolboxSnapshotProvider) resolveAccount(
	ctx context.Context,
	selector Selector,
	opts ResolveOptions,
	platform, imUserID string,
	region renderregion.Value,
) (*accountdata.ResolvedBinding, int64, error) {
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
		return nil, 0, err
	}
	p.logger.DebugContext(ctx, "toolbox snapshot binding selected",
		"upstream", "toolbox",
		"region", region.String(),
		"binding_region", binding.Server,
	)

	uid, err := strconv.ParseInt(binding.PJSKUserID, 10, 64)
	if err != nil {
		return nil, 0, fmt.Errorf("snapshot: invalid bound pjsk user id: %w", err)
	}
	return binding, uid, nil
}

func (p *ToolboxSnapshotProvider) fetchPrivateData(
	ctx context.Context,
	server, dataType string,
	uid int64,
	platform, imUserID string,
	fetch conditionalPrivateDataFetcher,
) (toolboxPrivateDataResult, error) {
	started := time.Now()
	result := toolboxPrivateDataResult{}
	data, err, requestCacheHit := cachedPrivateData(ctx, privateDataCacheKey{
		Server:         server,
		DataType:       dataType,
		UserID:         uid,
		Platform:       platform,
		PlatformUserID: imUserID,
	}, func() ([]byte, error) {
		data, cross, ferr := p.privateCache.Fetch(
			PrivateDataKey{Server: server, DataType: dataType, UID: uid},
			fetch,
		)
		result.crossRequestCacheHit = cross
		return data, ferr
	})
	result.data = data
	result.requestCacheHit = requestCacheHit
	result.elapsed = time.Since(started)
	if err != nil {
		p.logger.WarnContext(ctx, "toolbox private data fetch failed",
			"upstream", "toolbox",
			"data_type", dataType,
			"region", server,
			"duration_ms", commandtrace.Milliseconds(result.elapsed),
			"error_type", fmt.Sprintf("%T", err),
		)
		return toolboxPrivateDataResult{}, err
	}
	p.logger.DebugContext(ctx, "toolbox private data fetch completed",
		"upstream", "toolbox",
		"data_type", dataType,
		"region", server,
		"cache_hit", result.requestCacheHit,
		"cross_request_cache_hit", result.crossRequestCacheHit,
		"duration_ms", commandtrace.Milliseconds(result.elapsed),
		"response_bytes", len(result.data),
	)
	return result, nil
}

func (p *ToolboxSnapshotProvider) logEmptyPrivateData(ctx context.Context, server, dataType string) {
	p.logger.WarnContext(ctx, "toolbox private data fetch returned empty payload",
		"upstream", "toolbox",
		"data_type", dataType,
		"region", server,
	)
}

func (p *ToolboxSnapshotProvider) resolveMySekaiData(ctx context.Context, server string, uid int64, platform, imUserID string, needed bool) ([]byte, error) {
	if !needed {
		return nil, nil
	}
	result, err := p.fetchPrivateData(ctx, server, "mysekai", uid, platform, imUserID, func(knownUploadTime int64) ([]byte, bool, error) {
		return p.client.GetMySekaiDataConditionalContext(ctx, server, uid, platform, imUserID, knownUploadTime)
	})
	return result.data, err
}

func (p *ToolboxSnapshotProvider) resolveSupplementalData(region renderregion.Value, bindingServer string, opts ResolveOptions) (renderregion.Value, []byte) {
	metaRegion := region
	if bindingRegion := renderregion.Normalize(bindingServer); !bindingRegion.IsZero() {
		metaRegion = bindingRegion
	}
	if opts.NeedMusicMeta && p.metas != nil {
		return metaRegion, p.metas.Get(metaRegion.String())
	}
	return metaRegion, nil
}

func (p *ToolboxSnapshotProvider) resolveBuiltSnapshot(
	ctx context.Context,
	started time.Time,
	region renderregion.Value,
	uid int64,
	suiteJSON, mysekaiJSON, musicMetaJSON []byte,
	opts ResolveOptions,
) (Snapshot, error) {
	// Memoize the fully built snapshot across commands. The build is fully
	// determined by the region, account, and each source payload's upload_time
	// (parsed from the payloads already fetched above), so an unchanged account
	// reuses the parsed model — skipping the suite unmarshal, the leader-image
	// DB lookup, and the transforms inside factory.Build. Only memoize when music
	// meta is not folded in (the normal case) and every contributing upload_time
	// is known, so the key fully determines the built result.
	suiteUploadTime, _ := parseTopLevelUploadTime(suiteJSON)
	var mysekaiUploadTime int64
	if opts.NeedMySekai {
		mysekaiUploadTime, _ = parseTopLevelUploadTime(mysekaiJSON)
	}
	memoizable := !opts.NeedMusicMeta && suiteUploadTime > 0 && (!opts.NeedMySekai || mysekaiUploadTime > 0)
	memoKey := builtSnapshotKey{
		Region:            region.String(),
		UID:               uid,
		SuiteUploadTime:   suiteUploadTime,
		NeedMySekai:       opts.NeedMySekai,
		MySekaiUploadTime: mysekaiUploadTime,
	}
	if memoizable {
		if cached := p.builtCache.Get(memoKey); cached != nil {
			p.logger.DebugContext(ctx, "toolbox snapshot resolved",
				"upstream", "toolbox",
				"region", region.String(),
				"built_cache_hit", true,
				"duration_ms", commandtrace.Milliseconds(time.Since(started)),
			)
			return cached, nil
		}
	}

	snapshot, err := p.factory.Build(ctx, BuildInput{
		Region:        region,
		Source:        "toolbox_live",
		SuiteJSON:     suiteJSON,
		MySekaiJSON:   mysekaiJSON,
		MusicMetaJSON: musicMetaJSON,
	})
	if err != nil {
		p.logger.WarnContext(ctx, "toolbox snapshot build failed",
			"upstream", "toolbox",
			"region", region.String(),
			"suite_bytes", len(suiteJSON),
			"mysekai_bytes", len(mysekaiJSON),
			"error_type", fmt.Sprintf("%T", err),
		)
		return nil, err
	}
	if memoizable {
		p.builtCache.Put(memoKey, snapshot, int64(len(suiteJSON)+len(mysekaiJSON)))
	}
	p.logger.DebugContext(ctx, "toolbox snapshot resolved",
		"upstream", "toolbox",
		"region", region.String(),
		"built_cache_hit", false,
		"duration_ms", commandtrace.Milliseconds(time.Since(started)),
	)
	return snapshot, nil
}
