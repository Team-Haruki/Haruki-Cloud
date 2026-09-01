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
	"haruki-cloud/internal/testutil"
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
		{
			_, ok := set[id]
			testutil.Require(t, ok, "collector missing id %d", id)
		}

	}
	{
		testutil.Require(t, !(len(c.profileHonors) == 0), "collector query sets are incomplete: %+v", c)
		testutil.Require(t, !(len(c.honorQueries) == 0), "collector query sets are incomplete: %+v", c)
		testutil.Require(t, !(len(c.bondsHonorQueries) == 0), "collector query sets are incomplete: %+v", c)
		testutil.Require(t, !(len(c.storyFavorites) != 1), "collector query sets are incomplete: %+v", c)
	}

	resources := drawing.CustomProfileResources{}
	{
		err := collectCustomProfileMasterResources(context.Background(), nil, renderregion.JP, c, resources)
		testutil.RequireArgs(t, !(err == nil), "missing custom-profile masterdata unexpectedly succeeded")
	}
	{

		err := collectCustomProfileOmikujiResources(context.Background(), nil, renderregion.JP, customProfileResourceCollector{}, nil, resources)
		testutil.Require(t, !(err != nil), "empty omikuji collector = %v", err)
	}

	omikujiCollector := customProfileResourceCollector{collectionTargets: map[int]map[int]struct{}{1: {2: {}}}}
	{
		err := collectCustomProfileOmikujiResources(context.Background(), nil, renderregion.JP, omikujiCollector,
			map[int]map[string]any{1: {"customProfileResourceCollectionType": " OMIKUJI "}}, resources)
		testutil.RequireArgs(t, !(err == nil), "missing omikuji masterdata unexpectedly succeeded")
	}
	{

		err := collectCustomProfileStampResources(context.Background(), nil, renderregion.JP, customProfileResourceCollector{}, resources)
		testutil.Require(t, !(err != nil), "empty stamp collector = %v", err)
	}
	{

		err := collectCustomProfileStampResources(context.Background(), nil, renderregion.JP,
			customProfileResourceCollector{stampIDs: map[int]struct{}{1: {}}}, resources)
		testutil.RequireArgs(t, !(err == nil), "missing stamp provider unexpectedly succeeded")
	}
	{

		err := collectCustomProfileCardResources(context.Background(), nil, renderregion.JP, customProfileResourceCollector{}, resources)
		testutil.Require(t, !(err != nil), "empty card collector = %v", err)
	}
	{

		err := collectCustomProfileCardResources(context.Background(), nil, renderregion.JP,
			customProfileResourceCollector{cardIDs: map[int]struct{}{1: {}}}, resources)
		testutil.RequireArgs(t, !(err == nil), "missing card provider unexpectedly succeeded")
	}
	{

		err := collectCustomProfileHonorResources(context.Background(), nil, renderregion.JP, customProfileResourceCollector{}, resources)
		testutil.Require(t, !(err != nil), "empty honor collector = %v", err)
	}
	{

		err := collectCustomProfileHonorResources(context.Background(), nil, renderregion.JP,
			customProfileResourceCollector{honorQueries: map[string]renderhonor.Query{"x": {HonorID: 1}}}, resources)
		testutil.RequireArgs(t, !(err == nil), "missing honor controller unexpectedly succeeded")
	}

}

