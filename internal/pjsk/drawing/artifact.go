package drawing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/core/urlhost"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/storage"
	"haruki-cloud/utils/logger"

	"github.com/go-resty/resty/v2"
	"golang.org/x/sync/singleflight"
)

const (
	artifactRefKind              = "artifact_ref"
	artifactStorageGarage        = "garage"
	artifactStorageLegacyDisk    = "legacy_disk"
	defaultArtifactFetchTimeout  = 10 * time.Second
	defaultArtifactRenderTimeout = 15 * time.Second
	artifactAllowAll             = "*"
)

// ErrArtifactBytesUnavailable reports that neither the image_cache store nor
// the public image hosts could produce the bytes of an artifact ref.
var ErrArtifactBytesUnavailable = errors.New("drawing artifact bytes are unavailable")

// errDrawingBadArtifact marks a JSON body that claims to be an artifact ref
// but fails validation. It is a render error: the body is never image bytes.
var errDrawingBadArtifactRef = errors.New("drawing returned an invalid artifact ref")

func errDrawingBadArtifact(err error) error {
	return fmt.Errorf("%w: %w", errDrawingBadArtifactRef, err)
}

// ArtifactRef is Drawing's JSON answer to an artifact-mode render: the
// rendered image lives in the image-cache bucket at CDNPath.
type ArtifactRef struct {
	Kind           string  `json:"kind"`
	Hash           string  `json:"hash"`
	CDNPath        string  `json:"cdn_path"`
	StorageBackend string  `json:"storage_backend"`
	Bucket         string  `json:"bucket"`
	ObjectKey      string  `json:"object_key"`
	SizeBytes      int64   `json:"size_bytes"`
	MediaType      string  `json:"media_type"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	CacheKey       string  `json:"cache_key"`
	TTLSeconds     int64   `json:"ttl_seconds"`
	ExpiresAt      *string `json:"expires_at"` // RFC3339; null or "" = infinite
	Reused         bool    `json:"reused"`
	IndexWritten   bool    `json:"index_written"`
	UploadElapsed  float64 `json:"upload_elapsed"`
	NodeName       string  `json:"node_name"` // rendering node; host preference
}

// parseArtifactRef decodes and validates an artifact ref body. cdn_path only
// has to be a valid storage key: reused refs may point at rows that are not
// shaped pjsk/<api_path>/<sha256>.<ext>.
func parseArtifactRef(body []byte) (*ArtifactRef, error) {
	var ref ArtifactRef
	if err := json.Unmarshal(body, &ref); err != nil {
		return nil, fmt.Errorf("decode artifact ref: %w", err)
	}
	if ref.Kind != artifactRefKind {
		return nil, fmt.Errorf("artifact ref kind %q", ref.Kind)
	}
	if !isHex64(ref.Hash) {
		return nil, fmt.Errorf("artifact ref hash is not 64 hex characters")
	}
	if err := validateArtifactCDNPath(ref.CDNPath); err != nil {
		return nil, err
	}
	switch ref.StorageBackend {
	case artifactStorageGarage, artifactStorageLegacyDisk:
	case "":
		ref.StorageBackend = artifactStorageGarage
		cacheLogger.Debug("artifact ref without storage_backend, assuming garage", "cdn_path", ref.CDNPath)
	default:
		return nil, fmt.Errorf("artifact ref storage_backend %q", ref.StorageBackend)
	}
	if ref.MediaType != "" && !strings.HasPrefix(ref.MediaType, "image/") {
		return nil, fmt.Errorf("artifact ref media_type %q is not an image", ref.MediaType)
	}
	if ref.SizeBytes < 0 {
		return nil, fmt.Errorf("artifact ref size_bytes is negative")
	}
	if _, err := ref.ExpiresAtTime(); err != nil {
		return nil, err
	}
	if ref.CacheKey != "" && !isHex64(ref.CacheKey) {
		return nil, fmt.Errorf("artifact ref cache_key is not 64 hex characters")
	}
	return &ref, nil
}

func validateArtifactCDNPath(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("artifact ref cdn_path is empty")
	}
	if strings.HasPrefix(raw, "/") || strings.Contains(raw, "\\") {
		return fmt.Errorf("artifact ref cdn_path %q is not a relative key", raw)
	}
	key, err := storage.CleanKey(raw)
	if err != nil {
		return fmt.Errorf("artifact ref cdn_path: %w", err)
	}
	if string(key) != raw {
		return fmt.Errorf("artifact ref cdn_path %q is not canonical", raw)
	}
	return nil
}

// ExpiresAtTime returns the expiry; nil or "" (infinite) is the zero time.
func (r *ArtifactRef) ExpiresAtTime() (time.Time, error) {
	if r == nil || r.ExpiresAt == nil || strings.TrimSpace(*r.ExpiresAt) == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*r.ExpiresAt))
	if err != nil {
		return time.Time{}, fmt.Errorf("artifact ref expires_at: %w", err)
	}
	return parsed, nil
}

// EscapedCDNPath returns cdn_path with every segment path-escaped, ready to
// join onto a public host base URL.
func (r *ArtifactRef) EscapedCDNPath() string {
	if r == nil {
		return ""
	}
	return escapeCDNPath(r.CDNPath)
}

func escapeCDNPath(cdnPath string) string {
	segments := strings.Split(cdnPath, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}

func isHex64(value string) bool {
	if len(value) != 64 {
		return false
	}
	for i := 0; i < len(value); i++ {
		switch c := value[i]; {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// ArtifactConfig is pjsk_render.drawing_artifact plus the dependencies the
// composition root injects for reading artifact bytes back.
type ArtifactConfig struct {
	// Endpoints lists normalised api paths ("api/pjsk/card/box") that run in
	// artifact mode; "*" enables every endpoint. Empty = artifact mode off.
	Endpoints []string
	// FetchTimeout bounds one Bytes() read of a ref-only result (default 10s).
	FetchTimeout time.Duration
	// ArtifactTimeout extends the shared render budget for Drawing's upload
	// in artifact mode (default 15s).
	ArtifactTimeout time.Duration
	// Objects is the image_cache slot; nil or Disabled skips the store rung.
	Objects storage.Store
	// Hosts are the public image-cache hosts named by Drawing node.
	Hosts *urlhost.Set
}

// WithArtifactConfig enables artifact mode for the allow-listed endpoints.
// An empty allow-list leaves the client byte-identical to a client without it.
func WithArtifactConfig(cfg ArtifactConfig) ClientOption {
	return func(_ *resty.Client, drawingClient *HarukiDrawingClient) {
		if drawingClient == nil {
			return
		}
		drawingClient.artifact = newArtifactSettings(cfg)
	}
}

// artifactAllowList matches normalised api paths exactly, or everything with "*".
type artifactAllowList struct {
	all   bool
	paths map[string]struct{}
}

func newArtifactAllowList(endpoints []string) artifactAllowList {
	list := artifactAllowList{paths: make(map[string]struct{}, len(endpoints))}
	for _, raw := range endpoints {
		trimmed := strings.TrimSpace(raw)
		if trimmed == artifactAllowAll {
			list.all = true
			continue
		}
		if parsed, err := url.Parse(trimmed); err == nil {
			trimmed = parsed.Path
		}
		if apiPath := normalizeRenderCacheAPIPath(trimmed); apiPath != "" {
			list.paths[apiPath] = struct{}{}
		}
	}
	return list
}

func (l artifactAllowList) empty() bool { return !l.all && len(l.paths) == 0 }

func (l artifactAllowList) has(apiPath string) bool {
	if apiPath == "" {
		return false
	}
	if l.all {
		return true
	}
	_, ok := l.paths[apiPath]
	return ok
}

// artifactSettings is the immutable artifact-mode state shared by every
// clone of a drawing client.
type artifactSettings struct {
	allow           artifactAllowList
	artifactTimeout time.Duration
	fetcher         *artifactFetcher
}

func newArtifactSettings(cfg ArtifactConfig) *artifactSettings {
	allow := newArtifactAllowList(cfg.Endpoints)
	if allow.empty() {
		return nil
	}
	artifactTimeout := cfg.ArtifactTimeout
	if artifactTimeout <= 0 {
		artifactTimeout = defaultArtifactRenderTimeout
	}
	return &artifactSettings{
		allow:           allow,
		artifactTimeout: artifactTimeout,
		fetcher:         newArtifactFetcher(cfg.Objects, cfg.Hosts, cfg.FetchTimeout),
	}
}

func (s *artifactSettings) allowsEndpoint(endpoint string) bool {
	if s == nil {
		return false
	}
	parsed, err := parseRenderCacheEndpoint(endpoint)
	if err != nil {
		return false
	}
	return s.allow.has(normalizeRenderCacheAPIPath(parsed.Path))
}

type artifactModeCtxKey struct{}

// withArtifactMode marks ctx for a cached render of an allow-listed endpoint:
// the shared flight budget grows by ArtifactTimeout and renderRemoteMiss
// attaches the directive.
func (c *HarukiDrawingClient) withArtifactMode(ctx context.Context, endpoint string) context.Context {
	if c == nil || c.cache == nil || !c.artifact.allowsEndpoint(endpoint) {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, artifactModeCtxKey{}, c.artifact)
}

func artifactModeFrom(ctx context.Context) *artifactSettings {
	if ctx == nil {
		return nil
	}
	settings, _ := ctx.Value(artifactModeCtxKey{}).(*artifactSettings)
	return settings
}

// artifactFetcher resolves the bytes behind a ref: the image_cache store
// first, then the public hosts (preferring the rendering node).
type artifactFetcher struct {
	objects storage.Store
	hosts   *urlhost.Set
	timeout time.Duration
	http    *http.Client
	flight  singleflight.Group
	logger  *logger.Logger
}

func newArtifactFetcher(objects storage.Store, hosts *urlhost.Set, timeout time.Duration) *artifactFetcher {
	if timeout <= 0 {
		timeout = defaultArtifactFetchTimeout
	}
	return &artifactFetcher{
		objects: objects,
		hosts:   hosts,
		timeout: timeout,
		http:    &http.Client{Transport: upstream.NewTunedTransport(upstream.TunedTransportConfig{})},
		logger:  cacheLogger,
	}
}

func (f *artifactFetcher) fetch(ctx context.Context, ref *ArtifactRef) ([]byte, error) {
	if f == nil || ref == nil {
		return nil, ErrArtifactBytesUnavailable
	}
	result := f.flight.DoChan(ref.Hash+"|"+ref.CDNPath, func() (any, error) {
		flightCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), f.timeout)
		defer cancel()
		return f.fetchOnce(flightCtx, ref)
	})
	select {
	case completed := <-result:
		if completed.Err != nil {
			return nil, completed.Err
		}
		data, _ := completed.Val.([]byte)
		return cloneRenderBytes(data), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *artifactFetcher) fetchOnce(ctx context.Context, ref *ArtifactRef) ([]byte, error) {
	var storeErr error
	if f.objects != nil {
		data, err := f.objects.Get(ctx, storage.Key(ref.CDNPath))
		if err == nil && len(data) <= drawingMaxResponseBytes {
			return data, nil
		}
		if err == nil {
			err = fmt.Errorf("artifact exceeds %d bytes", drawingMaxResponseBytes)
		}
		storeErr = err
		f.logger.DebugContext(ctx, "artifact store read failed, trying public hosts",
			"cdn_path", ref.CDNPath, "error", err)
	}
	if f.hosts.Len() == 0 {
		return nil, unavailableArtifact(storeErr)
	}
	prefer := ref.NodeName
	var lastErr error
	for range 2 {
		base := f.hosts.Base(prefer)
		data, transportErr, err := f.fetchHost(ctx, base, ref.CDNPath)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if transportErr {
			f.hosts.MarkFailure(hostNameFor(f.hosts, base))
		}
		if ctx.Err() != nil {
			break
		}
		prefer = ""
	}
	return nil, unavailableArtifact(errors.Join(storeErr, lastErr))
}

func unavailableArtifact(cause error) error {
	if cause == nil {
		return ErrArtifactBytesUnavailable
	}
	return fmt.Errorf("%w: %w", ErrArtifactBytesUnavailable, cause)
}

// fetchHost GETs one artifact from base. transportErr reports a failure to
// reach the host (the only case that puts it into cooldown); a non-200 answer
// such as a 404 inside the replication window is not a host fault.
func (f *artifactFetcher) fetchHost(ctx context.Context, base, cdnPath string) (data []byte, transportErr bool, err error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/"+escapeCDNPath(cdnPath), nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := f.http.Do(request)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("artifact host returned status %d", resp.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, drawingMaxResponseBytes+1))
	if err != nil {
		return nil, true, err
	}
	if len(data) > drawingMaxResponseBytes {
		return nil, false, fmt.Errorf("artifact exceeds %d bytes", drawingMaxResponseBytes)
	}
	return data, false, nil
}

func hostNameFor(hosts *urlhost.Set, base string) string {
	for _, host := range hosts.Hosts() {
		if host.BaseURL == base {
			return host.Name
		}
	}
	return ""
}
