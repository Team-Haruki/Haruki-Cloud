package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/config"
	pjskenttest "haruki-cloud/database/pjsk/enttest"
	sekaidb "haruki-cloud/database/sekai"
	sekaienttest "haruki-cloud/database/sekai/enttest"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/identity"
	pjskalias "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/music"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	renderscore "haruki-cloud/internal/pjsk/render/score"
	rendersk "haruki-cloud/internal/pjsk/render/sk"
	"haruki-cloud/internal/pjsk/render/userdata"
	rendervlive "haruki-cloud/internal/pjsk/render/vlive"
	accountdata "haruki-cloud/internal/pjsk/userdata"
	"haruki-cloud/utils/drawing"
	"haruki-cloud/utils/imagecache"
	sekaiapi "haruki-cloud/utils/sekai"

	_ "github.com/mattn/go-sqlite3"
)

type bridgeTestBindingValidator struct{}

func (bridgeTestBindingValidator) GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	if strings.EqualFold(server, "jp") {
		return &sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 12345678901234,
				Name:   "JPUser",
			},
		}, nil
	}
	return nil, sekaiapi.ErrUserNotFound
}

type bridgeMultiRegionBindingValidator struct {
	profiles map[string]map[string]string
}

func (v bridgeMultiRegionBindingValidator) GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	regionProfiles, ok := v.profiles[strings.ToLower(strings.TrimSpace(server))]
	if !ok {
		return nil, sekaiapi.ErrUserNotFound
	}
	name, ok := regionProfiles[strings.TrimSpace(userID)]
	if !ok {
		return nil, sekaiapi.ErrUserNotFound
	}
	uid, err := strconv.ParseInt(strings.TrimSpace(userID), 10, 64)
	if err != nil || uid <= 0 {
		return nil, sekaiapi.ErrUserNotFound
	}
	return &sekaiapi.GetAnotherProfileResponse{
		User: sekaiapi.AnotherUser{
			UserID: uid,
			Name:   name,
		},
	}, nil
}

