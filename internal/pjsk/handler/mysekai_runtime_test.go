package handler

import (
	"context"
	"errors"
	"fmt"
	json "github.com/bytedance/sonic"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	renderprofile "haruki-cloud/internal/pjsk/render/profile"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/imagecache"
)

type unavailableSnapshotProvider struct{}

func (unavailableSnapshotProvider) Resolve(context.Context, rendersnapshot.Selector, rendersnapshot.ResolveOptions) (rendersnapshot.Snapshot, error) {
	return nil, rendersnapshot.ErrSnapshotUnavailable
}

type fixedMySekaiPayloadProvider struct {
	payload []byte
}

func (p fixedMySekaiPayloadProvider) Resolve(context.Context, rendersnapshot.Selector, bool) ([]byte, error) {
	if len(p.payload) == 0 {
		return nil, errors.New("payload is unavailable")
	}
	return append([]byte(nil), p.payload...), nil
}

type handlerMySekaiProfileSource struct {
	region renderregion.Value
	cards  map[int]*masterdata.Card
}

func (s *handlerMySekaiProfileSource) DefaultRegion() renderregion.Value { return s.region }

func (s *handlerMySekaiProfileSource) GetHonorByID(id int) (*masterdata.Honor, error) {
	return nil, fmt.Errorf("honor not found: %d", id)
}

func (s *handlerMySekaiProfileSource) GetHonorGroupByID(id int) (*masterdata.HonorGroup, error) {
	return nil, fmt.Errorf("honor group not found: %d", id)
}

func (s *handlerMySekaiProfileSource) GetBondsHonorByID(id int) (*masterdata.BondsHonor, error) {
	return nil, fmt.Errorf("bonds honor not found: %d", id)
}

func (s *handlerMySekaiProfileSource) GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool) {
	return nil, false
}

func (s *handlerMySekaiProfileSource) GetPlayerFrameByID(id int) (*masterdata.PlayerFrame, error) {
	return nil, fmt.Errorf("player frame not found: %d", id)
}

func (s *handlerMySekaiProfileSource) GetPlayerFrameGroupByID(id int) (*masterdata.PlayerFrameGroup, error) {
	return nil, fmt.Errorf("player frame group not found: %d", id)
}

func (s *handlerMySekaiProfileSource) GetCardByID(id int) (*masterdata.Card, error) {
	item, ok := s.cards[id]
	if !ok {
		return nil, fmt.Errorf("card not found: %d", id)
	}
	return new(*item), nil
}

func (s *handlerMySekaiProfileSource) GetEventIDByHonorID(honorID int) int {
	return 0
}

func TestResolveMySekaiRenderContextFallsBackToPayloadProvider(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)

	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	controller := rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true})
	app := &renderapp.App{
		Bindings:        service,
		MySekai:         controller,
		Snapshots:       unavailableSnapshotProvider{},
		MySekaiPayloads: fixedMySekaiPayloadProvider{payload: []byte(`{"updatedResources":{"userMysekaiPhotos":[{"seq":1,"imagePath":"photos/test"}]}}`)},
	}

	result, err := resolveMySekaiRenderContext(ctx, app, userQueryParams{
		Mode:           "self",
		Platform:       "qq",
		PlatformUserID: "42",
	}, "jp", false)
	if err != nil {
		t.Fatalf("resolveMySekaiRenderContext() error = %v", err)
	}
	if result.Controller == nil {
		t.Fatal("expected controller")
	}

	photo, err := result.Controller.ResolvePhoto(rendermysekai.PhotoQuery{Region: "jp", Seq: 1})
	if err != nil {
		t.Fatalf("ResolvePhoto() error = %v", err)
	}
	if photo == nil || photo.ImagePath != "photos/test" {
		t.Fatalf("unexpected photo result: %+v", photo)
	}
}

