package requestbuilder

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	"haruki-cloud/internal/pjsk/parser"
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

	req, err := BuildMiscBirthdayRequest(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleMisc,
		Mode:   "misc-birthday",
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

	if req.GachaTime.StartText == "" || req.GachaTime.EndText == "" || req.LiveTime.StartText == "" || req.LiveTime.EndText == "" {
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

	req, err := BuildMiscBirthdayRequest(context.Background(), &parser.ResolvedCommand{
		Module: parser.ModuleMisc,
		Mode:   "misc-birthday",
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
