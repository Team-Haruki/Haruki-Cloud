package common

import (
	"strings"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/drawing"
)

// DataSourceLabel is the label Drawing shows for a profile data source.
func DataSourceLabel(kind drawing.DataSourceKind) string {
	switch kind {
	case drawing.DataSourceMySekai:
		return i18n.T("profile.data_source.mysekai")
	case drawing.DataSourcePublic:
		return i18n.T("profile.data_source.public")
	default:
		return i18n.T("profile.data_source.suite")
	}
}

// NewDataSource builds a data source entry with its Kind and label set.
func NewDataSource(kind drawing.DataSourceKind) drawing.ProfileDataSource {
	return drawing.ProfileDataSource{Name: DataSourceLabel(kind), Kind: kind}
}

// LiveShortLabel is the compact live-type label used where an image has
// little room ("单人", "多人", "自动"), such as the leaderboard rows of the
// song detail image. An unknown key is returned as is.
func LiveShortLabel(liveType string) string {
	switch strings.ToLower(strings.TrimSpace(liveType)) {
	case "solo":
		return i18n.T("render.live_short.solo")
	case "multi":
		return i18n.T("render.live_short.multi")
	case "auto":
		return i18n.T("render.live_short.auto")
	default:
		return liveType
	}
}

// CharacterFallbackName is shown when no character name is known.
func CharacterFallbackName(characterID int) string {
	return i18n.T("common.fallback.character", i18n.Data{"ID": characterID})
}
