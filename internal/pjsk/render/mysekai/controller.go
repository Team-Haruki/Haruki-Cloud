package mysekai

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

const mysekaiMasterdataResolveTimeout = 10 * time.Second

type masterdataResolveFlightToken byte

type masterdataResolveFlightResult struct {
	source     masterdataSource
	operations []commandtrace.Stats
	leader     *masterdataResolveFlightToken
}

var (
	mysekaiMapSiteOrder   = []int{5, 6, 7, 8}
	mysekaiMapSiteConfigs = map[int]mysekaiMapSiteConfig{
		5: {
			SiteImageName: "grassland",
			GridSize:      33.333,
			OffsetX:       0,
			OffsetZ:       -60,
			DirX:          -1,
			DirZ:          -1,
			RevXZ:         true,
			CropBBox:      []int{300, 0, 1280, 1080},
		},
		6: {
			SiteImageName: "beach",
			GridSize:      20.513,
			OffsetX:       0,
			OffsetZ:       80,
			DirX:          1,
			DirZ:          -1,
			RevXZ:         false,
			CropBBox:      []int{300, 0, 1280, 1080},
		},
		7: {
			SiteImageName: "flowergarden",
			GridSize:      24.806,
			OffsetX:       -62.015,
			OffsetZ:       20.672,
			DirX:          -1,
			DirZ:          -1,
			RevXZ:         true,
			CropBBox:      []int{350, 0, 1280, 1080},
		},
		8: {
			SiteImageName: "memorialplace",
			GridSize:      21.333,
			OffsetX:       0,
			OffsetZ:       -130,
			DirX:          1,
			DirZ:          -1,
			RevXZ:         false,
			CropBBox:      []int{200, 0, 1280, 1080},
		},
	}
)

func newMasterdataResolver(opts MasterdataOptions) *masterdataResolver {
	return &masterdataResolver{
		dsn:           strings.TrimSpace(opts.SekaiDSN),
		localDir:      cleanMasterdataDir(opts.LocalDir),
		allowFallback: opts.AllowFallback,
		cache:         make(map[string]masterdataSource),
	}
}

func cleanMasterdataDir(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	return filepath.Clean(dir)
}

func (r *masterdataResolver) Resolve(region renderregion.Value) masterdataSource {
	return r.ResolveContext(context.Background(), region)
}

func (r *masterdataResolver) ResolveContext(ctx context.Context, region renderregion.Value) masterdataSource {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	resolved := renderregion.WithDefault(region)
	key := resolved.String()

	r.mu.RLock()
	if cached, ok := r.cache[key]; ok {
		r.mu.RUnlock()
		return bindMasterdataContext(cached, ctx)
	}
	r.mu.RUnlock()

	callerToken := new(masterdataResolveFlightToken)
	resultCh := r.loads.DoChan(key, func() (any, error) {
		sharedBase, cancel := context.WithTimeout(context.Background(), mysekaiMasterdataResolveTimeout)
		defer cancel()
		sharedCtx, trace := commandtrace.WithNewTrace(sharedBase)
		complete := func(source masterdataSource) masterdataResolveFlightResult {
			return masterdataResolveFlightResult{
				source:     source,
				operations: trace.Snapshot().Operations,
				leader:     callerToken,
			}
		}

		r.mu.RLock()
		cached, ok := r.cache[key]
		r.mu.RUnlock()
		if ok {
			return complete(cached), nil
		}

		store := r.build(sharedCtx, resolved)
		if store == nil {
			return complete(nil), nil
		}
		// The resolver is process-scoped; never retain a request context in its
		// regional cache. Request clones below still carry the active command ctx.
		cached = bindMasterdataContext(store, context.Background())
		r.mu.Lock()
		if existing, exists := r.cache[key]; exists {
			cached = existing
		} else {
			r.cache[key] = cached
		}
		r.mu.Unlock()
		return complete(cached), nil
	})

	finishWait := commandtrace.MeasureOperation(ctx, "mysekai.masterdata_resolve_wait")
	var result masterdataResolveFlightResult
	select {
	case <-ctx.Done():
		finishWait()
		return nil
	case outcome := <-resultCh:
		finishWait()
		if outcome.Err != nil {
			return nil
		}
		var ok bool
		result, ok = outcome.Val.(masterdataResolveFlightResult)
		if !ok {
			return nil
		}
	}
	commandtrace.MergeOperations(ctx, result.operations)
	if result.leader != callerToken {
		commandtrace.RecordOperation(ctx, "mysekai.masterdata_resolve_shared", 0)
	}
	return bindMasterdataContext(result.source, ctx)
}

func (r *masterdataResolver) build(ctx context.Context, region renderregion.Value) masterdataSource {
	if r == nil {
		return nil
	}

	if r.dsn != "" {
		if store := newDBMasterdataStore(ctx, r.dsn, region.String()); store != nil && store.Configured() {
			return store
		}
	}

	if r.allowFallback {
		regionDir := ""
		if r.localDir != "" {
			regionDir = filepath.Join(r.localDir, region.String())
		}
		if store := newLocalMasterdataStore(regionDir, r.localDir); store != nil && store.Configured() {
			return store
		}
	}

	return nil
}

