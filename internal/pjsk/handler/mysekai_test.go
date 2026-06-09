package handler

import (
	"context"
	json "github.com/bytedance/sonic"
	"reflect"
	"slices"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
)

func TestMysekaiAliasRemap(t *testing.T) {
	resource := sekaiHandlers{}.MysekaiResourceHandle()
	if !slices.Contains(resource.Commands, "/msa") {
		t.Fatalf("resource aliases should contain /msa")
	}
	if slices.Contains(resource.Commands, "/msm") {
		t.Fatalf("resource aliases should not contain /msm")
	}
	if slices.Contains(resource.Commands, "/msr") {
		t.Fatalf("resource aliases should not contain /msr")
	}

	overview := sekaiHandlers{}.MysekaiOverviewHandle()
	if !slices.Contains(overview.Commands, "/msam") {
		t.Fatalf("overview aliases should contain /msam")
	}
	if slices.Contains(overview.Commands, "/msa") {
		t.Fatalf("overview aliases should not contain /msa")
	}

	musicRecord := sekaiHandlers{}.MysekaiMusicRecordHandle()
	if !slices.Contains(musicRecord.Commands, "/msr") {
		t.Fatalf("music record aliases should contain /msr")
	}
	if slices.Contains(musicRecord.Commands, "/msm") {
		t.Fatalf("music record aliases should not contain /msm")
	}

	mapHandle := sekaiHandlers{}.MysekaiMapHandle()
	if !slices.Contains(mapHandle.Commands, "/msm") {
		t.Fatalf("map aliases should contain /msm")
	}
	if !slices.Contains(mapHandle.Commands, "/msmap") {
		t.Fatalf("map aliases should contain /msmap")
	}

	previewHandle := sekaiHandlers{}.MysekaiPreviewHandle()
	if !slices.Contains(previewHandle.Commands, "/烤森预览") {
		t.Fatalf("preview aliases should contain /烤森预览")
	}
	if !slices.Contains(previewHandle.Commands, "/mspv") {
		t.Fatalf("preview aliases should contain /mspv")
	}

	housingSK := sekaiHandlers{}.MysekaiHousingSKHandle()
	if !slices.Contains(housingSK.Commands, "/百景sk") {
		t.Fatalf("housing sk aliases should contain /百景sk")
	}
}

func TestMysekaiOverviewHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.MysekaiOverviewHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	if h.GetPath() != "mysekai/overview" {
		t.Fatalf("handler path = %q", h.GetPath())
	}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msam",
		ArgText:    "13 all force",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-resource-map" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		MapIDs        []int `json:"map_ids"`
		ShowHarvested bool  `json:"show_harvested"`
		CheckTime     bool  `json:"check_time"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !reflect.DeepEqual(params.MapIDs, []int{5, 6}) {
		t.Fatalf("params.MapIDs = %+v", params.MapIDs)
	}
	if !params.ShowHarvested {
		t.Fatalf("params.ShowHarvested = %v", params.ShowHarvested)
	}
	if params.CheckTime {
		t.Fatalf("params.CheckTime = %v", params.CheckTime)
	}
}

func TestMysekaiMapHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.MysekaiMapHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	if h.GetPath() != "mysekai/map" {
		t.Fatalf("handler path = %q", h.GetPath())
	}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msm",
		ArgText:    "all",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-map" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		ShowHarvested bool `json:"show_harvested"`
		CheckTime     bool `json:"check_time"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.ShowHarvested {
		t.Fatalf("params.ShowHarvested = %v", params.ShowHarvested)
	}
	if !params.CheckTime {
		t.Fatalf("params.CheckTime = %v", params.CheckTime)
	}
}

func TestMysekaiPreviewHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.MysekaiPreviewHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	if h.GetPath() != "mysekai/preview" {
		t.Fatalf("handler path = %q", h.GetPath())
	}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/烤森预览",
		ArgText:    "1f2f3f force",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result == nil || result.Module != parser.ModuleMysekai || result.Mode != "mysekai-scene-preview" {
		t.Fatalf("unexpected command request: %+v", result)
	}

	var params struct {
		SiteIDs   []int `json:"site_ids"`
		CheckTime bool  `json:"check_time"`
	}
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !reflect.DeepEqual(params.SiteIDs, []int{2, 3, 4}) {
		t.Fatalf("params.SiteIDs = %+v", params.SiteIDs)
	}
	if params.CheckTime {
		t.Fatalf("params.CheckTime = %v", params.CheckTime)
	}
}

func TestMysekaiHousingSKHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.MysekaiHousingSKHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	if h.GetPath() != "mysekai/housing-sk" {
		t.Fatalf("handler path = %q", h.GetPath())
	}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/百景sk",
		ArgText:    "id=25 1-5 sample=2 interval=-1",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result == nil || result.Module != parser.ModuleMysekai || result.Mode != "mysekai-housing-sk" {
		t.Fatalf("unexpected command request: %+v", result)
	}

	var params rendermysekai.HousingCompetitionLineQuery
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.HousingID != 25 {
		t.Fatalf("params.HousingID = %d", params.HousingID)
	}
	if !reflect.DeepEqual(params.Ranks, []int{1, 2, 3, 4, 5}) {
		t.Fatalf("params.Ranks = %+v", params.Ranks)
	}
	if params.SampleCount != 2 || params.SampleIntervalMillis != -1 {
		t.Fatalf("unexpected sampling params: %+v", params)
	}
}

func TestMysekaiHousingSKHandleRejectsTooManyRanks(t *testing.T) {
	h := sekaiHandlers{}.MysekaiHousingSKHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/百景sk",
		ArgText:    "1-6",
	})
	if err == nil {
		t.Fatal("expected too many ranks error")
	}
}

func TestMysekaiMapHandleBuildsCommandRequestWithForce(t *testing.T) {
	h := sekaiHandlers{}.MysekaiMapHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msm",
		ArgText:    "all force",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-map" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		ShowHarvested bool `json:"show_harvested"`
		CheckTime     bool `json:"check_time"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.ShowHarvested {
		t.Fatalf("params.ShowHarvested = %v", params.ShowHarvested)
	}
	if params.CheckTime {
		t.Fatalf("params.CheckTime = %v", params.CheckTime)
	}
}

func TestMysekaiMapHandleBuildsSingleMapParams(t *testing.T) {
	h := sekaiHandlers{}.MysekaiMapHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msm",
		ArgText:    "1",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != "mysekai-map" {
		t.Fatalf("unexpected mode: %s", resolved.Mode)
	}

	var params struct {
		MapIDs        []int `json:"map_ids"`
		ShowHarvested bool  `json:"show_harvested"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !reflect.DeepEqual(params.MapIDs, []int{5}) {
		t.Fatalf("params.MapIDs = %+v", params.MapIDs)
	}
	if params.ShowHarvested {
		t.Fatalf("params.ShowHarvested = %v", params.ShowHarvested)
	}
}

func TestMysekaiMapHandleBuildsGardenMapParams(t *testing.T) {
	h := sekaiHandlers{}.MysekaiMapHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msm",
		ArgText:    "2",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != "mysekai-map" {
		t.Fatalf("unexpected mode: %s", resolved.Mode)
	}

	var params struct {
		MapIDs []int `json:"map_ids"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !reflect.DeepEqual(params.MapIDs, []int{7}) {
		t.Fatalf("params.MapIDs = %+v", params.MapIDs)
	}
}

func TestMysekaiMapHandleParsesCompactMapIndices(t *testing.T) {
	h := sekaiHandlers{}.MysekaiMapHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msm",
		ArgText:    "13 all",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}

	var params struct {
		MapIDs        []int `json:"map_ids"`
		ShowHarvested bool  `json:"show_harvested"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !reflect.DeepEqual(params.MapIDs, []int{5, 6}) {
		t.Fatalf("params.MapIDs = %+v", params.MapIDs)
	}
	if !params.ShowHarvested {
		t.Fatalf("params.ShowHarvested = %v", params.ShowHarvested)
	}
}

func TestMysekaiMapHandleRejectsInvalidMapIndex(t *testing.T) {
	h := sekaiHandlers{}.MysekaiMapHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msm",
		ArgText:    "9",
	})
	if err == nil {
		t.Fatalf("expected error for invalid map index")
	}
	if !strings.Contains(err.Error(), "地图编号仅支持 1-4") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMysekaiPhotoHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.MysekaiPhotoHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	if h.GetPath() != "mysekai/photo" {
		t.Fatalf("handler path = %q", h.GetPath())
	}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msp",
		ArgText:    "-1",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-photo" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		Seq int `json:"seq"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Seq != -1 {
		t.Fatalf("params.Seq = %d", params.Seq)
	}
}

