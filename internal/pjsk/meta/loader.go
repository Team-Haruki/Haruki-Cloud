package meta

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"expvar"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/storage"
	"haruki-cloud/utils/logger"

	"github.com/go-resty/resty/v2"
)

const (
	musicMetaMaxResponseBytes = 64 << 20

	// musicMetaFetchTimeout bounds one region's request, body included. A
	// region is a few MB uncompressed; nothing waits on the fetch, so the
	// bound only has to catch a stalled transfer on a slow link.
	musicMetaFetchTimeout = 60 * time.Second

	// musicMetaFetchConcurrency limits the regions fetched at once. On a
	// bandwidth-bound link more parallel transfers do not finish sooner, they
	// only make every request slower and push all of them towards the
	// timeout; two keeps one request's round trip and parsing overlapped
	// with another's transfer.
	musicMetaFetchConcurrency = 2

	// musicMetaInitialWait bounds how long Start blocks for regions that have
	// no persisted copy (a first boot). Regions with a copy never wait.
	musicMetaInitialWait = 15 * time.Second

	// validatorSuffix names the object holding the HTTP validators of a
	// persisted payload, next to it in the same store.
	validatorSuffix = ".validator"
)

// Active sources of a region's data.
const (
	SourcePersisted = "persisted"
	SourceRemote    = "remote"
)

// defaultRetryBackoff is the wait before each retry of the regions a refresh
// round could not fetch; the next round starts at the refresh interval.
var defaultRetryBackoff = []time.Duration{5 * time.Second, 15 * time.Second, 45 * time.Second}

// regionEntry holds a fetched music_metas payload together with the
// HTTP caching headers used for conditional re-fetches. Entries are never
// mutated after they are installed; updates install a new entry.
type regionEntry struct {
	data         []byte // processed JSON (omakase entry injected)
	view         *View
	etag         string
	lastModified string
	digest       string    // sha256 of data
	source       string    // SourcePersisted or SourceRemote
	loadedAt     time.Time // when this payload was installed
	checkedAt    time.Time // last remote 200/304 for this payload
}

// regionFailure records the last failed remote fetch of a region.
type regionFailure struct {
	at        time.Time
	errorType string
}

