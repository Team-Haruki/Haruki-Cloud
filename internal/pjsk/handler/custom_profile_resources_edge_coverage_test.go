package handler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderhonor "haruki-cloud/internal/pjsk/render/honor"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestCustomProfileCollectorAndMissingProviderBranches(t *testing.T) {
	card := sekaiapi.UserCustomProfileCard{CustomProfileCard: sekaiapi.ProfileCardData{
		Generals:           []sekaiapi.GeneralData{{PlayerInfoResourceID: 14}},
		GeneralBackgrounds: []sekaiapi.ImageData{{ID: 1}},
		StoryBackgrounds:   []sekaiapi.ImageData{{ID: 2}},
		StandMembers:       []sekaiapi.ImageData{{ID: 3}},
		Collections:        []sekaiapi.CollectionData{{ID: 4, TargetID: 5}, {ID: 0, TargetID: 6}},
		Others:             []sekaiapi.ImageData{{ID: 7}},
		Honors:             []sekaiapi.HonorData{{ID: 8, FullSize: true}, {ID: 0}},
		BondsHonors:        []sekaiapi.BondsHonorData{{ID: 9, WordID: 10, Inverse: true, UseUnitVirtualSinger: true}, {ID: 0}},
	}}
	resp := &sekaiapi.GetAnotherProfileResponse{
		UserDeck:          sekaiapi.UserDeck{Leader: 11, Member1: 12, Member2: 13, Member3: 14, Member4: 15, Member5: 16},
		UserProfileHonors: []sekaiapi.UserProfileHonor{{Seq: 1, HonorID: 17, HonorLevel: 2}, {Seq: 2, HonorID: 0}},
		UserHonors:        []sekaiapi.UserHonor{{HonorID: 8, Level: 3}},
		UserBondsHonors:   []sekaiapi.UserBondsHonor{{BondsHonorID: 9, Level: 4}},
		UserStoryFavorites: []sekaiapi.UserStoryFavorite{
			{StoryType: "event_story", StoryID: 18},
		},
	}
	c := newCustomProfileResourceCollector(card, resp)
	for id, set := range map[int]map[int]struct{}{1: c.generalBgIDs, 2: c.storyBgIDs, 3: c.standMemberIDs, 7: c.otherIDs, 11: c.cardIDs, 16: c.cardIDs} {
		if _, ok := set[id]; !ok {
			t.Fatalf("collector missing id %d", id)
		}
	}
	if len(c.profileHonors) == 0 || len(c.honorQueries) == 0 || len(c.bondsHonorQueries) == 0 || len(c.storyFavorites) != 1 {
		t.Fatalf("collector query sets are incomplete: %+v", c)
	}

	resources := drawing.CustomProfileResources{}
	if err := collectCustomProfileMasterResources(context.Background(), nil, renderregion.JP, c, resources); err == nil {
		t.Fatal("missing custom-profile masterdata unexpectedly succeeded")
	}
	if err := collectCustomProfileOmikujiResources(context.Background(), nil, renderregion.JP, customProfileResourceCollector{}, nil, resources); err != nil {
		t.Fatalf("empty omikuji collector = %v", err)
	}
	omikujiCollector := customProfileResourceCollector{collectionTargets: map[int]map[int]struct{}{1: {2: {}}}}
	if err := collectCustomProfileOmikujiResources(context.Background(), nil, renderregion.JP, omikujiCollector,
		map[int]map[string]any{1: {"customProfileResourceCollectionType": " OMIKUJI "}}, resources); err == nil {
		t.Fatal("missing omikuji masterdata unexpectedly succeeded")
	}

	if err := collectCustomProfileStampResources(context.Background(), nil, renderregion.JP, customProfileResourceCollector{}, resources); err != nil {
		t.Fatalf("empty stamp collector = %v", err)
	}
	if err := collectCustomProfileStampResources(context.Background(), nil, renderregion.JP,
		customProfileResourceCollector{stampIDs: map[int]struct{}{1: {}}}, resources); err == nil {
		t.Fatal("missing stamp provider unexpectedly succeeded")
	}
	if err := collectCustomProfileCardResources(context.Background(), nil, renderregion.JP, customProfileResourceCollector{}, resources); err != nil {
		t.Fatalf("empty card collector = %v", err)
	}
	if err := collectCustomProfileCardResources(context.Background(), nil, renderregion.JP,
		customProfileResourceCollector{cardIDs: map[int]struct{}{1: {}}}, resources); err == nil {
		t.Fatal("missing card provider unexpectedly succeeded")
	}
	if err := collectCustomProfileHonorResources(context.Background(), nil, renderregion.JP, customProfileResourceCollector{}, resources); err != nil {
		t.Fatalf("empty honor collector = %v", err)
	}
	if err := collectCustomProfileHonorResources(context.Background(), nil, renderregion.JP,
		customProfileResourceCollector{honorQueries: map[string]renderhonor.Query{"x": {HonorID: 1}}}, resources); err == nil {
		t.Fatal("missing honor controller unexpectedly succeeded")
	}
}

