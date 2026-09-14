// Package imagecache provides a content-addressable image store that writes
// rendered images to a local directory and returns a CDN-based URL suitable
// for use in OneBot11 image message segments.
//
// An optional PGStore can be attached for PostgreSQL-backed deduplication:
// repeated writes for the same image content are short-circuited by returning
// the previously computed CDN URL directly from the database.
package imagecache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/core/urlhost"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/storage"
	"haruki-cloud/utils/logger"

	"golang.org/x/sync/singleflight"
)

const imageStoreSharedTimeout = 30 * time.Second

// entryTouchInterval rate-limits last_referenced_at bumps per hash; it only
// needs to be far below gc_object_retention_days. entryTouchMemoCap bounds the
// per-client memo.
const (
	entryTouchInterval = time.Hour
	entryTouchMemoCap  = 4096
)

// ErrNoHosts is returned by NewClient when no public image host is configured.
var ErrNoHosts = errors.New("imagecache: no public hosts configured")

// ErrNoObjectStore is returned by NewClient when the image_cache slot is absent
// or Disabled.
var ErrNoObjectStore = errors.New("imagecache: no object store configured")

// ClientConfig wires a Client at the composition root.
type ClientConfig struct {
	// Hosts selects the public base URL of every emitted URL; required.
	Hosts *urlhost.Set
	// Objects is the image_cache slot; required (Disabled() is rejected).
	Objects storage.Store
	// LocalRoot is the absolute directory Objects writes to when the slot is
	// local, "" otherwise. It only fills file_path / storage_backend and gates
	// the legacy_disk existence probe; it is never sniffed from Objects.
	LocalRoot string
	// Index is the optional PostgreSQL deduplication store.
	Index *PGStore
}

// Client stores images on the image_cache slot and returns public URLs.
// A nil Client is safe to use — all methods become no-ops or return errors.
type Client struct {
	hosts     *urlhost.Set
	objects   storage.Store
	localRoot string
	store     *PGStore // optional PostgreSQL deduplication store
	flight    singleflight.Group

	now       func() time.Time
	touchMu   sync.Mutex
	lastTouch map[string]time.Time
}

type storeFlightToken byte

type storeFlightResult struct {
	url        string
	err        error
	operations []commandtrace.Stats
	leader     *storeFlightToken
}

// NewClient builds a Client from explicit composition-root inputs.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.Hosts.Len() == 0 {
		return nil, ErrNoHosts
	}
	if cfg.Objects == nil || cfg.Objects == storage.Disabled() {
		return nil, ErrNoObjectStore
	}
	return &Client{
		hosts:     cfg.Hosts,
		objects:   cfg.Objects,
		localRoot: strings.TrimSpace(cfg.LocalRoot),
		store:     cfg.Index,
		now:       time.Now,
		lastTouch: make(map[string]time.Time),
	}, nil
}

// New returns a new Client. Returns nil if uri or dir is empty.
func New(uri, dir string) *Client {
	return NewWithStore(uri, dir, nil)
}