func newBridgeTestBindingServiceWithValidator(t *testing.T, validator accountdata.ProfileValidator) *accountdata.BindingService {
	t.Helper()
	pjskClient := pjskenttest.Open(t, "sqlite3", "file:bridge_test_bind?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = pjskClient.Close() })
	usersClient := usersenttest.Open(t, "sqlite3", "file:bridge_test_users?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = usersClient.Close() })
	return accountdata.NewBindingService(
		pjskClient,
		identity.NewResolver(usersClient),
		validator,
	)
}

func newBridgeTestBindingService(t *testing.T) *accountdata.BindingService {
	t.Helper()
	return newBridgeTestBindingServiceWithValidator(t, bridgeTestBindingValidator{})
}

func TestExecuteCheckDataMySekaiRequiresVisibleMySekaiSnapshot(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingService(t)

	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := service.SetBindingMySekaiVisible(ctx, "qq", "42", "jp", false); err != nil {
		t.Fatalf("hide mysekai: %v", err)
	}

	params, err := json.Marshal(userQueryParams{
		Mode:           "self",
		Platform:       "qq",
		PlatformUserID: "42",
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	_, err = executeCheckData(NewRequestContext(ctx, &parser.ResolvedCommand{
		Module: parser.ModuleCheckData,
		Mode:   "mysekai",
		Region: "jp",
		Params: params,
	}, &renderapp.App{Bindings: service}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "当前账号没有可用的 MySekai 抓包数据" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTrackerRankQueryFromParamsUsesResolvedRegionWhenImplicit(t *testing.T) {
	raw, err := json.Marshal(rendersk.TrackerRankQuery{
		Region:         "jp",
		RegionExplicit: false,
		Ranks:          []int{100},
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	req, ok := trackerRankQueryFromParams(&parser.ResolvedCommand{
		Region: "tw",
		Params: raw,
	})
	if !ok {
		t.Fatal("expected tracker request to be parsed")
	}
	if req.Region != "tw" {
		t.Fatalf("expected region tw, got %q", req.Region)
	}
}

func TestTrackerRankQueryFromParamsKeepsExplicitRegion(t *testing.T) {
	raw, err := json.Marshal(rendersk.TrackerRankQuery{
		Region:         "jp",
		RegionExplicit: true,
		Ranks:          []int{100},
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	req, ok := trackerRankQueryFromParams(&parser.ResolvedCommand{
		Region: "tw",
		Params: raw,
	})
	if !ok {
		t.Fatal("expected tracker request to be parsed")
	}
	if req.Region != "jp" {
		t.Fatalf("expected region jp, got %q", req.Region)
	}
}

func TestTrackerRankQueryFromParamsAcceptsWlSelectorWithoutRanks(t *testing.T) {
	raw, err := json.Marshal(rendersk.TrackerRankQuery{
		Region:           "jp",
		RegionExplicit:   false,
		WlCharacterQuery: "wl",
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	req, ok := trackerRankQueryFromParams(&parser.ResolvedCommand{
		Region: "jp",
		Params: raw,
	})
	if !ok {
		t.Fatal("expected tracker request to be parsed")
	}
	if req.WlCharacterQuery != "wl" {
		t.Fatalf("unexpected wl selector: %q", req.WlCharacterQuery)
	}
}

func TestResolveTrackerTargetUserNoPrefixUsesGlobalDefaultBinding(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingServiceWithValidator(t, bridgeMultiRegionBindingValidator{
		profiles: map[string]map[string]string{
			"tw": {"11111111111111": "TW User"},
			"jp": {"22222222222222": "JP User"},
		},
	})

	if _, err := service.Bind(ctx, "qq", "9001", "11111111111111"); err != nil {
		t.Fatalf("bind tw: %v", err)
	}
	if _, err := service.Bind(ctx, "qq", "9001", "22222222222222"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	if _, err := service.SetBindingVisible(ctx, "qq", "9001", "tw", true); err != nil {
		t.Fatalf("show tw binding: %v", err)
	}

	req := rendersk.TrackerRankQuery{
		Region:         "jp",
		RegionExplicit: false,
		Ranks:          []int{100},
		TargetPlatform: "qq",
		TargetUserID:   "9001",
	}
	if err := resolveTrackerTargetUser(ctx, &renderapp.App{Bindings: service}, &req, "qq", "someone"); err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if req.UserID == nil || *req.UserID != 11111111111111 {
		t.Fatalf("unexpected resolved uid: %+v", req.UserID)
	}
	if req.Region != "tw" {
		t.Fatalf("expected region tw, got %q", req.Region)
	}
}

func TestResolveTrackerTargetUserNoPrefixFallsBackToJPWhenNoGlobalDefault(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingService(t)

	if _, err := service.Bind(ctx, "qq", "9002", "12345678901234"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	if _, err := service.SetBindingVisible(ctx, "qq", "9002", "jp", true); err != nil {
		t.Fatalf("show jp binding: %v", err)
	}
	if _, err := service.ClearDefault(ctx, "qq", "9002", "", "", accountdata.GlobalDefaultBindingScope); err != nil {
		t.Fatalf("clear global default: %v", err)
	}

	req := rendersk.TrackerRankQuery{
		Region:         "tw",
		RegionExplicit: false,
		Ranks:          []int{100},
		TargetPlatform: "qq",
		TargetUserID:   "9002",
	}
	if err := resolveTrackerTargetUser(ctx, &renderapp.App{Bindings: service}, &req, "qq", "someone"); err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if req.UserID == nil || *req.UserID != 12345678901234 {
		t.Fatalf("unexpected resolved uid: %+v", req.UserID)
	}
	if req.Region != "jp" {
		t.Fatalf("expected region jp fallback, got %q", req.Region)
	}
}

func TestResolveTrackerTargetUserRejectsHiddenAtTarget(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingService(t)

	if _, err := service.Bind(ctx, "qq", "9003", "12345678901234"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	if _, err := service.SetBindingVisible(ctx, "qq", "9003", "jp", false); err != nil {
		t.Fatalf("hide binding: %v", err)
	}

	req := rendersk.TrackerRankQuery{
		Region:         "jp",
		RegionExplicit: true,
		Ranks:          []int{100},
		TargetPlatform: "qq",
		TargetUserID:   "9003",
	}
	err := resolveTrackerTargetUser(ctx, &renderapp.App{Bindings: service}, &req, "qq", "another-user")
	if err == nil {
		t.Fatal("expected hidden-target error, got nil")
	}
	if !strings.Contains(err.Error(), "已隐藏个人信息") {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.UserID != nil {
		t.Fatalf("expected unresolved user id, got %+v", req.UserID)
	}
}

func TestResolveTrackerTargetUserAllowsHiddenSelfTarget(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingService(t)

	if _, err := service.Bind(ctx, "qq", "9005", "12345678901234"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	if _, err := service.SetBindingVisible(ctx, "qq", "9005", "jp", false); err != nil {
		t.Fatalf("hide binding: %v", err)
	}

	req := rendersk.TrackerRankQuery{
		Region:         "jp",
		RegionExplicit: true,
		EventID:        101,
		TargetPlatform: "qq",
		TargetUserID:   "9005",
	}
	if err := resolveTrackerTargetUser(ctx, &renderapp.App{Bindings: service}, &req, "qq", "9005"); err != nil {
		t.Fatalf("resolve hidden self target: %v", err)
	}
	if req.UserID == nil || *req.UserID != 12345678901234 {
		t.Fatalf("unexpected resolved uid: %+v", req.UserID)
	}
}

func TestResolveTrackerTargetUserSupportsSelector(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingServiceWithValidator(t, bridgeMultiRegionBindingValidator{
		profiles: map[string]map[string]string{
			"tw": {"11111111111111": "TW User"},
			"jp": {"33333333333333": "JP User"},
		},
	})

	if _, err := service.Bind(ctx, "qq", "9004", "33333333333333"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	if _, err := service.Bind(ctx, "qq", "9004", "11111111111111"); err != nil {
		t.Fatalf("bind tw: %v", err)
	}

	req := rendersk.TrackerRankQuery{
		Region:         "jp",
		RegionExplicit: false,
		Ranks:          []int{100},
		TargetPlatform: "qq",
		TargetUserID:   "9004",
		TargetSelector: "u1",
	}
	if err := resolveTrackerTargetUser(ctx, &renderapp.App{Bindings: service}, &req, "qq", "someone"); err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if req.UserID == nil || *req.UserID != 33333333333333 {
		t.Fatalf("unexpected resolved uid: %+v", req.UserID)
	}
	if req.Region != "jp" {
		t.Fatalf("expected selector region jp, got %q", req.Region)
	}
}

type bridgeMusicSource struct {
	musics       map[int]*masterdata.Music
	difficulties map[int][]*masterdata.MusicDifficulty
}

type bridgeMusicAliasResolver struct {
	ids map[string]int
}

func (s *bridgeMusicSource) DefaultRegion() renderregion.Value { return renderregion.JP }

func (r *bridgeMusicAliasResolver) TryResolveMusicID(_ context.Context, token string) (int, bool, error) {
	if r == nil {
		return 0, false, nil
	}
	id, ok := r.ids[strings.ToLower(strings.TrimSpace(token))]
	return id, ok, nil
}

func (r *bridgeMusicAliasResolver) TryResolveMusicTitleOrAliasID(_ context.Context, token string) (int, bool, error) {
	if r == nil {
		return 0, false, nil
	}
	id, ok := r.ids[strings.ToLower(strings.TrimSpace(token))]
	return id, ok, nil
}

func (s *bridgeMusicSource) SearchMusic(query string) (*masterdata.Music, error) {
	for _, item := range s.musics {
		if strings.EqualFold(item.Title, query) {
			copy := *item
			return &copy, nil
		}
	}
	return nil, os.ErrNotExist
}

func (s *bridgeMusicSource) GetMusicByID(id int) (*masterdata.Music, error) {
	item := s.musics[id]
	if item == nil {
		return nil, os.ErrNotExist
	}
	copy := *item
	return &copy, nil
}

func (s *bridgeMusicSource) GetMusicByEventID(int) (*masterdata.Music, error) {
	return nil, os.ErrNotExist
}

func (s *bridgeMusicSource) GetMusics() []*masterdata.Music {
	out := make([]*masterdata.Music, 0, len(s.musics))
	for _, item := range s.musics {
		copy := *item
		out = append(out, &copy)
	}
	return out
}

func (s *bridgeMusicSource) GetBanEvents(int) []*masterdata.Event { return nil }

func (s *bridgeMusicSource) GetMusicLocalizedTitles(int) ([]string, error) { return nil, nil }

func (s *bridgeMusicSource) GetMusicDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error) {
	items := s.difficulties[musicID]
	out := make([]*masterdata.MusicDifficulty, 0, len(items))
	for _, item := range items {
		copy := *item
		out = append(out, &copy)
	}
	return out, nil
}

func (s *bridgeMusicSource) GetMusicVocals(int) ([]*masterdata.MusicVocal, error) { return nil, nil }

func (s *bridgeMusicSource) GetMusicTags(int) ([]string, error) { return nil, nil }

func (s *bridgeMusicSource) GetCharacterByID(int) (*masterdata.Character, error) {
	return nil, os.ErrNotExist
}

func (s *bridgeMusicSource) GetPrimaryEventByMusicID(int) (*masterdata.Event, error) {
	return nil, os.ErrNotExist
}

func (s *bridgeMusicSource) GetLimitedTimeMusics(int) []*masterdata.LimitedTimeMusic { return nil }

func (s *bridgeMusicSource) GetOutsideCharacterByID(int) (string, error) { return "", nil }

func TestExecuteMusicCoverAndNoteCount(t *testing.T) {
	root := t.TempDir()
	jacketPath := filepath.Join(root, "music", "jacket", "jacket_test", "jacket_test.png")
	if err := os.MkdirAll(filepath.Dir(jacketPath), 0o755); err != nil {
		t.Fatalf("mkdir jacket: %v", err)
	}
	if err := os.WriteFile(jacketPath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write jacket: %v", err)
	}

	chartCalls := 0
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/pjsk/chart":
			chartCalls++
			var req drawing.GenerateMusicChartRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode chart request: %v", err)
			}
			if req.Difficulty != "expert" {
				t.Fatalf("unexpected chart difficulty: %+v", req)
			}
			if musicID, ok := req.MusicID.(float64); !ok || int(musicID) != 1 {
				t.Fatalf("unexpected chart music id: %+v", req.MusicID)
			}
			_, _ = w.Write([]byte("png"))
		default:
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
	}))
	defer drawingServer.Close()

	source := &bridgeMusicSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_test"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 27, TotalNoteCount: 777},
			},
		},
	}
	app := &renderapp.App{
		Music:      music.NewController(source, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper(root, nil), nil, nil),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	message, err := executeMusic(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleMusic,
		Mode:   "music-cover",
		Query:  "Song A",
		Region: "jp",
	}, app))
	if err != nil {
		t.Fatalf("executeMusic cover: %v", err)
	}
	if len(message) != 2 || message[0].Type != "image" || message[1].Type != "text" {
		t.Fatalf("unexpected cover message: %+v", message)
	}

	params, err := json.Marshal(map[string]int{"note_count": 777})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	message, err = executeMusic(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleMusic,
		Mode:   "music-note-count",
		Region: "jp",
		Params: params,
	}, app))
	if err != nil {
		t.Fatalf("executeMusic note-count: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected note-count message: %+v", message)
	}
	if chartCalls != 1 {
		t.Fatalf("expected 1 chart render call, got %d", chartCalls)
	}
}

func TestExecuteMusicBPMUsesBriefListImageForMultipleMatches(t *testing.T) {
	root := t.TempDir()
	chartA := filepath.Join(root, "music", "music_score", "0001_01", "expert.txt")
	chartB := filepath.Join(root, "music", "music_score", "0002_01", "master.txt")
	if err := os.MkdirAll(filepath.Dir(chartA), 0o755); err != nil {
		t.Fatalf("mkdir chartA: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(chartB), 0o755); err != nil {
		t.Fatalf("mkdir chartB: %v", err)
	}
	if err := os.WriteFile(chartA, []byte(strings.Join([]string{
		"#BPM01:200",
		"#00008:0100",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write chartA: %v", err)
	}
	if err := os.WriteFile(chartB, []byte(strings.Join([]string{
		"#BPM01:180",
		"#BPM02:200",
		"#00008:0100",
		"#00108:0200",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write chartB: %v", err)
	}

	briefListCalls := 0
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/pjsk/music/brief-list":
			briefListCalls++
			var req drawing.MusicBriefListRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode brief-list request: %v", err)
			}
			if req.Title == nil || *req.Title != "BPM 200 匹配结果" {
				t.Fatalf("unexpected brief-list title: %+v", req.Title)
			}
			if len(req.MusicList) != 2 {
				t.Fatalf("expected 2 brief-list items, got %d", len(req.MusicList))
			}
			if len(req.MusicList[0].Difficulty.Order) != 1 || req.MusicList[0].Difficulty.Order[0] != "expert" {
				t.Fatalf("unexpected first item difficulty: %+v", req.MusicList[0].Difficulty)
			}
			if len(req.MusicList[1].Difficulty.Order) != 1 || req.MusicList[1].Difficulty.Order[0] != "master" {
				t.Fatalf("unexpected second item difficulty: %+v", req.MusicList[1].Difficulty)
			}
			_, _ = w.Write([]byte("png"))
		default:
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
	}))
	defer drawingServer.Close()

	source := &bridgeMusicSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
			2: {ID: 2, Title: "Song B", AssetBundleName: "jacket_b"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 27},
			},
			2: {
				{MusicID: 2, MusicDifficulty: "master", PlayLevel: 31},
			},
		},
	}
	app := &renderapp.App{
		Music:      music.NewController(source, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper(root, nil), nil, nil),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	params, err := json.Marshal(map[string]any{"bpm": 200})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeMusic(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleMusic,
		Mode:   "music-bpm",
		Region: "jp",
		Params: params,
	}, app))
	if err != nil {
		t.Fatalf("executeMusic bpm: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected bpm message: %+v", message)
	}
	if briefListCalls != 1 {
		t.Fatalf("expected 1 brief-list render call, got %d", briefListCalls)
	}
}

func TestExecuteMusicListUsesQueryKeywordAndAlias(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/music/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req drawing.MusicListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.MusicList) != 1 {
			t.Fatalf("expected 1 music item, got %d", len(req.MusicList))
		}
		if id, ok := req.MusicList[0]["id"].(float64); !ok || int(id) != 1 {
			t.Fatalf("unexpected music list item: %+v", req.MusicList[0])
		}
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	source := &bridgeMusicSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
			2: {ID: 2, Title: "Song B", AssetBundleName: "jacket_b"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31},
			},
			2: {
				{MusicID: 2, MusicDifficulty: "master", PlayLevel: 30},
			},
		},
	}
	controller := music.NewController(source, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper("", nil), nil, nil)
	controller.SetAliasResolver(&bridgeMusicAliasResolver{
		ids: map[string]int{"blue song": 1},
	})

	app := &renderapp.App{
		Music:      controller,
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	params, err := json.Marshal(map[string]string{"difficulty": "master"})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeMusic(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleMusic,
		Mode:   "music-list",
		Query:  "blue song",
		Region: "jp",
		Params: params,
	}, app))
	if err != nil {
		t.Fatalf("executeMusic list: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected list message: %+v", message)
	}
}

func TestExecuteMusicListRequiresSuiteSnapshotWhenBindingVisible(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	source := &bridgeMusicSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31},
			},
		},
	}
	app := &renderapp.App{
		Music:    music.NewController(source, drawing.NewHarukiDrawingClient("https://drawing.invalid"), assets.NewAssetHelper("", nil), nil, nil),
		Bindings: service,
	}

	params, err := json.Marshal(map[string]string{"difficulty": "master"})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	_, err = executeMusic(NewRequestContext(ctx, &parser.ResolvedCommand{
		Module:            parser.ModuleMusic,
		Mode:              "music-list",
		Region:            "jp",
		Params:            params,
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, app))
	if err == nil {
		t.Fatal("expected missing suite snapshot to fail")
	}
	if err.Error() != ErrMsgSuiteDataNotFound {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteMusicListKeepsFallbackWhenSuiteHidden(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := service.SetBindingSuiteVisible(ctx, "qq", "42", "jp", false); err != nil {
		t.Fatalf("hide suite: %v", err)
	}

	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	source := &bridgeMusicSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31},
			},
		},
	}
	app := &renderapp.App{
		Music:      music.NewController(source, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper("", nil), nil, nil),
		Bindings:   service,
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	params, err := json.Marshal(map[string]string{"difficulty": "master"})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeMusic(NewRequestContext(ctx, &parser.ResolvedCommand{
		Module:            parser.ModuleMusic,
		Mode:              "music-list",
		Region:            "jp",
		Params:            params,
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, app))
	if err != nil {
		t.Fatalf("executeMusic list with hidden suite: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected message: %+v", message)
	}
}

func TestExecuteMusicListUsesSuiteSnapshotResults(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	root := t.TempDir()
	jacketPath := filepath.Join(root, "music", "jacket", "jacket_a", "jacket_a.png")
	if err := os.MkdirAll(filepath.Dir(jacketPath), 0o755); err != nil {
		t.Fatalf("mkdir jacket: %v", err)
	}
	if err := os.WriteFile(jacketPath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write jacket: %v", err)
	}

	var captured drawing.MusicListRequest
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/pjsk/music/list") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	source := &bridgeMusicSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31},
			},
		},
	}
	app := &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Music:    music.NewController(source, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper(root, nil), nil, nil),
		Bindings: service,
		Snapshots: userdata.NewStaticSnapshotProvider(&runtimeSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{
				Nickname:        "SnapshotUser",
				LeaderImagePath: "asset/user/leader.png",
				UserCards:       []any{},
			},
			musicResults: map[string]map[int]string{
				"master": {
					1: "ap",
				},
			},
		}),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	params, err := json.Marshal(map[string]string{"difficulty": "master"})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeMusic(NewRequestContext(ctx, &parser.ResolvedCommand{
		Module:            parser.ModuleMusic,
		Mode:              "music-list",
		Region:            "jp",
		Params:            params,
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, app))
	if err != nil {
		t.Fatalf("executeMusic list: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected list message: %+v", message)
	}
	if got := captured.Profile.Nickname; got != "SnapshotUser" {
		t.Fatalf("unexpected profile nickname: %q", got)
	}
	if got := captured.UserResults[1]; got != "ap" {
		t.Fatalf("unexpected user result: %+v", got)
	}
}

