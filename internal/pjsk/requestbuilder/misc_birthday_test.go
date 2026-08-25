package requestbuilder

import (
	"context"
	"fmt"
	json "haruki-cloud/internal/jsonutil"
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

	if _, err := sekaiClient.Gamecharacterunit.Create().
		SetServerRegion("jp").
		SetGameCharacterID(21).
		SetColorCode("#33AAFF").
		Save(ctx); err != nil {
		t.Fatalf("create gamecharacterunit: %v", err)
	}

	if _, err := sekaiClient.Card.Create().
		SetServerRegion("jp").
		SetGameID(91001).
		SetCharacterID(21).
		SetCardRarityType("rarity_birthday").
		SetAssetbundleName("birthday_card_test_1").
		SetReleaseAt(1).
		Save(ctx); err != nil {
		t.Fatalf("create first birthday card: %v", err)
	}
	if _, err := sekaiClient.Card.Create().
		SetServerRegion("jp").
		SetGameID(91002).
		SetCharacterID(21).
		SetCardRarityType("rarity_birthday").
		SetAssetbundleName("birthday_card_test_2").
		SetReleaseAt(2).
		Save(ctx); err != nil {
		t.Fatalf("create second birthday card: %v", err)
	}

	params, err := json.Marshal(miscBirthdaySelection{Cid: 21})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	req, err := BuildMiscBirthdayRequest(context.Background(), &CommandInput{
		Region: "jp",
		Params: params,
	}, &renderapp.App{
		Sekai:  sekaiClient,
		Assets: assets.NewAssetHelper(t.TempDir(), nil),
	})
	if err != nil {
		t.Fatalf("BuildMiscBirthdayRequest() error = %v", err)
	}

	if req.Cid != 21 || req.Month != 8 || req.Day != 31 {
		t.Fatalf("unexpected birthday target: %+v", req)
	}
	if req.RegionName != "日服" {
		t.Fatalf("unexpected region name: %q", req.RegionName)
	}
	if req.ColorCode != "#33AAFF" {
		t.Fatalf("unexpected color code: %q", req.ColorCode)
	}
	if !strings.Contains(req.SdImagePath, "character/character_sd_l/chr_sp_21.png") {
		t.Fatalf("unexpected sd path: %q", req.SdImagePath)
	}
	if !strings.Contains(req.TitleImagePath, "character/label_horizontal/chr_h_lb_21.png") {
		t.Fatalf("unexpected title path: %q", req.TitleImagePath)
	}
	if !strings.Contains(req.CardImagePath, "birthday_card_test_2/card_normal.png") {
		t.Fatalf("unexpected card image path: %q", req.CardImagePath)
	}

	if len(req.Cards) != 2 {
		t.Fatalf("unexpected cards: %+v", req.Cards)
	}
	if req.Cards[0].ID != 91001 || !strings.Contains(req.Cards[0].ThumbnailPath, "birthday_card_test_1_normal.png") {
		t.Fatalf("unexpected first birthday card: %+v", req.Cards[0])
	}
	if req.Cards[1].ID != 91002 || !strings.Contains(req.Cards[1].ThumbnailPath, "birthday_card_test_2_normal.png") {
		t.Fatalf("unexpected second birthday card: %+v", req.Cards[1])
	}

	if req.GachaTime.StartAt <= 0 || req.GachaTime.EndAt <= 0 || req.LiveTime.StartAt <= 0 || req.LiveTime.EndAt <= 0 {
		t.Fatalf("missing event times: %+v", req)
	}
	if !req.IsFifthAnniv || req.DropTime == nil || req.FlowerTime == nil || req.PartyTime == nil {
		t.Fatalf("expected fifth anniversary timing payload: %+v", req)
	}
	if req.DaysUntilBirthday < 0 || req.DaysUntilBirthday > 366 {
		t.Fatalf("unexpected days until birthday: %d", req.DaysUntilBirthday)
	}

	if len(req.AllCharacters) != 26 {
		t.Fatalf("unexpected all characters count: %d", len(req.AllCharacters))
	}
	foundMiku := false
	for _, item := range req.AllCharacters {
		if item.Cid != 21 {
			continue
		}
		foundMiku = true
		if item.Month != 8 || item.Day != 31 {
			t.Fatalf("unexpected birthday calendar item: %+v", item)
		}
		if item.IconPath != "static_images/chara_icon/miku.png" {
			t.Fatalf("unexpected icon path: %q", item.IconPath)
		}
	}
	if !foundMiku {
		t.Fatal("expected miku in birthday calendar")
	}
}

func TestBuildBirthdayInfosSelectsJune24AfterJune12(t *testing.T) {
	now := time.Date(2026, time.June, 12, 21, 8, 22, 0, birthdayDisplayLocation)
	infos := buildBirthdayInfos(renderregion.JP, now)

	selected, err := selectBirthdayInfo(infos, miscBirthdaySelection{UpcomingIndex: 1})
	if err != nil {
		t.Fatalf("selectBirthdayInfo() error = %v", err)
	}
	if selected.Cid != 16 || selected.Month != 6 || selected.Day != 24 {
		t.Fatalf("expected next birthday to be Rui 6/24, got %+v", selected)
	}
}

func TestBuildBirthdayInfosKeepsCurrentBirthdayOnSameServerDate(t *testing.T) {
	now := time.Date(2026, time.June, 24, 21, 8, 22, 0, birthdayDisplayLocation)
	infos := buildBirthdayInfos(renderregion.JP, now)

	selected, err := selectBirthdayInfo(infos, miscBirthdaySelection{UpcomingIndex: 1})
	if err != nil {
		t.Fatalf("selectBirthdayInfo() error = %v", err)
	}
	if selected.Cid != 16 || selected.Month != 6 || selected.Day != 24 {
		t.Fatalf("expected birthday day to keep Rui 6/24, got %+v", selected)
	}

	if got := birthdayDaysUntil(now, selected.Next); got != 0 {
		t.Fatalf("expected birthday-day countdown to stay 0, got %d", got)
	}
}