func TestResolveMySekaiRenderContextPrefersSnapshotProfileCard(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)

	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := service.SetBindingVisible(ctx, "qq", "42", "jp", true); err != nil {
		t.Fatalf("show id: %v", err)
	}

	controller := rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true})
	snapshot := &runtimeSnapshotStub{
		card: &drawing.ProfileCardRequest{
			Profile: &drawing.BasicProfile{ID: "99999999999999", Region: "TW", Nickname: "snapshot-card"},
			DataSources: []drawing.ProfileDataSource{
				{Name: "Suite数据"},
			},
		},
	}
	app := &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Bindings:  service,
		MySekai:   controller,
		Snapshots: &runtimeSnapshotProviderStub{snapshot: snapshot},
	}

	result, err := resolveMySekaiRenderContext(ctx, app, userQueryParams{
		Mode:           "self",
		Platform:       "qq",
		PlatformUserID: "42",
	}, "jp", false)
	if err != nil {
		t.Fatalf("resolveMySekaiRenderContext() error = %v", err)
	}
	if result.Profile == nil || result.Profile.Profile == nil || result.Profile.Profile.Nickname != "snapshot-card" {
		t.Fatalf("expected snapshot profile card, got %+v", result.Profile)
	}
	if result.Profile.Profile.IsHideUID {
		t.Fatalf("expected visible profile card id state, got %+v", result.Profile.Profile)
	}
	if result.Profile.Profile.ID != "12345678901234" {
		t.Fatalf("expected binding uid to override snapshot profile id, got %q", result.Profile.Profile.ID)
	}
	if result.Profile.Profile.Region != "JP" {
		t.Fatalf("expected normalized profile region JP, got %q", result.Profile.Profile.Region)
	}
	if len(result.Profile.DataSources) == 0 || result.Profile.DataSources[0].Name != "Suite数据" {
		t.Fatalf("expected suite data source, got %+v", result.Profile.DataSources)
	}
}

func TestResolveMySekaiRenderContextPrefersPublicProfileCardWhenAvailable(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)

	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := service.SetBindingVisible(ctx, "qq", "42", "jp", true); err != nil {
		t.Fatalf("show id: %v", err)
	}

	sekaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/jp/12345678901234/profile" {
			t.Fatalf("unexpected sekai api path: %s", r.URL.Path)
		}
		resp := &sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 12345678901234,
				Name:   "JPUser",
				Rank:   100,
			},
			UserProfile: sekaiapi.UserProfile{ProfileImageType: "default"},
			UserDeck:    sekaiapi.UserDeck{DeckID: 1, Leader: 1001, Member1: 1001},
			UserCards: []sekaiapi.AnotherUserCard{
				{CardID: 1001, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "special_training"},
			},
		}
		raw, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal sekai api response: %v", err)
		}
		_, _ = w.Write(raw)
	}))
	defer sekaiServer.Close()

	controller := rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true})
	snapshot := &runtimeSnapshotStub{
		card: &drawing.ProfileCardRequest{
			Profile: &drawing.BasicProfile{
				ID:              "99999999999999",
				Region:          "TW",
				Nickname:        "snapshot-card",
				LeaderImagePath: "asset/user/snapshot.png",
			},
			DataSources: []drawing.ProfileDataSource{
				{Name: "Suite数据"},
			},
		},
	}
	profileSource := &handlerMySekaiProfileSource{
		region: renderregion.JP,
		cards: map[int]*masterdata.Card{
			1001: {ID: 1001, CharacterID: 1, AssetBundleName: "res001_no001"},
		},
	}
	app := &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Bindings:  service,
		MySekai:   controller,
		Snapshots: &runtimeSnapshotProviderStub{snapshot: snapshot},
		Profiles:  renderprofile.NewController(profileSource, nil, assets.NewAssetHelper("", nil), nil),
		SekaiAPI:  sekaiapi.NewSekaiAPIClient(&harukiConfig.SekaiAPIConfig{BaseURL: sekaiServer.URL}),
	}

	result, err := resolveMySekaiRenderContext(ctx, app, userQueryParams{
		Mode:           "self",
		Platform:       "qq",
		PlatformUserID: "42",
	}, "jp", false)
	if err != nil {
		t.Fatalf("resolveMySekaiRenderContext() error = %v", err)
	}
	if result.Profile == nil || result.Profile.Profile == nil {
		t.Fatalf("expected public profile card, got %+v", result.Profile)
	}
	if result.Profile.Profile.Nickname != "JPUser" {
		t.Fatalf("expected public profile nickname, got %+v", result.Profile.Profile)
	}
	if got := result.Profile.Profile.LeaderImagePath; got == "asset/user/snapshot.png" || !strings.Contains(got, "res001_no001_after_training.png") {
		t.Fatalf("expected public leader art instead of snapshot leader art, got %q", got)
	}
	if result.Profile.Profile.ID != "12345678901234" {
		t.Fatalf("expected binding uid to override public profile id, got %q", result.Profile.Profile.ID)
	}
	if result.Profile.Profile.Region != "JP" {
		t.Fatalf("expected normalized profile region JP, got %q", result.Profile.Profile.Region)
	}
	if len(result.Profile.DataSources) == 0 || result.Profile.DataSources[0].Name != "Suite数据" {
		t.Fatalf("expected suite data source metadata to remain, got %+v", result.Profile.DataSources)
	}
}

