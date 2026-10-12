package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	json "haruki-cloud/internal/jsonutil"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"
	"haruki-cloud/utils/logger"
)

const (
	defaultMasterdataRegistryPollInterval = 3 * time.Minute
	masterdataRegistryPollTimeout         = 15 * time.Second
	masterdataRegistryPointerMaxBytes     = 1 << 20
)

// defaultMasterdataRegistrySettleDelays are the follow-up resets scheduled
// after a registry change. The registry pointer moves when the master is
// published, but the DB ingest that consumes it commits per file and lands
// later, so the immediate reset can reload old rows; the follow-ups catch
// the ingest once it has settled.
var defaultMasterdataRegistrySettleDelays = []time.Duration{5 * time.Minute, 15 * time.Minute}

// resolveMasterdataRegistryURL picks the registry base URL the DB-backed
// providers watch for master data changes: the explicit masterdata_registry
// key first, then the URLs deck recommend and the music meta loader already
// point at.
func resolveMasterdataRegistryURL(cfg Config) string {
	for _, candidate := range []string{cfg.MasterdataRegistry.URL, cfg.DeckRecommend.RegistryURL} {
		if value := strings.TrimSpace(candidate); value != "" {
			return value
		}
	}
	if strings.EqualFold(strings.TrimSpace(cfg.MusicMetaSource), "registry") {
		return strings.TrimSpace(cfg.MusicMetaBaseURL)
	}
	return ""
}

func resolveMasterdataRegistrySettleDelays(configured []time.Duration) []time.Duration {
	if configured == nil {
		return defaultMasterdataRegistrySettleDelays
	}
	delays := make([]time.Duration, 0, len(configured))
	for _, delay := range configured {
		if delay > 0 {
			delays = append(delays, delay)
		}
	}
	return delays
}

func masterdataRegistryCurrentURL(baseURL string, region renderregion.Value) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/v1/master/" + strings.ToLower(region.String()) + "/current"
}

// startRegistryMasterdataRefresh polls the master registry's per-region
// manifest pointer and resets a region's provider caches when it changes.
// Unlike the local file loop it does not need the local masterdata fallback:
// it is how DB-backed caches learn about a new ingest.
func (a *App) startRegistryMasterdataRefresh(ctx context.Context, cfg Config) {
	if a == nil || len(a.Providers) == 0 {
		return
	}
	baseURL := resolveMasterdataRegistryURL(cfg)
	if baseURL == "" {
		return
	}
	interval := cfg.MasterdataRegistry.PollInterval
	if interval == 0 {
		interval = defaultMasterdataRegistryPollInterval
	}
	if interval < 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	state := newRegistryMasterdataRefreshState(baseURL, nil, masterdataResettersByRegion(a.Providers), a.masterdataAdditionalResetters()...)
	state.settleDelays = resolveMasterdataRegistrySettleDelays(cfg.MasterdataRegistry.SettleDelays)
	state.versioners = masterdataIngestVersionersByRegion(a.Providers)
	state.warm = func(region renderregion.Value) { a.warmRegionAsync(ctx, region) }
	if len(state.providers) == 0 {
		return
	}
	logger.Info("masterdata registry poll loop started",
		"registry_url", baseURL,
		"poll_interval", interval,
		"settle_delays", state.settleDelays,
	)
	go func() {
		defer state.stop()
		state.poll(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				state.poll(ctx)
			}
		}
	}()
}

func (a *App) masterdataAdditionalResetters() []masterdataCacheResetter {
	resetters := make([]masterdataCacheResetter, 0, 2)
	if a.MySekai != nil {
		resetters = append(resetters, a.MySekai)
	}
	if a.Inventory != nil {
		resetters = append(resetters, a.Inventory)
	}
	return resetters
}

// masterdataIngestVersioner is implemented by providers that can read the
// content hash the database ingest last committed for their region.
type masterdataIngestVersioner interface {
	MasterdataIngestHash(ctx context.Context) (string, bool)
}

func masterdataIngestVersionersByRegion(providers map[renderregion.Value]provider.MasterDataProvider) map[renderregion.Value]masterdataIngestVersioner {
	versioners := make(map[renderregion.Value]masterdataIngestVersioner, len(providers))
	for region, src := range providers {
		versioner, ok := src.(masterdataIngestVersioner)
		if !ok || versioner == nil {
			continue
		}
		versioners[renderregion.WithDefault(region)] = versioner
	}
	return versioners
}

func masterdataResettersByRegion(providers map[renderregion.Value]provider.MasterDataProvider) map[renderregion.Value]masterdataCacheResetter {
	resetters := make(map[renderregion.Value]masterdataCacheResetter, len(providers))
	for region, src := range providers {
		resetter, ok := src.(masterdataCacheResetter)
		if !ok || resetter == nil {
			continue
		}
		resetters[renderregion.WithDefault(region)] = resetter
	}
	return resetters
}

