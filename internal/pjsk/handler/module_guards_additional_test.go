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
		if err != nil || got != tt.want {
			t.Errorf("parseInventoryFilter(%q) = %q, %v", tt.args, got, err)
		}
	}
	if _, err := parseInventoryFilter("unknown", "/查背包"); err == nil {
		t.Fatal("unknown inventory filter unexpectedly succeeded")
	}
	if err := validateInventoryFilterForRegion(renderregion.JP, renderinventory.FilterMysekai); err != nil {
		t.Fatalf("JP MySekai filter rejected: %v", err)
	}
	if err := validateInventoryFilterForRegion(renderregion.CN, renderinventory.FilterDefault); err != nil {
		t.Fatalf("CN default filter rejected: %v", err)
	}
	for _, filter := range []renderinventory.Filter{renderinventory.FilterMysekai, renderinventory.FilterMemory} {
		if err := validateInventoryFilterForRegion(renderregion.CN, filter); err == nil {
			t.Errorf("CN filter %q unexpectedly accepted", filter)
		}
	}

	ctx := additionalModuleContext("水晶", "/查背包", renderregion.JP)
	params, err := buildInventoryListParams(ctx)
	if err != nil || params.Filter != renderinventory.FilterJewel || params.Mode != "self" {
		t.Fatalf("buildInventoryListParams() = %+v, %v", params, err)
	}
	ctx.region = renderregion.CN
	ctx.ArgText = "ms材料"
	if _, err := buildInventoryListParams(ctx); err == nil {
		t.Fatal("CN MySekai inventory params unexpectedly succeeded")
	}
	ctx.region = renderregion.JP
	ctx.ArgText = ""
	ctx.uidArg = "@2"
	if _, err := buildInventoryListParams(ctx); err == nil {
		t.Fatal("targeted inventory params unexpectedly succeeded")
	}

	if _, err := executeInventory(nil); err == nil {
		t.Fatal("nil inventory runtime unexpectedly succeeded")
	}
	rc := &RequestContext{Ctx: context.Background(), App: &renderapp.App{}, Cmd: &CommandRequest{Mode: "inventory-list"}}
	if _, err := executeInventory(rc); err == nil {
		t.Fatal("missing inventory controller unexpectedly succeeded")
	}
	rc.App.Inventory = &renderinventory.Controller{}
	rc.Cmd.Mode = "wrong"
	if _, err := executeInventory(rc); err == nil {
		t.Fatal("invalid inventory mode unexpectedly succeeded")
	}
	rc.Cmd = nil
	if _, err := executeInventory(rc); err == nil {
		t.Fatal("missing inventory command unexpectedly succeeded")
	}
	rc.Cmd = &CommandRequest{Mode: "inventory-list", Params: json.RawMessage(`{"filter":"mysekai"}`)}
	rc.Region = renderregion.CN
	if _, err := executeInventory(rc); err == nil {
		t.Fatal("CN MySekai runtime filter unexpectedly succeeded")
	}
	rc.Cmd.Params = nil
	rc.Region = renderregion.JP
	rc.RegionStr = "jp"
	if _, err := executeInventory(rc); err == nil {
		t.Fatal("inventory request without snapshot unexpectedly succeeded")
	}
}

