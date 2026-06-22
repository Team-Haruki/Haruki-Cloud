package handler

import (
	"context"
	json "github.com/bytedance/sonic"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/education"
)

func TestAreaItemHandleBuildsCommandRequest(t *testing.T) {
	tests := []struct {
		name      string
		args      string
		wantErr   bool
		checkFunc func(*testing.T, education.AreaItemQuery)
	}{
		{
			name:    "no filter now requires explicit target",
			args:    "",
			wantErr: true,
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				if query.ShowFull || query.Unit != "" || query.Cid != 0 || query.CharacterQuery != "" || query.Attr != "" || query.Tree || query.Flower {
					t.Fatalf("unexpected query: %+v", query)
				}
			},
		},
		{
			name:    "full without filter still requires explicit target",
			args:    "full",
			wantErr: true,
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				if query.ShowFull || query.Unit != "" || query.Cid != 0 || query.CharacterQuery != "" || query.Attr != "" || query.Tree || query.Flower {
					t.Fatalf("unexpected query: %+v", query)
				}
			},
		},
		{
			name: "plant alias selects tree and flower",
			args: "花树",
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				if query.ShowFull || query.Unit != "" || query.Cid != 0 || query.CharacterQuery != "" || query.Attr != "" || !query.Tree || !query.Flower {
					t.Fatalf("unexpected query: %+v", query)
				}
			},
		},
		{
			name: "full can be combined with filter",
			args: "25h full",
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				if !query.ShowFull || query.Unit != "school_refusal" || query.Cid != 0 || query.CharacterQuery != "" || query.Attr != "" || query.Tree || query.Flower {
					t.Fatalf("unexpected query: %+v", query)
				}
			},
		},
		{
			name: "all filters are parsed and passed through",
			args: "25h miku 可爱 树 花",
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				if query.Unit != "school_refusal" || query.Cid != 0 || query.CharacterQuery != "miku" || query.Attr != "cute" || !query.Tree || !query.Flower {
					t.Fatalf("unexpected query: %+v", query)
				}
			},
		},
		{
			name: "piapro alias is normalized",
			args: "vs",
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				if query.Unit != "piapro" {
					t.Fatalf("unexpected query: %+v", query)
				}
			},
		},
		{
			name: "unknown nickname is preserved as character query",
			args: "初音未来",
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				if query.Cid != 0 || query.CharacterQuery != "初音未来" {
					t.Fatalf("unexpected query: %+v", query)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := sekaiHandlers{}.AreaItemHandle()
			result, err := h.Handle(&PjskHandlerContext{
				Context:    context.Background(),
				TriggerCmd: "/区域道具",
				ArgText:    tt.args,
			})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}

			resolved := result
			if resolved == nil {
				t.Fatal("expected command request, got nil")
			}
			if resolved.Module != parser.ModuleEducation || resolved.Mode != "education-area" {
				t.Fatalf("unexpected command request: %+v", resolved)
			}

			var query education.AreaItemQuery
			if err := json.Unmarshal(resolved.Params, &query); err != nil {
				t.Fatalf("unmarshal params: %v", err)
			}
			tt.checkFunc(t, query)
		})
	}
}

func TestBondsHandleBuildsCommandRequest(t *testing.T) {
	tests := []struct {
		name      string
		args      string
		checkFunc func(*testing.T, education.BondsQuery)
	}{
		{
			name: "no filter keeps default behavior",
			args: "",
			checkFunc: func(t *testing.T, query education.BondsQuery) {
				t.Helper()
				if query.Cid != 0 || query.CharacterQuery != "" {
					t.Fatalf("unexpected query: %+v", query)
				}
			},
		},
		{
			name: "character query is passed through",
			args: "初音未来",
			checkFunc: func(t *testing.T, query education.BondsQuery) {
				t.Helper()
				if query.Cid != 0 || query.CharacterQuery != "初音未来" {
					t.Fatalf("unexpected query: %+v", query)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := sekaiHandlers{}.BondsHandle()
			result, err := h.Handle(&PjskHandlerContext{
				Context:    context.Background(),
				TriggerCmd: "/羁绊",
				ArgText:    tt.args,
			})
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}

			resolved := result
			if resolved == nil {
				t.Fatal("expected command request, got nil")
			}
			if resolved.Module != parser.ModuleEducation || resolved.Mode != "education-bonds" {
				t.Fatalf("unexpected command request: %+v", resolved)
			}

			var query education.BondsQuery
			if err := json.Unmarshal(resolved.Params, &query); err != nil {
				t.Fatalf("unmarshal params: %v", err)
			}
			tt.checkFunc(t, query)
		})
	}
}

