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
	GetSuiteDataFieldsConditionalContext(ctx context.Context, server string, userID int64, platform, platformUserID string, knownUploadTime int64, fields []string) ([]byte, bool, error)
	GetMySekaiDataConditionalContext(ctx context.Context, server string, userID int64, platform, platformUserID string, knownUploadTime int64) ([]byte, bool, error)
}

type musicMetaSource interface {
	Get(region string) []byte
}

type conditionalPrivateDataFetcher func(knownUploadTime int64) ([]byte, bool, error)

type toolboxPrivateDataResult struct {
	privateDataPayload
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

	fields, err := normalizeSuiteFields(opts.SuiteFields)
	if err != nil {
		return nil, err
	}
	opts.SuiteFields = fields
	projection := strings.Join(fields, ",")

	tResolve := time.Now()
	region := renderregion.WithDefault(selector.Region)
	binding, uid, err := p.resolveAccount(ctx, selector, opts, platform, imUserID, region)
	if err != nil {
		return nil, err
	}

	snapshotRegion := snapshotRegionForBinding(region, binding.Server)
	built, hasBuilt := p.revalidationCandidate(snapshotRegion, uid, projection, opts)
	fetchSuite := func(knownUploadTime int64) ([]byte, bool, error) {
		if len(fields) > 0 {
			return p.client.GetSuiteDataFieldsConditionalContext(ctx, binding.Server, uid, platform, imUserID, knownUploadTime, fields)
		}
		return p.client.GetSuiteDataConditionalContext(ctx, binding.Server, uid, platform, imUserID, knownUploadTime)
	}
	suiteRequest := privateDataRequest{server: binding.Server, dataType: "suite", uid: uid, platform: platform, imUserID: imUserID, projection: projection}
	if hasBuilt {
		suiteRequest.fallbackKnown = built.SuiteUploadTime
	}
	suiteResult, err := p.fetchPrivateData(ctx, suiteRequest, fetchSuite)
	if err != nil {
		return nil, err
	}
	if len(suiteResult.data) == 0 && !suiteResult.versionOnly {
		p.logEmptyPrivateData(ctx, binding.Server, "suite")
		return nil, fmt.Errorf("snapshot: suite snapshot is empty")
	}

	mysekaiFallback := int64(0)
	if hasBuilt && opts.NeedMySekai && suiteResult.uploadTime == built.SuiteUploadTime {
		mysekaiFallback = built.MySekaiUploadTime
	}
	mysekaiJSON, err := p.resolveMySekaiData(ctx, binding.Server, uid, platform, imUserID, opts.NeedMySekai, mysekaiFallback)
	if err != nil {
		return nil, err
	}
	if suiteResult.versionOnly || mysekaiJSON.versionOnly {
		if snapshot := p.revalidatedSnapshot(ctx, snapshotRegion, uid, suiteResult.privateDataPayload, mysekaiJSON, opts); snapshot != nil {
			return snapshot, nil
		}
		if suiteResult, mysekaiJSON, err = p.refetchVersionOnly(ctx, suiteRequest, fetchSuite, suiteResult, mysekaiJSON); err != nil {
			return nil, err
		}
	}
	snapshotRegion, musicMetaJSON := p.resolveSupplementalData(region, binding.Server, opts)
	return p.resolveBuiltSnapshot(ctx, tResolve, snapshotRegion, uid, suiteResult.privateDataPayload, mysekaiJSON, musicMetaJSON, opts)
}

// revalidationCandidate returns the newest memoized snapshot key for this
// account and field set, whose upload_times a raw-cache miss can revalidate
// instead of transferring a body the built snapshot already covers.
func (p *ToolboxSnapshotProvider) revalidationCandidate(region renderregion.Value, uid int64, projection string, opts ResolveOptions) (builtSnapshotKey, bool) {
	if opts.NeedMusicMeta {
		return builtSnapshotKey{}, false
	}
	return p.builtCache.latestKey(builtSnapshotIdentity{
		Region:          region.String(),
		UID:             uid,
		SuiteProjection: projection,
		NeedMySekai:     opts.NeedMySekai,
	})
}