// NewController creates a mysekai Controller. If SekaiDSN is non-empty the
// controller queries the sekai database for masterdata. AllowFallback is a
// legacy/dev escape hatch for reading JSON files from LocalDir; production
// should keep it disabled so DB gaps are visible instead of hidden by files.
func NewController(drawingClient *drawing.HarukiDrawingClient, snapshot snapshot.Snapshot, defaultRegion renderregion.Value, assetHelper *assets.AssetHelper, mdOpts MasterdataOptions) *Controller {
	region := renderregion.WithDefault(defaultRegion)
	resolver := newMasterdataResolver(mdOpts)
	md := resolver.Resolve(region)
	return &Controller{
		drawing:       drawingClient,
		snapshot:      snapshot,
		masterdata:    md,
		resolver:      resolver,
		defaultRegion: region,
		nicknames:     cloneNicknames(defaultNicknames),
		assets:        assetHelper,
		housingCompetitionStats: newHousingCompetitionStatsCache(
			mdOpts.HousingCompetitionStatsCachePath,
			mdOpts.HousingCompetitionRefreshInterval,
		),
		housingCompetitionBanners: newHousingCompetitionBannerCache(
			defaultHousingCompetitionBannerCacheDir(mdOpts.HousingCompetitionStatsCachePath),
			assetHelper,
			mdOpts.AssetsBaseURL,
		),
	}
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	clone := *c
	clone.requestCtx = ctx
	clone.drawing = c.drawing.WithContext(ctx)
	clone.assets = c.assets.WithContext(ctx)
	clone.masterdata = bindMasterdataContext(c.masterdata, ctx)
	return &clone
}

func bindMasterdataContext(source masterdataSource, ctx context.Context) masterdataSource {
	if contextual, ok := source.(contextualMasterdataSource); ok {
		return contextual.WithContext(ctx)
	}
	return source
}

// regionPath resolves a region-specific asset path through the AssetHelper.
// For paths starting with "mysekai/", "event/", "gacha/" the ondemand mode is
// tried first; for others startapp is tried first.
func (c *Controller) regionPath(region renderregion.Value, relPath string) string {
	return assets.ResolveRegionAssetPath(c.assets, region.String(), relPath)
}

// staticPath resolves a path under the Drawing API's static_images directory.
func (c *Controller) staticPath(relPath string) string {
	relPath = strings.TrimSpace(strings.TrimPrefix(relPath, "/"))
	if relPath == "" {
		return ""
	}

	resolved := filepath.ToSlash(strings.TrimSpace(assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, relPath)))
	if resolved == "" {
		return filepath.ToSlash(filepath.Join(assets.StaticImagesDir, relPath))
	}
	if strings.HasPrefix(resolved, "http://") || strings.HasPrefix(resolved, "https://") {
		return resolved
	}
	if strings.HasPrefix(resolved, assets.StaticImagesDir+"/") {
		return resolved
	}

	if c.assets != nil {
		for _, root := range c.assets.Roots() {
			root = strings.TrimSpace(root)
			if root == "" {
				continue
			}
			if strings.HasPrefix(root, "http://") || strings.HasPrefix(root, "https://") {
				continue
			}

			rel := filepath.ToSlash(strings.TrimPrefix(assets.MakeRelative(root, resolved), "./"))
			if strings.HasPrefix(rel, assets.StaticImagesDir+"/") {
				return rel
			}

			// Local roots may point directly at "static_images". Normalize those
			// back to "static_images/..." payload paths.
			base := filepath.Base(filepath.ToSlash(root))
			if rel != resolved && rel != "" && base == assets.StaticImagesDir {
				return filepath.ToSlash(filepath.Join(assets.StaticImagesDir, strings.TrimPrefix(rel, "/")))
			}
		}
	}

	// Never return local absolute filesystem paths to Drawing API. Cloud and
	// Drawing may run in different containers, so absolute paths here are often
	// unreadable on the Drawing side.
	if filepath.IsAbs(resolved) {
		return filepath.ToSlash(filepath.Join(assets.StaticImagesDir, relPath))
	}

	return resolved
}

// WithSnapshot returns a shallow copy of this Controller that uses the given
// snapshot instead of the one configured at construction time. This is used by
// the bridge layer to inject a live Toolbox snapshot on a per-request basis.
func (c *Controller) WithSnapshot(s snapshot.Snapshot) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.snapshot = s
	return &clone
}

// WithMySekaiData returns a shallow copy that uses raw mysekai JSON bytes
// directly, without going through the snapshot.Service merge path. This is
// the preferred injection for mysekai-only commands: suite data is not needed
// because the profile card comes from the public API via query.Profile.
func (c *Controller) WithMySekaiData(data []byte) *Controller {
	if c == nil || len(data) == 0 {
		return nil
	}
	clone := *c
	clone.rawMySekaiJSON = data
	return &clone
}

func (c *Controller) withRegion(region string) *Controller {
	if c == nil {
		return nil
	}

	clone := *c
	resolved := c.resolveRegion(region)
	clone.defaultRegion = resolved
	if c.resolver != nil {
		clone.masterdata = c.resolver.ResolveContext(c.requestCtx, resolved)
	}
	return &clone
}

func (c *Controller) Close() {
	if c == nil || c.resolver == nil {
		return
	}
	c.resolver.Close()
}

func (r *masterdataResolver) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, source := range r.cache {
		if closer, ok := source.(interface{ Close() }); ok {
			closer.Close()
		}
	}
	r.cache = make(map[string]masterdataSource)
}
