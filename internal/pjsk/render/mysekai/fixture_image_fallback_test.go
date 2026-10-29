package mysekai

import (
	"os"
	"path/filepath"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
)

// TW's master lists Ena's birthday set (mdl_bir1119_*, birthday party 16)
// before TW ships its thumbnails; only JP has them. The fixture list used to
// send the TW path regardless, and Drawing logged a miss for every tile.
func TestBuildFixtureListRequestFallsBackForMissingRegionThumbnails(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	assetDir := filepath.Join(root, "asset")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	writeThumbnail := func(region, name string) {
		t.Helper()
		dir := filepath.Join(assetDir, region+"-assets", "ondemand", "mysekai", "thumbnail", "fixture")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir thumbnails: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+"_1.png"), []byte("png"), 0o644); err != nil {
			t.Fatalf("write thumbnail: %v", err)
		}
	}
	writeThumbnail("tw", "mdl_bir1118_fixture_cake1")
	writeThumbnail("jp", "mdl_bir1118_fixture_cake1")
	writeThumbnail("jp", "mdl_bir1119_fixture_cake1")

	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiFixtures.json"), []map[string]any{
		{"id": 914, "name": "Birthday Cake (Minori)", "assetbundleName": "mdl_bir1118_fixture_cake1", "mysekaiFixtureType": "normal", "mysekaiFixtureMainGenreId": 1, "mysekaiFixtureSubGenreId": 11},
		{"id": 919, "name": "Birthday Cake (Ena)", "assetbundleName": "mdl_bir1119_fixture_cake1", "mysekaiFixtureType": "normal", "mysekaiFixtureMainGenreId": 1, "mysekaiFixtureSubGenreId": 11},
		{"id": 990, "name": "Unshipped", "assetbundleName": "mdl_nowhere_fixture_chair1", "mysekaiFixtureType": "normal", "mysekaiFixtureMainGenreId": 1, "mysekaiFixtureSubGenreId": 11},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiFixtureMainGenres.json"), []map[string]any{{"id": 1, "name": "Main A", "assetbundleName": "main_a"}})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiFixtureSubGenres.json"), []map[string]any{{"id": 11, "name": "Sub A", "assetbundleName": "sub_a"}})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiBlueprints.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacters.json"), []map[string]any{})

	controller := NewController(nil, nil, renderregion.TW, assets.NewAssetHelper(assetDir, nil), MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true})
	off := false
	req, err := controller.BuildFixtureListRequest(FixtureListQuery{Region: "tw", ShowProfile: &off, ShowProgress: &off, ShowObtained: &off})
	if err != nil {
		t.Fatalf("BuildFixtureListRequest() error = %v", err)
	}
	images := map[int]string{}
	for _, genre := range req.MainGenres {
		for _, sub := range genre.SubGenres {
			for _, fixture := range sub.Fixtures {
				images[fixture.ID] = fixture.ImagePath
			}
		}
	}
	want := map[int]string{
		// Shipped in TW: the region's own thumbnail, never JP's.
		914: "asset/tw-assets/ondemand/mysekai/thumbnail/fixture/mdl_bir1118_fixture_cake1_1.png",
		// Missing in TW: JP's render of the same bundle.
		919: "asset/jp-assets/ondemand/mysekai/thumbnail/fixture/mdl_bir1119_fixture_cake1_1.png",
		// Missing everywhere: the static placeholder.
		990: "static_images/unknown.jpg",
	}
	for id, path := range want {
		if images[id] != path {
			t.Fatalf("fixture %d image = %q, want %q (all %v)", id, images[id], path, images)
		}
	}
}

func TestFixtureImageResolverKeepsTheRegionPathWhenNothingAnswers(t *testing.T) {
	// No asset root and no store: existence is unknown, so the region path
	// goes out as before and Drawing's own placeholder covers a miss.
	controller := NewController(nil, nil, renderregion.TW, assets.NewAssetHelper("", nil), MasterdataOptions{})
	resolve := controller.fixtureImageResolver(renderregion.TW)
	if got, want := resolve("mysekai/thumbnail/fixture/mdl_bir1119_fixture_cake1_1.png"), "asset/tw-assets/ondemand/mysekai/thumbnail/fixture/mdl_bir1119_fixture_cake1_1.png"; got != want {
		t.Fatalf("unknown presence = %q, want %q", got, want)
	}
	// JP never falls back to itself.
	root := t.TempDir()
	jp := NewController(nil, nil, renderregion.JP, assets.NewAssetHelper(root, nil), MasterdataOptions{})
	if got := jp.fixtureImageResolver(renderregion.JP)("mysekai/thumbnail/fixture/none_1.png"); got != "static_images/unknown.jpg" {
		t.Fatalf("jp miss = %q", got)
	}
	images := fixtureColorImages(controller.fixtureImageResolver(renderregion.TW), map[string]any{"assetbundleName": "chair"})
	if len(images) != 1 || images[0].ImagePath != "asset/tw-assets/ondemand/mysekai/thumbnail/fixture/chair_1.png" {
		t.Fatalf("color images = %+v", images)
	}
}
