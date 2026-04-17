package requestbuilder

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/music"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
)

type scoreControlTestSource struct {
	musics map[int]*masterdata.Music
}

type scoreControlTestAliasResolver struct {
	ids map[string]int
}

func (s *scoreControlTestSource) DefaultRegion() renderregion.Value { return renderregion.JP }

func (r *scoreControlTestAliasResolver) TryResolveMusicID(_ context.Context, token string) (int, bool, error) {
	if r == nil {
		return 0, false, nil
	}
	id, ok := r.ids[strings.ToLower(strings.TrimSpace(token))]
	return id, ok, nil
}

func (r *scoreControlTestAliasResolver) TryResolveMusicTitleOrAliasID(_ context.Context, token string) (int, bool, error) {
	if r == nil {
		return 0, false, nil
	}
	id, ok := r.ids[strings.ToLower(strings.TrimSpace(token))]
	return id, ok, nil
}

func (s *scoreControlTestSource) SearchMusic(query string) (*masterdata.Music, error) {
	for _, item := range s.musics {
		if strings.EqualFold(item.Title, strings.TrimSpace(query)) {
			return new(*item), nil
		}
	}
	return nil, os.ErrNotExist
}

func (s *scoreControlTestSource) GetMusicByID(id int) (*masterdata.Music, error) {
	item := s.musics[id]
	if item == nil {
		return nil, os.ErrNotExist
	}
	return new(*item), nil
}

func (s *scoreControlTestSource) GetMusicByEventID(int) (*masterdata.Music, error) {
	return nil, os.ErrNotExist
}

func (s *scoreControlTestSource) GetMusics() []*masterdata.Music {
	out := make([]*masterdata.Music, 0, len(s.musics))
	for _, item := range s.musics {
		out = append(out, new(*item))
	}
	return out
}

func (s *scoreControlTestSource) GetBanEvents(int) []*masterdata.Event { return nil }

func (s *scoreControlTestSource) GetMusicLocalizedTitles(int) ([]string, error) { return nil, nil }

func (s *scoreControlTestSource) GetMusicDifficulties(int) ([]*masterdata.MusicDifficulty, error) {
	return nil, nil
}

func (s *scoreControlTestSource) GetMusicVocals(int) ([]*masterdata.MusicVocal, error) {
	return nil, nil
}

func (s *scoreControlTestSource) GetMusicTags(int) ([]string, error) { return nil, nil }

func (s *scoreControlTestSource) GetCharacterByID(int) (*masterdata.Character, error) {
	return nil, os.ErrNotExist
}

func (s *scoreControlTestSource) GetOutsideCharacterByID(int) (string, error) {
	return "", os.ErrNotExist
}

func (s *scoreControlTestSource) GetPrimaryEventByMusicID(int) (*masterdata.Event, error) {
	return nil, os.ErrNotExist
}

func (s *scoreControlTestSource) GetLimitedTimeMusics(int) []*masterdata.LimitedTimeMusic { return nil }

func TestBuildScoreControlRequestPreservesControllerAliasResolver(t *testing.T) {
	root := t.TempDir()
	userJSON := filepath.Join(root, "user.json")
	metaJSON := filepath.Join(root, "music_meta.json")

	if err := os.WriteFile(userJSON, []byte(`{
		"now": 1700000000,
		"userGamedata": {"userId": 123, "name": "Tester", "deck": 1},
		"userProfile": {},
		"userDecks": [{"deckId": 1}],
		"userCards": []
	}`), 0o644); err != nil {
		t.Fatalf("write user snapshot: %v", err)
	}
	if err := os.WriteFile(metaJSON, []byte(`[
		{"music_id": 1, "difficulty": "master", "music_time": 120, "tap_count": 600, "event_rate": 100, "base_score": 1.2, "base_score_auto": 1.1, "skill_score_solo": [0.1,0.2], "skill_score_auto": [0.3,0.4], "skill_score_multi": [0.5,0.6], "fever_score": 0.7}
	]`), 0o644); err != nil {
		t.Fatalf("write music meta snapshot: %v", err)
	}

	source := &scoreControlTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_test"},
		},
	}
	snapshot := rendersnapshot.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), rendersnapshot.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSON,
		MusicMetaJSON: metaJSON,
	})

	controller := music.NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil)
	controller.SetAliasResolver(&scoreControlTestAliasResolver{
		ids: map[string]int{"blue song": 1},
	})

	params, err := json.Marshal(map[string]any{
		"target_point": 100,
		"query":        "blue song",
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	req, err := BuildScoreControlRequest(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleScore,
		Mode:   "score-control",
		Region: "jp",
		Params: params,
	}, &renderapp.App{
		Music: controller,
	})
	if err != nil {
		t.Fatalf("BuildScoreControlRequest() error = %v", err)
	}
	if req.MusicID != 1 || req.MusicTitle != "Song A" {
		t.Fatalf("unexpected score control request: %+v", req)
	}
	if len(req.ValidScores) == 0 {
		t.Fatalf("expected non-empty valid scores: %+v", req)
	}
}
