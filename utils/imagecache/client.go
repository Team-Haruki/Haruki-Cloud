// Package imagecache provides a content-addressable image store that writes
// rendered images to a local directory and returns a CDN-based URL suitable
// for use in OneBot11 image message segments.
package imagecache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Client writes images to a local directory and returns CDN URLs.
// A nil Client is safe to use — all methods become no-ops or return errors.
type Client struct {
	uri string // CDN base URI, e.g. "https://image-cache.example.com"
	dir string // Local root directory for stored images
}

// New returns a new Client. Returns nil if uri or dir is empty (image cache
// disabled), so callers can safely test for nil before using.
func New(uri, dir string) *Client {
	uri = strings.TrimRight(strings.TrimSpace(uri), "/")
	dir = strings.TrimSpace(dir)
	if uri == "" || dir == "" {
		return nil
	}
	return &Client{uri: uri, dir: dir}
}

// StoreAndGetURL writes data to disk under {dir}/{group}/{sha256}.png and
// returns the corresponding CDN URL {uri}/{group}/{sha256}.png.
// group is a slash-separated path component, e.g. "pjsk/profile".
func (c *Client) StoreAndGetURL(data []byte, group string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("imagecache: client is not configured")
	}

	digest := sha256.Sum256(data)
	name := hex.EncodeToString(digest[:]) + ".png"

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
	return c.uri + "/" + urlPath, nil
}
