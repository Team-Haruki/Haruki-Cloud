package drawing

import (
	"crypto/tls"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/utils/logger"

	"github.com/go-resty/resty/v2"
)

var cacheLogger = logger.NewLoggerFromGlobal("DrawingCache")

const (
	renderCacheFileExt    = "png"
	renderCachePublic     = "public"
	renderCacheKeyVersion = 3
)

func newLocalRenderCache(ttl time.Duration) *localRenderCache {
	if ttl <= 0 {
		ttl = config.LocalRenderCacheTTL
	}
	return &localRenderCache{
		entries: make(map[string]*localRenderEntry),
		ttl:     ttl,
	}
}

func (lc *localRenderCache) get(key string) ([]byte, bool) {
	lc.mu.RLock()
	entry, ok := lc.entries[key]
	lc.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if entry.permanent {
		return entry.data, true
	}
	if time.Now().After(entry.expiresAt) {
		lc.mu.Lock()
		delete(lc.entries, key)
		lc.mu.Unlock()
		return nil, false
	}
	return entry.data, true
}

func (lc *localRenderCache) set(key string, data []byte, ttl time.Duration, permanent bool) {
	if !permanent && ttl <= 0 {
		ttl = lc.ttl
	}
	entry := &localRenderEntry{
		data:      data,
		permanent: permanent,
	}
	if !permanent {
		entry.expiresAt = time.Now().Add(ttl)
	}
	lc.mu.Lock()
	lc.entries[key] = entry
	lc.mu.Unlock()
}

// Render returns cached bytes for identical requests; otherwise calls render() and stores the result.
// Concurrent calls with the same key are deduplicated via singleflight.
func (lc *localRenderCache) Render(endpoint string, request any, render func() ([]byte, error)) ([]byte, error) {
	policy, err := buildRenderCachePolicy(endpoint, request)
	if err != nil {
		return render()
	}
	key, err := buildRenderCacheKey(policy)
	if err != nil {
		return render()
	}
	ttl := policy.TTL
	if cached, ok := lc.get(key); ok {
		cacheLogger.Debugf("local cache hit: endpoint=%s", endpoint)
		return cached, nil
	}
	tRender := time.Now()
	v, err, _ := lc.flight.Do(key, func() (any, error) {
		if cached, ok := lc.get(key); ok {
			return cached, nil
		}
		data, err := render()
		if err != nil {
			return nil, err
		}
		lc.set(key, data, ttl, policy.Infinite)
		return data, nil
	})
	if err != nil {
		return nil, err
	}
	cacheLogger.Infof("local cache miss → rendered: endpoint=%s elapsed=%dms", endpoint, time.Since(tRender).Milliseconds())
	return v.([]byte), nil
}

func NewRenderCacheClient(cfg RenderCacheConfig) *RenderCacheClient {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	storageDir := strings.TrimSpace(cfg.StorageDir)
	if baseURL == "" || storageDir == "" || cfg.TTL <= 0 {
		return nil
	}

	return &RenderCacheClient{
		http: resty.New().
			SetTimeout(config.HTTPClientTimeout).
			SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}),
		baseURL:    baseURL,
		storageDir: storageDir,
		ttl:        cfg.TTL,
	}
}

func (c *RenderCacheClient) Render(endpoint string, request any, render func() ([]byte, error)) ([]byte, error) {
	if c == nil {
		return render()
	}

	policy, policyErr := buildRenderCachePolicy(endpoint, request)
	if policyErr == nil {
		key, keyErr := buildRenderCacheKey(policy)
		if keyErr == nil {
			v, err, _ := c.flight.Do(key, func() (any, error) {
				tLookup := time.Now()
				if cached, hit := c.lookup(key, policy.APIPath); hit {
					cacheLogger.Debugf(
						"remote cache hit: endpoint=%s api_path=%s user_id=%s key=%s lookup=%dms",
						endpoint,
						policy.APIPath,
						policy.UserID,
						shortRenderCacheKey(key),
						time.Since(tLookup).Milliseconds(),
					)
					return cached, nil
				}
				cacheLogger.Debugf(
					"remote cache miss: endpoint=%s api_path=%s user_id=%s key=%s lookup=%dms",
					endpoint,
					policy.APIPath,
					policy.UserID,
					shortRenderCacheKey(key),
					time.Since(tLookup).Milliseconds(),
				)

				tRender := time.Now()
				image, err := render()
				if err != nil {
					return nil, err
				}
				cacheLogger.Infof(
					"remote cache miss → rendered: endpoint=%s api_path=%s user_id=%s key=%s render=%dms",
					endpoint,
					policy.APIPath,
					policy.UserID,
					shortRenderCacheKey(key),
					time.Since(tRender).Milliseconds(),
				)

				ttl := policy.TTL
				if ttl <= 0 && !policy.Infinite {
					ttl = c.ttl
				}
				_ = c.store(key, policy.APIPath, policy.UserID, image, ttl, policy.Infinite)
				return image, nil
			})
			if err != nil {
				return nil, err
			}
			if data, ok := v.([]byte); ok {
				return data, nil
			}
			return nil, fmt.Errorf("remote render cache returned unexpected type %T", v)
		}
	}

	return render()
}

func shortRenderCacheKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 12 {
		return key
	}
	return key[:12]
}

func (c *RenderCacheClient) lookup(key string, apiPath string) ([]byte, bool) {
	var record renderCacheRecord
	var apiErr renderCacheAPIError

	request := c.http.R().
		SetQueryParam("key", key).
		SetResult(&record).
		SetError(&apiErr)
	if strings.TrimSpace(apiPath) != "" {
		request.SetQueryParam("api_path", apiPath)
	}
	resp, err := request.Get(c.baseURL + "/cache")
	if err != nil {
		return nil, false
	}
	if resp.StatusCode() != http.StatusOK || strings.TrimSpace(record.FilePath) == "" {
		return nil, false
	}

	body, err := os.ReadFile(record.FilePath)
	if err != nil {
		return nil, false
	}
	return body, true
}

func (c *RenderCacheClient) store(key string, apiPath string, userID string, image []byte, ttl time.Duration, infinite bool) error {
	if ttl <= 0 && !infinite {
		ttl = c.ttl
	}
	targetPath := c.defaultFilePath(apiPath, userID, key)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(targetPath, image, 0o644); err != nil {
		return err
	}

	var apiErr renderCacheAPIError
	ttlSeconds := "0"
	if !infinite {
		ttlSeconds = strconv.Itoa(int(math.Ceil(ttl.Seconds())))
	}

	resp, err := c.http.R().
		SetFormData(map[string]string{
			"key":       key,
			"ttl":       ttlSeconds,
			"api_path":  apiPath,
			"user_id":   userID,
			"ext":       renderCacheFileExt,
			"file_path": targetPath,
		}).
		SetError(&apiErr).
		Post(c.baseURL + "/cache")
	if err != nil || resp.StatusCode() != http.StatusOK {
		_ = os.Remove(targetPath)
		if err != nil {
			return err
		}
		return fmt.Errorf("cache register failed with status: %d", resp.StatusCode())
	}
	return nil
}

func (c *RenderCacheClient) defaultFilePath(apiPath string, userID string, key string) string {
	return filepath.Join(c.storageDir, filepath.FromSlash(apiPath), userID, key+"."+renderCacheFileExt)
}
