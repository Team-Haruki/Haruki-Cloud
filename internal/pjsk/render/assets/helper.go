package assets

import (
	"container/list"
	"context"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/sync/singleflight"

	"haruki-cloud/internal/observability/commandtrace"
)

// AssetHelper resolves assets against a primary directory with legacy fallbacks.
type AssetHelper struct {
	roots           []string
	ctx             context.Context
	fs              assetFileSystem
	directoryCache  *assetDirectoryCache
	resolutionCache *assetResolutionCache
}

type assetFileSystem interface {
	Stat(name string) (fs.FileInfo, error)
	ReadDir(name string) ([]fs.DirEntry, error)
}

type osAssetFileSystem struct{}

func (osAssetFileSystem) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (osAssetFileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

type assetDirectoryCache struct {
	mu         sync.Mutex
	entries    map[string]*assetDirectoryCacheEntry
	recent     list.List
	indexed    int
	maxEntries int
	maxNames   int
	loads      singleflight.Group
}

type assetDirectoryCacheEntry struct {
	index   *assetDirectoryIndex
	element *list.Element
}

type assetDirectoryIndex struct {
	modTime time.Time
	exact   map[string]string
	folded  map[string]string
}

const (
	assetDirectoryMaxEntries  = 4_096
	assetDirectoryMaxNames    = 262_144
	assetResolutionTTL        = 30 * time.Second
	assetResolutionMaxEntries = 65_536
)

type assetResolutionCache struct {
	mu         sync.Mutex
	entries    map[string]*assetResolutionEntry
	recent     list.List
	loads      singleflight.Group
	ttl        time.Duration
	maxEntries int
	generation uint64
}

type assetResolutionEntry struct {
	path      string
	expiresAt time.Time
	element   *list.Element
}

type assetResolutionFlightResult struct {
	path       string
	operations []commandtrace.Stats
}

type assetDirectoryFlightResult struct {
	index      *assetDirectoryIndex
	err        error
	operations []commandtrace.Stats
}

func NewAssetHelper(primary string, legacy []string) *AssetHelper {
	var roots []string
	seen := make(map[string]struct{})

	appendRoot := func(path string) {
		clean := strings.TrimSpace(path)
		if clean == "" {
			return
		}
		clean = normalizeAssetRoot(clean)
		if clean == "." {
			return
		}
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		roots = append(roots, clean)
	}

	appendRoot(primary)
	for _, dir := range legacy {
		appendRoot(dir)
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}

	return &AssetHelper{
		roots: roots,
		fs:    osAssetFileSystem{},
		directoryCache: &assetDirectoryCache{
			entries:    make(map[string]*assetDirectoryCacheEntry),
			maxEntries: assetDirectoryMaxEntries,
			maxNames:   assetDirectoryMaxNames,
		},
		resolutionCache: &assetResolutionCache{
			entries:    make(map[string]*assetResolutionEntry),
			ttl:        assetResolutionTTL,
			maxEntries: assetResolutionMaxEntries,
		},
	}
}

// WithContext returns a shallow copy that records filesystem operations in the
// command trace carried by ctx. The directory cache remains shared by all copies.
func (h *AssetHelper) WithContext(ctx context.Context) *AssetHelper {
	if h == nil {
		return nil
	}
	clone := *h
	clone.ctx = ctx
	return &clone
}

func (h *AssetHelper) Roots() []string {
	out := make([]string, len(h.roots))
	copy(out, h.roots)
	return out
}

func (h *AssetHelper) Primary() string {
	if len(h.roots) == 0 {
		return ""
	}
	return h.roots[0]
}

func (h *AssetHelper) Join(parts ...string) string {
	if len(h.roots) == 0 {
		return ""
	}
	return joinAssetPath(h.Primary(), parts...)
}

// FirstExisting returns the first candidate path that exists on disk. For each
// relative candidate it checks exact paths in every root before trying the
// case-insensitive fallback. Consequently, an exact match in a legacy root takes
// precedence over a differently-cased match in an earlier root.
func (h *AssetHelper) FirstExisting(relPaths ...string) string {
	key := assetResolutionKey(relPaths)
	if key == "" {
		return ""
	}
	cache := h.resolutions()
	if resolved, ok := cache.lookup(key, time.Now()); ok {
		commandtrace.RecordOperation(h.ctx, "asset.resolve_cache_hit", 0)
		return resolved
	}
	commandtrace.RecordOperation(h.ctx, "asset.resolve_cache_miss", 0)
	finishWait := commandtrace.MeasureOperation(h.ctx, "asset.resolve_wait")
	detachedHelper := h.WithContext(context.Background())
	generation := cache.currentGeneration()
	flightKey := key + "\x00generation=" + strconv.FormatUint(generation, 10)
	value, _, shared := cache.loads.Do(flightKey, func() (any, error) {
		sharedCtx, trace := commandtrace.WithNewTrace(context.Background())
		sharedHelper := detachedHelper.WithContext(sharedCtx)
		if resolved, ok := cache.lookup(key, time.Now()); ok {
			return assetResolutionFlightResult{path: resolved, operations: trace.Snapshot().Operations}, nil
		}
		resolved := sharedHelper.firstExistingUncached(relPaths...)
		cache.storeForGeneration(key, resolved, time.Now(), generation)
		return assetResolutionFlightResult{path: resolved, operations: trace.Snapshot().Operations}, nil
	})
	finishWait()
	if shared {
		commandtrace.RecordOperation(h.ctx, "asset.resolve_shared", 0)
	}
	result, _ := value.(assetResolutionFlightResult)
	commandtrace.MergeOperations(h.ctx, result.operations)
	return result.path
}

func (h *AssetHelper) firstExistingUncached(relPaths ...string) string {
	for _, rel := range relPaths {
		if strings.TrimSpace(rel) == "" {
			continue
		}
		for _, candidateRel := range assetPathCandidates(rel) {
			candidates := h.localCandidatePaths(candidateRel)
			for _, candidate := range candidates {
				if _, err := h.stat(candidate); err == nil {
					return filepath.ToSlash(candidate)
				}
			}
			for _, candidate := range candidates {
				if resolved, ok := h.resolveCaseInsensitivePath(candidate); ok {
					return filepath.ToSlash(resolved)
				}
			}
		}
	}
	return ""
}

func assetResolutionKey(relPaths []string) string {
	parts := make([]string, 0, len(relPaths))
	for _, rel := range relPaths {
		if clean := filepath.ToSlash(strings.TrimSpace(rel)); clean != "" {
			parts = append(parts, clean)
		}
	}
	return strings.Join(parts, "\x00")
}

// ClearResolutionCache makes external asset updates visible immediately.
// Without an explicit clear, positive and negative results refresh after the
// short resolution TTL.
func (h *AssetHelper) ClearResolutionCache() {
	if h == nil || h.resolutionCache == nil {
		return
	}
	h.resolutionCache.clear()
}

func (h *AssetHelper) localCandidatePaths(candidateRel string) []string {
	if filepath.IsAbs(candidateRel) {
		return []string{candidateRel}
	}
	candidates := make([]string, 0, len(h.roots))
	for _, root := range h.roots {
		if isAssetURL(root) {
			continue
		}
		candidates = append(candidates, filepath.Join(root, candidateRel))
	}
	return candidates
}

func assetPathCandidates(rel string) []string {
	clean := filepath.ToSlash(strings.TrimSpace(rel))
	if clean == "" {
		return nil
	}
	candidates := []string{clean}
	if strings.HasPrefix(clean, "asset/") {
		trimmed := strings.TrimPrefix(clean, "asset/")
		if trimmed != "" && trimmed != clean {
			candidates = append(candidates, trimmed)
		}
	}
	return candidates
}

func (h *AssetHelper) resolveCaseInsensitivePath(candidatePath string) (string, bool) {
	if commandtrace.FromContext(h.ctx) != nil {
		startedAt := time.Now()
		defer func() {
			commandtrace.RecordOperation(h.ctx, "asset.case_walk", time.Since(startedAt))
		}()
	}

	clean := filepath.Clean(candidatePath)
	if clean == "" {
		return "", false
	}

	current := "."
	remaining := clean

	if filepath.IsAbs(clean) {
		volume := filepath.VolumeName(clean)
		if volume != "" {
			current = volume + string(filepath.Separator)
			remaining = strings.TrimPrefix(clean, volume)
		} else {
			current = string(filepath.Separator)
		}
	}

	for _, segment := range strings.Split(filepath.ToSlash(remaining), "/") {
		if segment == "" || segment == "." {
			continue
		}
		if segment == ".." {
			current = filepath.Dir(current)
			continue
		}

		exact := filepath.Join(current, segment)
		if _, err := h.stat(exact); err == nil {
			current = exact
			continue
		}

		matched, ok := h.matchPathComponent(current, segment)
		if !ok {
			return "", false
		}
		current = filepath.Join(current, matched)
	}

	if _, err := h.stat(current); err != nil {
		return "", false
	}
	return current, true
}

func (h *AssetHelper) matchPathComponent(parent, segment string) (string, bool) {
	index, ok := h.directoryIndex(parent)
	if !ok {
		return "", false
	}
	if matched, exists := index.exact[segment]; exists {
		return matched, true
	}
	if matched, exists := index.folded[foldAssetName(segment)]; exists {
		return matched, true
	}
	return "", false
}

func (h *AssetHelper) directoryIndex(parent string) (*assetDirectoryIndex, bool) {
	parent = filepath.Clean(parent)
	info, err := h.stat(parent)
	if err != nil || !info.IsDir() {
		return nil, false
	}
	cache := h.cache()
	if cached, ok := cache.lookup(parent, info.ModTime()); ok {
		return cached, true
	}

	finishWait := commandtrace.MeasureOperation(h.ctx, "asset.directory_wait")
	detachedHelper := h.WithContext(context.Background())
	value, _, shared := cache.loads.Do(parent, func() (any, error) {
		sharedCtx, trace := commandtrace.WithNewTrace(context.Background())
		sharedHelper := detachedHelper.WithContext(sharedCtx)
		complete := func(index *assetDirectoryIndex, err error) assetDirectoryFlightResult {
			return assetDirectoryFlightResult{
				index:      index,
				err:        err,
				operations: trace.Snapshot().Operations,
			}
		}

		currentInfo, statErr := sharedHelper.stat(parent)
		if statErr != nil {
			return complete(nil, statErr), nil
		}
		if cached, ok := cache.lookup(parent, currentInfo.ModTime()); ok {
			return complete(cached, nil), nil
		}

		entries, readErr := sharedHelper.readDir(parent)
		if readErr != nil {
			return complete(nil, readErr), nil
		}
		afterRead, statErr := sharedHelper.stat(parent)
		if statErr != nil {
			return complete(nil, statErr), nil
		}
		if !currentInfo.ModTime().Equal(afterRead.ModTime()) {
			entries, readErr = sharedHelper.readDir(parent)
			if readErr != nil {
				return complete(nil, readErr), nil
			}
			afterRead, statErr = sharedHelper.stat(parent)
			if statErr != nil {
				return complete(nil, statErr), nil
			}
		}

		index := newAssetDirectoryIndex(afterRead.ModTime(), entries)
		cache.store(parent, index)
		return complete(index, nil), nil
	})
	finishWait()
	if shared {
		commandtrace.RecordOperation(h.ctx, "asset.directory_shared", 0)
	}
	result, ok := value.(assetDirectoryFlightResult)
	if !ok {
		return nil, false
	}
	commandtrace.MergeOperations(h.ctx, result.operations)
	return result.index, result.err == nil && result.index != nil
}

func newAssetDirectoryIndex(modTime time.Time, entries []fs.DirEntry) *assetDirectoryIndex {
	index := &assetDirectoryIndex{
		modTime: modTime,
		exact:   make(map[string]string, len(entries)),
		folded:  make(map[string]string, len(entries)),
	}
	for _, entry := range entries {
		name := entry.Name()
		index.exact[name] = name
		folded := foldAssetName(name)
		if _, exists := index.folded[folded]; !exists {
			index.folded[folded] = name
		}
	}
	return index
}

// foldAssetName returns a stable representative for every rune's Unicode
// simple-fold cycle, matching the equivalence classes used by strings.EqualFold.
func foldAssetName(name string) string {
	var folded strings.Builder
	folded.Grow(len(name))
	for _, current := range name {
		canonical := current
		for candidate := unicode.SimpleFold(current); candidate != current; candidate = unicode.SimpleFold(candidate) {
			if candidate < canonical {
				canonical = candidate
			}
		}
		folded.WriteRune(canonical)
	}
	return folded.String()
}

func (h *AssetHelper) stat(name string) (fs.FileInfo, error) {
	if commandtrace.FromContext(h.ctx) == nil {
		return h.fileSystem().Stat(name)
	}
	startedAt := time.Now()
	info, err := h.fileSystem().Stat(name)
	commandtrace.RecordOperation(h.ctx, "asset.stat", time.Since(startedAt))
	return info, err
}

func (h *AssetHelper) readDir(name string) ([]fs.DirEntry, error) {
	if commandtrace.FromContext(h.ctx) == nil {
		return h.fileSystem().ReadDir(name)
	}
	startedAt := time.Now()
	entries, err := h.fileSystem().ReadDir(name)
	commandtrace.RecordOperation(h.ctx, "asset.readdir", time.Since(startedAt))
	return entries, err
}

func (h *AssetHelper) fileSystem() assetFileSystem {
	if h.fs == nil {
		return osAssetFileSystem{}
	}
	return h.fs
}

func (h *AssetHelper) cache() *assetDirectoryCache {
	if h.directoryCache == nil {
		// AssetHelpers are constructed by NewAssetHelper. Keep a nil-safe fallback
		// for zero-value helpers without adding synchronization to the hot path.
		return &assetDirectoryCache{
			entries:    make(map[string]*assetDirectoryCacheEntry),
			maxEntries: assetDirectoryMaxEntries,
			maxNames:   assetDirectoryMaxNames,
		}
	}
	return h.directoryCache
}

func (h *AssetHelper) resolutions() *assetResolutionCache {
	if h == nil || h.resolutionCache == nil {
		return &assetResolutionCache{
			entries:    make(map[string]*assetResolutionEntry),
			ttl:        assetResolutionTTL,
			maxEntries: assetResolutionMaxEntries,
		}
	}
	return h.resolutionCache
}

func (c *assetResolutionCache) lookup(key string, now time.Time) (string, bool) {
	if c == nil || c.ttl <= 0 {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if now.Before(entry.expiresAt) {
		c.recent.MoveToFront(entry.element)
		return entry.path, true
	}
	c.removeLocked(key, entry)
	return "", false
}

func (c *assetResolutionCache) store(key, resolved string, now time.Time) {
	if c == nil || c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.storeLocked(key, resolved, now)
}

func (c *assetResolutionCache) storeForGeneration(key, resolved string, now time.Time, generation uint64) bool {
	if c == nil || c.ttl <= 0 {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation != generation {
		return false
	}
	c.storeLocked(key, resolved, now)
	return true
}

func (c *assetResolutionCache) storeLocked(key, resolved string, now time.Time) {
	if c.entries == nil {
		c.entries = make(map[string]*assetResolutionEntry)
	}
	if entry, ok := c.entries[key]; ok {
		entry.path = resolved
		entry.expiresAt = now.Add(c.ttl)
		c.recent.MoveToFront(entry.element)
		return
	}
	entry := &assetResolutionEntry{path: resolved, expiresAt: now.Add(c.ttl)}
	entry.element = c.recent.PushFront(key)
	c.entries[key] = entry
	for len(c.entries) > c.entryLimit() {
		oldest := c.recent.Back()
		if oldest == nil {
			break
		}
		oldestKey, _ := oldest.Value.(string)
		c.removeLocked(oldestKey, c.entries[oldestKey])
	}
}

func (c *assetResolutionCache) clear() {
	c.mu.Lock()
	clear(c.entries)
	c.recent.Init()
	c.generation++
	c.mu.Unlock()
}

func (c *assetResolutionCache) currentGeneration() uint64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	generation := c.generation
	c.mu.Unlock()
	return generation
}

func (c *assetResolutionCache) entryLimit() int {
	if c.maxEntries > 0 {
		return c.maxEntries
	}
	return assetResolutionMaxEntries
}

func (c *assetResolutionCache) removeLocked(key string, entry *assetResolutionEntry) {
	delete(c.entries, key)
	if entry != nil && entry.element != nil {
		c.recent.Remove(entry.element)
		entry.element = nil
	}
}

func (c *assetDirectoryCache) lookup(parent string, modTime time.Time) (*assetDirectoryIndex, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[parent]
	if !ok {
		return nil, false
	}
	if entry.index == nil || !entry.index.modTime.Equal(modTime) {
		c.removeLocked(parent, entry)
		return nil, false
	}
	c.recent.MoveToFront(entry.element)
	return entry.index, true
}

func (c *assetDirectoryCache) store(parent string, index *assetDirectoryIndex) {
	if c == nil || index == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]*assetDirectoryCacheEntry)
	}
	if current, ok := c.entries[parent]; ok {
		c.removeLocked(parent, current)
	}
	names := len(index.exact)
	if names > c.nameLimit() {
		return
	}
	entry := &assetDirectoryCacheEntry{index: index}
	entry.element = c.recent.PushFront(parent)
	c.entries[parent] = entry
	c.indexed += names
	for len(c.entries) > c.entryLimit() || c.indexed > c.nameLimit() {
		oldest := c.recent.Back()
		if oldest == nil {
			break
		}
		oldestParent, _ := oldest.Value.(string)
		c.removeLocked(oldestParent, c.entries[oldestParent])
	}
}

