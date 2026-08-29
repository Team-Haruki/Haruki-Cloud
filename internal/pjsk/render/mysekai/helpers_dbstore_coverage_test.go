//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package mysekai

import (
	"context"
	"database/sql"
	stdjson "encoding/json"
	"reflect"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestConversionHelpersCoverSupportedRepresentations(t *testing.T) {
	intCases := []struct {
		value any
		want  int
	}{
		{float64(1.9), 1}, {float32(2.9), 2}, {int(3), 3}, {int32(4), 4}, {int64(5), 5},
		{stdjson.Number("6"), 6}, {stdjson.Number("7.9"), 7}, {" 8 ", 8}, {"bad", -1}, {true, -1},
	}
	for _, tc := range intCases {
		if got := intNumber(tc.value, -1); got != tc.want {
			t.Fatalf("intNumber(%#v) = %d, want %d", tc.value, got, tc.want)
		}
	}
	if got := intNumberFrom(nil, 9, "id"); got != 9 {
		t.Fatalf("intNumberFrom(nil) = %d", got)
	}
	if got := intNumberFrom(map[string]any{"first": "bad", "second": "12"}, 9, "first", "second"); got != 12 {
		t.Fatalf("intNumberFrom aliases = %d", got)
	}

	floatCases := []struct {
		value any
		want  float64
	}{
		{float64(1.5), 1.5}, {float32(2.5), 2.5}, {int(3), 3}, {int32(4), 4}, {int64(5), 5},
		{stdjson.Number("6.5"), 6.5}, {" 7.5 ", 7.5}, {"bad", -1},
	}
	for _, tc := range floatCases {
		if got := floatNumber(tc.value, -1); got != tc.want {
			t.Fatalf("floatNumber(%#v) = %f, want %f", tc.value, got, tc.want)
		}
	}

	int64Cases := []struct {
		value any
		want  int64
	}{
		{float64(1.9), 1}, {float32(2.9), 2}, {int(3), 3}, {int32(4), 4}, {int64(5), 5},
		{stdjson.Number("9007199254740993"), 9007199254740993}, {stdjson.Number("7.9"), 7}, {" 8 ", 8}, {"bad", -1},
	}
	for _, tc := range int64Cases {
		if got := int64Number(tc.value, -1); got != tc.want {
			t.Fatalf("int64Number(%#v) = %d, want %d", tc.value, got, tc.want)
		}
	}

	truthy := []any{true, float64(1), float32(1), int(1), int32(1), int64(1), stdjson.Number("1"), "true", "YES", " y "}
	for _, value := range truthy {
		if !boolValue(value) {
			t.Fatalf("boolValue(%#v) = false", value)
		}
	}
	for _, value := range []any{false, float64(0), stdjson.Number("bad"), "no", struct{}{}} {
		if boolValue(value) {
			t.Fatalf("boolValue(%#v) = true", value)
		}
	}
	if boolValueFrom(nil, "ok") || !boolValueFrom(map[string]any{"ok": "1"}, "missing", "ok") {
		t.Fatal("boolValueFrom did not handle nil and aliases")
	}
	if got := stringValueFrom(nil, "name"); got != "" {
		t.Fatalf("stringValueFrom(nil) = %q", got)
	}
	if got := stringValueFrom(map[string]any{"one": 1, "two": " value "}, "one", "two"); got != "value" {
		t.Fatalf("stringValueFrom aliases = %q", got)
	}
	if stringValue(1) != "" || stringValue(" x ") != "x" {
		t.Fatal("stringValue conversion mismatch")
	}

	direct := []any{1, 2}
	updated := map[string]any{"updatedResources": map[string]any{"items": []any{3}}}
	if got := nestedList(map[string]any{"items": direct}, "items"); !reflect.DeepEqual(got, direct) {
		t.Fatalf("nestedList direct = %#v", got)
	}
	if got := nestedList(updated, "items"); !reflect.DeepEqual(got, []any{3}) {
		t.Fatalf("nestedList updated = %#v", got)
	}
	if nestedList(nil, "items") != nil || nestedList(map[string]any{}, "items") != nil {
		t.Fatal("nestedList missing should return nil")
	}
	if nestedInt(nil, "size", "width") != 0 || nestedInt(map[string]any{"size": 1}, "size", "width") != 0 || nestedInt(map[string]any{"size": map[string]any{"width": "4"}}, "size", "width") != 4 {
		t.Fatal("nestedInt conversion mismatch")
	}
	if got := parseIntTokens("1, 2，2\t-1 bad\n3"); !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("parseIntTokens = %v", got)
	}
}

