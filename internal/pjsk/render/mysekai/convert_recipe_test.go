package mysekai

import (
	"slices"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/i18n"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/usererror"
)

type convertRecipeTestSource map[string][]map[string]any

func (convertRecipeTestSource) Configured() bool { return true }
func (s convertRecipeTestSource) loadList(filename string) []map[string]any {
	return s[filename]
}
func (s convertRecipeTestSource) loadMapByID(filename string) map[int]map[string]any {
	result := make(map[int]map[string]any)
	for _, row := range s[filename] {
		result[intNumber(row["id"], 0)] = row
	}
	return result
}
func (convertRecipeTestSource) loadObject(string, any) bool { return false }

func convertRecipeTestData() convertRecipeTestSource {
	return convertRecipeTestSource{
		"mysekaiConvertItems.json": {convertRecipeTestRow(1, 1, "resource_box", 10)},
		"mysekaiConvertItemTabs.json": {
			{"id": 1, "seq": 10, "mysekaiConvertItemTabType": "normal", "name": "アイテム"},
			{"id": 2, "seq": 20, "mysekaiConvertItemTabType": "tone", "name": "音色"},
			{"id": 3, "seq": 30, "mysekaiConvertItemTabType": "mysekai_blueprint"},
			{"id": 4, "seq": 40, "mysekaiConvertItemTabType": "mysekai_music_record"},
			{"id": 5, "seq": 50, "mysekaiConvertItemTabType": "limited"},
		},
		"resourceBoxes.json": {
			{"id": 10, "resourceBoxPurpose": "mysekai_convert_consumption", "details": []any{
				map[string]any{"resourceType": "mysekai_material", "resourceId": 6, "resourceQuantity": 100},
				map[string]any{"resourceType": "coin", "resourceQuantity": 1000},
			}},
			{"id": 10, "resourceBoxPurpose": "mysekai_convert_obtain", "name": "想いのカケラ", "details": []any{
				map[string]any{"resourceType": "material", "resourceId": 15, "resourceQuantity": 15},
			}},
			{"id": 10, "resourceBoxPurpose": "mission_reward", "name": "错误奖励", "details": []any{
				map[string]any{"resourceType": "material", "resourceId": 999, "resourceQuantity": 999},
			}},
		},
		"materials.json":        {{"id": 15, "name": "心愿碎片"}},
		"mysekaiMaterials.json": {{"id": 6, "name": "石头", "iconAssetbundleName": "item_stone"}},
		"mysekaiConvertItemObtainMysekaiBlueprints.json": {
			{"id": 1, "mysekaiConvertItemObtainMysekaiBlueprintType": "random", "quantity": 1},
			{"id": 2, "mysekaiConvertItemObtainMysekaiBlueprintType": "mysekai_fixture_main_genre_choice", "quantity": 1},
			{"id": 3, "mysekaiConvertItemObtainMysekaiBlueprintType": "white_blueprint", "quantity": 2},
		},
		"mysekaiConvertItemObtainMysekaiMusicRecords.json": {
			{"id": 1, "mysekaiConvertItemObtainMysekaiMusicRecordType": "random", "quantity": 1},
			{"id": 2, "mysekaiConvertItemObtainMysekaiMusicRecordType": "mysekai_music_record_category_choice", "quantity": 1},
		},
	}
}

func convertRecipeTestRow(id, tab int, obtainType string, target int) map[string]any {
	return map[string]any{
		"id": id, "mysekaiConvertItemTabId": tab, "consumeResourceBoxId": 10,
		"mysekaiConvertItemObtainType": obtainType, "externalId": target,
		"requiredConvertFixtureLevel": 1, "convertMinutes": 2640,
		"refreshCycle": "none", "onceConvertLimit": 1,
	}
}