func (c *assetDirectoryCache) entryLimit() int {
	if c.maxEntries > 0 {
		return c.maxEntries
	}
	return assetDirectoryMaxEntries
}

func (c *assetDirectoryCache) nameLimit() int {
	if c.maxNames > 0 {
		return c.maxNames
	}
	return assetDirectoryMaxNames
}

func (c *assetDirectoryCache) removeLocked(parent string, entry *assetDirectoryCacheEntry) {
	delete(c.entries, parent)
	if entry == nil {
		return
	}
	if entry.index != nil {
		c.indexed -= len(entry.index.exact)
		if c.indexed < 0 {
			c.indexed = 0
		}
	}
	if entry.element != nil {
		c.recent.Remove(entry.element)
		entry.element = nil
	}
}

func ResolveAssetPath(helper *AssetHelper, assetDir string, relPaths ...string) string {
	if len(relPaths) == 0 {
		return ""
	}
	if helper != nil {
		if resolved := helper.FirstExisting(relPaths...); resolved != "" {
			return filepath.ToSlash(resolved)
		}
	}
	base := assetDir
	if base == "" && helper != nil {
		base = helper.Primary()
	}
	if base == "" {
		return filepath.ToSlash(relPaths[0])
	}
	return joinAssetPath(base, relPaths[0])
}

