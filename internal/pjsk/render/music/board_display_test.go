package music

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
)

type boardDisplaySource struct {
	*lookupTestSource
	localizedIDs []int
}

func (s *boardDisplaySource) GetMusicLocalizedTitles(id int) ([]string, error) {
	s.localizedIDs = append(s.localizedIDs, id)
	return []string{fmt.Sprintf("譯名 %03d", id)}, nil
}

func newBoardDisplayFixture(t *testing.T) (*Controller, *boardDisplaySource, *commandtrace.Trace) {
	t.Helper()
	root := t.TempDir()
	source := &boardDisplaySource{lookupTestSource: &lookupTestSource{
		region: renderregion.TW, musics: make(map[int]*masterdata.Music), difficulties: make(map[int][]*masterdata.MusicDifficulty),
	}}
	var metas []map[string]any
	for id := 1; id <= 120; id++ {
		bundle := fmt.Sprintf("jacket_%03d", id)
		source.musics[id] = &masterdata.Music{ID: id, Title: fmt.Sprintf("Song %03d", id), AssetBundleName: bundle}
		cover := filepath.Join(root, "asset", "tw-assets", "ondemand", "music", "jacket", bundle, bundle+".png")
		if err := os.MkdirAll(filepath.Dir(cover), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cover, []byte("cover"), 0o644); err != nil {
			t.Fatal(err)
		}
		for index, diff := range []string{"expert", "master"} {
			source.difficulties[id] = append(source.difficulties[id], &masterdata.MusicDifficulty{
				MusicID: id, MusicDifficulty: diff, PlayLevel: 25 + index*5 + id%3,
			})
			metas = append(metas, map[string]any{
				"music_id": id, "difficulty": diff, "music_time": 1000 + id*2 + index, "tap_count": 5000,
				"event_rate": 100, "base_score": 1000 + id*10 + index, "base_score_auto": 900 + id*10 + index,
				"skill_score_solo": []float64{0.1}, "skill_score_auto": []float64{0.1}, "skill_score_multi": []float64{0.1},
			})
		}
	}
	metaJSON, err := json.Marshal(metas)
	if err != nil {
		t.Fatal(err)
	}
	helper := assets.NewAssetHelper(root, nil)
	snapshot, err := rendersnapshot.NewFromBytes(nil, helper, renderregion.TW, []byte(`{"userGamedata":{"userId":1,"name":"Test"}}`), nil, metaJSON)
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace := commandtrace.WithNewTrace(t.Context())
	return NewController(source, nil, helper, snapshot, nil).WithContext(ctx), source, trace
}

func TestMusicBoardLoadsDisplayDataForSelectedRows(t *testing.T) {
	cases := []struct {
		name     string
		query    BoardQuery
		wantHash string
	}{
		{name: "first page", query: BoardQuery{}, wantHash: "8480014808f918c43d95f5aaeafb9b6cb7e165fc1654be18cda6575e9dd46bc1"},
		{name: "second page", query: BoardQuery{Page: 2}, wantHash: "7968f1a129fa3a0d645f462a4ee3c9ed32a3d0c5855f2b486c1deb9389bfe16d"},
		{name: "filtered with pinned difficulties", query: BoardQuery{Page: 2, DiffFilter: []string{"master"}, LevelFilter: ">=31", SpecQueries: []string{"1*"}}, wantHash: "32eebba6e3035367510dac389abd8e4e975eaa119176b5e830b0cdd2dd189a78"},
		{name: "one difficulty per song", query: BoardQuery{Page: 2, Target: "time", SpecQueries: []string{"1*"}}, wantHash: "b4669c140d6c540cd2e014d8b22704a5ebdd496aadd4589ea8df5db2f29412ac"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			controller, source, trace := newBoardDisplayFixture(t)
			request, err := controller.ResolveMusicBoardRequest("tw", tc.query)
			if err != nil {
				t.Fatal(err)
			}
			shown := make(map[int]bool)
			for _, item := range request.Items {
				shown[item.MusicID] = true
				if want := fmt.Sprintf("Song %03d (譯名 %03d)", item.MusicID, item.MusicID); item.MusicTitle != want {
					t.Fatalf("music %d title = %q, want %q", item.MusicID, item.MusicTitle, want)
				}
				if want := fmt.Sprintf("asset/tw-assets/ondemand/music/jacket/jacket_%03d/jacket_%03d.png", item.MusicID, item.MusicID); item.MusicCoverPath != want {
					t.Fatalf("music %d cover = %q, want %q", item.MusicID, item.MusicCoverPath, want)
				}
			}

			if len(source.localizedIDs) != len(shown) {
				t.Fatalf("localized %d songs, want only %d displayed songs", len(source.localizedIDs), len(shown))
			}
			for _, id := range source.localizedIDs {
				if !shown[id] {
					t.Errorf("loaded translation for undisplayed music %d", id)
				}
			}
			payload, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			operations := map[string]int{}
			for _, operation := range trace.Snapshot().Operations {
				operations[operation.Name] = operation.Count
			}

			// These hashes preserve the complete Drawing requests from the eager
			// implementation; only the localized title and description changed since.
			if got := fmt.Sprintf("%x", sha256.Sum256(payload)); got != tc.wantHash {
				t.Fatalf("Drawing payload changed: SHA256=%s, want %s; payload=%s", got, tc.wantHash, payload)
			}
			if got := operations["asset.resolve_cache_miss"] + operations["asset.resolve_cache_hit"]; got != 2*len(shown) {
				t.Fatalf("asset path lookups=%d, want %d for displayed songs only", got, 2*len(shown))
			}
			t.Logf("payload SHA256=%x; items=%d songs=%d localized=%d asset-resolve-miss=%d asset-resolve-hit=%d", sha256.Sum256(payload), len(request.Items), len(shown), len(source.localizedIDs), operations["asset.resolve_cache_miss"], operations["asset.resolve_cache_hit"])
		})
	}
}

func TestInvalidMusicBoardSelectionSkipsDisplayData(t *testing.T) {
	for _, query := range []BoardQuery{
		{Page: 100},
		{LevelFilter: ">=99"},
		{SpecQueries: []string{"999999"}},
	} {
		controller, source, trace := newBoardDisplayFixture(t)
		if request, err := controller.ResolveMusicBoardRequest("tw", query); err == nil || request != nil {
			t.Fatalf("query=%+v: request=%+v, err=%v", query, request, err)
		}
		if len(query.SpecQueries) == 0 && len(source.localizedIDs) != 0 {
			t.Errorf("query=%+v: loaded %d titles for an invalid selection", query, len(source.localizedIDs))
		}
		for _, operation := range trace.Snapshot().Operations {
			if operation.Name == "asset.resolve_cache_miss" || operation.Name == "asset.resolve_cache_hit" {
				t.Errorf("query=%+v: probed assets for an invalid selection: %+v", query, operation)
			}
		}
	}
}
