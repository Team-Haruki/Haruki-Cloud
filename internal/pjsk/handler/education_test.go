package handler

import (
	"context"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/testutil"
	"strings"
	"testing"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
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
				{
					testutil.Require(t, !(query.ShowFull), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Unit != ""), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Cid != 0), "unexpected query: %+v", query)
					testutil.Require(t, !(query.CharacterQuery != ""), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Attr != ""), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Tree), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Flower), "unexpected query: %+v", query)
				}

			},
		},
		{
			name:    "full without filter still requires explicit target",
			args:    "full",
			wantErr: true,
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				{
					testutil.Require(t, !(query.ShowFull), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Unit != ""), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Cid != 0), "unexpected query: %+v", query)
					testutil.Require(t, !(query.CharacterQuery != ""), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Attr != ""), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Tree), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Flower), "unexpected query: %+v", query)
				}

			},
		},
		{
			name: "plant alias selects tree and flower",
			args: "花树",
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				{
					testutil.Require(t, !(query.ShowFull), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Unit != ""), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Cid != 0), "unexpected query: %+v", query)
					testutil.Require(t, !(query.CharacterQuery != ""), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Attr != ""), "unexpected query: %+v", query)
					testutil.Require(t, query.Tree, "unexpected query: %+v", query)
					testutil.Require(t, query.Flower, "unexpected query: %+v", query)
				}

			},
		},
		{
			name: "full can be combined with filter",
			args: "25h full",
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				{
					testutil.Require(t, query.ShowFull, "unexpected query: %+v", query)
					testutil.Require(t, !(query.Unit != "school_refusal"), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Cid != 0), "unexpected query: %+v", query)
					testutil.Require(t, !(query.CharacterQuery != ""), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Attr != ""), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Tree), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Flower), "unexpected query: %+v", query)
				}

			},
		},
		{
			name: "all filters are parsed and passed through",
			args: "25h miku 可爱 树 花",
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				{
					testutil.Require(t, !(query.Unit != "school_refusal"), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Cid != 0), "unexpected query: %+v", query)
					testutil.Require(t, !(query.CharacterQuery != "miku"), "unexpected query: %+v", query)
					testutil.Require(t, !(query.Attr != "cute"), "unexpected query: %+v", query)
					testutil.Require(t, query.Tree, "unexpected query: %+v", query)
					testutil.Require(t, query.Flower, "unexpected query: %+v", query)
				}

			},
		},
		{
			name: "piapro alias is normalized",
			args: "vs",
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				testutil.Require(t, !(query.Unit != "piapro"), "unexpected query: %+v", query)

			},
		},
		{
			name: "unknown nickname is preserved as character query",
			args: "初音未来",
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				{
					testutil.Require(t, !(query.Cid != 0), "unexpected query: %+v", query)
					testutil.Require(t, !(query.CharacterQuery != "初音未来"), "unexpected query: %+v", query)
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
				testutil.RequireArgs(t, !(err == nil), "expected error, got nil")

				return
			}
			testutil.Require(t, !(err != nil), "Handle() error = %v", err)

			resolved := result
			testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
			{

				testutil.Require(t, !(resolved.Module != parser.ModuleEducation), "unexpected command request: %+v", resolved)
				testutil.Require(t, !(resolved.Mode != "education-area"), "unexpected command request: %+v", resolved)
			}

			var query education.AreaItemQuery
			{
				err := json.Unmarshal(resolved.Params, &query)
				testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
			}

			tt.checkFunc(t, query)
		})
	}
}

