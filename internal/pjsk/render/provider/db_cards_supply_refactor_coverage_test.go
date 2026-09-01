package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"

	_ "github.com/mattn/go-sqlite3"
)

func TestDBCardSupplyRefactorBranches(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:card_supply_refactor_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	provider := &dbCardProvider{client: client, region: renderregion.JP}
	if got := provider.GetSupplyType(ctx, nil); got != "normal" {
		t.Fatalf("nil card supply = %q", got)
	}
	if got := provider.GetSupplyType(ctx, &masterdata.Card{CardRarityType: "rarity_birthday"}); got != "birthday" {
		t.Fatalf("birthday card supply = %q", got)
	}
	if got := provider.GetSupplyType(ctx, &masterdata.Card{}); got != "normal" {
		t.Fatalf("zero card supply = %q", got)
	}
	if got := provider.GetSupplyType(ctx, &masterdata.Card{ID: 9, CardSupplyID: 99}); got != "normal" {
		t.Fatalf("missing card supply = %q", got)
	}

	if _, err := client.Cardsupplie.Create().
		SetGameID(7).
		SetCardSupplyType("term_limited").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create supply: %v", err)
	}
	if got := provider.GetSupplyType(ctx, &masterdata.Card{ID: 70, CardSupplyID: 7}); got != "term_limited" {
		t.Fatalf("term-limited card supply = %q", got)
	}
	if !provider.loadWorldLink3Cards(ctx) || provider.isWorldLink3Card(ctx, 0) {
		t.Fatal("fresh empty world-link index mismatch")
	}

	if _, err := client.Event.Create().
		SetGameID(11).
		SetEventType("world_bloom").
		SetUnit("none").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create world-link event: %v", err)
	}
	if _, err := client.Eventcard.Create().
		SetGameID(12).
		SetEventID(11).
		SetCardID(71).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create world-link card: %v", err)
	}
	worldLinkProvider := &dbCardProvider{client: client, region: renderregion.JP}
	card := &masterdata.Card{ID: 71, CardSupplyID: 7}
	if got := worldLinkProvider.GetSupplyType(ctx, card); got != "unit_event_limited" {
		t.Fatalf("world-link card supply = %q", got)
	}
	if got := worldLinkProvider.GetSupplyType(ctx, card); got != "unit_event_limited" {
		t.Fatalf("cached world-link card supply = %q", got)
	}
}