func TestCustomProfileStoryAndMasterdataErrorBranches(t *testing.T) {
	resources := drawing.CustomProfileResources{}
	if err := collectCustomProfileStoryFavoriteResources(context.Background(), nil, renderregion.JP, customProfileResourceCollector{}, resources); err != nil {
		t.Fatalf("empty story favorites = %v", err)
	}
	invalidStories := customProfileResourceCollector{storyFavorites: []sekaiapi.UserStoryFavorite{{StoryID: 0}, {StoryID: 1, StoryType: "unknown"}}}
	if err := collectCustomProfileStoryFavoriteResources(context.Background(), nil, renderregion.JP, invalidStories, resources); err != nil {
		t.Fatalf("ignored story favorites = %v", err)
	}
	eventStories := customProfileResourceCollector{storyFavorites: []sekaiapi.UserStoryFavorite{{StoryID: 1, StoryType: "event_story"}}}
	if err := collectCustomProfileStoryFavoriteResources(context.Background(), nil, renderregion.JP, eventStories, resources); err == nil {
		t.Fatal("missing event-story masterdata unexpectedly succeeded")
	}
	if got, err := loadCustomProfileMasterTable(context.Background(), nil, renderregion.JP, "x.json", nil); err != nil || len(got) != 0 {
		t.Fatalf("empty master table = %#v, %v", got, err)
	}
	if _, err := loadCustomProfileMasterTable(context.Background(), nil, renderregion.JP, "missing.json", map[int]struct{}{1: {}}); err == nil {
		t.Fatal("missing master table unexpectedly succeeded")
	}

	if customProfileMasterdataDirs(nil, renderregion.JP) != nil {
		t.Fatal("nil app masterdata dirs should be nil")
	}
	root := filepath.Join(t.TempDir(), "nested", "master")
	app := &renderapp.App{Config: renderapp.Config{
		LocalMasterdata: renderapp.LocalMasterdataConfig{Dir: root},
		DeckRecommend:   renderapp.DeckRecommendConfig{MasterdataDir: root},
	}}
	dirs := customProfileMasterdataDirs(app, renderregion.CN)
	if len(dirs) < 3 {
		t.Fatalf("masterdata dirs = %#v", dirs)
	}
	for _, tc := range []struct {
		region renderregion.Value
		want   string
	}{{renderregion.JP, "haruki-sekai-master"}, {renderregion.CN, "haruki-sekai-sc-master"}, {renderregion.TW, "haruki-sekai-tc-master"}, {renderregion.KR, "haruki-sekai-kr-master"}, {renderregion.EN, "haruki-sekai-en-master"}} {
		if got := customProfileMasterdataRepoDir(tc.region); got != tc.want {
			t.Fatalf("repo dir for %s = %q", tc.region, got)
		}
	}
}

func TestCustomProfileMappingAndPathBranches(t *testing.T) {
	local := provider.NewLocalProvider(t.TempDir(), renderregion.JP)
	app := &renderapp.App{Provider: local, Providers: map[renderregion.Value]provider.MasterDataProvider{renderregion.CN: local}}
	if customProfileProviderForRegion(nil, renderregion.JP) != nil || customProfileProviderForRegion(app, renderregion.CN) != local || customProfileProviderForRegion(app, renderregion.JP) != local {
		t.Fatal("custom profile provider selection mismatch")
	}

	card := &masterdata.Card{ID: 1, CharacterID: 2, CardRarityType: "rarity_4", Attr: "cool", Prefix: "prefix", AssetBundleName: "bundle", ReleaseAt: 3}
	master := customProfileCardMasterMap(card)
	assets := customProfileCardAssetMap(nil, renderregion.JP, card)
	if master["id"] != 1 || !strings.Contains(assets["normalPath"].(string), "bundle") {
		t.Fatalf("card mappings = %#v / %#v", master, assets)
	}
	for _, after := range []bool{false, true} {
		for _, path := range []string{
			resolveCustomProfileSmallCardImagePath(nil, renderregion.JP, "bundle", after),
			resolveCustomProfileDeckCardImagePath(nil, renderregion.JP, "bundle", after),
			resolveCustomProfileClipCardImagePath(nil, renderregion.JP, "bundle", after),
		} {
			if !strings.Contains(path, "bundle") {
				t.Fatalf("card path = %q", path)
			}
		}
	}
	if resolveCustomProfileEventStoryBannerPath(nil, renderregion.JP, " ") != "" || resolveCustomProfileUnitStoryBannerPath(nil, renderregion.JP, "") != "" {
		t.Fatal("empty story bundle should produce no path")
	}
	if !strings.Contains(resolveCustomProfileEventStoryBannerPath(nil, renderregion.JP, "event"), "event") ||
		!strings.Contains(resolveCustomProfileUnitStoryBannerPath(nil, renderregion.JP, "unit"), "unit") {
		t.Fatal("story banner path mismatch")
	}

	for _, tc := range []struct {
		row  map[string]any
		want string
	}{
		{map[string]any{"title": " Title "}, "Title"},
		{map[string]any{"outline": "First\nSecond"}, "First"},
		{map[string]any{"outline": "Only"}, "Only"},
		{map[string]any{}, ""},
	} {
		if got := customProfileUnitStoryTitle(tc.row); got != tc.want {
			t.Fatalf("unit story title = %q, want %q", got, tc.want)
		}
	}
	if resolveCustomProfileResourceImagePath(nil, renderregion.JP, map[string]any{}, "") != "" {
		t.Fatal("missing resource filename should produce no path")
	}
	for _, row := range []map[string]any{
		{"fileName": "image", "resourceLoadVal": "custom_profile/shape"},
		{"fileName": "image.png", "resourceLoadVal": "custom_profile"},
		{"fileName": "image", "resourceLoadVal": "shape"},
		{"fileName": "image"},
	} {
		if got := resolveCustomProfileResourceImagePath(nil, renderregion.JP, row, "fallback"); !strings.HasSuffix(got, ".png") {
			t.Fatalf("resource path = %q", got)
		}
	}
}

