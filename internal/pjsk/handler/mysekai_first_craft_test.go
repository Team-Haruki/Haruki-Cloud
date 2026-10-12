package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/i18n"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	renderprofile "haruki-cloud/internal/pjsk/render/profile"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/imagecache"
)

func firstCraftHandleParams(t *testing.T, h HarukiSekaiCommandHandler, args string) (*CommandRequest, map[string]any) {
	t.Helper()
	h.Regions = []renderregion.Value{renderregion.JP}
	result, err := h.Handle(&PjskHandlerContext{Context: context.Background(), TriggerCmd: "/test", ArgText: args, Platform: "qq", UserId: "42"})
	if err != nil {
		t.Fatalf("Handle(%q): %v", args, err)
	}
	params := map[string]any{}
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	return result, params
}

func TestMysekaiFirstCraftHandleParsesCategoryAndAll(t *testing.T) {
	h := sekaiHandlers{}.MysekaiFirstCraftHandle()
	cases := []struct {
		args     string
		showAll  bool
		category string
	}{
		{"", false, ""},
		{"全部", true, ""},
		{"all 椅子", true, "椅子"},
		{"画布 full", true, "画布"},
		{"chair", false, "chair"},
	}
	for _, tc := range cases {
		result, params := firstCraftHandleParams(t, h, tc.args)
		if result.Module != parser.ModuleMysekai || result.Mode != mySekaiFirstCraftCommand {
			t.Fatalf("%q: unexpected request %+v", tc.args, result)
		}
		if got, _ := params["show_all"].(bool); got != tc.showAll {
			t.Errorf("%q: show_all = %v, want %v", tc.args, got, tc.showAll)
		}
		if got, _ := params["category_query"].(string); got != tc.category {
			t.Errorf("%q: category_query = %q, want %q", tc.args, got, tc.category)
		}
		if params["mode"] != "self" {
			t.Errorf("%q: expected a self query, got %v", tc.args, params)
		}
	}
}

func TestMysekaiFixtureListHandleAcceptsACategory(t *testing.T) {
	h := sekaiHandlers{}.MysekaiFixtureListHandle()
	cases := []struct {
		args          string
		onlyCraftable bool
		category      string
	}{
		{"", false, ""},
		{"craft 椅子", true, "椅子"},
		{"full noid", false, ""},
		{"画布 noid", false, "画布"},
	}
	for _, tc := range cases {
		_, params := firstCraftHandleParams(t, h, tc.args)
		if got, _ := params["only_craftable"].(bool); got != tc.onlyCraftable {
			t.Errorf("%q: only_craftable = %v, want %v", tc.args, got, tc.onlyCraftable)
		}
		if got, _ := params["category_query"].(string); got != tc.category {
			t.Errorf("%q: category_query = %q, want %q", tc.args, got, tc.category)
		}
	}
}

// firstCraftHandlerFixture writes a small master and returns its directory:
// fixture 2001 (blueprint owned, never held), 2002 (held at quantity 0) and
// 2003 (never held, blueprint missing).
func firstCraftHandlerFixture(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "masterdata")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	write := func(name string, data any) {
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	fixture := func(id int) map[string]any {
		return map[string]any{
			"id": id, "name": "Wood Chair", "assetbundleName": "wood_chair", "mysekaiFixtureType": "normal",
			"mysekaiFixtureMainGenreId": 2, "mysekaiFixtureSubGenreId": 4,
			"gridSize": map[string]any{"width": 1, "depth": 1, "height": 1},
		}
	}
	write("mysekaiFixtures.json", []map[string]any{fixture(2001), fixture(2002), fixture(2003)})
	write("mysekaiFixtureMainGenres.json", []map[string]any{{"id": 2, "name": "一般", "assetbundleName": "icon_normal_furniture"}})
	write("mysekaiFixtureSubGenres.json", []map[string]any{{"id": 4, "name": "チェア", "assetbundleName": "icon_chair"}})
	write("mysekaiBlueprints.json", []map[string]any{
		{"id": 1001, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 2001},
		{"id": 1002, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 2002},
		{"id": 1003, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 2003},
	})
	write("mysekaiRankObtainedExps.json", []map[string]any{
		{"id": 1, "mysekaiRankObtainedExpType": "craft_mysekai_fixture_first_bonus", "quantity": 1000},
	})
	for _, name := range []string{
		"gameCharacters.json", "mysekaiBlueprintMysekaiMaterialCosts.json", "mysekaiFixtureOnlyDisassembleMaterials.json",
		"mysekaiFixtureTags.json", "mysekaiMaterials.json",
	} {
		write(name, []map[string]any{})
	}
	return dir
}

func firstCraftHandlerPayload(t *testing.T) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"upload_time": time.Now().UnixMilli(),
		"updatedResources": map[string]any{
			"userMysekaiGamedata":   map[string]any{"mysekaiRank": 9},
			"userMysekaiFixtures":   []map[string]any{{"mysekaiFixtureId": 2002, "textureId": 1, "quantity": 0}},
			"userMysekaiCanvases":   []map[string]any{},
			"userMysekaiBlueprints": []map[string]any{{"mysekaiBlueprintId": 1001}, {"mysekaiBlueprintId": 1002}},
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}

