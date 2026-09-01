package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/testutil"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	renderhonor "haruki-cloud/internal/pjsk/render/honor"
	"haruki-cloud/internal/pjsk/render/provider"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestBuildCustomProfileResourcesResolvesPathsInCloud(t *testing.T) {
	root := t.TempDir()
	masterRoot := filepath.Join(root, "masterdata")
	master := filepath.Join(masterRoot, "haruki-sekai-sc-master", "master")
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileTextColors.json"), []map[string]any{
		{"id": 10, "colorCode": "#ffffff"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileTextFonts.json"), []map[string]any{
		{"id": 11, "fontName": "FOT-RodinNTLGPro-DB"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfilePlayerInfoResources.json"), []map[string]any{
		{"id": 14, "fileName": "StoryFavorite"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileCollectionResources.json"), []map[string]any{
		{"id": 1000, "customProfileResourceCollectionType": "omikuji", "resourceLoadVal": "lottery_game/new_year_2026", "fileName": "Prefabs/Omikuji"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "omikujis.json"), []map[string]any{
		{"id": 183, "unit": "idol", "fortuneType": "grate_fortune", "summary": "Test summary"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileShapeResources.json"), []map[string]any{
		{"id": 12, "resourceLoadVal": "custom_profile/shape", "fileName": "circle"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileCharacterIconResources.json"), []map[string]any{
		{"id": 21, "customProfileResourceType": "character_icon", "resourceLoadVal": "custom_profile/character_icon", "fileName": "profile_chr_icon_miku"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileMaterialResources.json"), []map[string]any{
		{"id": 1, "customProfileResourceType": "material", "resourceLoadVal": "custom_profile/material", "fileName": "profile_icon_item_0001"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileUserInterfaceIconResources.json"), []map[string]any{
		{"id": 42, "customProfileResourceType": "user_interface_icon", "resourceLoadVal": "custom_profile/user_interface_icon", "fileName": "profile_icon_0042"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "stamps.json"), []map[string]any{
		{"id": 146, "assetbundleName": "stamp0230", "characterId": 1},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "cards.json"), []map[string]any{
		{"id": 915, "characterId": 10, "cardRarityType": "rarity_4", "attr": "cute", "prefix": "Card", "assetbundleName": "res010_no034"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "honors.json"), []map[string]any{
		{"id": 7001, "groupId": 701, "honorRarity": "low", "assetbundleName": "honor_top_event_demo"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "honorGroups.json"), []map[string]any{
		{"id": 701, "honorType": "event", "backgroundAssetBundleName": "honor_bg_event_demo"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "bondsHonors.json"), []map[string]any{
		{"id": 1020501, "gameCharacterUnitID1": 11, "gameCharacterUnitID2": 22, "honorRarity": "highest"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "bondsHonorWords.json"), []map[string]any{
		{"id": 10205002, "assetBundleName": "honorname_0205_default_0502"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "gameCharacterUnits.json"), []map[string]any{
		{"id": 11, "gameCharacterID": 2},
		{"id": 22, "gameCharacterID": 5},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "eventStories.json"), []map[string]any{
		{"id": 19, "eventId": 190, "assetbundleName": "event_story_test"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "events.json"), []map[string]any{
		{"id": 190, "name": "Test Event Story", "assetbundleName": "event_story_test"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "unitStoryEpisodeGroups.json"), []map[string]any{
		{"id": 5, "unit": "piapro", "unitEpisodeCategory": "school_refusal", "outline": "Test Unit Story\nSecond line", "assetbundleName": "main_schoolrefusal_piapro"},
	})

	src := provider.NewLocalProvider(master, renderregion.CN)
	app := &renderapp.App{
		Assets:   assets.NewAssetHelper(root, nil),
		Provider: src,
		Providers: map[renderregion.Value]provider.MasterDataProvider{
			renderregion.CN: src,
		},
		Honors: renderhonor.NewController(renderhonor.NewProviderAdapter(src), nil, assets.NewAssetHelper(root, nil)),
		Config: renderapp.Config{
			LocalMasterdata: renderapp.LocalMasterdataConfig{Dir: masterRoot},
		},
	}

	card := sekaiapi.UserCustomProfileCard{
		CustomProfileID:     1,
		CustomProfileCardID: 1,
		CustomProfileCard: sekaiapi.ProfileCardData{
			Texts: []sekaiapi.TextData{{
				FontID:         11,
				ColorID:        10,
				OutlineColorID: 10,
			}},
			Shapes:             []sekaiapi.ShapeData{{ID: 12, ColorID: 10}},
			Stamps:             []sekaiapi.ImageData{{ID: 146}},
			CharacterIcons:     []sekaiapi.ImageData{{ID: 21}},
			Materials:          []sekaiapi.ImageData{{ID: 1}},
			UserInterfaceIcons: []sekaiapi.ImageData{{ID: 42}},
			CardMembers:        []sekaiapi.CardData{{ID: 915}},
			Collections: []sekaiapi.CollectionData{{
				ID:       1000,
				TargetID: 183,
			}},
			Honors:      []sekaiapi.HonorData{{ID: 7001, FullSize: true}},
			BondsHonors: []sekaiapi.BondsHonorData{{ID: 1020501, FullSize: true, WordID: 10205002, Inverse: true}},
			Generals:    []sekaiapi.GeneralData{{PlayerInfoResourceID: 14}},
		},
	}
	resp := &sekaiapi.GetAnotherProfileResponse{
		UserCards:       []sekaiapi.AnotherUserCard{{CardID: 915, SpecialTrainingStatus: "done", DefaultImage: "special_training"}},
		UserHonors:      []sekaiapi.UserHonor{{HonorID: 7001, Level: 1}},
		UserBondsHonors: []sekaiapi.UserBondsHonor{{BondsHonorID: 1020501, Level: 3}},
		UserStoryFavorites: []sekaiapi.UserStoryFavorite{
			{StoryType: "event_story", StoryID: 19},
			{StoryType: "unit_story", StoryID: 5},
		},
	}

	resources, err := buildCustomProfileResources(context.Background(), app, "cn", card, resp)
	testutil.Require(t, !(err != nil), "buildCustomProfileResources() error = %v", err)

	shape := resources["customProfileShapeResources"].(map[int]map[string]any)[12]
	{
		got := shape["imagePath"].(string)
		testutil.Require(t, !(got != "asset/cn-assets/startapp/custom_profile/shape/circle.png"), "unexpected shape image path: %s", got)
	}

	characterIcon := resources["customProfileCharacterIconResources"].(map[int]map[string]any)[21]
	{
		got := characterIcon["imagePath"].(string)
		testutil.Require(t, !(got != "asset/cn-assets/startapp/custom_profile/character_icon/profile_chr_icon_miku.png"), "unexpected character icon image path: %s", got)
	}

	material := resources["customProfileMaterialResources"].(map[int]map[string]any)[1]
	{
		got := material["imagePath"].(string)
		testutil.Require(t, !(got != "asset/cn-assets/startapp/custom_profile/material/profile_icon_item_0001.png"), "unexpected material image path: %s", got)
	}

	userInterfaceIcon := resources["customProfileUserInterfaceIconResources"].(map[int]map[string]any)[42]
	{
		got := userInterfaceIcon["imagePath"].(string)
		testutil.Require(t, !(got != "asset/cn-assets/startapp/custom_profile/user_interface_icon/profile_icon_0042.png"), "unexpected user interface icon image path: %s", got)
	}

	stamp := resources["stampAssets"].(map[int]map[string]any)[146]
	{
		got := stamp["imagePath"].(string)
		testutil.Require(t, !(got != "asset/cn-assets/startapp/stamp/stamp0230/stamp0230.png"), "unexpected stamp image path: %s", got)
	}

	charaIcons := resources["charaRankIconPathMap"].(map[string]string)
	{
		got := charaIcons["21"]
		testutil.Require(t, !(got != "static_images/chara_icon/miku.png"), "unexpected chara rank icon path: %s", got)
	}

	omikujis := resources["omikujis"].(map[int]map[string]any)
	{
		got := omikujis[183]["fortuneType"].(string)
		testutil.Require(t, !(got != "grate_fortune"), "unexpected omikuji fortune type: %s", got)
	}

	storyFavorites := resources["storyFavoriteResources"].(map[string]any)
	story := storyFavorites["event_story:19"].(map[string]any)
	{
		got := story["title"].(string)
		testutil.Require(t, !(got != "Test Event Story"), "unexpected story favorite title: %s", got)
	}
	{

		got := story["imagePath"].(string)
		testutil.Require(t, !(got != "asset/cn-assets/ondemand/event_story/event_story_test/screen_image/banner_event_story.png"), "unexpected story favorite image path: %s", got)
	}

	unitStory := storyFavorites["unit_story:5"].(map[string]any)
	{
		got := unitStory["title"].(string)
		testutil.Require(t, !(got != "Test Unit Story"), "unexpected unit story title: %s", got)
	}
	{

		got := unitStory["imagePath"].(string)
		testutil.Require(t, !(got != "asset/cn-assets/ondemand/unit_story/main_schoolrefusal_piapro/screen_image/banner_unit_story.png"), "unexpected unit story image path: %s", got)
	}

	cardAsset := resources["cardAssets"].(map[int]map[string]any)[915]
	{
		got := cardAsset["afterTrainingPath"].(string)
		testutil.Require(t, !(got != "asset/cn-assets/startapp/character/member/res010_no034/card_after_training.png"), "unexpected card after-training path: %s", got)
	}
	{

		got := cardAsset["deckAfterTrainingPath"].(string)
		testutil.Require(t, !(got != "asset/cn-assets/startapp/character/member_cutout/res010_no034/after_training.png"), "unexpected deck after-training path: %s", got)
	}
	{

		got := cardAsset["clipAfterTrainingPath"].(string)
		testutil.Require(t, !(got != "asset/cn-assets/startapp/character/member_cutout_trm/res010_no034/after_training.png"), "unexpected clip after-training path: %s", got)
	}
	{

		got := cardAsset["smallAfterTrainingPath"].(string)
		testutil.Require(t, !(got != "asset/cn-assets/startapp/character/member_small/res010_no034/card_after_training.png"), "unexpected small after-training path: %s", got)
	}

	honorRequests := resources["honorRequests"].(map[string]any)
	honorReq := honorRequests["7001:1:main"].(*drawing.HonorRequest)
	{
		testutil.Require(t, !(honorReq.HonorImgPath == nil), "unexpected honor image path: %+v", honorReq.HonorImgPath)
		testutil.Require(t, strings.HasSuffix(*honorReq.HonorImgPath, "honor/honor_bg_event_demo/degree_main.png"), "unexpected honor image path: %+v", honorReq.HonorImgPath)
	}

	bondsHonorRequests := resources["bondsHonorRequests"].(map[string]any)
	bondsReq := bondsHonorRequests["1020501:3:main:10205002:reverse"].(*drawing.HonorRequest)
	{
		testutil.Require(t, !(bondsReq.WordImgPath == nil), "unexpected bonds honor word path: %+v", bondsReq.WordImgPath)
		testutil.Require(t, !(*bondsReq.WordImgPath != "asset/cn-assets/startapp/bonds_honor/word/honorname_0205_default_0502_01.png"), "unexpected bonds honor word path: %+v", bondsReq.WordImgPath)
	}
	{
		testutil.Require(t, !(bondsReq.CharaID == nil), "unexpected reverse bonds honor character order: %+v %+v", bondsReq.CharaID, bondsReq.CharaID2)
		testutil.Require(t, !(*bondsReq.CharaID != "22"), "unexpected reverse bonds honor character order: %+v %+v", bondsReq.CharaID, bondsReq.CharaID2)
		testutil.Require(t, !(bondsReq.CharaID2 == nil), "unexpected reverse bonds honor character order: %+v %+v", bondsReq.CharaID, bondsReq.CharaID2)
		testutil.Require(t, !(*bondsReq.CharaID2 != "11"), "unexpected reverse bonds honor character order: %+v %+v", bondsReq.CharaID, bondsReq.CharaID2)
	}

}

func TestCustomProfileHonorFcApLevelsUseMusicClearCounts(t *testing.T) {
	levels := customProfileHonorFcApLevels(&sekaiapi.GetAnotherProfileResponse{
		UserMusicDifficultyClearCount: []sekaiapi.AnotherUserMusicDifficultyClearCount{
			{MusicDifficultyType: "master", FullCombo: 394, AllPerfect: 12},
			{MusicDifficultyType: "append", FullCombo: 3, AllPerfect: 1},
		},
	})
	{

		got := *levels[3013]
		testutil.Require(t, !(got != 394), "master fc level = %d, want 394", got)
	}
	{

		got := *levels[3014]
		testutil.Require(t, !(got != 12), "master ap level = %d, want 12", got)
	}
	{

		got := *levels[4700]
		testutil.Require(t, !(got != 3), "append fc level = %d, want 3", got)
	}

}

func writeCustomProfileJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	testutil.Require(t, !(err != nil), "marshal %s: %v", path, err)
	{

		err := os.MkdirAll(filepath.Dir(path), 0o755)
		testutil.Require(t, !(err != nil), "mkdir %s: %v", filepath.Dir(path), err)
	}
	{

		err := os.WriteFile(path, data, 0o644)
		testutil.Require(t, !(err != nil), "write %s: %v", path, err)
	}

}