type registryMasterdataRefreshState struct {
	baseURL             string
	client              *http.Client
	providers           map[renderregion.Value]masterdataCacheResetter
	additionalResetters []masterdataCacheResetter
	settleDelays        []time.Duration
	// versioners read each region's committed ingest hash; regions without
	// one (or whose hash is unknown) fall back to the time-driven resets.
	versioners map[renderregion.Value]masterdataIngestVersioner
	// warm, when set, reloads a region's indexes after it was reset.
	warm func(region renderregion.Value)

	mu       sync.Mutex
	etags    map[renderregion.Value]string
	signals  map[renderregion.Value]string
	failures map[renderregion.Value]int
	settling map[renderregion.Value][]*time.Timer
	// loaded records, per region, the ingest hash the caches are known to
	// hold: the hash observed when they were last reset, or on the first
	// poll for caches filled lazily since startup. An empty entry marks a
	// reset made while the hash was unknown, so the next known hash must
	// reset again; a missing entry means no hash was observed yet.
	loaded  map[renderregion.Value]string
	stopped bool
}

type registryMasterdataPointer struct {
	ContentHash string `json:"contentHash"`
}

func newRegistryMasterdataRefreshState(baseURL string, client *http.Client, providers map[renderregion.Value]masterdataCacheResetter, additionalResetters ...masterdataCacheResetter) *registryMasterdataRefreshState {
	if client == nil {
		client = &http.Client{Timeout: masterdataRegistryPollTimeout}
	}
	state := &registryMasterdataRefreshState{
		baseURL:             strings.TrimSpace(baseURL),
		client:              client,
		providers:           make(map[renderregion.Value]masterdataCacheResetter, len(providers)),
		additionalResetters: additionalResetters,
		settleDelays:        defaultMasterdataRegistrySettleDelays,
		etags:               make(map[renderregion.Value]string, len(providers)),
		signals:             make(map[renderregion.Value]string, len(providers)),
		failures:            make(map[renderregion.Value]int, len(providers)),
		settling:            make(map[renderregion.Value][]*time.Timer, len(providers)),
		loaded:              make(map[renderregion.Value]string, len(providers)),
	}
	for region, resetter := range providers {
		if resetter == nil {
			continue
		}
		state.providers[renderregion.WithDefault(region)] = resetter
	}
	return state
}

// poll checks every region once. The ingest hash is consulted first: a
// change since the caches were last reset reloads them on its own. Then the
// registry pointer is polled; its first signal is only recorded, every later
// change resets the region (unless the ingest hash shows the database has
// not changed) and schedules the settle follow-ups. The shared controllers
// are reset with every region reset.
func (s *registryMasterdataRefreshState) poll(ctx context.Context) {
	if s == nil {
		return
	}
	for region := range s.providers {
		s.pollIngest(ctx, region)
		signal, updated, err := s.pollRegion(ctx, region)
		if err != nil {
			s.recordFailure(region, err)
			continue
		}
		s.recordSuccess(region)
		if !updated {
			continue
		}
		s.resetRegionIfChanged(ctx, region, "registry_changed", signal)
		s.scheduleSettleResets(region, signal)
	}
}

// pollIngest resets the region when its committed ingest hash differs from
// the one the caches hold. The first hash observed is only recorded: the
// caches filled since startup were read from that version. When the hash
// is unknown at the first observation (Cloud started mid-ingest, or the
// ingest failed), the region is recorded as unknown so that the first hash
// seen afterwards resets it instead of becoming the baseline.
func (s *registryMasterdataRefreshState) pollIngest(ctx context.Context, region renderregion.Value) {
	hash, known := s.ingestedHash(ctx, region)
	s.mu.Lock()
	previous, seen := s.loaded[region]
	if !seen {
		s.loaded[region] = ""
		if known {
			s.loaded[region] = hash
		}
	}
	s.mu.Unlock()
	if !known || !seen || previous == hash {
		return
	}
	s.resetRegion(region, "ingest_changed", hash, hash, true)
}

// resetRegionIfChanged resets the region unless its ingest hash is known
// and the caches were already reset for that hash. An unknown hash resets
// as before, so a database without the ingest version table keeps the
// time-driven behaviour.
func (s *registryMasterdataRefreshState) resetRegionIfChanged(ctx context.Context, region renderregion.Value, reason, signal string) bool {
	hash, known := s.ingestedHash(ctx, region)
	if known {
		s.mu.Lock()
		loaded := s.loaded[region]
		s.mu.Unlock()
		if loaded == hash {
			logger.Info("masterdata cache reset skipped",
				"reason", reason,
				"region", region,
				"signal", signal,
				"ingest_hash", hash,
			)
			return false
		}
	}
	s.resetRegion(region, reason, signal, hash, known)
	return true
}