// revalidatedSnapshot returns the built snapshot whose upload_times this
// request just confirmed. A versionOnly result can also come from this
// command's request cache for a resolve the built cache does not memoize
// (music meta), so that case never serves an entry.
func (p *ToolboxSnapshotProvider) revalidatedSnapshot(ctx context.Context, region renderregion.Value, uid int64, suite, mysekai privateDataPayload, opts ResolveOptions) Snapshot {
	if opts.NeedMusicMeta {
		return nil
	}
	snapshot := p.builtCache.Get(newBuiltSnapshotKey(region, uid, suite, mysekai, opts))
	if snapshot != nil {
		commandtrace.RecordOperation(ctx, "snapshot.built_cache_revalidated", 0)
	}
	return snapshot
}

// refetchVersionOnly replaces versionOnly results with full reads when no
// built entry can be served for them (it was evicted since the lookup).
func (p *ToolboxSnapshotProvider) refetchVersionOnly(
	ctx context.Context,
	suiteRequest privateDataRequest,
	fetchSuite conditionalPrivateDataFetcher,
	suite toolboxPrivateDataResult,
	mysekai privateDataPayload,
) (toolboxPrivateDataResult, privateDataPayload, error) {
	var err error
	if suite.versionOnly {
		if suite, err = p.refetchPrivateData(ctx, suiteRequest, fetchSuite); err != nil {
			return suite, mysekai, err
		}
		if len(suite.data) == 0 {
			p.logEmptyPrivateData(ctx, suiteRequest.server, "suite")
			return suite, mysekai, fmt.Errorf("snapshot: suite snapshot is empty")
		}
	}
	if mysekai.versionOnly {
		mysekai, err = p.refetchMySekaiData(ctx, suiteRequest.server, suiteRequest.uid, suiteRequest.platform, suiteRequest.imUserID)
	}
	return suite, mysekai, err
}