func TestCustomProfileStoryAndMasterdataErrorBranches(t *testing.T) {
	resources := drawing.CustomProfileResources{}
	{
		err := collectCustomProfileStoryFavoriteResources(context.Background(), nil, renderregion.JP, customProfileResourceCollector{}, resources)
		testutil.Require(t, !(err != nil), "empty story favorites = %v", err)
	}

	invalidStories := customProfileResourceCollector{storyFavorites: []sekaiapi.UserStoryFavorite{{StoryID: 0}, {StoryID: 1, StoryType: "unknown"}}}
	{
		err := collectCustomProfileStoryFavoriteResources(context.Background(), nil, renderregion.JP, invalidStories, resources)
		testutil.Require(t, !(err != nil), "ignored story favorites = %v", err)
	}

	eventStories := customProfileResourceCollector{storyFavorites: []sekaiapi.UserStoryFavorite{{StoryID: 1, StoryType: "event_story"}}}
	{
		err := collectCustomProfileStoryFavoriteResources(context.Background(), nil, renderregion.JP, eventStories, resources)
		testutil.RequireArgs(t, !(err == nil), "missing event-story masterdata unexpectedly succeeded")
	}
	{

		got, err := loadCustomProfileMasterTable(context.Background(), nil, renderregion.JP, "x.json", nil)
		{
			testutil.Require(t, !(err != nil), "empty master table = %#v, %v", got, err)
			testutil.Require(t, !(len(got) != 0), "empty master table = %#v, %v", got, err)
		}
	}
	{

		_, err := loadCustomProfileMasterTable(context.Background(), nil, renderregion.JP, "missing.json", map[int]struct{}{1: {}})
		testutil.RequireArgs(t, !(err == nil), "missing master table unexpectedly succeeded")
	}
	testutil.RequireArgs(t, !(customProfileMasterdataDirs(nil, renderregion.JP) != nil), "nil app masterdata dirs should be nil")

	root := filepath.Join(t.TempDir(), "nested", "master")
	app := &renderapp.App{Config: renderapp.Config{
		LocalMasterdata: renderapp.LocalMasterdataConfig{Dir: root},
		DeckRecommend:   renderapp.DeckRecommendConfig{MasterdataDir: root},
	}}
	dirs := customProfileMasterdataDirs(app, renderregion.CN)
	testutil.Require(t, !(len(dirs) < 3), "masterdata dirs = %#v", dirs)

	for _, tc := range []struct {
		region renderregion.Value
		want   string
	}{{renderregion.JP, "haruki-sekai-master"}, {renderregion.CN, "haruki-sekai-sc-master"}, {renderregion.TW, "haruki-sekai-tc-master"}, {renderregion.KR, "haruki-sekai-kr-master"}, {renderregion.EN, "haruki-sekai-en-master"}} {
		{
			got := customProfileMasterdataRepoDir(tc.region)
			testutil.Require(t, !(got != tc.want), "repo dir for %s = %q", tc.region, got)
		}

	}
}

