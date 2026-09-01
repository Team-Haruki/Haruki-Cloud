package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/accountdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendercostume "haruki-cloud/internal/pjsk/render/costume"
	renderinventory "haruki-cloud/internal/pjsk/render/inventory"
	rendermisc "haruki-cloud/internal/pjsk/render/misc"
	"haruki-cloud/internal/testutil"
)

func additionalModuleContext(args, trigger string, region renderregion.Value) HarrukiSekaiHandlerContext {
	return HarrukiSekaiHandlerContext{
		PjskHandlerContext: PjskHandlerContext{
			Context:    context.Background(),
			Platform:   "qq",
			UserId:     "10001",
			TriggerCmd: trigger,
			ArgText:    args,
		},
		region:             region,
		explicitRegion:     true,
		originalTriggerCmd: trigger,
		flags:              map[string]bool{},
	}
}

func TestInventoryParsingAndExecutionGuards(t *testing.T) {
	filters := []struct {
		args string
		want renderinventory.Filter
	}{
		{"", renderinventory.FilterDefault},
		{" 水 晶 ", renderinventory.FilterJewel},
		{"演出能量", renderinventory.FilterBoost},
		{"MS 素材", renderinventory.FilterMysekai},
		{"memory", renderinventory.FilterMemory},
	}
	for _, tt := range filters {
		got, err := parseInventoryFilter(tt.args, "/查背包")
		testutil.Check(t, !(err != nil || got != tt.want), "parseInventoryFilter(%q) = %q, %v", tt.args, got, err)

	}
	{
		_, err := parseInventoryFilter("unknown", "/查背包")
		testutil.RequireArgs(t, !(err == nil), "unknown inventory filter unexpectedly succeeded")
	}
	{

		err := validateInventoryFilterForRegion(renderregion.JP, renderinventory.FilterMysekai)
		testutil.Require(t, !(err != nil), "JP MySekai filter rejected: %v", err)
	}
	{

		err := validateInventoryFilterForRegion(renderregion.CN, renderinventory.FilterDefault)
		testutil.Require(t, !(err != nil), "CN default filter rejected: %v", err)
	}

	for _, filter := range []renderinventory.Filter{renderinventory.FilterMysekai, renderinventory.FilterMemory} {
		{
			err := validateInventoryFilterForRegion(renderregion.CN, filter)
			testutil.Check(t, !(err == nil), "CN filter %q unexpectedly accepted", filter)
		}

	}

	ctx := additionalModuleContext("水晶", "/查背包", renderregion.JP)
	params, err := buildInventoryListParams(ctx)
	{
		testutil.Require(t, !(err != nil), "buildInventoryListParams() = %+v, %v", params, err)
		testutil.Require(t, !(params.Filter != renderinventory.FilterJewel), "buildInventoryListParams() = %+v, %v", params, err)
		testutil.Require(t, !(params.Mode != "self"), "buildInventoryListParams() = %+v, %v", params, err)
	}

	ctx.region = renderregion.CN
	ctx.ArgText = "ms材料"
	{
		_, err := buildInventoryListParams(ctx)
		testutil.RequireArgs(t, !(err == nil), "CN MySekai inventory params unexpectedly succeeded")
	}

	ctx.region = renderregion.JP
	ctx.ArgText = ""
	ctx.uidArg = "@2"
	{
		_, err := buildInventoryListParams(ctx)
		testutil.RequireArgs(t, !(err == nil), "targeted inventory params unexpectedly succeeded")
	}
	{

		_, err := executeInventory(nil)
		testutil.RequireArgs(t, !(err == nil), "nil inventory runtime unexpectedly succeeded")
	}

	rc := &RequestContext{Ctx: context.Background(), App: &renderapp.App{}, Cmd: &CommandRequest{Mode: "inventory-list"}}
	{
		_, err := executeInventory(rc)
		testutil.RequireArgs(t, !(err == nil), "missing inventory controller unexpectedly succeeded")
	}

	rc.App.Inventory = &renderinventory.Controller{}
	rc.Cmd.Mode = "wrong"
	{
		_, err := executeInventory(rc)
		testutil.RequireArgs(t, !(err == nil), "invalid inventory mode unexpectedly succeeded")
	}

	rc.Cmd = nil
	{
		_, err := executeInventory(rc)
		testutil.RequireArgs(t, !(err == nil), "missing inventory command unexpectedly succeeded")
	}

	rc.Cmd = &CommandRequest{Mode: "inventory-list", Params: json.RawMessage(`{"filter":"mysekai"}`)}
	rc.Region = renderregion.CN
	{
		_, err := executeInventory(rc)
		testutil.RequireArgs(t, !(err == nil), "CN MySekai runtime filter unexpectedly succeeded")
	}

	rc.Cmd.Params = nil
	rc.Region = renderregion.JP
	rc.RegionStr = "jp"
	{
		_, err := executeInventory(rc)
		testutil.RequireArgs(t, !(err == nil), "inventory request without snapshot unexpectedly succeeded")
	}

}