func TestMysekaiBlueprintHandleBuildsCommandRequests(t *testing.T) {
	h := sekaiHandlers{}.MysekaiBlueprintHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	if h.GetPath() != "mysekai/talk-list" {
		t.Fatalf("handler path = %q", h.GetPath())
	}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msb",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() list error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-fixture-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var listParams struct {
		ShowID         bool   `json:"show_id"`
		OnlyCraftable  bool   `json:"only_craftable"`
		ObtainedSource string `json:"obtained_source"`
		ShowProfile    *bool  `json:"show_profile"`
		ShowProgress   *bool  `json:"show_progress"`
		ShowObtained   *bool  `json:"show_obtained"`
	}
	if err := json.Unmarshal(resolved.Params, &listParams); err != nil {
		t.Fatalf("unmarshal list params: %v", err)
	}
	if !listParams.ShowID || !listParams.OnlyCraftable {
		t.Fatalf("unexpected list params: %+v", listParams)
	}
	if listParams.ObtainedSource != "blueprint" {
		t.Fatalf("expected /msb list to use blueprint obtained source, got %+v", listParams)
	}
	if listParams.ShowProfile != nil || listParams.ShowProgress != nil || listParams.ShowObtained != nil {
		t.Fatalf("unexpected list params: %+v", listParams)
	}

	result, err = h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msb",
		ArgText:    "miku ln all",
	})
	if err != nil {
		t.Fatalf("Handle() talk error = %v", err)
	}

	resolved = result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-talk-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "light_sound miku" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}

	var talkParams struct {
		ShowID       bool `json:"show_id"`
		ShowAllTalks bool `json:"show_all_talks"`
	}
	if err := json.Unmarshal(resolved.Params, &talkParams); err != nil {
		t.Fatalf("unmarshal talk params: %v", err)
	}
	if !talkParams.ShowID || !talkParams.ShowAllTalks {
		t.Fatalf("unexpected talk params: %+v", talkParams)
	}

	_, err = h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msb",
		ArgText:    "not-a-character",
	})
	if err == nil {
		t.Fatal("expected invalid character query to fail")
	}
	if !strings.Contains(err.Error(), "/msb 角色名") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMysekaiBlueprintHandleSupportsCompactCharacterAliases(t *testing.T) {
	h := sekaiHandlers{}.MysekaiBlueprintHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msb",
		ArgText:    "akt vbs all",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-talk-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "street akt" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}

	var params struct {
		ShowID       bool `json:"show_id"`
		ShowAllTalks bool `json:"show_all_talks"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.ShowID || !params.ShowAllTalks {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestMysekaiTalkListHandleBuildsCommandRequests(t *testing.T) {
	h := sekaiHandlers{}.MysekaiTalkListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/烤森对话列表",
		ArgText:    "miku ln all",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-talk-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "light_sound miku" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}

	var params struct {
		ShowID       bool `json:"show_id"`
		ShowAllTalks bool `json:"show_all_talks"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.ShowID || !params.ShowAllTalks {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestMysekaiTalkListHandleWithoutQueryFallsBackToFixtureList(t *testing.T) {
	h := sekaiHandlers{}.MysekaiTalkListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/烤森对话列表",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-fixture-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		ShowID         bool   `json:"show_id"`
		OnlyCraftable  bool   `json:"only_craftable"`
		ObtainedSource string `json:"obtained_source"`
		ShowProfile    *bool  `json:"show_profile"`
		ShowProgress   *bool  `json:"show_progress"`
		ShowObtained   *bool  `json:"show_obtained"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.ShowID || !params.OnlyCraftable {
		t.Fatalf("unexpected params: %+v", params)
	}
	if params.ObtainedSource != "blueprint" {
		t.Fatalf("expected talk list fallback to use blueprint obtained source, got %+v", params)
	}
	if params.ShowProfile != nil || params.ShowProgress != nil || params.ShowObtained != nil {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestMysekaiFixtureListHandleSupportsFull(t *testing.T) {
	h := sekaiHandlers{}.MysekaiFixtureListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/烤森家具列表",
		ArgText:    "full",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-fixture-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		ShowID        bool `json:"show_id"`
		OnlyCraftable bool `json:"only_craftable"`
		ShowProfile   bool `json:"show_profile"`
		ShowProgress  bool `json:"show_progress"`
		ShowObtained  bool `json:"show_obtained"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.ShowID || params.OnlyCraftable || params.ShowProfile || params.ShowProgress || params.ShowObtained {
		t.Fatalf("expected full fixture list to be static without profile/progress/obtained, got %+v", params)
	}
}

func TestMysekaiBlueprintHandleRejectsFixtureIDs(t *testing.T) {
	h := sekaiHandlers{}.MysekaiBlueprintHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msb",
		ArgText:    "123",
	})
	if err == nil {
		t.Fatal("expected fixture id query to fail")
	}
	if !strings.Contains(err.Error(), "/msf 家具ID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMysekaiFurnitureHandleBuildsCommandRequests(t *testing.T) {
	h := sekaiHandlers{}.MysekaiFurnitureHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msf",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() list error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-fixture-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var listParams struct {
		ShowID         *bool  `json:"show_id"`
		OnlyCraftable  *bool  `json:"only_craftable"`
		ObtainedSource string `json:"obtained_source"`
		ShowProfile    *bool  `json:"show_profile"`
		ShowProgress   *bool  `json:"show_progress"`
		ShowObtained   *bool  `json:"show_obtained"`
		CategoryQuery  string `json:"category_query"`
	}
	if err := json.Unmarshal(resolved.Params, &listParams); err != nil {
		t.Fatalf("unmarshal list params: %v", err)
	}
	if listParams.ShowID == nil || !*listParams.ShowID {
		t.Fatalf("unexpected list params: %+v", listParams)
	}
	if listParams.ObtainedSource != "fixture" {
		t.Fatalf("expected default /msf to use fixture obtained source, got %+v", listParams)
	}
	if listParams.OnlyCraftable != nil || listParams.ShowProfile != nil || listParams.ShowProgress != nil || listParams.ShowObtained != nil || listParams.CategoryQuery != "" {
		t.Fatalf("expected default /msf to use dynamic fixture list defaults, got %+v", listParams)
	}

	result, err = h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msf",
		ArgText:    "full",
	})
	if err != nil {
		t.Fatalf("Handle() full list error = %v", err)
	}
	resolved = result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-fixture-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	var fullParams struct {
		ShowID       bool `json:"show_id"`
		ShowProfile  bool `json:"show_profile"`
		ShowProgress bool `json:"show_progress"`
		ShowObtained bool `json:"show_obtained"`
	}
	if err := json.Unmarshal(resolved.Params, &fullParams); err != nil {
		t.Fatalf("unmarshal full params: %v", err)
	}
	if !fullParams.ShowID || fullParams.ShowProfile || fullParams.ShowProgress || fullParams.ShowObtained {
		t.Fatalf("expected /msf full to use static all-lit params, got %+v", fullParams)
	}

	result, err = h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msf",
		ArgText:    "テーブル",
	})
	if err != nil {
		t.Fatalf("Handle() category list error = %v", err)
	}
	resolved = result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-fixture-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	var categoryParams struct {
		ShowID        *bool  `json:"show_id"`
		CategoryQuery string `json:"category_query"`
	}
	if err := json.Unmarshal(resolved.Params, &categoryParams); err != nil {
		t.Fatalf("unmarshal category params: %v", err)
	}
	if categoryParams.ShowID == nil || !*categoryParams.ShowID || categoryParams.CategoryQuery != "テーブル" {
		t.Fatalf("unexpected category params: %+v", categoryParams)
	}

	result, err = h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msf",
		ArgText:    "1 2",
	})
	if err != nil {
		t.Fatalf("Handle() detail error = %v", err)
	}

	resolved = result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-fixture-detail" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "1 2" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}
}