func TestEducationSelfHandlersEmbedSelector(t *testing.T) {
	tests := []struct {
		name string
		mode string
		run  func() HarukiSekaiCommandHandler
		cmd  string
	}{
		{name: "challenge", mode: "education-challenge", run: sekaiHandlers{}.ChallengeInfoHandle, cmd: "/挑战信息"},
		{name: "power", mode: "education-power", run: sekaiHandlers{}.PowerBonusInfoHandle, cmd: "/加成信息"},
		{name: "leader", mode: "education-leader", run: sekaiHandlers{}.LeaderCountHandle, cmd: "/队长统计"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.run()
			result, err := h.Handle(&PjskHandlerContext{
				Context:    context.Background(),
				Platform:   "qq",
				UserId:     "42",
				TriggerCmd: tt.cmd,
				ArgText:    "u2",
			})
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}

			resolved := result
			if resolved == nil {
				t.Fatal("expected command request, got nil")
			}
			if resolved.Module != parser.ModuleEducation || resolved.Mode != tt.mode {
				t.Fatalf("unexpected command request: %+v", resolved)
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
		})
	}
}

func TestAreaItemHandleEmbedsSelfSelector(t *testing.T) {
	h := sekaiHandlers{}.AreaItemHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/区域道具",
		ArgText:    "u2 25h miku 可爱 树 花",
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
		Unit           string `json:"unit"`
		CharacterQuery string `json:"character_query"`
		Attr           string `json:"attr"`
		Tree           bool   `json:"tree"`
		Flower         bool   `json:"flower"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Mode != "self" || params.Platform != "qq" || params.PlatformUserID != "42" || params.Selector != "u2" {
		t.Fatalf("unexpected self params: %+v", params)
	}
	if params.Unit != "school_refusal" || params.CharacterQuery != "miku" || params.Attr != "cute" || !params.Tree || !params.Flower {
		t.Fatalf("unexpected area item params: %+v", params)
	}
}

func TestBondsHandleEmbedsSelfSelector(t *testing.T) {
	h := sekaiHandlers{}.BondsHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/羁绊",
		ArgText:    "u2 初音未来",
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
		CharacterQuery string `json:"character_query"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Mode != "self" || params.Platform != "qq" || params.PlatformUserID != "42" || params.Selector != "u2" || params.CharacterQuery != "初音未来" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestCharacterMissionHandleBuildsOverviewRequest(t *testing.T) {
	h := sekaiHandlers{}.CharacterMissionHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/cr任务",
		ArgText:    "u2 miku",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEducation || resolved.Mode != "education-character-mission" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		Mode           string `json:"mode"`
		Platform       string `json:"platform"`
		PlatformUserID string `json:"platform_user_id"`
		Selector       string `json:"selector"`
		CharacterQuery string `json:"character_query"`
		ShowAll        bool   `json:"show_all"`
		MissionType    string `json:"mission_type"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Mode != "self" || params.Platform != "qq" || params.PlatformUserID != "42" || params.Selector != "u2" {
		t.Fatalf("unexpected self params: %+v", params)
	}
	if params.CharacterQuery != "miku" || params.ShowAll || params.MissionType != "" {
		t.Fatalf("unexpected character mission params: %+v", params)
	}
}

func TestCharacterMissionHandleBuildsAllRequest(t *testing.T) {
	h := sekaiHandlers{}.CharacterMissionHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/cr任务",
		ArgText:    "u2 miku all 队长次数",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEducation || resolved.Mode != "education-character-mission" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		Mode           string `json:"mode"`
		Platform       string `json:"platform"`
		PlatformUserID string `json:"platform_user_id"`
		Selector       string `json:"selector"`
		CharacterQuery string `json:"character_query"`
		ShowAll        bool   `json:"show_all"`
		MissionType    string `json:"mission_type"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.CharacterQuery != "miku" || !params.ShowAll || params.MissionType != "play_live" {
		t.Fatalf("unexpected character mission all params: %+v", params)
	}
}

func TestCharacterMissionHandleParsesFlowerTreeAlias(t *testing.T) {
	h := sekaiHandlers{}.CharacterMissionHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/cr任务",
		ArgText:    "u2 miku all 花树",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEducation || resolved.Mode != "education-character-mission" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		CharacterQuery string `json:"character_query"`
		ShowAll        bool   `json:"show_all"`
		MissionType    string `json:"mission_type"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.CharacterQuery != "miku" || !params.ShowAll || params.MissionType != "area_item_level_up_reality_world" {
		t.Fatalf("unexpected character mission params: %+v", params)
	}
}