func TestExecuteMusicProgressRequiresSuiteData(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingService(t)

	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := service.SetBindingSuiteVisible(ctx, "qq", "42", "jp", false); err != nil {
		t.Fatalf("hide suite: %v", err)
	}

	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	app := &renderapp.App{
		Bindings:   service,
		Music:      music.NewController(&bridgeMusicSource{}, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper("", nil), nil, nil),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	_, err := executeMusic(NewRequestContext(ctx, &parser.ResolvedCommand{
		Module:            parser.ModuleMusic,
		Mode:              "music-progress",
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, app))
	if err != nil {
		t.Fatalf("expected hidden suite to keep fallback, got %v", err)
	}
}

func TestExecuteMusicProgressRequiresResolvableSuiteSnapshot(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingService(t)

	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := service.SetBindingSuiteVisible(ctx, "qq", "42", "jp", true); err != nil {
		t.Fatalf("show suite: %v", err)
	}

	app := &renderapp.App{
		Bindings: service,
		Music:    music.NewController(&bridgeMusicSource{}, nil, assets.NewAssetHelper("", nil), nil, nil),
	}

	_, err := executeMusic(NewRequestContext(ctx, &parser.ResolvedCommand{
		Module:            parser.ModuleMusic,
		Mode:              "music-progress",
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, app))
	if err == nil {
		t.Fatal("expected snapshot error, got nil")
	}
	if err.Error() != ErrMsgSuiteDataNotFound {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveDeckMusicSelection(t *testing.T) {
	root := t.TempDir()
	jacketPath := filepath.Join(root, "music", "jacket", "jacket_test", "jacket_test.png")
	if err := os.MkdirAll(filepath.Dir(jacketPath), 0o755); err != nil {
		t.Fatalf("mkdir jacket: %v", err)
	}
	if err := os.WriteFile(jacketPath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write jacket: %v", err)
	}

	source := &bridgeMusicSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_test"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31},
			},
		},
	}
	app := &renderapp.App{
		Music: music.NewController(source, nil, assets.NewAssetHelper(root, nil), nil, nil),
	}

	query := renderdeck.AutoQuery{
		Region:     "jp",
		MusicQuery: "Song A",
	}
	if err := resolveDeckMusicSelection(&query, app); err != nil {
		t.Fatalf("resolveDeckMusicSelection: %v", err)
	}
	if query.MusicID == nil || *query.MusicID != 1 {
		t.Fatalf("unexpected music id: %+v", query.MusicID)
	}
	if query.MusicTitle != "Song A" {
		t.Fatalf("unexpected music title: %q", query.MusicTitle)
	}
	if !strings.Contains(query.MusicCoverPath, "jacket_test.png") {
		t.Fatalf("unexpected music cover path: %q", query.MusicCoverPath)
	}
}

func TestResolveDeckMusicSelectionMusicCompareSelections(t *testing.T) {
	root := t.TempDir()
	for _, asset := range []string{"jacket_a", "jacket_b"} {
		jacketPath := filepath.Join(root, "music", "jacket", asset, asset+".png")
		if err := os.MkdirAll(filepath.Dir(jacketPath), 0o755); err != nil {
			t.Fatalf("mkdir jacket: %v", err)
		}
		if err := os.WriteFile(jacketPath, []byte("png"), 0o644); err != nil {
			t.Fatalf("write jacket: %v", err)
		}
	}

	source := &bridgeMusicSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
			2: {ID: 2, Title: "Song B", AssetBundleName: "jacket_b"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {{MusicID: 1, MusicDifficulty: "hard", PlayLevel: 21}},
			2: {{MusicID: 2, MusicDifficulty: "master", PlayLevel: 31}},
		},
	}
	app := &renderapp.App{
		Music: music.NewController(source, nil, assets.NewAssetHelper(root, nil), nil, nil),
	}

	query := renderdeck.AutoQuery{
		Region:              "jp",
		MusicCompare:        true,
		MusicCompareQueries: []string{"Song Ahd", "Song B"},
	}
	if err := resolveDeckMusicSelection(&query, app); err != nil {
		t.Fatalf("resolveDeckMusicSelection compare: %v", err)
	}
	if query.MusicID != nil || query.MusicTitle != "" || query.MusicCoverPath != "" {
		t.Fatalf("unexpected single-music fields in compare mode: %+v", query)
	}
	if len(query.MusicCompareSelections) != 2 {
		t.Fatalf("unexpected compare selections: %+v", query.MusicCompareSelections)
	}
	if query.MusicCompareSelections[0].MusicID != 1 || query.MusicCompareSelections[0].MusicDiff != "hard" {
		t.Fatalf("unexpected first compare selection: %+v", query.MusicCompareSelections[0])
	}
	if query.MusicCompareSelections[1].MusicID != 2 || query.MusicCompareSelections[1].MusicDiff != "master" {
		t.Fatalf("unexpected second compare selection: %+v", query.MusicCompareSelections[1])
	}
}

func TestExecuteScoreMusicMetaBuildsRequestsFromQueries(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/score/music-meta" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req []drawing.MusicMetaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req) != 1 {
			t.Fatalf("expected 1 request, got %d", len(req))
		}
		if req[0].MusicID != 1 || req[0].MusicTitle != "Song A" {
			t.Fatalf("unexpected request header: %+v", req[0])
		}
		if len(req[0].Metas) != 2 {
			t.Fatalf("expected 2 meta entries, got %d", len(req[0].Metas))
		}
		if req[0].Metas[0].Difficulty != "expert" || req[0].Metas[1].Difficulty != "master" {
			t.Fatalf("unexpected meta order: %+v", req[0].Metas)
		}
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	root := t.TempDir()
	userPath := filepath.Join(root, "user.json")
	metaPath := filepath.Join(root, "music_meta.json")
	if err := os.WriteFile(userPath, []byte(`{
  "now": 1700000000,
  "userGamedata": {"userId": 12345678901234, "name": "Tester", "deck": 1},
  "userProfile": {},
  "userDecks": [{"deckId": 1}],
  "userCards": []
}`), 0o644); err != nil {
		t.Fatalf("write user snapshot: %v", err)
	}
	if err := os.WriteFile(metaPath, []byte(`[
  {"music_id": 1, "difficulty": "master", "music_time": 120, "tap_count": 600, "event_rate": 100, "base_score": 1.2, "base_score_auto": 1.1, "skill_score_solo": [0.1,0.2], "skill_score_auto": [0.3,0.4], "skill_score_multi": [0.5,0.6], "fever_score": 0.7},
  {"music_id": 1, "difficulty": "expert", "music_time": 118, "tap_count": 550, "event_rate": 90, "base_score": 1.0, "base_score_auto": 0.9, "skill_score_solo": [0.11,0.22], "skill_score_auto": [0.33,0.44], "skill_score_multi": [0.55,0.66], "fever_score": 0.77}
]`), 0o644); err != nil {
		t.Fatalf("write music meta snapshot: %v", err)
	}

	source := &bridgeMusicSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_test"},
		},
	}
	snapshot := userdata.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), userdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MusicMetaJSON: metaPath,
	})
	app := &renderapp.App{
		Music:      music.NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil),
		Score:      renderscore.NewController(drawing.NewHarukiDrawingClient(drawingServer.URL)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	params, err := json.Marshal(struct {
		Queries []string `json:"queries"`
	}{Queries: []string{"Song A"}})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeScore(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleScore,
		Mode:   "score-music-meta",
		Region: "jp",
		Params: params,
	}, app))
	if err != nil {
		t.Fatalf("executeScore music-meta: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected music-meta message: %+v", message)
	}
}