// persistedValidators is stored next to a persisted payload. sha256 binds the
// validators to the exact bytes they were stored with: a payload written
// without its validators (or with a failed validator write) does not match
// and is revalidated with a full fetch instead of a 304 pinning wrong data.
type persistedValidators struct {
	SHA256       string `json:"sha256"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

// persistError marks a remote payload that was installed but could not be
// written to the store; retrying the fetch would not help.
type persistError struct{ err error }

func (e *persistError) Error() string { return e.err.Error() }
func (e *persistError) Unwrap() error { return e.err }

// Loader fetches and caches music_metas JSON per region.
// It uses ETag / Last-Modified for conditional HTTP requests so
// unchanged responses cost only a single round-trip with no body transfer.
type Loader struct {
	http         *resty.Client
	mu           sync.RWMutex
	cache        map[string]*regionEntry
	failures     map[string]regionFailure
	loadMu       sync.Mutex
	loadLocks    map[string]*sync.Mutex
	logger       *logger.Logger
	store        storage.Store
	source       string
	baseURL      string
	retryBackoff []time.Duration
	initialWait  time.Duration
	concurrency  int
}

type LoaderOption func(*Loader)

// WithSource selects where music_metas are fetched from (SourceLegacy or
// SourceRegistry). Unknown values surface as a load error per region.
func WithSource(source string) LoaderOption {
	return func(l *Loader) {
		l.source = strings.ToLower(strings.TrimSpace(source))
	}
}

// WithBaseURL overrides the upstream host. Required for SourceRegistry;
// optional for SourceLegacy (a mirror of the community layout).
func WithBaseURL(baseURL string) LoaderOption {
	return func(l *Loader) {
		l.baseURL = strings.TrimSpace(baseURL)
	}
}

// WithOutputDir enables atomic persistence of fetched music_metas JSON in a
// local directory ("" -> no persistence). It is WithStore over a local store.
func WithOutputDir(dir string) LoaderOption {
	return func(l *Loader) {
		l.store = nil
		if strings.TrimSpace(dir) == "" {
			return
		}
		if store, err := storage.NewLocalAt(dir, 0); err == nil {
			l.store = store
		}
	}
}

// WithStore persists fetched music_metas JSON as one object per region
// (music_metas.json, music_metas-en.json, ...) in store; nil disables
// persistence, and a store that is not configured behaves the same.
func WithStore(store storage.Store) LoaderOption {
	return func(l *Loader) {
		l.store = store
	}
}

// NewLoader creates a Loader ready to use.
// Pass nil for log to silence all output.
func NewLoader(log *logger.Logger, options ...LoaderOption) *Loader {
	l := &Loader{
		// No Accept-Encoding is set by hand: net/http then asks for gzip and
		// decompresses transparently, and the body limit applies to the
		// decompressed payload.
		http: resty.New().
			SetTimeout(musicMetaFetchTimeout).
			SetResponseBodyLimit(musicMetaMaxResponseBytes),
		cache:        make(map[string]*regionEntry),
		failures:     make(map[string]regionFailure),
		loadLocks:    make(map[string]*sync.Mutex),
		logger:       log,
		retryBackoff: defaultRetryBackoff,
		initialWait:  musicMetaInitialWait,
		concurrency:  musicMetaFetchConcurrency,
	}
	for _, option := range options {
		if option != nil {
			option(l)
		}
	}
	return l
}

// Start installs every region's persisted copy, then fetches all regions from
// the remote source in the background (retrying failed regions with backoff)
// and keeps refreshing them every interval until ctx is cancelled. Startup
// only waits, at most musicMetaInitialWait, when a region has no persisted
// copy; the fetch carries on in the background either way.
func (l *Loader) Start(ctx context.Context, interval time.Duration) {
	if l == nil {
		return
	}
	missing := l.LoadPersistedAll(ctx)
	firstPass := make(chan struct{})
	go func() {
		l.refresh(ctx, func() { close(firstPass) })
		l.refreshEvery(ctx, interval)
	}()
	if missing == 0 {
		return
	}
	timer := time.NewTimer(l.initialWait)
	defer timer.Stop()
	select {
	case <-firstPass:
	case <-timer.C:
		if l.logger != nil {
			l.logger.WarnContext(ctx, "music metadata without a persisted copy still loading in the background",
				"regions_without_copy", missing, "waited_ms", l.initialWait.Milliseconds())
		}
	case <-ctx.Done():
	}
}

// LoadPersistedAll installs the persisted copy of every region that has none
// cached yet and returns how many regions are still without data.
func (l *Loader) LoadPersistedAll(ctx context.Context) int {
	missing := 0
	for _, region := range Regions() {
		err := l.withRegionLock(region, func() error {
			if l.entry(region) != nil {
				return nil
			}
			return l.loadPersisted(ctx, region)
		})
		if err != nil {
			missing++
			if l.logger != nil && l.store != nil && !errors.Is(err, storage.ErrNotExist) {
				l.logger.WarnContext(ctx, "music metadata persisted copy unusable",
					"region", region, "error_type", fmt.Sprintf("%T", err))
			}
		}
	}
	return missing
}

// refresh runs one refresh round: every region once, then the failed ones
// again after each retry backoff. firstPass, when set, runs after the first
// attempt of every region.
func (l *Loader) refresh(ctx context.Context, firstPass func()) {
	failed := l.loadRegions(ctx, Regions()).failed
	if firstPass != nil {
		firstPass()
	}
	for _, delay := range l.retryBackoff {
		if len(failed) == 0 || ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		failed = l.loadRegions(ctx, failed).failed
	}
	if len(failed) > 0 && ctx.Err() == nil && l.logger != nil {
		l.logger.WarnContext(ctx, "music metadata refresh retries exhausted; keeping the active data until the next refresh",
			"regions", strings.Join(failed, ","))
	}
}

func (l *Loader) refreshEvery(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.refresh(ctx, nil)
		}
	}
}

type loadRound struct {
	failed   []string
	firstErr error
}

// LoadAll fetches all known regions (at most l.concurrency at a time). It
// returns the first error encountered, but always attempts every region,
// and logs one summary line with the total and slowest region so startup
// and refresh timings are visible without per-region arithmetic.
func (l *Loader) LoadAll(ctx context.Context) error {
	return l.loadRegions(ctx, Regions()).firstErr
}

func (l *Loader) loadRegions(ctx context.Context, regions []string) loadRound {
	type result struct {
		region   string
		err      error
		fellBack bool
		elapsed  time.Duration
	}

	startedAt := time.Now()
	results := make(chan result, len(regions))
	slots := make(chan struct{}, max(l.concurrency, 1))
	for _, region := range regions {
		go func(r string) {
			slots <- struct{}{}
			defer func() { <-slots }()
			began := time.Now()
			fellBack, err := l.loadRegion(ctx, r)
			results <- result{r, err, fellBack, time.Since(began)}
		}(region)
	}

	var round loadRound
	var slowest result
	for range regions {
		res := <-results
		if res.elapsed > slowest.elapsed {
			slowest = res
		}
		if res.fellBack {
			// Usable from the persisted copy, but the remote still failed.
			round.failed = append(round.failed, res.region)
		}
		if res.err == nil {
			continue
		}
		if round.firstErr == nil {
			round.firstErr = res.err
		}
		var persistErr *persistError
		if !errors.As(res.err, &persistErr) {
			round.failed = append(round.failed, res.region)
			l.recordFailure(res.region, res.err)
		}
		if l.logger != nil {
			l.logger.WarnContext(ctx, "music metadata load failed",
				"region", res.region,
				"error_type", fmt.Sprintf("%T", res.err),
				"elapsed_ms", res.elapsed.Milliseconds(),
				"active_source", l.activeSource(res.region),
			)
		}
	}
	if l.logger != nil {
		l.logger.InfoContext(ctx, "music metadata load complete",
			"regions", len(regions), "failed", len(round.failed),
			"elapsed_ms", time.Since(startedAt).Milliseconds(),
			"slowest_region", slowest.region, "slowest_ms", slowest.elapsed.Milliseconds(),
		)
	}
	return round
}

func (l *Loader) withRegionLock(region string, fn func() error) error {
	l.loadMu.Lock()
	loadLock := l.loadLocks[region]
	if loadLock == nil {
		loadLock = &sync.Mutex{}
		l.loadLocks[region] = loadLock
	}
	l.loadMu.Unlock()
	loadLock.Lock()
	defer loadLock.Unlock()
	return fn()
}

func (l *Loader) entry(region string) *regionEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.cache[region]
}

func (l *Loader) install(region string, entry *regionEntry) {
	l.mu.Lock()
	l.cache[region] = entry
	if entry.source == SourceRemote {
		delete(l.failures, region)
	}
	l.mu.Unlock()
}

func (l *Loader) recordFailure(region string, err error) {
	l.mu.Lock()
	l.failures[region] = regionFailure{at: time.Now(), errorType: fmt.Sprintf("%T", err)}
	l.mu.Unlock()
}

func (l *Loader) activeSource(region string) string {
	if entry := l.entry(region); entry != nil {
		return entry.source
	}
	return "none"
}

// load performs a single conditional HTTP GET for one region.
// On 304 Not Modified the existing payload is kept.
// On 200 the body is processed (omakase injected) and cached.
// Without any cached payload a failed fetch falls back to the persisted copy.
func (l *Loader) load(ctx context.Context, region string) error {
	_, err := l.loadRegion(ctx, region)
	return err
}

// loadRegion is load that also reports whether the remote failed and the
// region was served from the persisted fallback instead (err is then nil).
func (l *Loader) loadRegion(ctx context.Context, region string) (bool, error) {
	url, err := ResolveURL(l.source, l.baseURL, region)
	if err != nil {
		return false, err
	}
	fellBack := false
	err = l.withRegionLock(region, func() error {
		existing := l.entry(region)
		err := l.loadRemote(ctx, region, url, existing)
		if err == nil {
			return nil
		}
		var persistErr *persistError
		if l.entry(region) != nil || errors.As(err, &persistErr) {
			return err
		}
		if fallbackErr := l.loadPersisted(ctx, region); fallbackErr != nil {
			return errors.Join(err, fmt.Errorf("meta: load persisted %s: %w", region, fallbackErr))
		}
		if l.logger != nil {
			l.logger.WarnContext(ctx, "music metadata remote load failed, using persisted fallback",
				"region", region,
				"error_type", fmt.Sprintf("%T", err),
			)
		}
		l.recordFailure(region, err)
		fellBack = true
		return nil
	})
	return fellBack, err
}

func (l *Loader) loadRemote(ctx context.Context, region, url string, existing *regionEntry) error {
	req := l.http.R().SetContext(ctx)
	if existing != nil {
		if existing.etag != "" {
			req = req.SetHeader("If-None-Match", existing.etag)
		}
		if existing.lastModified != "" {
			req = req.SetHeader("If-Modified-Since", existing.lastModified)
		}
	}

	resp, err := req.Get(url)
	if err != nil {
		return fmt.Errorf("meta: fetch %s: %w", region, err)
	}

	switch resp.StatusCode() {
	case 304:
		if existing == nil {
			return fmt.Errorf("meta: fetch %s returned 304 without a cached payload", region)
		}
		confirmed := *existing
		confirmed.checkedAt = time.Now()
		confirmed.source = SourceRemote
		l.install(region, &confirmed)
		if l.logger != nil {
			if existing.source == SourcePersisted {
				l.logger.InfoContext(ctx, "music metadata persisted copy confirmed by remote",
					"region", region, "status_code", 304, "sha256", shortDigest(existing.digest))
			} else {
				l.logger.DebugContext(ctx, "music metadata unchanged", "region", region, "status_code", 304)
			}
		}
		return nil

	case 200:
		processed, view, err := Prepare(resp.Body())
		if err != nil {
			return fmt.Errorf("meta: parse %s: %w", region, err)
		}
		now := time.Now()
		entry := &regionEntry{
			data:         processed,
			view:         view,
			etag:         resp.Header().Get("ETag"),
			lastModified: resp.Header().Get("Last-Modified"),
			digest:       digestOf(processed),
			source:       SourceRemote,
			loadedAt:     now,
			checkedAt:    now,
		}
		l.install(region, entry)
		if l.logger != nil {
			previous := "none"
			if existing != nil {
				previous = existing.source
			}
			l.logger.InfoContext(ctx, "music metadata updated",
				"region", region, "source", SourceRemote, "previous_source", previous,
				"response_bytes", len(processed), "sha256", shortDigest(entry.digest),
				"unchanged", existing != nil && existing.digest == entry.digest)
		}
		if err := l.persist(ctx, region, entry); err != nil {
			return &persistError{fmt.Errorf("meta: persist %s: %w", region, err)}
		}
		return nil

	default:
		return fmt.Errorf("meta: fetch %s returned HTTP %d", region, resp.StatusCode())
	}
}

func (l *Loader) loadPersisted(ctx context.Context, region string) error {
	if l == nil || l.store == nil {
		return fmt.Errorf("persisted metadata directory is not configured")
	}
	filename, ok := regionFilenames[region]
	if !ok {
		return fmt.Errorf("unknown region %q", region)
	}
	payload, err := l.store.Get(ctx, storage.Key(filename))
	if err != nil {
		return err
	}
	processed, view, err := Prepare(payload)
	if err != nil {
		return err
	}
	entry := &regionEntry{
		data:     processed,
		view:     view,
		digest:   digestOf(processed),
		source:   SourcePersisted,
		loadedAt: time.Now(),
	}
	// Validators only count for the exact bytes they were stored with.
	if validators, ok := l.loadValidators(ctx, filename); ok && validators.SHA256 == entry.digest && digestOf(payload) == entry.digest {
		entry.etag = validators.ETag
		entry.lastModified = validators.LastModified
	}
	l.install(region, entry)
	if l.logger != nil {
		l.logger.InfoContext(ctx, "music metadata loaded from persisted copy",
			"region", region, "source", SourcePersisted, "bytes", len(processed),
			"sha256", shortDigest(entry.digest), "revalidatable", entry.etag != "" || entry.lastModified != "")
	}
	return nil
}

func (l *Loader) loadValidators(ctx context.Context, filename string) (persistedValidators, bool) {
	raw, err := l.store.Get(ctx, storage.Key(filename+validatorSuffix))
	if err != nil {
		return persistedValidators{}, false
	}
	var validators persistedValidators
	if err := json.Unmarshal(raw, &validators); err != nil || validators.SHA256 == "" {
		return persistedValidators{}, false
	}
	return validators, true
}

func (l *Loader) persist(ctx context.Context, region string, entry *regionEntry) error {
	if l == nil || l.store == nil {
		return nil
	}
	filename, ok := regionFilenames[region]
	if !ok {
		return fmt.Errorf("unknown region %q", region)
	}
	ctx = context.WithoutCancel(ctx)
	err := l.store.Put(ctx, storage.Key(filename), entry.data, storage.PutOptions{ContentType: "application/json"})
	if errors.Is(err, storage.ErrNotConfigured) {
		return nil
	}
	if err != nil {
		return err
	}
	if entry.etag == "" && entry.lastModified == "" {
		return nil
	}
	validators, err := json.Marshal(persistedValidators{SHA256: entry.digest, ETag: entry.etag, LastModified: entry.lastModified})
	if err != nil {
		return err
	}
	return l.store.Put(ctx, storage.Key(filename+validatorSuffix), validators, storage.PutOptions{ContentType: "application/json"})
}

// Get returns a copy of the cached music_metas JSON for region.
// Returns nil if the region has not been loaded yet.
func (l *Loader) Get(region string) []byte {
	if l == nil {
		return nil
	}
	entry := l.entry(region)
	if entry == nil {
		return nil
	}
	out := make([]byte, len(entry.data))
	copy(out, entry.data)
	return out
}

// View returns the current immutable parsed generation for region. Refreshes
// replace the pointer atomically; the returned View remains valid forever.
func (l *Loader) View(region string) *View {
	if l == nil {
		return nil
	}
	entry := l.entry(region)
	if entry == nil {
		return nil
	}
	return entry.view
}

// RegionStatus describes the data a region is served from.
type RegionStatus struct {
	Source        string `json:"source"` // SourcePersisted, SourceRemote or "none"
	SHA256        string `json:"sha256,omitempty"`
	Bytes         int    `json:"bytes"`
	ETag          string `json:"etag,omitempty"`
	LoadedAt      string `json:"loaded_at,omitempty"`
	CheckedAt     string `json:"checked_at,omitempty"`
	LastErrorAt   string `json:"last_error_at,omitempty"`
	LastErrorType string `json:"last_error_type,omitempty"`
	RemoteSource  string `json:"remote_source,omitempty"`
	Revalidatable bool   `json:"revalidatable"`
}

// Status reports, per region, which source is active and when the remote
// last confirmed it.
func (l *Loader) Status() map[string]RegionStatus {
	if l == nil {
		return nil
	}
	remote := l.source
	if remote == "" {
		remote = SourceLegacy
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make(map[string]RegionStatus, len(regionFilenames))
	for _, region := range Regions() {
		status := RegionStatus{Source: "none", RemoteSource: remote}
		if entry := l.cache[region]; entry != nil {
			status.Source = entry.source
			status.SHA256 = entry.digest
			status.Bytes = len(entry.data)
			status.ETag = entry.etag
			status.LoadedAt = formatTime(entry.loadedAt)
			status.CheckedAt = formatTime(entry.checkedAt)
			status.Revalidatable = entry.etag != "" || entry.lastModified != ""
		}
		if failure, ok := l.failures[region]; ok {
			status.LastErrorAt = formatTime(failure.at)
			status.LastErrorType = failure.errorType
		}
		out[region] = status
	}
	return out
}

const musicMetasExpvarKey = "music_metas"

var (
	publishedLoader   atomic.Pointer[Loader]
	publishStatusOnce sync.Once
)

// PublishStatus exposes l.Status() on /debug/vars as "music_metas". The last
// loader published wins.
func (l *Loader) PublishStatus() {
	publishedLoader.Store(l)
	publishStatusOnce.Do(func() {
		expvar.Publish(musicMetasExpvarKey, expvar.Func(func() any {
			return publishedLoader.Load().Status()
		}))
	})
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func shortDigest(digest string) string {
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