func TestCostumeRequestHelpersAndExecutionGuards(t *testing.T) {
	for _, trigger := range []string{"/查服装", " /查头饰 ", "/查发型"} {
		testutil.Check(t, isCostumeNameSearchTrigger(trigger), "name-search trigger %q not recognized", trigger)

	}
	testutil.RequireArgs(t, !(isCostumeNameSearchTrigger("/服装列表")), "list trigger recognized as a name search")

	partTypes := map[string]string{
		"/查头饰":         "head",
		"/accessories": "head",
		"/查发型":         "hair",
		"/hairstyles":  "hair",
		"/服装列表":        "",
	}
	for trigger, want := range partTypes {
		{
			got := costumeListPartTypeForTrigger(trigger)
			testutil.Check(t, !(got != want), "costumeListPartTypeForTrigger(%q) = %q", trigger, got)
		}

	}
	{
		testutil.RequireArgs(t, !(costumeDetailPartTypeForTrigger("/costume") != "body"), "costume detail part type mismatch")
		testutil.RequireArgs(t, !(costumeDetailPartTypeForTrigger("/查头饰") != "head"), "costume detail part type mismatch")
	}

	ctx := additionalModuleContext("miku", "/查服装", renderregion.JP)
	detail := makeCostumeDetailCommandRequest(ctx, rendercostume.Query{Query: "miku", ID: 7})
	{
		testutil.Require(t, !(detail.Mode != "costume-detail"), "detail command = %+v", detail)
		testutil.Require(t, !(detail.Query != "miku"), "detail command = %+v", detail)
		testutil.Require(t, !(detail.Region != "jp"), "detail command = %+v", detail)
	}

	var detailQuery rendercostume.Query
	{
		err := json.Unmarshal(detail.Params, &detailQuery)
		{
			testutil.Require(t, !(err != nil), "detail params = %+v, %v", detailQuery, err)
			testutil.Require(t, !(detailQuery.ID != 7), "detail params = %+v, %v", detailQuery, err)
			testutil.Require(t, !(detailQuery.Region != "jp"), "detail params = %+v, %v", detailQuery, err)
		}
	}

	list := makeCostumeListCommandRequest(ctx, "body")
	var listQuery rendercostume.ListQuery
	{
		err := json.Unmarshal(list.Params, &listQuery)
		{
			testutil.Require(t, !(err != nil), "name list params = %+v, %v", listQuery, err)
			testutil.Require(t, !(listQuery.Query != ""), "name list params = %+v, %v", listQuery, err)
			testutil.Require(t, !(listQuery.Keyword != "miku"), "name list params = %+v, %v", listQuery, err)
			testutil.Require(t, !(listQuery.PartType != "body"), "name list params = %+v, %v", listQuery, err)
		}
	}

	ctx.TriggerCmd = "/服装列表"
	ctx.originalTriggerCmd = "/服装列表"
	list = makeCostumeListCommandRequest(ctx, "")
	listQuery = rendercostume.ListQuery{}
	{
		err := json.Unmarshal(list.Params, &listQuery)
		{
			testutil.Require(t, !(err != nil), "ordinary list params = %+v, %v", listQuery, err)
			testutil.Require(t, !(listQuery.Query != "miku"), "ordinary list params = %+v, %v", listQuery, err)
			testutil.Require(t, !(listQuery.Keyword != ""), "ordinary list params = %+v, %v", listQuery, err)
		}
	}

	legacy := &rendercostume.LegacyAccessoryIDError{AccessoryIDs: []int{11, 12}}
	converted, ok := legacyAccessoryListQuery(legacy, rendercostume.Query{Region: "cn", Character3DID: 21})
	{
		testutil.Require(t, ok, "legacy accessory conversion = %+v, %v", converted, ok)
		testutil.Require(t, !(converted.Region != "cn"), "legacy accessory conversion = %+v, %v", converted, ok)
		testutil.Require(t, !(converted.PartType != "head"), "legacy accessory conversion = %+v, %v", converted, ok)
		testutil.Require(t, !(converted.Character3DID != 21), "legacy accessory conversion = %+v, %v", converted, ok)
		testutil.Require(t, !(len(converted.AccessoryIDs) != 2), "legacy accessory conversion = %+v, %v", converted, ok)
	}

	legacy.AccessoryIDs[0] = 99
	testutil.RequireArgs(t, !(converted.AccessoryIDs[0] != 11), "legacy accessory IDs were not cloned")

	for _, err := range []error{nil, errors.New("ordinary"), &rendercostume.LegacyAccessoryIDError{}} {
		{
			_, ok := legacyAccessoryListQuery(err, rendercostume.Query{})
			testutil.Check(t, !(ok), "legacyAccessoryListQuery(%v) unexpectedly succeeded", err)
		}

	}

	rc := &RequestContext{Ctx: context.Background(), App: &renderapp.App{}, Cmd: &CommandRequest{Mode: "costume-list"}}
	{
		_, err := executeCostume(rc)
		testutil.RequireArgs(t, !(err == nil), "missing costume controller unexpectedly succeeded")
	}

	rc.App.Costumes = rendercostume.NewController(nil, nil, nil)
	rc.Cmd.Mode = "wrong"
	{
		_, err := executeCostume(rc)
		testutil.RequireArgs(t, !(err == nil), "invalid costume mode unexpectedly succeeded")
	}

}

