package music

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
)

func TestResolveMusicBoardRequestBuildsItemsFromMeta(t *testing.T) {
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
		{"music_id": 1, "difficulty": "master", "music_time": 120, "tap_count": 600, "event_rate": 100, "base_score": 1.20, "base_score_auto": 1.10, "skill_score_solo": [0.12,0.11,0.10,0.09,0.08,0.07], "skill_score_auto": [0.10,0.09,0.08,0.07,0.06,0.05], "skill_score_multi": [0.14,0.13,0.12,0.11,0.10,0.09], "fever_score": 0.70},
		{"music_id": 1, "difficulty": "expert", "music_time": 118, "tap_count": 550, "event_rate": 90, "base_score": 1.00, "base_score_auto": 0.95, "skill_score_solo": [0.10,0.09,0.08,0.07,0.06,0.05], "skill_score_auto": [0.09,0.08,0.07,0.06,0.05,0.04], "skill_score_multi": [0.12,0.11,0.10,0.09,0.08,0.07], "fever_score": 0.60},
		{"music_id": 2, "difficulty": "master", "music_time": 140, "tap_count": 500, "event_rate": 110, "base_score": 1.05, "base_score_auto": 0.98, "skill_score_solo": [0.08,0.07,0.06,0.05,0.04,0.03], "skill_score_auto": [0.07,0.06,0.05,0.04,0.03,0.02], "skill_score_multi": [0.10,0.09,0.08,0.07,0.06,0.05], "fever_score": 0.55}
	]`), 0o644); err != nil {
		t.Fatalf("write music meta snapshot: %v", err)
	}

	source := &lookupTestSource{
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
	snapshot := rendersnapshot.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), rendersnapshot.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSON,
		MusicMetaJSON: metaJSON,
	})

	controller := NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil)
	req, err := controller.ResolveMusicBoardRequest("jp", BoardQuery{
		SpecQueries: []string{"Song A"},
	})
	if err != nil {
		t.Fatalf("ResolveMusicBoardRequest() error = %v", err)
	}
	if req.LiveType != "solo" || req.Target != "score" {
		t.Fatalf("unexpected board mode: %+v", req)
	}
	if req.Page != 1 || req.TotalPage != 1 {
		t.Fatalf("unexpected paging: %+v", req)
	}
	if len(req.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(req.Items))
	}
	if len(req.SpecMidDiffs) != 2 {
		t.Fatalf("expected 2 highlighted diffs, got %d", len(req.SpecMidDiffs))
	}
	if req.Items[0].MusicID != 1 || req.Items[0].Difficulty != "master" {
		t.Fatalf("unexpected first item: %+v", req.Items[0])
	}
	if req.Items[0].LiveTypeScore == nil || *req.Items[0].LiveTypeScore <= 0 {
		t.Fatalf("expected score metrics: %+v", req.Items[0])
	}
	if req.Items[0].PlayCountPerHour == nil || *req.Items[0].PlayCountPerHour <= 0 {
		t.Fatalf("expected play-count metric: %+v", req.Items[0])
	}
	if req.TitleText == "" {
		t.Fatal("expected title text")
	}
}

func TestResolveMusicBoardRequestBuildsPtTimeMetrics(t *testing.T) {
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
		{"music_id": 1, "difficulty": "master", "music_time": 120, "tap_count": 600, "event_rate": 100, "base_score": 1.20, "base_score_auto": 1.10, "skill_score_solo": [0.12,0.11,0.10,0.09,0.08,0.07], "skill_score_auto": [0.10,0.09,0.08,0.07,0.06,0.05], "skill_score_multi": [0.14,0.13,0.12,0.11,0.10,0.09], "fever_score": 0.70}
	]`), 0o644); err != nil {
		t.Fatalf("write music meta snapshot: %v", err)
	}

	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31},
			},
		},
	}
	snapshot := rendersnapshot.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), rendersnapshot.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSON,
		MusicMetaJSON: metaJSON,
	})

	controller := NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil)
	req, err := controller.ResolveMusicBoardRequest("jp", BoardQuery{
		LiveType: "multi",
		Target:   "pt/time",
	})
	if err != nil {
		t.Fatalf("ResolveMusicBoardRequest() error = %v", err)
	}
	if len(req.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(req.Items))
	}
	item := req.Items[0]
	if item.LiveTypeRealScore == nil || *item.LiveTypeRealScore <= 0 {
		t.Fatalf("expected non-zero live real score: %+v", item)
	}
	if item.LiveTypePtPerHour == nil || *item.LiveTypePtPerHour <= 0 {
		t.Fatalf("expected non-zero pt/hour: %+v", item)
	}
	if item.LiveTypeSkillAccount == nil || *item.LiveTypeSkillAccount <= 0 {
		t.Fatalf("expected non-zero skill account: %+v", item)
	}
}

