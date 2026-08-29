package requestbuilder

import (
	"context"
	json "haruki-cloud/internal/jsonutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
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

	req, err := BuildScoreControlRequest(context.Background(), &CommandInput{
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

func TestResolveScoreControlSelectionAndRangeBranches(t *testing.T) {
	params, err := json.Marshal(scoreControlSelection{TargetPoint: 120, Query: " song ", WL: true})
	if err != nil {
		t.Fatalf("marshal selection: %v", err)
	}
	selection, err := resolveScoreControlSelection(&CommandInput{Params: params, Query: "ignored"})
	if err != nil || selection.TargetPoint != 120 || selection.Query != " song " || !selection.WL {
		t.Fatalf("parameter selection = %#v, %v", selection, err)
	}
	selection, err = resolveScoreControlSelection(&CommandInput{Query: " 200   blue song "})
	if err != nil || selection.TargetPoint != 200 || selection.Query != "blue song" {
		t.Fatalf("query selection = %#v, %v", selection, err)
	}
	for name, input := range map[string]*CommandInput{
		"nil":          nil,
		"empty":        {},
		"negative":     {Query: "-1 song"},
		"invalid":      {Query: "many song"},
		"invalid json": {Params: []byte(`{`)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := resolveScoreControlSelection(input); err == nil {
				t.Fatal("invalid selection unexpectedly succeeded")
			}
		})
	}

	if got := selectScoreControlBasicPoint(nil); got != 0 {
		t.Fatalf("empty basic point = %d", got)
	}
	metas := []drawing.MusicMetaInfo{
		{Difficulty: "hard", EventRate: 80},
		{Difficulty: "expert", EventRate: 90},
		{Difficulty: "master", EventRate: 100},
		{Difficulty: "expert", EventRate: 200},
	}
	if got := selectScoreControlBasicPoint(metas); got != 100 {
		t.Fatalf("selected basic point = %d", got)
	}
	if got := selectScoreControlBasicPoint(metas[:2]); got != 90 {
		t.Fatalf("selected non-master basic point = %d", got)
	}

	limited := findValidScoreRanges(100, 100, false, 1)
	if len(limited) != 1 {
		t.Fatalf("limited score ranges = %#v", limited)
	}
	if got := findValidScoreRanges(1, 100, true, 0); len(got) != 0 {
		t.Fatalf("impossible WL score ranges = %#v", got)
	}
}

func TestBuildScoreControlRequestValidationBranches(t *testing.T) {
	if _, err := BuildScoreControlRequest(context.Background(), &CommandInput{Query: "100"}, nil); err == nil {
		t.Fatal("nil application unexpectedly succeeded")
	}
	app := &renderapp.App{Music: music.NewController(nil, nil, nil, nil, nil)}
	if _, err := BuildScoreControlRequest(context.Background(), &CommandInput{Params: []byte(`{`)}, app); err == nil {
		t.Fatal("malformed selection unexpectedly succeeded")
	}
	if _, err := BuildScoreControlRequest(context.Background(), &CommandInput{Query: "100"}, app); err == nil || !strings.Contains(err.Error(), "data source") {
		t.Fatalf("unconfigured source error = %v", err)
	}
}
