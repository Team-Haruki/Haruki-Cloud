package imagecache

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// URLForFile returns a URL only for regular files inside this client's served
// directory. It never reads the image or consults the content-hash index.
func (c *Client) URLForFile(ctx context.Context, candidate string) (string, bool) {
	if c == nil || strings.TrimSpace(candidate) == "" || ctx.Err() != nil {
		return "", false
	}
	root, err := filepath.Abs(c.dir)
	if err != nil {
		return "", false
	}
	target, err := filepath.Abs(candidate)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || !containedRelativeFile(rel) {
		return "", false
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", false
	}
	realRel, err := filepath.Rel(realRoot, realTarget)
	if err != nil || !containedRelativeFile(realRel) {
		return "", false
	}
	info, err := os.Stat(realTarget)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return c.uri + "/" + strings.Join(parts, "/"), true
}

func containedRelativeFile(rel string) bool {
	return rel != "." && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