// NewWithStore returns a Client that writes below dir, emits URLs under uri
// and deduplicates through store (which may be nil). Returns nil if uri or dir
// is empty.
func NewWithStore(uri, dir string, store *PGStore) *Client {
	uri = strings.TrimSpace(uri)
	dir = strings.TrimSpace(dir)
	if uri == "" || dir == "" {
		return nil
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	objects, err := storage.NewLocal(root, 0)
	if err != nil {
		return nil
	}
	client, err := NewClient(ClientConfig{Hosts: urlhost.Single(uri), Objects: objects, LocalRoot: root, Index: store})
	if err != nil {
		return nil
	}
	return client
}

func (c *Client) Close() error {
	if c == nil || c.store == nil {
		return nil
	}
	return c.store.Close()
}

// StoreAndGetURL returns the public URL for data, writing it to the object
// store if needed. If a PGStore is configured and the hash is already known,
// the write is skipped and the indexed URL is returned directly.
// group is a slash-separated path component, e.g. "pjsk/profile".
func (c *Client) StoreAndGetURL(ctx context.Context, data []byte, group string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("imagecache: client is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	group, err := normalizeImageGroup(group)
	if err != nil {
		return "", err
	}

	finishHash := commandtrace.MeasureOperation(ctx, "image.hash")
	// Shared work may outlive this caller after it returns on cancellation. Own
	// the bytes before hashing so neither the content-derived path nor the
	// eventual file write can observe caller mutations.
	ownedData := bytes.Clone(data)
	digest := sha256.Sum256(ownedData)
	hashHex := hex.EncodeToString(digest[:])
	name := hashHex + extFromData(ownedData)
	finishHash()
	if err := ctx.Err(); err != nil {
		return "", err
	}

	urlPath := path.Join(group, name)

	// The PostgreSQL index deduplicates by content hash across groups. Without
	// it, the object key is the deduplication boundary.
	flightKey := urlPath
	if c.store != nil {
		flightKey = hashHex
	}
	callerToken := new(storeFlightToken)
	finishWait := commandtrace.MeasureOperation(ctx, "image.wait")
	resultCh := c.flight.DoChan(flightKey, func() (any, error) {
		sharedCtx, cancel := imageStoreSharedContext()
		defer cancel()
		sharedCtx, trace := commandtrace.WithNewTrace(sharedCtx)
		url, err := c.storeHashed(sharedCtx, ownedData, hashHex, group, urlPath)
		return storeFlightResult{
			url:        url,
			err:        err,
			operations: trace.Snapshot().Operations,
			leader:     callerToken,
		}, nil
	})

	select {
	case <-ctx.Done():
		finishWait()
		return "", ctx.Err()
	case completed := <-resultCh:
		finishWait()
		if completed.Err != nil {
			return "", completed.Err
		}
		resolved, ok := completed.Val.(storeFlightResult)
		if !ok {
			return "", fmt.Errorf("imagecache: unexpected singleflight result %T", completed.Val)
		}
		commandtrace.MergeOperations(ctx, resolved.operations)
		if resolved.leader != callerToken {
			commandtrace.RecordOperation(ctx, "image.shared", 0)
		}
		return resolved.url, resolved.err
	}
}

func normalizeImageGroup(group string) (string, error) {
	group = strings.ReplaceAll(strings.TrimSpace(group), "\\", "/")
	if group == "" || path.IsAbs(group) {
		return "", fmt.Errorf("imagecache: group must be a non-empty relative path")
	}
	cleaned := path.Clean(group)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("imagecache: group escapes the cache directory")
	}
	return cleaned, nil
}

func (c *Client) storeHashed(ctx context.Context, data []byte, hashHex string, group string, urlPath string) (string, error) {
	finishLookup := commandtrace.MeasureOperation(ctx, "image.lookup")
	if url, ok := c.lookupIndexed(ctx, hashHex); ok {
		finishLookup()
		return url, nil
	}

	key := storage.Key(urlPath)
	_, statErr := c.objects.Stat(ctx, key)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, storage.ErrNotExist) {
		finishLookup()
		return "", fmt.Errorf("imagecache: stat %s: %w", urlPath, statErr)
	}
	finishLookup()

	if !exists {
		finishWrite := commandtrace.MeasureOperation(ctx, "image.write")
		// Content type only: node Caddy owns Cache-Control for the bucket.
		err := c.objects.Put(ctx, key, data, storage.PutOptions{ContentType: mediaTypeFromPath(urlPath)})
		finishWrite()
		if err != nil {
			return "", fmt.Errorf("imagecache: write %s: %w", urlPath, err)
		}
	}

	// Record the relative path (no domain) only after the object exists:
	// Drawing trusts an indexed row and skips its own upload. Storing only the
	// path means changing the public hosts in config re-bases every URL.
	if c.store != nil {
		finishIndex := commandtrace.MeasureOperation(ctx, "image.index")
		err := c.store.InsertEntry(ctx, c.entryFor(hashHex, group, urlPath, int64(len(data))))
		finishIndex()
		if err != nil {
			logger.ErrorContext(ctx, "image cache index insert failed", "error", err)
		}
	}
	return c.url(urlPath)
}

