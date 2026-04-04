package sekai

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

func TestNoteNumHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.NoteNumHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/物量",
		ArgText:    "777",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-note-count" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params struct {
		NoteCount int `json:"note_count"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.NoteCount != 777 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestBPMHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.BPMHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查BPM",
		ArgText:    "テオ ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-bpm" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}
	if resolved.Query != "テオ" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}

	var params struct {
		Difficulty string `json:"difficulty"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Difficulty != "expert" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestMusicCoverHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.MusicCoverHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/曲绘",
		ArgText:    "テオ",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-cover" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}
	if resolved.Query != "テオ" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}
}

func TestMusicProgressHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.MusicProgressHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/pjsk progress",
		ArgText:    "ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-progress" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}
	if resolved.Query != "" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}

	var params struct {
		Difficulty string `json:"difficulty"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Difficulty != "expert" {
		t.Fatalf("unexpected params: %+v", params)
	}
}
