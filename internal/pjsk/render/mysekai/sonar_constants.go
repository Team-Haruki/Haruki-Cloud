package mysekai

import (
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/render/common"
)

const (
	refreshIconFileName       = "icon_refresh.png"
	housingCacheSnapshotStage = "housing_cache.snapshot"
)

// mySekaiDataLabel names the MySekai data source on a profile card.
func mySekaiDataLabel() string {
	return common.DataSourceLabel(drawing.DataSourceMySekai)
}

// isMySekaiDataSource reports whether a profile card entry is the MySekai
// data source. Entries built in this process carry a Kind; an entry without
// one (decoded from a cached request) falls back to its label.
func isMySekaiDataSource(source drawing.ProfileDataSource) bool {
	if source.Kind != "" {
		return source.Kind == drawing.DataSourceMySekai
	}
	return strings.TrimSpace(source.Name) == mySekaiDataLabel()
}
