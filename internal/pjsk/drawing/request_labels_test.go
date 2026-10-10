package drawing

import (
	"context"
	"testing"
	"time"

	"haruki-cloud/internal/i18n"
)

func preparedLabelsRoot(t *testing.T, endpoint string, body any) map[string]any {
	t.Helper()
	root := mapAt(prepareDrawingRequestBody(endpoint, body, time.Unix(1710000000, 0), context.Background()))
	if root == nil {
		t.Fatalf("%s: prepared body is not an object", endpoint)
	}
	return root
}

func TestPreparedBodyCarriesRegionLabel(t *testing.T) {
	root := preparedLabelsRoot(t, "/api/pjsk/card/detail", &CardDetailRequest{Region: "jp"})
	if got, want := root["region_label"], i18n.RegionLabel("jp").String(); got != want {
		t.Fatalf("region_label = %v, want %q", got, want)
	}
	if root["region"] != "jp" {
		t.Fatalf("the raw region code must stay: %v", root["region"])
	}
}

func TestPreparedBodyLabelsProfiles(t *testing.T) {
	root := preparedLabelsRoot(t, "/api/pjsk/profile", &ProfileRequest{
		Profile: BasicProfile{ID: "7354311836516539153", Region: "tw", IsHideUID: true},
	})
	profile := mapAt(root, "profile")
	if got, want := profile["region_label"], i18n.RegionLabel("tw").String(); got != want {
		t.Fatalf("profile region_label = %v, want %q", got, want)
	}
	if got, want := profile["account_label"], i18n.AccountLabel("tw", "7354311836516539153", false).String(); got != want {
		t.Fatalf("hidden account_label = %v, want %q", got, want)
	}

	visible := mapAt(preparedLabelsRoot(t, "/api/pjsk/profile", &ProfileRequest{
		Profile: BasicProfile{ID: "7354311836516539153", Region: "tw"},
	}), "profile")
	if got, want := visible["account_label"], i18n.AccountLabel("tw", "7354311836516539153", true).String(); got != want {
		t.Fatalf("visible account_label = %v, want %q", got, want)
	}
}

func TestPreparedBodyLabelsNestedProfilesAndDeckRequests(t *testing.T) {
	// A ProfileCardRequest embedded as "profile" holds its BasicProfile as "profile".
	card := &ProfileCardRequest{Profile: &BasicProfile{ID: "123456789", Region: "cn"}}
	root := preparedLabelsRoot(t, InfoPanelEndpoint, card)
	inner := mapAt(root, "profile")
	if inner["account_label"] != i18n.AccountLabel("cn", "123456789", true).String() || inner["region_label"] != i18n.RegionLabel("cn").String() {
		t.Fatalf("info panel profile labels = %v / %v", inner["region_label"], inner["account_label"])
	}

	planner := &EventPlannerRequest{
		Region:      "kr",
		Profile:     &DetailedProfileCardRequest{ID: "42424242", Region: "kr"},
		DeckRequest: &DeckRequest{Region: "kr", Profile: DetailedProfileCardRequest{ID: "42424242", Region: "kr", IsHideUID: true}},
	}
	root = preparedLabelsRoot(t, "/api/pjsk/event/planner", planner)
	label := i18n.RegionLabel("kr").String()
	if root["region_label"] != label || mapAt(root, "profile")["region_label"] != label {
		t.Fatalf("planner region labels missing: %v", root)
	}
	deck := mapAt(root, "deck_request")
	if deck["region_label"] != label {
		t.Fatalf("deck_request region_label = %v", deck["region_label"])
	}
	if got, want := mapAt(deck, "profile")["account_label"], i18n.AccountLabel("kr", "42424242", false).String(); got != want {
		t.Fatalf("deck profile account_label = %v, want %q", got, want)
	}
}

func TestPreparedBodyKeepsBuilderLabelsAndSkipsBodiesWithoutRegion(t *testing.T) {
	root := map[string]any{"region": "jp", "region_label": "preset", "profile": map[string]any{"id": "1", "region": "jp", "account_label": "preset"}}
	applyDrawingRequestLabels(root, i18n.DefaultLocale)
	if root["region_label"] != "preset" || mapAt(root, "profile")["account_label"] != "preset" {
		t.Fatalf("builder labels overwritten: %v", root)
	}

	root = map[string]any{"title": "x", "profile": map[string]any{"region": "jp"}}
	applyDrawingRequestLabels(root, i18n.DefaultLocale)
	if _, ok := root["region_label"]; ok {
		t.Fatal("a body without a region must not get a region label")
	}
	if _, ok := mapAt(root, "profile")["account_label"]; ok {
		t.Fatal("a profile without a UID must not get an account label")
	}
	applyDrawingRequestLabels(nil, i18n.DefaultLocale)
}