func TestFixtureHelpersCoverAlternateLayoutsAndEdges(t *testing.T) {
	resolve := func(path string) string { return "resolved/" + path }
	characters := map[int]map[string]any{5: {"givenName": "みのり"}, 6: {"givenName": ""}}
	if got := birthdayCharacterID(characters, "家具（みのり）"); got != 5 {
		t.Fatalf("birthdayCharacterID = %d", got)
	}
	if got := birthdayCharacterID(characters, "家具"); got != 0 {
		t.Fatalf("birthdayCharacterID missing = %d", got)
	}
	if fixtureThumbnailPath(resolve, map[string]any{}) != "" {
		t.Fatal("fixture without an asset bundle should not have a thumbnail")
	}
	if got := fixtureThumbnailPath(resolve, map[string]any{"assetbundle_name": "floor", "mysekai_fixture_type": "surface_appearance"}); got != "resolved/mysekai/thumbnail/surface_appearance/floor/tex_floor_floor_appearance_1.png" {
		t.Fatalf("surface thumbnail = %q", got)
	}
	if got := fixtureThumbnailPath(resolve, map[string]any{"assetbundleName": "wall", "mysekaiFixtureType": "surface_appearance", "mysekaiSettableLayoutType": "wall"}); got != "resolved/mysekai/thumbnail/surface_appearance/wall/tex_wall_wall_1.png" {
		t.Fatalf("wall thumbnail = %q", got)
	}
	if got := fixtureThumbnailPath(resolve, map[string]any{"assetbundleName": "chair"}); got != "resolved/mysekai/thumbnail/fixture/chair_1.png" {
		t.Fatalf("fixture thumbnail = %q", got)
	}

	if fixtureColorImages(resolve, map[string]any{}) != nil {
		t.Fatal("fixtureColorImages without a thumbnail should return nil")
	}
	baseOnly := fixtureColorImages(resolve, map[string]any{"assetbundleName": "chair", "colorCode": "#fff"})
	if len(baseOnly) != 1 || baseOnly[0].ColorCode == nil {
		t.Fatalf("base fixture colors = %+v", baseOnly)
	}
	colors := fixtureColorImages(resolve, map[string]any{
		"assetbundleName":           "floor",
		"mysekaiFixtureType":        "surface_appearance",
		"mysekaiSettableLayoutType": "",
		"mysekaiFixtureAnotherColors": []any{
			map[string]any{"color_code": "#123456"},
			"invalid color",
		},
	})
	if len(colors) != 3 || colors[1].ColorCode == nil || colors[2].ColorCode != nil {
		t.Fatalf("surface fixture colors = %+v", colors)
	}

	positiveInfo := fixtureBasicInfo(map[string]any{
		"isAssembled": true, "isDisassembled": true, "mysekaiFixturePlayerActionType": "sit", "isGameCharacterAction": true,
	})
	negativeInfo := fixtureBasicInfo(map[string]any{"mysekaiFixturePlayerActionType": "no_action"})
	if len(positiveInfo) != 4 || len(negativeInfo) != 4 || positiveInfo[0] == negativeInfo[0] {
		t.Fatalf("fixture basic info positive=%v negative=%v", positiveInfo, negativeInfo)
	}
	if got := fixtureBlueprintInfo(map[string]any{"isEnableSketch": true, "isObtainedByConvert": true, "craftCountLimit": 2}); len(got) != 3 || got[2] != "【最多制作2次】" {
		t.Fatalf("limited blueprint info = %v", got)
	}
	if got := fixtureBlueprintInfo(map[string]any{}); len(got) != 3 || got[2] != "【无制作次数限制】" {
		t.Fatalf("unlimited blueprint info = %v", got)
	}
	if fixtureTags(map[string]any{}, nil) != nil {
		t.Fatal("fixtureTags without a group should return nil")
	}
	tags := fixtureTags(map[string]any{"mysekaiFixtureTagGroup": map[string]any{
		"mysekaiFixtureTagId1": 1, "mysekaiFixtureTagId2": 2, "mysekaiFixtureTagId3": 0,
	}}, map[int]map[string]any{1: {"name": "cute"}, 2: {"name": ""}})
	if !reflect.DeepEqual(tags, []string{"cute"}) {
		t.Fatalf("fixtureTags = %v", tags)
	}
	blueprints := []map[string]any{{"mysekaiCraftType": "item", "craftTargetId": 7}, {"mysekaiCraftType": "mysekai_fixture", "craftTargetId": 7}}
	if findFixtureBlueprint(blueprints, 7) == nil || findFixtureBlueprint(blueprints, 8) != nil {
		t.Fatal("findFixtureBlueprint mismatch")
	}
	if charaIconName(1) != "ick" || charaIconName(999) != "miku" {
		t.Fatal("charaIconName fallback mismatch")
	}
	group := map[string]any{"gameCharacterUnitId1": 1, "game_character_unit_id2": 2, "game_character_unit_id_3": 3}
	if got := extractGroupCuids(group); !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("extractGroupCuids = %v", got)
	}
	if !containsInt([]int{1, 2}, 2) || containsInt([]int{1, 2}, 3) {
		t.Fatal("containsInt mismatch")
	}
	if !intsEqual([]int{1, 2}, []int{1, 2}) || intsEqual([]int{1}, []int{1, 2}) || intsEqual([]int{1, 3}, []int{1, 2}) {
		t.Fatal("intsEqual mismatch")
	}
	if !hasFixture(map[int]struct{}{7: {}}, 7) || hasFixture(nil, 7) || percent(1, 0) != 0 || percent(1, 4) != 25 {
		t.Fatal("fixture helper edge mismatch")
	}
	if !isMusicAvailableNow([]map[string]any{{"startAt": 10, "endAt": 20}}, 15) || isMusicAvailableNow([]map[string]any{{"startAt": 10, "endAt": 20}}, 21) {
		t.Fatal("music availability mismatch")
	}
}

