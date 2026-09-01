package requestbuilder

import (
	"context"
	"fmt"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/testutil"
	"strings"
	"testing"
	"time"

	pjskenttest "haruki-cloud/database/pjsk/enttest"
	sekaienttest "haruki-cloud/database/sekai/enttest"
	"haruki-cloud/internal/pjsk/alias"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"

	_ "github.com/mattn/go-sqlite3"
)

func TestBuildMiscBirthdayRequestFromCharacterID(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:misc_birthday_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = sekaiClient.Close() })
	{

		_, err := sekaiClient.Gamecharacterunit.Create().
			SetServerRegion("jp").
			SetGameCharacterID(21).
			SetColorCode("#33AAFF").
			Save(ctx)
		testutil.Require(t, !(err != nil), "create gamecharacterunit: %v", err)
	}
	{

		_, err := sekaiClient.Card.Create().
			SetServerRegion("jp").
			SetGameID(91001).
			SetCharacterID(21).
			SetCardRarityType("rarity_birthday").
			SetAssetbundleName("birthday_card_test_1").
			SetReleaseAt(1).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create first birthday card: %v", err)
	}
	{

		_, err := sekaiClient.Card.Create().
			SetServerRegion("jp").
			SetGameID(91002).
			SetCharacterID(21).
			SetCardRarityType("rarity_birthday").
			SetAssetbundleName("birthday_card_test_2").
			SetReleaseAt(2).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create second birthday card: %v", err)
	}

	params, err := json.Marshal(miscBirthdaySelection{Cid: 21})
	testutil.Require(t, !(err != nil), "marshal params: %v", err)

	req, err := BuildMiscBirthdayRequest(context.Background(), &CommandInput{
		Region: "jp",
		Params: params,
	}, &renderapp.App{
		Sekai:  sekaiClient,
		Assets: assets.NewAssetHelper(t.TempDir(), nil),
	})
	testutil.Require(t, !(err != nil), "BuildMiscBirthdayRequest() error = %v", err)
	{
		testutil.Require(t, !(req.Cid != 21), "unexpected birthday target: %+v", req)
		testutil.Require(t, !(req.Month != 8), "unexpected birthday target: %+v", req)
		testutil.Require(t, !(req.Day != 31), "unexpected birthday target: %+v", req)
	}
	testutil.Require(t, !(req.RegionName != "日服"), "unexpected region name: %q", req.RegionName)
	testutil.Require(t, !(req.ColorCode != "#33AAFF"), "unexpected color code: %q", req.ColorCode)
	testutil.Require(t, strings.Contains(req.SdImagePath, "character/character_sd_l/chr_sp_21.png"), "unexpected sd path: %q", req.SdImagePath)
	testutil.Require(t, strings.Contains(req.TitleImagePath, "character/label_horizontal/chr_h_lb_21.png"), "unexpected title path: %q", req.TitleImagePath)
	testutil.Require(t, strings.Contains(req.CardImagePath, "birthday_card_test_2/card_normal.png"), "unexpected card image path: %q", req.CardImagePath)
	testutil.Require(t, !(len(req.Cards) != 2), "unexpected cards: %+v", req.Cards)
	{
		testutil.Require(t, !(req.Cards[0].ID != 91001), "unexpected first birthday card: %+v", req.Cards[0])
		testutil.Require(t, strings.Contains(req.Cards[0].ThumbnailPath, "birthday_card_test_1_normal.png"), "unexpected first birthday card: %+v", req.Cards[0])
	}
	{
		testutil.Require(t, !(req.Cards[1].ID != 91002), "unexpected second birthday card: %+v", req.Cards[1])
		testutil.Require(t, strings.Contains(req.Cards[1].ThumbnailPath, "birthday_card_test_2_normal.png"), "unexpected second birthday card: %+v", req.Cards[1])
	}
	{
		testutil.Require(t, !(req.GachaTime.StartAt <= 0), "missing event times: %+v", req)
		testutil.Require(t, !(req.GachaTime.EndAt <= 0), "missing event times: %+v", req)
		testutil.Require(t, !(req.LiveTime.StartAt <= 0), "missing event times: %+v", req)
		testutil.Require(t, !(req.LiveTime.EndAt <= 0), "missing event times: %+v", req)
	}
	{
		testutil.Require(t, req.IsFifthAnniv, "expected fifth anniversary timing payload: %+v", req)
		testutil.Require(t, !(req.DropTime == nil), "expected fifth anniversary timing payload: %+v", req)
		testutil.Require(t, !(req.FlowerTime == nil), "expected fifth anniversary timing payload: %+v", req)
		testutil.Require(t, !(req.PartyTime == nil), "expected fifth anniversary timing payload: %+v", req)
	}
	{
		testutil.Require(t, !(req.DaysUntilBirthday < 0), "unexpected days until birthday: %d", req.DaysUntilBirthday)
		testutil.Require(t, !(req.DaysUntilBirthday > 366), "unexpected days until birthday: %d", req.DaysUntilBirthday)
	}
	testutil.Require(t, !(len(req.AllCharacters) != 26), "unexpected all characters count: %d", len(req.AllCharacters))

	foundMiku := false
	for _, item := range req.AllCharacters {
		if item.Cid != 21 {
			continue
		}
		foundMiku = true
		{
			testutil.Require(t, !(item.Month != 8), "unexpected birthday calendar item: %+v", item)
			testutil.Require(t, !(item.Day != 31), "unexpected birthday calendar item: %+v", item)
		}
		testutil.Require(t, !(item.IconPath != "static_images/chara_icon/miku.png"), "unexpected icon path: %q", item.IconPath)

	}
	testutil.RequireArgs(t, foundMiku, "expected miku in birthday calendar")

}