const (
	RegionAssetStartApp = "startapp"
	RegionAssetOnDemand = "ondemand"
)

var onDemandPreferredTopLevel = map[string]struct{}{
	"event":        {},
	"event_story":  {},
	"gacha":        {},
	"lottery_game": {},
	"mysekai":      {},
	"unit_story":   {},
	"virtual_live": {},
}

// RegionAssetDir returns the region-specific startapp asset subdirectory prefix
// used by the Haruki Drawing API, e.g. "asset/jp-assets/startapp" for "jp".
func RegionAssetDir(region string) string {
	return RegionAssetDirByMode(region, RegionAssetStartApp)
}

func RegionAssetDirByMode(region, mode string) string {
	normalizedRegion := strings.ToLower(strings.TrimSpace(region))
	if normalizedRegion == "" {
		normalizedRegion = "jp"
	}
	normalizedMode := strings.ToLower(strings.TrimSpace(mode))
	if normalizedMode == "" {
		normalizedMode = RegionAssetStartApp
	}
	return "asset/" + normalizedRegion + "-assets/" + normalizedMode
}

// 和Drawing不同，Cloud的asset_dirs没有挂载到asset/下
func CloudRegionAssetDirByMode(region, mode string) string {
	normalizedRegion := strings.ToLower(strings.TrimSpace(region))
	if normalizedRegion == "" {
		normalizedRegion = "jp"
	}
	normalizedMode := strings.ToLower(strings.TrimSpace(mode))
	if normalizedMode == "" {
		normalizedMode = RegionAssetStartApp
	}
	return normalizedRegion + "-assets/" + normalizedMode
}
func RegionAssetDirs(region string) []string {
	return []string{
		RegionAssetDirByMode(region, RegionAssetStartApp),
		RegionAssetDirByMode(region, RegionAssetOnDemand),
	}
}