func TestExecuteMySekaiFirstCraftRepliesWithSummaryAndImage(t *testing.T) {
	masterdataDir := firstCraftHandlerFixture(t)
	var drawn *drawing.MysekaiFixtureListRequest
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/mysekai/fixture-list" {
			t.Errorf("unexpected drawing path: %s", r.URL.Path)
		}
		drawn = &drawing.MysekaiFixtureListRequest{}
		if err := json.NewDecoder(r.Body).Decode(drawn); err != nil {
			t.Errorf("decode drawing request: %v", err)
		}
		_, _ = w.Write([]byte("first-craft"))
	}))
	defer drawingServer.Close()
	sekaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := json.Marshal(&sekaiapi.GetAnotherProfileResponse{
			User:        sekaiapi.AnotherUser{UserID: 12345678901234, Name: "JPUser", Rank: 100},
			UserProfile: sekaiapi.UserProfile{ProfileImageType: "default"},
			UserDeck:    sekaiapi.UserDeck{DeckID: 1, Leader: 1001, Member1: 1001},
			UserCards:   []sekaiapi.AnotherUserCard{{CardID: 1001, Level: 60, DefaultImage: "special_training"}},
		})
		_, _ = w.Write(raw)
	}))
	defer sekaiServer.Close()

	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(context.Background(), "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	app := &renderapp.App{
		Config:          renderapp.Config{UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true}},
		Bindings:        service,
		MySekai:         rendermysekai.NewController(drawing.NewHarukiDrawingClient(drawingServer.URL), nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true}),
		Profiles:        renderprofile.NewController(&handlerMySekaiProfileSource{region: renderregion.JP, cards: map[int]*masterdata.Card{1001: {ID: 1001, CharacterID: 1, AssetBundleName: "res001_no001"}}}, nil, assets.NewAssetHelper("", nil), nil),
		SekaiAPI:        sekaiapi.NewSekaiAPIClient(&harukiConfig.SekaiAPIConfig{BaseURL: sekaiServer.URL}),
		Snapshots:       unavailableSnapshotProvider{},
		MySekaiPayloads: &recordingMySekaiPayloadProvider{payload: firstCraftHandlerPayload(t)},
		ImageCache:      imagecache.New("https://image-cache.test", t.TempDir()),
	}

	message, err := executeMysekai(NewRequestContext(context.Background(), &CommandRequest{
		Module:            parser.ModuleMysekai,
		Mode:              mySekaiFirstCraftCommand,
		Region:            "jp",
		Params:            []byte(`{"show_all":true}`),
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, app))
	if err != nil {
		t.Fatalf("executeMysekai first craft: %v", err)
	}
	if len(message) != 2 || message[0].Type != onebot11.TypeText || message[1].Type != onebot11.TypeImage {
		t.Fatalf("expected the summary text and the image, got %+v", message)
	}
	want := i18n.LinesText(rendermysekai.FirstCraftSummaryLines(rendermysekai.FirstCraftSummary{BonusExp: 1000, Craftable: 1, NotOwned: 1, ShowAll: true}))
	if got := message[0].Data.(onebot11.TextData).Text; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if drawn == nil {
		t.Fatal("the image was not drawn")
	}
	tiles := map[int]bool{}
	for _, genre := range drawn.MainGenres {
		for _, sub := range genre.SubGenres {
			for _, fixture := range sub.Fixtures {
				tiles[fixture.ID] = fixture.Obtained
			}
		}
	}
	if !reflect.DeepEqual(tiles, map[int]bool{2001: true, 2003: false}) {
		t.Fatalf("tiles = %v, want 2001 bright and 2003 grey", tiles)
	}
}

func TestExecuteMySekaiFixtureDetailTagsTheFirstCraftForABoundUser(t *testing.T) {
	masterdataDir := firstCraftHandlerFixture(t)
	var basicInfo []string
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqs []drawing.MysekaiFixtureDetailRequest
		if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil || len(reqs) != 1 {
			t.Errorf("decode drawing request: %v %+v", err, reqs)
			return
		}
		basicInfo = reqs[0].BasicInfo
		_, _ = w.Write([]byte("fixture-detail"))
	}))
	defer drawingServer.Close()

	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(context.Background(), "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	payloads := &recordingMySekaiPayloadProvider{payload: firstCraftHandlerPayload(t)}
	app := &renderapp.App{
		Config:          renderapp.Config{UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true}},
		Bindings:        service,
		MySekai:         rendermysekai.NewController(drawing.NewHarukiDrawingClient(drawingServer.URL), nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true}),
		Snapshots:       unavailableSnapshotProvider{},
		MySekaiPayloads: payloads,
		ImageCache:      imagecache.New("https://image-cache.test", t.TempDir()),
	}
	detail := func(platformUserID, id string) []string {
		t.Helper()
		basicInfo = nil
		message, err := executeMysekai(NewRequestContext(context.Background(), &CommandRequest{
			Module:            parser.ModuleMysekai,
			Mode:              mySekaiFixtureDetailCommand,
			Region:            "jp",
			Query:             id,
			RequesterPlatform: "qq",
			RequesterUserID:   platformUserID,
		}, app))
		if err != nil || len(message) != 1 || message[0].Type != onebot11.TypeImage {
			t.Fatalf("fixture detail %s: %v %+v", id, err, message)
		}
		return basicInfo
	}

	if info := detail("42", "2001"); len(info) == 0 || info[len(info)-1] != i18n.T("mysekai.image.fixture.first_craft_available", i18n.Data{"Exp": 1000}) {
		t.Fatalf("bound user, never held: %v", info)
	}
	if info := detail("42", "2002"); len(info) == 0 || info[len(info)-1] != i18n.T("mysekai.image.fixture.first_craft_used") {
		t.Fatalf("bound user, held at quantity 0: %v", info)
	}
	if info := detail("someone-else", "2001"); strings.Contains(strings.Join(info, ""), "首次制作") {
		t.Fatalf("an unbound user gets the static detail: %v", info)
	}
}
