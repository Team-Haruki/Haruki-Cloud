package music

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/meta"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type musicMetaSnapshotStub struct {
	*musicSnapshotStub
	meta []byte
}

func (s *musicMetaSnapshotStub) MusicMetaBytes() []byte {
	return append([]byte(nil), s.meta...)
}

func TestResolveCustomRoomMusicListBranches(t *testing.T) {
	if _, err := (*Controller)(nil).ResolveCustomRoomMusicList("jp", []int{100}, 0); err == nil {
		t.Fatal("nil custom-room controller succeeded")
	}
	controller := newRenderCoverageController(t, nil)
	if got, err := controller.ResolveCustomRoomMusicList("jp", nil, 0); err != nil || len(got) != 0 {
		t.Fatalf("empty event rates = %v, %v", got, err)
	}
	if _, err := controller.ResolveCustomRoomMusicList("jp", []int{100}, 0); err == nil {
		t.Fatal("missing music metadata succeeded")
	}

	now := time.Now().UnixMilli()
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1:   {ID: 1, Title: "One", AssetBundleName: "one", PublishedAt: now - 1},
			2:   {ID: 2, Title: "Two", AssetBundleName: "two", PublishedAt: now - 1},
			3:   {ID: 3, Title: "Future", AssetBundleName: "future", PublishedAt: now + 60_000},
			241: {ID: 241, Title: "Hidden", AssetBundleName: "hidden", PublishedAt: now - 1},
		},
	}
	metaPayload := []byte(`[
		{"music_id":1,"difficulty":"expert","event_rate":100},
		{"music_id":1,"difficulty":"master","event_rate":99.6},
		{"music_id":1,"difficulty":"master","event_rate":100},
		{"music_id":2,"difficulty":"master","event_rate":100},
		{"music_id":2,"difficulty":"master","event_rate":110},
		{"music_id":2,"difficulty":"master","event_rate":110},
		{"music_id":3,"difficulty":"master","event_rate":100},
		{"music_id":241,"difficulty":"master","event_rate":100},
		{"music_id":999,"difficulty":"master","event_rate":100}
	]`)
	snap := &musicMetaSnapshotStub{musicSnapshotStub: &musicSnapshotStub{}, meta: metaPayload}
	controller = NewController(source, nil, assets.NewAssetHelper("", nil), snap, nil)
	if got, err := controller.ResolveCustomRoomMusicList("jp", []int{0, -1}, 0); err != nil || len(got) != 0 {
		t.Fatalf("invalid event rates = %v, %v", got, err)
	}
	got, err := controller.ResolveCustomRoomMusicList("jp", []int{100, 110}, 1)
	if err != nil {
		t.Fatalf("ResolveCustomRoomMusicList() error = %v", err)
	}
	if len(got[100]) != 1 || got[100][0]["music_id"] != 1 || len(got[110]) != 1 || got[110][0]["music_id"] != 2 {
		t.Fatalf("custom-room result = %#v", got)
	}
	unlimited, err := controller.ResolveCustomRoomMusicList("jp", []int{100}, 0)
	if err != nil || len(unlimited[100]) != 2 {
		t.Fatalf("unlimited custom-room result = %#v, %v", unlimited, err)
	}
}

