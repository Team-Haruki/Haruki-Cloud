package handler

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	json "haruki-cloud/internal/jsonutil"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/provider"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/internal/testutil"

	_ "github.com/mattn/go-sqlite3"
)

// The custom profile payload forwards whole master rows to Drawing, so the
// database path must produce byte-identical JSON to the local-file path:
// same camelCase keys, same values, NULL columns absent exactly like keys
// missing from the JSON file.
func TestCustomProfileResourcesDatabasePathMatchesFilePath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	assetHelper := assets.NewAssetHelper(root, nil)

	// File path: flag on, no provider.
	masterRoot := filepath.Join(root, "masterdata")
	master := filepath.Join(masterRoot, "haruki-sekai-sc-master", "master")
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileTextColors.json"), []map[string]any{
		{"id": 10, "seq": 1, "colorCode": "#ffffff"},
		{"id": 99, "seq": 2, "colorCode": "#unused"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileTextFonts.json"), []map[string]any{
		{"id": 11, "name": "ピュア１", "fontName": "FOT-RodinNTLGPro-DB"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileShapeResources.json"), []map[string]any{
		{"id": 12, "customProfileResourceType": "shape", "seq": 3, "name": "図形 : 丸", "pronunciation": "ずけい まる", "resourceLoadType": "assetbundle", "resourceLoadVal": "custom_profile/shape", "fileName": "round"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfilePlayerInfoResources.json"), []map[string]any{
		{"id": 14, "customProfileResourceType": "player_info", "seq": 7, "name": "StoryFavorite", "resourceLoadType": "prefab", "resourceLoadVal": "CustomProfile/ContentView/General", "fileName": "StoryFavorite", "groupId": 1},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileCollectionResources.json"), []map[string]any{
		{"id": 1000, "customProfileResourceType": "collection", "seq": 10, "name": "「2022新春」おみくじ", "resourceLoadType": "assetbundle", "resourceLoadVal": "lottery_game/new_year_2022", "fileName": "Prefabs/Omikuji", "customProfileResourceCollectionType": "omikuji", "groupId": 1},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "omikujis.json"), []map[string]any{
		{"id": 183, "omikujiGroupId": 1, "unit": "piapro", "fortuneType": "grate_fortune", "summary": "人を想う心により様々な\n出会いを経験する良い年\nになるでしょう",
			"title1": "願望", "description1": "必ず叶う", "title2": "健康", "description2": "大変良好\n体を動かすとなお良し", "title3": "待人", "description3": "必ず来る",
			"unitAssetbundleName": "lottery_game/new_year_2022_material", "fortuneAssetbundleName": "lottery_game/new_year_2022_material", "omikujiCoverAssetbundleName": "lottery_game/new_year_2022_material",
			"unitFilePath": "bird_VIRTUAL SINGER", "fortuneFilePath": "unsei_daikichi", "omikujiCoverFilePath": "omikuji_VIRTUAL SINGER"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileCharacterIconResources.json"), []map[string]any{
		{"id": 21, "customProfileResourceType": "character_icon", "seq": 21, "name": "ミク", "pronunciation": "みく", "resourceLoadType": "assetbundle", "resourceLoadVal": "custom_profile/character_icon", "fileName": "profile_chr_icon_miku"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileMaterialResources.json"), []map[string]any{
		{"id": 1, "customProfileResourceType": "material", "seq": 1, "name": "クリスタル", "resourceLoadType": "assetbundle", "resourceLoadVal": "custom_profile/material", "fileName": "profile_icon_item_0001"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileUserInterfaceIconResources.json"), []map[string]any{
		{"id": 42, "customProfileResourceType": "user_interface_icon", "seq": 42, "name": "アイコン42", "resourceLoadType": "assetbundle", "resourceLoadVal": "custom_profile/user_interface_icon", "fileName": "profile_icon_0042"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "eventStories.json"), []map[string]any{
		{"id": 19, "eventId": 190, "outline": "outline", "assetbundleName": "event_story_test"},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "events.json"), []map[string]any{
		{"id": 190, "eventType": "marathon", "name": "Test Event Story", "assetbundleName": "event_test", "startAt": 1, "aggregateAt": 2, "closedAt": 3},
	})
	writeCustomProfileJSONFile(t, filepath.Join(master, "unitStoryEpisodeGroups.json"), []map[string]any{
		{"id": 5, "unit": "piapro", "unitEpisodeCategory": "light_sound", "outline": "Test Unit Story\nSecond line", "assetbundleName": "main_lightsound_piapro"},
	})
	fileApp := &renderapp.App{
		Assets: assetHelper,
		Config: renderapp.Config{LocalMasterdata: renderapp.LocalMasterdataConfig{Enabled: true, AllowFallback: true, Dir: masterRoot}},
	}

	// Database path: flag off, the provider's raw row store on the same SQLite database.
	dsn := fmt.Sprintf("file:custom_profile_golden_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	client := sekaienttest.Open(t, "sqlite3", dsn)
	region := renderregion.CN.String()
	mustSave := func(what string, err error) {
		t.Helper()
		testutil.Require(t, err == nil, "create %s: %v", what, err)
	}
	_, err := client.Customprofiletextcolor.Create().SetGameID(10).SetSeq(1).SetColorCode("#ffffff").SetServerRegion(region).Save(ctx)
	mustSave("text color", err)
	_, err = client.Customprofiletextcolor.Create().SetGameID(99).SetSeq(2).SetColorCode("#unused").SetServerRegion(region).Save(ctx)
	mustSave("unused text color", err)
	_, err = client.Customprofiletextcolor.Create().SetGameID(10).SetSeq(1).SetColorCode("#other-region").SetServerRegion("jp").Save(ctx)
	mustSave("jp text color", err)
	_, err = client.Customprofiletextfont.Create().SetGameID(11).SetName("ピュア１").SetFontName("FOT-RodinNTLGPro-DB").SetServerRegion(region).Save(ctx)
	mustSave("text font", err)
	_, err = client.Customprofileshaperesource.Create().SetGameID(12).SetCustomProfileResourceType("shape").SetSeq(3).SetName("図形 : 丸").SetPronunciation("ずけい まる").
		SetResourceLoadType("assetbundle").SetResourceLoadVal("custom_profile/shape").SetFileName("round").SetServerRegion(region).Save(ctx)
	mustSave("shape", err)
	_, err = client.Customprofileplayerinforesource.Create().SetGameID(14).SetCustomProfileResourceType("player_info").SetSeq(7).SetName("StoryFavorite").
		SetResourceLoadType("prefab").SetResourceLoadVal("CustomProfile/ContentView/General").SetFileName("StoryFavorite").SetGroupID(1).SetServerRegion(region).Save(ctx)
	mustSave("player info", err)
	_, err = client.Customprofilecollectionresource.Create().SetGameID(1000).SetCustomProfileResourceType("collection").SetSeq(10).SetName("「2022新春」おみくじ").
		SetResourceLoadType("assetbundle").SetResourceLoadVal("lottery_game/new_year_2022").SetFileName("Prefabs/Omikuji").SetCustomProfileResourceCollectionType("omikuji").SetGroupID(1).SetServerRegion(region).Save(ctx)
	mustSave("collection", err)
	_, err = client.Omikuji.Create().SetGameID(183).SetOmikujiGroupID(1).SetUnit("piapro").SetFortuneType("grate_fortune").SetSummary("人を想う心により様々な\n出会いを経験する良い年\nになるでしょう").
		SetTitle1("願望").SetDescription1("必ず叶う").SetTitle2("健康").SetDescription2("大変良好\n体を動かすとなお良し").SetTitle3("待人").SetDescription3("必ず来る").
		SetUnitAssetbundleName("lottery_game/new_year_2022_material").SetFortuneAssetbundleName("lottery_game/new_year_2022_material").SetOmikujiCoverAssetbundleName("lottery_game/new_year_2022_material").
		SetUnitFilePath("bird_VIRTUAL SINGER").SetFortuneFilePath("unsei_daikichi").SetOmikujiCoverFilePath("omikuji_VIRTUAL SINGER").SetServerRegion(region).Save(ctx)
	mustSave("omikuji", err)
	_, err = client.Customprofilecharactericonresource.Create().SetGameID(21).SetCustomProfileResourceType("character_icon").SetSeq(21).SetName("ミク").SetPronunciation("みく").
		SetResourceLoadType("assetbundle").SetResourceLoadVal("custom_profile/character_icon").SetFileName("profile_chr_icon_miku").SetServerRegion(region).Save(ctx)
	mustSave("character icon", err)
	_, err = client.Customprofilematerialresource.Create().SetGameID(1).SetCustomProfileResourceType("material").SetSeq(1).SetName("クリスタル").
		SetResourceLoadType("assetbundle").SetResourceLoadVal("custom_profile/material").SetFileName("profile_icon_item_0001").SetServerRegion(region).Save(ctx)
	mustSave("material", err)
	_, err = client.Customprofileuserinterfaceiconresource.Create().SetGameID(42).SetCustomProfileResourceType("user_interface_icon").SetSeq(42).SetName("アイコン42").
		SetResourceLoadType("assetbundle").SetResourceLoadVal("custom_profile/user_interface_icon").SetFileName("profile_icon_0042").SetServerRegion(region).Save(ctx)
	mustSave("ui icon", err)
	_, err = client.Eventstorie.Create().SetGameID(19).SetEventID(190).SetOutline("outline").SetAssetbundleName("event_story_test").SetServerRegion(region).Save(ctx)
	mustSave("event story", err)
	_, err = client.Event.Create().SetGameID(190).SetEventType("marathon").SetName("Test Event Story").SetAssetbundleName("event_test").SetStartAt(1).SetAggregateAt(2).SetClosedAt(3).SetServerRegion(region).Save(ctx)
	mustSave("event", err)
	_, err = client.Unitstoryepisodegroup.Create().SetGameID(5).SetUnit("piapro").SetUnitEpisodeCategory("light_sound").SetOutline("Test Unit Story\nSecond line").SetAssetbundleName("main_lightsound_piapro").SetServerRegion(region).Save(ctx)
	mustSave("unit story episode group", err)

	src := provider.NewDatabaseProvider(client, renderregion.CN, provider.WithSekaiDatabase("sqlite3", dsn))
	t.Cleanup(func() { _ = src.Close() })
	dbApp := &renderapp.App{
		Assets:    assetHelper,
		Provider:  src,
		Providers: map[renderregion.Value]provider.MasterDataProvider{renderregion.CN: src},
	}

	card := sekaiapi.UserCustomProfileCard{
		CustomProfileID: 1, CustomProfileCardID: 1,
		CustomProfileCard: sekaiapi.ProfileCardData{
			Texts:              []sekaiapi.TextData{{FontID: 11, ColorID: 10, OutlineColorID: 10}},
			Shapes:             []sekaiapi.ShapeData{{ID: 12, ColorID: 10}},
			CharacterIcons:     []sekaiapi.ImageData{{ID: 21}},
			Materials:          []sekaiapi.ImageData{{ID: 1}},
			UserInterfaceIcons: []sekaiapi.ImageData{{ID: 42}},
			Collections:        []sekaiapi.CollectionData{{ID: 1000, TargetID: 183}},
			Generals:           []sekaiapi.GeneralData{{PlayerInfoResourceID: 14}},
		},
	}
	resp := &sekaiapi.GetAnotherProfileResponse{
		UserStoryFavorites: []sekaiapi.UserStoryFavorite{{StoryType: "event_story", StoryID: 19}, {StoryType: "unit_story", StoryID: 5}},
	}

	fromFiles, err := buildCustomProfileResources(ctx, fileApp, "cn", card, resp)
	testutil.Require(t, err == nil, "file path: %v", err)
	fromDB, err := buildCustomProfileResources(ctx, dbApp, "cn", card, resp)
	testutil.Require(t, err == nil, "database path: %v", err)

	fileJSON, err := json.Marshal(fromFiles)
	testutil.Require(t, err == nil, "marshal file payload: %v", err)
	dbJSON, err := json.Marshal(fromDB)
	testutil.Require(t, err == nil, "marshal database payload: %v", err)
	if !bytes.Equal(fileJSON, dbJSON) {
		t.Fatalf("database payload differs from file payload\nfiles: %s\ndb:    %s", fileJSON, dbJSON)
	}

	payload := string(dbJSON)
	for _, want := range []string{
		`"omikujiCoverFilePath":"omikuji_VIRTUAL SINGER"`, `"summary":"人を想う心により様々な\n出会いを経験する良い年\nになるでしょう"`,
		`"colorCode":"#ffffff"`, `"fontName":"FOT-RodinNTLGPro-DB"`, `"title":"Test Event Story"`, `"title":"Test Unit Story"`,
		`custom_profile/shape/round.png`, `"groupId":1`,
	} {
		testutil.Require(t, strings.Contains(payload, want), "payload lacks %s: %s", want, payload)
	}
	testutil.Require(t, !strings.Contains(payload, "#unused") && !strings.Contains(payload, "#other-region"), "payload leaked unrequested or other-region rows: %s", payload)
	_, hasOmikujiCache := fromDB["omikujis"].(map[int]map[string]any)[183]["title2"]
	testutil.Require(t, hasOmikujiCache, "omikuji row lost a column on the database path: %#v", fromDB["omikujis"])
}

// With the flag off and no row source, the handler must not read local files.
func TestCustomProfileMasterTableRequiresSourceWhenFallbackIsOff(t *testing.T) {
	root := t.TempDir()
	master := filepath.Join(root, "haruki-sekai-master", "master")
	writeCustomProfileJSONFile(t, filepath.Join(master, "customProfileTextColors.json"), []map[string]any{{"id": 1, "colorCode": "#000"}})
	app := &renderapp.App{Config: renderapp.Config{LocalMasterdata: renderapp.LocalMasterdataConfig{Dir: root}}}
	_, err := loadCustomProfileMasterTable(context.Background(), app, renderregion.JP, "customProfileTextColors.json", map[int]struct{}{1: {}})
	testutil.Require(t, err != nil, "flag off read local files")

	app.Config.LocalMasterdata.Enabled = true
	app.Config.LocalMasterdata.AllowFallback = true
	rows, err := loadCustomProfileMasterTable(context.Background(), app, renderregion.JP, "customProfileTextColors.json", map[int]struct{}{1: {}})
	testutil.Require(t, err == nil && rows[1]["colorCode"] == "#000", "flag on local fallback = %#v, %v", rows, err)
}
