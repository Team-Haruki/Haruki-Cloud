package mysekai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/core/upstream"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"

	"golang.org/x/sync/singleflight"
)

const (
	mysekaiRegistryCacheTTL = 5 * time.Minute
	mysekaiRegistryTimeout  = 10 * time.Second
	mysekaiRegistryMaxSize  = 1 << 20
)

type registryMasterdataStore struct {
	baseURL  string
	region   string
	fallback masterdataSource
	ctx      context.Context
	cache    *registryMasterdataCache
}

type registryMasterdataCache struct {
	mu         sync.Mutex
	entries    map[string]registryMasterdataEntry
	generation uint64
	loads      singleflight.Group
	client     *http.Client
}

type registryMasterdataEntry struct {
	items     []map[string]any
	etag      string
	expiresAt time.Time
}

func newRegistryMasterdataStore(baseURL, region string, fallback masterdataSource) masterdataSource {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return fallback
	}
	region = strings.ToLower(strings.TrimSpace(region))
	switch region {
	case "jp", "en", "cn", "tw", "kr":
	default:
		return fallback
	}
	return &registryMasterdataStore{
		baseURL: baseURL, region: region, fallback: fallback,
		cache: &registryMasterdataCache{
			entries: make(map[string]registryMasterdataEntry),
			client: &http.Client{
				Timeout:   mysekaiRegistryTimeout,
				Transport: upstream.NewTunedTransport(upstream.TunedTransportConfig{}),
			},
		},
	}
}

func isMysekaiRegistryFile(filename string) bool {
	switch filename {
	case "mysekaiConvertItems.json", "mysekaiConvertItemTabs.json",
		"mysekaiConvertItemObtainMysekaiBlueprints.json",
		"mysekaiConvertItemObtainMysekaiBlueprintChoices.json",
		"mysekaiConvertItemObtainMysekaiMusicRecords.json",
		"materials.json", "practiceTickets.json", "skillPracticeTickets.json":
		return true
	default:
		return false
	}
}

func (s *registryMasterdataStore) Configured() bool {
	return s != nil && s.cache != nil && s.baseURL != ""
}

func (s *registryMasterdataStore) WithContext(ctx context.Context) masterdataSource {
	if s == nil {
		return nil
	}
	clone := *s
	clone.ctx = ctx
	clone.fallback = bindMasterdataContext(s.fallback, ctx)
	return &clone
}

func (s *registryMasterdataStore) loadList(filename string) []map[string]any {
	if !s.Configured() {
		return nil
	}
	if !isMysekaiRegistryFile(filename) {
		if s.fallback != nil {
			return s.fallback.loadList(filename)
		}
		return nil
	}
	// Registry reads happen only in request-scoped controller clones.
	if s.ctx == nil || s.ctx.Err() != nil {
		return nil
	}
	ctx := s.ctx
	s.cache.mu.Lock()
	entry, exists := s.cache.entries[filename]
	generation := s.cache.generation
	s.cache.mu.Unlock()
	if exists && time.Now().Before(entry.expiresAt) {
		return entry.items
	}
	flightKey := strconv.FormatUint(generation, 10) + ":" + filename
	result := s.cache.loads.DoChan(flightKey, func() (any, error) {
		s.cache.mu.Lock()
		cached, exists := s.cache.entries[filename]
		s.cache.mu.Unlock()
		if exists && time.Now().Before(cached.expiresAt) {
			return cached.items, nil
		}
		// One canceled caller must not abort a refresh shared by other requests.
		sharedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mysekaiRegistryTimeout)
		defer cancel()
		refreshed, err := s.fetch(sharedCtx, filename, cached, exists)
		if err != nil {
			return nil, err
		}
		s.cache.mu.Lock()
		if s.cache.generation == generation {
			s.cache.entries[filename] = refreshed
		}
		s.cache.mu.Unlock()
		return refreshed.items, nil
	})
	select {
	case <-ctx.Done():
		return nil
	case outcome := <-result:
		if outcome.Err != nil {
			return nil
		}
		items, _ := outcome.Val.([]map[string]any)
		return items
	}
}

func (s *registryMasterdataStore) fetch(ctx context.Context, filename string, cached registryMasterdataEntry, exists bool) (registryMasterdataEntry, error) {
	requestURL := s.baseURL + "/v1/master/" + url.PathEscape(s.region) + "/files/" + url.PathEscape(filename)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return registryMasterdataEntry{}, err
	}
	if exists && cached.etag != "" {
		req.Header.Set("If-None-Match", cached.etag)
	}
	finish := commandtrace.MeasureOperation(ctx, "mysekai.registry_request")
	defer finish()
	resp, err := s.cache.client.Do(req)
	if err != nil {
		return registryMasterdataEntry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified && exists {
		cached.expiresAt = time.Now().Add(mysekaiRegistryCacheTTL)
		return cached, nil
	}
	if resp.StatusCode != http.StatusOK {
		return registryMasterdataEntry{}, fmt.Errorf("master registry returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, mysekaiRegistryMaxSize+1))
	if err != nil {
		return registryMasterdataEntry{}, err
	}
	if len(data) > mysekaiRegistryMaxSize {
		return registryMasterdataEntry{}, fmt.Errorf("master registry file exceeds %d bytes", mysekaiRegistryMaxSize)
	}
	if !json.Valid(data) {
		return registryMasterdataEntry{}, fmt.Errorf("master registry file is invalid JSON")
	}
	var items []map[string]any
	if err := decodeJSONUseNumber(data, &items); err != nil {
		return registryMasterdataEntry{}, err
	}
	if items == nil {
		return registryMasterdataEntry{}, fmt.Errorf("master registry file is not an array")
	}
	return registryMasterdataEntry{
		items: items, etag: resp.Header.Get("ETag"), expiresAt: time.Now().Add(mysekaiRegistryCacheTTL),
	}, nil
}

func (s *registryMasterdataStore) loadMapByID(filename string) map[int]map[string]any {
	if s == nil {
		return nil
	}
	if !isMysekaiRegistryFile(filename) && s.fallback != nil {
		return s.fallback.loadMapByID(filename)
	}
	items := s.loadList(filename)
	if items == nil {
		return nil
	}
	result := make(map[int]map[string]any, len(items))
	for _, item := range items {
		if id := intNumberFrom(item, 0, "id"); id > 0 {
			result[id] = item
		}
	}
	return result
}

func (s *registryMasterdataStore) loadObject(filename string, target any) bool {
	return s != nil && s.fallback != nil && s.fallback.loadObject(filename, target)
}

func (s *registryMasterdataStore) resetCache() {
	if s == nil {
		return
	}
	if s.cache != nil {
		s.cache.mu.Lock()
		s.cache.entries = make(map[string]registryMasterdataEntry)
		s.cache.generation++
		s.cache.mu.Unlock()
	}
	if resetter, ok := s.fallback.(resettableMasterdataSource); ok {
		resetter.resetCache()
	}
}

func (s *registryMasterdataStore) Close() {
	if s == nil {
		return
	}
	if s.cache != nil && s.cache.client != nil {
		s.cache.client.CloseIdleConnections()
	}
	if closer, ok := s.fallback.(interface{ Close() }); ok {
		closer.Close()
	}
}