func newBuiltSnapshotKey(region renderregion.Value, uid int64, suite, mysekai privateDataPayload, opts ResolveOptions) builtSnapshotKey {
	return builtSnapshotKey{
		Region:            region.String(),
		UID:               uid,
		SuiteUploadTime:   suite.uploadTime,
		SuiteProjection:   strings.Join(opts.SuiteFields, ","),
		NeedMySekai:       opts.NeedMySekai,
		MySekaiUploadTime: mysekai.uploadTime,
	}
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

// privateDataRequest identifies one private-data read. fallbackKnown is the
// upload_time of a built snapshot to revalidate when the raw entry is gone.
type privateDataRequest struct {
	server, dataType   string
	uid                int64
	platform, imUserID string
	projection         string
	fallbackKnown      int64
}

func (p *ToolboxSnapshotProvider) fetchPrivateData(ctx context.Context, request privateDataRequest, fetch conditionalPrivateDataFetcher) (toolboxPrivateDataResult, error) {
	return p.fetchPrivateDataMode(ctx, request, false, fetch)
}

// refetchPrivateData replaces a versionOnly result with a full read.
func (p *ToolboxSnapshotProvider) refetchPrivateData(ctx context.Context, request privateDataRequest, fetch conditionalPrivateDataFetcher) (toolboxPrivateDataResult, error) {
	request.fallbackKnown = 0
	return p.fetchPrivateDataMode(ctx, request, true, fetch)
}

func (p *ToolboxSnapshotProvider) fetchPrivateDataMode(ctx context.Context, request privateDataRequest, refresh bool, fetch conditionalPrivateDataFetcher) (toolboxPrivateDataResult, error) {
	server, dataType := request.server, request.dataType
	started := time.Now()
	result := toolboxPrivateDataResult{}
	requestKey := privateDataCacheKey{
		Server:         server,
		DataType:       dataType,
		UserID:         request.uid,
		Platform:       request.platform,
		PlatformUserID: request.imUserID,
		Projection:     request.projection,
	}
	load := func() (privateDataPayload, error) {
		data, cross, ferr := p.privateCache.fetchPayloadWithFallback(
			ctx,
			PrivateDataKey{Server: server, DataType: dataType, UID: request.uid, Projection: request.projection},
			request.fallbackKnown,
			fetch,
		)
		result.crossRequestCacheHit = cross
		return data, ferr
	}
	var (
		data            privateDataPayload
		err             error
		requestCacheHit bool
	)
	if refresh {
		data, err = refreshPrivateData(ctx, requestKey, load)
	} else {
		data, err, requestCacheHit = cachedPrivateData(ctx, requestKey, load)
	}
	result.privateDataPayload = data
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

func (p *ToolboxSnapshotProvider) resolveMySekaiData(ctx context.Context, server string, uid int64, platform, imUserID string, needed bool, fallbackKnown int64) (privateDataPayload, error) {
	if !needed {
		return privateDataPayload{}, nil
	}
	request := privateDataRequest{server: server, dataType: "mysekai", uid: uid, platform: platform, imUserID: imUserID, fallbackKnown: fallbackKnown}
	result, err := p.fetchPrivateData(ctx, request, p.mySekaiFetcher(ctx, server, uid, platform, imUserID))
	return result.privateDataPayload, err
}

func (p *ToolboxSnapshotProvider) refetchMySekaiData(ctx context.Context, server string, uid int64, platform, imUserID string) (privateDataPayload, error) {
	request := privateDataRequest{server: server, dataType: "mysekai", uid: uid, platform: platform, imUserID: imUserID}
	result, err := p.refetchPrivateData(ctx, request, p.mySekaiFetcher(ctx, server, uid, platform, imUserID))
	return result.privateDataPayload, err
}

func (p *ToolboxSnapshotProvider) mySekaiFetcher(ctx context.Context, server string, uid int64, platform, imUserID string) conditionalPrivateDataFetcher {
	return func(knownUploadTime int64) ([]byte, bool, error) {
		return p.client.GetMySekaiDataConditionalContext(ctx, server, uid, platform, imUserID, knownUploadTime)
	}
}

func snapshotRegionForBinding(region renderregion.Value, bindingServer string) renderregion.Value {
	if bindingRegion := renderregion.Normalize(bindingServer); !bindingRegion.IsZero() {
		return bindingRegion
	}
	return region
}

func (p *ToolboxSnapshotProvider) resolveSupplementalData(region renderregion.Value, bindingServer string, opts ResolveOptions) (renderregion.Value, []byte) {
	metaRegion := snapshotRegionForBinding(region, bindingServer)
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
	suite, mysekai privateDataPayload,
	musicMetaJSON []byte,
	opts ResolveOptions,
) (Snapshot, error) {
	// Toolbox advances upload_time whenever the payload changes. Reuse the
	// version parsed at ingestion; every request has already authorized its read.
	memoizable := !opts.NeedMusicMeta && suite.uploadTime > 0 && (!opts.NeedMySekai || mysekai.uploadTime > 0)
	memoKey := newBuiltSnapshotKey(region, uid, suite, mysekai, opts)
	build := func(buildCtx context.Context) (Snapshot, error) {
		return p.factory.Build(buildCtx, BuildInput{
			Region:        region,
			Source:        "toolbox_live",
			SuiteJSON:     suite.data,
			MySekaiJSON:   mysekai.data,
			MusicMetaJSON: musicMetaJSON,
			// Private-data payloads are immutable once ingested, so the built
			// snapshot shares the cached bytes instead of holding a second copy.
			SuiteJSONImmutable: true,
		})
	}
	var (
		snapshot Snapshot
		err      error
		cacheHit bool
	)
	if memoizable {
		snapshot, cacheHit, err = p.builtCache.getOrBuild(ctx, memoKey, int64(len(suite.data)+len(mysekai.data)), build)
	} else {
		commandtrace.RecordOperation(ctx, "snapshot.built_cache_bypass", 0)
		snapshot, err = build(ctx)
	}
	if err != nil {
		p.logger.WarnContext(ctx, "toolbox snapshot build failed",
			"upstream", "toolbox",
			"region", region.String(),
			"suite_bytes", len(suite.data),
			"mysekai_bytes", len(mysekai.data),
			"error_type", fmt.Sprintf("%T", err),
		)
		return nil, err
	}
	p.logger.DebugContext(ctx, "toolbox snapshot resolved",
		"upstream", "toolbox",
		"region", region.String(),
		"built_cache_hit", cacheHit,
		"duration_ms", commandtrace.Milliseconds(time.Since(started)),
	)
	return snapshot, nil
}