func TestBuildMiscBirthdayRequestFromRawQuery(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:misc_birthday_query_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = sekaiClient.Close() })

	if _, err := sekaiClient.Gamecharacterunit.Create().
		SetServerRegion("jp").
		SetGameCharacterID(21).
		SetColorCode("#33AAFF").
		Save(ctx); err != nil {
		t.Fatalf("create gamecharacterunit: %v", err)
	}

	if _, err := sekaiClient.Card.Create().
		SetServerRegion("jp").
		SetGameID(91001).
		SetCharacterID(21).
		SetCardRarityType("rarity_birthday").
		SetAssetbundleName("birthday_card_test_1").
		SetReleaseAt(1).
		Save(ctx); err != nil {
		t.Fatalf("create birthday card: %v", err)
	}

	req, err := BuildMiscBirthdayRequest(context.Background(), &CommandInput{
		Region: "jp",
		Query:  "miku",
	}, &renderapp.App{
		Sekai:  sekaiClient,
		Assets: assets.NewAssetHelper(t.TempDir(), nil),
	})
	if err != nil {
		t.Fatalf("BuildMiscBirthdayRequest() error = %v", err)
	}
	if req.Cid != 21 {
		t.Fatalf("unexpected birthday target cid: %d", req.Cid)
	}
}

func TestBuildMiscBirthdayRequestUsesApprovedCharacterAlias(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	sekaiClient := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:misc_birthday_alias_sekai_%d?mode=memory&cache=shared&_fk=1", suffix))
	t.Cleanup(func() { _ = sekaiClient.Close() })
	pjskClient := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:misc_birthday_alias_pjsk_%d?mode=memory&cache=shared&_fk=1", suffix))
	t.Cleanup(func() { _ = pjskClient.Close() })

	if _, err := sekaiClient.Gamecharacter.Create().
		SetServerRegion("jp").
		SetGameID(11).
		SetFirstName("东云").
		SetGivenName("彰人").
		SetFirstNameEnglish("Shinonome").
		SetGivenNameEnglish("Akito").
		Save(ctx); err != nil {
		t.Fatalf("create gamecharacter: %v", err)
	}

	if _, err := sekaiClient.Gamecharacterunit.Create().
		SetServerRegion("jp").
		SetGameCharacterID(11).
		SetColorCode("#FFAA33").
		Save(ctx); err != nil {
		t.Fatalf("create gamecharacterunit: %v", err)
	}

	if _, err := sekaiClient.Card.Create().
		SetServerRegion("jp").
		SetGameID(91101).
		SetCharacterID(11).
		SetCardRarityType("rarity_birthday").
		SetAssetbundleName("birthday_card_test_akito").
		SetReleaseAt(1).
		Save(ctx); err != nil {
		t.Fatalf("create birthday card: %v", err)
	}

	if _, err := pjskClient.Alias.Create().
		SetAliasType(alias.PjskAliasTypeCharacter).
		SetAliasTypeID(11).
		SetAlias("akt").
		Save(ctx); err != nil {
		t.Fatalf("create approved alias: %v", err)
	}

	req, err := BuildMiscBirthdayRequest(context.Background(), &CommandInput{
		Region: "jp",
		Query:  "akt",
	}, &renderapp.App{
		Sekai:   sekaiClient,
		Assets:  assets.NewAssetHelper(t.TempDir(), nil),
		Aliases: alias.NewService(sekaiClient, pjskClient, nil),
	})
	if err != nil {
		t.Fatalf("BuildMiscBirthdayRequest() error = %v", err)
	}
	if req.Cid != 11 {
		t.Fatalf("unexpected birthday target cid: %d", req.Cid)
	}
}

func TestBuildMiscBirthdayRequestUsesDefaultNicknameBeforeAliasLookup(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	sekaiClient := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:misc_birthday_default_alias_sekai_%d?mode=memory&cache=shared&_fk=1", suffix))
	t.Cleanup(func() { _ = sekaiClient.Close() })
	pjskClient := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:misc_birthday_default_alias_pjsk_%d?mode=memory&cache=shared&_fk=1", suffix))
	t.Cleanup(func() { _ = pjskClient.Close() })

	if _, err := sekaiClient.Gamecharacterunit.Create().
		SetServerRegion("jp").
		SetGameCharacterID(20).
		SetColorCode("#DDAACC").
		Save(ctx); err != nil {
		t.Fatalf("create gamecharacterunit: %v", err)
	}

	if _, err := sekaiClient.Card.Create().
		SetServerRegion("jp").
		SetGameID(91201).
		SetCharacterID(20).
		SetCardRarityType("rarity_birthday").
		SetAssetbundleName("birthday_card_test_mizuki").
		SetReleaseAt(1).
		Save(ctx); err != nil {
		t.Fatalf("create birthday card: %v", err)
	}

	req, err := BuildMiscBirthdayRequest(context.Background(), &CommandInput{
		Region: "jp",
		Query:  "mzk",
	}, &renderapp.App{
		Sekai:   sekaiClient,
		Assets:  assets.NewAssetHelper(t.TempDir(), nil),
		Aliases: alias.NewService(sekaiClient, pjskClient, nil),
	})
	if err != nil {
		t.Fatalf("BuildMiscBirthdayRequest() error = %v", err)
	}
	if req.Cid != 20 {
		t.Fatalf("unexpected birthday target cid: %d", req.Cid)
	}
}
