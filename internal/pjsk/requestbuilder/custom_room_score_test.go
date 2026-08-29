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
)

func TestResolveCustomRoomScoreSelection(t *testing.T) {
	selection, err := resolveCustomRoomScoreSelection(&CommandInput{Params: []byte(`{"target_point":42}`), Query: "99"})
	if err != nil || selection.TargetPoint != 42 {
		t.Fatalf("selection from params = %#v, %v", selection, err)
	}
	selection, err = resolveCustomRoomScoreSelection(&CommandInput{Query: " 23 "})
	if err != nil || selection.TargetPoint != 23 {
		t.Fatalf("selection from query = %#v, %v", selection, err)
	}
	for name, input := range map[string]*CommandInput{
		"nil":          nil,
		"empty":        {},
		"negative":     {Query: "-1"},
		"invalid json": {Params: []byte(`{`)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := resolveCustomRoomScoreSelection(input); err == nil {
				t.Fatal("invalid selection unexpectedly succeeded")
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
		if !ok || got != want {
			t.Errorf("parseCustomRoomBonus(%q) = (%d, %t)", raw, got, ok)
		}
	}
	for _, raw := range []string{"", "%", "invalid"} {
		if _, ok := parseCustomRoomBonus(raw); ok {
			t.Errorf("parseCustomRoomBonus(%q) unexpectedly succeeded", raw)
		}
	}

	pairs, err := findCustomRoomCandidatePairs(22)
	if err != nil || len(pairs) == 0 {
		t.Fatalf("find candidate pairs = %#v, %v", pairs, err)
	}
	found := false
	for _, pair := range pairs {
		if len(pair) == 2 && pair[0] == 100 && pair[1] == 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("candidate pairs do not contain the CSV's first cell: %#v", pairs)
	}
	missing, err := findCustomRoomCandidatePairs(-999)
	if err != nil || len(missing) != 0 {
		t.Fatalf("missing candidate pairs = %#v, %v", missing, err)
	}

	rates := customRoomEventRates([][]int{{100, 0}, {100, 5}, nil, {-1, 10}, {103, 10}})
	if len(rates) != 2 || rates[0] != 100 || rates[1] != 103 {
		t.Fatalf("custom room rates = %#v", rates)
	}
}

func TestBuildCustomRoomScoreRequestRequiresController(t *testing.T) {
	if _, err := BuildCustomRoomScoreRequest(&CommandInput{Query: "22"}, nil); err == nil || !strings.Contains(err.Error(), "music controller") {
		t.Fatalf("nil app error = %v", err)
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
			if _, err := BuildCustomRoomScoreRequest(input, app); err == nil {
				t.Fatal("invalid custom-room request unexpectedly succeeded")
			}
		})
	}
	if _, err := BuildCustomRoomScoreRequest(&CommandInput{Query: "22"}, app); err == nil || !strings.Contains(err.Error(), "data source") {
		t.Fatalf("missing music source error = %v", err)
	}
}

func TestBuildCustomRoomScoreRequestSuccess(t *testing.T) {
	root := t.TempDir()
	userJSON := filepath.Join(root, "user.json")
	metaJSON := filepath.Join(root, "music_meta.json")
	if err := os.WriteFile(userJSON, []byte(`{"now":1700000000,"userGamedata":{"userId":1,"name":"Tester","deck":1},"userProfile":{},"userDecks":[{"deckId":1}],"userCards":[]}`), 0o644); err != nil {
		t.Fatalf("write user snapshot: %v", err)
	}
	if err := os.WriteFile(metaJSON, []byte(`[{"music_id":1,"difficulty":"master","event_rate":100}]`), 0o644); err != nil {
		t.Fatalf("write music metadata: %v", err)
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
	if err != nil {
		t.Fatalf("BuildCustomRoomScoreRequest() error = %v", err)
	}
	if req.TargetPoint != 22 || len(req.CandidatePairs) == 0 || len(req.MusicListMap[100]) != 1 {
		t.Fatalf("unexpected custom-room request: %+v", req)
	}
}
