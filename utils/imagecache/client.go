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
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"

	"golang.org/x/sync/singleflight"
)

const imageStoreSharedTimeout = 30 * time.Second

// Client writes images to a local directory and returns CDN URLs.
// A nil Client is safe to use — all methods become no-ops or return errors.
type Client struct {
	uri    string   // CDN base URI, e.g. "https://image-cache.example.com"
	dir    string   // Local root directory for stored images
	store  *PGStore // optional PostgreSQL deduplication store
	flight singleflight.Group
	write  func(context.Context, string, []byte) error
}

type storeFlightToken byte

type storeFlightResult struct {
	url        string
	err        error
	operations []commandtrace.Stats
	leader     *storeFlightToken
}

// New returns a new Client. Returns nil if uri or dir is empty.
func New(uri, dir string) *Client {
	return NewWithStore(uri, dir, nil)
}

// NewWithStore returns a Client backed by the given PGStore for deduplication.
// store may be nil (disables DB deduplication). Returns nil if uri or dir is empty.
func NewWithStore(uri, dir string, store *PGStore) *Client {
	uri = strings.TrimRight(strings.TrimSpace(uri), "/")
	dir = strings.TrimSpace(dir)
	if uri == "" || dir == "" {
		return nil
	}
	return &Client{uri: uri, dir: dir, store: store, write: writeFileAtomically}
}

func (c *Client) Close() error {
	if c == nil || c.store == nil {
		return nil
	}
	return c.store.Close()
}

// StoreAndGetURL returns the CDN URL for data, writing it to disk if needed.
// If a PGStore is configured and the hash is already known, the filesystem write
// is skipped and the cached URL is returned directly.
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

	// Sanitize group to a safe relative path.
	group = filepath.FromSlash(strings.Trim(strings.TrimSpace(group), "/"))
	targetPath := filepath.Join(c.dir, group, name)
	urlPath := strings.ReplaceAll(filepath.ToSlash(filepath.Join(group, name)), "\\", "/")

	// The PostgreSQL index deduplicates by content hash across groups. Without
	// it, the concrete destination path is the deduplication boundary.
	flightKey := targetPath
	if c.store != nil {
		flightKey = hashHex
	}
	callerToken := new(storeFlightToken)
	finishWait := commandtrace.MeasureOperation(ctx, "image.wait")
	resultCh := c.flight.DoChan(flightKey, func() (any, error) {
		sharedCtx, cancel := imageStoreSharedContext()
		defer cancel()
		sharedCtx, trace := commandtrace.WithNewTrace(sharedCtx)
		url, err := c.storeHashed(sharedCtx, ownedData, hashHex, group, urlPath, targetPath)
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

func (c *Client) storeHashed(ctx context.Context, data []byte, hashHex string, group string, urlPath string, targetPath string) (string, error) {
	finishLookup := commandtrace.MeasureOperation(ctx, "image.lookup")

	// Fast path: return cached URL from PostgreSQL — but only if the file still exists on disk.
	if c.store != nil {
		if cachedPath, storedPath, ok := c.store.Lookup(ctx, hashHex); ok {
			if _, err := os.Stat(storedPath); err == nil {
				finishLookup()
				return c.uri + "/" + cachedPath, nil
			}
			// File was deleted; fall through to re-write it below.
		}
	}

	_, statErr := os.Stat(targetPath)
	fileExists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		finishLookup()
		return "", fmt.Errorf("imagecache: stat %s: %w", targetPath, statErr)
	}
	finishLookup()

	if !fileExists {
		finishWrite := commandtrace.MeasureOperation(ctx, "image.write")
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			finishWrite()
			return "", fmt.Errorf("imagecache: mkdir %s: %w", filepath.Dir(targetPath), err)
		}
		write := c.write
		if write == nil {
			write = writeFileAtomically
		}
		if err := write(ctx, targetPath, data); err != nil {
			finishWrite()
			return "", fmt.Errorf("imagecache: write %s: %w", targetPath, err)
		}
		finishWrite()
	}

	cdnURL := c.uri + "/" + urlPath

	// Record the relative path (no domain) in PostgreSQL for future deduplication.
	// Storing only the path means changing the CDN base URI in config is sufficient
	// to update all returned URLs — no DB update required.
	if c.store != nil {
		finishIndex := commandtrace.MeasureOperation(ctx, "image.index")
		c.store.Insert(ctx, hashHex, group, urlPath, targetPath, int64(len(data)))
		finishIndex()
	}

	return cdnURL, nil
}

func imageStoreSharedContext() (context.Context, context.CancelFunc) {
	shared := logger.WithContextAttrs(context.Background(), slog.Bool("shared_work", true))
	return context.WithTimeout(shared, imageStoreSharedTimeout)
}

func writeFileAtomically(ctx context.Context, targetPath string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Dir(targetPath)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(targetPath)+".tmp-*")
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
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(tmpName, targetPath)
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
