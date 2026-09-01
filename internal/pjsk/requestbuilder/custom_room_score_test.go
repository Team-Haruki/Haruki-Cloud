package requestbuilder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/music"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/testutil"
)

func TestResolveCustomRoomScoreSelection(t *testing.T) {
	selection, err := resolveCustomRoomScoreSelection(&CommandInput{Params: []byte(`{"target_point":42}`), Query: "99"})
	{
		testutil.Require(t, !(err != nil), "selection from params = %#v, %v", selection, err)
		testutil.Require(t, !(selection.TargetPoint != 42), "selection from params = %#v, %v", selection, err)
	}

	selection, err = resolveCustomRoomScoreSelection(&CommandInput{Query: " 23 "})
	{
		testutil.Require(t, !(err != nil), "selection from query = %#v, %v", selection, err)
		testutil.Require(t, !(selection.TargetPoint != 23), "selection from query = %#v, %v", selection, err)
	}

	for name, input := range map[string]*CommandInput{
		"nil":          nil,
		"empty":        {},
		"negative":     {Query: "-1"},
		"invalid json": {Params: []byte(`{`)},
	} {
		t.Run(name, func(t *testing.T) {
			{
				_, err := resolveCustomRoomScoreSelection(input)
				testutil.RequireArgs(t, !(err == nil), "invalid selection unexpectedly succeeded")
			}

		})
	}
}

func TestCustomRoomScoreCSVHelpers(t *testing.T) {
	for raw, want := range map[string]int{
		" 25% ":      25,
		"0":          0,
		"\ufeff300%": 300,
	} {
		got, ok := parseCustomRoomBonus(raw)
		testutil.Check(t, !(!ok || got != want), "parseCustomRoomBonus(%q) = (%d, %t)", raw, got, ok)

	}
	for _, raw := range []string{"", "%", "invalid"} {
		{
			_, ok := parseCustomRoomBonus(raw)
			testutil.Check(t, !(ok), "parseCustomRoomBonus(%q) unexpectedly succeeded", raw)
		}

	}

	pairs, err := findCustomRoomCandidatePairs(22)
	{
		testutil.Require(t, !(err != nil), "find candidate pairs = %#v, %v", pairs, err)
		testutil.Require(t, !(len(pairs) == 0), "find candidate pairs = %#v, %v", pairs, err)
	}

	found := false
	for _, pair := range pairs {
		if len(pair) == 2 && pair[0] == 100 && pair[1] == 0 {
			found = true
			break
		}
	}
	testutil.Require(t, found, "candidate pairs do not contain the CSV's first cell: %#v", pairs)

	missing, err := findCustomRoomCandidatePairs(-999)
	{
		testutil.Require(t, !(err != nil), "missing candidate pairs = %#v, %v", missing, err)
		testutil.Require(t, !(len(missing) != 0), "missing candidate pairs = %#v, %v", missing, err)
	}

	rates := customRoomEventRates([][]int{{100, 0}, {100, 5}, nil, {-1, 10}, {103, 10}})
	{
		testutil.Require(t, !(len(rates) != 2), "custom room rates = %#v", rates)
		testutil.Require(t, !(rates[0] != 100), "custom room rates = %#v", rates)
		testutil.Require(t, !(rates[1] != 103), "custom room rates = %#v", rates)
	}

}

func TestBuildCustomRoomScoreRequestRequiresController(t *testing.T) {
	{
		_, err := BuildCustomRoomScoreRequest(&CommandInput{Query: "22"}, nil)
		{
			testutil.Require(t, !(err == nil), "nil app error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "music controller"), "nil app error = %v", err)
		}
	}

}

func TestBuildCustomRoomScoreRequestValidationAndMusicErrors(t *testing.T) {
	app := &renderapp.App{Music: music.NewController(nil, nil, nil, nil, nil)}
	for name, input := range map[string]*CommandInput{
		"invalid selection": {Query: "invalid"},
		"nonpositive":       {Params: []byte(`{"target_point":-1}`)},
		"large impossible":  {Query: "999999"},
		"small impossible":  {Query: "1"},
	} {
		t.Run(name, func(t *testing.T) {
			{
				_, err := BuildCustomRoomScoreRequest(input, app)
				testutil.RequireArgs(t, !(err == nil), "invalid custom-room request unexpectedly succeeded")
			}

		})
	}
	{
		_, err := BuildCustomRoomScoreRequest(&CommandInput{Query: "22"}, app)
		{
			testutil.Require(t, !(err == nil), "missing music source error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "data source"), "missing music source error = %v", err)
		}
	}

}

func TestBuildCustomRoomScoreRequestSuccess(t *testing.T) {
	root := t.TempDir()
	userJSON := filepath.Join(root, "user.json")
	metaJSON := filepath.Join(root, "music_meta.json")
	{
		err := os.WriteFile(userJSON, []byte(`{"now":1700000000,"userGamedata":{"userId":1,"name":"Tester","deck":1},"userProfile":{},"userDecks":[{"deckId":1}],"userCards":[]}`), 0o644)
		testutil.Require(t, !(err != nil), "write user snapshot: %v", err)
	}
	{

		err := os.WriteFile(metaJSON, []byte(`[{"music_id":1,"difficulty":"master","event_rate":100}]`), 0o644)
		testutil.Require(t, !(err != nil), "write music metadata: %v", err)
	}

	helper := assets.NewAssetHelper(root, nil)
	snapshot := rendersnapshot.NewLocalFileService(nil, helper, rendersnapshot.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSON,
		MusicMetaJSON: metaJSON,
	})
	controller := music.NewController(&scoreControlTestSource{musics: map[int]*masterdata.Music{
		1: {ID: 1, Title: "Room Song", AssetBundleName: "jacket_room"},
	}}, nil, helper, snapshot, nil)

	req, err := BuildCustomRoomScoreRequest(&CommandInput{Region: "jp", Query: "22"}, &renderapp.App{Music: controller})
	testutil.Require(t, !(err != nil), "BuildCustomRoomScoreRequest() error = %v", err)
	{
		testutil.Require(t, !(req.TargetPoint != 22), "unexpected custom-room request: %+v", req)
		testutil.Require(t, !(len(req.CandidatePairs) == 0), "unexpected custom-room request: %+v", req)
		testutil.Require(t, !(len(req.MusicListMap[100]) != 1), "unexpected custom-room request: %+v", req)
	}

}