func TestResolveMySekaiRenderContextUsesSelectedBindingRegion(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingServiceWithValidator(t, handlerMultiRegionBindingValidator{
		profiles: map[string]map[string]string{
			"cn": {
				"11111111111111": "CN User 1",
			},
			"jp": {
				"33333333333333": "JP User 1",
			},
		},
	})

	if _, err := service.Bind(ctx, "qq", "42", "11111111111111"); err != nil {
		t.Fatalf("bind cn: %v", err)
	}
	if _, err := service.Bind(ctx, "qq", "42", "33333333333333"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}

	controller := rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true})
	provider := &runtimeSnapshotProviderStub{
		snapshot: &runtimeSnapshotStub{
			card: &drawing.ProfileCardRequest{
				Profile: &drawing.BasicProfile{ID: "99999999999999", Region: "JP", Nickname: "selector-card"},
			},
		},
	}
	app := &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Bindings:  service,
		MySekai:   controller,
		Snapshots: provider,
	}

	result, err := resolveMySekaiRenderContext(ctx, app, userQueryParams{
		Mode:           "self",
		Platform:       "qq",
		PlatformUserID: "42",
		Selector:       "u1",
	}, "jp", false)
	if err != nil {
		t.Fatalf("resolveMySekaiRenderContext() error = %v", err)
	}
	if result.Region != "cn" {
		t.Fatalf("expected resolved region cn, got %q", result.Region)
	}
	if result.Profile == nil || result.Profile.Profile == nil || result.Profile.Profile.Nickname != "selector-card" {
		t.Fatalf("unexpected profile card: %+v", result.Profile)
	}
	if result.Profile.Profile.ID != "11111111111111" {
		t.Fatalf("expected selected binding uid to override snapshot profile id, got %q", result.Profile.Profile.ID)
	}
	if result.Profile.Profile.Region != "CN" {
		t.Fatalf("expected selected binding region CN, got %q", result.Profile.Profile.Region)
	}
	if len(provider.selectors) != 1 {
		t.Fatalf("expected one snapshot selector, got %d", len(provider.selectors))
	}
	if provider.selectors[0].Region != renderregion.CN {
		t.Fatalf("expected cn selector region, got %+v", provider.selectors[0].Region)
	}
}

func TestExecuteMySekaiRequiresController(t *testing.T) {
	_, err := executeMysekai(NewRequestContext(context.Background(), &CommandRequest{
		Module: parser.ModuleMysekai,
		Mode:   "mysekai-photo",
		Region: "jp",
	}, &renderapp.App{}))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "烤森服务未就绪") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteMySekaiMissingSnapshotUsesStandardReplayError(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	params, err := json.Marshal(rendermysekai.PhotoQuery{Seq: 1})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	_, err = executeMysekai(NewRequestContext(ctx, &CommandRequest{
		Module:            parser.ModuleMysekai,
		Mode:              "mysekai-photo",
		Region:            "jp",
		Params:            params,
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Bindings: service,
		MySekai:  rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true}),
	}))

	var replyErr onebot11.ReplayError
	if !errors.As(WrapDomainError(err), &replyErr) {
		t.Fatalf("expected ReplayError, got %T (%v)", err, err)
	}
	if string(replyErr) != buildPrivateDataNotFoundMessage("mysekai", &accountdata.ResolvedBinding{
		Server:     "jp",
		PJSKUserID: "12345678901234",
		Visible:    false,
	}) {
		t.Fatalf("unexpected replay error: %q", replyErr)
	}
}