func preferredRegionAssetModes(relPath string) []string {
	parts := strings.Split(strings.Trim(filepath.ToSlash(relPath), "/"), "/")
	if len(parts) > 0 {
		if _, ok := onDemandPreferredTopLevel[parts[0]]; ok {
			return []string{RegionAssetOnDemand, RegionAssetStartApp}
		}
	}
	return []string{RegionAssetStartApp, RegionAssetOnDemand}
}

// ResolveRegionAssetPath resolves a region game asset by trying startapp first and
// ondemand second for each relative candidate path.
// For top-level paths that are primarily delivered from ondemand (for example
// gacha/event/event_story/music/mysekai), the priority is reversed to
// ondemand first and startapp second.
func ResolveRegionAssetPath(helper *AssetHelper, region string, relPaths ...string) string {
	if len(relPaths) == 0 {
		return ""
	}
	candidates := make([]string, 0, len(relPaths)*2)
	for _, rel := range relPaths {
		cleanRel := filepath.ToSlash(strings.TrimSpace(rel))
		if cleanRel == "" {
			continue
		}
		for _, mode := range preferredRegionAssetModes(cleanRel) {
			base := RegionAssetDirByMode(region, mode)
			candidates = append(candidates, filepath.ToSlash(filepath.Join(base, cleanRel)))
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	// Probe local existence to choose the right candidate, but keep the returned
	// path relative for DrawingAPI callers that cannot access host-local absolute paths.
	if helper != nil {
		for _, candidate := range candidates {
			if helper.FirstExisting(candidate) != "" {
				return candidate
			}
		}
	}
	// Fall back to the first candidate as a relative path so that callers forwarding
	// the result to the Drawing API always receive a relative path, not an absolute
	// local path that the Drawing API container cannot access.
	return candidates[0]
}

func ResolveEventBannerPath(helper *AssetHelper, region, assetBundleName string) string {
	assetBundleName = strings.TrimSpace(assetBundleName)
	if assetBundleName == "" {
		return ""
	}
	return ResolveRegionAssetPath(
		helper,
		region,
		filepath.Join("home", "banner", assetBundleName, assetBundleName+".png"),
		filepath.Join("event", assetBundleName, "banner.png"),
		filepath.Join("event_story", assetBundleName, "screen_image", "banner_event_story.png"),
	)
}

// StaticImagesDir is the base directory for Drawing API static UI images
// (card frames, rarity icons, attribute icons, character icons, unit logos,
// skill icons, play-result icons, etc.) within the Drawing API data root.
const StaticImagesDir = "static_images"

// ResolveProfilePlaceholderPath returns the stable Drawing placeholder avatar
// used when no region-specific leader image can be resolved.
func ResolveProfilePlaceholderPath(helper *AssetHelper) string {
	return ResolveAssetPath(helper, StaticImagesDir, "unknown.jpg")
}

var CharacterIDToNickname = map[int]string{
	1:  "ick",
	2:  "saki",
	3:  "hnm",
	4:  "shiho",
	5:  "mnr",
	6:  "hrk",
	7:  "airi",
	8:  "szk",
	9:  "khn",
	10: "an",
	11: "akt",
	12: "toya",
	13: "tks",
	14: "emu",
	15: "nene",
	16: "rui",
	17: "knd",
	18: "mfy",
	19: "ena",
	20: "mzk",
	21: "miku",
	22: "rin",
	23: "len",
	24: "luka",
	25: "meiko",
	26: "kaito",
}

func MakeRelative(base, target string) string {
	if base == "" || target == "" {
		return target
	}

	cleanBase := normalizeAssetRoot(base)
	cleanTarget := normalizeAssetRoot(target)
	if strings.HasPrefix(cleanTarget, cleanBase) {
		rel := strings.TrimPrefix(cleanTarget, cleanBase)
		rel = strings.TrimPrefix(rel, "/")
		return rel
	}
	return cleanTarget
}

func normalizeAssetRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	if isAssetURL(root) {
		return strings.TrimRight(root, "/")
	}
	return filepath.ToSlash(filepath.Clean(root))
}

func joinAssetPath(base string, parts ...string) string {
	base = normalizeAssetRoot(base)
	if base == "" {
		if len(parts) == 0 {
			return ""
		}
		return filepath.ToSlash(parts[0])
	}
	if isAssetURL(base) {
		parsed, err := url.Parse(base)
		if err != nil {
			return base
		}
		joined := make([]string, 0, len(parts)+1)
		if parsed.Path != "" {
			joined = append(joined, parsed.Path)
		}
		for _, part := range parts {
			if strings.TrimSpace(part) == "" {
				continue
			}
			joined = append(joined, filepath.ToSlash(part))
		}
		parsed.Path = path.Join(joined...)
		return parsed.String()
	}
	all := append([]string{base}, parts...)
	return filepath.ToSlash(filepath.Join(all...))
}

func isAssetURL(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}
