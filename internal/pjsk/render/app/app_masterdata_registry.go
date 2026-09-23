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

	mu       sync.Mutex
	etags    map[renderregion.Value]string
	signals  map[renderregion.Value]string
	failures map[renderregion.Value]int
	settling map[renderregion.Value][]*time.Timer
	stopped  bool
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
	}
	for region, resetter := range providers {
		if resetter == nil {
			continue
		}
		state.providers[renderregion.WithDefault(region)] = resetter
	}
	return state
}

// poll checks every region once. The first signal seen for a region is only
// recorded; every later change resets that region's caches right away and
// schedules the settle follow-ups. The shared controllers are reset with
// every region reset.
func (s *registryMasterdataRefreshState) poll(ctx context.Context) {
	if s == nil {
		return
	}
	for region := range s.providers {
		signal, updated, err := s.pollRegion(ctx, region)
		if err != nil {
			s.recordFailure(region, err)
			continue
		}
		s.recordSuccess(region)
		if !updated {
			continue
		}
		s.resetRegion(region, "registry_changed", signal)
		s.scheduleSettleResets(region, signal)
	}
}

func (s *registryMasterdataRefreshState) resetRegion(region renderregion.Value, reason, signal string) {
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
	logger.Info("masterdata cache reset",
		"reason", reason,
		"region", region,
		"signal", signal,
	)
}

// scheduleSettleResets replaces any follow-ups still pending for the region,
// so a second change inside the settle window yields one series, not two.
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
			s.resetRegion(region, "registry_settle", signal)
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
