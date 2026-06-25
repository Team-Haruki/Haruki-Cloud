package handler

import (
	"context"
	json "github.com/bytedance/sonic"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/card"
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
			args:     "mnr 4星 id box before 未持有",
			wantMode: "card-box",
			checkParam: func(t *testing.T, raw []byte) {
				t.Helper()
				var params struct {
					ShowID           bool `json:"show_id"`
					ShowBox          bool `json:"show_box"`
					UnownedOnly      bool `json:"unowned_only"`
					UseAfterTraining bool `json:"use_after_training"`
				}
				if err := json.Unmarshal(raw, &params); err != nil {
					t.Fatalf("unmarshal params: %v", err)
				}
				if !params.ShowID || !params.ShowBox || !params.UnownedOnly || params.UseAfterTraining {
					t.Fatalf("unexpected params: %+v", params)
				}
			},
		},
	}

	builders := []struct {
		name string
		make func() HarukiSekaiCommandHandler
	}{
		{name: "detail", make: func() HarukiSekaiCommandHandler { return sekaiHandlers{}.CardDetailHandle() }},
		{name: "list", make: func() HarukiSekaiCommandHandler { return sekaiHandlers{}.CardListHandle() }},
	}

	for _, builder := range builders {
		for _, tt := range tests {
			t.Run(builder.name+"_"+tt.name, func(t *testing.T) {
				h := builder.make()
				h.Regions = []renderregion.Value{renderregion.JP}

				result, err := h.Handle(&PjskHandlerContext{
					Context:    context.Background(),
					TriggerCmd: "/查卡",
					ArgText:    tt.args,
				})
				if err != nil {
					t.Fatalf("Handle() error = %v", err)
				}

				resolved := result
				if resolved == nil {
					t.Fatal("expected command request, got nil")
				}
				expectedMode := tt.wantMode
				if builder.name == "list" && tt.wantMode != "card-box" {
					expectedMode = "card-list"
				}
				if resolved.Module != parser.ModuleCard || resolved.Mode != expectedMode {
					t.Fatalf("unexpected command request: %+v", resolved)
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

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡牌列表",
		ArgText:    "25",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleCard || resolved.Mode != "card-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
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

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡牌列表",
		ArgText:    "4",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleCard || resolved.Mode != "card-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
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

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡牌列表",
		ArgText:    "tks 4",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleCard || resolved.Mode != "card-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
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

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡牌一览",
		ArgText:    "25",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleCard || resolved.Mode != "card-box" {
		t.Fatalf("unexpected command request: %+v", resolved)
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

func TestCardBoxHandleParsesAttributeGrouping(t *testing.T) {
	h := sekaiHandlers{}.CardBoxHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡牌一览",
		ArgText:    "mnr 4 属性 未持有",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleCard || resolved.Mode != "card-box" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "mnr 4" {
		t.Fatalf("unexpected cleaned query: %q", resolved.Query)
	}

	var params struct {
		GroupBy          string `json:"group_by"`
		UnownedOnly      bool   `json:"unowned_only"`
		StrictFilterOnly bool   `json:"strict_filter_only"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.GroupBy != card.CardBoxGroupByAttr || !params.UnownedOnly || !params.StrictFilterOnly {
		t.Fatalf("unexpected card box grouping params: %+v", params)
	}
}

func TestCardListHandleEmbedsSelfSelector(t *testing.T) {
	h := sekaiHandlers{}.CardListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/卡牌列表",
		ArgText:    "u2 mnr 4星",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleCard || resolved.Mode != "card-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "mnr 4星" {
		t.Fatalf("unexpected cleaned query: %q", resolved.Query)
	}

	var params struct {
		Mode             string `json:"mode"`
		Platform         string `json:"platform"`
		PlatformUserID   string `json:"platform_user_id"`
		Selector         string `json:"selector"`
		Query            string `json:"query"`
		Region           string `json:"region"`
		StrictFilterOnly bool   `json:"strict_filter_only"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Mode != "self" || params.Platform != "qq" || params.PlatformUserID != "42" || params.Selector != "u2" {
		t.Fatalf("unexpected self params: %+v", params)
	}
	if params.Query != "mnr 4星" || params.Region != "jp" || !params.StrictFilterOnly {
		t.Fatalf("unexpected card list params: %+v", params)
	}
}

func TestCardBoxHandleEmbedsSelfSelector(t *testing.T) {
	h := sekaiHandlers{}.CardBoxHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/卡牌一览",
		ArgText:    "u2 mnr 4星 box id before",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleCard || resolved.Mode != "card-box" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "mnr 4星" {
		t.Fatalf("unexpected cleaned query: %q", resolved.Query)
	}

	var params struct {
		Mode             string `json:"mode"`
		Platform         string `json:"platform"`
		PlatformUserID   string `json:"platform_user_id"`
		Selector         string `json:"selector"`
		ShowID           bool   `json:"show_id"`
		ShowBox          bool   `json:"show_box"`
		UseAfterTraining bool   `json:"use_after_training"`
		StrictFilterOnly bool   `json:"strict_filter_only"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Mode != "self" || params.Platform != "qq" || params.PlatformUserID != "42" || params.Selector != "u2" {
		t.Fatalf("unexpected self params: %+v", params)
	}
	if !params.ShowID || !params.ShowBox || params.UseAfterTraining || !params.StrictFilterOnly {
		t.Fatalf("unexpected card box params: %+v", params)
	}
}
