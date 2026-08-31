//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package handler

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
)

func mysekaiEdgeContext(args string) HarrukiSekaiHandlerContext {
	return HarrukiSekaiHandlerContext{
		PjskHandlerContext: PjskHandlerContext{
			Context: context.Background(), Platform: "qq", UserId: "actor", ArgText: args,
		},
		originalTriggerCmd: "/test",
	}
}

func TestMysekaiHandlersSelfOnlyAndSpecialBranches(t *testing.T) {
	selfOnly := []HarukiSekaiCommandHandler{
		sekaiHandlers{}.MysekaiResourceHandle(),
		sekaiHandlers{}.MysekaiOverviewHandle(),
		sekaiHandlers{}.MysekaiMapHandle(),
		sekaiHandlers{}.MysekaiTalkListHandle(),
		sekaiHandlers{}.MysekaiFixtureListHandle(),
		sekaiHandlers{}.MysekaiFurnitureHandle(),
		sekaiHandlers{}.MysekaiDoorUpgradeHandle(),
		sekaiHandlers{}.MysekaiMusicRecordHandle(),
		sekaiHandlers{}.MysekaiBlueprintHandle(),
		sekaiHandlers{}.MysekaiPhotoHandle(),
	}
	for _, h := range selfOnly {
		ctx := mysekaiEdgeContext("1")
		ctx.uidArg = "@target"
		if _, err := h.handleFunc(ctx); err == nil {
			t.Fatalf("handler %s unexpectedly accepted a target query", h.Path)
		}
	}

	for _, tc := range []struct {
		handler HarukiSekaiCommandHandler
		args    string
	}{
		{sekaiHandlers{}.MysekaiTalkListHandle(), "123"},
		{sekaiHandlers{}.MysekaiTalkListHandle(), "not-a-character"},
		{sekaiHandlers{}.MysekaiBlueprintHandle(), "not-a-character"},
		{sekaiHandlers{}.MysekaiPhotoHandle(), "0"},
		{sekaiHandlers{}.MysekaiPhotoHandle(), "not-a-number"},
	} {
		if _, err := tc.handler.handleFunc(mysekaiEdgeContext(tc.args)); err == nil {
			t.Fatalf("handler %s unexpectedly accepted %q", tc.handler.Path, tc.args)
		}
	}

	furniture, err := sekaiHandlers{}.MysekaiFurnitureHandle().handleFunc(mysekaiEdgeContext("full table"))
	if err != nil || furniture == nil || !strings.Contains(string(furniture.Params), "category_query") {
		t.Fatalf("full furniture category = %#v, %v", furniture, err)
	}
	door, err := sekaiHandlers{}.MysekaiDoorUpgradeHandle().handleFunc(mysekaiEdgeContext("mmj extra"))
	if err != nil || door == nil || door.Query != "2 extra" {
		t.Fatalf("door gate query = %#v, %v", door, err)
	}
	record, err := sekaiHandlers{}.MysekaiMusicRecordHandle().handleFunc(mysekaiEdgeContext("ID remaining"))
	if err != nil || record == nil || !strings.Contains(string(record.Params), "show_id") {
		t.Fatalf("music record id params = %#v, %v", record, err)
	}
}

func TestMysekaiHousingParserErrorBranches(t *testing.T) {
	for _, input := range []string{
		"id=bad", "sample=bad", "sample=0", "interval=bad", "bad-rank",
	} {
		if _, err := parseMysekaiHousingSKArgs(input); err == nil {
			t.Fatalf("parseMysekaiHousingSKArgs(%q) unexpectedly succeeded", input)
		}
	}
	query, err := parseMysekaiHousingSKArgs("25 1-2")
	if err != nil || query.HousingID != 25 || !reflect.DeepEqual(query.Ranks, []int{1, 2}) {
		t.Fatalf("housing shorthand = %+v, %v", query, err)
	}
	for _, part := range []string{"x-2", "2-x", "2-0"} {
		if _, err := parseMysekaiHousingRankPart(part); err == nil {
			t.Fatalf("parseMysekaiHousingRankPart(%q) unexpectedly succeeded", part)
		}
	}
	if _, err := parseMysekaiHousingRankTokens([]string{"1-3", "4-6"}); err == nil {
		t.Fatal("expected aggregate rank count limit")
	}
}

