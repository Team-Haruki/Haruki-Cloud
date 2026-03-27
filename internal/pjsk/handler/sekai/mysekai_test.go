package sekai

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

func TestMysekaiPhotoHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.MysekaiPhotoHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	if h.GetPath() != "mysekai/photo" {
		t.Fatalf("handler path = %q", h.GetPath())
	}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msp",
		ArgText:    "-1",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
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

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msb",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() list error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
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

	result, err = h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msb",
		ArgText:    "miku all",
	})
	if err != nil {
		t.Fatalf("Handle() talk error = %v", err)
	}

	resolved, ok = result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-talk-list" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}
	if resolved.Query != "miku" {
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
}
