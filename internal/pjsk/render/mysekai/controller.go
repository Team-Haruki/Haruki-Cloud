package mysekai

import (
	"path/filepath"
	"strings"
	"sync"

	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
)

type Controller struct {
	drawing        *drawing.HarukiDrawingClient
	snapshot       userdata.Snapshot
	rawMySekaiJSON []byte // direct mysekai JSON (bypasses snapshot merge)
	masterdata     masterdataSource
	resolver       *masterdataResolver
	defaultRegion  renderregion.Value
	nicknames      map[string]int
	assets         *assets.AssetHelper
}

type masterdataResolver struct {
	dsn           string
	localDir      string
	allowFallback bool

	mu    sync.Mutex
	cache map[string]masterdataSource
}

type mysekaiMapSiteConfig struct {
	SiteImageName string
	GridSize      float64
	OffsetX       float64
	OffsetZ       float64
	DirX          float64
	DirZ          float64
	RevXZ         bool
	CropBBox      []int
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

// MasterdataOptions configures the masterdata source for NewController.
type MasterdataOptions struct {
	SekaiDSN      string
	LocalDir      string
	AllowFallback bool // when false, DB failure is fatal; when true, fallback to local files
}

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
	if r == nil {
		return nil
	}

	resolved := renderregion.WithDefault(region)
	key := resolved.String()

	r.mu.Lock()
	defer r.mu.Unlock()

	if cached, ok := r.cache[key]; ok {
		return cached
	}

	store := r.build(resolved)
	r.cache[key] = store
	return store
}

func (r *masterdataResolver) build(region renderregion.Value) masterdataSource {
	if r == nil {
		return nil
	}

	if r.dsn != "" {
		if store := newDBMasterdataStore(r.dsn, region.String()); store != nil && store.Configured() {
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
// controller queries the sekai database for masterdata. When AllowFallback is
// true and the DB is unavailable, it falls back to reading JSON files from
// LocalDir. In production (AllowFallback=false) a DB failure leaves masterdata
// nil so callers get a clear error.
func NewController(drawingClient *drawing.HarukiDrawingClient, snapshot userdata.Snapshot, defaultRegion renderregion.Value, assetHelper *assets.AssetHelper, mdOpts MasterdataOptions) *Controller {
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
	}
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
func (c *Controller) WithSnapshot(s userdata.Snapshot) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.snapshot = s
	return &clone
}

// WithMySekaiData returns a shallow copy that uses raw mysekai JSON bytes
// directly, without going through the userdata.Service merge path. This is
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
		clone.masterdata = c.resolver.Resolve(resolved)
	}
	return &clone
}
