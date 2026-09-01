package handler

import (
	"context"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/testutil"
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
				{
					err := json.Unmarshal(raw, &params)
					testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
				}
				{

					testutil.Require(t, !(params.Query != "1001"), "unexpected params: %+v", params)
					testutil.Require(t, !(params.Region != "jp"), "unexpected params: %+v", params)
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
				{
					err := json.Unmarshal(raw, &params)
					testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
				}
				{

					testutil.Require(t, !(params.Query != "-1"), "unexpected params: %+v", params)
					testutil.Require(t, !(params.Region != "jp"), "unexpected params: %+v", params)
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
				{
					err := json.Unmarshal(raw, &params)
					testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
				}
				{

					testutil.Require(t, !(params.Query != "mnr-1"), "unexpected params: %+v", params)
					testutil.Require(t, !(params.Region != "jp"), "unexpected params: %+v", params)
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
				{
					err := json.Unmarshal(raw, &params)
					testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
				}
				{

					testutil.Require(t, !(params.Query != "mnr 4星"), "unexpected params: %+v", params)
					testutil.Require(t, !(params.Region != "jp"), "unexpected params: %+v", params)
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
				{
					err := json.Unmarshal(raw, &params)
					testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
				}
				{

					testutil.Require(t, params.ShowID, "unexpected params: %+v", params)
					testutil.Require(t, params.ShowBox, "unexpected params: %+v", params)
					testutil.Require(t, params.UnownedOnly, "unexpected params: %+v", params)
					testutil.Require(t, !(params.UseAfterTraining), "unexpected params: %+v", params)
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
				testutil.Require(t, !(err != nil), "Handle() error = %v", err)

				resolved := result
				testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")

				expectedMode := tt.wantMode
				if builder.name == "list" && tt.wantMode != "card-box" {
					expectedMode = "card-list"
				}
				{
					testutil.Require(t, !(resolved.Module != parser.ModuleCard), "unexpected command request: %+v", resolved)
					testutil.Require(t, !(resolved.Mode != expectedMode), "unexpected command request: %+v", resolved)
				}

				tt.checkParam(t, resolved.Params)
				if builder.name == "list" && resolved.Mode == "card-list" {
					var params card.ListRequest
					{
						err := json.Unmarshal(resolved.Params, &params)
						testutil.Require(t, !(err != nil), "unmarshal strict list params: %v", err)
					}

					testutil.Require(t, params.StrictFilterOnly, "expected strict filter mode for card-list handler, got %+v", params)

				}
				testutil.Require(t, !(tt.wantMode == "card-box" && resolved.Query != "mnr 4星"), "unexpected cleaned query: %q", resolved.Query)

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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleCard), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "card-list"), "unexpected command request: %+v", resolved)
	}

	var params card.ListRequest
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.Query != "25"), "unexpected params: %+v", params)
		testutil.Require(t, !(params.Region != "jp"), "unexpected params: %+v", params)
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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleCard), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "card-list"), "unexpected command request: %+v", resolved)
	}

	var params card.ListRequest
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.Query != "4"), "unexpected params: %+v", params)
		testutil.Require(t, !(params.Region != "jp"), "unexpected params: %+v", params)
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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleCard), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "card-list"), "unexpected command request: %+v", resolved)
	}

	var params card.ListRequest
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.Query != "tks 4"), "unexpected params: %+v", params)
		testutil.Require(t, !(params.Region != "jp"), "unexpected params: %+v", params)
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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleCard), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "card-box"), "unexpected command request: %+v", resolved)
	}
	testutil.Require(t, !(resolved.Query != "25"), "unexpected box query: %q", resolved.Query)

	var params struct {
		StrictFilterOnly bool `json:"strict_filter_only"`
	}
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, params.StrictFilterOnly, "expected strict filter mode for /卡牌一览 25, got %+v", params)

}

func TestCardBoxHandleParsesAttributeGrouping(t *testing.T) {
	h := sekaiHandlers{}.CardBoxHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡牌一览",
		ArgText:    "mnr 4 属性 未持有",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleCard), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "card-box"), "unexpected command request: %+v", resolved)
	}
	testutil.Require(t, !(resolved.Query != "mnr 4"), "unexpected cleaned query: %q", resolved.Query)

	var params struct {
		GroupBy          string `json:"group_by"`
		UnownedOnly      bool   `json:"unowned_only"`
		StrictFilterOnly bool   `json:"strict_filter_only"`
	}
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.GroupBy != card.CardBoxGroupByAttr), "unexpected card box grouping params: %+v", params)
		testutil.Require(t, params.UnownedOnly, "unexpected card box grouping params: %+v", params)
		testutil.Require(t, params.StrictFilterOnly, "unexpected card box grouping params: %+v", params)
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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleCard), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "card-list"), "unexpected command request: %+v", resolved)
	}
	testutil.Require(t, !(resolved.Query != "mnr 4星"), "unexpected cleaned query: %q", resolved.Query)

	var params struct {
		Mode             string `json:"mode"`
		Platform         string `json:"platform"`
		PlatformUserID   string `json:"platform_user_id"`
		Selector         string `json:"selector"`
		Query            string `json:"query"`
		Region           string `json:"region"`
		StrictFilterOnly bool   `json:"strict_filter_only"`
	}
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.Mode != "self"), "unexpected self params: %+v", params)
		testutil.Require(t, !(params.Platform != "qq"), "unexpected self params: %+v", params)
		testutil.Require(t, !(params.PlatformUserID != "42"), "unexpected self params: %+v", params)
		testutil.Require(t, !(params.Selector != "u2"), "unexpected self params: %+v", params)
	}
	{
		testutil.Require(t, !(params.Query != "mnr 4星"), "unexpected card list params: %+v", params)
		testutil.Require(t, !(params.Region != "jp"), "unexpected card list params: %+v", params)
		testutil.Require(t, params.StrictFilterOnly, "unexpected card list params: %+v", params)
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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleCard), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "card-box"), "unexpected command request: %+v", resolved)
	}
	testutil.Require(t, !(resolved.Query != "mnr 4星"), "unexpected cleaned query: %q", resolved.Query)

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
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.Mode != "self"), "unexpected self params: %+v", params)
		testutil.Require(t, !(params.Platform != "qq"), "unexpected self params: %+v", params)
		testutil.Require(t, !(params.PlatformUserID != "42"), "unexpected self params: %+v", params)
		testutil.Require(t, !(params.Selector != "u2"), "unexpected self params: %+v", params)
	}
	{
		testutil.Require(t, params.ShowID, "unexpected card box params: %+v", params)
		testutil.Require(t, params.ShowBox, "unexpected card box params: %+v", params)
		testutil.Require(t, !(params.UseAfterTraining), "unexpected card box params: %+v", params)
		testutil.Require(t, params.StrictFilterOnly, "unexpected card box params: %+v", params)
	}

}