func TestBuilderLocalizedTitleAndVocalCaptionBranches(t *testing.T) {
	titleCases := []struct {
		region string
		titles []string
		want   string
	}{
		{"cn", []string{"Base", " ", "中文标题", "かな"}, "中文标题"},
		{"tw", []string{"かな", "English"}, ""},
		{"kr", []string{"English", "한국어"}, "한국어"},
		{"en", []string{"中文", "English Title"}, "English Title"},
		{"jp", []string{"Alternative"}, "Alternative"},
		{"jp", []string{"Base", " "}, ""},
	}
	for _, tc := range titleCases {
		if got := selectLocalizedTitle("Base", tc.region, tc.titles); got != tc.want {
			t.Fatalf("localized title %s/%v = %q, want %q", tc.region, tc.titles, got, tc.want)
		}
	}

	if buildJPVocalOrderKey(nil) != "90_vocal" || buildJPVocalOrderKey(&masterdata.MusicVocal{}) != "90_vocal" {
		t.Fatal("nil/blank vocal order key mismatch")
	}
	for bundle, prefix := range map[string]string{"vs_test": "10_", "se_test": "20_", "an_test": "30_", "other": "90_"} {
		if got := buildJPVocalOrderKey(&masterdata.MusicVocal{AssetBundleName: bundle}); !strings.HasPrefix(got, prefix) {
			t.Fatalf("vocal order %q = %q", bundle, got)
		}
	}

	captionCases := []struct {
		raw, vocalType, bundle string
		region                 renderregion.Value
		want                   string
	}{
		{"SEKAI version", "", "", renderregion.CN, "「世界」"},
		{"Virtual Singer ver", "", "", renderregion.KR, "버추얼 싱어"},
		{"Unknown Caption", "", "", renderregion.EN, "Unknown Caption"},
		{"", "", "se_bundle", renderregion.TW, "「世界」"},
		{"", "", "vs_bundle", renderregion.CN, "虚拟歌手"},
		{"", "", "an_bundle", renderregion.EN, "Another Vocal"},
		{"", "original_song", "other", renderregion.EN, "original_song"},
		{"", "virtual singer", "other", renderregion.TW, "虚擬歌手"},
		{"", "unknown", "other", renderregion.EN, "unknown"},
	}
	for _, tc := range captionCases {
		if got := normalizeVocalCaption(tc.raw, tc.vocalType, tc.bundle, tc.region); got != tc.want {
			t.Fatalf("vocal caption %+v = %q, want %q", tc, got, tc.want)
		}
	}
	if localizeVocalCaption("", renderregion.EN) != "" || localizeVocalCaption("Custom", renderregion.EN) != "Custom" {
		t.Fatal("vocal localization fallback mismatch")
	}
	if got := []string{
		classifyVocalByAssetBundle("se_x", renderregion.EN),
		classifyVocalByAssetBundle("vs_x", renderregion.EN),
		classifyVocalByAssetBundle("an_x", renderregion.EN),
		classifyVocalByAssetBundle("other", renderregion.EN),
	}; !reflect.DeepEqual(got, []string{"Sekai", "Virtual Singer", "Another Vocal", ""}) {
		t.Fatalf("classified vocal bundles = %v", got)
	}
}

func TestControllerMusicChartMetaAndKeywordHelpers(t *testing.T) {
	view, err := meta.Parse([]byte(`[{"music_id":1,"difficulty":"master","event_rate":100}]`))
	if err != nil {
		t.Fatal(err)
	}
	if findMusicMetaInView(view, 1, "master")["event_rate"] == nil {
		t.Fatal("meta fixture is invalid")
	}
	controller := newRenderCoverageController(t, nil)
	if controller.resolveMusicChartMeta(renderregion.JP, 0, "master") != nil || controller.resolveMusicChartMeta(renderregion.JP, 1, " ") != nil {
		t.Fatal("invalid chart-meta lookup returned data")
	}

	source := newQueryMatchSource(&masterdata.Music{ID: 1, Title: "Alpha", Pronunciation: "ARUFA"})
	source.tags[1] = []string{"Vocaloid"}
	source.localized[1] = []string{"阿尔法"}
	item := source.musics[1]
	if matchesMusicKeyword(source, nil, "alpha") || !matchesMusicKeyword(source, item, "alpha") || !matchesMusicKeyword(source, item, "aru") || !matchesMusicKeyword(source, item, "vocal") || !matchesMusicKeyword(source, item, "阿尔") || matchesMusicKeyword(source, item, "missing") {
		t.Fatal("music keyword matching branches failed")
	}
	source.tagErr[1] = errNotFound("tags")
	source.localizedErr[1] = errNotFound("titles")
	if matchesMusicKeyword(source, item, "vocal") {
		t.Fatal("failed keyword sources unexpectedly matched")
	}
}