func TestResolveMusicBoardRequestBuildsItemsFromAlias(t *testing.T) {
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
		{"music_id": 1, "difficulty": "master", "music_time": 120, "tap_count": 600, "event_rate": 100, "base_score": 1.20, "base_score_auto": 1.10, "skill_score_solo": [0.12,0.11,0.10,0.09,0.08,0.07], "skill_score_auto": [0.10,0.09,0.08,0.07,0.06,0.05], "skill_score_multi": [0.14,0.13,0.12,0.11,0.10,0.09], "fever_score": 0.70}
	]`), 0o644); err != nil {
		t.Fatalf("write music meta snapshot: %v", err)
	}

	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31},
			},
		},
	}
	snapshot := rendersnapshot.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), rendersnapshot.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSON,
		MusicMetaJSON: metaJSON,
	})

	controller := NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil)
	controller.SetAliasResolver(&lookupTestAliasResolver{
		ids: map[string]int{"blue song": 1},
	})

	req, err := controller.ResolveMusicBoardRequest("jp", BoardQuery{
		SpecQueries: []string{"blue song"},
	})
	if err != nil {
		t.Fatalf("ResolveMusicBoardRequest() error = %v", err)
	}
	if len(req.SpecMidDiffs) != 1 {
		t.Fatalf("unexpected board alias resolution: %+v", req.SpecMidDiffs)
	}
	if len(req.SpecMidDiffs[0]) != 2 || req.SpecMidDiffs[0][0] != 1 || req.SpecMidDiffs[0][1] != "master" {
		t.Fatalf("unexpected board alias resolution: %+v", req.SpecMidDiffs)
	}
}

func TestResolveMusicBoardRequestExpandsWildcardSpecDiffs(t *testing.T) {
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
		{"music_id": 1, "difficulty": "master", "music_time": 120, "tap_count": 600, "event_rate": 100, "base_score": 1.20, "base_score_auto": 1.10, "skill_score_solo": [0.12,0.11,0.10,0.09,0.08,0.07], "skill_score_auto": [0.10,0.09,0.08,0.07,0.06,0.05], "skill_score_multi": [0.14,0.13,0.12,0.11,0.10,0.09], "fever_score": 0.70},
		{"music_id": 1, "difficulty": "expert", "music_time": 118, "tap_count": 550, "event_rate": 90, "base_score": 1.00, "base_score_auto": 0.95, "skill_score_solo": [0.10,0.09,0.08,0.07,0.06,0.05], "skill_score_auto": [0.09,0.08,0.07,0.06,0.05,0.04], "skill_score_multi": [0.12,0.11,0.10,0.09,0.08,0.07], "fever_score": 0.60}
	]`), 0o644); err != nil {
		t.Fatalf("write music meta snapshot: %v", err)
	}

	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31},
				{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 27},
			},
		},
	}
	snapshot := rendersnapshot.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), rendersnapshot.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSON,
		MusicMetaJSON: metaJSON,
	})

	controller := NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil)
	req, err := controller.ResolveMusicBoardRequest("jp", BoardQuery{
		SpecQueries: []string{"music1*"},
	})
	if err != nil {
		t.Fatalf("ResolveMusicBoardRequest() error = %v", err)
	}
	if len(req.SpecMidDiffs) != 2 {
		t.Fatalf("unexpected wildcard spec resolution: %+v", req.SpecMidDiffs)
	}
	if len(req.SpecMidDiffs[0]) != 2 || req.SpecMidDiffs[0][0] != 1 || req.SpecMidDiffs[0][1] != "master" {
		t.Fatalf("unexpected wildcard spec resolution: %+v", req.SpecMidDiffs)
	}
	if len(req.SpecMidDiffs[1]) != 2 || req.SpecMidDiffs[1][0] != 1 || req.SpecMidDiffs[1][1] != "expert" {
		t.Fatalf("unexpected wildcard spec resolution: %+v", req.SpecMidDiffs)
	}
}