func TestAreaItemHandleReportsSpecificFullUsageError(t *testing.T) {
	h := sekaiHandlers{}.AreaItemHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/区域道具",
		ArgText:    "full",
	})
	testutil.RequireArgs(t, !(err == nil), "expected error, got nil")
	{

		got := err.Error()
		{
			testutil.Require(t, strings.Contains(got, "full 需要和区域道具分类一起使用"), "unexpected error: %q", got)
			testutil.Require(t, !(strings.Contains(got, "使用方式")), "unexpected error: %q", got)
		}
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
				{
					testutil.Require(t, !(query.Cid != 0), "unexpected query: %+v", query)
					testutil.Require(t, !(query.CharacterQuery != ""), "unexpected query: %+v", query)
				}

			},
		},
		{
			name: "character query is passed through",
			args: "初音未来",
			checkFunc: func(t *testing.T, query education.BondsQuery) {
				t.Helper()
				{
					testutil.Require(t, !(query.Cid != 0), "unexpected query: %+v", query)
					testutil.Require(t, !(query.CharacterQuery != "初音未来"), "unexpected query: %+v", query)
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
			testutil.Require(t, !(err != nil), "Handle() error = %v", err)

			resolved := result
			testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
			{

				testutil.Require(t, !(resolved.Module != parser.ModuleEducation), "unexpected command request: %+v", resolved)
				testutil.Require(t, !(resolved.Mode != "education-bonds"), "unexpected command request: %+v", resolved)
			}

			var query education.BondsQuery
			{
				err := json.Unmarshal(resolved.Params, &query)
				testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
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
			testutil.Require(t, !(err != nil), "Handle() error = %v", err)

			resolved := result
			testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
			{

				testutil.Require(t, !(resolved.Module != parser.ModuleEducation), "unexpected command request: %+v", resolved)
				testutil.Require(t, !(resolved.Mode != tt.mode), "unexpected command request: %+v", resolved)
			}

			var params struct {
				Mode           string `json:"mode"`
				Platform       string `json:"platform"`
				PlatformUserID string `json:"platform_user_id"`
				Selector       string `json:"selector"`
			}
			{
				err := json.Unmarshal(resolved.Params, &params)
				testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
			}
			{

				testutil.Require(t, !(params.Mode != "self"), "unexpected params: %+v", params)
				testutil.Require(t, !(params.Platform != "qq"), "unexpected params: %+v", params)
				testutil.Require(t, !(params.PlatformUserID != "42"), "unexpected params: %+v", params)
				testutil.Require(t, !(params.Selector != "u2"), "unexpected params: %+v", params)
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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")

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
		testutil.Require(t, !(params.Unit != "school_refusal"), "unexpected area item params: %+v", params)
		testutil.Require(t, !(params.CharacterQuery != "miku"), "unexpected area item params: %+v", params)
		testutil.Require(t, !(params.Attr != "cute"), "unexpected area item params: %+v", params)
		testutil.Require(t, params.Tree, "unexpected area item params: %+v", params)
		testutil.Require(t, params.Flower, "unexpected area item params: %+v", params)
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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")

	var params struct {
		Mode           string `json:"mode"`
		Platform       string `json:"platform"`
		PlatformUserID string `json:"platform_user_id"`
		Selector       string `json:"selector"`
		CharacterQuery string `json:"character_query"`
	}
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.Mode != "self"), "unexpected params: %+v", params)
		testutil.Require(t, !(params.Platform != "qq"), "unexpected params: %+v", params)
		testutil.Require(t, !(params.PlatformUserID != "42"), "unexpected params: %+v", params)
		testutil.Require(t, !(params.Selector != "u2"), "unexpected params: %+v", params)
		testutil.Require(t, !(params.CharacterQuery != "初音未来"), "unexpected params: %+v", params)
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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleEducation), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "education-character-mission"), "unexpected command request: %+v", resolved)
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
		testutil.Require(t, !(params.CharacterQuery != "miku"), "unexpected character mission params: %+v", params)
		testutil.Require(t, !(params.ShowAll), "unexpected character mission params: %+v", params)
		testutil.Require(t, !(params.MissionType != ""), "unexpected character mission params: %+v", params)
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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleEducation), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "education-character-mission"), "unexpected command request: %+v", resolved)
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
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.CharacterQuery != "miku"), "unexpected character mission all params: %+v", params)
		testutil.Require(t, params.ShowAll, "unexpected character mission all params: %+v", params)
		testutil.Require(t, !(params.MissionType != "play_live"), "unexpected character mission all params: %+v", params)
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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleEducation), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "education-character-mission"), "unexpected command request: %+v", resolved)
	}

	var params struct {
		CharacterQuery string `json:"character_query"`
		ShowAll        bool   `json:"show_all"`
		MissionType    string `json:"mission_type"`
	}
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.CharacterQuery != "miku"), "unexpected character mission params: %+v", params)
		testutil.Require(t, params.ShowAll, "unexpected character mission params: %+v", params)
		testutil.Require(t, !(params.MissionType != "area_item_level_up_reality_world"), "unexpected character mission params: %+v", params)
	}

}

func TestResolveEducationQueryCharacters(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", "file:handler_education_query_characters?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	{
		_, err := client.Gamecharacter.Create().
			SetServerRegion("jp").SetGameID(21).
			SetFirstName("初音").SetGivenName("未来").
			SetFirstNameEnglish("Hatsune").SetGivenNameEnglish("Miku").
			Save(ctx)
		testutil.Require(t, !(err != nil), "create game character: %v", err)
	}

	rc := &RequestContext{Ctx: ctx, App: &renderapp.App{Sekai: client}, Region: renderregion.JP}

	area := education.AreaItemQuery{CharacterQuery: "Hatsune Miku"}
	{
		err := resolveEducationAreaQueryCharacter(rc, &area)
		{
			testutil.Require(t, !(err != nil), "resolve area character = %+v, %v", area, err)
			testutil.Require(t, !(area.Cid != 21), "resolve area character = %+v, %v", area, err)
		}
	}

	bonds := education.BondsQuery{CharacterQuery: "Hatsune Miku"}
	{
		err := resolveEducationBondsQueryCharacter(rc, &bonds)
		{
			testutil.Require(t, !(err != nil), "resolve bonds character = %+v, %v", bonds, err)
			testutil.Require(t, !(bonds.Cid != 21), "resolve bonds character = %+v, %v", bonds, err)
		}
	}

	mission := education.CharacterMissionQuery{CharacterQuery: "Hatsune Miku"}
	{
		err := resolveEducationMissionQueryCharacter(rc, &mission)
		{
			testutil.Require(t, !(err != nil), "resolve mission character = %+v, %v", mission, err)
			testutil.Require(t, !(mission.Cid != 21), "resolve mission character = %+v, %v", mission, err)
		}
	}

}

func TestResolveEducationQueryCharacterErrorsAndDefaultRegion(t *testing.T) {
	rc := &RequestContext{Ctx: context.Background(), Region: renderregion.JP}
	{
		err := resolveEducationAreaQueryCharacter(rc, &education.AreaItemQuery{CharacterQuery: "unknown"})
		testutil.RequireArgs(t, !(err == nil), "expected area character resolution error")
	}
	{

		err := resolveEducationBondsQueryCharacter(rc, &education.BondsQuery{CharacterQuery: "unknown"})
		testutil.RequireArgs(t, !(err == nil), "expected bonds character resolution error")
	}
	{

		err := resolveEducationMissionQueryCharacter(rc, &education.CharacterMissionQuery{CharacterQuery: "unknown"})
		testutil.RequireArgs(t, !(err == nil), "expected mission character resolution error")
	}

	var region renderregion.Value
	setDefaultEducationRegion(&region, renderregion.EN)
	testutil.Require(t, !(region != renderregion.EN), "default education region = %q", region)

}
