package drawing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"haruki-cloud/config"

	"github.com/go-resty/resty/v2"
)

const (
	renderCacheFileExt    = "png"
	renderCachePublic     = "public"
	renderCacheKeyVersion = 2
)

// localRenderCache is an in-process TTL cache for rendered images.
// It is keyed by a stable hash of (endpoint, sanitized request payload) and
// avoids repeated Drawing API calls when the request has not changed.
type localRenderCache struct {
	mu      sync.RWMutex
	entries map[string]*localRenderEntry
	ttl     time.Duration
}

type localRenderEntry struct {
	data      []byte
	expiresAt time.Time
}

func newLocalRenderCache(ttl time.Duration) *localRenderCache {
	if ttl <= 0 {
		ttl = config.LocalRenderCacheTTL
	}
	return &localRenderCache{
		entries: make(map[string]*localRenderEntry),
		ttl:     ttl,
	}
}

func (lc *localRenderCache) buildKey(endpoint string, request interface{}) (string, error) {
	payload, err := normalizeRenderCachePayload(request)
	if err != nil {
		return "", err
	}
	sanitizeRenderCachePayload(endpoint, payload)

	b, err := json.Marshal(map[string]interface{}{
		"v":        renderCacheKeyVersion,
		"endpoint": endpoint,
		"payload":  payload,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
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

func (lc *localRenderCache) set(key string, data []byte) {
	entry := &localRenderEntry{
		data:      data,
		expiresAt: time.Now().Add(lc.ttl),
	}
	lc.mu.Lock()
	lc.entries[key] = entry
	lc.mu.Unlock()
}

// Render returns cached bytes for identical requests; otherwise calls render() and stores the result.
func (lc *localRenderCache) Render(endpoint string, request interface{}, render func() ([]byte, error)) ([]byte, error) {
	key, err := lc.buildKey(endpoint, request)
	if err != nil {
		return render()
	}
	if cached, ok := lc.get(key); ok {
		return cached, nil
	}
	data, err := render()
	if err != nil {
		return nil, err
	}
	lc.set(key, data)
	return data, nil
}

type RenderCacheConfig struct {
	BaseURL    string
	StorageDir string
	TTL        time.Duration
}

type RenderCacheClient struct {
	http       *resty.Client
	baseURL    string
	storageDir string
	ttl        time.Duration
}

type renderCacheRecord struct {
	Key       string `json:"key"`
	FilePath  string `json:"file_path"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
}

type renderCacheAPIError struct {
	Error string `json:"error"`
}

type renderCacheEndpoint struct {
	Path       string
	Query      neturl.Values
	Normalized string
}

type renderCachePolicy struct {
	Endpoint string
	APIPath  string
	UserID   string
	Params   any
}

type renderCacheKeyMaterial struct {
	Version  int    `json:"version"`
	Endpoint string `json:"endpoint"`
	APIPath  string `json:"api_path"`
	UserID   string `json:"user_id"`
	Params   any    `json:"params,omitempty"`
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

func (c *RenderCacheClient) Render(endpoint string, request interface{}, render func() ([]byte, error)) ([]byte, error) {
	if c == nil {
		return render()
	}

	policy, policyErr := buildRenderCachePolicy(endpoint, request)
	if policyErr == nil {
		key, keyErr := buildRenderCacheKey(policy)
		if keyErr == nil {
			if cached, hit := c.lookup(key); hit {
				return cached, nil
			}
		}

		image, err := render()
		if err != nil {
			return nil, err
		}

		if key, keyErr := buildRenderCacheKey(policy); keyErr == nil {
			_ = c.store(key, policy.APIPath, policy.UserID, image)
		}
		return image, nil
	}

	return render()
}

func (c *RenderCacheClient) lookup(key string) ([]byte, bool) {
	var record renderCacheRecord
	var apiErr renderCacheAPIError

	resp, err := c.http.R().
		SetQueryParam("key", key).
		SetResult(&record).
		SetError(&apiErr).
		Get(c.baseURL + "/cache")
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

func (c *RenderCacheClient) store(key string, apiPath string, userID string, image []byte) error {
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
			"ttl":       strconv.Itoa(int(math.Ceil(c.ttl.Seconds()))),
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