func TestCostumeRequestHelpersAndExecutionGuards(t *testing.T) {
	for _, trigger := range []string{"/查服装", " /查头饰 ", "/查发型"} {
		if !isCostumeNameSearchTrigger(trigger) {
			t.Errorf("name-search trigger %q not recognized", trigger)
		}
	}
	if isCostumeNameSearchTrigger("/服装列表") {
		t.Fatal("list trigger recognized as a name search")
	}
	partTypes := map[string]string{
		"/查头饰":         "head",
		"/accessories": "head",
		"/查发型":         "hair",
		"/hairstyles":  "hair",
		"/服装列表":        "",
	}
	for trigger, want := range partTypes {
		if got := costumeListPartTypeForTrigger(trigger); got != want {
			t.Errorf("costumeListPartTypeForTrigger(%q) = %q", trigger, got)
		}
	}
	if costumeDetailPartTypeForTrigger("/costume") != "body" || costumeDetailPartTypeForTrigger("/查头饰") != "head" {
		t.Fatal("costume detail part type mismatch")
	}

	ctx := additionalModuleContext("miku", "/查服装", renderregion.JP)
	detail := makeCostumeDetailCommandRequest(ctx, rendercostume.Query{Query: "miku", ID: 7})
	if detail.Mode != "costume-detail" || detail.Query != "miku" || detail.Region != "jp" {
		t.Fatalf("detail command = %+v", detail)
	}
	var detailQuery rendercostume.Query
	if err := json.Unmarshal(detail.Params, &detailQuery); err != nil || detailQuery.ID != 7 || detailQuery.Region != "jp" {
		t.Fatalf("detail params = %+v, %v", detailQuery, err)
	}
	list := makeCostumeListCommandRequest(ctx, "body")
	var listQuery rendercostume.ListQuery
	if err := json.Unmarshal(list.Params, &listQuery); err != nil || listQuery.Query != "" || listQuery.Keyword != "miku" || listQuery.PartType != "body" {
		t.Fatalf("name list params = %+v, %v", listQuery, err)
	}
	ctx.TriggerCmd = "/服装列表"
	ctx.originalTriggerCmd = "/服装列表"
	list = makeCostumeListCommandRequest(ctx, "")
	listQuery = rendercostume.ListQuery{}
	if err := json.Unmarshal(list.Params, &listQuery); err != nil || listQuery.Query != "miku" || listQuery.Keyword != "" {
		t.Fatalf("ordinary list params = %+v, %v", listQuery, err)
	}

	legacy := &rendercostume.LegacyAccessoryIDError{AccessoryIDs: []int{11, 12}}
	converted, ok := legacyAccessoryListQuery(legacy, rendercostume.Query{Region: "cn", Character3DID: 21})
	if !ok || converted.Region != "cn" || converted.PartType != "head" || converted.Character3DID != 21 || len(converted.AccessoryIDs) != 2 {
		t.Fatalf("legacy accessory conversion = %+v, %v", converted, ok)
	}
	legacy.AccessoryIDs[0] = 99
	if converted.AccessoryIDs[0] != 11 {
		t.Fatal("legacy accessory IDs were not cloned")
	}
	for _, err := range []error{nil, errors.New("ordinary"), &rendercostume.LegacyAccessoryIDError{}} {
		if _, ok := legacyAccessoryListQuery(err, rendercostume.Query{}); ok {
			t.Errorf("legacyAccessoryListQuery(%v) unexpectedly succeeded", err)
		}
	}

	rc := &RequestContext{Ctx: context.Background(), App: &renderapp.App{}, Cmd: &CommandRequest{Mode: "costume-list"}}
	if _, err := executeCostume(rc); err == nil {
		t.Fatal("missing costume controller unexpectedly succeeded")
	}
	rc.App.Costumes = rendercostume.NewController(nil, nil, nil)
	rc.Cmd.Mode = "wrong"
	if _, err := executeCostume(rc); err == nil {
		t.Fatal("invalid costume mode unexpectedly succeeded")
	}
}