func TestResolveMusicBoardRequestReturnsErrorForMissingExplicitDiff(t *testing.T) {
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
		{"music_id": 1, "difficulty": "master", "music_time": 120, "tap_count": 600, "event_rate": 100, "base_score": 1.20, "base_score_auto": 1.10, "skill_score_solo": [0.12,0.11,0.10,0.09,0.08,0.07], "skill_score_auto": [0.10,0.09,0.08,0.07,0.06,0.05], "skill_score_multi": [0.14,0.13,0.12,0.11,0.10,0.09], "fever_score": 0.70}
	]`), 0o644); err != nil {
		t.Fatalf("write music meta snapshot: %v", err)
	}

	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31},
			},
		},
	}
	snapshot := rendersnapshot.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), rendersnapshot.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSON,
		MusicMetaJSON: metaJSON,
	})

	controller := NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil)
	if _, err := controller.ResolveMusicBoardRequest("jp", BoardQuery{
		SpecQueries: []string{"music1append"},
	}); err == nil {
		t.Fatal("expected explicit missing diff to fail")
	}
}

func TestResolveMusicBoardRequestReturnsAmbiguousSpecError(t *testing.T) {
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
		{"music_id": 1, "difficulty": "master", "music_time": 120, "tap_count": 600, "event_rate": 100, "base_score": 1.20, "base_score_auto": 1.10, "skill_score_solo": [0.12,0.11,0.10,0.09,0.08,0.07], "skill_score_auto": [0.10,0.09,0.08,0.07,0.06,0.05], "skill_score_multi": [0.14,0.13,0.12,0.11,0.10,0.09], "fever_score": 0.70},
		{"music_id": 2, "difficulty": "master", "music_time": 118, "tap_count": 550, "event_rate": 90, "base_score": 1.00, "base_score_auto": 0.95, "skill_score_solo": [0.10,0.09,0.08,0.07,0.06,0.05], "skill_score_auto": [0.09,0.08,0.07,0.06,0.05,0.04], "skill_score_multi": [0.12,0.11,0.10,0.09,0.08,0.07], "fever_score": 0.60}
	]`), 0o644); err != nil {
		t.Fatalf("write music meta snapshot: %v", err)
	}

	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
			2: {ID: 2, Title: "Song Alpha", AssetBundleName: "jacket_b"},
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
	snapshot := rendersnapshot.NewLocalFileService(nil, assets.NewAssetHelper(root, nil), rendersnapshot.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSON,
		MusicMetaJSON: metaJSON,
	})

	controller := NewController(source, nil, assets.NewAssetHelper(root, nil), snapshot, nil)
	_, err := controller.ResolveMusicBoardRequest("jp", BoardQuery{
		SpecQueries: []string{"Song"},
	})
	if err == nil {
		t.Fatal("expected ambiguous spec query to fail")
	}
	if !strings.Contains(err.Error(), "匹配到多个歌曲") {
		t.Fatalf("expected ambiguous error, got %v", err)
	}
	if !strings.Contains(err.Error(), "music1/Song A") || !strings.Contains(err.Error(), "music2/Song Alpha") {
		t.Fatalf("expected music id hints in error, got %v", err)
	}
}