func TestExecuteScoreControlBuildsRequestFromParams(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/score/control" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req drawing.ScoreControlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.MusicID != 1 || req.MusicTitle != "Song A" || req.MusicBasicPoint != 100 {
			t.Fatalf("unexpected request header: %+v", req)
		}
		if req.TargetPoint != 100 || len(req.ValidScores) == 0 {
			t.Fatalf("unexpected request payload: %+v", req)
		}
		if !strings.Contains(req.MusicCoverPath, "music/jacket/jacket_test/jacket_test.png") {
			t.Fatalf("unexpected cover path: %q", req.MusicCoverPath)
		}
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	root := t.TempDir()
	userPath := filepath.Join(root, "user.json")
	metaPath := filepath.Join(root, "music_meta.json")
	if err := os.WriteFile(userPath, []byte(`{
  "now": 1700000000,
  "userGamedata": {"userId": 12345678901234, "name": "Tester", "deck": 1},
  "userProfile": {},
  "userDecks": [{"deckId": 1}],
  "userCards": []
}`), 0o644); err != nil {
		t.Fatalf("write user snapshot: %v", err)
	}
	if err := os.WriteFile(metaPath, []byte(`[
  {"music_id": 1, "difficulty": "master", "music_time": 120, "tap_count": 600, "event_rate": 100, "base_score": 1.2, "base_score_auto": 1.1, "skill_score_solo": [0.1,0.2], "skill_score_auto": [0.3,0.4], "skill_score_multi": [0.5,0.6], "fever_score": 0.7},
  {"music_id": 1, "difficulty": "expert", "music_time": 118, "tap_count": 550, "event_rate": 90, "base_score": 1.0, "base_score_auto": 0.9, "skill_score_solo": [0.11,0.22], "skill_score_auto": [0.33,0.44], "skill_score_multi": [0.55,0.66], "fever_score": 0.77}
]`), 0o644); err != nil {
		t.Fatalf("write music meta snapshot: %v", err)
	}

	source := &bridgeMusicSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_test"},
		},
	}
	snapshot := userdata.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), userdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MusicMetaJSON: metaPath,
	})
	app := &renderapp.App{
		Music:      music.NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil),
		Score:      renderscore.NewController(drawing.NewHarukiDrawingClient(drawingServer.URL)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	params, err := json.Marshal(struct {
		TargetPoint int    `json:"target_point"`
		Query       string `json:"query"`
		WL          bool   `json:"wl"`
	}{
		TargetPoint: 100,
		Query:       "Song A",
		WL:          false,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeScore(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleScore,
		Mode:   "score-control",
		Region: "jp",
		Params: params,
	}, app))
	if err != nil {
		t.Fatalf("executeScore score-control: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected score-control message: %+v", message)
	}
}

func TestExecuteCustomRoomScoreBuildsRequestFromParams(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/score/custom-room" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req drawing.CustomRoomScoreRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.TargetPoint != 22 {
			t.Fatalf("unexpected target point: %+v", req)
		}
		if len(req.CandidatePairs) != 1 || len(req.CandidatePairs[0]) != 2 || req.CandidatePairs[0][0] != 100 || req.CandidatePairs[0][1] != 0 {
			t.Fatalf("unexpected candidate pairs: %+v", req.CandidatePairs)
		}
		list := req.MusicListMap[100]
		if len(list) != 1 {
			t.Fatalf("unexpected music list map: %+v", req.MusicListMap)
		}
		if title, _ := list[0]["music_title"].(string); title != "Song A" {
			t.Fatalf("unexpected music title: %+v", list[0])
		}
		if cover, _ := list[0]["music_cover"].(string); !strings.Contains(cover, "music/jacket/jacket_test/jacket_test.png") {
			t.Fatalf("unexpected music cover: %+v", list[0])
		}
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	root := t.TempDir()
	userPath := filepath.Join(root, "user.json")
	metaPath := filepath.Join(root, "music_meta.json")
	if err := os.WriteFile(userPath, []byte(`{
  "now": 1700000000,
  "userGamedata": {"userId": 12345678901234, "name": "Tester", "deck": 1},
  "userProfile": {},
  "userDecks": [{"deckId": 1}],
  "userCards": []
}`), 0o644); err != nil {
		t.Fatalf("write user snapshot: %v", err)
	}
	if err := os.WriteFile(metaPath, []byte(`[
  {"music_id": 1, "difficulty": "master", "music_time": 120, "tap_count": 600, "event_rate": 100, "base_score": 1.2, "base_score_auto": 1.1, "skill_score_solo": [0.1,0.2], "skill_score_auto": [0.3,0.4], "skill_score_multi": [0.5,0.6], "fever_score": 0.7}
]`), 0o644); err != nil {
		t.Fatalf("write music meta snapshot: %v", err)
	}

	source := &bridgeMusicSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_test"},
		},
	}
	snapshot := userdata.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), userdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MusicMetaJSON: metaPath,
	})
	app := &renderapp.App{
		Music:      music.NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil),
		Score:      renderscore.NewController(drawing.NewHarukiDrawingClient(drawingServer.URL)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	params, err := json.Marshal(struct {
		TargetPoint int `json:"target_point"`
	}{TargetPoint: 22})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeScore(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleScore,
		Mode:   "score-custom-room",
		Region: "jp",
		Params: params,
	}, app))
	if err != nil {
		t.Fatalf("executeScore custom-room: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected custom-room message: %+v", message)
	}
}

func TestExecuteScoreMusicBoardBuildsRequestFromParams(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/score/music-board" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req drawing.MusicBoardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.LiveType != "solo" || req.Target != "score" {
			t.Fatalf("unexpected board mode: %+v", req)
		}
		if len(req.Items) == 0 {
			t.Fatalf("expected board items, got none")
		}
		if len(req.SpecMidDiffs) != 2 {
			t.Fatalf("expected highlighted diffs, got %+v", req.SpecMidDiffs)
		}
		if req.Items[0].MusicID != 1 || req.Items[0].Difficulty != "master" {
			t.Fatalf("unexpected first board item: %+v", req.Items[0])
		}
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	root := t.TempDir()
	userPath := filepath.Join(root, "user.json")
	metaPath := filepath.Join(root, "music_meta.json")
	if err := os.WriteFile(userPath, []byte(`{
  "now": 1700000000,
  "userGamedata": {"userId": 12345678901234, "name": "Tester", "deck": 1},
  "userProfile": {},
  "userDecks": [{"deckId": 1}],
  "userCards": []
}`), 0o644); err != nil {
		t.Fatalf("write user snapshot: %v", err)
	}
	if err := os.WriteFile(metaPath, []byte(`[
  {"music_id": 1, "difficulty": "master", "music_time": 120, "tap_count": 600, "event_rate": 100, "base_score": 1.20, "base_score_auto": 1.10, "skill_score_solo": [0.12,0.11,0.10,0.09,0.08,0.07], "skill_score_auto": [0.10,0.09,0.08,0.07,0.06,0.05], "skill_score_multi": [0.14,0.13,0.12,0.11,0.10,0.09], "fever_score": 0.70},
  {"music_id": 1, "difficulty": "expert", "music_time": 118, "tap_count": 550, "event_rate": 90, "base_score": 1.00, "base_score_auto": 0.95, "skill_score_solo": [0.10,0.09,0.08,0.07,0.06,0.05], "skill_score_auto": [0.09,0.08,0.07,0.06,0.05,0.04], "skill_score_multi": [0.12,0.11,0.10,0.09,0.08,0.07], "fever_score": 0.60},
  {"music_id": 2, "difficulty": "master", "music_time": 140, "tap_count": 500, "event_rate": 110, "base_score": 1.05, "base_score_auto": 0.98, "skill_score_solo": [0.08,0.07,0.06,0.05,0.04,0.03], "skill_score_auto": [0.07,0.06,0.05,0.04,0.03,0.02], "skill_score_multi": [0.10,0.09,0.08,0.07,0.06,0.05], "fever_score": 0.55}
]`), 0o644); err != nil {
		t.Fatalf("write music meta snapshot: %v", err)
	}

	source := &bridgeMusicSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
			2: {ID: 2, Title: "Song B", AssetBundleName: "jacket_b"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31},
				{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 27},
			},
			2: {
				{MusicID: 2, MusicDifficulty: "master", PlayLevel: 30},
			},
		},
	}
	snapshot := userdata.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), userdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MusicMetaJSON: metaPath,
	})
	app := &renderapp.App{
		Music:      music.NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil),
		Score:      renderscore.NewController(drawing.NewHarukiDrawingClient(drawingServer.URL)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	params, err := json.Marshal(music.BoardQuery{
		SpecQueries: []string{"Song A"},
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeScore(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleScore,
		Mode:   "score-music-board",
		Region: "jp",
		Params: params,
	}, app))
	if err != nil {
		t.Fatalf("executeScore music-board: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected music-board message: %+v", message)
	}
}

