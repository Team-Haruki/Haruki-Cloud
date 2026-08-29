package handler

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	sekaidb "haruki-cloud/database/sekai"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendersk "haruki-cloud/internal/pjsk/render/sk"
	renderstamp "haruki-cloud/internal/pjsk/render/stamp"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type handlerStampSource struct {
	region renderregion.Value
	stamps []masterdata.Stamp
}

func (s *handlerStampSource) DefaultRegion() renderregion.Value { return s.region }

func (s *handlerStampSource) GetStamps() ([]masterdata.Stamp, error) {
	return append([]masterdata.Stamp(nil), s.stamps...), nil
}

func TestStampParsingAndHandlerBranches(t *testing.T) {
	for _, tt := range []struct {
		args string
		want []int
	}{
		{args: "", want: nil},
		{args: "1 2 3", want: []int{1, 2, 3}},
		{args: "1 bad", want: nil},
		{args: "0", want: nil},
	} {
		if got := parseStampIDs(tt.args); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseStampIDs(%q) = %v", tt.args, got)
		}
	}
	for _, tt := range []struct {
		args      string
		page      int
		remaining string
		ok        bool
	}{
		{args: "mzk page 2", page: 2, remaining: "mzk", ok: true},
		{args: "p 3 mzk", page: 3, remaining: "mzk", ok: true},
		{args: "页 4", page: 4, remaining: "", ok: true},
		{args: "page", ok: false},
		{args: "page bad", ok: false},
		{args: "page 0", ok: false},
		{args: "mzk", ok: false},
	} {
		page, remaining, ok := parseStampPageWithRemaining(tt.args)
		if page != tt.page || remaining != tt.remaining || ok != tt.ok {
			t.Errorf("parseStampPageWithRemaining(%q) = %d %q %v", tt.args, page, remaining, ok)
		}
	}
	for _, value := range []string{"all", "ALL", "全部", "所有"} {
		if !parseStampAll(value) {
			t.Errorf("parseStampAll(%q) = false", value)
		}
	}
	if parseStampAll("one") {
		t.Fatal("unexpected all mode")
	}

	handler := sekaiHandlers{}.StampHandle()
	for _, tt := range []struct {
		args      string
		wantAll   bool
		wantIDs   []int
		wantQuery string
	}{
		{args: "all", wantAll: true},
		{args: "1 2", wantIDs: []int{1, 2}},
		{args: "mzk", wantQuery: "mzk"},
	} {
		request, err := handler.Handle(&PjskHandlerContext{Context: context.Background(), TriggerCmd: "/stamp", ArgText: tt.args})
		if err != nil {
			t.Fatalf("StampHandle(%q) error = %v", tt.args, err)
		}
		var params struct {
			All bool  `json:"all"`
			IDs []int `json:"ids"`
		}
		if len(request.Params) > 0 {
			if err := json.Unmarshal(request.Params, &params); err != nil {
				t.Fatalf("unmarshal stamp params: %v", err)
			}
		}
		if params.All != tt.wantAll || !reflect.DeepEqual(params.IDs, tt.wantIDs) || request.Query != tt.wantQuery {
			t.Errorf("stamp request = %+v params=%+v", request, params)
		}
	}
}