func (s *registryMasterdataRefreshState) ingestedHash(ctx context.Context, region renderregion.Value) (string, bool) {
	versioner := s.versioners[region]
	if versioner == nil {
		return "", false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return versioner.MasterdataIngestHash(ctx)
}

// resetRegion drops the region's caches and the shared controllers, records
// the ingest hash the reset was made at (empty when unknown) and starts the
// region's reload when a warmer is configured.
func (s *registryMasterdataRefreshState) resetRegion(region renderregion.Value, reason, signal, ingestHash string, ingestKnown bool) {
	resetter := s.providers[region]
	if resetter == nil {
		return
	}
	resetter.ResetMasterdataCache()
	for _, additional := range s.additionalResetters {
		if additional != nil {
			additional.ResetMasterdataCache()
		}
	}
	s.mu.Lock()
	if ingestKnown {
		s.loaded[region] = ingestHash
	} else {
		s.loaded[region] = ""
	}
	s.mu.Unlock()
	logger.Info("masterdata cache reset",
		"reason", reason,
		"region", region,
		"signal", signal,
		"ingest_hash", ingestHash,
	)
	if s.warm != nil {
		s.warm(region)
	}
}

// scheduleSettleResets replaces any follow-ups still pending for the region,
// so a second change inside the settle window yields one series, not two.
// Each follow-up skips its reset when the ingest hash shows the database
// has not changed since the caches were last reset.
func (s *registryMasterdataRefreshState) scheduleSettleResets(region renderregion.Value, signal string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, timer := range s.settling[region] {
		timer.Stop()
	}
	if s.stopped || len(s.settleDelays) == 0 {
		delete(s.settling, region)
		return
	}
	timers := make([]*time.Timer, 0, len(s.settleDelays))
	for _, delay := range s.settleDelays {
		timers = append(timers, time.AfterFunc(delay, func() {
			s.resetRegionIfChanged(context.Background(), region, "registry_settle", signal)
		}))
	}
	s.settling[region] = timers
}

// stop cancels pending settle resets; it is called when the poll loop ends.
func (s *registryMasterdataRefreshState) stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	for region, timers := range s.settling {
		for _, timer := range timers {
			timer.Stop()
		}
		delete(s.settling, region)
	}
}

// recordFailure logs the 1st, 2nd, 4th, 8th... consecutive failure of a
// region so an unreachable registry does not warn on every poll.
func (s *registryMasterdataRefreshState) recordFailure(region renderregion.Value, err error) {
	s.mu.Lock()
	s.failures[region]++
	count := s.failures[region]
	s.mu.Unlock()
	if count&(count-1) != 0 {
		return
	}
	logger.Warn("masterdata registry poll failed",
		"region", region,
		"consecutive_failures", count,
		"error_type", fmt.Sprintf("%T", err),
		"error", err,
	)
}

func (s *registryMasterdataRefreshState) recordSuccess(region renderregion.Value) {
	s.mu.Lock()
	count := s.failures[region]
	s.failures[region] = 0
	s.mu.Unlock()
	if count > 0 {
		logger.Info("masterdata registry poll recovered",
			"region", region,
			"consecutive_failures", count,
		)
	}
}

// pollRegion returns the region's change signal (contentHash, or the ETag
// when the pointer carries no hash) and whether it differs from the last
// one recorded.
func (s *registryMasterdataRefreshState) pollRegion(ctx context.Context, region renderregion.Value) (string, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, masterdataRegistryPollTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, masterdataRegistryCurrentURL(s.baseURL, region), nil)
	if err != nil {
		return "", false, err
	}
	s.mu.Lock()
	etag := s.etags[region]
	s.mu.Unlock()
	if etag != "" {
		request.Header.Set("If-None-Match", etag)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return "", false, err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusNotModified:
		return "", false, nil
	case http.StatusOK:
	default:
		return "", false, fmt.Errorf("registry returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, masterdataRegistryPointerMaxBytes))
	if err != nil {
		return "", false, err
	}
	var pointer registryMasterdataPointer
	if err := json.Unmarshal(body, &pointer); err != nil {
		return "", false, err
	}
	responseETag := strings.TrimSpace(response.Header.Get("ETag"))
	signal := strings.TrimSpace(pointer.ContentHash)
	if signal == "" {
		signal = responseETag
	}
	if signal == "" {
		return "", false, fmt.Errorf("manifest pointer has neither contentHash nor ETag")
	}

	s.mu.Lock()
	previous := s.signals[region]
	s.signals[region] = signal
	s.etags[region] = responseETag
	s.mu.Unlock()
	return signal, previous != "" && previous != signal, nil
}