func TestMysekaiFurnitureHandleTreatsUnknownTextAsCategoryQuery(t *testing.T) {
	h := sekaiHandlers{}.MysekaiFurnitureHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msf",
		ArgText:    "miku",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result == nil || result.Module != parser.ModuleMysekai || result.Mode != "mysekai-fixture-list" {
		t.Fatalf("unexpected command request: %+v", result)
	}
	var params struct {
		CategoryQuery string `json:"category_query"`
	}
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.CategoryQuery != "miku" {
		t.Fatalf("category_query = %q", params.CategoryQuery)
	}
}

func TestMysekaiDoorUpgradeHandleSupportsShowAll(t *testing.T) {
	h := sekaiHandlers{}.MysekaiDoorUpgradeHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msg",
		ArgText:    "all",
		UserId:     "10001",
		Platform:   "qq",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-door-upgrade" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if strings.TrimSpace(resolved.Query) != "" {
		t.Fatalf("expected empty query for /msg all, got %q", resolved.Query)
	}

	var params struct {
		ShowAll        bool   `json:"show_all"`
		Mode           string `json:"mode"`
		Platform       string `json:"platform"`
		PlatformUserID string `json:"platform_user_id"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.ShowAll {
		t.Fatalf("expected show_all params, got %+v", params)
	}
	if params.Mode != "self" || params.Platform != "qq" || params.PlatformUserID != "10001" {
		t.Fatalf("unexpected self params: %+v", params)
	}
}

func TestMysekaiDoorUpgradeHandleSupportsShowFull(t *testing.T) {
	h := sekaiHandlers{}.MysekaiDoorUpgradeHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msg",
		ArgText:    "full",
		UserId:     "10001",
		Platform:   "qq",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-door-upgrade" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if strings.TrimSpace(resolved.Query) != "" {
		t.Fatalf("expected empty query for /msg full, got %q", resolved.Query)
	}

	var params struct {
		ShowAll  bool `json:"show_all"`
		ShowFull bool `json:"show_full"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.ShowFull || params.ShowAll {
		t.Fatalf("expected show_full without show_all, got %+v", params)
	}
}
