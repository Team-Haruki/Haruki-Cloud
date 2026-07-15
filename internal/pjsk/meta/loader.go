package meta

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/utils/logger"

	"github.com/go-resty/resty/v2"
)

const musicMetaMaxResponseBytes = 64 << 20

// regionEntry holds a fetched music_metas payload together with the
// HTTP caching headers used for conditional re-fetches.
type regionEntry struct {
	data         []byte // processed JSON (omakase entry injected)
	view         *View
	etag         string
	lastModified string
}

// Loader fetches and caches music_metas JSON per region.
// It uses ETag / Last-Modified for conditional HTTP requests so
// unchanged responses cost only a single round-trip with no body transfer.
type Loader struct {
	http      *resty.Client
	mu        sync.RWMutex
	cache     map[string]*regionEntry
	loadMu    sync.Mutex
	loadLocks map[string]*sync.Mutex
	logger    *logger.Logger
	outputDir string
}

type LoaderOption func(*Loader)

// WithOutputDir enables atomic persistence of fetched music_metas JSON.
func WithOutputDir(dir string) LoaderOption {
	return func(l *Loader) {
		l.outputDir = strings.TrimSpace(dir)
	}
}

// NewLoader creates a Loader ready to use.
// Pass nil for log to silence all output.
func NewLoader(log *logger.Logger, options ...LoaderOption) *Loader {
	l := &Loader{
		http: resty.New().
			SetTimeout(config.HTTPClientTimeout).
			SetResponseBodyLimit(musicMetaMaxResponseBytes),
		cache:     make(map[string]*regionEntry),
		loadLocks: make(map[string]*sync.Mutex),
		logger:    log,
	}
	for _, option := range options {
		if option != nil {
			option(l)
		}
	}
	return l
}

// LoadAll fetches all known regions concurrently.
// It returns the first error encountered, but always attempts every region.
func (l *Loader) LoadAll(ctx context.Context) error {
	type result struct {
		region string
		err    error
	}

	results := make(chan result, len(Regions()))
	for _, region := range Regions() {
		go func(r string) {
			results <- result{r, l.load(ctx, r)}
		}(region)
	}

	var firstErr error
	for range Regions() {
		if res := <-results; res.err != nil {
			if l.logger != nil {
				l.logger.WarnContext(ctx, "music metadata load failed",
					"region", res.region,
					"error_type", fmt.Sprintf("%T", res.err),
				)
			}
			if firstErr == nil {
				firstErr = res.err
			}
		}
	}
	return firstErr
}

// load performs a single conditional HTTP GET for one region.
// On 304 Not Modified the existing cache entry is kept as-is.
// On 200 the body is processed (omakase injected) and cached.
func (l *Loader) load(ctx context.Context, region string) error {
	url, ok := regionURLs[region]
	if !ok {
		return fmt.Errorf("meta: unknown region %q", region)
	}
	l.loadMu.Lock()
	loadLock := l.loadLocks[region]
	if loadLock == nil {
		loadLock = &sync.Mutex{}
		l.loadLocks[region] = loadLock
	}
	l.loadMu.Unlock()
	loadLock.Lock()
	defer loadLock.Unlock()

	l.mu.RLock()
	existing := l.cache[region]
	l.mu.RUnlock()

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
		if l.logger != nil {
			l.logger.DebugContext(ctx, "music metadata unchanged", "region", region, "status_code", 304)
		}
		return nil

	case 200:
		processed, view, err := Prepare(resp.Body())
		if err != nil {
			return fmt.Errorf("meta: parse %s: %w", region, err)
		}
		entry := &regionEntry{
			data:         processed,
			view:         view,
			etag:         resp.Header().Get("ETag"),
			lastModified: resp.Header().Get("Last-Modified"),
		}
		l.mu.Lock()
		l.cache[region] = entry
		l.mu.Unlock()
		if err := l.persist(region, processed); err != nil {
			return fmt.Errorf("meta: persist %s: %w", region, err)
		}
		if l.logger != nil {
			l.logger.InfoContext(ctx, "music metadata updated", "region", region, "response_bytes", len(processed))
		}
		return nil

	default:
		return fmt.Errorf("meta: fetch %s returned HTTP %d", region, resp.StatusCode())
	}
}

func (l *Loader) persist(region string, data []byte) error {
	if l == nil || strings.TrimSpace(l.outputDir) == "" {
		return nil
	}
	filename, ok := regionFilenames[region]
	if !ok {
		return fmt.Errorf("unknown region %q", region)
	}
	dir := filepath.Clean(l.outputDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filename+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filepath.Join(dir, filename))
}

// Get returns a copy of the cached music_metas JSON for region.
// Returns nil if the region has not been loaded yet.
func (l *Loader) Get(region string) []byte {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	entry := l.cache[region]
	l.mu.RUnlock()
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
	l.mu.RLock()
	entry := l.cache[region]
	l.mu.RUnlock()
	if entry == nil {
		return nil
	}
	return entry.view
}

// StartBackgroundRefresh spawns a goroutine that calls LoadAll on every interval.
// The goroutine exits when ctx is cancelled. Typical interval: 30 minutes.
func (l *Loader) StartBackgroundRefresh(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := l.LoadAll(ctx); err != nil && l.logger != nil {
					l.logger.WarnContext(ctx, "music metadata background refresh failed",
						"error_type", fmt.Sprintf("%T", err),
					)
				}
			}
		}
	}()
}