func TestCustomProfileHonorAndMapHelpers(t *testing.T) {
	resp := &sekaiapi.GetAnotherProfileResponse{
		UserHonors:        []sekaiapi.UserHonor{{HonorID: 1, Level: 2}},
		UserProfileHonors: []sekaiapi.UserProfileHonor{{HonorID: 3, HonorLevel: 4}},
		UserBondsHonors:   []sekaiapi.UserBondsHonor{{BondsHonorID: 5, Level: 6}},
		UserMusicDifficultyClearCount: []sekaiapi.AnotherUserMusicDifficultyClearCount{
			{MusicDifficultyType: sekaiapi.MusicDifficultyMaster, FullCombo: 7, AllPerfect: 8},
		},
	}
	if customProfileUserHonorLevel(nil, 1) != 0 || customProfileUserHonorLevel(resp, 1) != 2 || customProfileUserHonorLevel(resp, 3) != 4 || customProfileUserHonorLevel(resp, 99) != 0 {
		t.Fatal("custom profile honor level mismatch")
	}
	if customProfileUserBondsHonorLevel(nil, 5) != 0 || customProfileUserBondsHonorLevel(resp, 5) != 6 || customProfileUserBondsHonorLevel(resp, 99) != 0 {
		t.Fatal("custom profile bonds level mismatch")
	}
	if customProfileHonorFcApLevels(nil) != nil || len(customProfileHonorFcApLevels(resp)) == 0 {
		t.Fatal("custom profile FC/AP levels mismatch")
	}
	if !strings.Contains(customProfileHonorRequestKey(1, 2, true), "main") || !strings.Contains(customProfileHonorRequestKey(1, 2, false), "sub") ||
		!strings.Contains(customProfileBondsHonorRequestKey(1, 2, true, 3, true, true), "reverse:unit_vs") ||
		customProfileStoryFavoriteKey(" event ", 1) != "event:1" || customProfileBondsViewType(true) != "reverse" || customProfileBondsViewType(false) != "" {
		t.Fatal("custom profile request key mismatch")
	}

	set := map[int]struct{}{}
	addID(set, 0)
	addID(set, 2)
	nested := map[int]map[int]struct{}{}
	addNestedID(nested, 0, 1)
	addNestedID(nested, 1, 2)
	addNestedID(nested, 1, 3)
	if len(set) != 1 || len(nested[1]) != 2 {
		t.Fatalf("ID helpers = %#v / %#v", set, nested)
	}

	for _, tc := range []struct {
		value any
		want  int
		ok    bool
	}{{1, 1, true}, {int64(2), 2, true}, {float64(3), 3, true}, {" 4 ", 4, true}, {"bad", 0, false}, {true, 0, false}} {
		got, ok := mapInt(map[string]any{"x": tc.value}, "x")
		if got != tc.want || ok != tc.ok {
			t.Fatalf("mapInt(%#v) = %d, %v", tc.value, got, ok)
		}
	}
	if _, ok := mapInt(nil, "x"); ok || mapString(nil, "x") != "" || mapString(map[string]any{"x": nil}, "x") != "" || mapString(map[string]any{"x": json.Number("5")}, "x") != "5" {
		t.Fatal("map helper fallback mismatch")
	}
	if got := customProfileCharaRankIconPathMap(nil); len(got) == 0 {
		t.Fatal("character rank icon map should not be empty")
	}
	if !reflect.DeepEqual(customProfileHonorFcApLevels(&sekaiapi.GetAnotherProfileResponse{}), map[int]*int(nil)) {
		t.Fatal("empty FC/AP data should return nil")
	}
}
