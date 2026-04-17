package chartstyle

import (
	"path/filepath"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
)

const (
	Black = "black"
	White = "white"

	Default = Black
)

func Normalize(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case Black:
		return Black
	case White:
		return White
	default:
		return ""
	}
}

func CSSRelativePath(style string) string {
	normalized := Normalize(style)
	if normalized == "" {
		normalized = Default
	}
	return filepath.ToSlash(filepath.Join("chart_asset", "css", normalized+".css"))
}

func CSSPath(style string) string {
	return filepath.ToSlash(filepath.Join(assets.StaticImagesDir, CSSRelativePath(style)))
}
