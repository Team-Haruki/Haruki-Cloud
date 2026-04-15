package sekai

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/card"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

func TestCardDetailAndListHandlersShareDispatchRules(t *testing.T) {
	tests := []struct {
		name       string
		args       string
		wantMode   string
		checkParam func(*testing.T, []byte)
	}{
		{
			name:     "card detail query",
			args:     "1001",
			wantMode: "card-detail",
			checkParam: func(t *testing.T, raw []byte) {
				t.Helper()
				var params card.Query
				if err := json.Unmarshal(raw, &params); err != nil {
					t.Fatalf("unmarshal params: %v", err)
				}
				if params.Query != "1001" || params.Region != "jp" {
					t.Fatalf("unexpected params: %+v", params)
				}
			},
		},
		{
			name:     "card detail latest query",
			args:     "-1",
			wantMode: "card-detail",
			checkParam: func(t *testing.T, raw []byte) {
				t.Helper()
				var params card.Query
				if err := json.Unmarshal(raw, &params); err != nil {
					t.Fatalf("unmarshal params: %v", err)
				}
				if params.Query != "-1" || params.Region != "jp" {
					t.Fatalf("unexpected params: %+v", params)
				}
			},
		},
		{
			name:     "card detail sequence query",
			args:     "mnr-1",
			wantMode: "card-detail",
			checkParam: func(t *testing.T, raw []byte) {
				t.Helper()
				var params card.Query
				if err := json.Unmarshal(raw, &params); err != nil {
					t.Fatalf("unmarshal params: %v", err)
				}
				if params.Query != "mnr-1" || params.Region != "jp" {
					t.Fatalf("unexpected params: %+v", params)
				}
			},
		},
		{
			name:     "card list query",
			args:     "mnr 4星",
			wantMode: "card-list",
			checkParam: func(t *testing.T, raw []byte) {
				t.Helper()
				var params card.ListRequest
				if err := json.Unmarshal(raw, &params); err != nil {
					t.Fatalf("unmarshal params: %v", err)
				}
				if params.Query != "mnr 4星" || params.Region != "jp" {
					t.Fatalf("unexpected params: %+v", params)
				}
			},
		},
		{
			name:     "card box query",
			args:     "mnr 4星 id box before",
			wantMode: "card-box",
			checkParam: func(t *testing.T, raw []byte) {
				t.Helper()
				var params struct {
					ShowID           bool `json:"show_id"`
					ShowBox          bool `json:"show_box"`
					UseAfterTraining bool `json:"use_after_training"`
				}
				if err := json.Unmarshal(raw, &params); err != nil {
					t.Fatalf("unmarshal params: %v", err)
				}
				if !params.ShowID || !params.ShowBox || params.UseAfterTraining {
					t.Fatalf("unexpected params: %+v", params)
				}
			},
		},
	}

	builders := []struct {
		name string
		make func() SekaiCommandHandler
	}{
		{name: "detail", make: func() SekaiCommandHandler { return sekaiHandlers{}.CardDetailHandle() }},
		{name: "list", make: func() SekaiCommandHandler { return sekaiHandlers{}.CardListHandle() }},
	}

	for _, builder := range builders {
		for _, tt := range tests {
			t.Run(builder.name+"_"+tt.name, func(t *testing.T) {
				h := builder.make()
				h.Regions = []renderregion.Value{renderregion.JP}

				result, err := h.Handle(&handler.HandlerContext{
					Context:    context.Background(),
					TriggerCmd: "/查卡",
					ArgText:    tt.args,
				})
				if err != nil {
					t.Fatalf("Handle() error = %v", err)
				}

				resolved, ok := result.(*parser.ResolvedCommand)
				if !ok {
					t.Fatalf("handler returned %T", result)
				}
				expectedMode := tt.wantMode
				if builder.name == "list" && tt.wantMode != "card-box" {
					expectedMode = "card-list"
				}
				if resolved.Module != parser.ModuleCard || resolved.Mode != expectedMode {
					t.Fatalf("unexpected resolved command: %+v", resolved)
				}
				tt.checkParam(t, resolved.Params)
				if builder.name == "list" && resolved.Mode == "card-list" {
					var params card.ListRequest
					if err := json.Unmarshal(resolved.Params, &params); err != nil {
						t.Fatalf("unmarshal strict list params: %v", err)
					}
					if !params.StrictFilterOnly {
						t.Fatalf("expected strict filter mode for card-list handler, got %+v", params)
					}
				}

				if tt.wantMode == "card-box" && resolved.Query != "mnr 4星" {
					t.Fatalf("unexpected cleaned query: %q", resolved.Query)
				}
			})
		}
	}
}

func TestCardListHandlePrefers25UnitAliasOverCardID(t *testing.T) {
	h := sekaiHandlers{}.CardListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡牌列表",
		ArgText:    "25",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleCard || resolved.Mode != "card-list" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params card.ListRequest
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Query != "25" || params.Region != "jp" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestCardListHandlePrefersBare4RarityOverSingleCardID(t *testing.T) {
	h := sekaiHandlers{}.CardListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡牌列表",
		ArgText:    "4",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleCard || resolved.Mode != "card-list" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params card.ListRequest
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Query != "4" || params.Region != "jp" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestCardListHandleSupportsLunabotCharacterAlias(t *testing.T) {
	h := sekaiHandlers{}.CardListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡牌列表",
		ArgText:    "tks 4",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleCard || resolved.Mode != "card-list" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params card.ListRequest
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Query != "tks 4" || params.Region != "jp" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestCardBoxHandleTreats25AsStrictFilterQuery(t *testing.T) {
	h := sekaiHandlers{}.CardBoxHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡牌一览",
		ArgText:    "25",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleCard || resolved.Mode != "card-box" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}
	if resolved.Query != "25" {
		t.Fatalf("unexpected box query: %q", resolved.Query)
	}

	var params struct {
		StrictFilterOnly bool `json:"strict_filter_only"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.StrictFilterOnly {
		t.Fatalf("expected strict filter mode for /卡牌一览 25, got %+v", params)
	}
}
