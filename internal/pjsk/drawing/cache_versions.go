package drawing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-resty/resty/v2"
	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/httpcoding"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"
)

type ResourceVersionSource interface {
	Revision() string
	RevisionForPayload(any) string
}
type CacheVersionConfig struct {
	Enabled       bool
	PollInterval  time.Duration
	MaxStale      time.Duration
	RendererEpoch string
}
type rendererIdentities struct {
	epochs  map[string]string
	checked map[string]time.Time
}
type cacheVersions struct {
	cfg       CacheVersionConfig
	resources ResourceVersionSource
	targets   []upstream.TargetConfig
	http      *http.Client
	// coding learns each node's zstd advertisement from the identity polls,
	// before the first render request goes out.
	coding  *httpcoding.Negotiator
	current atomic.Pointer[rendererIdentities]
	cancel  context.CancelFunc
	done    chan struct{}
	close   sync.Once
}
type versionSnapshot struct {
	asset, scoped, renderer string
	epochs                  map[string]string
	ready                   bool
}
type versionContextKey struct{}

// WithCacheVersions starts an application-scoped renderer identity watcher.
// Legacy callers that omit it retain the original cache-key contract.
// Enabled clients must configure artifact endpoints ["*"] before this option;
// Drawing only interprets resource revisions inside a C13 directive.
func WithCacheVersions(ctx context.Context, cfg CacheVersionConfig, resources ResourceVersionSource, targets []upstream.TargetConfig) ClientOption {
	return func(client *resty.Client, c *HarukiDrawingClient) {
		if !cfg.Enabled {
			return
		}
		if cfg.PollInterval <= 0 {
			cfg.PollInterval = 30 * time.Second
		}
		if cfg.MaxStale <= 0 {
			cfg.MaxStale = 2 * time.Minute
		}
		if ctx == nil {
			ctx = context.Background()
		}
		base, cancel := context.WithCancel(ctx)
		v := &cacheVersions{cfg: cfg, resources: resources, targets: upstream.ResolveTargets(c.baseURL, targets, "drawing"), http: &http.Client{Transport: client.GetClient().Transport, Timeout: 3 * time.Second}, coding: c.coding, cancel: cancel, done: make(chan struct{})}
		c.versions = v
		go func() {
			defer close(v.done)
			for {
				v.refresh(base)
				timer := time.NewTimer(cfg.PollInterval)
				select {
				case <-base.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}()
	}
}
func (v *cacheVersions) refresh(ctx context.Context) {
	next := &rendererIdentities{epochs: map[string]string{}, checked: map[string]time.Time{}}
	if old := v.current.Load(); old != nil {
		for k, e := range old.epochs {
			next.epochs[k] = e
			next.checked[k] = old.checked[k]
		}
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, target := range v.targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			epoch := strings.ToLower(strings.TrimSpace(v.cfg.RendererEpoch))
			if !isHex64(epoch) {
				var err error
				epoch, err = v.identity(ctx, target.BaseURL)
				if err != nil {
					return
				}
			}
			mu.Lock()
			next.epochs[target.BaseURL] = epoch
			next.checked[target.BaseURL] = time.Now()
			mu.Unlock()
		}()
	}
	wg.Wait()
	v.current.Store(next)
}
func (v *cacheVersions) identity(ctx context.Context, baseURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/cache/identity", nil)
	if err != nil {
		return "", err
	}
	resp, err := v.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	v.coding.Observe(baseURL, resp.Header)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("drawing identity status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	if err != nil || len(body) > 4096 {
		return "", fmt.Errorf("drawing identity invalid body")
	}
	var identity struct {
		Version       int    `json:"version"`
		RendererEpoch string `json:"renderer_epoch"`
	}
	if err = json.Unmarshal(body, &identity); err != nil {
		return "", err
	}
	if identity.Version != 1 || !isHex64(identity.RendererEpoch) || strings.ToLower(identity.RendererEpoch) != identity.RendererEpoch {
		return "", fmt.Errorf("drawing identity invalid version")
	}
	return identity.RendererEpoch, nil
}
func hashVersion(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
func (v *cacheVersions) snapshot(payload any) *versionSnapshot {
	if v == nil {
		return nil
	}
	s := &versionSnapshot{epochs: map[string]string{}, ready: len(v.targets) > 0}
	if v.resources != nil {
		if atomicSource, ok := v.resources.(interface{ SnapshotForPayload(any) (string, string) }); ok {
			s.asset, s.scoped = atomicSource.SnapshotForPayload(payload)
		} else {
			s.asset = v.resources.Revision()
			s.scoped = v.resources.RevisionForPayload(payload)
		}
	}
	current := v.current.Load()
	tokens := make([]string, 0, len(v.targets))
	for _, target := range v.targets {
		if current == nil || current.epochs[target.BaseURL] == "" || time.Since(current.checked[target.BaseURL]) > v.cfg.MaxStale {
			s.ready = false
			continue
		}
		epoch := current.epochs[target.BaseURL]
		s.epochs[target.BaseURL] = epoch
		tokens = append(tokens, target.BaseURL+":"+epoch)
	}
	sort.Strings(tokens)
	s.renderer = hashVersion(strings.Join(tokens, "\n"))
	return s
}
func versionFrom(ctx context.Context) *versionSnapshot {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(versionContextKey{}).(*versionSnapshot)
	return s
}
func attachVersions(ctx context.Context, s *versionSnapshot) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, versionContextKey{}, s)
}
func versionedCacheKey(ctx context.Context, key string) string {
	s := versionFrom(ctx)
	if s == nil {
		return key
	}
	return hashVersion("render-versions-v1\n" + key + "\n" + s.scoped + "\n" + s.renderer)
}
func (v *cacheVersions) Close() {
	if v != nil {
		v.close.Do(func() { v.cancel(); <-v.done })
	}
}
func (c *HarukiDrawingClient) applyVersionHeaders(ctx context.Context, base string, request *resty.Request, directive *renderDirective) {
	s := versionFrom(ctx)
	if s == nil || directive == nil {
		return
	}
	directive.KeyVersion = 6
	request.SetHeader(headerCacheKeyVersion, "6")
	if s.asset != "" {
		request.SetHeader("X-Haruki-Asset-Revision", s.asset)
	}
	if epoch := s.epochs[strings.TrimRight(strings.TrimSpace(base), "/")]; epoch != "" {
		request.SetHeader("X-Haruki-Renderer-Epoch", epoch)
	}
	if !s.ready {
		request.SetHeader(headerCacheStore, "0")
		directive.Store = false
		commandtrace.RecordOperation(ctx, "drawing.cache_identity_unknown", 0)
	}
}

// responseCacheability follows the response independently of artifact mode,
// so a local bytes cache cannot retain a placeholder or a render made by a
// node whose renderer identity changed after the last poll. placeholders is
// the X-Haruki-Render-Missing-Assets count: such a render is cached only for
// the short placeholder TTL.
type responseCacheability struct {
	noStore      atomic.Bool
	placeholders atomic.Int64
}
type responseCacheabilityKey struct{}

func withResponseCacheability(ctx context.Context) context.Context {
	return context.WithValue(ctx, responseCacheabilityKey{}, &responseCacheability{})
}
func markRenderNoStore(ctx context.Context) {
	if ctx != nil {
		if state, ok := ctx.Value(responseCacheabilityKey{}).(*responseCacheability); ok {
			state.noStore.Store(true)
		}
	}
}
func renderNoStore(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	state, _ := ctx.Value(responseCacheabilityKey{}).(*responseCacheability)
	return state != nil && state.noStore.Load()
}
