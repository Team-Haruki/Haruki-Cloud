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

func masterdataRegistryCurrentURL(baseURL string, region renderregion.Value) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/v1/master/" + strings.ToLower(region.String()) + "/current"
}

// startRegistryMasterdataRefresh polls the master registry's per-region
// manifest pointer and resets a region's provider caches when its
// contentHash changes. Unlike the local file loop it does not need the local
// masterdata fallback: it is how DB-backed caches learn about a new ingest.
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
	if len(state.providers) == 0 {
		return
	}
	logger.Info("masterdata registry poll loop started",
		"registry_url", baseURL,
		"poll_interval", interval,
	)
	go func() {
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

	mu     sync.Mutex
	etags  map[renderregion.Value]string
	hashes map[renderregion.Value]string
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
		etags:               make(map[renderregion.Value]string, len(providers)),
		hashes:              make(map[renderregion.Value]string, len(providers)),
	}
	for region, resetter := range providers {
		if resetter == nil {
			continue
		}
		state.providers[renderregion.WithDefault(region)] = resetter
	}
	return state
}

// poll checks every region once. The first hash seen for a region is only
// recorded; every later change resets that region's caches, and the shared
// controllers are reset once per poll when any region changed.
func (s *registryMasterdataRefreshState) poll(ctx context.Context) {
	if s == nil {
		return
	}
	changed := false
	for region, resetter := range s.providers {
		hash, updated, err := s.pollRegion(ctx, region)
		if err != nil {
			logger.Warn("masterdata registry poll failed",
				"region", region,
				"error_type", fmt.Sprintf("%T", err),
				"error", err,
			)
			continue
		}
		if !updated {
			continue
		}
		resetter.ResetMasterdataCache()
		changed = true
		logger.Info("masterdata cache reset",
			"reason", "registry_changed",
			"region", region,
			"content_hash", hash,
		)
	}
	if !changed {
		return
	}
	for _, resetter := range s.additionalResetters {
		if resetter != nil {
			resetter.ResetMasterdataCache()
		}
	}
}

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
	hash := strings.TrimSpace(pointer.ContentHash)
	if hash == "" {
		return "", false, fmt.Errorf("manifest pointer has no contentHash")
	}

	s.mu.Lock()
	previous := s.hashes[region]
	s.hashes[region] = hash
	s.etags[region] = response.Header.Get("ETag")
	s.mu.Unlock()
	return hash, previous != "" && previous != hash, nil
}
