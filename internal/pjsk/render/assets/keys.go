package assets

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"haruki-cloud/internal/storage"
)

const regionAssetsSuffix = "-assets"

// ErrNoAssetBaseURL is returned by PublicAssetURL when no public base is known.
var ErrNoAssetBaseURL = errors.New("assets: public asset base URL is not configured")

// ObjectKey is the ONE Cloud-side strip of the "asset/" prefix (C3). It accepts
// every path form Cloud produces and returns the bucket key:
//
//	"asset/jp-assets/startapp/x", "/asset/jp-assets/startapp/x", "jp-assets/startapp/x",
//	"/local/root/jp-assets/startapp/x"  ->  "jp-assets/startapp/x"
//	"static_images/pjsk_3d_preview/1.png" -> unchanged (cleaned, no leading "/")
//
// When a "<region>-assets" segment is present everything before it is cut;
// otherwise a leading "/" and "asset/" are trimmed. ".." anywhere in the input,
// empty input and control characters are rejected (storage.ErrInvalidKey).
func ObjectKey(drawingPath string) (storage.Key, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(drawingPath), "\\", "/")
	segments := strings.Split(normalized, "/")
	for _, segment := range segments {
		if segment == ".." {
			return "", fmt.Errorf("%w %q: parent segment", storage.ErrInvalidKey, drawingPath)
		}
	}
	for idx, segment := range segments {
		if len(segment) > len(regionAssetsSuffix) && strings.HasSuffix(segment, regionAssetsSuffix) {
			return storage.CleanKey(strings.Join(segments[idx:], "/"))
		}
	}
	rel := strings.TrimLeft(normalized, "/")
	rel = strings.TrimPrefix(rel, assetPathPrefix)
	return storage.CleanKey(rel)
}

// PublicAssetURL is the single Drawing-path -> public URL rule. It is ObjectKey
// plus per-segment url.PathEscape appended to base. An http(s) input is
// returned unchanged; an empty base is an error.
func PublicAssetURL(base string, drawingPath string) (string, error) {
	trimmed := strings.TrimSpace(drawingPath)
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed, nil
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return "", ErrNoAssetBaseURL
	}
	key, err := ObjectKey(drawingPath)
	if err != nil {
		return "", err
	}
	return base + "/" + EscapeKey(key), nil
}

// EscapeKey escapes every segment of key for use in a URL path.
func EscapeKey(key storage.Key) string {
	segments := strings.Split(string(key), "/")
	for idx, segment := range segments {
		segments[idx] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
