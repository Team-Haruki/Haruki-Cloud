package drawing

import (
	"strings"

	"haruki-cloud/internal/i18n"
)

// Drawing draws region_label where it used to upper-case the raw region code,
// and account_label as a profile's account line. It keeps keying colours and
// assets on the raw code, and falls back to the code when a label is absent.
const (
	drawingRegionLabelKey  = "region_label"
	drawingAccountLabelKey = "account_label"
	// A DetailedProfileCardRequest's data source: the raw kind and its name.
	drawingDataSourceKindKey  = "data_source_kind"
	drawingDataSourceLabelKey = "data_source_label"
)

// drawingProfileKeys are the request fields holding a player profile
// (BasicProfile, DetailedProfileCardRequest or ProfileCardRequest).
var drawingProfileKeys = []string{"profile", "user_info"}

// applyDrawingRequestLabels adds the localized region and account labels to a
// prepared request body: on the request itself, on its profiles and on an
// embedded deck request. Labels a builder already set are kept.
func applyDrawingRequestLabels(endpointPath string, root map[string]any, locale i18n.Locale) {
	if root == nil {
		return
	}
	applyDrawingEndpointLabels(endpointPath, root, locale)
	applyDrawingRegionLabel(root, locale)
	applyDrawingProfileLabels(root, locale, 2)
	if deck := mapAt(root, "deck_request"); deck != nil {
		applyDrawingRegionLabel(deck, locale)
		applyDrawingProfileLabels(deck, locale, 2)
	}
}

// applyDrawingProfileLabels labels the profiles under parent. depth bounds
// the nesting: a ProfileCardRequest holds its BasicProfile as "profile".
func applyDrawingProfileLabels(parent map[string]any, locale i18n.Locale, depth int) {
	if depth <= 0 {
		return
	}
	for _, key := range drawingProfileKeys {
		profile := mapAt(parent, key)
		if profile == nil {
			continue
		}
		applyDrawingRegionLabel(profile, locale)
		applyDrawingAccountLabel(profile, locale)
		applyDrawingDataSourceLabel(profile, locale)
		mergeDrawingLabels(profile, profileDrawingLabels, locale)
		applyDrawingProfileLabels(profile, locale, depth-1)
	}
}

func applyDrawingRegionLabel(body map[string]any, locale i18n.Locale) {
	region := strings.TrimSpace(scalarString(body["region"]))
	if region == "" || strings.TrimSpace(scalarString(body[drawingRegionLabelKey])) != "" {
		return
	}
	body[drawingRegionLabelKey] = i18n.RegionLabel(region).In(locale)
}

// applyDrawingDataSourceLabel names the data source of a detailed profile
// from its raw data_source_kind; Drawing draws the name and keys nothing on it.
func applyDrawingDataSourceLabel(profile map[string]any, locale i18n.Locale) {
	kind := DataSourceKind(strings.TrimSpace(scalarString(profile[drawingDataSourceKindKey])))
	if kind == "" || strings.TrimSpace(scalarString(profile[drawingDataSourceLabelKey])) != "" {
		return
	}
	profile[drawingDataSourceLabelKey] = dataSourceLabel(kind).In(locale)
}

// dataSourceLabel is the catalog name of a profile data source.
func dataSourceLabel(kind DataSourceKind) i18n.Message {
	switch kind {
	case DataSourceMySekai:
		return i18n.M("profile.data_source.mysekai")
	case DataSourcePublic:
		return i18n.M("profile.data_source.public")
	default:
		return i18n.M("profile.data_source.suite")
	}
}

// applyDrawingAccountLabel adds "[日服(JP)] 123***789" to a profile with a
// game UID, hiding the UID as the profile's is_hide_uid asks.
func applyDrawingAccountLabel(profile map[string]any, locale i18n.Locale) {
	region := strings.TrimSpace(scalarString(profile["region"]))
	uid := strings.TrimSpace(scalarString(profile["id"]))
	if region == "" || uid == "" || strings.TrimSpace(scalarString(profile[drawingAccountLabelKey])) != "" {
		return
	}
	hidden, _ := profile["is_hide_uid"].(bool)
	profile[drawingAccountLabelKey] = i18n.AccountLabel(region, uid, !hidden).In(locale)
}