func TestExecuteMySekaiReturnsBindingErrorBeforeDataMessage(t *testing.T) {
	_, err := executeMysekai(NewRequestContext(context.Background(), &CommandRequest{
		Module:            parser.ModuleMysekai,
		Mode:              "mysekai-photo",
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Bindings: newHandlerTestBindingService(t),
		MySekai:  rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true}),
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	var replyErr onebot11.ReplayError
	if !errors.As(WrapDomainError(err), &replyErr) {
		t.Fatalf("expected ReplayError, got %T (%v)", err, err)
	}
	if string(replyErr) != ErrMsgBindingNotFound {
		t.Fatalf("unexpected replay error: %q", replyErr)
	}
}

func TestExecuteMySekaiBlocksCNRegion(t *testing.T) {
	original := harukiConfig.Cfg.PJSK.AllowCNMySekai
	harukiConfig.Cfg.PJSK.AllowCNMySekai = nil
	t.Cleanup(func() {
		harukiConfig.Cfg.PJSK.AllowCNMySekai = original
	})

	message, err := executeMysekai(NewRequestContext(context.Background(), &CommandRequest{
		Module: parser.ModuleMysekai,
		Mode:   "mysekai-photo",
		Region: "cn",
	}, &renderapp.App{
		MySekai: rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true}),
	}))
	if err != nil {
		t.Fatalf("executeMysekai() error = %v", err)
	}
	assertSingleMySekaiUnavailableMessage(t, message)
}

func TestExecuteMySekaiMapPrependsExpiredNotice(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}

	writeJSON := func(name string, data any) {
		t.Helper()
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(masterdataDir, name), raw, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	writeJSON("mysekaiSiteHarvestFixtures.json", []map[string]any{
		{
			"id":                                  1001,
			"assetbundleName":                     "mdl_site_wood_common_fieldtree01",
			"mysekaiSiteHarvestFixtureType":       "wood",
			"mysekaiSiteHarvestFixtureRarityType": "rarity_1",
		},
	})
	writeJSON("mysekaiMaterials.json", []map[string]any{})
	writeJSON("mysekaiItems.json", []map[string]any{})
	writeJSON("mysekaiFixtures.json", []map[string]any{})
	writeJSON("mysekaiMusicRecords.json", []map[string]any{})
	writeJSON("gameCharacters.json", []map[string]any{})

	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/mysekai/map" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.MysekaiMsrMapRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if len(req.Maps) != 1 {
			t.Fatalf("expected 1 map in drawing request, got %+v", req.Maps)
		}
		_, _ = w.Write([]byte("mysekai-map"))
	}))
	defer drawingServer.Close()

	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(context.Background(), "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	staleSnapshot, err := json.Marshal(map[string]any{
		"upload_time": time.Now().Add(-48 * time.Hour).UnixMilli(),
		"userMysekaiHarvestMaps": []map[string]any{{
			"mysekaiSiteId": 5,
			"userMysekaiSiteHarvestFixtures": []map[string]any{{
				"mysekaiSiteHarvestFixtureId":         1001,
				"userMysekaiSiteHarvestFixtureStatus": "spawned",
				"positionX":                           0,
				"positionZ":                           0,
			}},
			"userMysekaiSiteHarvestResourceDrops": []map[string]any{},
		}},
	})
	if err != nil {
		t.Fatalf("marshal stale snapshot: %v", err)
	}

	app := &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Bindings: service,
		MySekai:  rendermysekai.NewController(drawing.NewHarukiDrawingClient(drawingServer.URL), nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true}),
		Snapshots: &runtimeSnapshotProviderStub{
			snapshot: &runtimeSnapshotStub{rawBytes: staleSnapshot},
		},
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	message, err := executeMysekai(NewRequestContext(context.Background(), &CommandRequest{
		Module:            parser.ModuleMysekai,
		Mode:              "mysekai-map",
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, app))
	if err != nil {
		t.Fatalf("executeMysekai map: %v", err)
	}
	if len(message) != 3 {
		t.Fatalf("unexpected message segments: %+v", message)
	}
	if message[0].Type != onebot11.TypeText {
		t.Fatalf("expected warning text first, got %+v", message[0])
	}
	textData, ok := message[0].Data.(onebot11.TextData)
	if !ok || !strings.Contains(textData.Text, "Mysekai数据已过期要刷新请重新上传数据") {
		t.Fatalf("unexpected warning text: %+v", message[0].Data)
	}
	if message[1].Type != onebot11.TypeImage {
		t.Fatalf("expected image after warning, got %+v", message[1])
	}
	if message[2].Type != onebot11.TypeAt {
		t.Fatalf("expected at mention after image, got %+v", message[2])
	}
}