// lookupIndexed returns the indexed URL for hashHex. A legacy_disk row on a
// local slot is only trusted while its object still exists; garage rows are
// trusted outright (GC keeps them consistent).
func (c *Client) lookupIndexed(ctx context.Context, hashHex string) (string, bool) {
	if c.store == nil {
		return "", false
	}
	entry, ok, err := c.store.Lookup(ctx, hashHex)
	if err != nil {
		logger.ErrorContext(ctx, "image cache index lookup failed", "error", err)
		return "", false
	}
	if !ok {
		return "", false
	}
	if entry.StorageBackend == BackendLegacyDisk && c.localRoot != "" {
		if _, statErr := c.objects.Stat(ctx, storage.Key(entry.CDNPath)); statErr != nil {
			// Object was deleted (or is unreadable); fall through to re-write it.
			return "", false
		}
	}
	url, urlErr := c.url(entry.CDNPath)
	if urlErr != nil {
		return "", false
	}
	if entry.StorageBackend == BackendGarage {
		c.touchEntry(ctx, hashHex)
	}
	return url, true
}

// touchEntry bumps last_referenced_at on a garage dedup hit, at most once per
// hash per entryTouchInterval: the re-emitted URL must outlive GC's retention
// window (addendum A6). A failure is logged and the memo is not advanced.
func (c *Client) touchEntry(ctx context.Context, hashHex string) {
	now := c.now()
	c.touchMu.Lock()
	if last, ok := c.lastTouch[hashHex]; ok && now.Sub(last) < entryTouchInterval {
		c.touchMu.Unlock()
		return
	}
	if len(c.lastTouch) >= entryTouchMemoCap {
		for key, last := range c.lastTouch {
			if now.Sub(last) >= entryTouchInterval {
				delete(c.lastTouch, key)
			}
		}
		if len(c.lastTouch) >= entryTouchMemoCap {
			clear(c.lastTouch)
		}
	}
	c.lastTouch[hashHex] = now
	c.touchMu.Unlock()

	if err := c.store.TouchEntry(ctx, hashHex); err != nil {
		c.touchMu.Lock()
		if c.lastTouch[hashHex].Equal(now) {
			delete(c.lastTouch, hashHex)
		}
		c.touchMu.Unlock()
		logger.ErrorContext(ctx, "image cache index touch failed", "error", err)
	}
}

func (c *Client) entryFor(hashHex, group, urlPath string, size int64) ImageEntry {
	entry := ImageEntry{
		Hash: hashHex, GroupName: group, CDNPath: urlPath,
		StorageBackend: BackendGarage, MediaType: mediaTypeFromPath(urlPath), SizeBytes: size,
	}
	if c.localRoot != "" {
		entry.FilePath = filepath.Join(c.localRoot, filepath.FromSlash(urlPath))
		entry.StorageBackend = BackendLegacyDisk
	}
	return entry
}

func (c *Client) url(relPath string) (string, error) {
	url, ok := c.hosts.URL("", relPath)
	if !ok {
		return "", ErrNoHosts
	}
	return url, nil
}

func imageStoreSharedContext() (context.Context, context.CancelFunc) {
	shared := logger.WithContextAttrs(context.Background(), slog.Bool("shared_work", true))
	return context.WithTimeout(shared, imageStoreSharedTimeout)
}

// mediaTypeFromPath maps a stored image name to its Content-Type.
func mediaTypeFromPath(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

// extFromData sniffs the first 512 bytes of data to determine the file extension.
func extFromData(data []byte) string {
	sniff := data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	switch http.DetectContentType(sniff) {
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}