func TestBuildBirthdayInfosSelectsJune24AfterJune12(t *testing.T) {
	now := time.Date(2026, time.June, 12, 21, 8, 22, 0, birthdayDisplayLocation)
	infos := buildBirthdayInfos(renderregion.JP, now)

	selected, err := selectBirthdayInfo(infos, miscBirthdaySelection{UpcomingIndex: 1})
	testutil.Require(t, !(err != nil), "selectBirthdayInfo() error = %v", err)
	{
		testutil.Require(t, !(selected.Cid != 16), "expected next birthday to be Rui 6/24, got %+v", selected)
		testutil.Require(t, !(selected.Month != 6), "expected next birthday to be Rui 6/24, got %+v", selected)
		testutil.Require(t, !(selected.Day != 24), "expected next birthday to be Rui 6/24, got %+v", selected)
	}

}

func TestBuildBirthdayInfosKeepsCurrentBirthdayOnSameServerDate(t *testing.T) {
	now := time.Date(2026, time.June, 24, 21, 8, 22, 0, birthdayDisplayLocation)
	infos := buildBirthdayInfos(renderregion.JP, now)

	selected, err := selectBirthdayInfo(infos, miscBirthdaySelection{UpcomingIndex: 1})
	testutil.Require(t, !(err != nil), "selectBirthdayInfo() error = %v", err)
	{
		testutil.Require(t, !(selected.Cid != 16), "expected birthday day to keep Rui 6/24, got %+v", selected)
		testutil.Require(t, !(selected.Month != 6), "expected birthday day to keep Rui 6/24, got %+v", selected)
		testutil.Require(t, !(selected.Day != 24), "expected birthday day to keep Rui 6/24, got %+v", selected)
	}
	{

		got := birthdayDaysUntil(now, selected.Next)
		testutil.Require(t, !(got != 0), "expected birthday-day countdown to stay 0, got %d", got)
	}

}

func TestBuildMiscBirthdayRequestFromRawQuery(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:misc_birthday_query_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = sekaiClient.Close() })
	{

		_, err := sekaiClient.Gamecharacterunit.Create().
			SetServerRegion("jp").
			SetGameCharacterID(21).
			SetColorCode("#33AAFF").
			Save(ctx)
		testutil.Require(t, !(err != nil), "create gamecharacterunit: %v", err)
	}
	{

		_, err := sekaiClient.Card.Create().
			SetServerRegion("jp").
			SetGameID(91001).
			SetCharacterID(21).
			SetCardRarityType("rarity_birthday").
			SetAssetbundleName("birthday_card_test_1").
			SetReleaseAt(1).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create birthday card: %v", err)
	}

	req, err := BuildMiscBirthdayRequest(context.Background(), &CommandInput{
		Region: "jp",
		Query:  "miku",
	}, &renderapp.App{
		Sekai:  sekaiClient,
		Assets: assets.NewAssetHelper(t.TempDir(), nil),
	})
	testutil.Require(t, !(err != nil), "BuildMiscBirthdayRequest() error = %v", err)
	testutil.Require(t, !(req.Cid != 21), "unexpected birthday target cid: %d", req.Cid)

}

func TestBuildMiscBirthdayRequestUsesApprovedCharacterAlias(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	sekaiClient := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:misc_birthday_alias_sekai_%d?mode=memory&cache=shared&_fk=1", suffix))
	t.Cleanup(func() { _ = sekaiClient.Close() })
	pjskClient := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:misc_birthday_alias_pjsk_%d?mode=memory&cache=shared&_fk=1", suffix))
	t.Cleanup(func() { _ = pjskClient.Close() })
	{

		_, err := sekaiClient.Gamecharacter.Create().
			SetServerRegion("jp").
			SetGameID(11).
			SetFirstName("东云").
			SetGivenName("彰人").
			SetFirstNameEnglish("Shinonome").
			SetGivenNameEnglish("Akito").
			Save(ctx)
		testutil.Require(t, !(err != nil), "create gamecharacter: %v", err)
	}
	{

		_, err := sekaiClient.Gamecharacterunit.Create().
			SetServerRegion("jp").
			SetGameCharacterID(11).
			SetColorCode("#FFAA33").
			Save(ctx)
		testutil.Require(t, !(err != nil), "create gamecharacterunit: %v", err)
	}
	{

		_, err := sekaiClient.Card.Create().
			SetServerRegion("jp").
			SetGameID(91101).
			SetCharacterID(11).
			SetCardRarityType("rarity_birthday").
			SetAssetbundleName("birthday_card_test_akito").
			SetReleaseAt(1).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create birthday card: %v", err)
	}
	{

		_, err := pjskClient.Alias.Create().
			SetAliasType(alias.PjskAliasTypeCharacter).
			SetAliasTypeID(11).
			SetAlias("akt").
			Save(ctx)
		testutil.Require(t, !(err != nil), "create approved alias: %v", err)
	}

	req, err := BuildMiscBirthdayRequest(context.Background(), &CommandInput{
		Region: "jp",
		Query:  "akt",
	}, &renderapp.App{
		Sekai:   sekaiClient,
		Assets:  assets.NewAssetHelper(t.TempDir(), nil),
		Aliases: alias.NewService(sekaiClient, pjskClient, nil),
	})
	testutil.Require(t, !(err != nil), "BuildMiscBirthdayRequest() error = %v", err)
	testutil.Require(t, !(req.Cid != 11), "unexpected birthday target cid: %d", req.Cid)

}

