package music

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
)

type detailMetaTestSource struct {
	musics       map[int]*masterdata.Music
	difficulties map[int][]*masterdata.MusicDifficulty
}

func (s *detailMetaTestSource) DefaultRegion() renderregion.Value { return renderregion.JP }

func (s *detailMetaTestSource) SearchMusic(query string) (*masterdata.Music, error) {
	query = strings.TrimSpace(strings.ToLower(query))
	for _, item := range s.musics {
		if item != nil && strings.ToLower(strings.TrimSpace(item.Title)) == query {
			copy := *item
			return &copy, nil
		}
	}
	return nil, errNotFound("music")
}

func (s *detailMetaTestSource) GetMusicByID(id int) (*masterdata.Music, error) {
	if item := s.musics[id]; item != nil {
		copy := *item
		return &copy, nil
	}
	return nil, errNotFound("music")
}

func (s *detailMetaTestSource) GetMusicByEventID(int) (*masterdata.Music, error) {
	return nil, errNotFound("music")
}

func (s *detailMetaTestSource) GetMusics() []*masterdata.Music {
	result := make([]*masterdata.Music, 0, len(s.musics))
	for _, item := range s.musics {
		if item == nil {
			continue
		}
		copy := *item
		result = append(result, &copy)
	}
	return result
}

func (s *detailMetaTestSource) GetBanEvents(int) []*masterdata.Event          { return nil }
func (s *detailMetaTestSource) GetMusicLocalizedTitles(int) ([]string, error) { return nil, nil }
func (s *detailMetaTestSource) GetMusicVocals(int) ([]*masterdata.MusicVocal, error) {
	return nil, nil
}
func (s *detailMetaTestSource) GetMusicTags(int) ([]string, error) { return nil, nil }
func (s *detailMetaTestSource) GetCharacterByID(int) (*masterdata.Character, error) {
	return nil, errNotFound("character")
}
func (s *detailMetaTestSource) GetOutsideCharacterByID(int) (string, error) {
	return "", errNotFound("outside character")
}
func (s *detailMetaTestSource) GetPrimaryEventByMusicID(int) (*masterdata.Event, error) {
	return nil, errNotFound("event")
}
func (s *detailMetaTestSource) GetLimitedTimeMusics(int) []*masterdata.LimitedTimeMusic { return nil }

func (s *detailMetaTestSource) GetMusicDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error) {
	items := s.difficulties[musicID]
	result := make([]*masterdata.MusicDifficulty, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		copy := *item
		result = append(result, &copy)
	}
	return result, nil
}

func TestBuildMusicDetailRequestIncludesMetadataFields(t *testing.T) {
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
  {"music_id": 2, "difficulty": "master", "music_time": 100, "tap_count": 500, "event_rate": 80, "base_score": 1.00, "base_score_auto": 0.90, "skill_score_solo": [0.08,0.07,0.06,0.05,0.04,0.03], "skill_score_auto": [0.07,0.06,0.05,0.04,0.03,0.02], "skill_score_multi": [0.09,0.08,0.07,0.06,0.05,0.04], "fever_score": 0.50}
]`), 0o644); err != nil {
		t.Fatalf("write music meta snapshot: %v", err)
	}

	source := &detailMetaTestSource{
		musics: map[int]*masterdata.Music{
			1: {
				ID:              1,
				Title:           "Song A",
				Composer:        "Composer A",
				Lyricist:        "Lyricist A",
				Arranger:        "Arranger A",
				Categories:      []string{"mv", "mv_2d"},
				AssetBundleName: "jacket_a",
				PublishedAt:     1700000000000,
			},
			2: {
				ID:              2,
				Title:           "Song B",
				Composer:        "Composer B",
				Lyricist:        "Lyricist B",
				Arranger:        "Arranger B",
				Categories:      []string{"mv"},
				AssetBundleName: "jacket_b",
				PublishedAt:     1700000000000,
			},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31, TotalNoteCount: 999}},
			2: {{MusicID: 2, MusicDifficulty: "master", PlayLevel: 29, TotalNoteCount: 777}},
		},
	}

	assetHelper := assets.NewAssetHelper(root, nil)
	snapshot := userdata.NewLocalFileService(nil, assetHelper, userdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MusicMetaJSON: metaPath,
	})
	controller := NewController(source, nil, assetHelper, snapshot, nil)

	req, err := controller.BuildMusicDetailRequest(Query{Query: "Song A", Region: "jp"})
	if err != nil {
		t.Fatalf("BuildMusicDetailRequest() error = %v", err)
	}

	if len(req.MusicInfo.MVInfo) != 2 || req.MusicInfo.MVInfo[0] != "mv" {
		t.Fatalf("expected mv_info copied from categories, got %#v", req.MusicInfo.MVInfo)
	}
	if req.Length == nil || *req.Length != "120.0秒（2分0.0秒）" {
		t.Fatalf("expected formatted length from music meta, got %#v", req.Length)
	}
	if req.LeaderboardMusicNum == nil || *req.LeaderboardMusicNum != 2 {
		t.Fatalf("expected leaderboard music count 2, got %#v", req.LeaderboardMusicNum)
	}
	if req.LeaderboardLiveTypes["solo"] != "单人" || req.LeaderboardTargets["pt/time"] != "时速" {
		t.Fatalf("unexpected leaderboard labels: %#v %#v", req.LeaderboardLiveTypes, req.LeaderboardTargets)
	}
	if len(req.LeaderboardMatrix) != 3 || len(req.LeaderboardMatrix[0]) != 3 {
		t.Fatalf("unexpected leaderboard matrix shape: %#v", req.LeaderboardMatrix)
	}
	if req.LeaderboardMatrix[0][0] == nil || req.LeaderboardMatrix[0][0].Rank != 1 || req.LeaderboardMatrix[0][0].Diff != "master" {
		t.Fatalf("unexpected solo score leaderboard cell: %#v", req.LeaderboardMatrix[0][0])
	}
	if !strings.HasSuffix(req.LeaderboardMatrix[0][0].Value, "%") {
		t.Fatalf("expected percentage leaderboard value, got %#v", req.LeaderboardMatrix[0][0])
	}
}