func TestModerationParsingHandlersAndExecutionGuards(t *testing.T) {
	{
		_, _, _, err := parseGlobalKillArgs("", "usage")
		testutil.RequireArgs(t, !(err == nil), "empty kill args unexpectedly succeeded")
	}
	{

		_, _, _, err := parseGlobalKillArgs("1 reason 2 extra", "usage")
		testutil.RequireArgs(t, !(err == nil), "too many kill args unexpectedly succeeded")
	}
	{

		_, _, _, err := parseGlobalKillArgs("bad reason", "usage")
		testutil.RequireArgs(t, !(err == nil), "invalid QQ unexpectedly succeeded")
	}

	for _, args := range []string{"1 reason 0", "1 reason nope"} {
		{
			_, _, _, err := parseGlobalKillArgs(args, "usage")
			testutil.Check(t, !(err == nil), "invalid days in %q unexpectedly succeeded", args)
		}

	}
	{
		_, _, _, err := parseGlobalKillArgs("1 "+strings.Repeat("理", 256), "usage")
		testutil.RequireArgs(t, !(err == nil), "overlong reason unexpectedly succeeded")
	}

	qqID, reason, days, err := parseGlobalKillArgs("00012 spam 3", "usage")
	{
		testutil.Require(t, !(err != nil), "timed kill args = %q, %q, %v, %v", qqID, reason, days, err)
		testutil.Require(t, !(qqID != "12"), "timed kill args = %q, %q, %v, %v", qqID, reason, days, err)
		testutil.Require(t, !(reason != "spam"), "timed kill args = %q, %q, %v, %v", qqID, reason, days, err)
		testutil.Require(t, !(days == nil), "timed kill args = %q, %q, %v, %v", qqID, reason, days, err)
		testutil.Require(t, !(*days != 3), "timed kill args = %q, %q, %v, %v", qqID, reason, days, err)
	}

	_, _, days, err = parseGlobalKillArgs("12 spam", "usage")
	{
		testutil.Require(t, !(err != nil), "permanent kill args days = %v, err = %v", days, err)
		testutil.Require(t, !(days != nil), "permanent kill args days = %v, err = %v", days, err)
	}
	{

		_, err := parseQQIDArg("", "usage")
		testutil.RequireArgs(t, !(err == nil), "empty QQ arg unexpectedly succeeded")
	}
	{

		_, err := parseQQIDArg("1 2", "usage")
		testutil.RequireArgs(t, !(err == nil), "multiple QQ args unexpectedly succeeded")
	}
	{

		got, err := parseQQIDArg("00042", "usage")
		{
			testutil.Require(t, !(err != nil), "QQ normalization = %q, %v", got, err)
			testutil.Require(t, !(got != "42"), "QQ normalization = %q, %v", got, err)
		}
	}

	for _, value := range []string{"", "0", "-1", "not-a-number"} {
		{
			_, err := validateQQID(value)
			testutil.Check(t, !(err == nil), "validateQQID(%q) unexpectedly succeeded", value)
		}

	}

	killCtx := additionalModuleContext("10001 spam", "/kill", renderregion.JP)
	{
		_, err := (sekaiHandlers{}).GlobalKillHandle().handleFunc(killCtx)
		testutil.RequireArgs(t, !(err == nil), "self-kill handler unexpectedly succeeded")
	}

	killCtx.ArgText = "12 spam 2"
	request, err := (sekaiHandlers{}).GlobalKillHandle().handleFunc(killCtx)
	{
		testutil.Require(t, !(err != nil), "kill request = %+v, %v", request, err)
		testutil.Require(t, !(request.Mode != modeGlobalKill), "kill request = %+v, %v", request, err)
	}

	backCtx := additionalModuleContext("00012", "/back", renderregion.JP)
	request, err = (sekaiHandlers{}).GlobalBackHandle().handleFunc(backCtx)
	{
		testutil.Require(t, !(err != nil), "back request = %+v, %v", request, err)
		testutil.Require(t, !(request.Mode != modeGlobalBack), "back request = %+v, %v", request, err)
	}
	{

		_, err := executeGlobalModeration(nil)
		testutil.RequireArgs(t, !(err == nil), "nil moderation runtime unexpectedly succeeded")
	}

	service := &accountdata.BanService{}
	rc := &RequestContext{Ctx: context.Background(), App: &renderapp.App{BanChecker: service}, Cmd: &CommandRequest{Mode: modeGlobalKill, Params: json.RawMessage(`{`)}}
	{
		_, err := executeGlobalModeration(rc)
		testutil.RequireArgs(t, !(err == nil), "malformed kill params unexpectedly succeeded")
	}

	rc.Cmd.Params = json.RawMessage(`{"platform":"qq","platform_user_id":"not-admin","qq_id":"12","reason":"spam"}`)
	{
		_, err := executeGlobalModeration(rc)
		testutil.RequireArgs(t, !(err == nil), "non-admin kill unexpectedly succeeded")
	}

	service.SetAdminQQIDs([]string{"bad", "10001", "0"})
	rc.Cmd.Params = json.RawMessage(`{"platform":"qq","platform_user_id":"10001","qq_id":"12","reason":"spam","days":2}`)
	{
		_, err := executeGlobalModeration(rc)
		testutil.RequireArgs(t, !(err == nil), "unconfigured admin kill unexpectedly succeeded")
	}

	rc.Cmd = &CommandRequest{Mode: modeGlobalBack, Params: json.RawMessage(`{`)}
	{
		_, err := executeGlobalModeration(rc)
		testutil.RequireArgs(t, !(err == nil), "malformed back params unexpectedly succeeded")
	}

	rc.Cmd.Params = json.RawMessage(`{"platform":"qq","platform_user_id":"not-admin","qq_id":"12"}`)
	{
		_, err := executeGlobalModeration(rc)
		testutil.RequireArgs(t, !(err == nil), "non-admin back unexpectedly succeeded")
	}

	rc.Cmd.Params = json.RawMessage(`{"platform":"qq","platform_user_id":"10001","qq_id":"12"}`)
	{
		_, err := executeGlobalModeration(rc)
		testutil.RequireArgs(t, !(err == nil), "unconfigured admin back unexpectedly succeeded")
	}

	rc.Cmd = &CommandRequest{Mode: "wrong"}
	{
		_, err := executeGlobalModeration(rc)
		testutil.RequireArgs(t, !(err == nil), "invalid moderation mode unexpectedly succeeded")
	}

}

