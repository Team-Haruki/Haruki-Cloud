package drawing

import (
	"container/list"
	"haruki-cloud/internal/core/urlhost"
	"haruki-cloud/internal/storage"
	"haruki-cloud/utils/imagecache"
	neturl "net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-resty/resty/v2"
	"golang.org/x/sync/singleflight"
)

// localRenderCache is an in-process TTL cache for rendered images.
// It is keyed by a stable hash of (endpoint, sanitized request payload) and
// avoids repeated Drawing API calls when the request has not changed.
type localRenderCache struct {
	mu             sync.Mutex
	entries        map[string]*localRenderEntry
	lru            *list.List
	nextGeneration uint64
	totalBytes     int64
	maxEntries     int
	maxBytes       int64
	ttl            time.Duration
	flight         singleflight.Group
}

type localRenderEntry struct {
	generation uint64
	data       []byte
	ref        *ArtifactRef // index mode: a pending ref instead of bytes
	expiresAt  time.Time
	permanent  bool
	size       int64
	element    *list.Element
}

type RenderCacheConfig struct {
	BaseURL       string
	StorageDir    string
	TTL           time.Duration
	ImageCacheDir string
	ImageStore    *imagecache.PGStore
	// Index enables index mode: lookups read render_cache_index. Assign it
	// only from a non-nil pointer (typed nils are ignored anyway).
	Index RenderIndex
	// Artifacts is the image_cache slot used to read a ref's bytes back.
	Artifacts storage.Store
	// Hosts are the public image-cache hosts, the fallback for ref bytes.
	Hosts *urlhost.Set
	// TouchInterval is the per-key sliding-TTL throttle (default 60s).
	TouchInterval time.Duration
	// FetchTimeout bounds one ref byte read (default 10s).
	FetchTimeout time.Duration
}

type RenderCacheClient struct {
	http          *resty.Client
	baseURL       string
	storageDir    string
	ttl           time.Duration
	imageCacheDir string
	imageStore    *imagecache.PGStore
	// index is non-nil in index mode; indexWriter batches its touches and
	// expired-row deletes, fetcher reads ref bytes back.
	index       RenderIndex
	indexWriter *renderIndexWriter
	indexErrLog atomic.Int64
	fetcher     *artifactFetcher
	flight      singleflight.Group
	readFlight  singleflight.Group
	pending     *localRenderCache
	// storeSlots bounds concurrent write-behind cache stores; storeWG lets
	// tests (and future shutdown hooks) wait for pending stores to drain.
	storeSlots chan struct{}
	storeWG    sync.WaitGroup
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
