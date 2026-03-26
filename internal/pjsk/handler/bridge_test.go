package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pjskenttest "haruki-cloud/database/pjsk/enttest"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/music"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	accountdata "haruki-cloud/internal/pjsk/userdata"
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

func (s *bridgeMusicSource) DefaultRegion() renderregion.Value { return renderregion.JP }

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