func TestDBMasterdataStoreDefensiveAndConversionBranches(t *testing.T) {
	var nilStore *dbMasterdataStore
	if nilStore.Configured() || nilStore.WithContext(context.Background()) != nil || nilStore.contextOrBackground() == nil {
		t.Fatal("nil db store defensive behavior mismatch")
	}
	nilStore.resetCache()
	nilStore.Close()
	if nilStore.loadList("musics.json") != nil || len(nilStore.loadMapByID("musics.json")) != 0 || nilStore.loadObject("x", nil) {
		t.Fatal("nil db store load behavior mismatch")
	}
	if newDBMasterdataStore(context.Background(), " ", "jp") != nil {
		t.Fatal("blank DSN should not configure a DB store")
	}

	store := newTestDBMasterdataStore(t)
	if !store.Configured() || store.WithContext(nil) == nil || store.contextOrBackground() == nil {
		t.Fatal("configured DB store behavior mismatch")
	}
	if store.loadList("unknown.json") != nil {
		t.Fatal("unknown masterdata filename should not query a table")
	}
	if got := store.loadList("musics.json"); len(got) != 1 {
		t.Fatalf("cold load = %+v", got)
	}
	if got := store.loadList("musics.json"); len(got) != 1 {
		t.Fatalf("cached load = %+v", got)
	}
	if got := store.loadMapByID("musics.json"); len(got) != 1 || stringValue(got[1]["title"]) != "Test Song" {
		t.Fatalf("cold map = %+v", got)
	}
	if got := store.loadMapByID("musics.json"); len(got) != 1 {
		t.Fatalf("cached map = %+v", got)
	}

	withoutCache := *store
	withoutCache.cache = nil
	if withoutCache.loadList("musics.json") != nil || len(withoutCache.loadMapByID("musics.json")) != 0 {
		t.Fatal("store without cache should not load data")
	}
	withoutDB := *store
	withoutDB.db = nil
	if withoutDB.Configured() || withoutDB.loadList("musics.json") != nil || len(withoutDB.loadMapByID("musics.json")) != 0 {
		t.Fatal("store without DB should not load data")
	}

	brokenDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open broken DB: %v", err)
	}
	broken := &dbMasterdataStore{db: brokenDB, region: "jp", cache: &dbMasterdataCache{lists: map[string][]map[string]any{}, mapsByID: map[string]map[int]map[string]any{}}}
	if got := broken.loadList("musics.json"); got != nil {
		t.Fatalf("querying a missing table = %+v", got)
	}
	broken.Close()

	if mapColumnName("id") != "" || mapColumnName("game_id") != "id" || mapColumnName("server_region") != "" || mapColumnName("my_field_name") != "myFieldName" {
		t.Fatal("mapColumnName mismatch")
	}
	if snakeToCamel("") != "" || snakeToCamel("already") != "already" || snakeToCamel("my__field") != "myField" {
		t.Fatal("snakeToCamel mismatch")
	}
	if got := normalizeValue([]byte(`{"id":9007199254740993}`), nil, 0); got == nil {
		t.Fatal("normalizeValue valid JSON returned nil")
	}
	if got := normalizeValue([]byte("plain text"), nil, 0); got != "plain text" {
		t.Fatalf("normalizeValue text = %#v", got)
	}
	if got := normalizeValue(int64(7), nil, 0); got != int64(7) {
		t.Fatalf("normalizeValue integer = %#v", got)
	}
	type marker struct{ Value int }
	m := marker{Value: 1}
	if got := normalizeValue(m, nil, 0); got != m {
		t.Fatalf("normalizeValue default = %#v", got)
	}
}
