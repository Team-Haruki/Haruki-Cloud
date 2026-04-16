package sekai

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
)

func TestScoreControlHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.ScoreControlHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/wl控分",
		ArgText:    "100 Song A",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Module != parser.ModuleScore || resolved.Mode != "score-control" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params scoreControlParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.TargetPoint != 100 || params.Query != "Song A" || !params.WL {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestMusicMetaHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.MusicMetaHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/歌曲meta",
		ArgText:    "Tell Your World / 初音未来的消失",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Module != parser.ModuleScore || resolved.Mode != "score-music-meta" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params musicMetaQueriesParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if len(params.Queries) != 2 || params.Queries[0] != "Tell Your World" || params.Queries[1] != "初音未来的消失" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestMusicMetaHandleSplitsQueriesByNewline(t *testing.T) {
	h := sekaiHandlers{}.MusicMetaHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/歌曲meta",
		ArgText:    "Tell Your World\n初音未来的消失",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}

	var params musicMetaQueriesParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if len(params.Queries) != 2 || params.Queries[0] != "Tell Your World" || params.Queries[1] != "初音未来的消失" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestCustomRoomScoreControlHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.CustomRoomScoreControlHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/自定义房间控分",
		ArgText:    "22",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Module != parser.ModuleScore || resolved.Mode != "score-custom-room" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params customRoomScoreParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.TargetPoint != 22 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestMusicBoardHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.MusicBoardHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/歌曲排行",
		ArgText:    "多人 时速 升序 2页 ex >30 Song A / Song B",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Module != parser.ModuleScore || resolved.Mode != "score-music-board" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params rendermusic.BoardQuery
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.LiveType != "multi" || params.Target != "pt/time" || !params.Ascend || params.Page != 2 {
		t.Fatalf("unexpected board params: %+v", params)
	}
	if params.LevelFilter != ">30" {
		t.Fatalf("unexpected level filter: %+v", params)
	}
	if len(params.DiffFilter) != 1 || params.DiffFilter[0] != "expert" {
		t.Fatalf("unexpected diff filter: %+v", params)
	}
	if len(params.SpecQueries) != 2 || params.SpecQueries[0] != "Song A" || params.SpecQueries[1] != "Song B" {
		t.Fatalf("unexpected spec queries: %+v", params)
	}
}

func TestMusicBoardHandleSplitsSpecQueriesByWhitespaceLikeRefer(t *testing.T) {
	h := sekaiHandlers{}.MusicBoardHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/歌曲排行",
		ArgText:    "虾ex 虾ma 龙hd",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}

	var params rendermusic.BoardQuery
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if len(params.SpecQueries) != 3 || params.SpecQueries[0] != "虾ex" || params.SpecQueries[1] != "虾ma" || params.SpecQueries[2] != "龙hd" {
		t.Fatalf("unexpected spec queries: %+v", params)
	}
}

func TestMusicBoardHandleAllowsModeOnlyQueryWithoutSpecSongs(t *testing.T) {
	h := sekaiHandlers{}.MusicBoardHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/歌曲比较",
		ArgText:    "多人 火效率",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}

	var params rendermusic.BoardQuery
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.LiveType != "multi" || params.Target != "pt" {
		t.Fatalf("unexpected board mode: %+v", params)
	}
	if len(params.SpecQueries) != 0 {
		t.Fatalf("expected no spec queries, got %+v", params.SpecQueries)
	}
}

func TestMusicBoardHandleParsesSkillsAndKeepsSpecQueryDifficulty(t *testing.T) {
	h := sekaiHandlers{}.MusicBoardHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/歌曲排行",
		ArgText:    "solo max 技能 120 110 100 90 80 SongAex / music2ma",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}

	var params rendermusic.BoardQuery
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.LiveType != "solo" || params.SkillStrategy != "max" {
		t.Fatalf("unexpected board mode: %+v", params)
	}
	if len(params.Skills) != 5 {
		t.Fatalf("unexpected skills: %+v", params.Skills)
	}
	expectedSkills := []float64{1.2, 1.1, 1.0, 0.9, 0.8}
	for idx, expected := range expectedSkills {
		if params.Skills[idx] != expected {
			t.Fatalf("unexpected skills: %+v", params.Skills)
		}
	}
	if len(params.SpecQueries) != 2 || params.SpecQueries[0] != "SongAex" || params.SpecQueries[1] != "music2ma" {
		t.Fatalf("unexpected spec queries: %+v", params)
	}
}

func TestMusicBoardHandleKeepsBareNumericSpecQuery(t *testing.T) {
	h := sekaiHandlers{}.MusicBoardHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/歌曲排行",
		ArgText:    "多人 123",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}

	var params rendermusic.BoardQuery
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.LiveType != "multi" {
		t.Fatalf("unexpected live type: %+v", params)
	}
	if len(params.Skills) != 0 {
		t.Fatalf("unexpected skills: %+v", params.Skills)
	}
	if len(params.SpecQueries) != 1 || params.SpecQueries[0] != "123" {
		t.Fatalf("unexpected spec queries: %+v", params)
	}
}

func TestMusicBoardHandleParsesMultiSkillWithKeyword(t *testing.T) {
	h := sekaiHandlers{}.MusicBoardHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/歌曲排行",
		ArgText:    "多人 200实效 Song A",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}

	var params rendermusic.BoardQuery
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if len(params.Skills) != 5 {
		t.Fatalf("unexpected skills: %+v", params.Skills)
	}
	for _, skill := range params.Skills {
		if skill != 2.0 {
			t.Fatalf("unexpected skills: %+v", params.Skills)
		}
	}
	if len(params.SpecQueries) != 1 || params.SpecQueries[0] != "Song A" {
		t.Fatalf("unexpected spec queries: %+v", params)
	}
}

func TestMusicBoardHandleParsesPageWithTrailingP(t *testing.T) {
	h := sekaiHandlers{}.MusicBoardHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/歌曲排行",
		ArgText:    "多人 2p SongA",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}

	var params rendermusic.BoardQuery
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Page != 2 {
		t.Fatalf("unexpected page: %+v", params)
	}
	if len(params.SpecQueries) != 1 || params.SpecQueries[0] != "SongA" {
		t.Fatalf("unexpected spec queries: %+v", params)
	}
}

func TestMusicBoardHandleExtractsLevelAndDiffFiltersFromAnyPosition(t *testing.T) {
	h := sekaiHandlers{}.MusicBoardHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/歌曲排行",
		ArgText:    "多人 SongA >30 ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}

	var params rendermusic.BoardQuery
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.LevelFilter != ">30" {
		t.Fatalf("unexpected level filter: %+v", params)
	}
	if len(params.DiffFilter) != 1 || params.DiffFilter[0] != "expert" {
		t.Fatalf("unexpected diff filter: %+v", params)
	}
	if len(params.SpecQueries) != 1 || params.SpecQueries[0] != "SongA" {
		t.Fatalf("unexpected spec queries: %+v", params)
	}
}
