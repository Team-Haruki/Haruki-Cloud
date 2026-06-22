package handler

import (
	"context"
	json "github.com/bytedance/sonic"
	"strings"
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
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
		ArgText:    "Help me, ERINNNNNN!! ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-bpm-detail" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "Help me, ERINNNNNN!!" {
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

func TestBPMSearchHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.BPMSearchHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/bpms",
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
	if params.BPM != 200 || params.Difficulty != "expert" {
		t.Fatalf("unexpected bpm search params: %+v", params)
	}
}

func TestBPMHandleReturnsUpdatedHelp(t *testing.T) {
	h := sekaiHandlers{}.BPMHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	if h.GetHelper() != bpmDetailHelp {
		t.Fatalf("unexpected helper: %q", h.GetHelper())
	}

	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查BPM",
	})
	if err == nil || !strings.Contains(err.Error(), "请输入要查询 BPM 的歌曲名") {
		t.Fatalf("expected updated BPM help, got %v", err)
	}
}

func TestFormatMusicBPMSequenceDoesNotTruncate(t *testing.T) {
	events := []rendermusic.BPMEvent{
		{BPM: 264},
		{BPM: 200},
		{BPM: 190},
		{BPM: 180},
		{BPM: 175},
		{BPM: 174},
		{BPM: 232},
		{BPM: 222},
		{BPM: 212},
		{BPM: 202},
		{BPM: 182},
		{BPM: 273},
		{BPM: 260},
		{BPM: 250},
	}

	got := formatMusicBPMSequence(events)
	want := "264 / 200 / 190 / 180 / 175 / 174 / 232 / 222 / 212 / 202 / 182 / 273 / 260 / 250"
	if got != want {
		t.Fatalf("formatMusicBPMSequence() = %q, want %q", got, want)
	}
	if strings.Contains(got, "...") {
		t.Fatalf("formatMusicBPMSequence() should not truncate, got %q", got)
	}
}

func TestB30HandleReturnsUnavailableMessage(t *testing.T) {
	h := sekaiHandlers{}.B30Handle()
	h.Regions = []renderregion.Value{renderregion.JP}
	for _, command := range []string{"/b30", "/pjskb30", "/b39", "/pjskb39"} {
		if !containsString(h.GetCommands(), command) {
			t.Fatalf("expected B30 commands to include %s, got %v", command, h.GetCommands())
		}
	}

	resolved, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/b30",
		ArgText:    "anything",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "rating-unavailable" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	message, err := executeRatingUnavailable(nil)
	if err != nil {
		t.Fatalf("executeRatingUnavailable() error = %v", err)
	}
	if len(message) != 1 || message[0].Type != onebot11.TypeText {
		t.Fatalf("unexpected message: %+v", message)
	}
	data, ok := message[0].Data.(onebot11.TextData)
	if !ok || data.Text != ratingUnavailableMessage {
		t.Fatalf("unexpected text data: %+v", message[0].Data)
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
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

func TestMusicListHandleBuildsCommandRequestWithFullFlag(t *testing.T) {
	h := sekaiHandlers{}.MusicListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/难度排行",
		ArgText:    "full 31 ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Query != "" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}

	var params struct {
		Full       bool   `json:"full"`
		Difficulty string `json:"difficulty"`
		Level      int    `json:"level"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.Full || params.Difficulty != "expert" || params.Level != 31 {
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