func TestExecuteScoreMusicBoardDoesNotTreatModeArgsAsSpecQueries(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/score/music-board" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req drawing.MusicBoardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.LiveType != "multi" || req.Target != "pt" {
			t.Fatalf("unexpected board mode: %+v", req)
		}
		if len(req.SpecMidDiffs) != 0 {
			t.Fatalf("expected no highlighted songs, got %+v", req.SpecMidDiffs)
		}
		if len(req.Items) == 0 {
			t.Fatalf("expected board items, got none")
		}
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	root := t.TempDir()
	userPath := filepath.Join(root, "user.json")
	metaPath := filepath.Join(root, "music_meta.json")
	if err := os.WriteFile(userPath, []byte(`{
  "now": 1700000000,
  "userGamedata": {"userId": 12345678901234, "name": "Tester", "deck": 1},
  "userProfile": {},
  "userDecks": [{"deckId": 1}],
  "userCards": []
}`), 0o644); err != nil {
		t.Fatalf("write user snapshot: %v", err)
	}
	if err := os.WriteFile(metaPath, []byte(`[
  {"music_id": 1, "difficulty": "master", "music_time": 120, "tap_count": 600, "event_rate": 100, "base_score": 1.20, "base_score_auto": 1.10, "skill_score_solo": [0.12,0.11,0.10,0.09,0.08,0.07], "skill_score_auto": [0.10,0.09,0.08,0.07,0.06,0.05], "skill_score_multi": [0.14,0.13,0.12,0.11,0.10,0.09], "fever_score": 0.70},
  {"music_id": 2, "difficulty": "master", "music_time": 140, "tap_count": 500, "event_rate": 110, "base_score": 1.05, "base_score_auto": 0.98, "skill_score_solo": [0.08,0.07,0.06,0.05,0.04,0.03], "skill_score_auto": [0.07,0.06,0.05,0.04,0.03,0.02], "skill_score_multi": [0.10,0.09,0.08,0.07,0.06,0.05], "fever_score": 0.55}
]`), 0o644); err != nil {
		t.Fatalf("write music meta snapshot: %v", err)
	}

	source := &bridgeMusicSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
			2: {ID: 2, Title: "Song B", AssetBundleName: "jacket_b"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31},
			},
			2: {
				{MusicID: 2, MusicDifficulty: "master", PlayLevel: 30},
			},
		},
	}
	snapshot := userdata.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), userdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MusicMetaJSON: metaPath,
	})
	app := &renderapp.App{
		Music:      music.NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil),
		Score:      renderscore.NewController(drawing.NewHarukiDrawingClient(drawingServer.URL)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	params, err := json.Marshal(music.BoardQuery{
		LiveType: "multi",
		Target:   "pt",
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeScore(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleScore,
		Mode:   "score-music-board",
		Region: "jp",
		Query:  "多人 火效率",
		Params: params,
	}, app))
	if err != nil {
		t.Fatalf("executeScore music-board without spec queries: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected music-board message: %+v", message)
	}
}

func TestFormatArrestTextUsesResolvedChallengeCharacterName(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_arrest?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	if _, err := sekaiClient.Gamecharacter.Create().
		SetServerRegion("jp").
		SetGameID(21).
		SetFirstName("初音").
		SetGivenName("ミク").
		Save(ctx); err != nil {
		t.Fatalf("create gamecharacter: %v", err)
	}

	app := &renderapp.App{Sekai: sekaiClient}
	resp := &sekaiapi.GetAnotherProfileResponse{
		User: sekaiapi.AnotherUser{
			UserID: 123456789,
			Name:   "ArrestUser",
			Rank:   88,
		},
		UserChallengeLiveSoloResult: sekaiapi.UserChallengeLiveSoloResult{
			CharacterID: 21,
			HighScore:   123456,
		},
	}

	text := formatArrestText(resp, defaultEnabledDiffs(), resolveArrestChallengeCharacterName(ctx, app, 21), true)
	if !strings.Contains(text, "逮捕: ArrestUser (UID: 123456789) Lv.88") {
		t.Fatalf("unexpected arrest text: %s", text)
	}
	if !strings.Contains(text, "挑战Live(") || !strings.Contains(text, "123,456") {
		t.Fatalf("unexpected arrest text: %s", text)
	}
	masked := formatArrestText(resp, defaultEnabledDiffs(), resolveArrestChallengeCharacterName(ctx, app, 21), false)
	if !strings.Contains(masked, "UID: 123***789") {
		t.Fatalf("expected masked uid, got: %s", masked)
	}
}

func TestResolveEducationAreaCharacterIDUsesMasterdataName(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_education_area_char?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	if _, err := sekaiClient.Gamecharacter.Create().
		SetServerRegion("jp").
		SetGameID(21).
		SetFirstName("初音").
		SetGivenName("未来").
		SetFirstNameEnglish("Hatsune").
		SetGivenNameEnglish("Miku").
		Save(ctx); err != nil {
		t.Fatalf("create gamecharacter: %v", err)
	}

	id, err := resolveEducationAreaCharacterID(ctx, &renderapp.App{Sekai: sekaiClient}, renderregion.JP, "初音未来")
	if err != nil {
		t.Fatalf("resolveEducationAreaCharacterID() error = %v", err)
	}
	if id != 21 {
		t.Fatalf("unexpected character id: %d", id)
	}

	id, err = resolveEducationAreaCharacterID(ctx, &renderapp.App{Sekai: sekaiClient}, renderregion.JP, "Hatsune Miku")
	if err != nil {
		t.Fatalf("resolveEducationAreaCharacterID() english error = %v", err)
	}
	if id != 21 {
		t.Fatalf("unexpected english character id: %d", id)
	}
}

func TestResolveDeckCharacterSelectionsUsesMasterdataQueries(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_deck_char?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	createCharacter := func(id int, firstName, givenName, firstNameEN, givenNameEN string) {
		t.Helper()
		if _, err := sekaiClient.Gamecharacter.Create().
			SetServerRegion("jp").
			SetGameID(int64(id)).
			SetFirstName(firstName).
			SetGivenName(givenName).
			SetFirstNameEnglish(firstNameEN).
			SetGivenNameEnglish(givenNameEN).
			Save(ctx); err != nil {
			t.Fatalf("create gamecharacter %d: %v", id, err)
		}
	}
	createCharacter(21, "初音", "未来", "Hatsune", "Miku")
	createCharacter(24, "巡音", "流歌", "Megurine", "Luka")

	query := renderdeck.AutoQuery{
		Region:                      "jp",
		RecommendType:               "event",
		WorldBloomEventTurn:         drawing.IntPtr(2),
		WorldBloomCharacterQuery:    "初音未来",
		ChallengeLiveCharacterQuery: "Hatsune Miku",
		FixedCharacterQueries:       []string{"巡音流歌"},
	}

	if err := resolveDeckCharacterSelections(context.Background(), &query, &renderapp.App{Sekai: sekaiClient}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.WorldBloomCharacterID == nil || *query.WorldBloomCharacterID != 21 {
		t.Fatalf("unexpected world bloom character id: %+v", query.WorldBloomCharacterID)
	}
	if query.EventUnit != "piapro" {
		t.Fatalf("unexpected world bloom event unit: %q", query.EventUnit)
	}
	if query.ChallengeLiveCharacterID == nil || *query.ChallengeLiveCharacterID != 21 {
		t.Fatalf("unexpected challenge character id: %+v", query.ChallengeLiveCharacterID)
	}
	if len(query.FixedCharacters) != 1 || query.FixedCharacters[0] != 24 {
		t.Fatalf("unexpected fixed character ids: %+v", query.FixedCharacters)
	}
	if query.WorldBloomCharacterQuery != "" || query.ChallengeLiveCharacterQuery != "" || len(query.FixedCharacterQueries) != 0 {
		t.Fatalf("expected character queries to be consumed: %+v", query)
	}
}

func TestResolveDeckCharacterSelectionsFallsBackChallengeQueryToMusic(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_deck_challenge_fallback?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	if _, err := sekaiClient.Gamecharacter.Create().
		SetServerRegion("jp").
		SetGameID(21).
		SetFirstName("初音").
		SetGivenName("未来").
		Save(ctx); err != nil {
		t.Fatalf("create gamecharacter: %v", err)
	}

	query := renderdeck.AutoQuery{
		Region:                      "jp",
		RecommendType:               "challenge",
		ChallengeLiveCharacterQuery: "neo",
	}

	if err := resolveDeckCharacterSelections(context.Background(), &query, &renderapp.App{Sekai: sekaiClient}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.ChallengeLiveCharacterID != nil {
		t.Fatalf("unexpected challenge character id: %+v", query.ChallengeLiveCharacterID)
	}
	if query.MusicQuery != "neo" {
		t.Fatalf("unexpected fallback music query: %q", query.MusicQuery)
	}
	if query.ChallengeLiveCharacterQuery != "" {
		t.Fatalf("expected challenge query to be cleared: %q", query.ChallengeLiveCharacterQuery)
	}
}

func TestResolveDeckCharacterSelectionsFallbackChallengeQueryExtractsInlineDifficulty(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_deck_challenge_fallback_inline_diff?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	if _, err := sekaiClient.Gamecharacter.Create().
		SetServerRegion("jp").
		SetGameID(21).
		SetFirstName("初音").
		SetGivenName("未来").
		Save(ctx); err != nil {
		t.Fatalf("create gamecharacter: %v", err)
	}

	query := renderdeck.AutoQuery{
		Region:                      "jp",
		RecommendType:               "challenge",
		ChallengeLiveCharacterQuery: "群青ex",
	}

	if err := resolveDeckCharacterSelections(context.Background(), &query, &renderapp.App{Sekai: sekaiClient}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.ChallengeLiveCharacterID != nil {
		t.Fatalf("unexpected challenge character id: %+v", query.ChallengeLiveCharacterID)
	}
	if query.MusicQuery != "群青" {
		t.Fatalf("unexpected fallback music query: %q", query.MusicQuery)
	}
	if query.MusicDiff != "expert" {
		t.Fatalf("unexpected fallback music diff: %q", query.MusicDiff)
	}
	if query.ChallengeLiveCharacterQuery != "" {
		t.Fatalf("expected challenge query to be cleared: %q", query.ChallengeLiveCharacterQuery)
	}
}

func TestResolveDeckCharacterSelectionsFallsBackWorldBloomQueryToMusic(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_deck_world_bloom_fallback?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	if _, err := sekaiClient.Gamecharacter.Create().
		SetServerRegion("jp").
		SetGameID(21).
		SetFirstName("初音").
		SetGivenName("未来").
		Save(ctx); err != nil {
		t.Fatalf("create gamecharacter: %v", err)
	}

	query := renderdeck.AutoQuery{
		Region:                   "jp",
		RecommendType:            "event",
		EventID:                  drawing.IntPtr(123),
		WorldBloomCharacterQuery: "neo",
	}

	if err := resolveDeckCharacterSelections(context.Background(), &query, &renderapp.App{Sekai: sekaiClient}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.WorldBloomCharacterID != nil {
		t.Fatalf("unexpected world bloom character id: %+v", query.WorldBloomCharacterID)
	}
	if query.MusicQuery != "neo" {
		t.Fatalf("unexpected fallback music query: %q", query.MusicQuery)
	}
	if query.WorldBloomCharacterQuery != "" {
		t.Fatalf("expected world bloom query to be cleared: %q", query.WorldBloomCharacterQuery)
	}
}

func TestResolveDeckCharacterSelectionsResolvesExplicitWorldBloomSelector(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_deck_world_bloom_selector?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	for _, item := range []struct {
		id    int64
		first string
		last  string
	}{
		{id: 21, first: "初音", last: "未来"},
		{id: 24, first: "巡音", last: "流歌"},
	} {
		if _, err := sekaiClient.Gamecharacter.Create().
			SetServerRegion("jp").
			SetGameID(item.id).
			SetFirstName(item.first).
			SetGivenName(item.last).
			Save(ctx); err != nil {
			t.Fatalf("create gamecharacter %d: %v", item.id, err)
		}
	}
	seedBridgeTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 501, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []bridgeTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(3*time.Hour/time.Millisecond), aggregateAt: now - int64(time.Hour/time.Millisecond), characterID: 21},
		{chapterNo: 2, startAt: now + int64(time.Hour/time.Millisecond), aggregateAt: now + int64(2*time.Hour/time.Millisecond), characterID: 24},
	})

	query := renderdeck.AutoQuery{
		Region:                   "jp",
		RecommendType:            "event",
		EventID:                  drawing.IntPtr(501),
		WorldBloomCharacterQuery: "wl2",
	}

	if err := resolveDeckCharacterSelections(ctx, &query, &renderapp.App{Sekai: sekaiClient}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.WorldBloomCharacterID == nil || *query.WorldBloomCharacterID != 24 {
		t.Fatalf("unexpected world bloom character id: %+v", query.WorldBloomCharacterID)
	}
	if query.WorldBloomCharacterQuery != "" {
		t.Fatalf("expected world bloom query to be cleared: %q", query.WorldBloomCharacterQuery)
	}
}

func TestResolveDeckCharacterSelectionsResolvesDefaultWorldBloomChapterForExplicitEvent(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_deck_world_bloom_default_explicit?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	for _, item := range []struct {
		id    int64
		first string
		last  string
	}{
		{id: 21, first: "初音", last: "未来"},
		{id: 24, first: "巡音", last: "流歌"},
	} {
		if _, err := sekaiClient.Gamecharacter.Create().
			SetServerRegion("jp").
			SetGameID(item.id).
			SetFirstName(item.first).
			SetGivenName(item.last).
			Save(ctx); err != nil {
			t.Fatalf("create gamecharacter %d: %v", item.id, err)
		}
	}
	seedBridgeTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 502, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []bridgeTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(3*time.Hour/time.Millisecond), aggregateAt: now + int64(time.Hour/time.Millisecond), characterID: 21},
		{chapterNo: 2, startAt: now + int64(time.Hour/time.Millisecond), aggregateAt: now + int64(2*time.Hour/time.Millisecond), characterID: 24},
	})

	query := renderdeck.AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       drawing.IntPtr(502),
	}

	if err := resolveDeckCharacterSelections(ctx, &query, &renderapp.App{Sekai: sekaiClient}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.WorldBloomCharacterID == nil || *query.WorldBloomCharacterID != 21 {
		t.Fatalf("unexpected default world bloom character id: %+v", query.WorldBloomCharacterID)
	}
	if query.EventID == nil || *query.EventID != 502 {
		t.Fatalf("unexpected event id: %+v", query.EventID)
	}
}

