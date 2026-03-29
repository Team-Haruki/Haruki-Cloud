package music

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	renderuserdata "haruki-cloud/internal/pjsk/render/userdata"
)

func TestResolveMusicMetaRequestsBuildsRequestsFromQuery(t *testing.T) {
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
		{"music_id": 1, "difficulty": "master", "music_time": 120, "tap_count": 600, "event_rate": 100, "base_score": 1.2, "base_score_auto": 1.1, "skill_score_solo": [0.1,0.2], "skill_score_auto": [0.3,0.4], "skill_score_multi": [0.5,0.6], "fever_score": 0.7},
		{"music_id": 1, "difficulty": "expert", "music_time": 118, "tap_count": 550, "event_rate": 90, "base_score": 1.0, "base_score_auto": 0.9, "skill_score_solo": [0.11,0.22], "skill_score_auto": [0.33,0.44], "skill_score_multi": [0.55,0.66], "fever_score": 0.77}
	]`), 0o644); err != nil {
		t.Fatalf("write music meta snapshot: %v", err)
	}

	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_test"},
		},
	}
	snapshot := renderuserdata.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), renderuserdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSON,
		MusicMetaJSON: metaJSON,
	})

	controller := NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil)
	reqs, err := controller.ResolveMusicMetaRequests("jp", []string{"Song A"})
	if err != nil {
		t.Fatalf("ResolveMusicMetaRequests() error = %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if reqs[0].MusicID != 1 || reqs[0].MusicTitle != "Song A" {
		t.Fatalf("unexpected request header: %+v", reqs[0])
	}
	if len(reqs[0].Metas) != 2 {
		t.Fatalf("expected 2 meta entries, got %d", len(reqs[0].Metas))
	}
	if reqs[0].Metas[0].Difficulty != "expert" || reqs[0].Metas[1].Difficulty != "master" {
		t.Fatalf("unexpected meta order: %+v", reqs[0].Metas)
	}
	if reqs[0].Metas[0].TapCount != 550 || reqs[0].Metas[1].TapCount != 600 {
		t.Fatalf("unexpected metas: %+v", reqs[0].Metas)
	}
}

func TestResolveMusicMetaRequestsBuildsRequestsFromAlias(t *testing.T) {
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

	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_test"},
		},
	}
	snapshot := renderuserdata.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), renderuserdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSON,
		MusicMetaJSON: metaJSON,
	})

	controller := NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil)
	controller.SetAliasResolver(&lookupTestAliasResolver{
		ids: map[string]int{"blue song": 1},
	})

	reqs, err := controller.ResolveMusicMetaRequests("jp", []string{"blue song"})
	if err != nil {
		t.Fatalf("ResolveMusicMetaRequests() error = %v", err)
	}
	if len(reqs) != 1 || reqs[0].MusicID != 1 {
		t.Fatalf("unexpected requests from alias: %+v", reqs)
	}
}

func TestResolveMusicMetaRequestsBuildsRequestsFromExplicitMusicID(t *testing.T) {
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

	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_test"},
		},
	}
	snapshot := renderuserdata.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), renderuserdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSON,
		MusicMetaJSON: metaJSON,
	})

	controller := NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil)
	reqs, err := controller.ResolveMusicMetaRequests("jp", []string{"music1"})
	if err != nil {
		t.Fatalf("ResolveMusicMetaRequests() error = %v", err)
	}
	if len(reqs) != 1 || reqs[0].MusicID != 1 {
		t.Fatalf("unexpected requests from explicit id: %+v", reqs)
	}
}

func TestResolveMusicMetaRequestsRejectsMissingExplicitMusicID(t *testing.T) {
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

	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_test"},
		},
	}
	snapshot := renderuserdata.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), renderuserdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSON,
		MusicMetaJSON: metaJSON,
	})

	controller := NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil)
	if _, err := controller.ResolveMusicMetaRequests("jp", []string{"music999"}); err == nil {
		t.Fatal("expected missing explicit music id to fail")
	}
}

func TestResolveMusicMetaRequestsRejectsAmbiguousKeywordQuery(t *testing.T) {
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
		{"music_id": 1, "difficulty": "master", "music_time": 120, "tap_count": 600, "event_rate": 100, "base_score": 1.2, "base_score_auto": 1.1, "skill_score_solo": [0.1,0.2], "skill_score_auto": [0.3,0.4], "skill_score_multi": [0.5,0.6], "fever_score": 0.7},
		{"music_id": 2, "difficulty": "master", "music_time": 118, "tap_count": 550, "event_rate": 90, "base_score": 1.0, "base_score_auto": 0.9, "skill_score_solo": [0.11,0.22], "skill_score_auto": [0.33,0.44], "skill_score_multi": [0.55,0.66], "fever_score": 0.77}
	]`), 0o644); err != nil {
		t.Fatalf("write music meta snapshot: %v", err)
	}

	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Alpha", Pronunciation: "shared"},
			2: {ID: 2, Title: "Beta", Pronunciation: "shared"},
		},
	}
	snapshot := renderuserdata.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), renderuserdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSON,
		MusicMetaJSON: metaJSON,
	})

	controller := NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil)
	_, err := controller.ResolveMusicMetaRequests("jp", []string{"shared"})
	if err == nil {
		t.Fatal("expected ambiguous keyword query to fail")
	}
	if !strings.Contains(err.Error(), "匹配到多个歌曲") {
		t.Fatalf("expected ambiguous error, got %v", err)
	}
	if !strings.Contains(err.Error(), "music1/Alpha") || !strings.Contains(err.Error(), "music2/Beta") {
		t.Fatalf("expected music id hints in error, got %v", err)
	}
}
