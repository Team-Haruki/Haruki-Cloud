package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/config"
	pjskenttest "haruki-cloud/database/pjsk/enttest"
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

func newBridgeTestBindingService(t *testing.T) *accountdata.BindingService {
	t.Helper()
	pjskClient := pjskenttest.Open(t, "sqlite3", "file:bridge_test_bind?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = pjskClient.Close() })
	usersClient := usersenttest.Open(t, "sqlite3", "file:bridge_test_users?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = usersClient.Close() })
	return accountdata.NewBindingService(
		pjskClient,
		identity.NewResolver(usersClient),
		bridgeTestBindingValidator{},
	)
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

	_, err = executeCheckData(ctx, &parser.ResolvedCommand{
		Module: parser.ModuleCheckData,
		Mode:   "mysekai",
		Region: "jp",
		Params: params,
	}, &renderapp.App{Bindings: service})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "当前账号没有可用的 MySekai 抓包数据" {
		t.Fatalf("unexpected error: %v", err)
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

func TestExecuteMusicCoverAndNoteCount(t *testing.T) {
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
				{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 27, TotalNoteCount: 777},
			},
		},
	}
	app := &renderapp.App{
		Music:      music.NewController(source, nil, assets.NewAssetHelper(root, nil), nil, nil),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	message, err := executeMusic(&parser.ResolvedCommand{
		Module: parser.ModuleMusic,
		Mode:   "music-cover",
		Query:  "Song A",
		Region: "jp",
	}, app)
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
	message, err = executeMusic(&parser.ResolvedCommand{
		Module: parser.ModuleMusic,
		Mode:   "music-note-count",
		Region: "jp",
		Params: params,
	}, app)
	if err != nil {
		t.Fatalf("executeMusic note-count: %v", err)
	}
	if len(message) != 1 || message[0].Type != "text" {
		t.Fatalf("unexpected note-count message: %+v", message)
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

	message, err := executeMusic(&parser.ResolvedCommand{
		Module: parser.ModuleMusic,
		Mode:   "music-list",
		Query:  "blue song",
		Region: "jp",
		Params: params,
	}, app)
	if err != nil {
		t.Fatalf("executeMusic list: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected list message: %+v", message)
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

	message, err := executeScore(&parser.ResolvedCommand{
		Module: parser.ModuleScore,
		Mode:   "score-music-meta",
		Region: "jp",
		Params: params,
	}, app)
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

	message, err := executeScore(&parser.ResolvedCommand{
		Module: parser.ModuleScore,
		Mode:   "score-control",
		Region: "jp",
		Params: params,
	}, app)
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

	message, err := executeScore(&parser.ResolvedCommand{
		Module: parser.ModuleScore,
		Mode:   "score-custom-room",
		Region: "jp",
		Params: params,
	}, app)
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

	message, err := executeScore(&parser.ResolvedCommand{
		Module: parser.ModuleScore,
		Mode:   "score-music-board",
		Region: "jp",
		Params: params,
	}, app)
	if err != nil {
		t.Fatalf("executeScore music-board: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("unexpected music-board message: %+v", message)
	}
}

func TestBuildBondsRequestFromSuiteIncludesFallbackIconsAndProgress(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_bonds?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })

	if _, err := sekaiClient.Bond.Create().
		SetServerRegion("jp").
		SetGroupID(2127).
		SetCharacterId1(21).
		SetCharacterId2(27).
		Save(ctx); err != nil {
		t.Fatalf("create bond master: %v", err)
	}
	for _, item := range []struct {
		Level    int64
		TotalExp int64
	}{
		{Level: 1, TotalExp: 0},
		{Level: 2, TotalExp: 10},
		{Level: 3, TotalExp: 30},
	} {
		if _, err := sekaiClient.Level.Create().
			SetServerRegion("jp").
			SetLevelType("bonds").
			SetLevel(item.Level).
			SetTotalExp(item.TotalExp).
			Save(ctx); err != nil {
			t.Fatalf("create level master: %v", err)
		}
	}
	for _, item := range []struct {
		CharacterID int64
		ColorCode   string
	}{
		{CharacterID: 21, ColorCode: "#112233"},
		{CharacterID: 27, ColorCode: "#445566"},
	} {
		if _, err := sekaiClient.Gamecharacterunit.Create().
			SetServerRegion("jp").
			SetGameCharacterID(item.CharacterID).
			SetColorCode(item.ColorCode).
			Save(ctx); err != nil {
			t.Fatalf("create gamecharacterunit: %v", err)
		}
	}

	toolboxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/private/game-data/jp/suite/123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch r.URL.Query().Get("key") {
		case "userBonds":
			_, _ = w.Write([]byte(`[{"bondsGroupId":2127,"rank":2,"exp":7}]`))
		case "userCharacters":
			_, _ = w.Write([]byte(`[{"characterId":21,"characterRank":55},{"characterId":27,"characterRank":33}]`))
		default:
			t.Fatalf("unexpected key: %s", r.URL.Query().Get("key"))
		}
	}))
	defer toolboxServer.Close()

	oldToolbox := config.Cfg.Toolbox
	config.Cfg.Toolbox.BaseURL = toolboxServer.URL
	config.Cfg.Toolbox.APIToken = "test-token"
	config.Cfg.Toolbox.UserAgent = "bridge-test"
	t.Cleanup(func() {
		config.Cfg.Toolbox = oldToolbox
	})

	req, err := buildBondsRequestFromSuite(
		&renderapp.App{
			Sekai:  sekaiClient,
			Assets: assets.NewAssetHelper(t.TempDir(), nil),
		},
		"jp",
		123,
		"qq",
		"42",
		nil,
	)
	if err != nil {
		t.Fatalf("buildBondsRequestFromSuite: %v", err)
	}
	if req.MaxLevel != 3 {
		t.Fatalf("unexpected max level: %d", req.MaxLevel)
	}
	if len(req.Bonds) != 1 {
		t.Fatalf("unexpected bonds: %+v", req.Bonds)
	}

	bond := req.Bonds[0]
	if bond.CharaID1 != 21 || bond.CharaID2 != 27 {
		t.Fatalf("unexpected bond pair: %+v", bond)
	}
	if bond.CharaIconPath1 != "static_images/chara_icon/miku.png" {
		t.Fatalf("unexpected icon path1: %q", bond.CharaIconPath1)
	}
	if bond.CharaIconPath2 != "static_images/chara_icon/chr_icon_27.png" {
		t.Fatalf("unexpected icon path2: %q", bond.CharaIconPath2)
	}
	if bond.CharaRank1 != 55 || bond.CharaRank2 != 33 {
		t.Fatalf("unexpected character ranks: %+v", bond)
	}
	if bond.BondLevel != 2 || !bond.HasBond {
		t.Fatalf("unexpected bond core fields: %+v", bond)
	}
	if bond.NeedExp == nil || *bond.NeedExp != 13 {
		t.Fatalf("unexpected need exp: %+v", bond.NeedExp)
	}
	if len(bond.Color1) != 3 || bond.Color1[0] != 17 || bond.Color1[1] != 34 || bond.Color1[2] != 51 {
		t.Fatalf("unexpected color1: %+v", bond.Color1)
	}
	if len(bond.Color2) != 3 || bond.Color2[0] != 68 || bond.Color2[1] != 85 || bond.Color2[2] != 102 {
		t.Fatalf("unexpected color2: %+v", bond.Color2)
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
	if !strings.Contains(text, "挑战Live(初音ミク): 123,456分") {
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

	if err := resolveDeckCharacterSelections(&query, &renderapp.App{Sekai: sekaiClient}); err != nil {
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

	if err := resolveDeckCharacterSelections(&query, &renderapp.App{Sekai: sekaiClient}); err != nil {
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

	if err := resolveDeckCharacterSelections(&query, &renderapp.App{Sekai: sekaiClient}); err != nil {
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

func TestResolveTrackerCharacterSelectionUsesMasterdataQuery(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:bridge_test_tracker_character?mode=memory&cache=shared&_fk=1")
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
		MySekai:    rendermysekai.NewController(nil, snapshot, "", renderregion.JP, nil),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}

	params, err := json.Marshal(rendermysekai.PhotoQuery{Seq: 1})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	message, err := executeMysekai(&parser.ResolvedCommand{
		Module: parser.ModuleMysekai,
		Mode:   "mysekai-photo",
		Region: "jp",
		Params: params,
	}, app)
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
	message, err := executeVLive(&parser.ResolvedCommand{
		Module: parser.ModuleVLive,
		Mode:   "vlive-list",
		Region: "jp",
		Params: params,
	}, app)
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

type bridgeCardSource struct{}

func (s *bridgeCardSource) DefaultRegion() renderregion.Value { return renderregion.JP }

func (s *bridgeCardSource) GetCardByID(id int) (*masterdata.Card, error) {
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
		filepath.Join("asset", "jp-assets", "startapp", "character", "member", "card_test_rip", "card_after_training.png"),
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
	message, err := executeCard(&parser.ResolvedCommand{
		Module: parser.ModuleCard,
		Mode:   "card-image",
		Query:  "1001",
		Region: "jp",
	}, app)
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

	message, err := executeCard(&parser.ResolvedCommand{
		Module: parser.ModuleCard,
		Mode:   "card-box",
		Query:  "1001",
		Region: "jp",
		Params: params,
	}, app)
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

	_, err = executeCard(&parser.ResolvedCommand{
		Module: parser.ModuleCard,
		Mode:   "card-box",
		Query:  "1001",
		Region: "jp",
		Params: params,
	}, app)
	if err == nil {
		t.Fatal("expected missing owned-card data to fail")
	}
	if !strings.Contains(err.Error(), "box 模式需要用户卡牌持有数据") {
		t.Fatalf("unexpected error: %v", err)
	}
}