func TestResolveDeckCharacterSelectionsUsesDefaultWorldBloomChapterAfterMusicFallback(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_deck_world_bloom_music_fallback?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	for _, item := range []struct {
		id    int64
		first string
		last  string
	}{
		{id: 21, first: "初音", last: "未来"},
		{id: 24, first: "巡音", last: "流歌"},
	} {
		if _, err := sekaiClient.Gamecharacter.Create().
			SetServerRegion("jp").
			SetGameID(item.id).
			SetFirstName(item.first).
			SetGivenName(item.last).
			Save(ctx); err != nil {
			t.Fatalf("create gamecharacter %d: %v", item.id, err)
		}
	}
	seedBridgeTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 504, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []bridgeTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(3*time.Hour/time.Millisecond), aggregateAt: now + int64(time.Hour/time.Millisecond), characterID: 21},
		{chapterNo: 2, startAt: now + int64(time.Hour/time.Millisecond), aggregateAt: now + int64(2*time.Hour/time.Millisecond), characterID: 24},
	})

	query := renderdeck.AutoQuery{
		Region:                   "jp",
		RecommendType:            "event",
		EventID:                  drawing.IntPtr(504),
		WorldBloomCharacterQuery: "虾",
	}

	if err := resolveDeckCharacterSelections(ctx, &query, &renderapp.App{Sekai: sekaiClient}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.WorldBloomCharacterID == nil || *query.WorldBloomCharacterID != 21 {
		t.Fatalf("unexpected default world bloom character id after music fallback: %+v", query.WorldBloomCharacterID)
	}
	if query.WorldBloomCharacterQuery != "" {
		t.Fatalf("expected world bloom query to be cleared after music fallback: %q", query.WorldBloomCharacterQuery)
	}
	if query.MusicQuery != "虾" {
		t.Fatalf("expected music query fallback to be preserved, got %q", query.MusicQuery)
	}
}

func TestResolveDeckCharacterSelectionsResolvesCurrentWorldBloomEventWhenEventIDMissing(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_deck_world_bloom_default_current?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	if _, err := sekaiClient.Gamecharacter.Create().
		SetServerRegion("jp").
		SetGameID(21).
		SetFirstName("初音").
		SetGivenName("未来").
		Save(ctx); err != nil {
		t.Fatalf("create gamecharacter: %v", err)
	}
	seedBridgeTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 503, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []bridgeTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(2*time.Hour/time.Millisecond), aggregateAt: now + int64(time.Hour/time.Millisecond), characterID: 21},
	})

	query := renderdeck.AutoQuery{
		Region:        "jp",
		RecommendType: "event",
	}

	if err := resolveDeckCharacterSelections(ctx, &query, &renderapp.App{Sekai: sekaiClient}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.EventID == nil || *query.EventID != 503 {
		t.Fatalf("unexpected event id: %+v", query.EventID)
	}
	if query.WorldBloomCharacterID == nil || *query.WorldBloomCharacterID != 21 {
		t.Fatalf("unexpected default world bloom character id: %+v", query.WorldBloomCharacterID)
	}
}

func TestResolveDeckCharacterSelectionsRejectsWorldBloomSelectorOnNonWorldBloomEvent(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_deck_non_wl_selector?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	seedBridgeTestRegularEvent(t, ctx, sekaiClient, "jp", 601, now-int64(time.Hour/time.Millisecond), now+int64(time.Hour/time.Millisecond))

	query := renderdeck.AutoQuery{
		Region:                   "jp",
		RecommendType:            "event",
		EventID:                  drawing.IntPtr(601),
		WorldBloomCharacterQuery: "wl1",
	}

	err := resolveDeckCharacterSelections(ctx, &query, &renderapp.App{Sekai: sekaiClient})
	if err == nil {
		t.Fatalf("expected non-world bloom event to reject wl selector")
	}
	if !strings.Contains(err.Error(), "不是WL活动") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveDeckCharacterSelectionsClearsWorldBloomCharacterForNonWorldBloomEvent(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_deck_non_wl_character?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	seedBridgeTestRegularEvent(t, ctx, sekaiClient, "jp", 602, now-int64(time.Hour/time.Millisecond), now+int64(time.Hour/time.Millisecond))

	query := renderdeck.AutoQuery{
		Region:                "jp",
		RecommendType:         "event",
		EventID:               drawing.IntPtr(602),
		WorldBloomCharacterID: drawing.IntPtr(21),
	}

	if err := resolveDeckCharacterSelections(ctx, &query, &renderapp.App{Sekai: sekaiClient}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.WorldBloomCharacterID != nil {
		t.Fatalf("expected world bloom character id to be cleared: %+v", query.WorldBloomCharacterID)
	}
}

type bridgeTestWorldBloomChapter struct {
	chapterNo   int64
	startAt     int64
	aggregateAt int64
	characterID int64
}

func seedBridgeTestWorldBloomEvent(
	t *testing.T,
	ctx context.Context,
	sekaiClient *sekaidb.Client,
	region string,
	eventID int64,
	startAt int64,
	aggregateAt int64,
	chapters []bridgeTestWorldBloomChapter,
) {
	t.Helper()

	if _, err := sekaiClient.Event.Create().
		SetServerRegion(region).
		SetGameID(eventID).
		SetEventType("world_bloom").
		SetName(fmt.Sprintf("WL %d", eventID)).
		SetAssetbundleName(fmt.Sprintf("wl_%d", eventID)).
		SetStartAt(startAt).
		SetAggregateAt(aggregateAt).
		SetClosedAt(aggregateAt + int64(time.Hour/time.Millisecond)).
		Save(ctx); err != nil {
		t.Fatalf("create wl event %d: %v", eventID, err)
	}

	for _, chapter := range chapters {
		create := sekaiClient.Worldbloom.Create().
			SetServerRegion(region).
			SetEventID(eventID).
			SetChapterNo(chapter.chapterNo).
			SetChapterStartAt(chapter.startAt).
			SetAggregateAt(chapter.aggregateAt)
		if chapter.characterID > 0 {
			create.SetGameCharacterID(chapter.characterID)
		}
		if _, err := create.Save(ctx); err != nil {
			t.Fatalf("create wl chapter %d for event %d: %v", chapter.chapterNo, eventID, err)
		}
	}
}

func seedBridgeTestRegularEvent(
	t *testing.T,
	ctx context.Context,
	sekaiClient *sekaidb.Client,
	region string,
	eventID int64,
	startAt int64,
	aggregateAt int64,
) {
	t.Helper()

	if _, err := sekaiClient.Event.Create().
		SetServerRegion(region).
		SetGameID(eventID).
		SetEventType("marathon").
		SetName(fmt.Sprintf("EV %d", eventID)).
		SetAssetbundleName(fmt.Sprintf("event_%d", eventID)).
		SetStartAt(startAt).
		SetAggregateAt(aggregateAt).
		SetClosedAt(aggregateAt + int64(time.Hour/time.Millisecond)).
		Save(ctx); err != nil {
		t.Fatalf("create event %d: %v", eventID, err)
	}
}

func TestResolveTrackerCharacterSelectionUsesWorldBloomCharacterQuery(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_tracker_character?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	if _, err := sekaiClient.Gamecharacter.Create().
		SetServerRegion("jp").
		SetGameID(21).
		SetFirstName("初音").
		SetGivenName("未来").
		SetFirstNameEnglish("Hatsune").
		SetGivenNameEnglish("Miku").
		Save(ctx); err != nil {
		t.Fatalf("create gamecharacter: %v", err)
	}
	seedBridgeTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 101, now-int64(2*time.Hour/time.Millisecond), now+int64(2*time.Hour/time.Millisecond), []bridgeTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(time.Hour/time.Millisecond), aggregateAt: now + int64(time.Hour/time.Millisecond), characterID: 21},
	})

	req := rendersk.TrackerRankQuery{
		Region:           "jp",
		EventID:          101,
		Ranks:            []int{100},
		WlCharacterQuery: "Hatsune Miku",
	}

	if err := resolveTrackerCharacterSelection(ctx, &renderapp.App{Sekai: sekaiClient}, &req); err != nil {
		t.Fatalf("resolveTrackerCharacterSelection() error = %v", err)
	}
	if req.WlCharacterID == nil || *req.WlCharacterID != 21 {
		t.Fatalf("unexpected wl character id: %+v", req.WlCharacterID)
	}
	if req.WlCharacterQuery != "" {
		t.Fatalf("expected wl character query to be cleared: %q", req.WlCharacterQuery)
	}
}

