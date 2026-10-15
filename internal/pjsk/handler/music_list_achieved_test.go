package handler

import (
	"context"
	"haruki-cloud/utils/usererror"
	"testing"

	"haruki-cloud/internal/jsonutil"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestMusicListAchievedResultFilters(t *testing.T) {
	for _, tc := range []struct{ token, filter string }{
		{"ap", "ap"}, {"AP", "ap"}, {"已ap", "ap"}, {"已AP", "ap"}, {"已allperfect", "ap"}, {"已全perfect", "ap"}, {"已全p", "ap"},
		{"fc", "fc"}, {"FC", "fc"}, {"已fc", "fc"}, {"已FC", "fc"}, {"已fullcombo", "fc"},
		{"clear", "clear"}, {"CLEAR", "clear"}, {"已clear", "clear"}, {"已CLEAR", "clear"}, {"已完成", "clear"}, {"已通关", "clear"},
	} {
		t.Run(tc.token, func(t *testing.T) {
			filter, rest, ok := extractMusicListResultFilter("31-32 " + tc.token + " ex")
			if !ok || filter != tc.filter || rest != "31-32 ex" {
				t.Fatalf("extract = %q, %q, %v", filter, rest, ok)
			}
			h := sekaiHandlers{}.MusicListHandle()
			h.Regions = []renderregion.Value{renderregion.JP}
			request, err := h.Handle(&PjskHandlerContext{Context: context.Background(), TriggerCmd: "/难度排行", ArgText: "31-32 " + tc.token + " ex"})
			if err != nil {
				t.Fatal(err)
			}
			var params struct {
				Difficulty   string `json:"difficulty"`
				LevelMin     int    `json:"level_min"`
				LevelMax     int    `json:"level_max"`
				ResultFilter string `json:"result_filter"`
			}
			if err := jsonutil.Unmarshal(request.Params, &params); err != nil {
				t.Fatal(err)
			}
			if params.Difficulty != "expert" || params.LevelMin != 31 || params.LevelMax != 32 || params.ResultFilter != tc.filter {
				t.Fatalf("unexpected params: %+v", params)
			}
		})
	}
}

func TestMusicListBareResultFilterDoesNotBecomeSongSearch(t *testing.T) {
	for _, tc := range []struct{ args, filter, difficulty, keyword string }{
		{"28 fc", "fc", "", ""},
		{"FC 28", "fc", "", ""},
		{"28 ap", "ap", "", ""},
		{"28 clear", "clear", "", ""},
		{"28 apd fc", "fc", "append", ""},
	} {
		t.Run(tc.args, func(t *testing.T) {
			h := sekaiHandlers{}.MusicListHandle()
			h.Regions = []renderregion.Value{renderregion.JP}
			request, err := h.Handle(&PjskHandlerContext{Context: context.Background(), TriggerCmd: "/歌曲列表", ArgText: tc.args})
			if err != nil {
				t.Fatal(err)
			}
			var params struct {
				Level        int    `json:"level"`
				ResultFilter string `json:"result_filter"`
				Difficulty   string `json:"difficulty"`
			}
			if err := jsonutil.Unmarshal(request.Params, &params); err != nil {
				t.Fatal(err)
			}
			if params.Level != 28 || params.ResultFilter != tc.filter || params.Difficulty != tc.difficulty || request.Query != tc.keyword {
				t.Fatalf("params=%+v query=%q", params, request.Query)
			}
		})
	}
}

func TestMusicListRejectsNonFilterArguments(t *testing.T) {
	for _, trigger := range []string{"/难度排行", "/歌曲列表"} {
		for _, args := range []string{"28 fcc", "28 fc music123", "blue song", "music123", "28 fc ap", "28 expert master"} {
			t.Run(trigger+" "+args, func(t *testing.T) {
				h := sekaiHandlers{}.MusicListHandle()
				h.Regions = []renderregion.Value{renderregion.JP}
				request, err := h.Handle(&PjskHandlerContext{Context: t.Context(), TriggerCmd: trigger, ArgText: args})
				if usererror.CodeOf(err) != usererror.CodeUsage || request != nil {
					t.Fatalf("expected parameter error before song search, request=%+v err=%v", request, err)
				}
			})
		}
	}
}

func TestMusicListFilterOnlyKeepsAccountSelectorAndFull(t *testing.T) {
	h := sekaiHandlers{}.MusicListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	request, err := h.Handle(&PjskHandlerContext{Context: t.Context(), TriggerCmd: "/难度排行", ArgText: "28 fc u2 full"})
	if err != nil {
		t.Fatal(err)
	}
	var params struct {
		Level        int    `json:"level"`
		ResultFilter string `json:"result_filter"`
		Selector     string `json:"selector"`
		Full         bool   `json:"full"`
	}
	if err := jsonutil.Unmarshal(request.Params, &params); err != nil {
		t.Fatal(err)
	}
	if request.Query != "" || params.Level != 28 || params.ResultFilter != "fc" || params.Selector != "u2" || !params.Full {
		t.Fatalf("unexpected params=%+v query=%q", params, request.Query)
	}
}