func TestModerationParsingHandlersAndExecutionGuards(t *testing.T) {
	if _, _, _, err := parseGlobalKillArgs("", "usage"); err == nil {
		t.Fatal("empty kill args unexpectedly succeeded")
	}
	if _, _, _, err := parseGlobalKillArgs("1 reason 2 extra", "usage"); err == nil {
		t.Fatal("too many kill args unexpectedly succeeded")
	}
	if _, _, _, err := parseGlobalKillArgs("bad reason", "usage"); err == nil {
		t.Fatal("invalid QQ unexpectedly succeeded")
	}
	for _, args := range []string{"1 reason 0", "1 reason nope"} {
		if _, _, _, err := parseGlobalKillArgs(args, "usage"); err == nil {
			t.Errorf("invalid days in %q unexpectedly succeeded", args)
		}
	}
	if _, _, _, err := parseGlobalKillArgs("1 "+strings.Repeat("理", 256), "usage"); err == nil {
		t.Fatal("overlong reason unexpectedly succeeded")
	}
	qqID, reason, days, err := parseGlobalKillArgs("00012 spam 3", "usage")
	if err != nil || qqID != "12" || reason != "spam" || days == nil || *days != 3 {
		t.Fatalf("timed kill args = %q, %q, %v, %v", qqID, reason, days, err)
	}
	_, _, days, err = parseGlobalKillArgs("12 spam", "usage")
	if err != nil || days != nil {
		t.Fatalf("permanent kill args days = %v, err = %v", days, err)
	}
	if _, err := parseQQIDArg("", "usage"); err == nil {
		t.Fatal("empty QQ arg unexpectedly succeeded")
	}
	if _, err := parseQQIDArg("1 2", "usage"); err == nil {
		t.Fatal("multiple QQ args unexpectedly succeeded")
	}
	if got, err := parseQQIDArg("00042", "usage"); err != nil || got != "42" {
		t.Fatalf("QQ normalization = %q, %v", got, err)
	}
	for _, value := range []string{"", "0", "-1", "not-a-number"} {
		if _, err := validateQQID(value); err == nil {
			t.Errorf("validateQQID(%q) unexpectedly succeeded", value)
		}
	}

	killCtx := additionalModuleContext("10001 spam", "/kill", renderregion.JP)
	if _, err := (sekaiHandlers{}).GlobalKillHandle().handleFunc(killCtx); err == nil {
		t.Fatal("self-kill handler unexpectedly succeeded")
	}
	killCtx.ArgText = "12 spam 2"
	request, err := (sekaiHandlers{}).GlobalKillHandle().handleFunc(killCtx)
	if err != nil || request.Mode != modeGlobalKill {
		t.Fatalf("kill request = %+v, %v", request, err)
	}
	backCtx := additionalModuleContext("00012", "/back", renderregion.JP)
	request, err = (sekaiHandlers{}).GlobalBackHandle().handleFunc(backCtx)
	if err != nil || request.Mode != modeGlobalBack {
		t.Fatalf("back request = %+v, %v", request, err)
	}

	if _, err := executeGlobalModeration(nil); err == nil {
		t.Fatal("nil moderation runtime unexpectedly succeeded")
	}
	service := &accountdata.BanService{}
	rc := &RequestContext{Ctx: context.Background(), App: &renderapp.App{BanChecker: service}, Cmd: &CommandRequest{Mode: modeGlobalKill, Params: json.RawMessage(`{`)}}
	if _, err := executeGlobalModeration(rc); err == nil {
		t.Fatal("malformed kill params unexpectedly succeeded")
	}
	rc.Cmd.Params = json.RawMessage(`{"platform":"qq","platform_user_id":"not-admin","qq_id":"12","reason":"spam"}`)
	if _, err := executeGlobalModeration(rc); err == nil {
		t.Fatal("non-admin kill unexpectedly succeeded")
	}
	service.SetAdminQQIDs([]string{"bad", "10001", "0"})
	rc.Cmd.Params = json.RawMessage(`{"platform":"qq","platform_user_id":"10001","qq_id":"12","reason":"spam","days":2}`)
	if _, err := executeGlobalModeration(rc); err == nil {
		t.Fatal("unconfigured admin kill unexpectedly succeeded")
	}
	rc.Cmd = &CommandRequest{Mode: modeGlobalBack, Params: json.RawMessage(`{`)}
	if _, err := executeGlobalModeration(rc); err == nil {
		t.Fatal("malformed back params unexpectedly succeeded")
	}
	rc.Cmd.Params = json.RawMessage(`{"platform":"qq","platform_user_id":"not-admin","qq_id":"12"}`)
	if _, err := executeGlobalModeration(rc); err == nil {
		t.Fatal("non-admin back unexpectedly succeeded")
	}
	rc.Cmd.Params = json.RawMessage(`{"platform":"qq","platform_user_id":"10001","qq_id":"12"}`)
	if _, err := executeGlobalModeration(rc); err == nil {
		t.Fatal("unconfigured admin back unexpectedly succeeded")
	}
	rc.Cmd = &CommandRequest{Mode: "wrong"}
	if _, err := executeGlobalModeration(rc); err == nil {
		t.Fatal("invalid moderation mode unexpectedly succeeded")
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
		if first != tt.first || remaining != tt.remaining {
			t.Errorf("splitFirstArg(%q) = %q, %q", tt.args, first, remaining)
		}
	}
	if _, err := buildEducationAreaQuery("", "/区域道具"); err == nil {
		t.Fatal("empty area query unexpectedly succeeded")
	}
	if _, err := buildEducationAreaQuery("full", "/区域道具"); err == nil {
		t.Fatal("bare full area query unexpectedly succeeded")
	}
	area, err := buildEducationAreaQuery("花树 full", "/区域道具")
	if err != nil || !area.ShowFull || !area.Tree || !area.Flower {
		t.Fatalf("plant area query = %+v, %v", area, err)
	}
	area, err = buildEducationAreaQuery("mmj cute", "/区域道具")
	if err != nil || area.Unit != "idol" || area.Attr != "cute" {
		t.Fatalf("unit/attribute area query = %+v, %v", area, err)
	}
	area, err = buildEducationAreaQuery("初音未来", "/区域道具")
	if err != nil || area.CharacterQuery != "初音未来" {
		t.Fatalf("character area query = %+v, %v", area, err)
	}
	if full, rest := extractEducationAreaFullFlag("全部 mmj full"); !full || rest != "mmj" {
		t.Fatalf("full extraction = %v, %q", full, rest)
	}
	if found, rest := extractEducationAreaFlag("tree x tree", "tree"); !found || rest != "x" {
		t.Fatalf("flag extraction = %v, %q", found, rest)
	}
	if unit, rest := extractEducationAreaUnit("mmj vbs tail"); unit != "idol" || rest != "vbs tail" {
		t.Fatalf("unit extraction = %q, %q", unit, rest)
	}
	if attr, rest := extractEducationAreaAttr("cute tail"); attr != "cute" || rest != "tail" {
		t.Fatalf("attribute extraction = %q, %q", attr, rest)
	}
	if cid, query, rest := extractEducationAreaCharacter(" 初音未来 "); cid != 0 || query != "初音未来" || rest != "" {
		t.Fatalf("character extraction = %d, %q, %q", cid, query, rest)
	}

	ctx := additionalModuleContext("", "/cr任务", renderregion.JP)
	missionHandler := (sekaiHandlers{}).CharacterMissionHandle()
	if _, err := missionHandler.handleFunc(ctx); err == nil {
		t.Fatal("empty character mission unexpectedly succeeded")
	}
	ctx.ArgText = "miku extra"
	if _, err := missionHandler.handleFunc(ctx); err == nil {
		t.Fatal("character mission with extra args unexpectedly succeeded")
	}
	ctx.ArgText = "miku 全部 unknown"
	if _, err := missionHandler.handleFunc(ctx); err == nil {
		t.Fatal("unknown all-mission type unexpectedly succeeded")
	}
	ctx.ArgText = "miku 全部 队长次数"
	request, err := missionHandler.handleFunc(ctx)
	if err != nil || request.Mode != "education-character-mission" {
		t.Fatalf("character mission request = %+v, %v", request, err)
	}
	areaCtx := additionalModuleContext("花树 full", "/区域道具", renderregion.JP)
	request, err = (sekaiHandlers{}).AreaItemHandle().handleFunc(areaCtx)
	if err != nil || request.Mode != "education-area" {
		t.Fatalf("area request = %+v, %v", request, err)
	}
	if _, err := executeEducation(&RequestContext{App: nil, Cmd: &CommandRequest{Mode: "education-area"}}); err == nil {
		t.Fatal("missing education controller unexpectedly succeeded")
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
		if (err != nil) != tt.wantErr || params.UpcomingIndex != tt.wantIndex || params.Query != tt.wantQuery {
			t.Errorf("buildMiscBirthdayParams(%q) = %+v, %v", tt.args, params, err)
		}
	}
	miscCtx := additionalModuleContext("2", "/生日", renderregion.JP)
	request, err = (sekaiHandlers{}).MiscBirthdayHandle().handleFunc(miscCtx)
	if err != nil || request.Mode != "misc-birthday" {
		t.Fatalf("birthday request = %+v, %v", request, err)
	}
	miscRC := &RequestContext{Ctx: context.Background(), App: &renderapp.App{Misc: rendermisc.NewController(nil)}, Cmd: &CommandRequest{Mode: "wrong"}}
	if _, err := executeMisc(miscRC); err == nil {
		t.Fatal("invalid misc mode unexpectedly succeeded")
	}

	if _, err := executeMysekaiHousingSK(nil, "jp"); err == nil {
		t.Fatal("nil MySekai housing runtime unexpectedly succeeded")
	}
	if _, err := executeMysekaiHousingSK(&RequestContext{App: &renderapp.App{}}, "jp"); err == nil {
		t.Fatal("missing MySekai controller unexpectedly succeeded")
	}
}
