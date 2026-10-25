package mysekai

import (
	"math"
	"slices"
	"testing"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/usererror"
)

func convertRecipeOutputSearchTestData() convertRecipeTestSource {
	data := convertRecipeTestData()
	for _, spec := range []struct {
		id, tab, target int
		kind            string
	}{
		{2, 2, 20, "resource_box"},
		{3, 3, 30, "resource_box"},
		{4, 4, 40, "resource_box"},
		{5, 3, 3, "mysekai_blueprint"},
		{6, 1, 60, "resource_box"},
		{7, 3, 70, "resource_box"},
		{8, 1, 80, "resource_box"},
		{9, 1, 90, "resource_box"},
	} {
		data["mysekaiConvertItems.json"] = append(data["mysekaiConvertItems.json"], convertRecipeTestRow(spec.id, spec.tab, spec.kind, spec.target))
	}
	data["materials.json"] = append(data["materials.json"],
		map[string]any{"id": 16, "name": "Magic Cloth 12"},
		map[string]any{"id": 17, "name": "Magic"},
		map[string]any{"id": 18, "name": "Cloth"},
		map[string]any{"id": 19, "name": "蓝图颜料"},
		map[string]any{"id": 20, "name": "Wood"},
		map[string]any{"id": 21, "name": "Coin Fabric"},
		map[string]any{"id": 22, "name": "Level=2 Thread"},
	)
	for _, box := range []struct {
		id      int
		name    string
		details []any
	}{
		{20, "Stone product", []any{map[string]any{"resourceType": "mysekai_material", "resourceId": 6, "resourceQuantity": 1}}},
		{30, "Holiday Cloth Bundle", []any{map[string]any{"resourceType": "material", "resourceId": 16, "resourceQuantity": 1}}},
		{40, "Paper Pack", []any{
			map[string]any{"resourceType": "material", "resourceId": 17, "resourceQuantity": 1},
			map[string]any{"resourceType": "material", "resourceId": 18, "resourceQuantity": 1},
		}},
		{60, "Paint", []any{map[string]any{"resourceType": "material", "resourceId": 19, "resourceQuantity": 1}}},
		{70, "Wood product", []any{map[string]any{"resourceType": "material", "resourceId": 20, "resourceQuantity": 1}}},
		{80, "", []any{map[string]any{"resourceType": "material", "resourceId": 21, "resourceQuantity": 321}}},
		{90, "Thread", []any{map[string]any{"resourceType": "material", "resourceId": 22, "resourceQuantity": 1}}},
	} {
		data["resourceBoxes.json"] = append(data["resourceBoxes.json"], map[string]any{
			"id": box.id, "resourceBoxPurpose": "mysekai_convert_obtain", "name": box.name, "details": box.details,
		})
	}
	return data
}

func TestConvertRecipeSearchOnlyMatchesProductNames(t *testing.T) {
	controller := &Controller{masterdata: convertRecipeOutputSearchTestData()}
	for _, test := range []struct {
		query string
		ids   []int
	}{
		{"", []int{1, 2, 3, 4, 5, 6, 7, 8, 9}},
		{"心愿", []int{1}},
		{"石头", []int{2}},
		{"蓝图", []int{5, 6}},
		{"magic cloth", []int{3}},
		{"MAGIC CLOTH 12", []int{3}},
		{"Holiday Cloth", []int{3}},
		{"paper pack", []int{4}},
		{"coin fab", []int{8}},
		{"level=2", []int{9}},
		{"1", []int{3}},
		{"2", []int{3, 9}},
		{"12", []int{3}},
	} {
		t.Run(test.query, func(t *testing.T) {
			request, err := controller.BuildConvertRecipeRequest(ConvertRecipeQuery{Query: test.query})
			if err != nil {
				t.Fatal(err)
			}
			ids := convertRecipeTestIDs(request.Recipes)
			slices.Sort(ids)
			if !slices.Equal(ids, test.ids) {
				t.Fatalf("recipe IDs = %v, want %v", ids, test.ids)
			}
			if request.Filter != test.query || request.Total != len(test.ids) {
				t.Fatalf("filter/total = %q/%d", request.Filter, request.Total)
			}
		})
	}
	for _, query := range []string{
		"金币", "道具", "音色", "唱片", "normal", "全部", "all",
		"cloth magic", "心愿 石头", "8", "15", "321", "9223372036854775808",
		"材料=石头", "产物=蓝图", "等级<=2", "耗时<=2h", "分类=道具,蓝图",
	} {
		t.Run("no_match_"+query, func(t *testing.T) {
			_, err := controller.BuildConvertRecipeRequest(ConvertRecipeQuery{Query: query})
			testutil.RequireUserError(t, err, usererror.CodeNotFound, "convert.error.not_found")
		})
	}
	request, err := controller.BuildConvertRecipeRequest(ConvertRecipeQuery{Query: "  MaGiC    CLOTH  "})
	if err != nil {
		t.Fatal(err)
	}
	if got := convertRecipeTestIDs(request.Recipes); !slices.Equal(got, []int{3}) || request.Filter != "MaGiC CLOTH" {
		t.Fatalf("normalized search = %#v", request)
	}
	request, err = controller.BuildConvertRecipeRequest(ConvertRecipeQuery{Query: " \t\n "})
	if err != nil {
		t.Fatal(err)
	}
	if request.Total != 9 || request.Filter != "" {
		t.Fatalf("blank search = %#v", request)
	}
}

func TestConvertRecipeDurationDoesNotOverflow(t *testing.T) {
	if got := convertRecipeDuration(map[string]any{"convertMinutes": int64(math.MaxInt64)}); got != i18n.T("convert.image.unknown") {
		t.Fatalf("overflowed duration = %q", got)
	}
}