func TestResolveTrackerCharacterSelectionResolvesCurrentWorldBloomChapter(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_tracker_wl_current?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	for _, item := range []struct {
		id    int64
		first string
		last  string
	}{
		{id: 21, first: "初音", last: "未来"},
		{id: 24, first: "巡音", last: "流歌"},
	} {
		if _, err := sekaiClient.Gamecharacter.Create().
			SetServerRegion("jp").
			SetGameID(item.id).
			SetFirstName(item.first).
			SetGivenName(item.last).
			Save(ctx); err != nil {
			t.Fatalf("create gamecharacter %d: %v", item.id, err)
		}
	}
	seedBridgeTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 201, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []bridgeTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(3*time.Hour/time.Millisecond), aggregateAt: now - int64(time.Hour/time.Millisecond), characterID: 21},
		{chapterNo: 2, startAt: now + int64(time.Hour/time.Millisecond), aggregateAt: now + int64(2*time.Hour/time.Millisecond), characterID: 24},
	})

	req := rendersk.TrackerRankQuery{
		Region:           "jp",
		Ranks:            []int{100},
		WlCharacterQuery: "wl",
	}

	if err := resolveTrackerCharacterSelection(ctx, &renderapp.App{Sekai: sekaiClient}, &req); err != nil {
		t.Fatalf("resolveTrackerCharacterSelection() error = %v", err)
	}
	if req.EventID != 201 {
		t.Fatalf("expected current wl event 201, got %d", req.EventID)
	}
	if req.WlCharacterID == nil || *req.WlCharacterID != 21 {
		t.Fatalf("unexpected wl character id: %+v", req.WlCharacterID)
	}
}

func TestResolveTrackerCharacterSelectionResolvesWorldBloomChapterSelector(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_tracker_wl_selector?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	for _, item := range []struct {
		id    int64
		first string
		last  string
	}{
		{id: 21, first: "初音", last: "未来"},
		{id: 24, first: "巡音", last: "流歌"},
	} {
		if _, err := sekaiClient.Gamecharacter.Create().
			SetServerRegion("jp").
			SetGameID(item.id).
			SetFirstName(item.first).
			SetGivenName(item.last).
			Save(ctx); err != nil {
			t.Fatalf("create gamecharacter %d: %v", item.id, err)
		}
	}
	seedBridgeTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 301, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []bridgeTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(3*time.Hour/time.Millisecond), aggregateAt: now - int64(time.Hour/time.Millisecond), characterID: 21},
		{chapterNo: 2, startAt: now + int64(time.Hour/time.Millisecond), aggregateAt: now + int64(2*time.Hour/time.Millisecond), characterID: 24},
	})

	req := rendersk.TrackerRankQuery{
		Region:           "jp",
		Ranks:            []int{100},
		WlCharacterQuery: "wl2",
	}

	if err := resolveTrackerCharacterSelection(ctx, &renderapp.App{Sekai: sekaiClient}, &req); err != nil {
		t.Fatalf("resolveTrackerCharacterSelection() error = %v", err)
	}
	if req.EventID != 301 {
		t.Fatalf("expected current wl event 301, got %d", req.EventID)
	}
	if req.WlCharacterID == nil || *req.WlCharacterID != 24 {
		t.Fatalf("unexpected wl character id: %+v", req.WlCharacterID)
	}
}

func TestResolveTrackerCharacterSelectionFallsBackToPreviousWorldBloomEvent(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_tracker_wl_prev?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	for _, item := range []struct {
		id    int64
		first string
		last  string
	}{
		{id: 21, first: "初音", last: "未来"},
		{id: 24, first: "巡音", last: "流歌"},
	} {
		if _, err := sekaiClient.Gamecharacter.Create().
			SetServerRegion("jp").
			SetGameID(item.id).
			SetFirstName(item.first).
			SetGivenName(item.last).
			Save(ctx); err != nil {
			t.Fatalf("create gamecharacter %d: %v", item.id, err)
		}
	}
	seedBridgeTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 401, now-int64(72*time.Hour/time.Millisecond), now-int64(48*time.Hour/time.Millisecond), []bridgeTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(70*time.Hour/time.Millisecond), aggregateAt: now - int64(66*time.Hour/time.Millisecond), characterID: 21},
		{chapterNo: 2, startAt: now - int64(60*time.Hour/time.Millisecond), aggregateAt: now - int64(56*time.Hour/time.Millisecond), characterID: 24},
	})

	req := rendersk.TrackerRankQuery{
		Region:           "jp",
		Ranks:            []int{100},
		WlCharacterQuery: "wl",
	}

	if err := resolveTrackerCharacterSelection(ctx, &renderapp.App{Sekai: sekaiClient}, &req); err != nil {
		t.Fatalf("resolveTrackerCharacterSelection() error = %v", err)
	}
	if req.EventID != 401 {
		t.Fatalf("expected previous wl event 401, got %d", req.EventID)
	}
	if req.WlCharacterID == nil || *req.WlCharacterID != 24 {
		t.Fatalf("unexpected wl character id: %+v", req.WlCharacterID)
	}
}

func TestResolveGameCharacterIDByQueryUsesApprovedAlias(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_character_alias_sekai?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	pjskClient := pjskenttest.Open(t, "sqlite3", "file:bridge_test_character_alias_pjsk?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = pjskClient.Close() })

	if _, err := sekaiClient.Gamecharacter.Create().
		SetServerRegion("jp").
		SetGameID(21).
		SetFirstName("初音").
		SetGivenName("未来").
		SetFirstNameEnglish("Hatsune").
		SetGivenNameEnglish("Miku").
		Save(ctx); err != nil {
		t.Fatalf("create gamecharacter: %v", err)
	}
	if _, err := pjskClient.Alias.Create().
		SetAliasType(pjskalias.AliasTypeCharacter).
		SetAliasTypeID(21).
		SetAlias("葱").
		Save(ctx); err != nil {
		t.Fatalf("create alias: %v", err)
	}

	app := &renderapp.App{
		Sekai:   sekaiClient,
		Aliases: pjskalias.NewService(sekaiClient, pjskClient, nil),
	}
	charID, err := resolveGameCharacterIDByQuery(ctx, app, renderregion.JP, "葱", "bridge test")
	if err != nil {
		t.Fatalf("resolveGameCharacterIDByQuery() error = %v", err)
	}
	if charID != 21 {
		t.Fatalf("unexpected character id: %d", charID)
	}
}

func TestExecuteMysekaiPhoto(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/image/jp/mysekai/photos/test" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Haruki-Sekai-Token"); got != "test-token" {
			t.Fatalf("unexpected token: %q", got)
		}
		_, _ = w.Write([]byte("image-bytes"))
	}))
	defer server.Close()

	oldBaseURL := config.Cfg.SekaiAPI.BaseURL
	oldToken := config.Cfg.SekaiAPI.Token
	config.Cfg.SekaiAPI.BaseURL = server.URL
	config.Cfg.SekaiAPI.Token = "test-token"
	t.Cleanup(func() {
		config.Cfg.SekaiAPI.BaseURL = oldBaseURL
		config.Cfg.SekaiAPI.Token = oldToken
	})

	root := t.TempDir()
	userPath := filepath.Join(root, "user.json")
	mysekaiPath := filepath.Join(root, "mysekai.json")
	if err := os.WriteFile(userPath, []byte(`{
  "now": 1700000000,
  "userGamedata": {"userId": 12345678901234, "name": "Tester", "deck": 1},
  "userProfile": {},
  "userDecks": [{"deckId": 1}],
  "userCards": []
}`), 0o644); err != nil {
		t.Fatalf("write user snapshot: %v", err)
	}
	if err := os.WriteFile(mysekaiPath, []byte(`{
  "updatedResources": {
    "userMysekaiPhotos": [
      {"seq": 1, "obtainedAt": 1700000000000, "imagePath": "photos/test"}
    ]
  }
}`), 0o644); err != nil {
		t.Fatalf("write mysekai snapshot: %v", err)
	}

	snapshot := userdata.NewLocalFileService(nil, nil, userdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MySekaiJSON:   mysekaiPath,
	})
	app := &renderapp.App{
		MySekai:    rendermysekai.NewController(nil, snapshot, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true}),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	params, err := json.Marshal(rendermysekai.PhotoQuery{Seq: 1})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	message, err := executeMysekai(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleMysekai,
		Mode:   "mysekai-photo",
		Region: "jp",
		Params: params,
	}, app))
	if err != nil {
		t.Fatalf("executeMysekai photo: %v", err)
	}
	if len(message) != 2 || message[0].Type != "image" || message[1].Type != "text" {
		t.Fatalf("unexpected mysekai photo message: %+v", message)
	}
	textData, ok := message[1].Data.(onebot11.TextData)
	if !ok {
		t.Fatalf("unexpected text data type: %T", message[1].Data)
	}
	if !strings.HasPrefix(textData.Text, "拍摄时间: ") {
		t.Fatalf("unexpected photo text: %q", textData.Text)
	}
}