func TestExecuteMySekaiFixtureListStaticSkipsBindingAndSnapshot(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}

	writeJSON := func(name string, data any) {
		t.Helper()
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(masterdataDir, name), raw, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	writeJSON("mysekaiFixtures.json", []map[string]any{
		{
			"id":                        2001,
			"name":                      "Wood Chair",
			"assetbundleName":           "wood_chair",
			"mysekaiFixtureType":        "furniture",
			"mysekaiFixtureMainGenreId": 1,
			"mysekaiFixtureSubGenreId":  11,
		},
	})
	writeJSON("mysekaiFixtureMainGenres.json", []map[string]any{
		{"id": 1, "name": "Main A", "assetbundleName": "main_a"},
	})
	writeJSON("mysekaiFixtureSubGenres.json", []map[string]any{
		{"id": 11, "name": "Sub A", "assetbundleName": "sub_a"},
	})
	writeJSON("mysekaiBlueprints.json", []map[string]any{
		{"id": 1001, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 2001},
	})
	writeJSON("gameCharacters.json", []map[string]any{})

	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/mysekai/fixture-list" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.MysekaiFixtureListRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.Profile != nil {
			t.Fatalf("expected static fixture list to skip profile, got %+v", req.Profile)
		}
		if req.ProgressMessage != nil {
			t.Fatalf("expected static fixture list to skip progress, got %+v", req.ProgressMessage)
		}
		if len(req.MainGenres) != 1 || len(req.MainGenres[0].SubGenres) != 1 || len(req.MainGenres[0].SubGenres[0].Fixtures) != 1 {
			t.Fatalf("unexpected fixture list payload: %+v", req.MainGenres)
		}
		if !req.MainGenres[0].SubGenres[0].Fixtures[0].Obtained {
			t.Fatalf("expected static fixture list to render all fixtures as obtained, got %+v", req.MainGenres[0].SubGenres[0].Fixtures[0])
		}
		_, _ = w.Write([]byte("fixture-list"))
	}))
	defer drawingServer.Close()

	app := &renderapp.App{
		MySekai:    rendermysekai.NewController(drawing.NewHarukiDrawingClient(drawingServer.URL), nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true}),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	showProfile := false
	showProgress := false
	showObtained := false
	params, err := json.Marshal(rendermysekai.FixtureListQuery{
		ShowID:       boolPtr(true),
		ShowProfile:  &showProfile,
		ShowProgress: &showProgress,
		ShowObtained: &showObtained,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeMysekai(NewRequestContext(context.Background(), &CommandRequest{
		Module: parser.ModuleMysekai,
		Mode:   "mysekai-fixture-list",
		Region: "jp",
		Params: params,
	}, app))
	if err != nil {
		t.Fatalf("executeMysekai fixture list: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected static fixture list message: %+v", message)
	}
}

func TestExecuteMySekaiFixtureDetailSkipsBindingAndSnapshot(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}

	writeJSON := func(name string, data any) {
		t.Helper()
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(masterdataDir, name), raw, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	writeJSON("mysekaiFixtures.json", []map[string]any{
		{
			"id":                        2001,
			"name":                      "Wood Chair",
			"assetbundleName":           "wood_chair",
			"mysekaiFixtureType":        "furniture",
			"mysekaiFixtureMainGenreId": 1,
			"mysekaiFixtureSubGenreId":  11,
			"gridSize":                  map[string]any{"width": 2, "depth": 1, "height": 3},
		},
	})
	writeJSON("mysekaiFixtureMainGenres.json", []map[string]any{
		{"id": 1, "name": "Main A", "assetbundleName": "main_a"},
	})
	writeJSON("mysekaiFixtureSubGenres.json", []map[string]any{
		{"id": 11, "name": "Sub A", "assetbundleName": "sub_a"},
	})
	writeJSON("mysekaiBlueprints.json", []map[string]any{
		{"id": 1001, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 2001},
	})
	writeJSON("mysekaiBlueprintMysekaiMaterialCosts.json", []map[string]any{})
	writeJSON("mysekaiFixtureOnlyDisassembleMaterials.json", []map[string]any{})
	writeJSON("mysekaiFixtureTags.json", []map[string]any{})
	writeJSON("mysekaiMaterials.json", []map[string]any{})

	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/mysekai/fixture-detail" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var reqs []drawing.MysekaiFixtureDetailRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&reqs); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if len(reqs) != 1 {
			t.Fatalf("expected 1 fixture detail request, got %+v", reqs)
		}
		if reqs[0].Title != "【JP-2001】Wood Chair" {
			t.Fatalf("unexpected title: %+v", reqs[0])
		}
		_, _ = w.Write([]byte("fixture-detail"))
	}))
	defer drawingServer.Close()

	app := &renderapp.App{
		MySekai:    rendermysekai.NewController(drawing.NewHarukiDrawingClient(drawingServer.URL), nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true}),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	message, err := executeMysekai(NewRequestContext(context.Background(), &CommandRequest{
		Module: parser.ModuleMysekai,
		Mode:   "mysekai-fixture-detail",
		Region: "jp",
		Query:  "2001",
	}, app))
	if err != nil {
		t.Fatalf("executeMysekai fixture detail: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected fixture detail message: %+v", message)
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func TestExecuteConcurrentMessagesRunsJobsInParallelAndPreservesOrder(t *testing.T) {
	started := make(chan string, 2)
	finished := make(chan string, 2)
	release := make(chan struct{})

	job := func(name string, delay time.Duration) concurrentMessageJob {
		return func(context.Context) (onebot11.Message, error) {
			started <- name
			<-release
			time.Sleep(delay)
			finished <- name
			return onebot11.Message{onebot11.Text(name)}, nil
		}
	}

	resultCh := make(chan onebot11.Message, 1)
	errCh := make(chan error, 1)
	go func() {
		message, err := executeConcurrentMessages(
			context.Background(),
			job("resource", 80*time.Millisecond),
			job("map", 0),
		)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- message
	}()

	startedSet := make(map[string]bool, 2)
	timeout := time.After(2 * time.Second)
	for len(startedSet) < 2 {
		select {
		case name := <-started:
			startedSet[name] = true
		case <-timeout:
			t.Fatal("expected both concurrent jobs to start before release")
		}
	}

	close(release)

	select {
	case err := <-errCh:
		t.Fatalf("executeConcurrentMessages() error = %v", err)
	case message := <-resultCh:
		if got := messageTextOrder(message); len(got) != 2 || got[0] != "resource" || got[1] != "map" {
			t.Fatalf("unexpected message order: %v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for concurrent message execution")
	}

	firstFinished := <-finished
	if firstFinished != "map" {
		t.Fatalf("expected map to finish first, got %q", firstFinished)
	}
}

func TestExecuteConcurrentMessagesReturnsJobError(t *testing.T) {
	expectedErr := errors.New("boom")
	_, err := executeConcurrentMessages(
		context.Background(),
		func(context.Context) (onebot11.Message, error) {
			return nil, expectedErr
		},
	)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func messageTextOrder(message onebot11.Message) []string {
	var texts []string
	for _, segment := range message {
		text, ok := segment.Data.(onebot11.TextData)
		if !ok {
			texts = append(texts, fmt.Sprintf("non-text:%T", segment.Data))
			continue
		}
		texts = append(texts, text.Text)
	}
	return texts
}