func TestExecuteStampDirectAndGuardPaths(t *testing.T) {
	ctx := context.Background()
	if _, err := executeStamp(&RequestContext{Ctx: ctx, Cmd: &CommandRequest{}, App: &renderapp.App{}}); err == nil {
		t.Fatal("expected missing stamp service error")
	}

	dir := t.TempDir()
	assetPath := filepath.Join(dir, "stamp", "stamp_a", "stamp_a.png")
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		t.Fatalf("mkdir stamp asset: %v", err)
	}
	if err := os.WriteFile(assetPath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write stamp asset: %v", err)
	}
	source := &handlerStampSource{
		region: renderregion.JP,
		stamps: []masterdata.Stamp{{ID: 1, AssetBundleName: "stamp_a"}},
	}
	controller := renderstamp.NewController(source, nil, assets.NewAssetHelper(dir, nil))
	app := &renderapp.App{
		Stamps: controller,
		Config: renderapp.Config{AssetsBaseURL: "https://cdn.example"},
	}
	params, _ := json.Marshal(renderstamp.ListQuery{IDs: []int{1}})
	rc := &RequestContext{Ctx: ctx, Cmd: &CommandRequest{Mode: "stamp-list", Region: "jp", Params: params}, App: app}
	message, err := executeStamp(rc)
	if err != nil || len(message) != 1 || message[0].Type != onebot11.TypeImage {
		t.Fatalf("direct stamp = %+v, err = %v", message, err)
	}

	for _, query := range []renderstamp.ListQuery{{All: true}, {IDs: []int{1, 2}}} {
		params, _ := json.Marshal(query)
		rc.Cmd.Params = params
		if _, err := executeStamp(rc); err == nil || !strings.Contains(err.Error(), "drawing client") {
			t.Fatalf("expected drawing guard for %+v, got %v", query, err)
		}
	}
	rc.Cmd.Mode = "unknown"
	if _, err := executeStamp(rc); err == nil || !strings.Contains(err.Error(), "unsupported stamp mode") {
		t.Fatalf("unexpected unsupported-mode error: %v", err)
	}

	if _, ok, err := resolveDirectStampImage(ctx, nil, app, renderstamp.ListQuery{}); ok || err != nil {
		t.Fatalf("nil direct resolver = %v %v", ok, err)
	}
	if _, ok, err := resolveDirectStampImage(ctx, controller, &renderapp.App{}, renderstamp.ListQuery{IDs: []int{1}}); ok || err != nil {
		t.Fatalf("empty base URL direct resolver = %v %v", ok, err)
	}
	if _, ok, err := resolveDirectStampImage(ctx, controller, app, renderstamp.ListQuery{All: true, IDs: []int{1}}); ok || err != nil {
		t.Fatalf("all direct resolver = %v %v", ok, err)
	}
	if _, ok, err := resolveDirectStampImage(ctx, controller, app, renderstamp.ListQuery{IDs: []int{99}}); !ok || err == nil {
		t.Fatalf("missing direct stamp = %v %v", ok, err)
	}
}

func TestSKModeDispatchWithoutExternalServices(t *testing.T) {
	controller := rendersk.NewController(nil)
	rc := &RequestContext{
		Ctx:       context.Background(),
		Cmd:       &CommandRequest{Region: "jp"},
		App:       &renderapp.App{},
		RegionStr: "jp",
	}
	for _, mode := range []string{
		"sk-line", "sk-query", "sk-check-room", "sk-csb", "sk-speed", "sk-daily-speed",
		"sk-player-trace", "sk-rank-trace", "sk-predict", "sk-winrate",
	} {
		rc.Cmd.Mode = mode
		if _, err := executeSKMode(rc, controller); err == nil {
			t.Errorf("executeSKMode(%q) unexpectedly succeeded", mode)
		}
	}
	rc.Cmd.Mode = "unknown"
	if _, err := executeSKMode(rc, controller); err == nil || !strings.Contains(err.Error(), "unsupported sk mode") {
		t.Fatalf("unsupported SK mode error = %v", err)
	}

	if result, err := skImageResult([]byte("image"), nil); err != nil || string(result.image) != "image" {
		t.Fatalf("skImageResult success = %+v, %v", result, err)
	}
	wantErr := errors.New("failed")
	if _, err := skImageResult(nil, wantErr); err != wantErr {
		t.Fatalf("skImageResult error = %v", err)
	}

	if _, err := executeSK(nil); err == nil {
		t.Fatal("expected nil SK context error")
	}
	rc.App = &renderapp.App{SK: controller}
	rc.Cmd.Mode = "sk-line"
	if _, err := executeSK(rc); err == nil {
		t.Fatal("expected SK drawing error")
	}
}

