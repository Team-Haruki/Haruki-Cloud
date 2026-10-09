package mysekai

import "haruki-cloud/internal/i18n"

const (
	refreshIconFileName       = "icon_refresh.png"
	housingCacheSnapshotStage = "housing_cache.snapshot"
)

// mySekaiDataLabel names the MySekai data source on a profile card; the
// snapshot merge also uses it to find that source again.
func mySekaiDataLabel() string {
	return i18n.T("profile.data_source.mysekai")
}
