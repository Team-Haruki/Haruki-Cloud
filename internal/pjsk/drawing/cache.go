package drawing

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/config"

	"github.com/go-resty/resty/v2"
)

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
	if time.Now().After(entry.expiresAt) {
		lc.mu.Lock()
		delete(lc.entries, key)
		lc.mu.Unlock()
		return nil, false
	}
	return entry.data, true
}

func (lc *localRenderCache) set(key string, data []byte, ttl time.Duration) {
	if ttl <= 0 {
		ttl = lc.ttl
	}
	entry := &localRenderEntry{
		data:      data,
		expiresAt: time.Now().Add(ttl),
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
		return cached, nil
	}
	v, err, _ := lc.flight.Do(key, func() (any, error) {
		if cached, ok := lc.get(key); ok {
			return cached, nil
		}
		data, err := render()
		if err != nil {
			return nil, err
		}
		lc.set(key, data, ttl)
		return data, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]byte), nil
}

func NewRenderCacheClient(cfg RenderCacheConfig) *RenderCacheClient {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	storageDir := strings.TrimSpace(cfg.StorageDir)
	if baseURL == "" || storageDir == "" || cfg.TTL <= 0 {
		return nil
	}

	return &RenderCacheClient{
		http:       resty.New().SetTimeout(config.HTTPClientTimeout),
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
			if cached, hit := c.lookup(key, policy.APIPath); hit {
				return cached, nil
			}
		}

		image, err := render()
		if err != nil {
			return nil, err
		}

		if keyErr == nil {
			ttl := policy.TTL
			if ttl <= 0 {
				ttl = c.ttl
			}
			_ = c.store(key, policy.APIPath, policy.UserID, image, ttl)
		}
		return image, nil
	}

	return render()
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

func (c *RenderCacheClient) store(key string, apiPath string, userID string, image []byte, ttl time.Duration) error {
	if ttl <= 0 {
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
	resp, err := c.http.R().
		SetFormData(map[string]string{
			"key":       key,
			"ttl":       strconv.Itoa(int(math.Ceil(ttl.Seconds()))),
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