func TestConvertRecipeUsesPurposeScopedBoxesWithoutUserSnapshot(t *testing.T) {
	data := convertRecipeTestData()
	row := data["mysekaiConvertItems.json"][0]
	row["refreshCycle"], row["convertLimit"], row["onceConvertLimit"] = "monthly", 50, 10
	row["requiredConvertFixtureLevel"] = 2
	controller := &Controller{masterdata: data, defaultRegion: renderregion.CN}
	request, err := controller.BuildConvertRecipeRequest(ConvertRecipeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if request.Title != i18n.T("convert.image.title", i18n.Data{"Region": i18n.RegionLabel("cn")}) || request.Total != 1 {
		t.Fatalf("request metadata = %#v", request)
	}
	if len(request.Recipes) != 1 {
		t.Fatalf("recipes = %#v", request.Recipes)
	}
	recipe := request.Recipes[0]
	if recipe.CategoryKey != "normal" || recipe.RequiredLevel != 2 || len(recipe.Inputs) != 2 || len(recipe.Outputs) != 1 {
		t.Fatalf("recipe = %#v", recipe)
	}
	for _, resource := range []struct {
		actual       drawing.MysekaiConvertResource
		id, quantity int
		kind, path   string
	}{
		{recipe.Inputs[0], 6, 100, "mysekai_material", "asset/cn-assets/ondemand/mysekai/thumbnail/material/item_stone.png"},
		{recipe.Inputs[1], 0, 1000, "coin", "asset/cn-assets/startapp/thumbnail/common_material/coin.png"},
		{recipe.Outputs[0], 15, 15, "material", "asset/cn-assets/startapp/thumbnail/material/material15.png"},
	} {
		if resource.actual.ID != resource.id || resource.actual.Quantity == nil || *resource.actual.Quantity != resource.quantity || resource.actual.ResourceType != resource.kind || resource.actual.ImagePath != resource.path {
			t.Errorf("resource = %#v, want %#v", resource.actual, resource)
		}
	}
	for _, want := range []string{
		request.Title, "共 1 条配方", "[1] 想いのカケラ · 道具", "投入：石头 × 100 + 金币 × 1000",
		"产出：心愿碎片 × 15", "基础耗时：" + i18n.FormatDuration(44*time.Hour).String(), "转换器等级：Lv.2", "单次最多 10 次；每月最多 50 次",
	} {
		if !strings.Contains(request.Text, want) {
			t.Errorf("markdown missing %q:\n%s", want, request.Text)
		}
	}
	if strings.Contains(request.Text, "999") || strings.Contains(request.Text, "错误奖励") {
		t.Fatalf("resource box from another purpose leaked into recipe:\n%s", request.Text)
	}
}

func TestConvertRecipeSearchAndSpecialOutputs(t *testing.T) {
	data := convertRecipeTestData()
	data["mysekaiConvertItems.json"] = append(data["mysekaiConvertItems.json"],
		convertRecipeTestRow(11, 2, "resource_box", 10),
		convertRecipeTestRow(22, 3, "mysekai_blueprint", 1),
		convertRecipeTestRow(23, 3, "mysekai_blueprint", 1),
		convertRecipeTestRow(24, 3, "mysekai_blueprint", 2),
		convertRecipeTestRow(25, 3, "mysekai_blueprint", 3),
		convertRecipeTestRow(26, 4, "mysekai_music_record", 1),
		convertRecipeTestRow(27, 4, "mysekai_music_record", 2),
	)
	controller := &Controller{masterdata: data}
	for _, test := range []struct {
		name, query string
		ids         []int
		text        string
	}{
		{"output name", "心愿", []int{1, 11}, "心愿碎片 × 15"},
		{"box name", "想い", []int{1, 11}, "想いのカケラ"},
		{"duplicate random blueprints", "随机蓝图", []int{22, 23}, "随机蓝图 × 1"},
		{"blueprint output substring", "蓝图", []int{22, 23, 24, 25}, "自选分类蓝图 × 1"},
		{"blank blueprint quantity", "空白蓝图", []int{25}, "空白蓝图 × 2"},
		{"record output substring", "唱片", []int{26, 27}, "自选分类唱片 × 1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, err := controller.BuildConvertRecipeRequest(ConvertRecipeQuery{Query: test.query})
			if err != nil {
				t.Fatal(err)
			}
			if got := convertRecipeTestIDs(request.Recipes); !slices.Equal(got, test.ids) {
				t.Fatalf("recipe IDs = %v, want %v:\n%s", got, test.ids, request.Text)
			}
			if !strings.Contains(request.Text, test.text) {
				t.Fatalf("missing %q:\n%s", test.text, request.Text)
			}
		})
	}
	for _, query := range []string{"1", "999", "不存在的物品", "限时", "石头", "道具", "音色", "道具 唱片"} {
		_, err := controller.BuildConvertRecipeRequest(ConvertRecipeQuery{Query: query})
		testutil.RequireUserError(t, err, usererror.CodeNotFound, "convert.error.not_found")
	}
}

