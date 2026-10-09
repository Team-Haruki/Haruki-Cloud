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
		return i18n.T("render.data_source.mysekai")
	case drawing.DataSourcePublic:
		return i18n.T("render.data_source.public")
	default:
		return i18n.T("render.data_source.suite")
	}
}

// NewDataSource builds a data source entry with its Kind and label set.
func NewDataSource(kind drawing.DataSourceKind) drawing.ProfileDataSource {
	return drawing.ProfileDataSource{Name: DataSourceLabel(kind), Kind: kind}
}

// LiveShortLabel is the compact live-type label used where an image has
// little room ("单人", "多人", "自动", "烤森"). An unknown key is returned
// as is.
func LiveShortLabel(liveType string) string {
	switch strings.ToLower(strings.TrimSpace(liveType)) {
	case "solo":
		return i18n.T("render.live_short.solo")
	case "multi":
		return i18n.T("render.live_short.multi")
	case "auto":
		return i18n.T("render.live_short.auto")
	case "mysekai":
		return i18n.T("render.live_short.mysekai")
	default:
		return liveType
	}
}

// CharacterFallbackName is shown when no character name is known.
func CharacterFallbackName(characterID int) string {
	return i18n.T("render.fallback.character", i18n.Data{"ID": characterID})
}

// EventFallbackName is shown when no event name is known.
func EventFallbackName(eventID int) string {
	return i18n.T("render.fallback.event", i18n.Data{"ID": eventID})
}

// ItemFallbackName is shown when no item name is known.
func ItemFallbackName(itemID int) string {
	return i18n.T("render.fallback.item", i18n.Data{"ID": itemID})
}

// ProgressText is a named collection progress line, e.g.
// "总收集进度：12/40（30%）". label is a render_mysekai.progress.* message.
func ProgressText(label i18n.Message, done, total int) string {
	return i18n.T("render.progress", i18n.Data{"Label": label, "Ratio": progressRatio(done, total)})
}

// ProgressRatio is a bare progress value, e.g. "12/40（30%）".
func ProgressRatio(done, total int) string {
	return progressRatio(done, total).String()
}

func progressRatio(done, total int) i18n.Message {
	percent := 0.0
	if total > 0 {
		percent = float64(done) * 100 / float64(total)
	}
	return i18n.M("render.progress_ratio", i18n.Data{"Done": done, "Total": total, "Percent": i18n.Percent(percent)})
}