func TestEducationAndMiscPureHelpersAndGuards(t *testing.T) {
	for _, tt := range []struct {
		args      string
		first     string
		remaining string
	}{
		{"", "", ""},
		{"miku", "miku", ""},
		{" miku  队长次数 ", "miku", "队长次数"},
	} {
		first, remaining := splitFirstArg(tt.args)
		testutil.Check(t, !(first != tt.first || remaining != tt.remaining), "splitFirstArg(%q) = %q, %q", tt.args, first, remaining)

	}
	{
		_, err := buildEducationAreaQuery("", "/区域道具")
		testutil.RequireArgs(t, !(err == nil), "empty area query unexpectedly succeeded")
	}
	{

		_, err := buildEducationAreaQuery("full", "/区域道具")
		testutil.RequireArgs(t, !(err == nil), "bare full area query unexpectedly succeeded")
	}

	area, err := buildEducationAreaQuery("花树 full", "/区域道具")
	{
		testutil.Require(t, !(err != nil), "plant area query = %+v, %v", area, err)
		testutil.Require(t, area.ShowFull, "plant area query = %+v, %v", area, err)
		testutil.Require(t, area.Tree, "plant area query = %+v, %v", area, err)
		testutil.Require(t, area.Flower, "plant area query = %+v, %v", area, err)
	}

	area, err = buildEducationAreaQuery("mmj cute", "/区域道具")
	{
		testutil.Require(t, !(err != nil), "unit/attribute area query = %+v, %v", area, err)
		testutil.Require(t, !(area.Unit != "idol"), "unit/attribute area query = %+v, %v", area, err)
		testutil.Require(t, !(area.Attr != "cute"), "unit/attribute area query = %+v, %v", area, err)
	}

	area, err = buildEducationAreaQuery("初音未来", "/区域道具")
	{
		testutil.Require(t, !(err != nil), "character area query = %+v, %v", area, err)
		testutil.Require(t, !(area.CharacterQuery != "初音未来"), "character area query = %+v, %v", area, err)
	}
	{

		full, rest := extractEducationAreaFullFlag("全部 mmj full")
		{
			testutil.Require(t, full, "full extraction = %v, %q", full, rest)
			testutil.Require(t, !(rest != "mmj"), "full extraction = %v, %q", full, rest)
		}
	}
	{

		found, rest := extractEducationAreaFlag("tree x tree", "tree")
		{
			testutil.Require(t, found, "flag extraction = %v, %q", found, rest)
			testutil.Require(t, !(rest != "x"), "flag extraction = %v, %q", found, rest)
		}
	}
	{

		unit, rest := extractEducationAreaUnit("mmj vbs tail")
		{
			testutil.Require(t, !(unit != "idol"), "unit extraction = %q, %q", unit, rest)
			testutil.Require(t, !(rest != "vbs tail"), "unit extraction = %q, %q", unit, rest)
		}
	}
	{

		attr, rest := extractEducationAreaAttr("cute tail")
		{
			testutil.Require(t, !(attr != "cute"), "attribute extraction = %q, %q", attr, rest)
			testutil.Require(t, !(rest != "tail"), "attribute extraction = %q, %q", attr, rest)
		}
	}
	{

		cid, query, rest := extractEducationAreaCharacter(" 初音未来 ")
		{
			testutil.Require(t, !(cid != 0), "character extraction = %d, %q, %q", cid, query, rest)
			testutil.Require(t, !(query != "初音未来"), "character extraction = %d, %q, %q", cid, query, rest)
			testutil.Require(t, !(rest != ""), "character extraction = %d, %q, %q", cid, query, rest)
		}
	}

	ctx := additionalModuleContext("", "/cr任务", renderregion.JP)
	missionHandler := (sekaiHandlers{}).CharacterMissionHandle()
	{
		_, err := missionHandler.handleFunc(ctx)
		testutil.RequireArgs(t, !(err == nil), "empty character mission unexpectedly succeeded")
	}

	ctx.ArgText = "miku extra"
	{
		_, err := missionHandler.handleFunc(ctx)
		testutil.RequireArgs(t, !(err == nil), "character mission with extra args unexpectedly succeeded")
	}

	ctx.ArgText = "miku 全部 unknown"
	{
		_, err := missionHandler.handleFunc(ctx)
		testutil.RequireArgs(t, !(err == nil), "unknown all-mission type unexpectedly succeeded")
	}

	ctx.ArgText = "miku 全部 队长次数"
	request, err := missionHandler.handleFunc(ctx)
	{
		testutil.Require(t, !(err != nil), "character mission request = %+v, %v", request, err)
		testutil.Require(t, !(request.Mode != "education-character-mission"), "character mission request = %+v, %v", request, err)
	}

	areaCtx := additionalModuleContext("花树 full", "/区域道具", renderregion.JP)
	request, err = (sekaiHandlers{}).AreaItemHandle().handleFunc(areaCtx)
	{
		testutil.Require(t, !(err != nil), "area request = %+v, %v", request, err)
		testutil.Require(t, !(request.Mode != "education-area"), "area request = %+v, %v", request, err)
	}
	{

		_, err := executeEducation(&RequestContext{App: nil, Cmd: &CommandRequest{Mode: "education-area"}})
		testutil.RequireArgs(t, !(err == nil), "missing education controller unexpectedly succeeded")
	}

	for _, tt := range []struct {
		args      string
		wantIndex int
		wantQuery string
		wantErr   bool
	}{
		{"", 1, "", false},
		{"1", 1, "", false},
		{"26", 26, "", false},
		{"0", 0, "", true},
		{"27", 0, "", true},
		{"miku", 0, "miku", false},
	} {
		params, err := buildMiscBirthdayParams(tt.args)
		testutil.Check(t, !((err != nil) != tt.wantErr || params.UpcomingIndex != tt.wantIndex || params.Query != tt.wantQuery), "buildMiscBirthdayParams(%q) = %+v, %v", tt.args, params, err)

	}
	miscCtx := additionalModuleContext("2", "/生日", renderregion.JP)
	request, err = (sekaiHandlers{}).MiscBirthdayHandle().handleFunc(miscCtx)
	{
		testutil.Require(t, !(err != nil), "birthday request = %+v, %v", request, err)
		testutil.Require(t, !(request.Mode != "misc-birthday"), "birthday request = %+v, %v", request, err)
	}

	miscRC := &RequestContext{Ctx: context.Background(), App: &renderapp.App{Misc: rendermisc.NewController(nil)}, Cmd: &CommandRequest{Mode: "wrong"}}
	{
		_, err := executeMisc(miscRC)
		testutil.RequireArgs(t, !(err == nil), "invalid misc mode unexpectedly succeeded")
	}
	{

		_, err := executeMysekaiHousingSK(nil, "jp")
		testutil.RequireArgs(t, !(err == nil), "nil MySekai housing runtime unexpectedly succeeded")
	}
	{

		_, err := executeMysekaiHousingSK(&RequestContext{App: &renderapp.App{}}, "jp")
		testutil.RequireArgs(t, !(err == nil), "missing MySekai controller unexpectedly succeeded")
	}

}
