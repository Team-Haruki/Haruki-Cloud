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
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/music"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	rendervlive "haruki-cloud/internal/pjsk/render/vlive"
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
		MySekai:    rendermysekai.NewController(nil, snapshot, "", renderregion.JP),
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
		filepath.Join("character", "member", "card_test", "card_normal.png"),
		filepath.Join("character", "member", "card_test_rip", "card_after_training.png"),
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
