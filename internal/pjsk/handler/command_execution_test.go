package handler

import (
	"bytes"
	"context"
	json "github.com/bytedance/sonic"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"haruki-cloud/config"
	pjskenttest "haruki-cloud/database/pjsk/enttest"
	sekaidb "haruki-cloud/database/sekai"
	sekaienttest "haruki-cloud/database/sekai/enttest"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	pjskalias "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/music"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	renderprofile "haruki-cloud/internal/pjsk/render/profile"
	renderprovider "haruki-cloud/internal/pjsk/render/provider"
	renderscore "haruki-cloud/internal/pjsk/render/score"
	rendersk "haruki-cloud/internal/pjsk/render/sk"
	"haruki-cloud/internal/pjsk/render/snapshot"
	rendervlive "haruki-cloud/internal/pjsk/render/vlive"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/imagecache"

	_ "github.com/mattn/go-sqlite3"
)

type handlerTestBindingValidator struct{}

func (handlerTestBindingValidator) GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
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

type handlerMultiRegionBindingValidator struct {
	profiles map[string]map[string]string
}

type handlerTestFastVerifier struct {
	bindings []sekaiapi.UserGameBinding
	err      error
}

func (f handlerTestFastVerifier) GetToolboxUserFastVerificationGameAccountBindings(platform, platformUserID string) ([]sekaiapi.UserGameBinding, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]sekaiapi.UserGameBinding(nil), f.bindings...), nil
}

func (v handlerMultiRegionBindingValidator) GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
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

