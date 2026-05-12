package drawing

import (
	neturl "net/url"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"golang.org/x/sync/singleflight"
)

// localRenderCache is an in-process TTL cache for rendered images.
// It is keyed by a stable hash of (endpoint, sanitized request payload) and
// avoids repeated Drawing API calls when the request has not changed.
type localRenderCache struct {
	mu      sync.RWMutex
	entries map[string]*localRenderEntry
	ttl     time.Duration
	flight  singleflight.Group
}

type localRenderEntry struct {
	data      []byte
	expiresAt time.Time
	permanent bool
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
	flight     singleflight.Group
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
	TTL      time.Duration
	Infinite bool
}

type renderCacheKeyMaterial struct {
	Version  int    `json:"version"`
	Endpoint string `json:"endpoint"`
	APIPath  string `json:"api_path"`
	UserID   string `json:"user_id"`
	Params   any    `json:"params,omitempty"`
}