func TestSKTrackerQueryHelperBranches(t *testing.T) {
	if _, ok := trackerRankQueryFromParams(nil); ok {
		t.Fatal("nil request unexpectedly decoded")
	}
	if _, ok := trackerRankQueryFromParams(&CommandRequest{Params: []byte("{")}); ok {
		t.Fatal("invalid JSON unexpectedly decoded")
	}
	empty, _ := json.Marshal(rendersk.TrackerRankQuery{})
	if _, ok := trackerRankQueryFromParams(&CommandRequest{Params: empty, Region: "jp"}); ok {
		t.Fatal("empty tracker query unexpectedly accepted")
	}
	params, _ := json.Marshal(rendersk.TrackerRankQuery{Ranks: []int{100}, Region: "cn"})
	query, ok := trackerRankQueryFromParams(&CommandRequest{Params: params, Region: "jp", RegionExplicit: true})
	if !ok || query.Region != "jp" || !query.RegionExplicit {
		t.Fatalf("explicit tracker query = %+v %v", query, ok)
	}
	params, _ = json.Marshal(rendersk.TrackerRankQuery{Ranks: []int{100}, RegionExplicit: true, Region: "tw"})
	query, ok = trackerRankQueryFromParams(&CommandRequest{Params: params, Region: "jp"})
	if !ok || query.Region != "tw" || !query.RegionExplicit {
		t.Fatalf("embedded explicit tracker query = %+v %v", query, ok)
	}

	requester := &RequestContext{Cmd: &CommandRequest{RequesterPlatform: "qq", RequesterUserID: "1"}}
	if isSKSelfTrackerQuery(nil, rendersk.TrackerRankQuery{}) || isSKSelfTrackerQuery(&RequestContext{}, rendersk.TrackerRankQuery{}) {
		t.Fatal("nil command classified as self")
	}
	if !isSKSelfTrackerQuery(requester, rendersk.TrackerRankQuery{TargetPlatform: "QQ", TargetUserID: "1"}) {
		t.Fatal("matching explicit target not classified as self")
	}
	if isSKSelfTrackerQuery(requester, rendersk.TrackerRankQuery{TargetPlatform: "qq", TargetUserID: "2"}) {
		t.Fatal("different target classified as self")
	}
	uid := int64(10)
	if isSKSelfTrackerQuery(requester, rendersk.TrackerRankQuery{UserID: &uid}) || isSKSelfTrackerQuery(requester, rendersk.TrackerRankQuery{Ranks: []int{1}}) {
		t.Fatal("resolved tracker target classified as self")
	}
	if !isSKSelfTrackerQuery(requester, rendersk.TrackerRankQuery{}) {
		t.Fatal("implicit requester not classified as self")
	}

	original := errors.New("other")
	if normalizeSKSelfRankingNotFoundError(false, "jp", sekaiapi.ErrRankingNotFound) != sekaiapi.ErrRankingNotFound || normalizeSKSelfRankingNotFoundError(true, "jp", nil) != nil || normalizeSKSelfRankingNotFoundError(true, "jp", original) != original {
		t.Fatal("non-matching self ranking error changed")
	}
	assertReplayErrorText(t, normalizeSKSelfRankingNotFoundError(true, "jp", sekaiapi.ErrRankingNotFound), "当前JP服活动没有找到你的排行榜数据")

	chapter := &sekaidb.Worldbloom{ChapterStartAt: 100, AggregateAt: 200}
	request := &rendersk.TrackerRankQuery{}
	applyTrackerWorldBloomChapterTiming(nil, chapter)
	applyTrackerWorldBloomChapterTiming(request, nil)
	applyTrackerWorldBloomChapterTiming(request, chapter)
	if request.EventStartAt == nil || *request.EventStartAt != 100 || request.EventAggregateAt == nil || *request.EventAggregateAt != 200 {
		t.Fatalf("chapter timing = %+v", request)
	}
	invalidID := 0
	request = &rendersk.TrackerRankQuery{WlCharacterID: &invalidID}
	if err := resolveTrackerCharacterSelection(context.Background(), nil, request); err != nil || request.WlCharacterID != nil {
		t.Fatalf("invalid character selection = %+v, %v", request, err)
	}
	request = &rendersk.TrackerRankQuery{WlCharacterQuery: "miku"}
	if err := resolveTrackerCharacterSelection(context.Background(), nil, request); err == nil {
		t.Fatal("expected missing world-bloom provider error")
	}
	if err := prepareTrackerRankQuery(context.Background(), nil, nil, "qq", "1"); err != nil {
		t.Fatalf("nil tracker request prepare = %v", err)
	}

	if err := resolveTrackerTargetUser(context.Background(), nil, nil, "qq", "1"); err != nil {
		t.Fatalf("nil target request = %v", err)
	}
	request = &rendersk.TrackerRankQuery{UserID: &uid}
	if err := resolveTrackerTargetUser(context.Background(), nil, request, "qq", "1"); err != nil {
		t.Fatalf("resolved target request = %v", err)
	}
	request = &rendersk.TrackerRankQuery{TargetPlatform: "qq"}
	if err := resolveTrackerTargetUser(context.Background(), nil, request, "qq", "1"); err != nil {
		t.Fatalf("partial target request = %v", err)
	}
	request = &rendersk.TrackerRankQuery{TargetPlatform: "qq", TargetUserID: "2"}
	if err := resolveTrackerTargetUser(context.Background(), nil, request, "qq", "1"); !errors.Is(err, accountdata.ErrBindingServiceUnavailable) {
		t.Fatalf("missing binding service error = %v", err)
	}
	if normalizeTrackerRegion("") != DefaultRegionStr || normalizeTrackerRegion("CN") != "cn" {
		t.Fatal("tracker region normalization mismatch")
	}

}

