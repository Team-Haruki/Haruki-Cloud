package mysekai

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/usererror"

	json "haruki-cloud/internal/jsonutil"
)

func decodeFirstCraftSnapshot(t *testing.T, raw string) map[string]any {
	t.Helper()
	var merged map[string]any
	if err := json.Unmarshal([]byte(raw), &merged); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	return merged
}

func TestFirstCraftStatusFollowsTheClientRule(t *testing.T) {
	history := newFirstCraftHistory(decodeFirstCraftSnapshot(t, `{
  "userMysekaiFixtures": [
    {"mysekaiFixtureId": 10, "textureId": 1, "quantity": 0},
    {"mysekaiFixtureId": 11, "textureId": 3, "quantity": 2},
    {"mysekaiFixtureId": 50, "textureId": 1, "quantity": 1}
  ],
  "userMysekaiCanvases": [
    {"mysekaiFixtureId": 40, "cardId": 999, "isSpecialTraining": true, "quantity": 1}
  ],
  "userMysekaiBlueprints": [{"mysekaiBlueprintId": 101}, {"mysekaiBlueprintId": 112}, {"mysekaiBlueprintId": 141}]
}`))

	cases := []struct {
		name      string
		blueprint map[string]any
		want      firstCraftStatus
	}{
		{
			name:      "dismantled to quantity 0 still counts as held",
			blueprint: map[string]any{"id": 101, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 10},
			want:      firstCraftStatus{Known: true, Available: false, Craftable: true},
		},
		{
			name:      "another colour counts as held",
			blueprint: map[string]any{"id": 111, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 11},
			want:      firstCraftStatus{Known: true, Available: false, Craftable: false},
		},
		{
			name:      "never held, blueprint owned",
			blueprint: map[string]any{"id": 112, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 12},
			want:      firstCraftStatus{Known: true, Available: true, Craftable: true},
		},
		{
			name:      "never held, blueprint not owned",
			blueprint: map[string]any{"id": 113, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 13},
			want:      firstCraftStatus{Known: true, Available: true, Craftable: false},
		},
		{
			name:      "never held, usable without the blueprint",
			blueprint: map[string]any{"id": 114, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 14, "isAvailableWithoutPossession": true},
			want:      firstCraftStatus{Known: true, Available: true, Craftable: true},
		},
		{
			name:      "canvas held with any card and training state",
			blueprint: map[string]any{"id": 140, "mysekaiCraftType": "mysekai_canvas", "craftTargetId": 40},
			want:      firstCraftStatus{Known: true, Available: false, Craftable: false},
		},
		{
			name:      "canvas never held",
			blueprint: map[string]any{"id": 141, "mysekaiCraftType": "mysekai_canvas", "craftTargetId": 41},
			want:      firstCraftStatus{Known: true, Available: true, Craftable: true},
		},
		{
			name:      "canvases are checked against userMysekaiCanvases only",
			blueprint: map[string]any{"id": 150, "mysekaiCraftType": "mysekai_canvas", "craftTargetId": 50},
			want:      firstCraftStatus{Known: true, Available: true, Craftable: false},
		},
		{
			name:      "tools never give the bonus",
			blueprint: map[string]any{"id": 160, "mysekaiCraftType": "mysekai_tool", "craftTargetId": 1},
			want:      firstCraftStatus{},
		},
	}
	for _, tc := range cases {
		if got := history.status(tc.blueprint); got != tc.want {
			t.Errorf("%s: status = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestFirstCraftStatusNeedsTheMatchingList(t *testing.T) {
	fixture := map[string]any{"id": 1, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 10}
	canvas := map[string]any{"id": 2, "mysekaiCraftType": "mysekai_canvas", "craftTargetId": 40}

	onlyCanvases := newFirstCraftHistory(decodeFirstCraftSnapshot(t, `{"userMysekaiCanvases": []}`))
	if got := onlyCanvases.status(fixture); got.Known {
		t.Fatalf("a snapshot without userMysekaiFixtures cannot tell fixtures, got %+v", got)
	}
	if got := onlyCanvases.status(canvas); !got.Known || !got.Available {
		t.Fatalf("an empty canvas list means never held, got %+v", got)
	}

	nested := newFirstCraftHistory(decodeFirstCraftSnapshot(t, `{"updatedResources": {"userMysekaiFixtures": []}}`))
	if got := nested.status(fixture); !got.Known || !got.Available {
		t.Fatalf("an empty fixture list under updatedResources means never held, got %+v", got)
	}
	if got := nested.status(canvas); got.Known {
		t.Fatalf("a snapshot without userMysekaiCanvases cannot tell canvases, got %+v", got)
	}

	controller := &Controller{}
	if controller.newFirstCraftTally(decodeFirstCraftSnapshot(t, `{"userMysekaiBlueprints": []}`), nil, time.Now()) != nil {
		t.Fatal("a snapshot without either list has no first-craft tally")
	}
}

func TestFirstCraftStatusOfLimitedBlueprints(t *testing.T) {
	history := newFirstCraftHistory(decodeFirstCraftSnapshot(t, `{"userMysekaiFixtures": [], "userMysekaiBlueprints": [{"mysekaiBlueprintId": 3}]}`))
	history.terms = newBlueprintTermWindows([]map[string]any{
		{"mysekaiBlueprintId": 1, "startAt": 1000, "endAt": 2000},
		{"mysekaiBlueprintId": 1, "startAt": 5000, "endAt": 6000},
		{"mysekaiBlueprintId": 3, "startAt": 1000, "endAt": 2000},
	}, 5500)
	party := map[string]any{"id": 1, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 10, "isAvailableWithoutPossession": true}
	if got := history.status(party); !got.Craftable || got.Limited {
		t.Fatalf("an open window makes it craftable: %+v", got)
	}
	history.terms.now = 3000
	if got := history.status(party); got.Craftable || !got.Limited || !got.Available {
		t.Fatalf("between windows it is limited: %+v", got)
	}
	noTerms := map[string]any{"id": 2, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 11, "isAvailableWithoutPossession": true}
	if got := history.status(noTerms); !got.Craftable {
		t.Fatalf("without any term the flag alone decides: %+v", got)
	}
	owned := map[string]any{"id": 3, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 12, "isAvailableWithoutPossession": true}
	if got := history.status(owned); !got.Craftable || got.Limited {
		t.Fatalf("an owned blueprint is craftable whatever its terms: %+v", got)
	}
}

// newFirstCraftTestController writes a small master: fixtures in two
// categories plus a canvas and a tool, and the given exp rows.
func newFirstCraftTestController(t *testing.T, expRows []map[string]any, mysekaiJSON string) *Controller {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "masterdata")
	fixture := func(id, mainGenre, subGenre int) map[string]any {
		return map[string]any{
			"id": id, "name": "Fixture", "assetbundleName": "fixture", "mysekaiFixtureType": "normal",
			"mysekaiFixtureMainGenreId": mainGenre, "mysekaiFixtureSubGenreId": subGenre,
		}
	}
	writeTestJSON(t, filepath.Join(dir, "mysekaiFixtures.json"), []map[string]any{
		fixture(10, 2, 4), fixture(11, 2, 4), fixture(12, 2, 4), fixture(13, 2, 3), fixture(14, 2, 3),
		fixture(15, 2, 3), fixture(40, 6, 18), fixture(41, 6, 18),
	})
	writeTestJSON(t, filepath.Join(dir, "mysekaiFixtureMainGenres.json"), []map[string]any{
		{"id": 2, "name": "一般", "assetbundleName": "icon_normal_furniture"},
		{"id": 6, "name": "キャンバス", "assetbundleName": "icon_canvas"},
	})
	writeTestJSON(t, filepath.Join(dir, "mysekaiFixtureSubGenres.json"), []map[string]any{
		{"id": 3, "name": "テーブル", "assetbundleName": "icon_table"},
		{"id": 4, "name": "チェア", "assetbundleName": "icon_chair"},
		{"id": 18, "name": "キャンバス", "assetbundleName": "icon_canvas"},
	})
	writeTestJSON(t, filepath.Join(dir, "mysekaiBlueprints.json"), []map[string]any{
		{"id": 110, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 10},
		{"id": 111, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 11},
		{"id": 112, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 12},
		{"id": 113, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 13},
		{"id": 114, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 14, "isAvailableWithoutPossession": true},
		{"id": 140, "mysekaiCraftType": "mysekai_canvas", "craftTargetId": 40},
		{"id": 141, "mysekaiCraftType": "mysekai_canvas", "craftTargetId": 41},
		{"id": 199, "mysekaiCraftType": "mysekai_tool", "craftTargetId": 1},
	})
	writeTestJSON(t, filepath.Join(dir, "gameCharacters.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(dir, "mysekaiBlueprintMysekaiMaterialCosts.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(dir, "mysekaiFixtureOnlyDisassembleMaterials.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(dir, "mysekaiFixtureTags.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(dir, "mysekaiBlueprintTerms.json"), []map[string]any{
		{"id": 1, "mysekaiBlueprintId": 114, "startAt": firstCraftTermStart, "endAt": firstCraftTermEnd, "mysekaiBlueprintTermTabType": "birthday_anniversary"},
	})
	if expRows != nil {
		writeTestJSON(t, filepath.Join(dir, "mysekaiRankObtainedExps.json"), expRows)
	}
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: dir, AllowFallback: true})
	if mysekaiJSON == "" {
		return controller
	}
	return controller.WithMySekaiData([]byte(mysekaiJSON))
}

// Blueprint 114 is a party blueprint usable without owning it inside this
// window only.
var (
	firstCraftTermStart = time.Now().Add(-time.Hour).UnixMilli()
	firstCraftTermEnd   = time.Now().Add(time.Hour).UnixMilli()
	firstCraftAfterTerm = time.Now().Add(48 * time.Hour)
)

// firstCraftTestSnapshot: 10 held at quantity 0, 11 held in another colour,
// 12 never held with its blueprint, 13 never held without it, 14 never held
// but usable without a blueprint, 15 has no blueprint, canvas 40 held,
// canvas 41 never held with its blueprint.
const firstCraftTestSnapshot = `{
  "updatedResources": {
    "userMysekaiFixtures": [
      {"mysekaiFixtureId": 10, "textureId": 1, "quantity": 0},
      {"mysekaiFixtureId": 11, "textureId": 2, "quantity": 1}
    ],
    "userMysekaiCanvases": [{"mysekaiFixtureId": 40, "cardId": 1, "isSpecialTraining": false, "quantity": 1}],
    "userMysekaiBlueprints": [
      {"mysekaiBlueprintId": 110}, {"mysekaiBlueprintId": 111}, {"mysekaiBlueprintId": 112},
      {"mysekaiBlueprintId": 141}, {"mysekaiBlueprintId": 199}
    ],
    "userMysekaiGamedata": {"mysekaiRank": 20}
  }
}`

var firstCraftExpRows = []map[string]any{
	{"id": 1, "mysekaiRankObtainedExpType": "craft_mysekai_fixture_first_bonus", "quantity": 1234},
}

func TestFirstCraftBonusExpComesFromMaster(t *testing.T) {
	if got := newFirstCraftTestController(t, firstCraftExpRows, "").firstCraftBonusExp(); got != 1234 {
		t.Fatalf("bonus exp = %d, want the master quantity 1234", got)
	}
	other := []map[string]any{{"id": 1, "mysekaiRankObtainedExpType": "something_else", "quantity": 99}}
	if got := newFirstCraftTestController(t, other, "").firstCraftBonusExp(); got != 0 {
		t.Fatalf("bonus exp = %d, want 0 without a first-craft row", got)
	}
	if got := newFirstCraftTestController(t, nil, "").firstCraftBonusExp(); got != 0 {
		t.Fatalf("bonus exp = %d, want 0 without the table", got)
	}
}

func fixtureTiles(req *drawing.MysekaiFixtureListRequest) map[int]bool {
	if req == nil {
		return nil
	}
	return fixtureObtainedByID(req)
}

func TestBuildFirstCraftListListsCraftableBonuses(t *testing.T) {
	controller := newFirstCraftTestController(t, firstCraftExpRows, firstCraftTestSnapshot)

	list, err := controller.BuildFirstCraftList(FirstCraftListQuery{
		Region:  "jp",
		Profile: &drawing.ProfileCardRequest{Profile: &drawing.BasicProfile{Nickname: "Tester"}},
	})
	if err != nil {
		t.Fatalf("BuildFirstCraftList: %v", err)
	}
	want := FirstCraftSummary{BonusExp: 1234, Craftable: 3, NotOwned: 1}
	if list.Summary != want {
		t.Fatalf("summary = %+v, want %+v", list.Summary, want)
	}
	if got := fixtureTiles(list.Request); !reflect.DeepEqual(got, map[int]bool{12: true, 14: true, 41: true}) {
		t.Fatalf("tiles = %v, want the craftable bonuses 12, 14 and canvas 41", got)
	}
	if list.Request.ProgressMessage == nil || *list.Request.ProgressMessage != i18n.T("mysekai.image.first_craft.header", i18n.Data{"Craftable": 3, "Exp": i18n.Wan(3 * 1234)}) {
		t.Fatalf("header = %v", list.Request.ProgressMessage)
	}
	if list.Request.Profile == nil || list.Request.Profile.MysekaiLevel == nil || *list.Request.Profile.MysekaiLevel != 20 {
		t.Fatalf("expected the MySekai profile card, got %+v", list.Request.Profile)
	}

	all, err := controller.BuildFirstCraftList(FirstCraftListQuery{Region: "jp", ShowAll: true})
	if err != nil {
		t.Fatalf("BuildFirstCraftList(all): %v", err)
	}
	if got := fixtureTiles(all.Request); !reflect.DeepEqual(got, map[int]bool{12: true, 13: false, 14: true, 41: true}) {
		t.Fatalf("tiles = %v, want 13 added greyed out", got)
	}
	if !all.Summary.ShowAll || all.Summary.NotOwned != 1 {
		t.Fatalf("summary = %+v", all.Summary)
	}
}

func TestBuildFirstCraftListAppliesTheCategoryFilter(t *testing.T) {
	controller := newFirstCraftTestController(t, firstCraftExpRows, firstCraftTestSnapshot)

	tables, err := controller.BuildFirstCraftList(FirstCraftListQuery{Region: "jp", CategoryQuery: "桌子", ShowAll: true})
	if err != nil {
		t.Fatalf("BuildFirstCraftList(桌子): %v", err)
	}
	if got := fixtureTiles(tables.Request); !reflect.DeepEqual(got, map[int]bool{13: false, 14: true}) {
		t.Fatalf("tiles = %v, want the tables only", got)
	}
	if tables.Summary.Craftable != 1 || tables.Summary.NotOwned != 1 {
		t.Fatalf("summary = %+v, want the counts of the category", tables.Summary)
	}

	canvases, err := controller.BuildFirstCraftList(FirstCraftListQuery{Region: "jp", CategoryQuery: "画布"})
	if err != nil {
		t.Fatalf("BuildFirstCraftList(画布): %v", err)
	}
	if got := fixtureTiles(canvases.Request); !reflect.DeepEqual(got, map[int]bool{41: true}) {
		t.Fatalf("tiles = %v, want canvas 41", got)
	}

	closed, err := controller.BuildFirstCraftList(FirstCraftListQuery{Region: "jp", CategoryQuery: "桌子", ShowAll: true, Now: firstCraftAfterTerm})
	if err != nil {
		t.Fatalf("BuildFirstCraftList(after the term): %v", err)
	}
	if got := fixtureTiles(closed.Request); !reflect.DeepEqual(got, map[int]bool{13: false, 14: false}) {
		t.Fatalf("tiles = %v, want the closed party fixture greyed out", got)
	}
	if closed.Summary.Craftable != 0 || closed.Summary.NotOwned != 1 || closed.Summary.Limited != 1 {
		t.Fatalf("summary = %+v, want 14 counted as limited", closed.Summary)
	}

	_, err = controller.BuildFirstCraftList(FirstCraftListQuery{Region: "jp", CategoryQuery: "不存在"})
	testutil.RequireUserError(t, err, usererror.CodeNotFound, "mysekai.fixture.category_not_found")
}

func TestBuildFirstCraftListWithNothingToDraw(t *testing.T) {
	allHeld := `{"userMysekaiFixtures": [
  {"mysekaiFixtureId": 10}, {"mysekaiFixtureId": 11}, {"mysekaiFixtureId": 12},
  {"mysekaiFixtureId": 14}
], "userMysekaiCanvases": [{"mysekaiFixtureId": 40}, {"mysekaiFixtureId": 41}], "userMysekaiBlueprints": []}`
	controller := newFirstCraftTestController(t, firstCraftExpRows, allHeld)
	list, err := controller.BuildFirstCraftList(FirstCraftListQuery{Region: "jp"})
	if err != nil {
		t.Fatalf("BuildFirstCraftList: %v", err)
	}
	if list.Request != nil || list.Summary.Craftable != 0 || list.Summary.NotOwned != 1 {
		t.Fatalf("only 13 is left and needs a blueprint: %+v request=%v", list.Summary, list.Request != nil)
	}
	if ids := summaryLineIDs(FirstCraftSummaryLines(list.Summary)); !slices.Equal(ids, []string{
		"mysekai.first_craft.bonus", "mysekai.first_craft.craftable_no_exp", "mysekai.first_craft.not_owned", "mysekai.first_craft.hint_all_only",
	}) {
		t.Fatalf("summary lines = %v", ids)
	}

	all, err := controller.BuildFirstCraftList(FirstCraftListQuery{Region: "jp", ShowAll: true})
	if err != nil || all.Request == nil {
		t.Fatalf("with 全部 the missing blueprint is drawn: %v %+v", err, all)
	}

	missing := newFirstCraftTestController(t, firstCraftExpRows, `{"userMysekaiBlueprints": []}`)
	_, err = missing.BuildFirstCraftList(FirstCraftListQuery{Region: "jp"})
	testutil.RequireUserError(t, err, usererror.CodeSetup, "mysekai.first_craft.no_history")
}

func summaryLineIDs(lines []i18n.Message) []string {
	ids := make([]string, 0, len(lines))
	for _, line := range lines {
		ids = append(ids, line.ID)
	}
	return ids
}

func TestFirstCraftSummaryLines(t *testing.T) {
	cases := []struct {
		summary FirstCraftSummary
		want    []string
	}{
		{FirstCraftSummary{BonusExp: 1000}, []string{"mysekai.first_craft.none_left"}},
		{FirstCraftSummary{BonusExp: 1000, Craftable: 3}, []string{"mysekai.first_craft.bonus", "mysekai.first_craft.craftable"}},
		{FirstCraftSummary{BonusExp: 1000, Craftable: 3, NotOwned: 2}, []string{"mysekai.first_craft.bonus", "mysekai.first_craft.craftable", "mysekai.first_craft.not_owned", "mysekai.first_craft.hint_all"}},
		{FirstCraftSummary{BonusExp: 1000, Craftable: 3, NotOwned: 2, ShowAll: true}, []string{"mysekai.first_craft.bonus", "mysekai.first_craft.craftable", "mysekai.first_craft.not_owned", "mysekai.first_craft.hint_grey"}},
		{FirstCraftSummary{Craftable: 3, NotOwned: 2}, []string{"mysekai.first_craft.craftable_no_exp", "mysekai.first_craft.not_owned_no_exp", "mysekai.first_craft.hint_all"}},
		{FirstCraftSummary{BonusExp: 1000, Limited: 4}, []string{"mysekai.first_craft.bonus", "mysekai.first_craft.craftable_no_exp", "mysekai.first_craft.limited", "mysekai.first_craft.hint_all_only"}},
		{FirstCraftSummary{Limited: 4, ShowAll: true}, []string{"mysekai.first_craft.craftable_no_exp", "mysekai.first_craft.limited_no_exp", "mysekai.first_craft.hint_grey"}},
	}
	for _, tc := range cases {
		if got := summaryLineIDs(FirstCraftSummaryLines(tc.summary)); !slices.Equal(got, tc.want) {
			t.Errorf("FirstCraftSummaryLines(%+v) = %v, want %v", tc.summary, got, tc.want)
		}
	}
	text := i18n.LinesText(FirstCraftSummaryLines(FirstCraftSummary{BonusExp: 1000, Craftable: 23, NotOwned: 45}))
	for _, part := range []string{"1000 经验", "23 件，共 2.3w 经验", "45 件，共 4.5w 经验"} {
		if !strings.Contains(text, part) {
			t.Errorf("summary %q lacks %q", text, part)
		}
	}
}

func TestFixtureListsMentionCraftableFirstCrafts(t *testing.T) {
	controller := newFirstCraftTestController(t, firstCraftExpRows, firstCraftTestSnapshot)

	collection, err := controller.BuildFixtureListRequest(FixtureListQuery{Region: "jp", ObtainedSource: "fixture"})
	if err != nil {
		t.Fatalf("BuildFixtureListRequest(fixture): %v", err)
	}
	wantProgress := i18n.T("mysekai.image.fixture.progress_first_craft", i18n.Data{
		"Obtained": 3, "Total": 8, "Percent": i18n.Percent(percent(3, 8)), "Count": 3, "Exp": i18n.Wan(3 * 1234),
	})
	if collection.ProgressMessage == nil || *collection.ProgressMessage != wantProgress {
		t.Fatalf("progress = %v, want %q", collection.ProgressMessage, wantProgress)
	}
	if tiles := fixtureTiles(collection); !tiles[40] || tiles[41] || !tiles[10] || !tiles[11] {
		t.Fatalf("a canvas row or a quantity-0 row counts as obtained: %v", tiles)
	}

	onlyCraftable := true
	blueprints, err := controller.BuildFixtureListRequest(FixtureListQuery{Region: "jp", ObtainedSource: "blueprint", OnlyCraftable: &onlyCraftable})
	if err != nil {
		t.Fatalf("BuildFixtureListRequest(blueprint): %v", err)
	}
	if tiles := fixtureTiles(blueprints); !reflect.DeepEqual(tiles, map[int]bool{10: true, 11: true, 12: true, 13: false, 14: false, 40: false, 41: true}) {
		t.Fatalf("blueprint list tiles = %v, want canvases listed by their blueprints", tiles)
	}
	if blueprints.ProgressMessage == nil || !strings.Contains(*blueprints.ProgressMessage, i18n.Wan(3*1234).String()) {
		t.Fatalf("blueprint list progress = %v", blueprints.ProgressMessage)
	}

	noExp := newFirstCraftTestController(t, nil, firstCraftTestSnapshot)
	plain, err := noExp.BuildFixtureListRequest(FixtureListQuery{Region: "jp", ObtainedSource: "fixture"})
	if err != nil {
		t.Fatalf("BuildFixtureListRequest(no exp): %v", err)
	}
	if plain.ProgressMessage == nil || *plain.ProgressMessage != i18n.T("mysekai.image.fixture.progress", i18n.Data{"Obtained": 3, "Total": 8, "Percent": i18n.Percent(percent(3, 8))}) {
		t.Fatalf("without the exp row the progress stays as before, got %v", plain.ProgressMessage)
	}
}

func fixtureDetailInfo(t *testing.T, controller *Controller, query FixtureDetailQuery) []string {
	t.Helper()
	reqs, err := controller.BuildFixtureDetailRequests(query)
	if err != nil {
		t.Fatalf("BuildFixtureDetailRequests(%q): %v", query.Query, err)
	}
	return reqs[0].BasicInfo
}

func TestFixtureDetailShowsTheFirstCraftState(t *testing.T) {
	controller := newFirstCraftTestController(t, firstCraftExpRows, firstCraftTestSnapshot)
	cases := map[string]string{
		"10": i18n.T("mysekai.image.fixture.first_craft_used"),
		"11": i18n.T("mysekai.image.fixture.first_craft_used"),
		"12": i18n.T("mysekai.image.fixture.first_craft_available", i18n.Data{"Exp": 1234}),
		"13": i18n.T("mysekai.image.fixture.first_craft_available_no_blueprint", i18n.Data{"Exp": 1234}),
		"14": i18n.T("mysekai.image.fixture.first_craft_available", i18n.Data{"Exp": 1234}),
		"40": i18n.T("mysekai.image.fixture.first_craft_used"),
		"41": i18n.T("mysekai.image.fixture.first_craft_available", i18n.Data{"Exp": 1234}),
	}
	for id, want := range cases {
		info := fixtureDetailInfo(t, controller, FixtureDetailQuery{Region: "jp", Query: id, WithUserData: true})
		if len(info) == 0 || info[len(info)-1] != want {
			t.Errorf("fixture %s: info = %v, want it to end with %q", id, info, want)
		}
	}

	closed := fixtureDetailInfo(t, controller, FixtureDetailQuery{Region: "jp", Query: "14", WithUserData: true, NowMillis: firstCraftAfterTerm.UnixMilli()})
	if closed[len(closed)-1] != i18n.T("mysekai.image.fixture.first_craft_available_limited", i18n.Data{"Exp": 1234}) {
		t.Errorf("a closed party blueprint: %v", closed)
	}
	if info := fixtureDetailInfo(t, controller, FixtureDetailQuery{Region: "jp", Query: "15", WithUserData: true}); slices.ContainsFunc(info, isFirstCraftTag) {
		t.Errorf("a fixture without a blueprint has no first-craft tag: %v", info)
	}
	if info := fixtureDetailInfo(t, controller, FixtureDetailQuery{Region: "jp", Query: "12"}); slices.ContainsFunc(info, isFirstCraftTag) {
		t.Errorf("the static detail has no first-craft tag: %v", info)
	}
	static := newFirstCraftTestController(t, firstCraftExpRows, "")
	if info := fixtureDetailInfo(t, static, FixtureDetailQuery{Region: "jp", Query: "12", WithUserData: true}); slices.ContainsFunc(info, isFirstCraftTag) {
		t.Errorf("without a snapshot the detail stays static: %v", info)
	}
	noExp := newFirstCraftTestController(t, nil, firstCraftTestSnapshot)
	if info := fixtureDetailInfo(t, noExp, FixtureDetailQuery{Region: "jp", Query: "13", WithUserData: true}); info[len(info)-1] != i18n.T("mysekai.image.fixture.first_craft_available_no_exp_no_blueprint") {
		t.Errorf("without the exp row: %v", info)
	}
}

func isFirstCraftTag(info string) bool {
	return strings.Contains(info, "首次制作")
}