func newHandlerTestBindingServiceWithValidator(t *testing.T, validator accountdata.ProfileValidator) *accountdata.BindingService {
	t.Helper()
	pjskClient := pjskenttest.Open(t, "sqlite3", "file:handler_test_bind?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = pjskClient.Close() })
	usersClient := usersenttest.Open(t, "sqlite3", "file:handler_test_users?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = usersClient.Close() })
	return accountdata.NewBindingService(
		pjskClient,
		identity.NewResolver(usersClient),
		validator,
	)
}

func newHandlerTestBindingService(t *testing.T) *accountdata.BindingService {
	t.Helper()
	return newHandlerTestBindingServiceWithValidator(t, handlerTestBindingValidator{})
}

func mustEncodeTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(40 + x), G: uint8(90 + y), B: 180, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestExecuteCheckDataMySekaiRequiresVisibleMySekaiSnapshot(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)

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

	_, err = executeCheckData(NewRequestContext(ctx, &CommandRequest{
		Module: parser.ModuleCheckData,
		Mode:   "mysekai",
		Region: "jp",
		Params: params,
	}, &renderapp.App{Bindings: service}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != ErrMsgMySekaiDataNotFound {
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

	req, ok := trackerRankQueryFromParams(&CommandRequest{
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

	req, ok := trackerRankQueryFromParams(&CommandRequest{
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

	req, ok := trackerRankQueryFromParams(&CommandRequest{
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
	service := newHandlerTestBindingServiceWithValidator(t, handlerMultiRegionBindingValidator{
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
	service := newHandlerTestBindingService(t)

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
	service := newHandlerTestBindingService(t)

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
	service := newHandlerTestBindingService(t)

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
	service := newHandlerTestBindingServiceWithValidator(t, handlerMultiRegionBindingValidator{
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
		RegionExplicit: true,
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

type bridgeAmbiguousMusicAliasResolver struct{}

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

func (r *bridgeAmbiguousMusicAliasResolver) TryResolveMusicID(_ context.Context, token string) (int, bool, error) {
	return r.TryResolveMusicTitleOrAliasID(context.Background(), token)
}

func (r *bridgeAmbiguousMusicAliasResolver) TryResolveMusicTitleOrAliasID(_ context.Context, token string) (int, bool, error) {
	if strings.ToLower(strings.TrimSpace(token)) != "shared alias" {
		return 0, false, nil
	}
	return 0, false, fmt.Errorf("别名匹配到多个歌曲，请改用 music<id> 查询：\nmusic1/Song A\nmusic2/Song B")
}

func (s *bridgeMusicSource) SearchMusic(query string) (*masterdata.Music, error) {
	for _, item := range s.musics {
		if strings.EqualFold(item.Title, query) {
			return item, nil
		}
	}
	return nil, os.ErrNotExist
}

func (s *bridgeMusicSource) GetMusicByID(id int) (*masterdata.Music, error) {
	item := s.musics[id]
	if item == nil {
		return nil, os.ErrNotExist
	}
	return item, nil
}

func (s *bridgeMusicSource) GetMusicByEventID(int) (*masterdata.Music, error) {
	return nil, os.ErrNotExist
}

func (s *bridgeMusicSource) GetMusics() []*masterdata.Music {
	out := make([]*masterdata.Music, 0, len(s.musics))
	for _, item := range s.musics {
		out = append(out, item)
	}
	return out
}

func (s *bridgeMusicSource) GetBanEvents(int) []*masterdata.Event { return nil }

func (s *bridgeMusicSource) GetMusicLocalizedTitles(int) ([]string, error) { return nil, nil }

func (s *bridgeMusicSource) GetMusicDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error) {
	items := s.difficulties[musicID]
	out := make([]*masterdata.MusicDifficulty, 0, len(items))
	out = append(out, items...)
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
			if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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

	message, err := executeMusic(NewRequestContext(context.Background(), &CommandRequest{
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
	message, err = executeMusic(NewRequestContext(context.Background(), &CommandRequest{
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

func TestExecuteMusicChartUsesStaticChartCachePath(t *testing.T) {
	root := t.TempDir()
	chartCacheDir := t.TempDir()

	chartCalls := 0
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/pjsk/chart":
			chartCalls++
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
		Music: music.NewController(source, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper(root, nil), nil, nil),
		Config: renderapp.Config{
			ImageCacheURI: "https://image-cache.test",
			ChartsBaseURL: "https://charts.test",
			ImageCacheDir: chartCacheDir,
		},
	}

	params, err := json.Marshal(map[string]string{
		"difficulty": "expert",
		"style":      "white",
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	request := &CommandRequest{
		Module: parser.ModuleMusic,
		Mode:   "music-chart",
		Query:  "Song A",
		Region: "jp",
		Params: params,
	}

	first, err := executeMusic(NewRequestContext(context.Background(), request, app))
	if err != nil {
		t.Fatalf("executeMusic chart first: %v", err)
	}
	if len(first) != 1 || first[0].Type != onebot11.TypeImage {
		t.Fatalf("unexpected first chart message: %+v", first)
	}
	imageData, ok := first[0].Data.(onebot11.ImageData)
	if !ok {
		t.Fatalf("unexpected image data: %#v", first[0].Data)
	}
	if imageData.File != "https://charts.test/white/jp/1/expert/no-skill.png" {
		t.Fatalf("unexpected chart cache url: %q", imageData.File)
	}

	expectedPath := filepath.Join(chartCacheDir, "charts", "white", "jp", "1", "expert", "no-skill.png")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected chart cache file %s: %v", expectedPath, err)
	}

	second, err := executeMusic(NewRequestContext(context.Background(), request, app))
	if err != nil {
		t.Fatalf("executeMusic chart second: %v", err)
	}
	if len(second) != 1 || second[0].Type != onebot11.TypeImage {
		t.Fatalf("unexpected second chart message: %+v", second)
	}
	if chartCalls != 1 {
		t.Fatalf("expected 1 chart render call after cache hit, got %d", chartCalls)
	}
}

func TestExecuteMusicChartUsesSkillSpecificStaticChartCachePath(t *testing.T) {
	root := t.TempDir()
	chartCacheDir := t.TempDir()

	chartCalls := 0
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/pjsk/chart":
			chartCalls++
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
		Music: music.NewController(source, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper(root, nil), nil, nil),
		Config: renderapp.Config{
			ImageCacheURI: "https://image-cache.test",
			ChartsBaseURL: "https://charts.test",
			ImageCacheDir: chartCacheDir,
		},
	}

	params, err := json.Marshal(map[string]any{
		"difficulty": "expert",
		"style":      "white",
		"skill":      true,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	request := &CommandRequest{
		Module: parser.ModuleMusic,
		Mode:   "music-chart",
		Query:  "Song A",
		Region: "jp",
		Params: params,
	}

	first, err := executeMusic(NewRequestContext(context.Background(), request, app))
	if err != nil {
		t.Fatalf("executeMusic skill chart first: %v", err)
	}
	if len(first) != 1 || first[0].Type != onebot11.TypeImage {
		t.Fatalf("unexpected first skill chart message: %+v", first)
	}
	imageData, ok := first[0].Data.(onebot11.ImageData)
	if !ok {
		t.Fatalf("unexpected image data: %#v", first[0].Data)
	}
	if imageData.File != "https://charts.test/white/jp/1/expert/skill.png" {
		t.Fatalf("unexpected skill chart cache url: %q", imageData.File)
	}

	expectedPath := filepath.Join(chartCacheDir, "charts", "white", "jp", "1", "expert", "skill.png")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected skill chart cache file %s: %v", expectedPath, err)
	}

	second, err := executeMusic(NewRequestContext(context.Background(), request, app))
	if err != nil {
		t.Fatalf("executeMusic skill chart second: %v", err)
	}
	if len(second) != 1 || second[0].Type != onebot11.TypeImage {
		t.Fatalf("unexpected second skill chart message: %+v", second)
	}
	if chartCalls != 1 {
		t.Fatalf("expected 1 chart render call after skill cache hit, got %d", chartCalls)
	}
}

func TestExecuteMusicBPMUsesSingleMusicListImageForMixedDifficulties(t *testing.T) {
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
	titles := make([]string, 0, 1)
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/pjsk/music/brief-list":
			briefListCalls++
			var req drawing.MusicBriefListRequest
			if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode music-brief-list request: %v", err)
			}
			if req.Title == nil {
				t.Fatalf("expected list title, got nil")
			}
			titles = append(titles, *req.Title)
			if len(req.MusicList) != 2 {
				t.Fatalf("expected 2 list items in single request, got %d", len(req.MusicList))
			}
			gotIDs := []int{
				req.MusicList[0].ID,
				req.MusicList[1].ID,
			}
			wantIDs := []int{1, 2}
			for i := range wantIDs {
				if gotIDs[i] != wantIDs[i] {
					t.Fatalf("unexpected bpm song list ids: got=%v want=%v", gotIDs, wantIDs)
				}
			}
			if len(req.MusicList[0].Difficulty.Order) < 2 {
				t.Fatalf("expected full difficulty info for first song, got %+v", req.MusicList[0].Difficulty)
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

	message, err := executeMusic(NewRequestContext(context.Background(), &CommandRequest{
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
		t.Fatalf("expected 1 music-brief-list render call, got %d", briefListCalls)
	}
	if len(titles) != 1 || titles[0] != "BPM 200 匹配结果" {
		t.Fatalf("unexpected title list: %+v", titles)
	}
}

func TestExecuteMusicDetailUsesBriefListForAmbiguousAlias(t *testing.T) {
	briefListCalls := 0
	titles := make([]string, 0, 1)
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/pjsk/music/brief-list":
			briefListCalls++
			var req drawing.MusicBriefListRequest
			if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode music-brief-list request: %v", err)
			}
			if req.Title == nil {
				t.Fatalf("expected list title, got nil")
			}
			titles = append(titles, *req.Title)
			if len(req.MusicList) != 2 {
				t.Fatalf("expected 2 list items, got %d", len(req.MusicList))
			}
			if req.MusicList[0].ID != 1 || req.MusicList[1].ID != 2 {
				t.Fatalf("unexpected ambiguous alias ids: %+v", req.MusicList)
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
			1: {{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31}},
			2: {{MusicID: 2, MusicDifficulty: "master", PlayLevel: 32}},
		},
	}
	ctrl := music.NewController(source, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper("", nil), nil, nil)
	ctrl.SetAliasResolver(&bridgeAmbiguousMusicAliasResolver{})
	app := &renderapp.App{
		Music:      ctrl,
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	message, err := executeMusic(NewRequestContext(context.Background(), &CommandRequest{
		Module: parser.ModuleMusic,
		Mode:   "music-detail",
		Query:  "Shared Alias",
		Region: "jp",
	}, app))
	if err != nil {
		t.Fatalf("executeMusic detail: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected detail message: %+v", message)
	}
	if briefListCalls != 1 {
		t.Fatalf("expected 1 brief-list render call, got %d", briefListCalls)
	}
	if len(titles) != 1 || titles[0] != "匹配到多个歌曲，请使用 /查歌 <id> 查询：" {
		t.Fatalf("unexpected title list: %+v", titles)
	}
}

func TestExecuteMusicNoteCountUsesSingleMusicListImageWithoutSummaryText(t *testing.T) {
	listCalls := 0
	titles := make([]string, 0, 1)
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/pjsk/music/list":
			listCalls++
			var req drawing.MusicListRequest
			if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode music-list request: %v", err)
			}
			if req.Title == nil {
				t.Fatalf("expected list title, got nil")
			}
			titles = append(titles, *req.Title)
			if req.Profile != nil {
				t.Fatalf("expected nil profile for lookup list, got %+v", req.Profile)
			}
			if len(req.MusicList) != 2 {
				t.Fatalf("expected 2 list items in single request, got %d", len(req.MusicList))
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
				{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 27, TotalNoteCount: 777},
			},
			2: {
				{MusicID: 2, MusicDifficulty: "expert", PlayLevel: 28, TotalNoteCount: 777},
			},
		},
	}
	app := &renderapp.App{
		Music:      music.NewController(source, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper("", nil), nil, nil),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	params, err := json.Marshal(map[string]any{"note_count": 777, "difficulty": "expert"})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeMusic(NewRequestContext(context.Background(), &CommandRequest{
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
	if listCalls != 1 {
		t.Fatalf("expected 1 music-list render call, got %d", listCalls)
	}
	if len(titles) != 1 || titles[0] != "物量 777 EXPERT 匹配结果" {
		t.Fatalf("unexpected title list: %+v", titles)
	}
}

func TestExecuteMusicListUsesQueryKeywordAndAlias(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/music/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req drawing.MusicListRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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

	message, err := executeMusic(NewRequestContext(context.Background(), &CommandRequest{
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
	service := newHandlerTestBindingService(t)
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

	_, err = executeMusic(NewRequestContext(ctx, &CommandRequest{
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
	service := newHandlerTestBindingService(t)
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

	message, err := executeMusic(NewRequestContext(ctx, &CommandRequest{
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
	service := newHandlerTestBindingService(t)
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&captured); err != nil {
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
		Snapshots: snapshot.NewStaticSnapshotProvider(&runtimeSnapshotStub{
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

	message, err := executeMusic(NewRequestContext(ctx, &CommandRequest{
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

func TestExecuteMusicListPrefersAPIProfileOverSuiteSnapshotProfile(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/jp/12345678901234/profile" {
			http.NotFound(w, r)
			return
		}
		_ = json.ConfigDefault.NewEncoder(w).Encode(sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 12345678901234,
				Name:   "API User",
			},
			UserDeck: sekaiapi.UserDeck{
				DeckID: 1,
				Leader: 1001,
			},
			UserCards: []sekaiapi.AnotherUserCard{
				{
					CardID:       1001,
					DefaultImage: "special_training",
				},
			},
		})
	}))
	defer server.Close()

	oldBaseURL := config.Cfg.SekaiAPI.BaseURL
	oldToken := config.Cfg.SekaiAPI.Token
	config.Cfg.SekaiAPI.BaseURL = server.URL
	config.Cfg.SekaiAPI.Token = "test-token"
	defer func() {
		config.Cfg.SekaiAPI.BaseURL = oldBaseURL
		config.Cfg.SekaiAPI.Token = oldToken
	}()

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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&captured); err != nil {
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
	profileController := renderprofile.NewController(runtimeProfileDataSourceStub{
		region: renderregion.JP,
		cards: map[int]*masterdata.Card{
			1001: {
				ID:              1001,
				AssetBundleName: "card_test",
			},
		},
	}, nil, nil, nil)

	app := &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Music:    music.NewController(source, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper(root, nil), nil, nil),
		Profiles: profileController,
		Bindings: service,
		SekaiAPI: sekaiapi.NewSekaiAPIClient(&config.Cfg.SekaiAPI),
		Snapshots: snapshot.NewStaticSnapshotProvider(&runtimeSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{
				Nickname:        "SnapshotUser",
				LeaderImagePath: "asset/user/snapshot.png",
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

	message, err := executeMusic(NewRequestContext(ctx, &CommandRequest{
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
	if captured.Profile == nil {
		t.Fatal("expected profile in music list request")
	}
	if got := captured.Profile.Nickname; got != "API User" {
		t.Fatalf("expected API profile nickname, got %q", got)
	}
	if got := captured.Profile.LeaderImagePath; got == "asset/user/snapshot.png" {
		t.Fatalf("expected API leader image path, got snapshot fallback %q", got)
	}
	if got := captured.UserResults[1]; got != "ap" {
		t.Fatalf("unexpected user result: %+v", got)
	}
}

func TestExecuteProfileBGAdjustReturnsPreviewImage(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	service.SetFastVerificationProvider(handlerTestFastVerifier{
		bindings: []sekaiapi.UserGameBinding{{
			Server:     "jp",
			GameUserID: "12345678901234",
		}},
	})
	if _, _, err := service.VerifyCurrentBinding(ctx, "qq", "42", "jp"); err != nil {
		t.Fatalf("verify: %v", err)
	}

	bgRoot := t.TempDir()
	service.SetProfileBGStorage(accountdata.NewLocalProfileBGStore(bgRoot))

	bgBytes := mustEncodeTestPNG(t, 6, 12)
	bgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(bgBytes)
	}))
	defer bgServer.Close()

	if _, err := service.SetCurrentBindingProfileBG(ctx, "qq", "42", "jp", bgServer.URL+"/bg.png"); err != nil {
		t.Fatalf("set current binding profile bg: %v", err)
	}

	var requestedPath string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		if r.URL.Path != "/api/jp/12345678901234/profile" {
			http.NotFound(w, r)
			return
		}
		_ = json.ConfigDefault.NewEncoder(w).Encode(sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 12345678901234,
				Name:   "API User",
			},
			UserProfile: sekaiapi.UserProfile{
				Word: "Hello",
			},
			UserDeck: sekaiapi.UserDeck{
				DeckID: 1,
				Leader: 1001,
			},
			UserCards: []sekaiapi.AnotherUserCard{{
				CardID:       1001,
				DefaultImage: "normal",
			}},
		})
	}))
	defer apiServer.Close()

	oldBaseURL := config.Cfg.SekaiAPI.BaseURL
	oldToken := config.Cfg.SekaiAPI.Token
	config.Cfg.SekaiAPI.BaseURL = apiServer.URL
	config.Cfg.SekaiAPI.Token = "test-token"
	defer func() {
		config.Cfg.SekaiAPI.BaseURL = oldBaseURL
		config.Cfg.SekaiAPI.Token = oldToken
	}()

	var captured drawing.ProfileRequest
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/profile" {
			http.NotFound(w, r)
			return
		}
		defer r.Body.Close()
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode profile request: %v", err)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	profileController := renderprofile.NewController(runtimeProfileDataSourceStub{
		region: renderregion.JP,
		cards: map[int]*masterdata.Card{
			1001: {
				ID:              1001,
				AssetBundleName: "card_test",
			},
		},
	}, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper("", nil), nil)

	blur := 7
	alpha := 66
	vertical := true
	params, err := json.Marshal(accountdata.ProfileSettingsCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Server:         "jp",
		RegionExplicit: true,
		Blur:           &blur,
		Alpha:          &alpha,
		Vertical:       &vertical,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeProfile(NewRequestContext(ctx, &CommandRequest{
		Module:            parser.ModuleProfile,
		Mode:              accountdata.ProfileModeBGAdjust,
		Region:            "jp",
		RegionExplicit:    true,
		Params:            params,
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Bindings:   service,
		Profiles:   profileController,
		SekaiAPI:   sekaiapi.NewSekaiAPIClient(&config.Cfg.SekaiAPI),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}))
	if err != nil {
		t.Fatalf("executeProfile bg-adjust: %v", err)
	}
	if requestedPath != "/api/jp/12345678901234/profile" {
		t.Fatalf("unexpected profile api path: %q", requestedPath)
	}
	if len(message) < 2 {
		t.Fatalf("expected image preview plus text summary, got %+v", message)
	}
	if message[0].Type != "image" {
		t.Fatalf("expected first segment to be image, got %+v", message)
	}
	if message[1].Type != "text" {
		t.Fatalf("expected second segment to be text, got %+v", message)
	}
	textData, ok := message[1].Data.(onebot11.TextData)
	if !ok {
		t.Fatalf("unexpected text segment data: %+v", message[1].Data)
	}
	if !strings.Contains(textData.Text, "已更新JP服个人信息背景设置") {
		t.Fatalf("unexpected text summary: %q", textData.Text)
	}
	if captured.BgSettings == nil {
		t.Fatalf("expected bg settings in profile request, got %+v", captured)
	}
	if captured.BgSettings.Blur != blur || captured.BgSettings.Alpha != alpha || captured.BgSettings.Vertical != vertical {
		t.Fatalf("unexpected bg settings: %+v", captured.BgSettings)
	}
	if captured.Profile.Nickname != "API User" {
		t.Fatalf("expected rendered profile to use API data, got %+v", captured.Profile)
	}
}

func TestExecuteMusicProgressRequiresSuiteData(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)

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

	_, err := executeMusic(NewRequestContext(ctx, &CommandRequest{
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
	service := newHandlerTestBindingService(t)

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

	_, err := executeMusic(NewRequestContext(ctx, &CommandRequest{
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

func TestExecuteMusicRewardsReturnsNoBindingError(t *testing.T) {
	service := newHandlerTestBindingService(t)
	app := &renderapp.App{
		Bindings: service,
		Music:    music.NewController(&bridgeMusicSource{}, nil, assets.NewAssetHelper("", nil), nil, nil),
	}

	_, err := executeMusic(NewRequestContext(context.Background(), &CommandRequest{
		Module:            parser.ModuleMusic,
		Mode:              "music-rewards",
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, app))
	if !errors.Is(err, accountdata.ErrNoBinding) {
		t.Fatalf("expected ErrNoBinding, got %v", err)
	}
}

func TestExecuteMusicRewardsRequiresSuiteSnapshot(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	app := &renderapp.App{
		Bindings: service,
		Music:    music.NewController(&bridgeMusicSource{}, nil, assets.NewAssetHelper("", nil), nil, nil),
	}

	_, err := executeMusic(NewRequestContext(ctx, &CommandRequest{
		Module:            parser.ModuleMusic,
		Mode:              "music-rewards",
		Region:            "jp",
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
		MusicCompareQueries: []string{"Song A hd", "Song B"},
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
	snap := snapshot.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), snapshot.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MusicMetaJSON: metaPath,
	})
	app := &renderapp.App{
		Music:      music.NewController(source, nil, assets.NewAssetHelper(root, nil), snap, nil),
		Score:      renderscore.NewController(drawing.NewHarukiDrawingClient(drawingServer.URL)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	params, err := json.Marshal(struct {
		Queries []string `json:"queries"`
	}{Queries: []string{"Song A"}})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeScore(NewRequestContext(context.Background(), &CommandRequest{
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
	snap := snapshot.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), snapshot.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MusicMetaJSON: metaPath,
	})
	app := &renderapp.App{
		Music:      music.NewController(source, nil, assets.NewAssetHelper(root, nil), snap, nil),
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

	message, err := executeScore(NewRequestContext(context.Background(), &CommandRequest{
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
	snap := snapshot.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), snapshot.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MusicMetaJSON: metaPath,
	})
	app := &renderapp.App{
		Music:      music.NewController(source, nil, assets.NewAssetHelper(root, nil), snap, nil),
		Score:      renderscore.NewController(drawing.NewHarukiDrawingClient(drawingServer.URL)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	params, err := json.Marshal(struct {
		TargetPoint int `json:"target_point"`
	}{TargetPoint: 22})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeScore(NewRequestContext(context.Background(), &CommandRequest{
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
	snap := snapshot.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), snapshot.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MusicMetaJSON: metaPath,
	})
	app := &renderapp.App{
		Music:      music.NewController(source, nil, assets.NewAssetHelper(root, nil), snap, nil),
		Score:      renderscore.NewController(drawing.NewHarukiDrawingClient(drawingServer.URL)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	params, err := json.Marshal(music.BoardQuery{
		SpecQueries: []string{"Song A"},
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeScore(NewRequestContext(context.Background(), &CommandRequest{
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
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
	snap := snapshot.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), snapshot.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MusicMetaJSON: metaPath,
	})
	app := &renderapp.App{
		Music:      music.NewController(source, nil, assets.NewAssetHelper(root, nil), snap, nil),
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

	message, err := executeScore(NewRequestContext(context.Background(), &CommandRequest{
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
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_arrest?mode=memory&cache=shared&_fk=1")
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
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_education_area_char?mode=memory&cache=shared&_fk=1")
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
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_char?mode=memory&cache=shared&_fk=1")
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
	now := time.Now().UnixMilli()
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 410, now-int64(72*time.Hour/time.Millisecond), now-int64(48*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(71*time.Hour/time.Millisecond), aggregateAt: now - int64(69*time.Hour/time.Millisecond), characterID: 21},
	})
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 420, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(2*time.Hour/time.Millisecond), aggregateAt: now + int64(time.Hour/time.Millisecond), characterID: 21},
	})

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
	if query.EventID == nil || *query.EventID != 420 {
		t.Fatalf("unexpected resolved event id: %+v", query.EventID)
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
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_challenge_fallback?mode=memory&cache=shared&_fk=1")
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
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_challenge_fallback_inline_diff?mode=memory&cache=shared&_fk=1")
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

func TestResolveDeckCharacterSelectionsRejectsUnknownWorldBloomQuery(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_world_bloom_fallback?mode=memory&cache=shared&_fk=1")
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

	err := resolveDeckCharacterSelections(context.Background(), &query, &renderapp.App{Sekai: sekaiClient})
	if err == nil {
		t.Fatalf("expected unknown WL character query to fail")
	}
	if !isCharacterNotFoundError(err) {
		t.Fatalf("unexpected error: %v", err)
	}
	if query.MusicQuery != "" {
		t.Fatalf("unexpected music query fallback: %q", query.MusicQuery)
	}
}

func TestResolveDeckCharacterSelectionsResolvesExplicitWorldBloomSelector(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_world_bloom_selector?mode=memory&cache=shared&_fk=1")
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
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 501, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
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

func TestPreserveImplicitMysekaiWorldBloomMetadataSeparatesQueryState(t *testing.T) {
	query := renderdeck.AutoQuery{
		Region:                   "jp",
		RecommendType:            "mysekai",
		EventID:                  drawing.IntPtr(420),
		EventUnit:                "piapro",
		EventAttr:                "cool",
		WorldBloomEventTurn:      drawing.IntPtr(2),
		WorldBloomCharacterID:    drawing.IntPtr(21),
		WorldBloomCharacterQuery: "初音未来",
	}

	preserveImplicitMysekaiWorldBloomMetadata(&query)

	if query.MetadataWorldBloomCharacterID == nil || *query.MetadataWorldBloomCharacterID != 21 {
		t.Fatalf("expected wl metadata to be preserved for drawing: %+v", query.MetadataWorldBloomCharacterID)
	}
	if query.EventID != nil {
		t.Fatalf("expected event id to be cleared from mysekai deck query: %+v", query.EventID)
	}
	if query.EventUnit != "" || query.EventAttr != "" {
		t.Fatalf("expected simulated event filters to be cleared: %+v", query)
	}
	if query.WorldBloomEventTurn != nil || query.WorldBloomCharacterID != nil || query.WorldBloomCharacterQuery != "" {
		t.Fatalf("expected wl query state to be cleared from mysekai deck query: %+v", query)
	}
}

func TestResolveDeckCharacterSelectionsResolvesDefaultWorldBloomChapterForExplicitEvent(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_world_bloom_default_explicit?mode=memory&cache=shared&_fk=1")
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
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 502, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
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

func TestResolveDeckCharacterSelectionsPrefersWorldBloomCharacterAliasOverMusicForExplicitEvent(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_world_bloom_alias_explicit?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	if _, err := sekaiClient.Gamecharacter.Create().
		SetServerRegion("jp").
		SetGameID(10).
		SetFirstName("白石").
		SetGivenName("杏").
		Save(ctx); err != nil {
		t.Fatalf("create gamecharacter: %v", err)
	}
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 505, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(2*time.Hour/time.Millisecond), aggregateAt: now + int64(time.Hour/time.Millisecond), characterID: 10},
	})

	query := renderdeck.AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       drawing.IntPtr(505),
		MusicQuery:    "an",
	}

	if err := resolveDeckCharacterSelections(ctx, &query, &renderapp.App{Sekai: sekaiClient}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.WorldBloomCharacterID == nil || *query.WorldBloomCharacterID != 10 {
		t.Fatalf("unexpected world bloom character id: %+v", query.WorldBloomCharacterID)
	}
	if query.MusicQuery != "" {
		t.Fatalf("expected music query to be consumed as WL character alias, got %q", query.MusicQuery)
	}
}

func TestResolveDeckCharacterSelectionsPrefersWorldBloomCharacterAliasOverMusicForCurrentEvent(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_world_bloom_alias_current?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	if _, err := sekaiClient.Gamecharacter.Create().
		SetServerRegion("jp").
		SetGameID(10).
		SetFirstName("白石").
		SetGivenName("杏").
		Save(ctx); err != nil {
		t.Fatalf("create gamecharacter: %v", err)
	}
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 506, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(2*time.Hour/time.Millisecond), aggregateAt: now + int64(time.Hour/time.Millisecond), characterID: 10},
	})

	query := renderdeck.AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		MusicQuery:    "an",
	}

	if err := resolveDeckCharacterSelections(ctx, &query, &renderapp.App{Sekai: sekaiClient}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.EventID == nil || *query.EventID != 506 {
		t.Fatalf("unexpected event id: %+v", query.EventID)
	}
	if query.WorldBloomCharacterID == nil || *query.WorldBloomCharacterID != 10 {
		t.Fatalf("unexpected world bloom character id: %+v", query.WorldBloomCharacterID)
	}
	if query.MusicQuery != "" {
		t.Fatalf("expected music query to be consumed as WL character alias, got %q", query.MusicQuery)
	}
}

func TestResolveDeckCharacterSelectionsPrefersWorldBloomCharacterFullNameOverMusicForExplicitEvent(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_world_bloom_fullname_explicit?mode=memory&cache=shared&_fk=1")
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
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 507, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(2*time.Hour/time.Millisecond), aggregateAt: now + int64(time.Hour/time.Millisecond), characterID: 21},
	})

	query := renderdeck.AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       drawing.IntPtr(507),
		MusicQuery:    "Hatsune Miku",
	}

	if err := resolveDeckCharacterSelections(ctx, &query, &renderapp.App{Sekai: sekaiClient}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.WorldBloomCharacterID == nil || *query.WorldBloomCharacterID != 21 {
		t.Fatalf("unexpected world bloom character id: %+v", query.WorldBloomCharacterID)
	}
	if query.MusicQuery != "" {
		t.Fatalf("expected music query to be consumed as WL character full name, got %q", query.MusicQuery)
	}
}

func TestResolveDeckCharacterSelectionsRejectsUnknownWorldBloomQueryForWorldBloomEvent(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_world_bloom_music_fallback?mode=memory&cache=shared&_fk=1")
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
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 504, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(3*time.Hour/time.Millisecond), aggregateAt: now + int64(time.Hour/time.Millisecond), characterID: 21},
		{chapterNo: 2, startAt: now + int64(time.Hour/time.Millisecond), aggregateAt: now + int64(2*time.Hour/time.Millisecond), characterID: 24},
	})

	query := renderdeck.AutoQuery{
		Region:                   "jp",
		RecommendType:            "event",
		EventID:                  drawing.IntPtr(504),
		WorldBloomCharacterQuery: "虾",
	}

	err := resolveDeckCharacterSelections(ctx, &query, &renderapp.App{Sekai: sekaiClient})
	if err == nil {
		t.Fatalf("expected unknown WL chapter character query to fail")
	}
	if !isCharacterNotFoundError(err) {
		t.Fatalf("unexpected error: %v", err)
	}
	if query.WorldBloomCharacterID != nil {
		t.Fatalf("unexpected world bloom character id: %+v", query.WorldBloomCharacterID)
	}
	if query.MusicQuery != "" {
		t.Fatalf("unexpected music query fallback: %q", query.MusicQuery)
	}
}

func TestResolveDeckCharacterSelectionsTreatsFinalChapterAsWorldBloomWithoutSelector(t *testing.T) {
	query := renderdeck.AutoQuery{
		Region:                   "jp",
		RecommendType:            "event",
		EventID:                  drawing.IntPtr(180),
		WorldBloomCharacterQuery: "wl2",
		WorldBloomCharacterID:    drawing.IntPtr(21),
	}

	if err := resolveDeckCharacterSelections(context.Background(), &query, &renderapp.App{}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.EventID == nil || *query.EventID != 180 {
		t.Fatalf("unexpected event id: %+v", query.EventID)
	}
	if query.WorldBloomCharacterID != nil {
		t.Fatalf("expected final chapter to clear explicit world bloom character: %+v", query.WorldBloomCharacterID)
	}
	if query.WorldBloomCharacterQuery != "" {
		t.Fatalf("expected final chapter to clear world bloom selector: %q", query.WorldBloomCharacterQuery)
	}
	if query.MusicQuery != "" {
		t.Fatalf("unexpected music query: %q", query.MusicQuery)
	}
}

func TestResolveDeckCharacterSelectionsResolvesCurrentWorldBloomEventWhenEventIDMissing(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_world_bloom_default_current?mode=memory&cache=shared&_fk=1")
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
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 503, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
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

func TestResolveDeckCharacterSelectionsResolvesWorldBloomEventTurnByCharacterOccurrence(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_world_bloom_turn?mode=memory&cache=shared&_fk=1")
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

	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 510, now-int64(96*time.Hour/time.Millisecond), now-int64(72*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(95*time.Hour/time.Millisecond), aggregateAt: now - int64(93*time.Hour/time.Millisecond), characterID: 24},
	})
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 520, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(2*time.Hour/time.Millisecond), aggregateAt: now + int64(time.Hour/time.Millisecond), characterID: 24},
	})

	query := renderdeck.AutoQuery{
		Region:                   "jp",
		RecommendType:            "event",
		WorldBloomEventTurn:      drawing.IntPtr(2),
		WorldBloomCharacterQuery: "巡音流歌",
	}

	if err := resolveDeckCharacterSelections(ctx, &query, &renderapp.App{Sekai: sekaiClient}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.EventID == nil || *query.EventID != 520 {
		t.Fatalf("unexpected resolved event id: %+v", query.EventID)
	}
	if query.WorldBloomEventTurn != nil {
		t.Fatalf("expected wl event turn to be consumed: %+v", query.WorldBloomEventTurn)
	}
	if query.WorldBloomCharacterID == nil || *query.WorldBloomCharacterID != 24 {
		t.Fatalf("unexpected world bloom character id: %+v", query.WorldBloomCharacterID)
	}
}

func TestResolveDeckCharacterSelectionsFallsBackEventRecommendToNoEventWhenNoEventAvailable(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_event_fallback_no_event?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	query := renderdeck.AutoQuery{
		Region:        "jp",
		RecommendType: "event",
	}

	if err := resolveDeckCharacterSelections(ctx, &query, &renderapp.App{Sekai: sekaiClient}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.RecommendType != "no_event" {
		t.Fatalf("expected recommend type fallback to no_event, got %q", query.RecommendType)
	}
	if query.EventID != nil {
		t.Fatalf("expected event id to stay empty after fallback: %+v", query.EventID)
	}
}

func TestResolveDeckCharacterSelectionsUsesRequestedRegionInsteadOfDefaultProviderRegion(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_region_specific_provider?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "cn", 630, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(2*time.Hour/time.Millisecond), aggregateAt: now + int64(time.Hour/time.Millisecond), characterID: 21},
	})

	app := &renderapp.App{
		Sekai: sekaiClient,
		Provider: bridgeDeckTestMasterProvider{
			region: renderregion.JP,
			events: &bridgeDeckTestEventProvider{
				events: []*masterdata.Event{{
					ID:          701,
					EventType:   "marathon",
					Name:        "JP Default Event",
					StartAt:     now - int64(48*time.Hour/time.Millisecond),
					AggregateAt: now - int64(24*time.Hour/time.Millisecond),
				}},
			},
		},
	}

	query := renderdeck.AutoQuery{
		Region:        "cn",
		RecommendType: "event",
	}

	if err := resolveDeckCharacterSelections(ctx, &query, app); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.RecommendType != "event" {
		t.Fatalf("expected requested region to keep event recommend, got %q", query.RecommendType)
	}
	if query.EventID == nil || *query.EventID != 630 {
		t.Fatalf("unexpected resolved event id: %+v", query.EventID)
	}
	if query.WorldBloomCharacterID == nil || *query.WorldBloomCharacterID != 21 {
		t.Fatalf("unexpected world bloom character id: %+v", query.WorldBloomCharacterID)
	}
}

func TestResolveDeckCharacterSelectionsClearsWorldBloomTurnAfterResolvingCharacterRoundCNEvent(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_cn_world_bloom_turn_resolve?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	if _, err := sekaiClient.Gamecharacter.Create().
		SetServerRegion("cn").
		SetGameID(20).
		SetFirstName("晓山").
		SetGivenName("瑞希").
		SetFirstNameEnglish("Akiyama").
		SetGivenNameEnglish("Mizuki").
		Save(ctx); err != nil {
		t.Fatalf("create cn gamecharacter: %v", err)
	}

	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "cn", 112, now-int64(120*time.Hour/time.Millisecond), now-int64(96*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(119*time.Hour/time.Millisecond), aggregateAt: now - int64(113*time.Hour/time.Millisecond), characterID: 17},
		{chapterNo: 2, startAt: now - int64(113*time.Hour/time.Millisecond), aggregateAt: now - int64(107*time.Hour/time.Millisecond), characterID: 20},
		{chapterNo: 3, startAt: now - int64(107*time.Hour/time.Millisecond), aggregateAt: now - int64(101*time.Hour/time.Millisecond), characterID: 19},
		{chapterNo: 4, startAt: now - int64(101*time.Hour/time.Millisecond), aggregateAt: now - int64(96*time.Hour/time.Millisecond), characterID: 18},
	})
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "cn", 130, now-int64(72*time.Hour/time.Millisecond), now-int64(48*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(71*time.Hour/time.Millisecond), aggregateAt: now - int64(65*time.Hour/time.Millisecond), characterID: 7},
		{chapterNo: 2, startAt: now - int64(65*time.Hour/time.Millisecond), aggregateAt: now - int64(59*time.Hour/time.Millisecond), characterID: 6},
		{chapterNo: 3, startAt: now - int64(59*time.Hour/time.Millisecond), aggregateAt: now - int64(53*time.Hour/time.Millisecond), characterID: 8},
		{chapterNo: 4, startAt: now - int64(53*time.Hour/time.Millisecond), aggregateAt: now - int64(48*time.Hour/time.Millisecond), characterID: 5},
	})
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "cn", 170, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(3*time.Hour/time.Millisecond), aggregateAt: now - int64(2*time.Hour/time.Millisecond), characterID: 17},
		{chapterNo: 2, startAt: now - int64(2*time.Hour/time.Millisecond), aggregateAt: now - int64(time.Hour/time.Millisecond), characterID: 19},
		{chapterNo: 3, startAt: now - int64(time.Hour/time.Millisecond), aggregateAt: now + int64(time.Hour/time.Millisecond), characterID: 20},
		{chapterNo: 4, startAt: now + int64(time.Hour/time.Millisecond), aggregateAt: now + int64(2*time.Hour/time.Millisecond), characterID: 18},
	})

	query := renderdeck.AutoQuery{
		Region:                "cn",
		RecommendType:         "event",
		WorldBloomEventTurn:   drawing.IntPtr(2),
		WorldBloomCharacterID: drawing.IntPtr(20),
		EventUnit:             "school_refusal",
	}

	if err := resolveDeckCharacterSelections(ctx, &query, &renderapp.App{Sekai: sekaiClient}); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.EventID == nil || *query.EventID != 170 {
		t.Fatalf("unexpected resolved event id: %+v", query.EventID)
	}
	if query.WorldBloomEventTurn != nil {
		t.Fatalf("expected world bloom turn to be cleared after event resolution: %+v", query.WorldBloomEventTurn)
	}
	if query.WorldBloomCharacterID == nil || *query.WorldBloomCharacterID != 20 {
		t.Fatalf("unexpected world bloom character id: %+v", query.WorldBloomCharacterID)
	}
}

func TestPickCurrentOrNextDeckEventAllowsJPFutureLeakAfterCardRelease(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_future_leak_after_release?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	app := &renderapp.App{
		Sekai: sekaiClient,
		Provider: bridgeDeckTestMasterProvider{
			events: &bridgeDeckTestEventProvider{
				events: []*masterdata.Event{{
					ID:          701,
					EventType:   "marathon",
					Name:        "Future Leak",
					StartAt:     now + int64(time.Hour/time.Millisecond),
					AggregateAt: now + int64(3*time.Hour/time.Millisecond),
				}},
				cardsByEvent: map[int][]*masterdata.Card{
					701: {{
						ID:        9001,
						ReleaseAt: now - int64(time.Minute/time.Millisecond),
					}},
				},
			},
		},
	}

	eventInfo, err := pickCurrentOrNextDeckEvent(ctx, app, renderregion.JP)
	if err != nil {
		t.Fatalf("pickCurrentOrNextDeckEvent() error = %v", err)
	}
	if eventInfo == nil || int(eventInfo.GameID) != 701 {
		t.Fatalf("unexpected future leak event: %+v", eventInfo)
	}
}

func TestPickCurrentOrNextDeckEventRejectsJPFutureLeakBeforeCardRelease(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_future_leak_before_release?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()

	app := &renderapp.App{
		Sekai: sekaiClient,
		Provider: bridgeDeckTestMasterProvider{
			events: &bridgeDeckTestEventProvider{
				events: []*masterdata.Event{{
					ID:          702,
					EventType:   "marathon",
					Name:        "Future Leak",
					StartAt:     now + int64(time.Hour/time.Millisecond),
					AggregateAt: now + int64(3*time.Hour/time.Millisecond),
				}},
				cardsByEvent: map[int][]*masterdata.Card{
					702: {{
						ID:        9002,
						ReleaseAt: now + int64(time.Minute/time.Millisecond),
					}},
				},
			},
		},
	}

	eventInfo, err := pickCurrentOrNextDeckEvent(ctx, app, renderregion.JP)
	if err == nil {
		t.Fatalf("expected future leak without released cards to be rejected, got %+v", eventInfo)
	}
}

func TestQueryDeckEventByIDUsesRegionProviderForNonDefaultRegion(t *testing.T) {
	ctx := context.Background()

	jpProvider := bridgeDeckTestMasterProvider{
		region: renderregion.JP,
		events: &bridgeDeckTestEventProvider{
			events: []*masterdata.Event{{
				ID:          100,
				EventType:   "marathon",
				Name:        "JP Event",
				StartAt:     time.Now().Add(-time.Hour).UnixMilli(),
				AggregateAt: time.Now().Add(time.Hour).UnixMilli(),
			}},
		},
	}
	enProvider := bridgeDeckTestMasterProvider{
		region: renderregion.EN,
		events: &bridgeDeckTestEventProvider{
			events: []*masterdata.Event{{
				ID:          163,
				EventType:   "marathon",
				Name:        "EN Event 163",
				StartAt:     time.Now().Add(-time.Hour).UnixMilli(),
				AggregateAt: time.Now().Add(time.Hour).UnixMilli(),
			}},
		},
	}

	app := &renderapp.App{
		Provider: jpProvider,
		Providers: map[renderregion.Value]renderprovider.MasterDataProvider{
			renderregion.JP: jpProvider,
			renderregion.EN: enProvider,
		},
	}

	eventInfo, err := queryDeckEventByID(ctx, app, renderregion.EN, 163)
	if err != nil {
		t.Fatalf("queryDeckEventByID() error = %v", err)
	}
	if eventInfo == nil || int(eventInfo.GameID) != 163 {
		t.Fatalf("unexpected event info: %+v", eventInfo)
	}
}

func TestResolveDeckCharacterSelectionsRejectsWorldBloomSelectorOnNonWorldBloomEvent(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_non_wl_selector?mode=memory&cache=shared&_fk=1")
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
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_deck_non_wl_character?mode=memory&cache=shared&_fk=1")
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

type bridgeDeckTestMasterProvider struct {
	region renderregion.Value
	events renderprovider.EventProvider
}

func (p bridgeDeckTestMasterProvider) Region() renderregion.Value {
	if region := renderregion.Normalize(p.region.String()); !region.IsZero() {
		return region
	}
	return renderregion.JP
}
func (p bridgeDeckTestMasterProvider) Cards() renderprovider.CardProvider {
	return nil
}
func (p bridgeDeckTestMasterProvider) Characters() renderprovider.CharacterProvider {
	return nil
}
func (p bridgeDeckTestMasterProvider) Skills() renderprovider.SkillProvider {
	return nil
}
func (p bridgeDeckTestMasterProvider) Events() renderprovider.EventProvider { return p.events }
func (p bridgeDeckTestMasterProvider) Musics() renderprovider.MusicProvider {
	return nil
}
func (p bridgeDeckTestMasterProvider) Gachas() renderprovider.GachaProvider {
	return nil
}
func (p bridgeDeckTestMasterProvider) Honors() renderprovider.HonorProvider {
	return nil
}
func (p bridgeDeckTestMasterProvider) Stamps() renderprovider.StampProvider {
	return nil
}
func (p bridgeDeckTestMasterProvider) VLives() renderprovider.VLiveProvider {
	return nil
}
func (p bridgeDeckTestMasterProvider) Education() renderprovider.EducationProvider {
	return nil
}
func (p bridgeDeckTestMasterProvider) PlayerFrames() renderprovider.PlayerFrameProvider {
	return nil
}
func (p bridgeDeckTestMasterProvider) MySekai() renderprovider.MySekaiProvider {
	return nil
}

type bridgeDeckTestEventProvider struct {
	events            []*masterdata.Event
	cardsByEvent      map[int][]*masterdata.Card
	worldBloomByEvent map[int][]*masterdata.WorldBloom
}

func (p *bridgeDeckTestEventProvider) GetByID(_ context.Context, id int) (*masterdata.Event, error) {
	for _, item := range p.events {
		if item != nil && item.ID == id {
			return item, nil
		}
	}
	return nil, fmt.Errorf("event %d not found", id)
}

func (p *bridgeDeckTestEventProvider) GetByCardID(_ context.Context, cardID int) (*masterdata.Event, error) {
	return nil, fmt.Errorf("event not found for card %d", cardID)
}

func (p *bridgeDeckTestEventProvider) GetAll(_ context.Context) []*masterdata.Event {
	return p.events
}

func (p *bridgeDeckTestEventProvider) GetCards(_ context.Context, eventID int) ([]*masterdata.Card, error) {
	if cards, ok := p.cardsByEvent[eventID]; ok {
		return cards, nil
	}
	return nil, fmt.Errorf("cards for event %d not found", eventID)
}

func (p *bridgeDeckTestEventProvider) GetBannerCharacterID(_ context.Context, eventID int) (int, error) {
	return 0, fmt.Errorf("banner character not found for event %d", eventID)
}

func (p *bridgeDeckTestEventProvider) GetDeckBonuses(_ context.Context, eventID int) ([]*masterdata.EventDeckBonus, error) {
	return nil, nil
}

func (p *bridgeDeckTestEventProvider) GetBanEvents(_ context.Context, charID int) []*masterdata.Event {
	return nil
}

func (p *bridgeDeckTestEventProvider) GetWorldBloomChapters(_ context.Context, eventID int) []*masterdata.WorldBloom {
	return p.worldBloomByEvent[eventID]
}

type handlerTestWorldBloomChapter struct {
	chapterNo   int64
	startAt     int64
	aggregateAt int64
	characterID int64
}

func seedHandlerTestWorldBloomEvent(
	t *testing.T,
	ctx context.Context,
	sekaiClient *sekaidb.Client,
	region string,
	eventID int64,
	startAt int64,
	aggregateAt int64,
	chapters []handlerTestWorldBloomChapter,
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
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_tracker_character?mode=memory&cache=shared&_fk=1")
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
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 101, now-int64(2*time.Hour/time.Millisecond), now+int64(2*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
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
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_tracker_wl_current?mode=memory&cache=shared&_fk=1")
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
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 201, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
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
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_tracker_wl_selector?mode=memory&cache=shared&_fk=1")
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
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 301, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
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
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_tracker_wl_prev?mode=memory&cache=shared&_fk=1")
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
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "jp", 401, now-int64(72*time.Hour/time.Millisecond), now-int64(48*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
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
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_character_alias_sekai?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	pjskClient := pjskenttest.Open(t, "sqlite3", "file:handler_test_character_alias_pjsk?mode=memory&cache=shared&_fk=1")
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
		SetAliasType(pjskalias.PjskAliasTypeCharacter).
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

	snap := snapshot.NewLocalFileService(nil, nil, snapshot.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MySekaiJSON:   mysekaiPath,
	})
	app := &renderapp.App{
		MySekai:    rendermysekai.NewController(nil, snap, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true}),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
		SekaiAPI:   sekaiapi.NewSekaiAPIClient(&config.Cfg.SekaiAPI),
	}

	params, err := json.Marshal(rendermysekai.PhotoQuery{Seq: 1})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	message, err := executeMysekai(NewRequestContext(context.Background(), &CommandRequest{
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
	lives         []*rendervlive.Live
	characters    map[int]*masterdata.GameCharacterUnit
	resourceBoxes map[int]*renderprovider.ResourceBox
}

func (s *bridgeVLiveSource) DefaultRegion() renderregion.Value { return renderregion.JP }

func (s *bridgeVLiveSource) GetLives(region renderregion.Value) ([]*rendervlive.Live, error) {
	return s.lives, nil
}

func (s *bridgeVLiveSource) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	if s.characters == nil {
		return nil, nil
	}
	return s.characters[id], nil
}

func (s *bridgeVLiveSource) GetResourceBoxByPurpose(_ string, id int) *renderprovider.ResourceBox {
	if s.resourceBoxes == nil {
		return nil
	}
	return s.resourceBoxes[id]
}

func TestExecuteVLiveReturnsImage(t *testing.T) {
	now := time.Now()
	ms := func(tm time.Time) int64 { return tm.UnixMilli() }

	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/vlive/list" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(mustEncodeTestPNG(t, 32, 24))
	}))
	defer drawingServer.Close()

	app := &renderapp.App{
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
		VLive: rendervlive.NewControllerWithDrawing(&bridgeVLiveSource{
			lives: []*rendervlive.Live{
				{
					ID:              3001,
					Name:            "Test Virtual Live",
					AssetBundleName: "vlentrance_03001_re",
					StartAt:         ms(now.Add(time.Hour)),
					EndAt:           ms(now.Add(2 * time.Hour)),
					Schedules: []rendervlive.Schedule{
						{StartAt: ms(now.Add(time.Hour)), EndAt: ms(now.Add(2 * time.Hour))},
					},
					Rewards: []rendervlive.Reward{
						{VirtualLiveType: "normal", ResourceBoxID: 11},
					},
					Characters: []rendervlive.Character{
						{GameCharacterUnitID: 21, VirtualLivePerformanceType: "main_only"},
					},
				},
			},
			characters: map[int]*masterdata.GameCharacterUnit{
				21: {ID: 21, GameCharacterID: 21, Unit: "piapro"},
			},
			resourceBoxes: map[int]*renderprovider.ResourceBox{
				11: {ID: 11, Details: []renderprovider.ResourceBoxDetail{{ResourceType: "jewel", ResourceQuantity: 100}}},
			},
		}, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper("", nil), renderregion.JP),
	}

	params, err := json.Marshal(rendervlive.ListQuery{})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	message, err := executeVLive(NewRequestContext(context.Background(), &CommandRequest{
		Module: parser.ModuleVLive,
		Mode:   "vlive-list",
		Region: "jp",
		Params: params,
	}, app))
	if err != nil {
		t.Fatalf("executeVLive: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected vlive message: %+v", message)
	}
	imageData, ok := message[0].Data.(onebot11.ImageData)
	if !ok {
		t.Fatalf("unexpected image data type: %T", message[0].Data)
	}
	if imageData.File == "" {
		t.Fatalf("expected cached image url, got empty image data: %+v", imageData)
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
			return card, nil
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

func (s *bridgeCardSource) FilterCards(info *rendercard.PjskCardQueryInfo) ([]*masterdata.Card, error) {
	if s.allowEmptyFilter && info != nil && info.CharacterID == 0 && info.Rarity == "" && info.Attr == "" &&
		info.SkillType == "" && info.Unit == "" && info.MainUnit == "" && info.SupportUnit == "" &&
		info.SupplyType == "" && info.Year == 0 && info.EventID == 0 && info.BanCharID == 0 && info.BanSeq == 0 {
		result := make([]*masterdata.Card, 0, len(s.cards))
		for _, card := range s.cards {
			if card == nil {
				continue
			}
			result = append(result, card)
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
		filepath.Join("asset", "jp-assets", "startapp", "character", "member", "card_test", "card_after_training.png"),
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
	message, err := executeCard(NewRequestContext(context.Background(), &CommandRequest{
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&captured); err != nil {
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

	message, err := executeCard(NewRequestContext(context.Background(), &CommandRequest{
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

func TestExecuteCardBoxAllowsNoBindingFallbackWithQuery(t *testing.T) {
	root := t.TempDir()
	var captured drawing.CardBoxRequest
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/card/box" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	service := newHandlerTestBindingServiceWithValidator(t, handlerTestBindingValidator{})
	app := &renderapp.App{
		Bindings:   service,
		Cards:      rendercard.NewController(&bridgeCardSource{allowEmptyFilter: true, cards: map[int]*masterdata.Card{1001: {ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Test Card", AssetBundleName: "card_test", ReleaseAt: 1700000000000}}}, &bridgeCardEventSource{}, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper(root, nil)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	message, err := executeCard(NewRequestContext(context.Background(), &CommandRequest{
		Module:            parser.ModuleCard,
		Mode:              "card-box",
		Query:             "1001",
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, app))
	if err != nil {
		t.Fatalf("expected no-binding fallback, got %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected message: %+v", message)
	}
	if captured.Title == nil || *captured.Title != CardCatalogTitleNoBinding {
		t.Fatalf("expected no-binding title %q, got %+v", CardCatalogTitleNoBinding, captured.Title)
	}
}

func TestExecuteCardBoxWithoutQueryReturnsNoBindingError(t *testing.T) {
	root := t.TempDir()
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	service := newHandlerTestBindingServiceWithValidator(t, handlerTestBindingValidator{})
	app := &renderapp.App{
		Bindings:   service,
		Cards:      rendercard.NewController(&bridgeCardSource{allowEmptyFilter: true, cards: map[int]*masterdata.Card{1001: {ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Test Card", AssetBundleName: "card_test", ReleaseAt: 1700000000000}}}, &bridgeCardEventSource{}, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper(root, nil)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	_, err := executeCard(NewRequestContext(context.Background(), &CommandRequest{
		Module:            parser.ModuleCard,
		Mode:              "card-box",
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, app))
	if !errors.Is(err, accountdata.ErrNoBinding) {
		t.Fatalf("expected ErrNoBinding, got %v", err)
	}
}

func TestExecuteCardBoxAddsNoSuiteTitleToDrawing(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	var captured drawing.CardBoxRequest
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/card/box" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	service := newHandlerTestBindingServiceWithValidator(t, handlerTestBindingValidator{})
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	app := &renderapp.App{
		Bindings:   service,
		Cards:      rendercard.NewController(&bridgeCardSource{allowEmptyFilter: true, cards: map[int]*masterdata.Card{1001: {ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Test Card", AssetBundleName: "card_test", ReleaseAt: 1700000000000}}}, &bridgeCardEventSource{}, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper(root, nil)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	message, err := executeCard(NewRequestContext(context.Background(), &CommandRequest{
		Module:            parser.ModuleCard,
		Mode:              "card-box",
		Query:             "1001",
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, app))
	if err != nil {
		t.Fatalf("executeCard box: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected message: %+v", message)
	}
	if captured.Title == nil || *captured.Title != CardCatalogTitleNoSuite {
		t.Fatalf("expected no-suite title %q, got %+v", CardCatalogTitleNoSuite, captured.Title)
	}
}

func TestExecuteCardBoxWithoutQueryRequiresSuiteData(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	service := newHandlerTestBindingServiceWithValidator(t, handlerTestBindingValidator{})
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	app := &renderapp.App{
		Bindings:   service,
		Cards:      rendercard.NewController(&bridgeCardSource{allowEmptyFilter: true, cards: map[int]*masterdata.Card{1001: {ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Test Card", AssetBundleName: "card_test", ReleaseAt: 1700000000000}}}, &bridgeCardEventSource{}, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper(root, nil)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	_, err := executeCard(NewRequestContext(context.Background(), &CommandRequest{
		Module:            parser.ModuleCard,
		Mode:              "card-box",
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, app))
	if err == nil {
		t.Fatal("expected missing suite data to fail")
	}
	if err.Error() != ErrMsgCardCatalogRequiresSuite {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteCardListAllowsNoBindingFallback(t *testing.T) {
	root := t.TempDir()
	var captured drawing.CardListRequest
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/card/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte("png"))
	}))
	defer drawingServer.Close()

	service := newHandlerTestBindingServiceWithValidator(t, handlerTestBindingValidator{})
	app := &renderapp.App{
		Bindings:   service,
		Cards:      rendercard.NewController(&bridgeCardSource{allowEmptyFilter: true, cards: map[int]*masterdata.Card{1001: {ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Test Card", AssetBundleName: "card_test", ReleaseAt: 1700000000000}}}, &bridgeCardEventSource{}, drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper(root, nil)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}
	params, err := json.Marshal(rendercard.ListRequest{
		Query:  "1001",
		Region: "jp",
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeCard(NewRequestContext(context.Background(), &CommandRequest{
		Module:            parser.ModuleCard,
		Mode:              "card-list",
		Region:            "jp",
		Params:            params,
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, app))
	if err != nil {
		t.Fatalf("expected no-binding fallback, got %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected message: %+v", message)
	}
	if captured.Title != nil {
		t.Fatalf("expected card-list to omit fallback title, got %+v", captured.Title)
	}
}

func TestExecuteCardBoxRequiresOwnedCardDataWhenShowBoxEnabled(t *testing.T) {
	root := t.TempDir()
	service := newHandlerTestBindingServiceWithValidator(t, handlerTestBindingValidator{})
	app := &renderapp.App{
		Bindings:   service,
		Cards:      rendercard.NewController(&bridgeCardSource{}, &bridgeCardEventSource{}, drawing.NewHarukiDrawingClient("https://drawing.invalid"), assets.NewAssetHelper(root, nil)),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}
	if _, err := service.Bind(context.Background(), "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	params, err := json.Marshal(map[string]any{
		"show_box": true,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	_, err = executeCard(NewRequestContext(context.Background(), &CommandRequest{
		Module:            parser.ModuleCard,
		Mode:              "card-box",
		Query:             "1001",
		Region:            "jp",
		Params:            params,
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, app))
	if err == nil {
		t.Fatal("expected missing owned-card data to fail")
	}
	if err.Error() != ErrMsgCardCatalogRequiresSuite {
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
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&captured); err != nil {
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

	message, err := executeCard(NewRequestContext(context.Background(), &CommandRequest{
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
