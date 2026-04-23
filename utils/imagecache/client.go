// Package imagecache provides a content-addressable image store that writes
// rendered images to a local directory and returns a CDN-based URL suitable
// for use in OneBot11 image message segments.
//
// An optional PGStore can be attached for PostgreSQL-backed deduplication:
// repeated writes for the same image content are short-circuited by returning
// the previously computed CDN URL directly from the database.
package imagecache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Client writes images to a local directory and returns CDN URLs.
// A nil Client is safe to use — all methods become no-ops or return errors.
type Client struct {
	uri   string   // CDN base URI, e.g. "https://image-cache.example.com"
	dir   string   // Local root directory for stored images
	store *PGStore // optional PostgreSQL deduplication store
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
	return &Client{uri: uri, dir: dir, store: store}
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

	digest := sha256.Sum256(data)
	hashHex := hex.EncodeToString(digest[:])
	name := hashHex + extFromData(data)

	// Fast path: return cached URL from PostgreSQL — but only if the file still exists on disk.
	if c.store != nil {
		if cachedPath, storedPath, ok := c.store.Lookup(ctx, hashHex); ok {
			if _, err := os.Stat(storedPath); err == nil {
				return c.uri + "/" + cachedPath, nil
			}
			// File was deleted; fall through to re-write it below.
		}
	}

	// Sanitize group to a safe relative path.
	group = filepath.FromSlash(strings.Trim(strings.TrimSpace(group), "/"))
	targetPath := filepath.Join(c.dir, group, name)

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", fmt.Errorf("imagecache: mkdir %s: %w", filepath.Dir(targetPath), err)
	}
	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		return "", fmt.Errorf("imagecache: write %s: %w", targetPath, err)
	}

	urlPath := strings.ReplaceAll(filepath.ToSlash(filepath.Join(group, name)), "\\", "/")
	cdnURL := c.uri + "/" + urlPath

	// Record the relative path (no domain) in PostgreSQL for future deduplication.
	// Storing only the path means changing the CDN base URI in config is sufficient
	// to update all returned URLs — no DB update required.
	if c.store != nil {
		c.store.Insert(ctx, hashHex, group, urlPath, targetPath, int64(len(data)))
	}

	return cdnURL, nil
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
