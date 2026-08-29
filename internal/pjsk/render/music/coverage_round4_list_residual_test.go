package music

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type round4ProfileSnapshot struct {
	*musicSnapshotStub
	detail  *drawing.DetailedProfileCardRequest
	card    *drawing.ProfileCardRequest
	results map[int]string
}

func (s *round4ProfileSnapshot) DetailedProfile(renderregion.Value) *drawing.DetailedProfileCardRequest {
	return s.detail
}

func (s *round4ProfileSnapshot) ProfileCard(renderregion.Value) *drawing.ProfileCardRequest {
	return s.card
}

func (s *round4ProfileSnapshot) MusicResults(string) map[int]string {
	return s.results
}

func TestMusicCoverByTitleLocalAndMissingJacketBranches(t *testing.T) {
	now := time.Now().UnixMilli()
	source := newRound4SearchSource()
	source.musics[1] = &masterdata.Music{ID: 1, Title: "No Jacket", PublishedAt: now - 1}
	controller := NewController(source, nil, nil, nil, nil)
	if _, err := controller.ResolveMusicCoverByTitleOrAlias(Query{Query: "No Jacket", Region: "jp"}); err == nil || !strings.Contains(err.Error(), "jacket") {
		t.Fatalf("missing title cover jacket error = %v", err)
	}

	root := t.TempDir()
	jacket := filepath.Join(root, "music", "jacket", "local_jacket", "local_jacket.png")
	if err := os.MkdirAll(filepath.Dir(jacket), 0o755); err != nil {
		t.Fatalf("mkdir local jacket: %v", err)
	}
	if err := os.WriteFile(jacket, []byte("png"), 0o644); err != nil {
		t.Fatalf("write local jacket: %v", err)
	}
	source.musics[2] = &masterdata.Music{ID: 2, Title: "Local Jacket", AssetBundleName: "local_jacket", PublishedAt: now - 1}
	controller = NewController(source, nil, assets.NewAssetHelper(root, nil), nil, nil)
	result, err := controller.ResolveMusicCoverByTitleOrAlias(Query{Query: "Local Jacket", Region: "jp"})
	if err != nil || filepath.Clean(result.JacketPath) != filepath.Clean(jacket) {
		t.Fatalf("local title cover = %#v, %v", result, err)
	}
}

