package music

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/snapshot"
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
			cp := *item
			return &cp, nil
		}
	}
	return nil, errNotFound("music")
}

func (s *detailMetaTestSource) GetMusicByID(id int) (*masterdata.Music, error) {
	if item := s.musics[id]; item != nil {
		cp := *item
		return &cp, nil
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
		cp := *item
		result = append(result, &cp)
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
		cp := *item
		result = append(result, &cp)
	}
	return result, nil
}

func TestBuildMusicDetailRequestIncludesMetadataFields(t *testing.T) {
	root := t.TempDir()
	userPath := filepath.Join(root, "user.json")
	metaPath := filepath.Join(root, "music_meta.json")
	chartPath := filepath.Join(root, "music", "music_score", "0001_01", "expert.txt")

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
	if err := os.MkdirAll(filepath.Dir(chartPath), 0o755); err != nil {
		t.Fatalf("mkdir chart: %v", err)
	}
	if err := os.WriteFile(chartPath, []byte(strings.Join([]string{
		"#BPM01:120",
		"#BPM02:180",
		"#00008:0100",
		"#00108:0200",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write chart: %v", err)
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
	snapshot := snapshot.NewLocalFileService(nil, assetHelper, snapshot.LocalFileConfig{
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
	if req.Bpm == nil || *req.Bpm != 120 {
		t.Fatalf("expected bpm from local chart, got %#v", req.Bpm)
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

func TestBuildMusicDetailRequestMergesApprovedAliases(t *testing.T) {
	source := &detailMetaTestSource{
		musics: map[int]*masterdata.Music{
			1: {
				ID:              1,
				Title:           "Song A",
				AssetBundleName: "jacket_a",
				Pronunciation:   "song a",
				PublishedAt:     1700000000000,
			},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31, TotalNoteCount: 999}},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	controller.SetAliasResolver(&lookupTestAliasResolver{
		ids:      map[string]int{"blue song": 1},
		approved: map[int][]string{1: {"Blue Song", "群青", "song a"}},
	})

	req, err := controller.BuildMusicDetailRequest(Query{Query: "blue song", Region: "jp"})
	if err != nil {
		t.Fatalf("BuildMusicDetailRequest() error = %v", err)
	}

	want := []string{"Blue Song", "群青", "song a"}
	if len(req.Alias) != len(want) {
		t.Fatalf("unexpected alias count: got=%v want=%v", req.Alias, want)
	}
	for i := range want {
		if req.Alias[i] != want[i] {
			t.Fatalf("unexpected aliases: got=%v want=%v", req.Alias, want)
		}
	}
}

func TestBuildMusicDetailRequestLeavesAliasEmptyWithoutApprovedAliases(t *testing.T) {
	source := &detailMetaTestSource{
		musics: map[int]*masterdata.Music{
			1: {
				ID:              1,
				Title:           "Song A",
				AssetBundleName: "jacket_a",
				Pronunciation:   "song a",
				PublishedAt:     1700000000000,
			},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31, TotalNoteCount: 999}},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	req, err := controller.BuildMusicDetailRequest(Query{Query: "Song A", Region: "jp"})
	if err != nil {
		t.Fatalf("BuildMusicDetailRequest() error = %v", err)
	}
	if req.Alias == nil {
		t.Fatalf("expected alias field to be an empty slice, got nil")
	}
	if len(req.Alias) != 0 {
		t.Fatalf("expected empty alias list without approved aliases, got=%v", req.Alias)
	}
	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if !strings.Contains(string(payload), `"alias":[]`) {
		t.Fatalf("expected serialized request to include empty alias array, got=%s", string(payload))
	}
}