func TestExecutionRuntimeAndCommandDispatch(t *testing.T) {
	ctx := context.Background()
	app := &renderapp.App{}
	if _, _, err := PrepareExecutionRuntime(ctx, nil, app); err == nil {
		t.Fatal("expected nil command preflight error")
	}
	if _, _, err := PrepareExecutionRuntime(ctx, &CommandRequest{}, nil); err == nil {
		t.Fatal("expected nil app preflight error")
	}
	resolved := &CommandRequest{Module: parser.ModuleMusic, Mode: "test", Region: "jp"}
	runtime, shortCircuit, err := PrepareExecutionRuntime(ctx, resolved, app)
	if err != nil || shortCircuit != nil || runtime == nil || runtime.Request == nil || runtime.Resolved != resolved {
		t.Fatalf("runtime = %+v, shortCircuit = %+v, err = %v", runtime, shortCircuit, err)
	}

	executor := wrapRequestExecutor(func(rc *RequestContext) (onebot11.Message, error) {
		return onebot11.Message{onebot11.Text(rc.Cmd.Mode)}, nil
	})
	if _, err := executor(nil); err == nil {
		t.Fatal("expected nil wrapped runtime error")
	}
	message, err := executor(runtime)
	if err != nil || len(message) != 1 {
		t.Fatalf("wrapped executor = %+v, %v", message, err)
	}

	if _, err := ExecuteCommandRequest(ctx, nil, app); err == nil {
		t.Fatal("expected nil command dispatch error")
	}
	help := &CommandRequest{IsHelp: true, CommandPath: "generic"}
	message, err = ExecuteCommandRequest(ctx, help, nil)
	if err != nil || len(message) != 1 || message[0].Type != onebot11.TypeText {
		t.Fatalf("help dispatch = %+v, %v", message, err)
	}
	if _, err := ExecuteCommandRequest(ctx, resolved, app); err == nil || !strings.Contains(err.Error(), "not bound") {
		t.Fatalf("unbound dispatch error = %v", err)
	}
	resolved.executor = func(*ExecutionRuntime) (onebot11.Message, error) {
		return onebot11.Message{onebot11.Text("ok")}, nil
	}
	message, err = ExecuteCommandRequest(ctx, resolved, app)
	if err != nil || len(message) != 1 {
		t.Fatalf("successful dispatch = %+v, %v", message, err)
	}
	resolved.executor = func(*ExecutionRuntime) (onebot11.Message, error) {
		return nil, accountdata.ErrNoBinding
	}
	if _, err := ExecuteCommandRequest(ctx, resolved, app); err == nil {
		t.Fatal("expected normalized domain error")
	} else if _, ok := errors.AsType[onebot11.ReplayError](err); !ok {
		t.Fatalf("domain error was not normalized: %T %v", err, err)
	}
}