func TestBuildMiscBirthdayRequestUsesDefaultNicknameBeforeAliasLookup(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	sekaiClient := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:misc_birthday_default_alias_sekai_%d?mode=memory&cache=shared&_fk=1", suffix))
	t.Cleanup(func() { _ = sekaiClient.Close() })
	pjskClient := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:misc_birthday_default_alias_pjsk_%d?mode=memory&cache=shared&_fk=1", suffix))
	t.Cleanup(func() { _ = pjskClient.Close() })
	{

		_, err := sekaiClient.Gamecharacterunit.Create().
			SetServerRegion("jp").
			SetGameCharacterID(20).
			SetColorCode("#DDAACC").
			Save(ctx)
		testutil.Require(t, !(err != nil), "create gamecharacterunit: %v", err)
	}
	{

		_, err := sekaiClient.Card.Create().
			SetServerRegion("jp").
			SetGameID(91201).
			SetCharacterID(20).
			SetCardRarityType("rarity_birthday").
			SetAssetbundleName("birthday_card_test_mizuki").
			SetReleaseAt(1).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create birthday card: %v", err)
	}

	req, err := BuildMiscBirthdayRequest(context.Background(), &CommandInput{
		Region: "jp",
		Query:  "mzk",
	}, &renderapp.App{
		Sekai:   sekaiClient,
		Assets:  assets.NewAssetHelper(t.TempDir(), nil),
		Aliases: alias.NewService(sekaiClient, pjskClient, nil),
	})
	testutil.Require(t, !(err != nil), "BuildMiscBirthdayRequest() error = %v", err)
	testutil.Require(t, !(req.Cid != 20), "unexpected birthday target cid: %d", req.Cid)

}

func TestLookupBirthdayCharactersRegionFallbackAndAmbiguity(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:misc_birthday_lookup_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = client.Close() })
	for _, row := range []struct {
		region string
		id     int64
		first  string
		given  string
	}{
		{region: "jp", id: 2, first: "Local", given: "Hero"},
		{region: "en", id: 3, first: "Remote", given: "Hero"},
		{region: "en", id: 4, first: "Shared", given: "Name"},
		{region: "tw", id: 5, first: "Shared", given: "Name"},
	} {
		{
			_, err := client.Gamecharacter.Create().
				SetServerRegion(row.region).
				SetGameID(row.id).
				SetFirstName(row.first).
				SetGivenName(row.given).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create game character: %v", err)
		}

	}
	app := &renderapp.App{Sekai: client}
	{
		ids, err := lookupBirthdayCharacterIDs(context.Background(), app, renderregion.JP, "LocalHero")
		{
			testutil.Require(t, !(err != nil), "regional lookup = %v, %v", ids, err)
			testutil.Require(t, !(len(ids) != 1), "regional lookup = %v, %v", ids, err)
			testutil.Require(t, !(ids[0] != 2), "regional lookup = %v, %v", ids, err)
		}
	}
	{

		ids, err := lookupBirthdayCharacterIDs(ctx, app, renderregion.JP, "RemoteHero")
		{
			testutil.Require(t, !(err != nil), "fallback lookup = %v, %v", ids, err)
			testutil.Require(t, !(len(ids) != 1), "fallback lookup = %v, %v", ids, err)
			testutil.Require(t, !(ids[0] != 3), "fallback lookup = %v, %v", ids, err)
		}
	}
	{

		_, err := resolveBirthdayCharacterID(ctx, app, renderregion.JP, "SharedName")
		{
			testutil.Require(t, !(err == nil), "ambiguous lookup error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "歧义"), "ambiguous lookup error = %v", err)
		}
	}
	{

		_, err := resolveBirthdayCharacterID(ctx, app, renderregion.JP, "NobodyHere")
		{
			testutil.Require(t, !(err == nil), "missing lookup error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "未找到"), "missing lookup error = %v", err)
		}
	}

}