func TestCustomProfileMappingAndPathBranches(t *testing.T) {
	local := provider.NewLocalProvider(t.TempDir(), renderregion.JP)
	app := &renderapp.App{Provider: local, Providers: map[renderregion.Value]provider.MasterDataProvider{renderregion.CN: local}}
	{
		testutil.RequireArgs(t, !(customProfileProviderForRegion(nil, renderregion.JP) != nil), "custom profile provider selection mismatch")
		testutil.RequireArgs(t, !(customProfileProviderForRegion(app, renderregion.CN) != local), "custom profile provider selection mismatch")
		testutil.RequireArgs(t, !(customProfileProviderForRegion(app, renderregion.JP) != local), "custom profile provider selection mismatch")
	}

	card := &masterdata.Card{ID: 1, CharacterID: 2, CardRarityType: "rarity_4", Attr: "cool", Prefix: "prefix", AssetBundleName: "bundle", ReleaseAt: 3}
	master := customProfileCardMasterMap(card)
	assets := customProfileCardAssetMap(nil, renderregion.JP, card)
	{
		testutil.Require(t, !(master["id"] != 1), "card mappings = %#v / %#v", master, assets)
		testutil.Require(t, strings.Contains(assets["normalPath"].(string), "bundle"), "card mappings = %#v / %#v", master, assets)
	}

	for _, after := range []bool{false, true} {
		for _, path := range []string{
			resolveCustomProfileSmallCardImagePath(nil, renderregion.JP, "bundle", after),
			resolveCustomProfileDeckCardImagePath(nil, renderregion.JP, "bundle", after),
			resolveCustomProfileClipCardImagePath(nil, renderregion.JP, "bundle", after),
		} {
			testutil.Require(t, strings.Contains(path, "bundle"), "card path = %q", path)

		}
	}
	{
		testutil.RequireArgs(t, !(resolveCustomProfileEventStoryBannerPath(nil, renderregion.JP, " ") != ""), "empty story bundle should produce no path")
		testutil.RequireArgs(t, !(resolveCustomProfileUnitStoryBannerPath(nil, renderregion.JP, "") != ""), "empty story bundle should produce no path")
	}
	{
		testutil.RequireArgs(t, strings.Contains(resolveCustomProfileEventStoryBannerPath(nil, renderregion.JP, "event"), "event"), "story banner path mismatch")
		testutil.RequireArgs(t, strings.Contains(resolveCustomProfileUnitStoryBannerPath(nil, renderregion.JP, "unit"), "unit"), "story banner path mismatch")
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
		{
			got := customProfileUnitStoryTitle(tc.row)
			testutil.Require(t, !(got != tc.want), "unit story title = %q, want %q", got, tc.want)
		}

	}
	testutil.RequireArgs(t, !(resolveCustomProfileResourceImagePath(nil, renderregion.JP, map[string]any{}, "") != ""), "missing resource filename should produce no path")

	for _, row := range []map[string]any{
		{"fileName": "image", "resourceLoadVal": "custom_profile/shape"},
		{"fileName": "image.png", "resourceLoadVal": "custom_profile"},
		{"fileName": "image", "resourceLoadVal": "shape"},
		{"fileName": "image"},
	} {
		{
			got := resolveCustomProfileResourceImagePath(nil, renderregion.JP, row, "fallback")
			testutil.Require(t, strings.HasSuffix(got, ".png"), "resource path = %q", got)
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
	{
		testutil.RequireArgs(t, !(customProfileUserHonorLevel(nil, 1) != 0), "custom profile honor level mismatch")
		testutil.RequireArgs(t, !(customProfileUserHonorLevel(resp, 1) != 2), "custom profile honor level mismatch")
		testutil.RequireArgs(t, !(customProfileUserHonorLevel(resp, 3) != 4), "custom profile honor level mismatch")
		testutil.RequireArgs(t, !(customProfileUserHonorLevel(resp, 99) != 0), "custom profile honor level mismatch")
	}
	{
		testutil.RequireArgs(t, !(customProfileUserBondsHonorLevel(nil, 5) != 0), "custom profile bonds level mismatch")
		testutil.RequireArgs(t, !(customProfileUserBondsHonorLevel(resp, 5) != 6), "custom profile bonds level mismatch")
		testutil.RequireArgs(t, !(customProfileUserBondsHonorLevel(resp, 99) != 0), "custom profile bonds level mismatch")
	}
	{
		testutil.RequireArgs(t, !(customProfileHonorFcApLevels(nil) != nil), "custom profile FC/AP levels mismatch")
		testutil.RequireArgs(t, !(len(customProfileHonorFcApLevels(resp)) == 0), "custom profile FC/AP levels mismatch")
	}
	{
		testutil.RequireArgs(t, strings.Contains(customProfileHonorRequestKey(1, 2, true), "main"), "custom profile request key mismatch")
		testutil.RequireArgs(t, strings.Contains(customProfileHonorRequestKey(1, 2, false), "sub"), "custom profile request key mismatch")
		testutil.RequireArgs(t, strings.Contains(customProfileBondsHonorRequestKey(1, 2, true, 3, true, true), "reverse:unit_vs"), "custom profile request key mismatch")
		testutil.RequireArgs(t, !(customProfileStoryFavoriteKey(" event ", 1) != "event:1"), "custom profile request key mismatch")
		testutil.RequireArgs(t, !(customProfileBondsViewType(true) != "reverse"), "custom profile request key mismatch")
		testutil.RequireArgs(t, !(customProfileBondsViewType(false) != ""), "custom profile request key mismatch")
	}

	set := map[int]struct{}{}
	addID(set, 0)
	addID(set, 2)
	nested := map[int]map[int]struct{}{}
	addNestedID(nested, 0, 1)
	addNestedID(nested, 1, 2)
	addNestedID(nested, 1, 3)
	{
		testutil.Require(t, !(len(set) != 1), "ID helpers = %#v / %#v", set, nested)
		testutil.Require(t, !(len(nested[1]) != 2), "ID helpers = %#v / %#v", set, nested)
	}

	for _, tc := range []struct {
		value any
		want  int
		ok    bool
	}{{1, 1, true}, {int64(2), 2, true}, {float64(3), 3, true}, {" 4 ", 4, true}, {"bad", 0, false}, {true, 0, false}} {
		got, ok := mapInt(map[string]any{"x": tc.value}, "x")
		{
			testutil.Require(t, !(got != tc.want), "mapInt(%#v) = %d, %v", tc.value, got, ok)
			testutil.Require(t, !(ok != tc.ok), "mapInt(%#v) = %d, %v", tc.value, got, ok)
		}

	}
	{
		_, ok := mapInt(nil, "x")
		{
			testutil.RequireArgs(t, !(ok), "map helper fallback mismatch")
			testutil.RequireArgs(t, !(mapString(nil, "x") != ""), "map helper fallback mismatch")
			testutil.RequireArgs(t, !(mapString(map[string]any{"x": nil}, "x") != ""), "map helper fallback mismatch")
			testutil.RequireArgs(t, !(mapString(map[string]any{"x": json.Number("5")}, "x") != "5"), "map helper fallback mismatch")
		}
	}
	{

		got := customProfileCharaRankIconPathMap(nil)
		testutil.RequireArgs(t, !(len(got) == 0), "character rank icon map should not be empty")
	}
	testutil.RequireArgs(t, reflect.DeepEqual(customProfileHonorFcApLevels(&sekaiapi.GetAnotherProfileResponse{}), map[int]*int(nil)), "empty FC/AP data should return nil")

}