func TestMysekaiRuntimePureBranches(t *testing.T) {
	for _, mode := range []string{"mysekai-map", "mysekai-photo", "mysekai-resource", "mysekai-fixture-list", "mysekai-door-upgrade", "other"} {
		options := mysekaiRenderContextOptionsForMode(mode)
		if mode == "other" && !options.NeedProfile {
			t.Fatalf("default options = %+v", options)
		}
	}
	if !shouldEnforceMysekaiExpiry("mysekai-resource-map") {
		t.Fatal("resource-map should enforce expiry")
	}

	showHarvested := true
	params := executionCoverageParams(t, rendermysekai.MapQuery{ShowHarvested: &showHarvested})
	if ok, err := mysekaiMapHasRemainingMaterials(mySekaiRenderContext{}, params); err != nil || !ok {
		t.Fatalf("show-harvested remaining check = %v, %v", ok, err)
	}

	wantErr := errors.New("job failed")
	if _, err := executeConcurrentMessages(nil, func(context.Context) (onebot11.Message, error) {
		return nil, wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("concurrent job error = %v", err)
	}
	message, err := executeConcurrentMessages(context.Background(),
		func(context.Context) (onebot11.Message, error) { return onebot11.Message{onebot11.Text("a")}, nil },
		func(context.Context) (onebot11.Message, error) { return onebot11.Message{onebot11.Text("b")}, nil },
	)
	if err != nil || len(message) != 2 {
		t.Fatalf("concurrent messages = %#v, %v", message, err)
	}

	rc := &RequestContext{Ctx: context.Background(), App: &renderapp.App{}, Platform: "qq", PlatformUserID: "actor"}
	err = buildMysekaiExpiredReplayError(rc, 0, rendermysekai.SnapshotStatus{LastUpdatedAt: time.Unix(1_700_000_000, 0)})
	if err == nil || !strings.Contains(err.Error(), "上次更新时间") {
		t.Fatalf("expired replay error = %v", err)
	}
}

func TestMysekaiExecutionHelperErrorBranches(t *testing.T) {
	controller := rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{})
	renderCtx := mySekaiRenderContext{Controller: controller, Region: "jp"}
	app := &renderapp.App{}
	for _, mode := range []string{
		mySekaiFixtureListCommand,
		mySekaiDoorUpgradeCommand,
		mySekaiMusicRecordCommand,
		mySekaiPhotoCommand,
		mySekaiTalkListCommand,
	} {
		rc := NewRequestContext(context.Background(), &CommandRequest{Module: parser.ModuleMysekai, Mode: mode, Region: "jp"}, app)
		if _, err := executeResolvedMysekaiMode(rc, renderCtx); err == nil {
			t.Errorf("executeResolvedMysekaiMode(%q) unexpectedly succeeded", mode)
		}
	}

	rc := NewRequestContext(context.Background(), &CommandRequest{Module: parser.ModuleMysekai, Mode: "other", Region: "jp"}, app)
	if _, err := executeResolvedMysekaiMode(rc, renderCtx); err == nil {
		t.Fatal("unsupported MySekai mode unexpectedly succeeded")
	}
	if _, handled, err := executeStaticMysekaiMode(rc, "jp"); handled || err != nil {
		t.Fatalf("static unsupported mode = handled %t, error %v", handled, err)
	}
	if got := defaultMysekaiQueryRegion("en", "jp"); got != "en" {
		t.Fatalf("explicit query region = %q", got)
	}
	if got := defaultMysekaiQueryRegion("", "tw"); got != "tw" {
		t.Fatalf("defaulted query region = %q", got)
	}

	rc.Cmd.Mode = mySekaiResourceCommand
	if err := validateMysekaiSnapshotExpiry(rc, renderCtx); err == nil {
		t.Fatal("snapshot expiry validation unexpectedly succeeded without snapshot data")
	}
	resourceJob := mysekaiResourceMessageJob(rc, renderCtx, rendermysekai.ResourceQuery{Region: "jp"})
	if _, err := resourceJob(context.Background()); err == nil {
		t.Fatal("resource message job unexpectedly succeeded")
	}
	mapJob := mysekaiMapMessageJob(rc, renderCtx, &drawing.MysekaiMsrMapRequest{})
	if _, err := mapJob(context.Background()); err == nil {
		t.Fatal("map message job unexpectedly succeeded")
	}
}
