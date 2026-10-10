package drawing

import (
	"context"
	"strings"
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
	applyDrawingRequestLabels("", root, i18n.DefaultLocale)
	if root["region_label"] != "preset" || mapAt(root, "profile")["account_label"] != "preset" {
		t.Fatalf("builder labels overwritten: %v", root)
	}

	root = map[string]any{"title": "x", "profile": map[string]any{"region": "jp"}}
	applyDrawingRequestLabels("", root, i18n.DefaultLocale)
	if _, ok := root["region_label"]; ok {
		t.Fatal("a body without a region must not get a region label")
	}
	if _, ok := mapAt(root, "profile")["account_label"]; ok {
		t.Fatal("a profile without a UID must not get an account label")
	}
	applyDrawingRequestLabels("", nil, i18n.DefaultLocale)
}

func preparedDrawingLabels(t *testing.T, root map[string]any) map[string]any {
	t.Helper()
	labels := mapAt(root, drawingLabelsKey)
	if labels == nil {
		t.Fatalf("no %s on %v", drawingLabelsKey, root)
	}
	return labels
}

func TestPreparedBodyCarriesEndpointLabels(t *testing.T) {
	sk := preparedDrawingLabels(t, preparedLabelsRoot(t, "/api/pjsk/sk/line?full=false", map[string]any{"region": "jp"}))
	if got, want := sk["sk.time_to_end"], i18n.T("sk.image.time_to_end", i18n.Data{"Duration": "{duration}"}); got != want || !strings.Contains(want, "{duration}") {
		t.Fatalf("sk.time_to_end = %v, want %q with a {duration} slot", got, want)
	}
	if sk["sk.prediction_notice"] != i18n.T("sk.forecast.notice") || sk["sk.event_ended"] != i18n.T("sk.image.event_ended") {
		t.Fatalf("sk line labels = %v", sk)
	}

	gacha := preparedDrawingLabels(t, preparedLabelsRoot(t, "/api/pjsk/gacha/detail", map[string]any{"region": "jp"}))
	if gacha["gacha.spin.ten"] != "/"+i18n.T("gacha.image.spin.ten") {
		t.Fatalf("spin label must keep Drawing's / separator: %v", gacha["gacha.spin.ten"])
	}
	if got := gacha["gacha.execute_limit"]; !strings.Contains(scalarString(got), "{count}") {
		t.Fatalf("execute_limit slot = %v", got)
	}

	planner := preparedDrawingLabels(t, preparedLabelsRoot(t, "/api/pjsk/event/planner", map[string]any{"region": "jp"}))
	for _, key := range []string{"deck.title.event", "deck.noun.planner", "deck.planner.target", "deck.algorithm.dfs_ga"} {
		if scalarString(planner[key]) == "" {
			t.Fatalf("planner label %s missing: %v", key, planner)
		}
	}
	if planner["deck.planner.title"] != i18n.T("event.planner.title") {
		t.Fatalf("planner title = %v", planner["deck.planner.title"])
	}

	// An endpoint without labels gets none.
	if labels := mapAt(preparedLabelsRoot(t, "/api/pjsk/card/detail", map[string]any{"region": "jp"}), drawingLabelsKey); labels != nil {
		t.Fatalf("card/detail got labels: %v", labels)
	}
}

func TestPreparedListPayloadLabelsEveryItem(t *testing.T) {
	prepared := prepareDrawingRequestBody("/api/pjsk/vlive/list", []any{map[string]any{"region": "jp"}, map[string]any{"region": "cn"}}, time.Unix(1710000000, 0), context.Background())
	items := sliceAt(prepared)
	if len(items) != 2 {
		t.Fatalf("list payload = %v", prepared)
	}
	for _, item := range items {
		if got := mapAt(item, drawingLabelsKey)["vlive.type.cheerful_carnival"]; got != i18n.T("vlive.image.type.cheerful_carnival") {
			t.Fatalf("vlive label = %v", got)
		}
	}
}

func TestPreparedBodyLabelsProfilesAndDataSources(t *testing.T) {
	root := preparedLabelsRoot(t, "/api/pjsk/music/list", map[string]any{
		"profile": map[string]any{"id": "1", "region": "jp", "data_source_kind": "public"},
	})
	profile := mapAt(root, "profile")
	if profile["data_source_label"] != i18n.T("profile.data_source.public") {
		t.Fatalf("data_source_label = %v", profile["data_source_label"])
	}
	if got := mapAt(profile, drawingLabelsKey)["profile.mysekai_level"]; got != i18n.T("profile.image.mysekai_level", i18n.Data{"Level": "{level}"}) {
		t.Fatalf("profile labels = %v", got)
	}

	info := preparedDrawingLabels(t, preparedLabelsRoot(t, InfoPanelEndpoint, map[string]any{"profile": map[string]any{"region": "jp"}}))
	if scalarString(info["profile.rank_level"]) == "" {
		t.Fatalf("info panel root labels = %v", info)
	}

	// No kind, no label: Drawing keeps its own fallback name.
	plain := mapAt(preparedLabelsRoot(t, "/api/pjsk/music/list", map[string]any{"profile": map[string]any{"id": "1", "region": "jp"}}), "profile")
	if _, ok := plain["data_source_label"]; ok {
		t.Fatalf("a profile without a kind got a data source label: %v", plain)
	}
}

func TestPreparedBodyKeepsBuilderDrawingLabels(t *testing.T) {
	root := map[string]any{
		"labels":  map[string]any{"score.target_pt": "preset"},
		"profile": map[string]any{"data_source_kind": "suite", "data_source_label": "preset"},
	}
	applyDrawingRequestLabels("/api/pjsk/score/control", root, i18n.DefaultLocale)
	if mapAt(root, drawingLabelsKey)["score.target_pt"] != "preset" || mapAt(root, "profile")["data_source_label"] != "preset" {
		t.Fatalf("builder labels overwritten: %v", root)
	}
}