func TestConvertRecipeFiltersReleaseWindowsAtBoundaries(t *testing.T) {
	data := convertRecipeTestData()
	data["mysekaiConvertItems.json"] = nil
	now := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	for _, window := range []struct {
		id         int
		start, end int64
	}{
		{1, now.UnixMilli(), now.UnixMilli() + 1},
		{2, 0, now.UnixMilli()},
		{3, now.UnixMilli() + 1, 0},
		{4, 0, 0},
	} {
		row := convertRecipeTestRow(window.id, 5, "resource_box", 10)
		row["startAt"], row["endAt"] = window.start, window.end
		data["mysekaiConvertItems.json"] = append(data["mysekaiConvertItems.json"], row)
	}
	controller := &Controller{masterdata: data}
	request, err := controller.buildConvertRecipeRequest(ConvertRecipeQuery{Query: "心愿"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := convertRecipeTestIDs(request.Recipes); !slices.Equal(got, []int{1, 4}) {
		t.Fatalf("available recipe IDs = %v, want [1 4]", got)
	}
}

func TestConvertRecipeIncludesAllMatchesInOneRequest(t *testing.T) {
	data := convertRecipeTestData()
	data["mysekaiConvertItems.json"] = nil
	for id := 58; id >= 1; id-- {
		data["mysekaiConvertItems.json"] = append(data["mysekaiConvertItems.json"], convertRecipeTestRow(id, 1, "resource_box", 10))
	}
	controller := &Controller{masterdata: data}
	request, err := controller.BuildConvertRecipeRequest(ConvertRecipeQuery{Query: "心愿"})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := make([]int, 58)
	for i := range wantIDs {
		wantIDs[i] = i + 1
	}
	if got := convertRecipeTestIDs(request.Recipes); !slices.Equal(got, wantIDs) {
		t.Fatalf("recipe IDs = %v, want every matching recipe", got)
	}
	if request.Filter != "心愿" || request.Total != 58 {
		t.Fatalf("unexpected request metadata: %#v", request)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"page", "page_count", "previous_command", "next_command"} {
		if strings.Contains(string(encoded), `"`+field+`":`) {
			t.Errorf("unexpected pagination field %q", field)
		}
	}
}

func TestConvertRecipeMissingNamesAndMetadataStayVisible(t *testing.T) {
	data := convertRecipeTestData()
	delete(data, "materials.json")
	data["resourceBoxes.json"][0]["details"] = []any{
		map[string]any{"resourceType": "mysekai_item", "resourceId": 7, "resourceQuantity": 5},
		map[string]any{"resourceType": "practice_ticket", "resourceId": 2, "resourceQuantity": 30},
		map[string]any{"resourceType": "future_resource", "resourceId": 3, "resourceQuantity": 2},
	}
	data["mysekaiItems.json"] = []map[string]any{{"id": 7, "name": "图纸碎片"}}
	data["practiceTickets.json"] = []map[string]any{{"id": 2, "name": "练习乐谱"}}
	data["mysekaiConvertItems.json"] = append(data["mysekaiConvertItems.json"], map[string]any{
		"id": 2, "mysekaiConvertItemTabId": 99, "consumeResourceBoxId": 999,
		"mysekaiConvertItemObtainType": "new_obtain_type", "externalId": 88,
	})
	request, err := (&Controller{masterdata: data}).BuildConvertRecipeRequest(ConvertRecipeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"图纸碎片 × 5 + 练习乐谱 × 30 + future_resource/3 × 2", "material/15 × 15",
		"资源数据缺失（mysekai_convert_consumption/999）", "new_obtain_type/88 × 数量未知",
		"基础耗时：未知；转换器等级：未知", "共 2 条配方",
	} {
		if !strings.Contains(request.Text, want) {
			t.Errorf("missing %q:\n%s", want, request.Text)
		}
	}
	_, err = (*Controller)(nil).BuildConvertRecipeRequest(ConvertRecipeQuery{})
	testutil.RequireUserError(t, err, usererror.CodeMisconfigured, "common.misconfigured")
	_, err = (&Controller{masterdata: convertRecipeTestSource{}}).BuildConvertRecipeRequest(ConvertRecipeQuery{})
	testutil.RequireUserError(t, err, usererror.CodeUnavailable, "convert.error.data_unavailable")
}

func TestConvertRecipeIconPathsAndNullableQuantities(t *testing.T) {
	data := convertRecipeTestData()
	catalog := loadConvertRecipeCatalog(data)
	for _, test := range []struct {
		kind     string
		id       int
		metadata map[string]any
		path     string
	}{
		{"coin", 0, nil, "thumbnail/common_material/coin.png"},
		{"material", 15, nil, "thumbnail/material/material15.png"},
		{"boost_item", 1, nil, "thumbnail/boost_item/boost_item1.png"},
		{"practice_ticket", 2, nil, "thumbnail/practice_ticket/ticket2.png"},
		{"skill_practice_ticket", 2, nil, "thumbnail/skill_practice_ticket/ticket2.png"},
		{"mysekai_item", 7, map[string]any{"iconAssetbundleName": "item_blueprint_fragment"}, "mysekai/thumbnail/item/item_blueprint_fragment.png"},
		{"mysekai_material", 6, map[string]any{"iconAssetbundleName": "item_stone"}, "mysekai/thumbnail/material/item_stone.png"},
		{"mysekai_material", 9, nil, ""},
		{"future_resource", 9, nil, ""},
	} {
		if got := catalog.resourceImage(test.kind, test.id, test.metadata); got != test.path {
			t.Errorf("%s/%d image = %q, want %q", test.kind, test.id, got, test.path)
		}
	}
	for _, test := range []struct {
		kind  string
		id    int
		image string
	}{
		{"mysekai_blueprint", 1, "mysekai/icon/category_icon/icon_blueprint.png"},
		{"mysekai_blueprint", 2, "mysekai/icon/category_icon/icon_blueprint.png"},
		{"mysekai_blueprint", 3, "mysekai/thumbnail/item/item_blank_blueprint.png"},
		{"mysekai_music_record", 1, "mysekai/icon/category_icon/icon_record_jacket.png"},
		{"mysekai_music_record", 2, "mysekai/icon/category_icon/icon_record_jacket.png"},
	} {
		_, output := catalog.output(convertRecipeTestRow(1, 1, test.kind, test.id))
		if len(output) != 1 || output[0].ImagePath != test.image || output[0].Quantity == nil || output[0].ResourceType != test.kind || output[0].ID != test.id {
			t.Errorf("%s/%d output = %#v", test.kind, test.id, output)
		}
	}
	request := &drawing.MysekaiConvertRecipesRequest{Text: "PRIVATE FALLBACK", Recipes: []drawing.MysekaiConvertRecipe{{
		Inputs:  []drawing.MysekaiConvertResource{{Name: "unknown", Quantity: convertRecipeQuantity(nil)}},
		Outputs: []drawing.MysekaiConvertResource{{Name: "zero", Quantity: convertRecipeQuantity(0)}},
	}}}
	dataJSON, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(dataJSON, &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"text", "Text", "markdown", "path"} {
		if _, exists := body[key]; exists {
			t.Fatalf("internal fallback field %q was serialized", key)
		}
	}
	if strings.Contains(string(dataJSON), request.Text) || !strings.Contains(string(dataJSON), `"quantity":null`) || !strings.Contains(string(dataJSON), `"quantity":0`) {
		t.Fatalf("quantity/fallback JSON = %s", dataJSON)
	}
}

func convertRecipeTestIDs(recipes []drawing.MysekaiConvertRecipe) []int {
	ids := make([]int, 0, len(recipes))
	for _, recipe := range recipes {
		ids = append(ids, recipe.ID)
	}
	return ids
}