func TestMusicListFilteringAndItemResidualBranches(t *testing.T) {
	now := time.Now().UnixMilli()
	source := newRound4SearchSource()
	source.region = renderregion.CN
	source.musics = map[int]*masterdata.Music{
		1: {ID: 1, Seq: 5, Title: "One", Pronunciation: "ichi", AssetBundleName: "one", PublishedAt: now - 2},
		2: {ID: 2, Seq: 5, Title: "Future", AssetBundleName: "two", PublishedAt: now + 100_000},
		3: {ID: 3, Seq: 5, Title: "Three", AssetBundleName: "three", PublishedAt: now - 1},
	}
	source.allMusics = []*masterdata.Music{nil, source.musics[3], source.musics[2], source.musics[1]}
	source.difficulties = []*masterdata.MusicDifficulty{{MusicDifficulty: "master", PlayLevel: 30}}
	controller := NewController(source, nil, nil, nil, nil)

	full, err := controller.BuildMusicListRequest(ListQuery{
		Region: "cn", Difficulty: "master", Full: true, LevelMin: 31, LevelMax: 29,
	})
	if err != nil || len(full.MusicList) != 3 || full.Profile != nil {
		t.Fatalf("full CN music list = %#v, %v", full, err)
	}
	if gotID, _ := full.MusicList[0]["id"].(int); gotID != 1 {
		t.Fatalf("stable ID tie ordering = %#v", full.MusicList)
	}

	if got, err := controller.BuildMusicListRequest(ListQuery{Region: "cn", Difficulty: "master", Keyword: "music1"}); err != nil || len(got.MusicList) != 1 {
		t.Fatalf("explicit ID filtered list = %#v, %v", got, err)
	}
	if _, err := controller.BuildMusicListRequest(ListQuery{Region: "cn", Difficulty: "master", Keyword: "unmatched"}); err == nil {
		t.Fatal("unmatched keyword list error = nil")
	}
	if _, err := controller.BuildMusicListRequest(ListQuery{Region: "cn", Difficulty: "master", LevelMin: 31}); err == nil {
		t.Fatal("minimum-level filtered list error = nil")
	}
	if _, err := controller.BuildMusicListRequest(ListQuery{Region: "cn", Difficulty: "master", LevelMax: 29}); err == nil {
		t.Fatal("maximum-level filtered list error = nil")
	}
	if _, err := controller.BuildMusicListRequest(ListQuery{
		Region: "cn", Difficulty: "master", ResultFilter: "not_ap", UserResults: map[int]string{1: "ap", 2: "ap", 3: "ap"},
	}); err == nil {
		t.Fatal("result-filtered list error = nil")
	}

	items, jackets := controller.buildMusicListEntriesFromItems(source, NewBuilder(source, nil, nil), renderregion.CN, "master", []ListItemQuery{
		{MusicID: 0},
		{MusicID: 99},
		{MusicID: 2},
		{MusicID: 1, Difficulty: "append"},
		{MusicID: 1, Difficulty: "master"},
		{MusicID: 1, Difficulty: "master"},
	}, false)
	if len(items) != 1 || len(jackets) != 1 {
		t.Fatalf("filtered explicit items = %#v, %#v", items, jackets)
	}

	source.difficulties = nil
	if _, err := controller.BuildMusicListRequest(ListQuery{Region: "cn", Difficulty: "master"}); err == nil {
		t.Fatal("zero-level list error = nil")
	}

	if got, want := buildMusicListUserResults(map[int]string{1: " AP "}, map[int]string{1: "clear", 2: "fc"}), map[int]string{1: "ap", 2: "fc"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("merged user results = %#v, want %#v", got, want)
	}
}

func TestMusicListRenderImplicitLeakAndProfileSnapshotBranches(t *testing.T) {
	now := time.Now().UnixMilli()
	source := newRound4SearchSource()
	source.region = renderregion.CN
	source.musics[1] = &masterdata.Music{ID: 1, Title: "Future", AssetBundleName: "future", PublishedAt: now + 100_000}
	source.difficulties = []*masterdata.MusicDifficulty{{MusicDifficulty: "master", PlayLevel: 30}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/music/list" || r.URL.Query().Get("show_leak") != "true" {
			t.Errorf("music list request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte("rendered"))
	}))
	defer server.Close()
	controller := NewController(source, drawing.NewHarukiDrawingClient(server.URL), nil, nil, nil)
	body, err := controller.RenderMusicList(ListQuery{Region: "cn", Difficulty: "master", Full: true})
	if err != nil || string(body) != "rendered" {
		t.Fatalf("implicit-leak render = %q, %v", body, err)
	}

	detail := &drawing.DetailedProfileCardRequest{ID: "detail", Region: "CN"}
	card := &drawing.ProfileCardRequest{Profile: &drawing.BasicProfile{ID: "card"}}
	snapshot := &round4ProfileSnapshot{
		musicSnapshotStub: &musicSnapshotStub{},
		detail:            detail,
		card:              card,
		results:           map[int]string{1: "fc"},
	}
	controller = controller.WithSnapshot(snapshot)
	if got := controller.resolveMusicListProfile(nil, renderregion.CN); got == detail || got.ID != "detail" {
		t.Fatalf("snapshot detailed profile = %#v", got)
	}
	if got := controller.profileCard(renderregion.CN); got.Profile == nil || got.Profile.ID != "card" {
		t.Fatalf("snapshot profile card = %#v", got)
	}
	if got := controller.buildUserResults("master"); !reflect.DeepEqual(got, map[int]string{1: "fc"}) {
		t.Fatalf("snapshot user results = %#v", got)
	}
}
