package sekai

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
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
}

func TestMysekaiOverviewHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.MysekaiOverviewHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	if h.GetPath() != "mysekai/overview" {
		t.Fatalf("handler path = %q", h.GetPath())
	}

	result, err := h.Handle(&handler.PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msam",
		ArgText:    "13 all force",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-resource-map" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
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

func TestMysekaiMapHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.MysekaiMapHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	if h.GetPath() != "mysekai/map" {
		t.Fatalf("handler path = %q", h.GetPath())
	}

	result, err := h.Handle(&handler.PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msm",
		ArgText:    "all",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-map" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params struct {
		ShowHarvested bool `json:"show_harvested"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.ShowHarvested {
		t.Fatalf("params.ShowHarvested = %v", params.ShowHarvested)
	}
}

func TestMysekaiMapHandleBuildsSingleMapParams(t *testing.T) {
	h := sekaiHandlers{}.MysekaiMapHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msm",
		ArgText:    "1",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
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

	result, err := h.Handle(&handler.PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msm",
		ArgText:    "2",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
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

	result, err := h.Handle(&handler.PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msm",
		ArgText:    "13 all",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
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

	_, err := h.Handle(&handler.PjskHandlerContext{
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

func TestMysekaiPhotoHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.MysekaiPhotoHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	if h.GetPath() != "mysekai/photo" {
		t.Fatalf("handler path = %q", h.GetPath())
	}

	result, err := h.Handle(&handler.PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msp",
		ArgText:    "-1",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-photo" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
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

func TestMysekaiBlueprintHandleBuildsResolvedCommands(t *testing.T) {
	h := sekaiHandlers{}.MysekaiBlueprintHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	if h.GetPath() != "mysekai/blueprint" {
		t.Fatalf("handler path = %q", h.GetPath())
	}

	result, err := h.Handle(&handler.PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msb",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() list error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-fixture-list" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var listParams struct {
		ShowID        bool `json:"show_id"`
		OnlyCraftable bool `json:"only_craftable"`
	}
	if err := json.Unmarshal(resolved.Params, &listParams); err != nil {
		t.Fatalf("unmarshal list params: %v", err)
	}
	if !listParams.ShowID || !listParams.OnlyCraftable {
		t.Fatalf("unexpected list params: %+v", listParams)
	}

	result, err = h.Handle(&handler.PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msb",
		ArgText:    "miku ln all",
	})
	if err != nil {
		t.Fatalf("Handle() talk error = %v", err)
	}

	resolved = result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-talk-list" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
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

	_, err = h.Handle(&handler.PjskHandlerContext{
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

	result, err := h.Handle(&handler.PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msb",
		ArgText:    "akt vbs all",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-talk-list" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
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

func TestMysekaiBlueprintHandleRejectsFixtureIDs(t *testing.T) {
	h := sekaiHandlers{}.MysekaiBlueprintHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	_, err := h.Handle(&handler.PjskHandlerContext{
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

func TestMysekaiFurnitureHandleBuildsResolvedCommands(t *testing.T) {
	h := sekaiHandlers{}.MysekaiFurnitureHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msf",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() list error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-fixture-list" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var listParams struct {
		ShowID        bool `json:"show_id"`
		OnlyCraftable bool `json:"only_craftable"`
	}
	if err := json.Unmarshal(resolved.Params, &listParams); err != nil {
		t.Fatalf("unmarshal list params: %v", err)
	}
	if !listParams.ShowID || listParams.OnlyCraftable {
		t.Fatalf("unexpected list params: %+v", listParams)
	}

	result, err = h.Handle(&handler.PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msf",
		ArgText:    "1 2",
	})
	if err != nil {
		t.Fatalf("Handle() detail error = %v", err)
	}

	resolved = result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-fixture-detail" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}
	if resolved.Query != "1 2" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}
}

func TestMysekaiFurnitureHandleRejectsCharacterQuery(t *testing.T) {
	h := sekaiHandlers{}.MysekaiFurnitureHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	_, err := h.Handle(&handler.PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msf",
		ArgText:    "miku",
	})
	if err == nil {
		t.Fatal("expected character query to fail")
	}
	if !strings.Contains(err.Error(), "/msb 角色名") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMysekaiDoorUpgradeHandleSupportsShowAll(t *testing.T) {
	h := sekaiHandlers{}.MysekaiDoorUpgradeHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.PjskHandlerContext{
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
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-door-upgrade" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
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