type bridgeVLiveSource struct {
	lives []*rendervlive.Live
}

func (s *bridgeVLiveSource) DefaultRegion() renderregion.Value { return renderregion.JP }

func (s *bridgeVLiveSource) GetLives(region renderregion.Value) ([]*rendervlive.Live, error) {
	return s.lives, nil
}

func TestExecuteVLiveReturnsText(t *testing.T) {
	now := time.Now()
	ms := func(tm time.Time) int64 { return tm.UnixMilli() }

	app := &renderapp.App{
		VLive: rendervlive.NewController(&bridgeVLiveSource{
			lives: []*rendervlive.Live{
				{
					ID:      3001,
					Name:    "Test Virtual Live",
					StartAt: ms(now.Add(time.Hour)),
					EndAt:   ms(now.Add(2 * time.Hour)),
					Schedules: []rendervlive.Schedule{
						{StartAt: ms(now.Add(time.Hour)), EndAt: ms(now.Add(2 * time.Hour))},
					},
				},
			},
		}, renderregion.JP),
	}

	params, err := json.Marshal(rendervlive.ListQuery{})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	message, err := executeVLive(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleVLive,
		Mode:   "vlive-list",
		Region: "jp",
		Params: params,
	}, app))
	if err != nil {
		t.Fatalf("executeVLive: %v", err)
	}
	if len(message) != 1 || message[0].Type != "text" {
		t.Fatalf("unexpected vlive message: %+v", message)
	}
	textData, ok := message[0].Data.(onebot11.TextData)
	if !ok {
		t.Fatalf("unexpected text data type: %T", message[0].Data)
	}
	if !strings.Contains(textData.Text, "Test Virtual Live") || !strings.Contains(textData.Text, "虚拟Live列表") {
		t.Fatalf("unexpected vlive text: %q", textData.Text)
	}
}

type bridgeCardSource struct {
	region           renderregion.Value
	cards            map[int]*masterdata.Card
	allowEmptyFilter bool
}

func (s *bridgeCardSource) DefaultRegion() renderregion.Value {
	if s.region.IsZero() {
		return renderregion.JP
	}
	return s.region
}

func (s *bridgeCardSource) GetCardByID(id int) (*masterdata.Card, error) {
	if s.cards != nil {
		if card := s.cards[id]; card != nil {
			copy := *card
			return &copy, nil
		}
		return nil, os.ErrNotExist
	}
	if id == 1001 {
		return &masterdata.Card{
			ID:              1001,
			CharacterID:     5,
			CardRarityType:  "rarity_4",
			Attr:            "cute",
			Prefix:          "Test Card",
			AssetBundleName: "card_test",
		}, nil
	}
	return nil, os.ErrNotExist
}

func (s *bridgeCardSource) GetCardByCharacterAndSeq(characterID, seq int) (*masterdata.Card, error) {
	return nil, os.ErrNotExist
}

func (s *bridgeCardSource) FilterCards(info *rendercard.CardQueryInfo) ([]*masterdata.Card, error) {
	if s.allowEmptyFilter && info != nil && info.CharacterID == 0 && info.Rarity == "" && info.Attr == "" &&
		info.SkillType == "" && info.Unit == "" && info.MainUnit == "" && info.SupportUnit == "" &&
		info.SupplyType == "" && info.Year == 0 && info.EventID == 0 && info.BanCharID == 0 && info.BanSeq == 0 {
		result := make([]*masterdata.Card, 0, len(s.cards))
		for _, card := range s.cards {
			if card == nil {
				continue
			}
			copy := *card
			result = append(result, &copy)
		}
		return result, nil
	}
	return nil, nil
}

func (s *bridgeCardSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	return nil, os.ErrNotExist
}

func (s *bridgeCardSource) GetCharacterColorCode(id int) (string, bool) {
	return "", false
}

func (s *bridgeCardSource) GetUnitByCardID(cardID int) (string, error) { return "", nil }

func (s *bridgeCardSource) GetCardSupplyType(card *masterdata.Card) string { return "" }

func (s *bridgeCardSource) GetSkillByID(id int) (*masterdata.Skill, error) {
	return nil, os.ErrNotExist
}

func (s *bridgeCardSource) FormatSkillDescription(skill *masterdata.Skill, cardCharacterID int) string {
	return ""
}

func (s *bridgeCardSource) GetGachaByCardID(cardID int) (*masterdata.Gacha, error) {
	return nil, os.ErrNotExist
}

func (s *bridgeCardSource) GetCostume3dsByCardID(cardID int) ([]*masterdata.Costume3d, error) {
	return nil, nil
}

type bridgeCardEventSource struct{}

func (s *bridgeCardEventSource) DefaultRegion() renderregion.Value { return renderregion.JP }
func (s *bridgeCardEventSource) GetEventByID(id int) (*masterdata.Event, error) {
	return nil, os.ErrNotExist
}
func (s *bridgeCardEventSource) GetEventByCardID(cardID int) (*masterdata.Event, error) {
	return nil, os.ErrNotExist
}
func (s *bridgeCardEventSource) GetEvents() []*masterdata.Event { return nil }
func (s *bridgeCardEventSource) GetEventCards(eventID int) ([]*masterdata.Card, error) {
	return nil, nil
}
func (s *bridgeCardEventSource) GetEventBannerCharacterID(eventID int) (int, error) {
	return 0, os.ErrNotExist
}
func (s *bridgeCardEventSource) GetEventDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	return nil, nil
}
func (s *bridgeCardEventSource) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	return nil, os.ErrNotExist
}
func (s *bridgeCardEventSource) GetBanEvents(charID int) []*masterdata.Event { return nil }
func (s *bridgeCardEventSource) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom {
	return nil
}
func (s *bridgeCardEventSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	return nil, os.ErrNotExist
}

func TestExecuteCardImageReturnsAllOriginalArts(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		filepath.Join("asset", "jp-assets", "startapp", "character", "member", "card_test", "card_normal.png"),
	} {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte("png"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	app := &renderapp.App{
		Cards:      rendercard.NewController(&bridgeCardSource{}, &bridgeCardEventSource{}, nil, assets.NewAssetHelper(root, nil)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}
	message, err := executeCard(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleCard,
		Mode:   "card-image",
		Query:  "1001",
		Region: "jp",
	}, app))
	if err != nil {
		t.Fatalf("executeCard image: %v", err)
	}
	if len(message) != 2 {
		t.Fatalf("expected 2 image segments, got %+v", message)
	}
	for _, segment := range message {
		if segment.Type != "image" {
			t.Fatalf("unexpected segment: %+v", segment)
		}
	}
}

func TestExecuteCardBoxPassesDisplayFlagsToDrawing(t *testing.T) {
	root := t.TempDir()
	var captured drawing.CardBoxRequest
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/card/box" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	app := &renderapp.App{
		Cards:      rendercard.NewController(&bridgeCardSource{}, &bridgeCardEventSource{}, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper(root, nil)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}
	params, err := json.Marshal(map[string]any{
		"show_id":            true,
		"show_box":           false,
		"use_after_training": false,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeCard(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleCard,
		Mode:   "card-box",
		Query:  "1001",
		Region: "jp",
		Params: params,
	}, app))
	if err != nil {
		t.Fatalf("executeCard box: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected message: %+v", message)
	}
	if !captured.ShowID || captured.ShowBox {
		t.Fatalf("unexpected box flags: %+v", captured)
	}
	if len(captured.Cards) != 1 || captured.Cards[0].Card.IsAfterTraining == nil || *captured.Cards[0].Card.IsAfterTraining {
		t.Fatalf("unexpected card payload: %+v", captured.Cards)
	}
}

func TestExecuteCardBoxRequiresOwnedCardDataWhenShowBoxEnabled(t *testing.T) {
	root := t.TempDir()
	app := &renderapp.App{
		Cards: rendercard.NewController(&bridgeCardSource{}, &bridgeCardEventSource{}, drawing.NewHarukiDrawingClient("https://drawing.invalid"), assets.NewAssetHelper(root, nil)),
	}
	params, err := json.Marshal(map[string]any{
		"show_box": true,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	_, err = executeCard(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleCard,
		Mode:   "card-box",
		Query:  "1001",
		Region: "jp",
		Params: params,
	}, app))
	if err == nil {
		t.Fatal("expected missing owned-card data to fail")
	}
	if !strings.Contains(err.Error(), "box 模式需要用户卡牌持有数据") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteCardListKeepsResolvedRegionInsteadOfStaleParamRegion(t *testing.T) {
	root := t.TempDir()
	var captured drawing.CardListRequest
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/card/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	jpSource := &bridgeCardSource{region: renderregion.JP, cards: map[int]*masterdata.Card{}}
	cnSource := &bridgeCardSource{
		region: renderregion.CN,
		cards: map[int]*masterdata.Card{
			1001: {
				ID:              1001,
				CharacterID:     5,
				CardRarityType:  "rarity_4",
				Attr:            "cute",
				Prefix:          "CN Test Card",
				AssetBundleName: "card_cn_test",
			},
		},
	}
	cardController := rendercard.NewController(jpSource, &bridgeCardEventSource{}, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper(root, nil))
	cardController.RegisterSource(cnSource)

	app := &renderapp.App{
		Cards:      cardController,
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}
	params, err := json.Marshal(rendercard.ListRequest{
		Query:  "1001",
		Region: "jp",
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeCard(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleCard,
		Mode:   "card-list",
		Region: "cn",
		Params: params,
	}, app))
	if err != nil {
		t.Fatalf("executeCard list: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected message: %+v", message)
	}
	if captured.Region != "cn" {
		t.Fatalf("expected rendered region cn, got %q", captured.Region)
	}
	if len(captured.Cards) != 1 || captured.Cards[0].CardID != 1001 {
		t.Fatalf("unexpected rendered cards: %+v", captured.Cards)
	}
}
