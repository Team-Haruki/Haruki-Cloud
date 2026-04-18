package handler

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestNoteNumHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.NoteNumHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/物量",
		ArgText:    "777",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-note-count" {
		t.Fatalf("unexpected command request: %+v", resolved)
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

func TestNoteNumHandleBuildsCommandRequestWithDifficulty(t *testing.T) {
	h := sekaiHandlers{}.NoteNumHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查物量",
		ArgText:    "777 ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-note-count" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "777" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}

	var params struct {
		NoteCount  int    `json:"note_count"`
		Difficulty string `json:"difficulty"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.NoteCount != 777 || params.Difficulty != "expert" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestBPMHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.BPMHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查BPM",
		ArgText:    "200 ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-bpm" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "200" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}

	var params struct {
		BPM        float64 `json:"bpm"`
		Difficulty string  `json:"difficulty"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.BPM != 200 {
		t.Fatalf("unexpected bpm params: %+v", params)
	}
	if params.Difficulty != "expert" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestMusicCoverHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.MusicCoverHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/曲绘",
		ArgText:    "テオ",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-cover" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "テオ" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}
}

func TestMusicProgressHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.MusicProgressHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/pjsk progress",
		ArgText:    "ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-progress" {
		t.Fatalf("unexpected command request: %+v", resolved)
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

func TestMusicProgressHandleEmbedsSelfSelector(t *testing.T) {
	h := sekaiHandlers{}.MusicProgressHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/pjsk progress",
		ArgText:    "u2 ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}

	var params struct {
		Mode           string `json:"mode"`
		Platform       string `json:"platform"`
		PlatformUserID string `json:"platform_user_id"`
		Selector       string `json:"selector"`
		Difficulty     string `json:"difficulty"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Mode != "self" || params.Platform != "qq" || params.PlatformUserID != "42" || params.Selector != "u2" || params.Difficulty != "expert" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestMusicListHandleBuildsCommandRequestWithExactLevel(t *testing.T) {
	h := sekaiHandlers{}.MusicListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/难度排行",
		ArgText:    "31",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}

	var params struct {
		Level int `json:"level"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Level != 31 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestMusicListHandleBuildsCommandRequestWithLevelRangeAndDiff(t *testing.T) {
	h := sekaiHandlers{}.MusicListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/难度排行",
		ArgText:    "31-32 ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}

	var params struct {
		Difficulty string `json:"difficulty"`
		LevelMin   int    `json:"level_min"`
		LevelMax   int    `json:"level_max"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Difficulty != "expert" || params.LevelMin != 31 || params.LevelMax != 32 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestMusicListHandleEmbedsSelfSelector(t *testing.T) {
	h := sekaiHandlers{}.MusicListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/难度排行",
		ArgText:    "u2 31-32 ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}

	var params struct {
		Mode           string `json:"mode"`
		Platform       string `json:"platform"`
		PlatformUserID string `json:"platform_user_id"`
		Selector       string `json:"selector"`
		Difficulty     string `json:"difficulty"`
		LevelMin       int    `json:"level_min"`
		LevelMax       int    `json:"level_max"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Mode != "self" || params.Platform != "qq" || params.PlatformUserID != "42" || params.Selector != "u2" {
		t.Fatalf("unexpected self params: %+v", params)
	}
	if params.Difficulty != "expert" || params.LevelMin != 31 || params.LevelMax != 32 {
		t.Fatalf("unexpected music list params: %+v", params)
	}
}

func TestMusicListHandleBuildsCommandRequestWithResultFilter(t *testing.T) {
	h := sekaiHandlers{}.MusicListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/难度排行",
		ArgText:    "未ap 31-32 ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}

	var params struct {
		Difficulty   string `json:"difficulty"`
		LevelMin     int    `json:"level_min"`
		LevelMax     int    `json:"level_max"`
		ResultFilter string `json:"result_filter"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Difficulty != "expert" || params.LevelMin != 31 || params.LevelMax != 32 || params.ResultFilter != "not_ap" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestMusicRewardsHandleEmbedsSelfSelector(t *testing.T) {
	h := sekaiHandlers{}.MusicRewardsHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/打歌奖励",
		ArgText:    "u2",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}

	var params struct {
		Mode           string `json:"mode"`
		Platform       string `json:"platform"`
		PlatformUserID string `json:"platform_user_id"`
		Selector       string `json:"selector"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Mode != "self" || params.Platform != "qq" || params.PlatformUserID != "42" || params.Selector != "u2" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestMusicListHandleBuildsCommandRequestWithClosedIntervalTwoTokensAndDiff(t *testing.T) {
	h := sekaiHandlers{}.MusicListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/难度排行",
		ArgText:    "31 32 ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}

	var params struct {
		Difficulty string `json:"difficulty"`
		LevelMin   int    `json:"level_min"`
		LevelMax   int    `json:"level_max"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Difficulty != "expert" || params.LevelMin != 31 || params.LevelMax != 32 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestMusicListHandleBuildsCommandRequestWithBracketedClosedInterval(t *testing.T) {
	h := sekaiHandlers{}.MusicListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/难度排行",
		ArgText:    "[31,32] ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}

	var params struct {
		Difficulty string `json:"difficulty"`
		LevelMin   int    `json:"level_min"`
		LevelMax   int    `json:"level_max"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Difficulty != "expert" || params.LevelMin != 31 || params.LevelMax != 32 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestMusicListHandleBuildsCommandRequestWithSpacedClosedInterval(t *testing.T) {
	h := sekaiHandlers{}.MusicListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/难度排行",
		ArgText:    "31 到 32 ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}

	var params struct {
		Difficulty string `json:"difficulty"`
		LevelMin   int    `json:"level_min"`
		LevelMax   int    `json:"level_max"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Difficulty != "expert" || params.LevelMin != 31 || params.LevelMax != 32 {
		t.Fatalf("unexpected params: %+v", params)
	}
}
